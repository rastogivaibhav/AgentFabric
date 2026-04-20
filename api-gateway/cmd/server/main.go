package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimid "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	migrate "github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres" // postgres driver for migrate
	_ "github.com/golang-migrate/migrate/v4/source/file"       // file:// source driver
	"github.com/govagn/api-gateway/internal/auth"
	"github.com/govagn/api-gateway/internal/budget"
	"github.com/govagn/api-gateway/internal/handlers"
	"github.com/govagn/api-gateway/internal/middleware"
	"github.com/govagn/api-gateway/internal/netproxy"
	"github.com/govagn/api-gateway/internal/policy"
	"github.com/govagn/api-gateway/internal/proxy"
	"github.com/govagn/api-gateway/internal/rollouts"
	"github.com/govagn/api-gateway/internal/store"
	"github.com/govagn/api-gateway/internal/vault"
	"github.com/govagn/api-gateway/internal/ws"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	pgDSN := envOr("DATABASE_URL", "postgres://fabric:fabric@localhost:5432/govagn?sslmode=disable")
	redisAddr := envOr("REDIS_URL", "redis://localhost:6379")
	jwtSecret := envOr("GV_JWT_SECRET", "dev-secret-change-in-production")
	listenAddr := envOr("LISTEN_ADDR", ":8080")
	authDisabled := os.Getenv("GV_AUTH_DISABLED") == "true"
	collectorAuthToken := strings.TrimSpace(os.Getenv("GV_GATEWAY_AUTH_TOKEN"))
	rateLimitRPM := int64(parseIntEnv("GV_RATE_LIMIT_RPM", 1000))
	openapiSpecPath := envOr("GV_OPENAPI_SPEC_PATH", "docs/openapi.yaml")
	policyPackPath := envOr("GV_POLICY_PACKS_PATH", "deploy/seed/policy-packs")
	evalPackPath := envOr("GV_EVAL_PACKS_PATH", "deploy/seed/eval-packs")
	strictMode := strictConfigEnabled()

	// TLS configuration — fail-secure: if GV_TLS_ENABLED=true the server will
	// refuse to start as plain HTTP when cert/key paths are absent.
	tlsEnabled := os.Getenv("GV_TLS_ENABLED") == "true"
	tlsCertFile := os.Getenv("GV_TLS_CERT_FILE")
	tlsKeyFile := os.Getenv("GV_TLS_KEY_FILE")

	// GV_JWT_SECRETS: comma-separated list for zero-downtime key rotation.
	// First entry = active signing key. All entries accepted for verification.
	// Falls back to GV_JWT_SECRET when not set.
	jwtSecrets := parseSecrets(envOr("GV_JWT_SECRETS", jwtSecret))

	if err := validateCollectorAuthConfig(authDisabled, collectorAuthToken); err != nil {
		logger.Fatal("invalid collector ingest auth configuration", zap.Error(err))
	}
	if err := validateProductionConfig(authDisabled, jwtSecrets, collectorAuthToken, envOr("GV_ADMIN_PASSWORD", "admin"), os.Getenv("GV_VAULT_KEY")); err != nil {
		logger.Fatal("unsafe production configuration", zap.Error(err))
	}
	if warning := liveStreamTopologyWarning(strictMode); warning != "" {
		logger.Warn(warning)
	}

	if authDisabled {
		logger.Warn("GV_AUTH_DISABLED=true — JWT authentication is OFF (dev mode only)")
	}
	// Pricing precedence:
	// 1. Bootstrap from GV_MODEL_PRICING_FILE / GV_MODEL_PRICING_JSON when present.
	// 2. Otherwise use built-in defaults.
	// 3. After migrations and store init, DB pricing rules override bootstrap pricing
	//    when the pricing_rules table contains rows.
	if err := proxy.ConfigurePricingFromEnv(); err != nil {
		logger.Fatal("invalid pricing configuration", zap.Error(err))
	}

	// ─── Database migrations ──────────────────────────────────────────────────
	// GV_MIGRATE_ON_STARTUP defaults to true.  Set to "false" to skip (tests,
	// read-only replicas, or environments where migrations are applied out-of-band).
	if os.Getenv("GV_MIGRATE_ON_STARTUP") != "false" {
		runMigrations(pgDSN, envOr("GV_MIGRATIONS_PATH", "deploy/migrations"), logger)
	}

	// Storage
	pgStore, err := store.NewPostgresStore(pgDSN, logger)
	if err != nil {
		logger.Fatal("postgres init failed", zap.Error(err))
	}
	defer pgStore.Close()
	if err := proxy.LoadPricingRules(context.Background(), pgStore); err != nil {
		logger.Fatal("pricing rules load failed", zap.Error(err))
	}
	policyEngine := policy.NewEngine()
	if err := policyEngine.LoadRules(context.Background(), pgStore); err != nil {
		logger.Fatal("policy rules load failed", zap.Error(err))
	}

	redisClient, err := store.NewRedisClient(redisAddr)
	if err != nil {
		logger.Fatal("redis init failed", zap.Error(err))
	}

	// WebSocket hub for live streaming.
	// This hub is process-local today, so complete /api/v1/stream/live delivery
	// is only supported with a single api-gateway replica.
	hub := ws.NewHub(logger)
	go hub.Run(context.Background())

	// OIDC handler (P0-4: enterprise SSO + S4: password login + GA: multi-secret rotation)
	// pgStore is passed as the UserLookup implementation so PasswordLogin can
	// verify credentials against the users table (bcrypt) with an env-var
	// admin as a break-glass fallback.
	oidcCfg := auth.OIDCConfig{
		Issuer:               envOr("GV_OIDC_ISSUER", ""),
		ClientID:             envOr("GV_OIDC_CLIENT_ID", ""),
		ClientSecret:         envOr("GV_OIDC_CLIENT_SECRET", ""),
		RedirectURI:          envOr("GV_OIDC_REDIRECT_URI", defaultOIDCRedirectURI(strictMode)),
		JWTSecret:            jwtSecrets[0],
		JWTSecrets:           jwtSecrets,
		LogoutURL:            envOr("GV_OIDC_LOGOUT_URL", ""),
		RequireSSO:           strings.EqualFold(envOr("GV_SSO_REQUIRED", "false"), "true"),
		DisablePasswordLogin: strings.EqualFold(envOr("GV_PASSWORD_LOGIN_DISABLED", "false"), "true"),
		AdminUser:            envOr("GV_ADMIN_USER", "admin"),
		AdminPassword:        envOr("GV_ADMIN_PASSWORD", "admin"),
	}
	if err := auth.ValidateConfig(oidcCfg, strictMode); err != nil {
		logger.Fatal("invalid enterprise auth configuration", zap.Error(err))
	}
	oidcHandler := auth.NewOIDCHandler(oidcCfg, pgStore, logger)

	// Budget enforcement
	budgetEnabled := os.Getenv("GV_BUDGET_ENABLED") != "false"
	var budgetEnforcer *budget.BudgetEnforcer
	if budgetEnabled {
		alerter := budget.NewWebhookAlerter(logger)
		budgetEnforcer = budget.NewBudgetEnforcer(pgStore, alerter, logger)
		logger.Info("budget enforcement enabled")
	}

	// ─── Vault (Layer 2: virtual key proxy) ──────────────────────────────────
	// GV_VAULT_KEY must be a 64-char hex string (32 bytes).
	// In dev a predictable key is used with a loud warning; production must set this.
	vaultKeyHex := envOr("GV_VAULT_KEY", "")
	var llmVault *vault.Vault
	if vaultKeyHex == "" {
		vaultKeyHex = "0000000000000000000000000000000000000000000000000000000000000000"
		logger.Warn("GV_VAULT_KEY not set — using insecure dev key. Set GV_VAULT_KEY in production.")
	}
	llmVault, err = vault.New(pgStore.Pool(), vaultKeyHex)
	if err != nil {
		logger.Fatal("vault init failed", zap.Error(err))
	}

	keyHandler := handlers.NewKeyHandler(llmVault, pgStore, logger)
	rolloutService := rollouts.NewService(pgStore)
	llmProxy := proxy.New(llmVault, budgetEnforcer, pgStore, policyEngine, rolloutService, hub, logger)

	// ─── NetProxy (Layer 3: transparent HTTPS MITM proxy) ────────────────────
	// Listens on GV_NETPROXY_ADDR (:8443) and handles HTTP CONNECT tunnelling.
	// Intercepts HTTPS to known LLM API domains; all other hosts pass through.
	// Clients must set HTTP_PROXY=http://localhost:8443 and install the CA cert.
	netProxyCACertFile := strings.TrimSpace(os.Getenv("GV_NETPROXY_CA_CERT_FILE"))
	netProxyCAKeyFile := strings.TrimSpace(os.Getenv("GV_NETPROXY_CA_KEY_FILE"))
	netProxyCA, err := loadNetProxyCA(netProxyCACertFile, netProxyCAKeyFile, strictMode, logger)
	if err != nil {
		logger.Fatal("netproxy CA init failed", zap.Error(err))
	}
	np := netproxy.New(netProxyCA, llmVault, budgetEnforcer, pgStore, policyEngine, hub, logger)

	// Wire handlers
	h := handlers.NewWithPackRoots(pgStore, redisClient, hub, logger, jwtSecret, budgetEnforcer, policyEngine, policyPackPath, evalPackPath)

	r := chi.NewRouter()

	// ─── Global middleware ───────────────────────────────────────────────────
	// GV_CORS_ORIGINS: comma-separated list of allowed CORS origins.
	// Defaults to the standard local dev ports + the production wildcard domain.
	allowedOrigins := parseOrigins(envOr(
		"GV_CORS_ORIGINS",
		"http://localhost:3000,http://localhost:5173,https://*.govagn.io",
	))

	r.Use(chimid.RequestID)
	r.Use(chimid.RealIP)
	r.Use(chimid.Recoverer)
	r.Use(chimid.Timeout(30 * time.Second))
	r.Use(middleware.SecurityHeaders)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID", "X-AF-Source", "X-AF-Tenant"},
		AllowCredentials: true,
		MaxAge:           300,
	}))
	r.Use(middleware.PrometheusMiddleware)

	// ─── Health & metrics ────────────────────────────────────────────────────
	r.Get("/healthz", h.Health)
	r.Get("/readyz", h.Ready)
	r.Handle("/metrics", promhttp.Handler())
	r.Get("/docs", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/docs/swagger", http.StatusFound)
	})
	r.Get("/docs/openapi.yaml", serveOpenAPISpec(openapiSpecPath))
	r.Get("/docs/swagger", serveSwaggerUI("/docs/openapi.yaml"))

	// ─── NetProxy CA cert download (no auth required) ─────────────────────────
	// GET /api/v1/netproxy/ca.crt — download the MITM CA certificate in PEM format.
	// Install with: scripts/install-ca-cert.sh or manually into OS trust store.
	r.Get("/api/v1/netproxy/ca.crt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-pem-file")
		w.Header().Set("Content-Disposition", `attachment; filename="govagn-ca.crt"`)
		w.Write(netProxyCA.CertPEM()) //nolint:errcheck
	})

	// ─── Auth endpoints ───────────────────────────────────────────────────────
	// Password login (default, no OIDC required): POST /auth/login {username, password}
	r.Post("/auth/login", oidcHandler.PasswordLogin)
	// OIDC SSO (P0-4, opt-in via GV_OIDC_ISSUER): GET /auth/login redirects to provider
	r.Get("/auth/login", oidcHandler.Login)
	r.Get("/auth/callback", oidcHandler.Callback)
	r.Get("/auth/logout", oidcHandler.Logout)
	r.Get("/auth/me", oidcHandler.Me)
	// Token refresh (v1.0.0 GA): valid token → new token with refreshed expiry
	r.Post("/auth/refresh", oidcHandler.Refresh)

	// ─── Internal ingest (collector → gateway) ───────────────────────────────
	// Skip collector auth in dev mode so the collector can send without the
	// shared ingest bearer token.
	if authDisabled {
		r.Post("/internal/ingest", h.Ingest)
	} else {
		r.With(middleware.CollectorAuth(collectorAuthToken)).Post("/internal/ingest", h.Ingest)
	}

	// Per-tenant rate limiter (Principle 2: tenant isolation)
	rlCfg := middleware.DefaultRateLimitConfig()
	rlCfg.RequestsPerMinute = rateLimitRPM
	rateLimiter := middleware.RateLimit(redisClient, rlCfg)

	// ─── Public API v1 ───────────────────────────────────────────────────────
	r.Route("/api/v1", func(r chi.Router) {
		if authDisabled {
			r.Use(middleware.DevAuthInjector)
		} else {
			// Multi-secret JWTAuth: accepts tokens signed by any key in the rotation list.
			r.Use(middleware.JWTAuth(jwtSecrets...))
		}
		r.Use(middleware.TenantInjector)
		r.Use(rateLimiter) // Rate limiting runs after TenantInjector (needs tenant_id)

		// Traces
		r.Route("/traces", func(r chi.Router) {
			r.Get("/", h.ListTraces)
			r.Get("/compare", h.GetTraceComparison)
			r.Get("/saved-views", h.ListTraceSavedViews)
			r.Put("/saved-views", h.UpsertTraceSavedView)
			r.Delete("/saved-views/{viewId}", h.DeleteTraceSavedView)
			r.Get("/{traceId}", h.GetTrace)
			r.Get("/{traceId}/decisions", h.ListTraceDecisions)
			r.Get("/{traceId}/graph", h.GetTraceGraph)
			r.Get("/{traceId}/timeline", h.GetTraceTimeline)
			r.Get("/{traceId}/cost", h.GetTraceCost)
		})

		// Agents
		r.Route("/agents", func(r chi.Router) {
			r.Get("/scorecards", h.ListAgentScorecards)
			r.Get("/", h.ListAgents)
			r.Get("/{agentId}", h.GetAgent)
			r.Get("/{agentId}/runs", h.GetAgentRuns)
			r.Get("/{agentId}/metrics", h.GetAgentMetrics)
			r.Get("/{agentId}/scorecard", h.GetAgentScorecard)
			r.Get("/{agentId}/topology", h.GetAgentTopology)
		})

		// Runs (LangSmith-style)
		r.Route("/runs", func(r chi.Router) {
			r.Get("/", h.ListRuns)
			r.Get("/{runId}", h.GetRun)
			r.Get("/{runId}/children", h.GetRunChildren)
			r.Post("/{runId}/feedback", h.PostFeedback)
		})

		r.Route("/managed-agents", func(r chi.Router) {
			r.Get("/", h.ListManagedAgents)
			r.With(middleware.RequireRole("admin")).Put("/", h.UpsertManagedAgent)
			r.Get("/{agentId}", h.GetManagedAgent)
		})
		r.Route("/managed-environments", func(r chi.Router) {
			r.Get("/", h.ListManagedEnvironments)
			r.With(middleware.RequireRole("admin")).Put("/", h.UpsertManagedEnvironment)
			r.Get("/{environmentId}", h.GetManagedEnvironment)
		})
		r.Route("/managed-sessions", func(r chi.Router) {
			r.Get("/", h.ListManagedSessions)
			r.With(middleware.RequireRole("admin")).Post("/", h.CreateManagedSession)
			r.Get("/{sessionId}", h.GetManagedSession)
			r.Get("/{sessionId}/events", h.ListManagedSessionEvents)
			r.With(middleware.RequireRole("admin")).Post("/{sessionId}/events", h.CreateManagedSessionEvent)
			r.Get("/{sessionId}/tasks", h.ListManagedSessionTasks)
		})
		r.Route("/managed-tasks", func(r chi.Router) {
			r.With(middleware.RequireRole("admin")).Put("/", h.UpsertManagedTask)
			r.Get("/{taskId}", h.GetManagedTask)
			r.Get("/{taskId}/artifacts", h.ListManagedTaskArtifacts)
			r.With(middleware.RequireRole("admin")).Post("/{taskId}/artifacts", h.CreateManagedTaskArtifact)
			r.With(middleware.RequireRole("admin")).Post("/{taskId}/approve", h.ApproveManagedTask)
			r.With(middleware.RequireRole("admin")).Post("/{taskId}/deny", h.DenyManagedTask)
		})

		// Live stream (Wireshark-style WebSocket, single-process fan-out only)
		r.Get("/stream/live", h.LiveStream)

		// Analytics
		r.Get("/analytics/overview", h.GetOverview)
		r.Get("/analytics/frameworks", h.GetFrameworkStats)
		r.Get("/analytics/cost", h.GetCostReport)
		r.Get("/analytics/cost/spikes", h.GetCostSpikes)
		r.Get("/analytics/errors", h.GetErrorReport)

		// Environments
		r.Get("/environments", h.ListEnvironments)

		// Audit log (Principle 4: immutable audit trail)
		r.Get("/decisions", h.ListDecisions)
		r.Get("/recommendations", h.ListRecommendations)
		r.Get("/audit", h.ListAudit)
		r.Get("/audit/verify", h.VerifyAuditChain)
		r.With(middleware.RequireRole("admin")).Get("/audit/control", h.ListControlAudit)
		r.With(middleware.RequireRole("admin")).Get("/audit/control-history", h.ListControlHistory)
		r.With(middleware.RequireRole("admin")).Get("/audit/evidence-bundles", h.ListEvidenceBundles)
		r.With(middleware.RequireRole("admin")).Post("/audit/evidence-bundles", h.CreateEvidenceBundle)
		r.With(middleware.RequireRole("admin")).Get("/audit/evidence-bundles/{bundleId}", h.GetEvidenceBundle)
		r.With(middleware.RequireRole("admin")).Get("/audit/evidence-bundles/{bundleId}/export", h.ExportEvidenceBundle)
		r.With(middleware.RequireRole("admin")).Post("/recommendations/{recommendationId}/status", h.UpdateRecommendationStatus)

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

		// Budgets — admin sets limits, any user can read usage
		r.Route("/budgets/{tenant_id}", func(r chi.Router) {
			r.Get("/", h.GetBudget)
			r.Get("/usage", h.GetBudgetUsage)
			r.Get("/alerts", h.GetBudgetAlerts)
			r.With(middleware.RequireRole("admin")).Put("/", h.UpsertBudget)
			r.With(middleware.RequireRole("admin")).Delete("/", h.DeleteBudget)
		})

		r.Route("/pricing", func(r chi.Router) {
			r.With(middleware.RequireRole("admin")).Get("/", h.ListPricingRules)
			r.With(middleware.RequireRole("admin")).Put("/", h.UpsertPricingRule)
			r.With(middleware.RequireRole("admin")).Post("/preview", h.PreviewPricingRule)
			r.With(middleware.RequireRole("admin")).Get("/audit", h.ListPricingAudit)
			r.With(middleware.RequireRole("admin")).Get("/export", h.ExportPricingRules)
			r.With(middleware.RequireRole("admin")).Delete("/{ruleId}", h.DeletePricingRule)
		})

		r.Route("/evals", func(r chi.Router) {
			r.With(middleware.RequireRole("admin")).Get("/", h.ListEvalRuns)
			r.With(middleware.RequireRole("admin")).Get("/packs", h.ListEvalPacks)
			r.With(middleware.RequireRole("admin")).Get("/packs/{packId}", h.GetEvalPack)
			r.With(middleware.RequireRole("admin")).Get("/datasets", h.ListEvalDatasets)
			r.With(middleware.RequireRole("admin")).Put("/datasets", h.UpsertEvalDataset)
			r.With(middleware.RequireRole("admin")).Get("/executions", h.ListEvalExecutions)
			r.With(middleware.RequireRole("admin")).Get("/executions/{executionId}", h.GetEvalExecution)
			r.With(middleware.RequireRole("admin")).Post("/execute", h.ExecuteEvalPack)
			r.With(middleware.RequireRole("admin")).Post("/score", h.ScoreTraceEval)
			r.With(middleware.RequireRole("admin")).Post("/regressions", h.CompareEvalRegressions)
		})

		r.Route("/prompts", func(r chi.Router) {
			r.With(middleware.RequireRole("admin")).Get("/", h.ListPrompts)
			r.With(middleware.RequireRole("admin")).Put("/", h.UpsertPromptVersion)
			r.With(middleware.RequireRole("admin")).Post("/promote", h.PromotePromptRelease)
		})

		r.Route("/rollouts", func(r chi.Router) {
			r.With(middleware.RequireRole("admin")).Get("/", h.ListRollouts)
			r.With(middleware.RequireRole("admin")).Put("/", h.UpsertRolloutRule)
			r.With(middleware.RequireRole("admin")).Post("/preview", h.PreviewRollout)
			r.With(middleware.RequireRole("admin")).Post("/{rolloutId}/status", h.UpdateRolloutStatus)
		})

		r.Route("/policies", func(r chi.Router) {
			r.With(middleware.RequireRole("admin")).Get("/", h.ListPolicyRules)
			r.With(middleware.RequireRole("admin")).Get("/packs", h.ListPolicyPacks)
			r.With(middleware.RequireRole("admin")).Get("/packs/{packId}", h.GetPolicyPack)
			r.With(middleware.RequireRole("admin")).Post("/packs/apply", h.ApplyPolicyPack)
			r.With(middleware.RequireRole("admin")).Put("/", h.UpsertPolicyRule)
			r.With(middleware.RequireRole("admin")).Post("/preview", h.PreviewPolicyRule)
			r.With(middleware.RequireRole("admin")).Post("/simulate", h.SimulatePolicyRule)
			r.With(middleware.RequireRole("admin")).Delete("/{ruleId}", h.DeletePolicyRule)
		})

		// Virtual keys (Layer 2) — register real LLM keys, receive af-vk-* virtual keys
		r.Route("/keys", func(r chi.Router) {
			r.Get("/", keyHandler.ListKeys)
			r.With(middleware.RequireRole("admin")).Post("/", keyHandler.RegisterKey)
			r.With(middleware.RequireRole("admin")).Delete("/{virtualKey}", keyHandler.RevokeKey)
		})
	})

	// ─── LLM Proxy (Layer 2) — virtual key → real key, budget check, span record ──
	// Routes: /proxy/{provider}/v1/*  (no JWT auth — virtual key IS the auth)
	r.Route("/proxy/{provider}", func(r chi.Router) {
		r.HandleFunc("/*", func(w http.ResponseWriter, r *http.Request) {
			provider := chi.URLParam(r, "provider")
			r = proxy.WithProvider(r, provider)
			tail := strings.TrimPrefix(chi.URLParam(r, "*"), "/")
			r.URL.Path = "/" + tail
			llmProxy.ServeHTTP(w, r)
		})
	})

	// ─── Start server ────────────────────────────────────────────────────────
	srv := &http.Server{
		Addr:              listenAddr,
		Handler:           r,
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MiB is enough for JWT/cookie headers without inviting abuse.
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := serve(srv, tlsEnabled, tlsCertFile, tlsKeyFile, logger); err != nil && err != http.ErrServerClosed {
			logger.Fatal("server error", zap.Error(err))
		}
	}()

	// ─── Layer 3: Net proxy server on :8443 ──────────────────────────────────
	netProxyAddr := envOr("GV_NETPROXY_ADDR", ":8443")
	netProxySrv := &http.Server{
		Addr:    netProxyAddr,
		Handler: np,
		// No write timeout — CONNECT tunnels are long-lived bidirectional streams.
		ReadTimeout:       60 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	go func() {
		logger.Info("net proxy listener starting", zap.String("addr", netProxyAddr))
		if err := netProxySrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("net proxy listener error", zap.Error(err))
		}
	}()

	<-sigCh
	logger.Info("Shutting down API gateway...")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	netProxySrv.Shutdown(ctx) //nolint:errcheck
	srv.Shutdown(ctx)
}

