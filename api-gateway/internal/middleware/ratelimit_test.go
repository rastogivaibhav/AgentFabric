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
	// Key format: rl:{tenantID}:{windowMinute}
	// This ensures different minutes have independent counters
	store := newMockStore()
	cfg := RateLimitConfig{RequestsPerMinute: 1, KeyPrefix: "rl"}

	// First request
	RateLimit(store, cfg)(http.HandlerFunc(okHandler)).ServeHTTP(httptest.NewRecorder(), requestWithTenant("tenant-g"))

	// Verify the key has been stored with the expected prefix
	windowMinute := time.Now().Truncate(time.Minute).Unix()
	expectedKey := fmt.Sprintf("rl:tenant-g:%d", windowMinute)
	if _, exists := store.counts[expectedKey]; !exists {
		t.Errorf("expected key %q in store, got keys: %v", expectedKey, store.counts)
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
