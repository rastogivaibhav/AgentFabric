// api-gateway/internal/middleware/ratelimit.go
// Per-tenant sliding-window rate limiting backed by Redis.
// Principle 2: tenant isolation — each tenant has an independent counter.
// Key: rl:{tenantID}:{windowMinute}  (minute-granularity sliding window)
// Exceeding the limit returns HTTP 429 with a Retry-After header.

package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// RateLimitStore is the minimal Redis interface required for rate limiting.
// Using an interface rather than a concrete type makes tests injector-friendly.
type RateLimitStore interface {
	// IncrWithExpiry atomically increments key, sets expiry on first creation,
	// and returns the new value. Expiry is only set when count == 1 (new window).
	IncrWithExpiry(ctx context.Context, key string, expiry time.Duration) (int64, error)
}

// RateLimitConfig holds the rate limit parameters.
type RateLimitConfig struct {
	// RequestsPerMinute is the maximum number of requests allowed per tenant per minute.
	RequestsPerMinute int64
	// KeyPrefix allows multiple rate limit tiers to share one Redis instance.
	KeyPrefix string
}

// DefaultRateLimitConfig returns the default configuration.
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		RequestsPerMinute: 1000,
		KeyPrefix:         "rl",
	}
}

// RateLimit returns a middleware that enforces per-tenant rate limiting.
// It must run AFTER TenantInjector so that tenant_id is available in ctx.
func RateLimit(store RateLimitStore, cfg RateLimitConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tenantID := TenantIDFromCtx(r.Context())

			// Window key: changes every minute — naturally expires old windows
			windowMinute := time.Now().Truncate(time.Minute).Unix()
			key := fmt.Sprintf("%s:%s:%d", cfg.KeyPrefix, tenantID, windowMinute)

			count, err := store.IncrWithExpiry(r.Context(), key, 2*time.Minute)
			if err != nil {
				// Redis failure: fail open (do not block legitimate traffic)
				next.ServeHTTP(w, r)
				return
			}

			if count > cfg.RequestsPerMinute {
				// Time until the current window resets
				retryAfter := int(time.Until(time.Now().Truncate(time.Minute).Add(time.Minute)).Seconds()) + 1
				w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
				w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", cfg.RequestsPerMinute))
				w.Header().Set("X-RateLimit-Remaining", "0")
				w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", time.Now().Truncate(time.Minute).Add(time.Minute).Unix()))
				http.Error(w, `{"error":"rate limit exceeded","code":"RATE_LIMITED"}`, http.StatusTooManyRequests)
				return
			}

			remaining := cfg.RequestsPerMinute - count
			w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", cfg.RequestsPerMinute))
			w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
			w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", time.Now().Truncate(time.Minute).Add(time.Minute).Unix()))

			next.ServeHTTP(w, r)
		})
	}
}
