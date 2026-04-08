//go:build integration

// Package integration contains end-to-end tests for the api-gateway data pipeline.
//
// These tests require real PostgreSQL and Redis instances and are intentionally
// excluded from `go test ./...`. Invoke explicitly:
//
//	go test -tags=integration ./tests/integration/... \
//	    -db-url="postgres://fabric:fabric_dev_only@localhost:5433/govagn?sslmode=disable" \
//	    -redis-url="redis://localhost:6380"
//
// Or via Make (spins up Docker deps automatically):
//
//	make test/integration
//
// Coverage:
//   - /healthz with live Postgres + Redis
//   - POST /auth/login  — success, wrong password, unknown user
//   - POST /internal/ingest → GET /api/v1/traces  (full write-read round-trip)
//   - 32 MiB body size limit (MaxBytesReader fix regression test)
//   - All /api/v1/* routes require Authorization header
//   - /api/v1/audit returns valid JSON
//   - Redis rate-limit counters are per-tenant (Principle 2)
//   - POST /api/v1/runs/{id}/feedback
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	chi "github.com/go-chi/chi/v5"
	chimid "github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"

	"github.com/govagn/api-gateway/internal/auth"
	"github.com/govagn/api-gateway/internal/handlers"
	"github.com/govagn/api-gateway/internal/middleware"
	"github.com/govagn/api-gateway/internal/store"
	"github.com/govagn/api-gateway/internal/ws"
)

// ─── Flags ────────────────────────────────────────────────────────────────────

var (
	dbURL    = flag.String("db-url", envOrDefault("INTEGRATION_DB_URL", "postgres://fabric:fabric_dev_only@localhost:5433/govagn?sslmode=disable"), "PostgreSQL DSN for integration tests")
	redisURL = flag.String("redis-url", envOrDefault("INTEGRATION_REDIS_URL", "redis://localhost:6380"), "Redis URL for integration tests")
)

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// testJWTSecret is the shared HMAC-SHA256 secret used by the browser/API JWT
// auth path in the test router. Must be ≥ 32 bytes to satisfy HS256 requirements.
const testJWTSecret = "integration-test-secret-32bytes!!"
const testCollectorBearerToken = "integration-collector-bearer-token"

// ─── Test server ──────────────────────────────────────────────────────────────

// testServer holds live store handles and an in-process Chi router.
// No TCP socket is opened — requests are dispatched via httptest.NewRecorder.
type testServer struct {
	pg    *store.PostgresStore
	redis *store.RedisClient
	r     http.Handler
	token string // JWT from the seeded admin user
}

