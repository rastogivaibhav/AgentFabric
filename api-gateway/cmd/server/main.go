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

	"github.com/agentfabric/api-gateway/internal/auth"
	"github.com/agentfabric/api-gateway/internal/budget"
	"github.com/agentfabric/api-gateway/internal/handlers"
	"github.com/agentfabric/api-gateway/internal/middleware"
	"github.com/agentfabric/api-gateway/internal/netproxy"
	"github.com/agentfabric/api-gateway/internal/proxy"
	"github.com/agentfabric/api-gateway/internal/store"
	"github.com/agentfabric/api-gateway/internal/vault"
	"github.com/agentfabric/api-gateway/internal/ws"
	"github.com/go-chi/chi/v5"
	chimid "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	migrate "github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres" // postgres driver for migrate
	_ "github.com/golang-migrate/migrate/v4/source/file"       // file:// source driver
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

	// TLS configuration — fail-secure: if AF_TLS_ENABLED=true the server will
	// refuse to start as plain HTTP when cert/key paths are absent.
	tlsEnabled := os.Getenv("AF_TLS_ENABLED") == "true"
	tlsCertFile := os.Getenv("AF_TLS_CERT_FILE")
	tlsKeyFile := os.Getenv("AF_TLS_KEY_FILE")

	// AF_JWT_SECRETS: comma-separated list for zero-downtime key rotation.
	// First entry = active signing key. All entries accepted for verification.
	// Falls back to AF_JWT_SECRET when not set.
	jwtSecrets := parseSecrets(envOr("AF_JWT_SECRETS", jwtSecret))

	if err := validateProductionConfig(authDisabled, jwtSecrets, envOr("AF_ADMIN_PASSWORD", "admin"), os.Getenv("AF_VAULT_KEY")); err != nil {
		logger.Fatal("unsafe production configuration", zap.Error(err))
	}

	if authDisabled {
		logger.Warn("AF_AUTH_DISABLED=true — JWT authentication is OFF (dev mode only)")
	}
	// Pricing precedence:
	// 1. Bootstrap from AF_MODEL_PRICING_FILE / AF_MODEL_PRICING_JSON when present.
	// 2. Otherwise use built-in defaults.
	// 3. After migrations and store init, DB pricing rules override bootstrap pricing
	//    when the pricing_rules table contains rows.
	if err := proxy.ConfigurePricingFromEnv(); err != nil {
		logger.Fatal("invalid pricing configuration", zap.Error(err))
	}

	// ─── Database migrations ──────────────────────────────────────────────────
	// AF_MIGRATE_ON_STARTUP defaults to true.  Set to "false" to skip (tests,
	// read-only replicas, or environments where migrations are applied out-of-band).
	if os.Getenv("AF_MIGRATE_ON_STARTUP") != "false" {
		runMigrations(pgDSN, envOr("AF_MIGRATIONS_PATH", "deploy/migrations"), logger)
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

	redisClient, err := store.NewRedisClient(redisAddr)
	if err != nil {
		logger.Fatal("redis init failed", zap.Error(err))
	}

	// WebSocket hub for live streaming
	hub := ws.NewHub(logger)
	go hub.Run(context.Background())

	// OIDC handler (P0-4: enterprise SSO + S4: password login + GA: multi-secret rotation)
	// pgStore is passed as the UserLookup implementation so PasswordLogin can
	// verify credentials against the users table (bcrypt) with an env-var
	// admin as a break-glass fallback.
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
	}, pgStore, logger)

	// Budget enforcement
	budgetEnabled := os.Getenv("AF_BUDGET_ENABLED") != "false"
	var budgetEnforcer *budget.BudgetEnforcer
	if budgetEnabled {
		alerter := budget.NewWebhookAlerter(logger)
		budgetEnforcer = budget.NewBudgetEnforcer(pgStore, alerter, logger)
		logger.Info("budget enforcement enabled")
	}

	// ─── Vault (Layer 2: virtual key proxy) ──────────────────────────────────
	// AF_VAULT_KEY must be a 64-char hex string (32 bytes).
	// In dev a predictable key is used with a loud warning; production must set this.
	vaultKeyHex := envOr("AF_VAULT_KEY", "")
	var llmVault *vault.Vault
	if vaultKeyHex == "" {
		vaultKeyHex = "0000000000000000000000000000000000000000000000000000000000000000"
		logger.Warn("AF_VAULT_KEY not set — using insecure dev key. Set AF_VAULT_KEY in production.")
	}
	llmVault, err = vault.New(pgStore.Pool(), vaultKeyHex)
	if err != nil {
		logger.Fatal("vault init failed", zap.Error(err))
	}

	keyHandler := handlers.NewKeyHandler(llmVault, logger)
	llmProxy := proxy.New(llmVault, budgetEnforcer, pgStore, logger)

	// ─── NetProxy (Layer 3: transparent HTTPS MITM proxy) ────────────────────
	// Listens on AF_NETPROXY_ADDR (:8443) and handles HTTP CONNECT tunnelling.
	// Intercepts HTTPS to known LLM API domains; all other hosts pass through.
	// Clients must set HTTP_PROXY=http://localhost:8443 and install the CA cert.
	netProxyCA, err := netproxy.NewCA()
	if err != nil {
		logger.Fatal("netproxy CA init failed", zap.Error(err))
	}
	np := netproxy.New(netProxyCA, llmVault, budgetEnforcer, pgStore, logger)

	// Wire handlers
	h := handlers.New(pgStore, redisClient, hub, logger, jwtSecret, budgetEnforcer)

	r := chi.NewRouter()

	// ─── Global middleware ───────────────────────────────────────────────────
	// AF_CORS_ORIGINS: comma-separated list of allowed CORS origins.
	// Defaults to the standard local dev ports + the production wildcard domain.
	allowedOrigins := parseOrigins(envOr(
		"AF_CORS_ORIGINS",
		"http://localhost:3000,http://localhost:5173,https://*.agentfabric.io",
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
	r.Handle("/metrics", promhttp.Handler())

	// ─── NetProxy CA cert download (no auth required) ─────────────────────────
	// GET /api/v1/netproxy/ca.crt — download the MITM CA certificate in PEM format.
	// Install with: scripts/install-ca-cert.sh or manually into OS trust store.
	r.Get("/api/v1/netproxy/ca.crt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-pem-file")
		w.Header().Set("Content-Disposition", `attachment; filename="agentfabric-ca.crt"`)
		w.Write(netProxyCA.CertPEM()) //nolint:errcheck
	})

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
			r.With(middleware.RequireRole("admin")).Delete("/{ruleId}", h.DeletePricingRule)
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
		r.HandleFunc("/v1/*", func(w http.ResponseWriter, r *http.Request) {
			provider := chi.URLParam(r, "provider")
			r = proxy.WithProvider(r, provider)
			// Strip /proxy/{provider} prefix so the upstream sees /v1/...
			r.URL.Path = "/v1/" + chi.URLParam(r, "*")
			llmProxy.ServeHTTP(w, r)
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
		if err := serve(srv, tlsEnabled, tlsCertFile, tlsKeyFile, logger); err != nil && err != http.ErrServerClosed {
			logger.Fatal("server error", zap.Error(err))
		}
	}()

	// ─── Layer 3: Net proxy server on :8443 ──────────────────────────────────
	netProxyAddr := envOr("AF_NETPROXY_ADDR", ":8443")
	netProxySrv := &http.Server{
		Addr:    netProxyAddr,
		Handler: np,
		// No write timeout — CONNECT tunnels are long-lived bidirectional streams.
		ReadTimeout: 60 * time.Second,
		IdleTimeout: 120 * time.Second,
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
				"AF_TLS_ENABLED=true but AF_TLS_CERT_FILE or AF_TLS_KEY_FILE is not set — " +
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

func validateProductionConfig(authDisabled bool, jwtSecrets []string, adminPassword, vaultKey string) error {
	if !strictConfigEnabled() {
		return nil
	}
	if authDisabled {
		return fmt.Errorf("AF_AUTH_DISABLED=true is not allowed when AF_ENV=production or AF_STRICT_CONFIG=true")
	}
	if len(jwtSecrets) == 0 || strings.TrimSpace(jwtSecrets[0]) == "" || jwtSecrets[0] == "dev-secret-change-in-production" {
		return fmt.Errorf("AF_JWT_SECRET/AF_JWT_SECRETS must be set to a non-default value")
	}
	if adminPassword == "admin" || strings.TrimSpace(adminPassword) == "" {
		return fmt.Errorf("AF_ADMIN_PASSWORD must be set to a non-default value")
	}
	if strings.TrimSpace(vaultKey) == "" || vaultKey == strings.Repeat("0", 64) {
		return fmt.Errorf("AF_VAULT_KEY must be set to a non-default value")
	}
	return nil
}

func strictConfigEnabled() bool {
	if os.Getenv("AF_STRICT_CONFIG") == "true" {
		return true
	}
	return strings.EqualFold(os.Getenv("AF_ENV"), "production")
}
