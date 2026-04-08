# Changelog

All notable changes to Govagn are documented in this file.

Format: [Semantic Versioning](https://semver.org/) — `[MAJOR.MINOR.PATCH] YYYY-MM-DD`

---

## [Unreleased]

### Planned
- Grafana alert notification channels (PagerDuty, Slack webhook)
- mTLS wiring in dev docker-compose (certs script already in `scripts/generate-dev-certs.sh`)
- Per-tenant audit retention policy enforcement
- Agent SDK trace sampling rate configuration

---

## [1.1.0] — 2026-03-16 — Production Hardening Sprint

All items verified: `make test` (113 portal + all Go tests pass), `make build` clean, `npx tsc --noEmit` zero errors.

### Security

- **H1 Constant-time admin authentication** — `api-gateway/internal/auth/oidc.go`:
  Replaced `!=` equality checks on username and password with
  `crypto/subtle.ConstantTimeCompare`. Both comparisons always execute regardless of
  whether the username is valid, eliminating a timing oracle that would allow an attacker
  to enumerate valid usernames via response-time differences. Zero new dependencies
  (stdlib `crypto/subtle`).

- **H2 JWT zero-downtime key rotation** — `api-gateway/cmd/server/main.go`:
  `GV_JWT_SECRETS` env var accepts a comma-separated list of signing secrets.
  The first entry is the active signing key; all entries are accepted for verification.
  Rotate by prepending a new secret, deploying, then removing the old one — no active
  sessions are invalidated. `parseSecrets()` trims whitespace and skips empty segments.

- **H3 Production Compose fail-fast secrets** — `deploy/docker/docker-compose.prod.yml` (NEW):
  Compose override that enforces all required secrets at startup using `${VAR:?error}` syntax.
  `GV_AUTH_DISABLED: "false"` and `GV_AUTH_REQUIRE_AUTH: "true"` are hardcoded — they cannot
  be accidentally relaxed. Grafana anonymous access disabled. Portal default-creds hint hidden.

- **H4 `.env.example`** — `.env.example` (NEW):
  Comprehensive reference for all 30+ env vars tagged `(required)`, `(conditional)`,
  `(optional)`, or `(dev-only)`. Includes secret generation commands, secrets policy, OIDC
  section, and rotation guidance. Whitelisted in `.gitignore` via `!.env.example`.

### Added

- **H5 TLS wiring on api-gateway** — `api-gateway/cmd/server/main.go`:
  New `serve()` helper reads `GV_TLS_ENABLED`, `GV_TLS_CERT_FILE`, `GV_TLS_KEY_FILE`.
  Fail-secure contract: if `GV_TLS_ENABLED=true` and cert/key paths are empty, the server
  returns a descriptive error immediately and **never** silently falls back to plain HTTP.
  8 new unit tests in `api-gateway/cmd/server/main_test.go`:
  `TestServe_TLSDisabled` (real HTTP GET), `TestServe_TLSMissingCert` (error names env vars),
  `TestServe_TLSCertFileNotFound`, and 5 `TestParseSecrets_*` edge-case tests.

- **H6 Versioned database migrations** — `deploy/migrations/` (NEW directory):
  - `001_initial_schema.up.sql` — promoted from `deploy/sql/init.sql`; authoritative first
    migration containing all 10 tables, RLS policies, immutable audit-log rules
    (`no_update_audit`, `no_delete_audit`), `af_audit_writer` INSERT-only role, and seed data.
  - `001_initial_schema.down.sql` — full rollback in strict reverse FK dependency order;
    revokes `af_audit_writer` grants before dropping the role.
  - `api-gateway/go.mod`: added `github.com/golang-migrate/migrate/v4 v4.17.1` as direct dep
    with `lib/pq` postgres driver.
  - `api-gateway/cmd/server/main.go`: `runMigrations()` called at startup before HTTP bind.
    `GV_MIGRATE_ON_STARTUP=false` skips (for tests / read-only replicas).
    `GV_MIGRATIONS_PATH` overrides the default `deploy/migrations` path.
  - `Makefile`: `make migrate/up`, `make migrate/down`, `make migrate/status` via `go run`
    (no separate binary install; version pinned in go.mod). `DATABASE_URL`-aware via
    `MIGRATE_DSN` variable.

- **H7 Portal RBAC-aware UI** — defence-in-depth UI layer (API already enforces 403):
  - `portal/src/hooks/auth.ts`: `hasRole(user, allowedRoles)` pure bool helper;
    `isSelfOrRole(user, roles, subjectId)` ABAC helper mirrors backend `RequireRoleOrSelf`.
  - `portal/src/App.tsx`: `RequireRole` component — renders children only when
    `hasRole(user, roles)` is true, returns `fallback` (default `null`) otherwise.
    `/users` route added.
  - `portal/src/components/Layout.tsx`: `adminOnly: true` flag on nav items (Users nav hidden
    from editors/viewers). Role badge chip in sidebar footer: **ADMIN** red · **EDITOR** amber ·
    **VIEWER** slate.
  - `portal/src/pages/UsersPage.tsx` (NEW): full user management page — list all users,
    Create (admin only), Edit (admin or own record — ABAC parity), Delete (admin only,
    two-step confirm, self-delete disabled).
  - `portal/src/pages/LoginPage.tsx`: "Default credentials: admin / admin" hint now guarded
    by `VITE_SHOW_DEFAULT_CREDS === 'true'` — hidden in all production builds.
  - `portal/src/hooks/auth.test.ts`: +21 tests (113 total) covering `hasRole` (8),
    `isSelfOrRole` (6), `RequireRole` gating contract (5).

### Changed

- **H8 K8s resource limits corrected** — `deploy/k8s/govagn.yaml` (full rewrite, 19 documents):
  - Collector resources corrected: `cpu 100m→200m / 500m→1000m`, `mem 128Mi→256Mi / 512Mi→1Gi`
    (collector is CPU-heavy — span processing, PII scrubbing, batch export).
  - API Gateway resources corrected: `cpu 200m→100m / 1000m→500m`, `mem 256Mi→128Mi / 1Gi→512Mi`
    (gateway is I/O-bound — database + Redis reads).
  - Probe timings tightened: liveness `periodSeconds 30→10`; readiness `initialDelaySeconds`
    reduced; `failureThreshold: 3` explicit on every probe.
  - `GV_JWT_SECRETS` and `GV_ADMIN_PASSWORD` wired into gateway Deployment env (were absent).
  - af-core and portal Deployments + Services added (were entirely missing from the manifest).
  - af-core HPA added: min 2, max 8, at 70% CPU.
  - 3 `PodDisruptionBudget` resources added (`policy/v1`, `minAvailable: 1`) for gateway,
    collector, and af-core — prevents simultaneous drain from taking all pods offline.

- **H9 Helm chart aligned** — `deploy/helm/values.yaml`:
  - af-core, gateway, portal resource blocks added.
  - Per-service `podDisruptionBudget` flags added.
  - `api.auth.jwtSecrets` rotation field added.
  - `deploy/helm/Chart.yaml`: version bumped `1.0.0 → 1.1.0`.

### Tests Added

- `api-gateway/cmd/server/main_test.go` (NEW) — 8 tests: TLS disabled HTTP, fail-secure TLS
  missing cert, cert file not found, `parseSecrets` multi/trim/single/empty/segment cases.
- `portal/src/hooks/auth.test.ts` — extended from 16 to 37 tests (see H7).

---

## [1.0.0] — 2026-03-16 — GA Release

### New Features

- **GA-1 Token Refresh** — `api-gateway/internal/auth/oidc.go`:
  Added `POST /auth/refresh` endpoint. Accepts `Authorization: Bearer <existing_token>`,
  verifies the HS256 signature, and re-issues a new AF JWT with a refreshed 8-hour expiry
  carrying the same identity claims (sub, email, name). Returns `{"token":"…","expires_in":28800}`.
  Enables the portal to silently refresh sessions without forcing users to re-login.

- **GA-2 User Management CRUD** — Full `/api/v1/users` REST resource:
  - **`models.go`**: Added `User`, `CreateUserRequest`, `UpdateUserRequest` types.
  - **`store/postgres.go`**: Added `ListUsers`, `GetUser`, `CreateUser`, `UpdateUser`,
    `DeleteUser` store methods. Schema const updated with `users` table (tenant-isolated,
    username/email unique per tenant). Passwords are SHA-256 hashed before storage;
    TODO(v1.1) notes upgrade path to bcrypt. Added `generateStoreID()` and `hashPassword()`
    helpers. Imported `crypto/rand`, `crypto/sha256`, `encoding/hex`, `strings`.
  - **`handlers/handlers.go`**: `ListUsers` (GET `/api/v1/users`), `GetUser`
    (GET `/{userId}`), `CreateUser` (POST, validates username + email), `UpdateUser`
    (PUT, partial update semantics), `DeleteUser` (DELETE, 204 on success).
  - **`cmd/server/main.go`**: Wired `/auth/refresh` + `/api/v1/users` route group.

### CI/CD

- **GA-3 Helm Chart Smoke Test** — `.github/workflows/ci.yml`: New `helm-smoke` job runs
  on every PR:
  1. `helm lint deploy/helm` — validates chart structure and YAML correctness.
  2. `helm template` dry-render verifying ≥10 Kubernetes resources are produced.
  3. `helm install --dry-run --generate-name` — full client-side dry-run.
  `ci-gate` now requires all 6 jobs (added `helm-smoke`) to pass before merge is allowed.

### Tests Added

- `api-gateway/internal/auth/oidc_test.go` — 8 new `TestRefresh_*` tests:
  valid token → 200 with new JWT, new token has fresh expiry (~8 h), identity preserved
  (sub/email unchanged), `expires_in` matches `SessionMaxAge`, missing token → 401,
  invalid JWT → 401, wrong signing secret → 401.
- `tests/e2e/test_e2e_pipeline.py` — 8 new `@pytest.mark.integration` tests covering
  auth login (valid/invalid), auth refresh (valid/missing token), and users CRUD lifecycle
  (list, create→read→update→delete, field validation, 404 on missing). Added
  `api_gateway_url` session-scoped fixture (reads `GV_GATEWAY_URL` env, defaults to
  `http://localhost:8080`). Updated module docstring to list tests 16–23.

---

## [0.4.0] — 2026-03-16 — Sprint 4: Production Hardening

### New Features

- **S4-1 Password Login in Real API Gateway** — `api-gateway/internal/auth/oidc.go`:
  Added `POST /auth/login` handler (`PasswordLogin`) to the real Go api-gateway, replacing
  the mock-only implementation. Accepts `{username, password}` JSON, validates against
  `GV_ADMIN_USER` / `GV_ADMIN_PASSWORD` env vars (defaults: `admin` / `admin`), issues
  an AF HS256 JWT using the identical token path as OIDC login. Whitespace is trimmed from
  credentials; a generic 401 is returned on mismatch to avoid username enumeration.
  `OIDCConfig` gains `AdminUser` + `AdminPassword` fields populated from env in `main.go`.

- **S4-2 Users Table** — `deploy/sql/init.sql`: Added `users` table with tenant isolation,
  bcrypt password hashes via pgcrypto `crypt()`/`gen_salt('bf', 10)`, role-based access
  (admin / editor / viewer), last-login tracking, and unique constraints on
  `(tenant_id, username)` and `(tenant_id, email)`. Default admin user seeded on
  first `docker-compose up` with password `admin`.

- **S4-3 Grafana Dashboards (Auto-Provisioned)** — `monitoring/grafana/`:
  - `provisioning/datasources/prometheus.yaml` — Prometheus datasource, auto-registered.
  - `provisioning/dashboards/dashboards.yaml` — filesystem dashboard provider.
  - `dashboards/govagn-overview.json` — 15-panel production dashboard covering:
    service health (UP/DOWN), HTTP request rate, error rate %, P50/P95/P99 latency,
    latency by endpoint, spans/min throughput, processor queue depth, spans by framework,
    cost rate (USD/hour), token rate, cost by framework, PII redaction rate, and policy
    decisions. Auto-loads on `docker-compose up` at `http://localhost:9091`.

- **S4-4 Prometheus Alert Rules** — `monitoring/alerts.yml`: Six production-grade alert
  rules across three groups:
  - `govagn.service_health`: ServiceDown, CollectorNotReceivingSpans, HighProcessorQueueDepth
  - `govagn.api_gateway`: HighErrorRate (>5% 5xx), HighP95Latency (>2s), HighRateLimitRate
  - `govagn.cost`: UnexpectedCostSpike (>$100/hour projected)
  All alerts include runbook URLs. `monitoring/prometheus.yml` updated to reference the rules
  file and alerting config block.

- **S4-5 mTLS Certificate Generation Script** — `scripts/generate-dev-certs.sh`:
  Generates a self-signed dev CA, collector server cert (with SAN for DNS:collector,
  DNS:localhost, IP:127.0.0.1), and client cert using OpenSSL. Certs land in
  `deploy/certs/` which is gitignored. Comment block in the script explains how to
  enable `GV_TLS_ENABLED: "true"` in docker-compose once certs are generated.
  The collector code already supports mTLS when this env var is set.

- **S4-6 af-core Readiness Probe** — `af-core/src/server/http.rs`: Upgraded `/healthz`
  to return structured JSON `{"status":"ok","service":"af-core","version":"<semver>"}`.
  Added proper `/readyz` readiness check returning `{"status":"ready"}` (200) or
  `{"status":"not_ready","reason":"..."}` (503). Kubernetes liveness vs. readiness probes
  now route correctly to their respective endpoints.

### CI/CD

- **S4-7 Coverage gate raised 80% → 90%** — `.github/workflows/ci.yml`: all five services
  (af-core, collector, api-gateway, portal, agent-sdk) now gate at 90%. Added
  `--cov-report=json` to pytest and a `Enforce coverage gate (90% Sprint 4)` step to
  agent-sdk (previously had no gate, only reporting).

### Tests Added

- `api-gateway/internal/auth/oidc_test.go` — 10 new tests for `PasswordLogin`:
  valid credentials → 200 with JWT, token carries correct claims (sub/email/iss/aud),
  wrong password → 401, wrong username → 401, empty username → 400, empty password → 400,
  invalid JSON → 400, custom credentials via config, whitespace trimming, default fallback.
  Import updated to add `net/http/httptest`.

---

## [0.3.0] — 2026-03-15 — Sprint 3: Portal Completeness + Coverage Gate

### New Features
- **S3-1 Topology Graph Component** — `portal/src/components/TopologyGraph.tsx`: SVG-based DAG
  renderer showing parent→child span relationships. Nodes are color-coded by framework and
  clickable for span selection. Responsive SVG viewBox layout adapts to any trace depth.
  Resolves the TraceDetail "Graph" tab placeholder left from Sprint 2.

- **S3-2 TracesPage Pagination** — `portal/src/pages/TracesPage.tsx`: Added cursor-based
  pagination (100 traces/page) with Prev/Next navigation controls and a "Showing X–Y results"
  display. Client-side text search filters on trace ID and root span name without a round trip.

- **S3-3 CostPage Token Attribution** — `portal/src/pages/CostPage.tsx`: Added input/output
  token breakdown stat cards and a cost-by-framework percentage section. Previously the page
  only showed trace-count share; it now shows actual cost share per framework.

- **S3-4 EnvironmentsPage Dynamic Collectors** — `portal/src/pages/EnvironmentsPage.tsx`:
  Collector endpoints are now fetched live from `/api/v1/collectors` with graceful static
  fallback when the endpoint is unavailable. Added per-endpoint copy-to-clipboard and a
  last-checked timestamp for each collector.

### CI/CD
- **S3-5 Coverage gate raised 60% → 80%** — `.github/workflows/ci.yml`: all four services
  (af-core, collector, api-gateway, portal) now gate at 80%. Sprint 4 target: 90%.

### Tests Added
- All 7 portal pages now have dedicated test files: Dashboard, Traces, TraceDetail, LiveStream,
  Agents, Cost, Environments
- `portal/src/pages/LoginPage.test.tsx` — LoginPage test added
- `portal/src/components/TopologyGraph.test.tsx` — TopologyGraph component test added
- `portal/src/hooks/api.test.ts` and `portal/src/hooks/auth.test.ts` — updated for new
  coverage requirements
- Total portal tests: 87 passing (8 test files). Red-green-clean cycle: 6 precision fixes
  applied (getByText → getAllByText for ambiguous text nodes).

---

## [0.2.0] — 2026-03-13 — Sprint 2: Security Hardening + Observability

### Security Improvements
- **S2-1 JWKS Signature Verification** — `api-gateway/internal/auth/oidc.go`: OIDC ID tokens
  are now verified against the provider's RS256 public keys fetched from `jwks_uri`. Keys are
  cached in-memory for 15 minutes with automatic invalidation on unknown `kid` (key rotation).
  A `parseJWKSPublicKey()` helper parses RSA JWK `n`/`e` fields using only stdlib (no new deps).
  Falls back to unsafe claim-only parsing when `GV_OIDC_ISSUER` is not configured (dev mode).

### New Features
- **S2-2 Per-tenant Rate Limiting** — `api-gateway/internal/middleware/ratelimit.go`: sliding-window
  rate limiter backed by Redis INCR + EXPIRE pipeline. Key: `rl:{tenantID}:{windowMinute}`.
  Returns 429 with `Retry-After`, `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset`.
  Configured via `GV_RATE_LIMIT_RPM` env var (default 1000 req/min). Fails open on Redis
  unavailability. Principle 2: each tenant has an independent counter.
  Wired in `main.go` after `TenantInjector` on all `/api/v1` routes.

- **S2-3 Audit Log API** — `GET /api/v1/audit` (paginated policy decisions) and
  `GET /api/v1/audit/verify` (SHA-256 chain replay returning 200 intact / 409 broken).
  Chain verification replicates the exact payload format from `af-core/src/policy/audit.rs`.
  Store methods: `ListAuditEntries`, `VerifyAuditChain` added to `api-gateway/internal/store/postgres.go`.

- **S2-4 Portal OIDC Session Management** — Full session lifecycle in portal:
  - `portal/src/hooks/auth.ts`: `useAuth()` hook, `getToken()` reads `af_token` cookie first
    (set by OIDC callback), falls back to `localStorage` (dev). `isAuthEnabled()` checks
    `VITE_AUTH_DISABLED`.
  - `portal/src/hooks/api.ts`: Fixed static TOKEN bug — now calls `getToken()` dynamically
    on every request, picking up OIDC-issued cookies without page reload.
  - `portal/src/pages/LoginPage.tsx`: Enterprise SSO login page with "Continue with SSO"
    button, auto-redirect if already authenticated, dev-mode bypass.
  - `portal/src/App.tsx`: Added `/login` route + `<RequireAuth>` guard wrapping all
    protected routes.
  - `portal/src/components/Layout.tsx`: Sidebar footer now shows user email + logout button
    when authenticated (via `useAuth()`).

### CI/CD
- **S2-6 Coverage gate raised 40% → 60%** — `.github/workflows/ci.yml`: all three Go/Rust
  coverage gates updated from 40 to 60%. Comment updated for Sprint 3 (80%) and Sprint 4 (90%).

### Tests Added
- `api-gateway/internal/middleware/ratelimit_test.go` — 11 tests: within-limit pass, 429 with
  headers, Principle 2 tenant isolation, Redis fail-open, concurrent no-panic, key format
- `api-gateway/internal/auth/oidc.go` — JWKS cache, `parseJWKSPublicKey`, `fetchJWKS`
  added (existing `oidc_test.go` covers PKCE/cookie; JWKS tests covered by functional flow)
- `af-core/src/policy/audit_tests.rs` — 11 tests: genesis chain, sequential linking, tamper
  detection, deletion detection, clean chain passes, determinism, different policies/results
  produce different hashes, Principle 4 compliance (entry_hash never empty, insert-only proof)
- `portal/src/hooks/auth.test.ts` — 11 tests: `getToken` cookie/localStorage priority,
  `isAuthEnabled` env flag, JWT shape validation

### Infrastructure
- `api-gateway/internal/store/redis.go`: Added `IncrWithExpiry` method (pipelined INCR + EXPIRE)
  satisfying `middleware.RateLimitStore` interface
- `af-core/src/policy/mod.rs`: Wired `mod audit_tests`
- `api-gateway/cmd/server/main.go`: Added `strconv` import, `GV_RATE_LIMIT_RPM` env var,
  rate limiter middleware, `/api/v1/audit` and `/api/v1/audit/verify` routes

---

## [0.1.0] — 2026-03-13 — Sprint 1: Production Foundation

### Breaking Changes
- `EnrichedSpan` JSON schema now includes `input_cost_usd` and `output_cost_usd` fields.
  Consumers relying on only `cost_usd` must be updated. The collector is the authoritative
  source; downstream services must not recompute costs (Principle 6).

### Security Fixes
- **P0-3 SQL Injection** — `af-core/src/storage/postgres.rs`: Replaced unsafe dynamic SQL
  string formatting with `sqlx::QueryBuilder` parameterized queries. All user-controlled
  values (tenant_id, framework, timestamps, pagination) are now bound parameters.

### Bug Fixes
- **P0-1 Cost Computation** — Cost was split 60/40 (hardcoded) between input and output tokens.
  Fixed: collector now computes `input_cost_usd` and `output_cost_usd` independently using
  per-model pricing tables and propagates both fields through the Kafka pipeline to af-core.
  Files: `collector/internal/processor/agent_processor.go`, `af-core/src/pipeline/span.rs`,
  `af-core/src/storage/postgres.rs`.
- **P0-2 N+1 Query** — `GetAgentTopology` issued one SQL query per trace ID. Replaced with
  a single batch query using `WHERE trace_id = ANY($1)`. Files: `api-gateway/internal/store/postgres.go`,
  `api-gateway/internal/handlers/handlers.go`.

### New Features
- **P0-4 OIDC Enterprise SSO** — Full Authorization Code + PKCE flow scaffolded in
  `api-gateway/internal/auth/oidc.go`. Supports Okta, Azure AD, and any OpenID Connect provider.
  Endpoints: `GET /auth/login`, `GET /auth/callback`, `GET /auth/logout`, `GET /auth/me`.
  State is stored as a signed HMAC JWT in an HttpOnly cookie (stateless — no Redis required).
  AF session tokens are HS256 JWTs with 8-hour expiry, issued on successful callback.
  Configure via env: `GV_OIDC_ISSUER`, `GV_OIDC_CLIENT_ID`, `GV_OIDC_CLIENT_SECRET`,
  `GV_OIDC_REDIRECT_URI`, `GV_OIDC_LOGOUT_URL`.

### CI/CD
- **P0-5 Test Coverage** — CI pipeline created at `.github/workflows/ci.yml`:
  - af-core (Rust): cargo-tarpaulin, 40% coverage gate, Clippy zero-warnings
  - collector (Go): race detector, 40% coverage gate
  - api-gateway (Go): race detector, 40% coverage gate, Postgres + Redis service containers
  - portal (TypeScript): vitest + @testing-library/react, coverage reporting
  - agent-sdk (Python): pytest-cov, coverage reporting
  - `ci-gate` job blocks merges if any service fails (env-var injection-safe)

### Tests Added
- `af-core/src/pipeline/tests.rs` — 35+ unit tests: `EnrichedSpan` fields, `TraceGraph` DAG,
  `AnomalyDetector`, all 5 policy types, `PolicyEngine.aggregate_result`, Principle compliance
- `collector/internal/processor/agent_processor_test.go` — 30+ tests: `computeCost`,
  `detectFramework`, `scrubPII`; includes P0-1 and Principle 5/7 compliance tests
- `api-gateway/internal/handlers/handlers_test.go` — pure function tests: `buildTrace`,
  `buildTopologyGraph`, `parseIntOr`, P0-2 fix documentation
- `api-gateway/internal/middleware/middleware_test.go` — JWTAuth, TenantInjector,
  CollectorAuth middleware tests
- `api-gateway/internal/auth/oidc_test.go` — 30+ tests: PKCE S256, nonce uniqueness,
  ID token parsing, AF token issuance, state cookie tamper detection, URL helpers
- `portal/src/pages/Dashboard.test.tsx` — 7 tests: loading state, stat values, zero safety
- `portal/src/hooks/api.test.ts` — type shape tests, URL construction logic
- `agent-sdk/tests/test_attrs.py` — 18 attribute constant tests, Principle 2/5/7 compliance

### Infrastructure
- `af-core/Cargo.toml`: added `tokio-test`, `mockall` dev-dependencies
- `portal/package.json`: added `@testing-library/react`, `@testing-library/user-event`,
  `@testing-library/jest-dom`, `@vitest/coverage-v8`, `jsdom`
- `portal/vitest.config.ts`: vitest configuration with jsdom environment and v8 coverage
- `portal/src/test/setup.ts`: jest-dom matchers setup
- `collector/go.mod`: added `github.com/stretchr/testify` test dependency

---

## [0.0.1] — Initial Architecture

### Added
- Multi-service architecture: af-core, collector, api-gateway, portal, agent-sdk
- OTLP ingestion (gRPC :4317, HTTP :4318) in collector
- Framework detection for CrewAI, LangGraph, OpenAI Agents, Google ADK, Claude Agents
- PII scrubbing (7 patterns: email, SSN, phone, credit card, IP, API key, JWT)
- JWT auth middleware in api-gateway
- WebSocket live stream hub
- React dashboard: Dashboard, LiveStream, Traces, TraceDetail, Agents, Cost, Environments pages
- PostgreSQL + ClickHouse + Redis storage layer
- Docker Compose full dev stack (12 services)
- Kubernetes manifests + Helm chart
- Prometheus scrape configs

[Unreleased]: https://github.com/rastogivaibhav/Govagn/compare/v1.1.0...HEAD
[1.1.0]: https://github.com/rastogivaibhav/Govagn/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/rastogivaibhav/Govagn/compare/v0.4.0...v1.0.0
[0.4.0]: https://github.com/rastogivaibhav/Govagn/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/rastogivaibhav/Govagn/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/rastogivaibhav/Govagn/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/rastogivaibhav/Govagn/compare/v0.0.1...v0.1.0
[0.0.1]: https://github.com/rastogivaibhav/Govagn/releases/tag/v0.0.1
