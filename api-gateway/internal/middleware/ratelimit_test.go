package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ─── Mock RateLimitStore ──────────────────────────────────────────────────────

// mockRateLimitStore is an in-memory stub implementing RateLimitStore.
// mu protects counts against concurrent map writes (required by TestRateLimit_Concurrent_NoPanic).
type mockRateLimitStore struct {
	mu      sync.Mutex
	counts  map[string]int64
	failErr error // if non-nil, IncrWithExpiry returns this error
}

func newMockStore() *mockRateLimitStore {
	return &mockRateLimitStore{counts: make(map[string]int64)}
}

func (m *mockRateLimitStore) IncrWithExpiry(_ context.Context, key string, _ time.Duration) (int64, error) {
	if m.failErr != nil {
		return 0, m.failErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counts[key]++
	return m.counts[key], nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func requestWithTenant(tenantID string) *http.Request {
	req := httptest.NewRequest("GET", "/api/v1/traces", nil)
	ctx := context.WithValue(req.Context(), tenantIDKey, tenantID)
	return req.WithContext(ctx)
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestRateLimit_FirstRequest_Passes(t *testing.T) {
	store := newMockStore()
	cfg := DefaultRateLimitConfig()
	cfg.RequestsPerMinute = 10

	rr := httptest.NewRecorder()
	RateLimit(store, cfg)(http.HandlerFunc(okHandler)).ServeHTTP(rr, requestWithTenant("tenant-a"))

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestRateLimit_BelowLimit_Passes(t *testing.T) {
	store := newMockStore()
	cfg := DefaultRateLimitConfig()
	cfg.RequestsPerMinute = 5

	for i := 0; i < 5; i++ {
		rr := httptest.NewRecorder()
		RateLimit(store, cfg)(http.HandlerFunc(okHandler)).ServeHTTP(rr, requestWithTenant("tenant-b"))
		if rr.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i, rr.Code)
		}
	}
}

func TestRateLimit_ExceedsLimit_Returns429(t *testing.T) {
	store := newMockStore()
	cfg := DefaultRateLimitConfig()
	cfg.RequestsPerMinute = 3

	var lastStatus int
	for i := 0; i < 5; i++ {
		rr := httptest.NewRecorder()
		RateLimit(store, cfg)(http.HandlerFunc(okHandler)).ServeHTTP(rr, requestWithTenant("tenant-c"))
		lastStatus = rr.Code
	}

	if lastStatus != http.StatusTooManyRequests {
		t.Errorf("expected 429 after exceeding limit, got %d", lastStatus)
	}
}

func TestRateLimit_429_IncludesRetryAfterHeader(t *testing.T) {
	store := newMockStore()
	cfg := DefaultRateLimitConfig()
	cfg.RequestsPerMinute = 1

	// First request OK, second should be rate limited
	RateLimit(store, cfg)(http.HandlerFunc(okHandler)).ServeHTTP(httptest.NewRecorder(), requestWithTenant("tenant-d"))
	rr := httptest.NewRecorder()
	RateLimit(store, cfg)(http.HandlerFunc(okHandler)).ServeHTTP(rr, requestWithTenant("tenant-d"))

	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rr.Code)
	}
	if rr.Header().Get("Retry-After") == "" {
		t.Error("429 response must include Retry-After header")
	}
}

func TestRateLimit_429_IncludesRateLimitHeaders(t *testing.T) {
	store := newMockStore()
	cfg := DefaultRateLimitConfig()
	cfg.RequestsPerMinute = 1

	RateLimit(store, cfg)(http.HandlerFunc(okHandler)).ServeHTTP(httptest.NewRecorder(), requestWithTenant("tenant-e"))
	rr := httptest.NewRecorder()
	RateLimit(store, cfg)(http.HandlerFunc(okHandler)).ServeHTTP(rr, requestWithTenant("tenant-e"))

	if rr.Header().Get("X-RateLimit-Limit") == "" {
		t.Error("missing X-RateLimit-Limit header")
	}
	if rr.Header().Get("X-RateLimit-Remaining") == "" {
		t.Error("missing X-RateLimit-Remaining header")
	}
	if rr.Header().Get("X-RateLimit-Reset") == "" {
		t.Error("missing X-RateLimit-Reset header")
	}
}

