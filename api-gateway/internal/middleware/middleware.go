package middleware

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	chi "github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// ─── Context keys ─────────────────────────────────────────────────────────────

// contextKey is an unexported named type for context keys used by this package.
// Using a named type prevents key collisions with other packages that use plain
// string or int keys — a common source of silent bugs (Go best practice).
type contextKey int

const (
	claimsKey   contextKey = iota // value: *Claims
	tenantIDKey                   // value: string
)

// DefaultTenantID is the canonical fallback tenant UUID — the seeded "default"
// row in deploy/migrations/001_initial_schema.up.sql.
// Used when a request carries no JWT tenant claim (unauthenticated dev mode).
const DefaultTenantID = "00000000-0000-0000-0000-000000000001"

// ClaimsFromCtx returns the JWT claims injected by JWTAuth.
// Returns nil for unauthenticated requests.
func ClaimsFromCtx(ctx context.Context) *Claims {
	c, _ := ctx.Value(claimsKey).(*Claims)
	return c
}

// TenantIDFromCtx returns the tenant ID set by TenantInjector.
// Falls back to DefaultTenantID when not present (never returns empty string).
func TenantIDFromCtx(ctx context.Context) string {
	if t, ok := ctx.Value(tenantIDKey).(string); ok && t != "" {
		return t
	}
	return DefaultTenantID
}

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

// Hijack forwards the connection hijack to the underlying ResponseWriter so
// that WebSocket upgrades work correctly when wrapped by PrometheusMiddleware.
func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("underlying ResponseWriter does not implement http.Hijacker")
	}
	return h.Hijack()
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

func SecurityHeadersBasic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self' ws: wss:; frame-ancestors 'none'")
		if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
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

// JWTAuth validates a JWT against one or more secrets (multi-secret rotation).
// Token source priority: Authorization: Bearer header (API/CLI clients) →
// af_token HttpOnly cookie (browser clients). The cookie path allows the portal
// to authenticate without storing the JWT in JavaScript-accessible storage.
// Secrets are tried in order; the first valid match accepts the request.
// The active (issuing) secret should always be secrets[0].
func JWTAuth(secrets ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 1. Check Authorization: Bearer header first (API / CLI clients).
			var tokenStr string
			if h := r.Header.Get("Authorization"); h != "" {
				parts := strings.SplitN(h, " ", 2)
				if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
					http.Error(w, `{"error":"invalid auth format"}`, http.StatusUnauthorized)
					return
				}
				tokenStr = parts[1]
			}

			// 2. Fall back to the HttpOnly af_token cookie (browser clients).
			// JS cannot read HttpOnly cookies so the token is never accessible
			// to scripts, eliminating the localStorage XSS attack surface.
			if tokenStr == "" {
				if c, err := r.Cookie("af_token"); err == nil && c.Value != "" {
					tokenStr = c.Value
				}
			}

			if tokenStr == "" {
				http.Error(w, `{"error":"missing authorization"}`, http.StatusUnauthorized)
				return
			}

			var validClaims *Claims
			for _, secret := range secrets {
				claims := &Claims{}
				token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
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
			if strings.TrimSpace(validClaims.TenantID) == "" {
				http.Error(w, `{"error":"missing tenant_id in token"}`, http.StatusUnauthorized)
				return
			}
			if !isValidUUID(validClaims.TenantID) {
				http.Error(w, `{"error":"invalid tenant_id in token"}`, http.StatusForbidden)
				return
			}

			ctx := context.WithValue(r.Context(), claimsKey, validClaims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ─── RBAC / ABAC middleware ───────────────────────────────────────────────────

// RequireRole blocks requests whose JWT does not carry one of the specified roles.
// Must run after JWTAuth (depends on claims being set in context).
//
// Role comparison is case-insensitive on both sides (both normalized to lowercase)
// to match the TypeScript hasRole() behaviour in the portal. DB roles are always
// stored lowercase (admin|editor|viewer) so this is a defensive guard only.
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := ClaimsFromCtx(r.Context())
			if claims == nil {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}
			userRole := strings.ToLower(claims.Role)
			for _, allowed := range roles {
				if userRole == strings.ToLower(allowed) {
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
			claims := ClaimsFromCtx(r.Context())
			if claims == nil {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}

			// ABAC: subject == {userId} route param → always allow (self-service)
			userID := chi.URLParam(r, "userId")
			if userID != "" && claims.Subject == userID {
				next.ServeHTTP(w, r)
				return
			}

			// RBAC fallback: check role (case-insensitive, matching TypeScript hasRole)
			userRole := strings.ToLower(claims.Role)
			for _, allowed := range roles {
				if userRole == strings.ToLower(allowed) {
					next.ServeHTTP(w, r)
					return
				}
			}
			http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
		})
	}
}

// ─── Tenant Injector ──────────────────────────────────────────────────────────

// uuidRE validates RFC 4122 UUID format.
// Used to guard against non-UUID strings reaching PostgreSQL UUID columns,
// which would cause a cast error rather than proper tenant isolation.
var uuidRE = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// isValidUUID returns true if s matches RFC 4122 UUID format (case-insensitive).
func isValidUUID(s string) bool { return uuidRE.MatchString(s) }

// TenantInjector reads the tenant_id from JWT claims (set by JWTAuth) and
// injects it into the request context for downstream handlers.
// Falls back to DefaultTenantID when no claims are present (unauthenticated / dev mode).
//
// Blocker 3 (UUID/TEXT mismatch): validates the tenant_id claim is a valid UUID
// before injecting it. The DB schema defines tenant_id as UUID; a non-UUID value
// would cause PostgreSQL to throw a cast error rather than silently failing.
// Rejecting invalid values here prevents 500s and potential tenant isolation bugs.
func TenantInjector(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantID := DefaultTenantID
		if claims := ClaimsFromCtx(r.Context()); claims != nil {
			if strings.TrimSpace(claims.TenantID) == "" {
				http.Error(w, `{"error":"missing tenant_id in token"}`, http.StatusUnauthorized)
				return
			}
			if !isValidUUID(claims.TenantID) {
				http.Error(w, `{"error":"invalid tenant_id in token"}`, http.StatusForbidden)
				return
			}
			tenantID = claims.TenantID
		}
		ctx := context.WithValue(r.Context(), tenantIDKey, tenantID)
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
