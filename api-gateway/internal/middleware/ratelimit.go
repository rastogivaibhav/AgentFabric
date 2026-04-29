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
	// RequestsPerUserPerMinute is the maximum number of requests allowed per user per minute.
	RequestsPerUserPerMinute int64
	// RequestsPerEndpointPerUserPerMinute is the maximum number of requests to a specific endpoint per user per minute.
	RequestsPerEndpointPerUserPerMinute int64
	// KeyPrefix allows multiple rate limit tiers to share one Redis instance.
	KeyPrefix string
}

// DefaultRateLimitConfig returns the default configuration.
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		RequestsPerMinute:                  1000,
		RequestsPerUserPerMinute:           10000,
		RequestsPerEndpointPerUserPerMinute: 100,
		KeyPrefix:                          "rl",
	}
}

// RateLimit returns a middleware that enforces per-tenant, per-user, and per-endpoint rate limiting.
// It must run AFTER JWTAuth and TenantInjector so that claims and tenant_id are available in ctx.
func RateLimit(store RateLimitStore, cfg RateLimitConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tenantID := TenantIDFromCtx(r.Context())
			claims := ClaimsFromCtx(r.Context())

			// If JWT auth is disabled or claims unavailable, use tenant-only limiting
			userID := "anonymous"
			if claims != nil && claims.Subject != "" {
				userID = claims.Subject
			}

			// Window key: changes every minute — naturally expires old windows
			windowMinute := time.Now().Truncate(time.Minute).Unix()

			// 1. Check per-tenant limit
			tenantKey := fmt.Sprintf("%s:tenant:%s:%d", cfg.KeyPrefix, tenantID, windowMinute)
			count, err := store.IncrWithExpiry(r.Context(), tenantKey, 2*time.Minute)
			if err != nil {
				// Redis failure: fail open (do not block legitimate traffic)
				next.ServeHTTP(w, r)
				return
			}

			if count > cfg.RequestsPerMinute {
				retryAfter := int(time.Until(time.Now().Truncate(time.Minute).Add(time.Minute)).Seconds()) + 1
				w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
				w.Header().Set("X-RateLimit-Type", "tenant")
				w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", cfg.RequestsPerMinute))
				w.Header().Set("X-RateLimit-Remaining", "0")
				w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", time.Now().Truncate(time.Minute).Add(time.Minute).Unix()))
				http.Error(w, `{"error":"tenant rate limit exceeded","code":"TENANT_RATE_LIMITED"}`, http.StatusTooManyRequests)
				return
			}

			// 2. Check per-user limit
			userKey := fmt.Sprintf("%s:user:%s:%s:%d", cfg.KeyPrefix, tenantID, userID, windowMinute)
			count, err = store.IncrWithExpiry(r.Context(), userKey, 2*time.Minute)
			if err != nil {
				// Redis failure: fail open
				next.ServeHTTP(w, r)
				return
			}

			if count > cfg.RequestsPerUserPerMinute {
				retryAfter := int(time.Until(time.Now().Truncate(time.Minute).Add(time.Minute)).Seconds()) + 1
				w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
				w.Header().Set("X-RateLimit-Type", "user")
				w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", cfg.RequestsPerUserPerMinute))
				w.Header().Set("X-RateLimit-Remaining", "0")
				w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", time.Now().Truncate(time.Minute).Add(time.Minute).Unix()))
				http.Error(w, `{"error":"user rate limit exceeded","code":"USER_RATE_LIMITED"}`, http.StatusTooManyRequests)
				return
			}

			// 3. Check per-endpoint limit (optional: only for specific endpoints like /ingest)
			if r.URL.Path == "/v1/ingest" {
				endpointKey := fmt.Sprintf("%s:endpoint:%s:%s:%s:%d", cfg.KeyPrefix, tenantID, userID, "ingest", windowMinute)
				count, err = store.IncrWithExpiry(r.Context(), endpointKey, 2*time.Minute)
				if err != nil {
					// Redis failure: fail open
					next.ServeHTTP(w, r)
					return
				}

				if count > cfg.RequestsPerEndpointPerUserPerMinute {
					retryAfter := int(time.Until(time.Now().Truncate(time.Minute).Add(time.Minute)).Seconds()) + 1
					w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
					w.Header().Set("X-RateLimit-Type", "endpoint")
					w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", cfg.RequestsPerEndpointPerUserPerMinute))
					w.Header().Set("X-RateLimit-Remaining", "0")
					w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", time.Now().Truncate(time.Minute).Add(time.Minute).Unix()))
					http.Error(w, `{"error":"endpoint rate limit exceeded","code":"ENDPOINT_RATE_LIMITED"}`, http.StatusTooManyRequests)
					return
				}
			}

			// All limits passed, set response headers
			w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", cfg.RequestsPerMinute))
			w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", cfg.RequestsPerMinute-count))
			w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", time.Now().Truncate(time.Minute).Add(time.Minute).Unix()))

			next.ServeHTTP(w, r)
		})
	}
}