func TestRateLimit_P2_DifferentTenants_IndependentCounters(t *testing.T) {
	// Principle 2: tenant isolation — exhausting one tenant's quota must not affect others
	store := newMockStore()
	cfg := DefaultRateLimitConfig()
	cfg.RequestsPerMinute = 2

	// Exhaust tenant-X limit
	for i := 0; i < 5; i++ {
		RateLimit(store, cfg)(http.HandlerFunc(okHandler)).ServeHTTP(httptest.NewRecorder(), requestWithTenant("tenant-X"))
	}

	// tenant-Y should still work
	rr := httptest.NewRecorder()
	RateLimit(store, cfg)(http.HandlerFunc(okHandler)).ServeHTTP(rr, requestWithTenant("tenant-Y"))

	if rr.Code != http.StatusOK {
		t.Errorf("Principle 2 violation: tenant-Y rate limited due to tenant-X exhaustion, got %d", rr.Code)
	}
}

func TestRateLimit_RedisFailure_FailsOpen(t *testing.T) {
	// When Redis is unavailable, fail open (don't block legitimate traffic)
	store := &mockRateLimitStore{
		counts:  make(map[string]int64),
		failErr: fmt.Errorf("redis connection refused"),
	}
	cfg := DefaultRateLimitConfig()

	rr := httptest.NewRecorder()
	RateLimit(store, cfg)(http.HandlerFunc(okHandler)).ServeHTTP(rr, requestWithTenant("tenant-f"))

	if rr.Code != http.StatusOK {
		t.Errorf("Redis failure should fail open (200), got %d", rr.Code)
	}
}

func TestRateLimit_EmptyTenantID_UsesDefault(t *testing.T) {
	store := newMockStore()
	cfg := DefaultRateLimitConfig()
	cfg.RequestsPerMinute = 100

	// No tenant_id in context
	req := httptest.NewRequest("GET", "/api/v1/traces", nil)
	rr := httptest.NewRecorder()
	RateLimit(store, cfg)(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("empty tenant should use 'default' and pass, got %d", rr.Code)
	}
}

func TestRateLimit_KeyIncludesWindowMinute(t *testing.T) {
	// Key format: rl:tenant:{tenantID}:{windowMinute} and rl:user:{tenantID}:{userID}:{windowMinute}
	// This ensures different minutes have independent counters
	store := newMockStore()
	cfg := RateLimitConfig{RequestsPerMinute: 1, KeyPrefix: "rl"}

	// First request
	RateLimit(store, cfg)(http.HandlerFunc(okHandler)).ServeHTTP(httptest.NewRecorder(), requestWithTenant("tenant-g"))

	// Verify the keys have been stored with the expected format
	windowMinute := time.Now().Truncate(time.Minute).Unix()
	expectedTenantKey := fmt.Sprintf("rl:tenant:tenant-g:%d", windowMinute)
	expectedUserKey := fmt.Sprintf("rl:user:tenant-g:anonymous:%d", windowMinute)

	if _, exists := store.counts[expectedTenantKey]; !exists {
		t.Errorf("expected tenant key %q in store, got keys: %v", expectedTenantKey, store.counts)
	}

	if _, exists := store.counts[expectedUserKey]; !exists {
		t.Errorf("expected user key %q in store, got keys: %v", expectedUserKey, store.counts)
	}
}

func TestRateLimit_RemainingHeader_DecreasesWithRequests(t *testing.T) {
	store := newMockStore()
	cfg := DefaultRateLimitConfig()
	cfg.RequestsPerMinute = 10

	var lastRemaining string
	for i := 0; i < 3; i++ {
		rr := httptest.NewRecorder()
		RateLimit(store, cfg)(http.HandlerFunc(okHandler)).ServeHTTP(rr, requestWithTenant("tenant-h"))
		lastRemaining = rr.Header().Get("X-RateLimit-Remaining")
	}

	if lastRemaining != "7" {
		t.Errorf("expected remaining=7 after 3 requests with limit=10, got %q", lastRemaining)
	}
}

