package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/agentfabric/api-gateway/internal/auth"
	"github.com/agentfabric/api-gateway/internal/handlers"
	"github.com/agentfabric/api-gateway/internal/middleware"
	"github.com/agentfabric/api-gateway/internal/store"
	"github.com/agentfabric/api-gateway/internal/ws"
	"github.com/go-chi/chi/v5"
	chimid "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	pgDSN := envOr("DATABASE_URL", "postgres://fabric:fabric@localhost:5432/agentfabric?sslmode=disable")
	redisAddr := envOr("REDIS_URL", "redis://localhost:6379")
	jwtSecret := envOr("AF_JWT_SECRET", "dev-secret-change-in-production")
	listenAddr := envOr("LISTEN_ADDR", ":8080")
	authDisabled := os.Getenv("AF_AUTH_DISABLED") == "true"
	rateLimitRPM := int64(parseIntEnv("AF_RATE_LIMIT_RPM", 1000))

	// AF_JWT_SECRETS: comma-separated list for zero-downtime key rotation.
	// First entry = active signing key. All entries accepted for verification.
	// Falls back to AF_JWT_SECRET when not set.
	jwtSecrets := parseSecrets(envOr("AF_JWT_SECRETS", jwtSecret))

	if authDisabled {
		logger.Warn("AF_AUTH_DISABLED=true — JWT authentication is OFF (dev mode only)")
	}

	// Storage
	pgStore, err := store.NewPostgresStore(pgDSN, logger)
	if err != nil {
		logger.Fatal("postgres init failed", zap.Error(err))
	}
	defer pgStore.Close()

	redisClient, err := store.NewRedisClient(redisAddr)
	if err != nil {
		logger.Fatal("redis init failed", zap.Error(err))
	}

	// WebSocket hub for live streaming
	hub := ws.NewHub(logger)
	go hub.Run(context.Background())

	// OIDC handler (P0-4: enterprise SSO + S4: password login + GA: multi-secret rotation)
	oidcHandler := auth.NewOIDCHandler(auth.OIDCConfig{
		Issuer:        envOr("AF_OIDC_ISSUER", ""),
		ClientID:      envOr("AF_OIDC_CLIENT_ID", ""),
		ClientSecret:  envOr("AF_OIDC_CLIENT_SECRET", ""),
		RedirectURI:   envOr("AF_OIDC_REDIRECT_URI", "http://localhost:8080/auth/callback"),
		JWTSecret:     jwtSecrets[0],
		JWTSecrets:    jwtSecrets,
		LogoutURL:     envOr("AF_OIDC_LOGOUT_URL", ""),
		AdminUser:     envOr("AF_ADMIN_USER", "admin"),
		AdminPassword: envOr("AF_ADMIN_PASSWORD", "admin"),
	}, logger)

	// Wire handlers
	h := handlers.New(pgStore, redisClient, hub, logger, jwtSecret)

	r := chi.NewRouter()

	// ─── Global middleware ───────────────────────────────────────────────────
	r.Use(chimid.RequestID)
	r.Use(chimid.RealIP)
	r.Use(chimid.Recoverer)
	r.Use(chimid.Timeout(30 * time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:3000", "http://localhost:5173", "https://*.agentfabric.io"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID", "X-AF-Source", "X-AF-Tenant"},
		AllowCredentials: true,
		MaxAge:           300,
	}))
	r.Use(middleware.PrometheusMiddleware)

	// ─── Health & metrics ────────────────────────────────────────────────────
	r.Get("/healthz", h.Health)
	r.Handle("/metrics", promhttp.Handler())

	// ─── Auth endpoints ───────────────────────────────────────────────────────
	// Password login (default, no OIDC required): POST /auth/login {username, password}
	r.Post("/auth/login", oidcHandler.PasswordLogin)
	// OIDC SSO (P0-4, opt-in via AF_OIDC_ISSUER): GET /auth/login redirects to provider
	r.Get("/auth/login", oidcHandler.Login)
	r.Get("/auth/callback", oidcHandler.Callback)
	r.Get("/auth/logout", oidcHandler.Logout)
	r.Get("/auth/me", oidcHandler.Me)
	// Token refresh (v1.0.0 GA): valid token → new token with refreshed expiry
	r.Post("/auth/refresh", oidcHandler.Refresh)

	// ─── Internal ingest (collector → gateway) ───────────────────────────────
	// Skip collector auth in dev mode so the collector can send without a signed JWT
	if authDisabled {
		r.Post("/internal/ingest", h.Ingest)
	} else {
		r.With(middleware.CollectorAuth(jwtSecret)).Post("/internal/ingest", h.Ingest)
	}

	// Per-tenant rate limiter (Principle 2: tenant isolation)
	rlCfg := middleware.DefaultRateLimitConfig()
	rlCfg.RequestsPerMinute = rateLimitRPM
	rateLimiter := middleware.RateLimit(redisClient, rlCfg)

	// ─── Public API v1 ───────────────────────────────────────────────────────
	r.Route("/api/v1", func(r chi.Router) {
		if !authDisabled {
			// Multi-secret JWTAuth: accepts tokens signed by any key in the rotation list.
			r.Use(middleware.JWTAuth(jwtSecrets...))
		}
		r.Use(middleware.TenantInjector)
		r.Use(rateLimiter) // Rate limiting runs after TenantInjector (needs tenant_id)

		// Traces
		r.Route("/traces", func(r chi.Router) {
			r.Get("/", h.ListTraces)
			r.Get("/{traceId}", h.GetTrace)
			r.Get("/{traceId}/graph", h.GetTraceGraph)
			r.Get("/{traceId}/timeline", h.GetTraceTimeline)
			r.Get("/{traceId}/cost", h.GetTraceCost)
		})

		// Agents
		r.Route("/agents", func(r chi.Router) {
			r.Get("/", h.ListAgents)
			r.Get("/{agentId}", h.GetAgent)
			r.Get("/{agentId}/runs", h.GetAgentRuns)
			r.Get("/{agentId}/metrics", h.GetAgentMetrics)
			r.Get("/{agentId}/topology", h.GetAgentTopology)
		})

		// Runs (LangSmith-style)
		r.Route("/runs", func(r chi.Router) {
			r.Get("/", h.ListRuns)
			r.Get("/{runId}", h.GetRun)
			r.Get("/{runId}/children", h.GetRunChildren)
			r.Post("/{runId}/feedback", h.PostFeedback)
		})

		// Live stream (Wireshark-style WebSocket)
		r.Get("/stream/live", h.LiveStream)

		// Analytics
		r.Get("/analytics/overview", h.GetOverview)
		r.Get("/analytics/frameworks", h.GetFrameworkStats)
		r.Get("/analytics/cost", h.GetCostReport)
		r.Get("/analytics/errors", h.GetErrorReport)

		// Environments
		r.Get("/environments", h.ListEnvironments)

		// Audit log (Principle 4: immutable audit trail)
		r.Get("/audit", h.ListAudit)
		r.Get("/audit/verify", h.VerifyAuditChain)

		// Users CRUD — RBAC + ABAC enforced per operation:
		//   GET  (list/read):   any authenticated user
		//   POST (create):      admin only
		//   PUT  (update):      admin OR the user updating their own record (ABAC self-service)
		//   DELETE:             admin only
		r.Route("/users", func(r chi.Router) {
			r.Get("/", h.ListUsers)
			r.With(middleware.RequireRole("admin")).Post("/", h.CreateUser)
			r.Get("/{userId}", h.GetUser)
			r.With(middleware.RequireRoleOrSelf("admin")).Put("/{userId}", h.UpdateUser)
			r.With(middleware.RequireRole("admin")).Delete("/{userId}", h.DeleteUser)
		})
	})

	// ─── Start server ────────────────────────────────────────────────────────
	srv := &http.Server{
		Addr:         listenAddr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		logger.Info("API Gateway listening", zap.String("addr", listenAddr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("server error", zap.Error(err))
		}
	}()

	<-sigCh
	logger.Info("Shutting down API gateway...")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func parseIntEnv(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// parseSecrets splits a comma-separated list of JWT secrets.
// The first entry is the active signing key; remaining entries are accepted for
// verification only (zero-downtime rotation: prepend new secret, retire old one).
func parseSecrets(raw string) []string {
	var out []string
	for _, s := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(s); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return []string{"dev-secret-change-in-production"}
	}
	return out
}