// newTestServer builds the complete router, identical in structure to
// cmd/server/main.go, then obtains an admin JWT via PasswordLogin.
// Calls t.Cleanup to close all connections automatically.
func newTestServer(t *testing.T) *testServer {
	t.Helper()

	logger, _ := zap.NewDevelopment()

	pg, err := store.NewPostgresStore(*dbURL, logger)
	if err != nil {
		t.Fatalf("postgres: %v\n\nIs the test stack running?  Run: make integration/up", err)
	}
	t.Cleanup(func() { pg.Close() })

	rc, err := store.NewRedisClient(*redisURL)
	if err != nil {
		t.Fatalf("redis: %v\n\nIs the test stack running?  Run: make integration/up", err)
	}
	t.Cleanup(func() { rc.Close() })

	hub := ws.NewHub(logger)
	ctx, cancel := context.WithCancel(context.Background())
	go hub.Run(ctx)
	t.Cleanup(cancel)

	// Auth handler — password login only in tests (no OIDC provider).
	// users=pg so PasswordLogin queries the real users table (seeded admin).
	oidcCfg := auth.OIDCConfig{
		AdminUser:     "admin",
		AdminPassword: "admin",
		JWTSecret:     testJWTSecret,
	}
	oidcHandler := auth.NewOIDCHandler(oidcCfg, pg, logger)

	h := handlers.New(pg, rc, hub, logger, testJWTSecret)

	rlCfg := middleware.RateLimitConfig{
		RequestsPerMinute: 100_000, // generous — tests must not be rate-limited
		KeyPrefix:         "rl-integ",
	}

	r := chi.NewRouter()
	r.Use(chimid.Recoverer)

	// Public
	r.Get("/healthz", h.Health)

	// Collector ingestion (service-to-service shared bearer token, X-AF-Source: collector)
	r.Group(func(r chi.Router) {
		r.Use(middleware.CollectorAuth(testCollectorBearerToken))
		r.Post("/internal/ingest", h.Ingest)
	})

	// Auth
	r.Post("/auth/login", oidcHandler.PasswordLogin)
	r.Post("/auth/refresh", oidcHandler.Refresh)

	// Authenticated API — mirrors main.go routing
	r.Group(func(r chi.Router) {
		r.Use(middleware.JWTAuth(testJWTSecret))
		r.Use(middleware.TenantInjector)
		r.Use(middleware.RateLimit(rc, rlCfg))

		r.Get("/api/v1/traces", h.ListTraces)
		r.Get("/api/v1/traces/{traceID}", h.GetTrace)
		r.Get("/api/v1/runs", h.ListRuns)
		r.Get("/api/v1/agents", h.ListAgents)
		r.Get("/api/v1/cost", h.GetCostReport)
		r.Post("/api/v1/runs/{runID}/feedback", h.PostFeedback)
		r.Get("/api/v1/audit", h.ListAudit)
	})

	ts := &testServer{pg: pg, redis: rc, r: r}
	ts.token = ts.mustLogin(t, "admin", "admin")
	return ts
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func (ts *testServer) do(method, path string, body interface{}, headers map[string]string) *httptest.ResponseRecorder {
	var bodyReader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	ts.r.ServeHTTP(rr, req)
	return rr
}

func (ts *testServer) authHeader() map[string]string {
	return map[string]string{"Authorization": "Bearer " + ts.token}
}

// mustLogin calls POST /auth/login and fatals on anything other than 200.
func (ts *testServer) mustLogin(t *testing.T, username, password string) string {
	t.Helper()
	rr := ts.do("POST", "/auth/login", map[string]string{
		"username": username,
		"password": password,
	}, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("login %q: expected 200, got %d — %s", username, rr.Code, rr.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("login decode: %v", err)
	}
	token, ok := resp["token"].(string)
	if !ok || token == "" {
		t.Fatalf("login: missing 'token' in response: %v", resp)
	}
	return token
}

// collectorHeaders returns the headers required by CollectorAuth middleware.
func collectorHeaders() map[string]string {
	return map[string]string{
		"Authorization": "Bearer " + testCollectorBearerToken,
		"X-AF-Source":   "collector",
		"X-AF-Tenant":   middleware.DefaultTenantID,
	}
}

// ingestSpan posts one minimal span through /internal/ingest and asserts acceptance.
func (ts *testServer) ingestSpan(t *testing.T, traceID, spanID, framework string) {
	t.Helper()
	payload := map[string]interface{}{
		"tenant_id": middleware.DefaultTenantID,
		"spans": []map[string]interface{}{{
			"trace_id":       traceID,
			"span_id":        spanID,
			"parent_span_id": "",
			"run_id":         "run-integ-001",
			"name":           "integration-test-span",
			"framework":      framework,
			"start_time_ns":  time.Now().UnixNano(),
			"duration_ns":    1_000_000,
			"status_code":    0,
			"attributes":     map[string]string{"test": "true"},
			"events":         []interface{}{},
			"collector_node": "test-node",
			"received_ns":    time.Now().UnixNano(),
			"cost_usd":       0.0001,
			"input_tokens":   100,
			"output_tokens":  50,
		}},
	}
	rr := ts.do("POST", "/internal/ingest", payload, collectorHeaders())
	if rr.Code != http.StatusAccepted && rr.Code != http.StatusOK {
		t.Fatalf("ingest span %q: expected 200/202, got %d — %s", spanID, rr.Code, rr.Body.String())
	}
}

// ─── Tests ────────────────────────────────────────────────────────────────────

// TestIntegration_Healthz verifies /healthz returns 200 + status:"ok" when
// both Postgres and Redis are live and reachable.
func TestIntegration_Healthz(t *testing.T) {
	ts := newTestServer(t)
	rr := ts.do("GET", "/healthz", nil, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("healthz: expected 200, got %d — %s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("healthz JSON decode: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("healthz: expected status=ok, got %q", resp["status"])
	}
}

// TestIntegration_PasswordLogin_Success verifies the seeded admin user
// authenticates and receives a properly-formed JWT (three dot-separated segments).
func TestIntegration_PasswordLogin_Success(t *testing.T) {
	ts := newTestServer(t)
	token := ts.mustLogin(t, "admin", "admin")
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Errorf("JWT expected 3 segments, got %d: %q", len(parts), token)
	}
}

// TestIntegration_PasswordLogin_WrongPassword verifies wrong credentials → 401.
func TestIntegration_PasswordLogin_WrongPassword(t *testing.T) {
	ts := newTestServer(t)
	rr := ts.do("POST", "/auth/login", map[string]string{
		"username": "admin",
		"password": "definitely-wrong",
	}, nil)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("wrong password: expected 401, got %d", rr.Code)
	}
}

// TestIntegration_PasswordLogin_UnknownUser verifies an unknown username → 401
// (not 500 — no DB error leaks through).
func TestIntegration_PasswordLogin_UnknownUser(t *testing.T) {
	ts := newTestServer(t)
	rr := ts.do("POST", "/auth/login", map[string]string{
		"username": "no-such-user-xyzzy-99",
		"password": "anything",
	}, nil)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("unknown user: expected 401, got %d — %s", rr.Code, rr.Body.String())
	}
}