// serve starts the HTTP or HTTPS server depending on tlsEnabled.
//
// Fail-secure contract: if tlsEnabled is true, both certFile and keyFile MUST
// be non-empty.  An empty path causes an immediate error return — the server
// will never silently fall back to plain HTTP because that would defeat the
// purpose of enabling TLS in the first place.
//
// The function is extracted from main() so it can be unit-tested without
// spinning up an entire router or requiring real TLS certificates on disk.
func serve(srv *http.Server, tlsEnabled bool, certFile, keyFile string, logger *zap.Logger) error {
	if tlsEnabled {
		if certFile == "" || keyFile == "" {
			return fmt.Errorf(
				"GV_TLS_ENABLED=true but GV_TLS_CERT_FILE or GV_TLS_KEY_FILE is not set — " +
					"refusing to fall back to plain HTTP (fail-secure); set both env vars or disable TLS",
			)
		}
		logger.Info("TLS enabled",
			zap.String("addr", srv.Addr),
			zap.String("cert", certFile),
		)
		return srv.ListenAndServeTLS(certFile, keyFile)
	}
	logger.Info("TLS disabled — serving plain HTTP", zap.String("addr", srv.Addr))
	return srv.ListenAndServe()
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

// runMigrations applies all pending SQL migrations from migrationsPath against
// the database at dsn.  It is called once at process startup, before the HTTP
// server is bound, so the schema is always consistent with the binary version.
//
// On success it logs "migrations: N applied" (version number) or
// "migrations: already at latest" when nothing needed to change.
// Any error is fatal — a binary that cannot guarantee its schema is sound should
// not serve traffic.
func runMigrations(dsn, migrationsPath string, logger *zap.Logger) {
	m, err := migrate.New("file://"+migrationsPath, dsn)
	if err != nil {
		logger.Fatal("migrations: init failed", zap.Error(err))
	}
	defer m.Close()

	if err := m.Up(); err != nil {
		if err == migrate.ErrNoChange {
			logger.Info("migrations: already at latest")
			return
		}
		logger.Fatal("migrations: up failed", zap.Error(err))
	}

	version, _, _ := m.Version()
	logger.Info("migrations: applied", zap.Uint("version", version))
}

// parseOrigins splits a comma-separated list of CORS origins, trimming whitespace.
func parseOrigins(raw string) []string {
	var out []string
	for _, s := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(s); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
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

func defaultOIDCRedirectURI(strict bool) string {
	if strict {
		return ""
	}
	return "http://localhost:8080/auth/callback"
}

func validateCollectorAuthConfig(authDisabled bool, collectorAuthToken string) error {
	if authDisabled {
		return nil
	}
	if strings.TrimSpace(collectorAuthToken) == "" {
		return fmt.Errorf("GV_GATEWAY_AUTH_TOKEN must be set when GV_AUTH_DISABLED=false because /internal/ingest expects Authorization: Bearer <GV_GATEWAY_AUTH_TOKEN>")
	}
	return nil
}

func validateProductionConfig(authDisabled bool, jwtSecrets []string, collectorAuthToken, adminPassword, vaultKey string) error {
	if !strictConfigEnabled() {
		return nil
	}
	if authDisabled {
		return fmt.Errorf("GV_AUTH_DISABLED=true is not allowed when GV_ENV=production or GV_STRICT_CONFIG=true")
	}
	if strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		return fmt.Errorf("DATABASE_URL must be set explicitly in production; refusing the built-in local default")
	}
	if strings.TrimSpace(os.Getenv("REDIS_URL")) == "" {
		return fmt.Errorf("REDIS_URL must be set explicitly in production; refusing the built-in local default")
	}
	if strings.TrimSpace(os.Getenv("GV_CORS_ORIGINS")) == "" {
		return fmt.Errorf("GV_CORS_ORIGINS must be set explicitly in production")
	}
	if strings.TrimSpace(os.Getenv("GV_NETPROXY_CA_CERT_FILE")) == "" || strings.TrimSpace(os.Getenv("GV_NETPROXY_CA_KEY_FILE")) == "" {
		return fmt.Errorf("GV_NETPROXY_CA_CERT_FILE and GV_NETPROXY_CA_KEY_FILE must be set in production so the NetProxy CA survives restarts")
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("GV_TLS_ENABLED")), "true") {
		if strings.TrimSpace(os.Getenv("GV_TLS_CERT_FILE")) == "" || strings.TrimSpace(os.Getenv("GV_TLS_KEY_FILE")) == "" {
			return fmt.Errorf("GV_TLS_ENABLED=true requires GV_TLS_CERT_FILE and GV_TLS_KEY_FILE in production")
		}
	}
	if len(jwtSecrets) == 0 || strings.TrimSpace(jwtSecrets[0]) == "" || containsDevelopmentJWTSecret(jwtSecrets) {
		return fmt.Errorf("GV_JWT_SECRET/GV_JWT_SECRETS must be set to a non-default value")
	}
	if strings.TrimSpace(collectorAuthToken) == "" {
		return fmt.Errorf("GV_GATEWAY_AUTH_TOKEN must be set to a non-empty shared bearer token")
	}
	if adminPassword == "admin" || strings.TrimSpace(adminPassword) == "" {
		return fmt.Errorf("GV_ADMIN_PASSWORD must be set to a non-default value")
	}
	if strings.TrimSpace(vaultKey) == "" || vaultKey == strings.Repeat("0", 64) {
		return fmt.Errorf("GV_VAULT_KEY must be set to a non-default value")
	}
	if err := vault.ValidateMasterKeyHex(vaultKey); err != nil {
		return err
	}
	return nil
}