func TestRateLimit_Concurrent_NoPanic(t *testing.T) {
	// Verify no race conditions under concurrent access
	store := newMockStore()
	cfg := DefaultRateLimitConfig()
	cfg.RequestsPerMinute = 10000

	var wg atomic.Int64
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Add(-1)
			rr := httptest.NewRecorder()
			RateLimit(store, cfg)(http.HandlerFunc(okHandler)).ServeHTTP(rr, requestWithTenant(fmt.Sprintf("t%d", id%5)))
		}(i)
	}
	// Wait for goroutines (simple spin; sufficient for unit test)
	for wg.Load() > 0 {
		time.Sleep(time.Millisecond)
	}
}

// ─── Per-User Rate Limiting Tests ─────────────────────────────────────────────

func requestWithTenantAndClaims(tenantID, userID string) *http.Request {
	req := httptest.NewRequest("GET", "/api/v1/traces", nil)
	ctx := context.WithValue(req.Context(), tenantIDKey, tenantID)
	claims := &Claims{
		TenantID: tenantID,
		Email:    userID + "@example.com",
		Name:     "Test User",
		Role:     "viewer",
	}
	claims.Subject = userID
	ctx = context.WithValue(ctx, claimsKey, claims)
	return req.WithContext(ctx)
}

func TestRateLimit_PerUser_IsolatesUserCounters(t *testing.T) {
	// Different users should have independent per-user counters
	store := newMockStore()
	cfg := DefaultRateLimitConfig()
	cfg.RequestsPerMinute = 100
	cfg.RequestsPerUserPerMinute = 5

	// User A: 5 requests (at limit)
	for i := 0; i < 5; i++ {
		rr := httptest.NewRecorder()
		RateLimit(store, cfg)(http.HandlerFunc(okHandler)).ServeHTTP(rr, requestWithTenantAndClaims("tenant-1", "user-a"))
		if rr.Code != http.StatusOK {
			t.Errorf("user-a request %d: expected 200, got %d", i, rr.Code)
		}
	}

	// User A: 6th request (exceeds limit)
	rr := httptest.NewRecorder()
	RateLimit(store, cfg)(http.HandlerFunc(okHandler)).ServeHTTP(rr, requestWithTenantAndClaims("tenant-1", "user-a"))
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("user-a 6th request: expected 429, got %d", rr.Code)
	}

	// User B: Should still have quota
	rr = httptest.NewRecorder()
	RateLimit(store, cfg)(http.HandlerFunc(okHandler)).ServeHTTP(rr, requestWithTenantAndClaims("tenant-1", "user-b"))
	if rr.Code != http.StatusOK {
		t.Errorf("user-b 1st request: expected 200, got %d", rr.Code)
	}
}

func TestRateLimit_PerUser_PerTenant(t *testing.T) {
	// Users in different tenants should have independent limits
	store := newMockStore()
	cfg := DefaultRateLimitConfig()
	cfg.RequestsPerMinute = 100
	cfg.RequestsPerUserPerMinute = 3

	// User X in Tenant A: 3 requests
	for i := 0; i < 3; i++ {
		rr := httptest.NewRecorder()
		RateLimit(store, cfg)(http.HandlerFunc(okHandler)).ServeHTTP(rr, requestWithTenantAndClaims("tenant-a", "user-x"))
		if rr.Code != http.StatusOK {
			t.Errorf("tenant-a/user-x request %d: expected 200, got %d", i, rr.Code)
		}
	}

	// User X in Tenant A: 4th request (exceeds limit)
	rr := httptest.NewRecorder()
	RateLimit(store, cfg)(http.HandlerFunc(okHandler)).ServeHTTP(rr, requestWithTenantAndClaims("tenant-a", "user-x"))
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("tenant-a/user-x 4th request: expected 429, got %d", rr.Code)
	}

	// User X in Tenant B: Should still have quota (independent tenant)
	rr = httptest.NewRecorder()
	RateLimit(store, cfg)(http.HandlerFunc(okHandler)).ServeHTTP(rr, requestWithTenantAndClaims("tenant-b", "user-x"))
	if rr.Code != http.StatusOK {
		t.Errorf("tenant-b/user-x 1st request: expected 200, got %d", rr.Code)
	}
}