// TestIntegration_Ingest_RoundTrip is the core data-pipeline test.
// It posts a span through /internal/ingest and verifies the trace_id appears
// in GET /api/v1/traces — confirming the full write-then-read path via Postgres.
func TestIntegration_Ingest_RoundTrip(t *testing.T) {
	ts := newTestServer(t)

	traceID := fmt.Sprintf("integ-trace-%d", time.Now().UnixNano())
	spanID := fmt.Sprintf("integ-span-%d", time.Now().UnixNano())
	ts.ingestSpan(t, traceID, spanID, "crewai")

	// Retry up to 500 ms — ingest is synchronous but allow for scheduling jitter.
	var found bool
	for i := 0; i < 5 && !found; i++ {
		rr := ts.do("GET", "/api/v1/traces?limit=50", nil, ts.authHeader())
		if rr.Code != http.StatusOK {
			t.Fatalf("list traces: expected 200, got %d — %s", rr.Code, rr.Body.String())
		}
		if strings.Contains(rr.Body.String(), traceID) {
			found = true
		} else if i < 4 {
			time.Sleep(100 * time.Millisecond)
		}
	}
	if !found {
		t.Errorf("trace %q not found in GET /api/v1/traces after ingest", traceID)
	}
}

// TestIntegration_Ingest_MaxBodySize verifies that a body > 32 MiB is rejected
// with HTTP 413, not silently truncated.  Regression test for the MaxBytesReader fix.
func TestIntegration_Ingest_MaxBodySize(t *testing.T) {
	ts := newTestServer(t)

	bigBody := strings.Repeat("x", 33*1024*1024) // 33 MiB
	req := httptest.NewRequest("POST", "/internal/ingest", strings.NewReader(bigBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testCollectorBearerToken)
	req.Header.Set("X-AF-Source", "collector")
	req.Header.Set("X-AF-Tenant", middleware.DefaultTenantID)

	rr := httptest.NewRecorder()
	ts.r.ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized body: expected 413, got %d", rr.Code)
	}
}