func containsDevelopmentJWTSecret(secrets []string) bool {
	for _, secret := range secrets {
		switch strings.TrimSpace(secret) {
		case "dev-secret-change-in-production", "dev-secret-change-in-prod":
			return true
		}
	}
	return false
}

func liveStreamTopologyWarning(strict bool) string {
	if !strict {
		return ""
	}
	return "/api/v1/stream/live uses an in-memory per-process hub; complete live event delivery is only supported with a single api-gateway replica, and multi-replica gateway deployments are not a supported HA topology for this endpoint"
}

func strictConfigEnabled() bool {
	if os.Getenv("GV_STRICT_CONFIG") == "true" {
		return true
	}
	return strings.EqualFold(os.Getenv("GV_ENV"), "production")
}

func loadNetProxyCA(certFile, keyFile string, strict bool, logger *zap.Logger) (*netproxy.CA, error) {
	switch {
	case strict:
		logger.Info("loading persisted netproxy CA", zap.String("cert_file", certFile), zap.String("key_file", keyFile))
		return netproxy.LoadCAFromFiles(certFile, keyFile)
	case certFile != "" || keyFile != "":
		logger.Info("loading or creating restart-stable dev netproxy CA", zap.String("cert_file", certFile), zap.String("key_file", keyFile))
		return netproxy.LoadOrCreateCA(certFile, keyFile)
	default:
		logger.Warn("GV_NETPROXY_CA_CERT_FILE/GV_NETPROXY_CA_KEY_FILE not set — generating ephemeral netproxy CA (dev mode only)")
		return netproxy.NewCA()
	}
}

func serveOpenAPISpec(specPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		http.ServeFile(w, r, specPath)
	}
}

func serveSwaggerUI(specURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <title>Govagn API Docs</title>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
    <style>
      body { margin: 0; background: #fafafa; }
      .topbar { display: none; }
    </style>
  </head>
  <body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
    <script>
      window.ui = SwaggerUIBundle({
        url: %q,
        dom_id: '#swagger-ui',
        deepLinking: true,
        docExpansion: 'list',
        persistAuthorization: false
      });
    </script>
  </body>
</html>`, specURL)
	}
}