func TestRateLimit_PerEndpoint_IngestLimited(t *testing.T) {
	// /v1/ingest endpoint should have separate per-endpoint limit
	store := newMockStore()
	cfg := DefaultRateLimitConfig()
	cfg.RequestsPerMinute = 1000
	cfg.RequestsPerUserPerMinute = 1000
	cfg.RequestsPerEndpointPerUserPerMinute = 3

	userID := "user-ingest-test"
	tenantID := "tenant-ingest"

	// Create requests for /v1/ingest endpoint
	ingestRequest := func() *http.Request {
		req := httptest.NewRequest("POST", "/v1/ingest", nil)
		ctx := context.WithValue(req.Context(), tenantIDKey, tenantID)
		claims := &Claims{
			TenantID: tenantID,
			Email:    userID + "@example.com",
			Name:     "Test User",
			Role:     "editor",
		}
		claims.Subject = userID
		ctx = context.WithValue(ctx, claimsKey, claims)
		return req.WithContext(ctx)
	}

	// First 3 requests should pass
	for i := 0; i < 3; i++ {
		rr := httptest.NewRecorder()
		RateLimit(store, cfg)(http.HandlerFunc(okHandler)).ServeHTTP(rr, ingestRequest())
		if rr.Code != http.StatusOK {
			t.Errorf("ingest request %d: expected 200, got %d", i, rr.Code)
		}
	}

	// 4th request should exceed per-endpoint limit
	rr := httptest.NewRecorder()
	RateLimit(store, cfg)(http.HandlerFunc(okHandler)).ServeHTTP(rr, ingestRequest())
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("ingest request 4: expected 429, got %d", rr.Code)
	}

	// Other endpoints should not be affected
	otherRequest := httptest.NewRequest("GET", "/api/v1/traces", nil)
	ctx := context.WithValue(otherRequest.Context(), tenantIDKey, tenantID)
	claims := &Claims{
		TenantID: tenantID,
		Email:    userID + "@example.com",
		Name:     "Test User",
		Role:     "editor",
	}
	claims.Subject = userID
	ctx = context.WithValue(ctx, claimsKey, claims)
	otherRequest = otherRequest.WithContext(ctx)

	rr = httptest.NewRecorder()
	RateLimit(store, cfg)(http.HandlerFunc(okHandler)).ServeHTTP(rr, otherRequest)
	if rr.Code != http.StatusOK {
		t.Errorf("/traces endpoint should not be affected by /ingest limit, got %d", rr.Code)
	}
}

func TestRateLimit_Headers_IndicateLimitType(t *testing.T) {
	// When different limits are exceeded, headers should indicate which limit was hit
	store := newMockStore()
	cfg := DefaultRateLimitConfig()
	cfg.RequestsPerMinute = 2
	cfg.RequestsPerUserPerMinute = 10

	userID := "user-header-test"
	tenantID := "tenant-header"

	// Exhaust tenant limit
	for i := 0; i < 3; i++ {
		RateLimit(store, cfg)(http.HandlerFunc(okHandler)).ServeHTTP(httptest.NewRecorder(), requestWithTenantAndClaims(tenantID, userID))
	}

	// Next request should fail on tenant limit and indicate that in headers
	rr := httptest.NewRecorder()
	RateLimit(store, cfg)(http.HandlerFunc(okHandler)).ServeHTTP(rr, requestWithTenantAndClaims(tenantID, userID))

	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rr.Code)
	}

	limitType := rr.Header().Get("X-RateLimit-Type")
	if limitType != "tenant" && limitType != "user" && limitType != "endpoint" {
		t.Errorf("expected X-RateLimit-Type header to indicate limit type, got %q", limitType)
	}
}