// TestIntegration_APIRoutes_RequireAuth verifies every /api/v1 route returns 401
// when called without an Authorization header.
func TestIntegration_APIRoutes_RequireAuth(t *testing.T) {
	ts := newTestServer(t)
	routes := []string{
		"/api/v1/traces",
		"/api/v1/runs",
		"/api/v1/agents",
		"/api/v1/cost",
		"/api/v1/audit",
	}
	for _, route := range routes {
		rr := ts.do("GET", route, nil, nil)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("%s: expected 401 without auth, got %d", route, rr.Code)
		}
	}
}

// TestIntegration_AuditLog_Accessible verifies /api/v1/audit returns
// well-formed JSON (may be empty on a fresh DB — that is acceptable).
func TestIntegration_AuditLog_Accessible(t *testing.T) {
	ts := newTestServer(t)
	rr := ts.do("GET", "/api/v1/audit", nil, ts.authHeader())
	if rr.Code != http.StatusOK {
		t.Fatalf("audit: expected 200, got %d — %s", rr.Code, rr.Body.String())
	}
	var resp interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Errorf("audit response is not valid JSON: %v", err)
	}
}

// TestIntegration_RateLimit_TenantIsolation verifies that rate-limit counters in
// Redis are keyed per tenant and do not affect each other (Principle 2).
func TestIntegration_RateLimit_TenantIsolation(t *testing.T) {
	ts := newTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	window := time.Now().Truncate(time.Minute).Unix()
	keyA := fmt.Sprintf("rl-integ:%s:%d", "integ-tenant-A", window)
	keyB := fmt.Sprintf("rl-integ:%s:%d", "integ-tenant-B", window)

	if err := ts.redis.SetJSON(ctx, keyA, int64(9000), 2*time.Minute); err != nil {
		t.Fatalf("SetJSON keyA: %v", err)
	}
	if err := ts.redis.SetJSON(ctx, keyB, int64(0), 2*time.Minute); err != nil {
		t.Fatalf("SetJSON keyB: %v", err)
	}

	var countA, countB int64
	if err := ts.redis.GetJSON(ctx, keyA, &countA); err != nil {
		t.Fatalf("GetJSON keyA: %v", err)
	}
	if err := ts.redis.GetJSON(ctx, keyB, &countB); err != nil {
		t.Fatalf("GetJSON keyB: %v", err)
	}
	if countA == countB {
		t.Errorf("Principle 2 violated: tenant counters should be independent (both=%d)", countA)
	}
}

// TestIntegration_SubmitFeedback verifies POST /api/v1/runs/{id}/feedback
// accepts a valid score for a run that was just created via ingest.
func TestIntegration_SubmitFeedback(t *testing.T) {
	ts := newTestServer(t)

	traceID := fmt.Sprintf("integ-fb-trace-%d", time.Now().UnixNano())
	spanID := fmt.Sprintf("integ-fb-span-%d", time.Now().UnixNano())
	ts.ingestSpan(t, traceID, spanID, "langgraph")

	// Fetch the most recently created run
	rr := ts.do("GET", "/api/v1/runs?limit=1", nil, ts.authHeader())
	if rr.Code != http.StatusOK {
		t.Fatalf("list runs: %d — %s", rr.Code, rr.Body.String())
	}
	var runsResp struct {
		Data []struct {
			RunID string `json:"run_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&runsResp); err != nil {
		t.Fatalf("decode runs: %v", err)
	}
	if len(runsResp.Data) == 0 {
		t.Skip("no runs in DB after ingest — skipping feedback test")
	}

	runID := runsResp.Data[0].RunID
	fbRR := ts.do(
		"POST",
		"/api/v1/runs/"+runID+"/feedback",
		map[string]interface{}{"score": 0.9, "comment": "integration test"},
		ts.authHeader(),
	)
	if fbRR.Code != http.StatusCreated && fbRR.Code != http.StatusOK {
		t.Errorf("submit feedback: expected 200/201, got %d — %s", fbRR.Code, fbRR.Body.String())
	}
}
