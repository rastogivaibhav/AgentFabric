package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	chi "github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// ─── Prometheus ───────────────────────────────────────────────────────────────

var (
	httpRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "agentfabric_http_requests_total",
	}, []string{"method", "path", "status"})

	httpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "agentfabric_http_duration_seconds",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path"})
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func PrometheusMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rec, r)
		path := r.URL.Path
		httpRequests.WithLabelValues(r.Method, path, http.StatusText(rec.status)).Inc()
		httpDuration.WithLabelValues(r.Method, path).Observe(time.Since(start).Seconds())
	})
}

// ─── JWT Auth ─────────────────────────────────────────────────────────────────

// Claims is the set of fields extracted from every AF JWT and stored in context.
// The embedded RegisteredClaims provides Subject (user ID), ExpiresAt, etc.
type Claims struct {
	TenantID string `json:"tenant_id"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	Role     string `json:"role"` // admin|editor|viewer
	jwt.RegisteredClaims
}

// JWTAuth validates a Bearer JWT against one or more secrets (multi-secret rotation).
// Secrets are tried in order; the first valid match accepts the request.
// The active (issuing) secret should always be secrets[0].
func JWTAuth(secrets ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			if h == "" {
				http.Error(w, `{"error":"missing authorization"}`, http.StatusUnauthorized)
				return
			}
			parts := strings.SplitN(h, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
				http.Error(w, `{"error":"invalid auth format"}`, http.StatusUnauthorized)
				return
			}

			var validClaims *Claims
			for _, secret := range secrets {
				claims := &Claims{}
				token, err := jwt.ParseWithClaims(parts[1], claims, func(t *jwt.Token) (interface{}, error) {
					return []byte(secret), nil
				})
				if err == nil && token.Valid {
					validClaims = claims
					break
				}
			}
			if validClaims == nil {
				http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), "claims", validClaims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ─── RBAC / ABAC middleware ───────────────────────────────────────────────────

// RequireRole blocks requests whose JWT does not carry one of the specified roles.
// Must run after JWTAuth (depends on "claims" being set in context).
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := r.Context().Value("claims").(*Claims)
			if !ok || claims == nil {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}
			for _, allowed := range roles {
				if strings.EqualFold(claims.Role, allowed) {
					next.ServeHTTP(w, r)
					return
				}
			}
			http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
		})
	}
}

// RequireRoleOrSelf implements ABAC: allows the request if the caller holds one
// of the specified roles, OR if the caller's JWT subject matches the {userId}
// route parameter (self-service — any authenticated user may act on their own record).
// Must run after JWTAuth inside a chi router so route params are available.
func RequireRoleOrSelf(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := r.Context().Value("claims").(*Claims)
			if !ok || claims == nil {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}

			// ABAC: subject == {userId} route param → always allow (self-service)
			userID := chi.URLParam(r, "userId")
			if userID != "" && claims.Subject == userID {
				next.ServeHTTP(w, r)
				return
			}

			// RBAC fallback: check role
			for _, allowed := range roles {
				if strings.EqualFold(claims.Role, allowed) {
					next.ServeHTTP(w, r)
					return
				}
			}
			http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
		})
	}
}

// ─── Tenant Injector ──────────────────────────────────────────────────────────

func TenantInjector(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantID := "default"
		if claims, ok := r.Context().Value("claims").(*Claims); ok && claims.TenantID != "" {
			tenantID = claims.TenantID
		}
		ctx := context.WithValue(r.Context(), "tenant_id", tenantID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ─── Collector Auth (service-to-service) ─────────────────────────────────────

func CollectorAuth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			src := r.Header.Get("X-AF-Source")
			if src != "collector" {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}
			h := r.Header.Get("Authorization")
			parts := strings.SplitN(h, " ", 2)
			if len(parts) != 2 {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			_, err := jwt.Parse(parts[1], func(t *jwt.Token) (interface{}, error) {
				return []byte(secret), nil
			})
			if err != nil {
				http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
