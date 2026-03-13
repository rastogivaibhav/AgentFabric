package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"strconv"
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

	// OIDC handler (P0-4: enterprise SSO)
	oidcHandler := auth.NewOIDCHandler(auth.OIDCConfig{
		Issuer:       envOr("AF_OIDC_ISSUER", ""),
		ClientID:     envOr("AF_OIDC_CLIENT_ID", ""),
		ClientSecret: envOr("AF_OIDC_CLIENT_SECRET", ""),
		RedirectURI:  envOr("AF_OIDC_REDIRECT_URI", "http://localhost:8080/auth/callback"),
		JWTSecret:    jwtSecret,
		LogoutURL:    envOr("AF_OIDC_LOGOUT_URL", ""),
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

	// ─── OIDC SSO endpoints (P0-4) ───────────────────────────────────────────
	r.Get("/auth/login", oidcHandler.Login)
	r.Get("/auth/callback", oidcHandler.Callback)
	r.Get("/auth/logout", oidcHandler.Logout)
	r.Get("/auth/me", oidcHandler.Me)

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
			r.Use(middleware.JWTAuth(jwtSecret))
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
