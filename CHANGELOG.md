# Changelog

All notable changes to AgentFabric are documented in this file.

Format: [Semantic Versioning](https://semver.org/) — `[MAJOR.MINOR.PATCH] YYYY-MM-DD`

---

## [Unreleased] — v1.0.0 GA

### Planned
- E2E test suite against real pipeline (full span ingest → storage → API → UI)
- mTLS enabled in dev docker-compose (certs generated; compose wiring next)
- Secret rotation: AF_JWT_SECRETS multi-key support
- Token refresh endpoint (`POST /auth/refresh`)
- User management API (`/api/v1/users` CRUD)
- Helm chart production smoke test
- Grafana alert notification channels (PagerDuty, Slack webhook)

---

## [0.4.0] — 2026-03-16 — Sprint 4: Production Hardening

### New Features

- **S4-1 Password Login in Real API Gateway** — `api-gateway/internal/auth/oidc.go`:
  Added `POST /auth/login` handler (`PasswordLogin`) to the real Go api-gateway, replacing
  the mock-only implementation. Accepts `{username, password}` JSON, validates against
  `AF_ADMIN_USER` / `AF_ADMIN_PASSWORD` env vars (defaults: `admin` / `admin`), issues
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
  - `dashboards/agentfabric-overview.json` — 15-panel production dashboard covering:
    service health (UP/DOWN), HTTP request rate, error rate %, P50/P95/P99 latency,
    latency by endpoint, spans/min throughput, processor queue depth, spans by framework,
    cost rate (USD/hour), token rate, cost by framework, PII redaction rate, and policy
    decisions. Auto-loads on `docker-compose up` at `http://localhost:9091`.

- **S4-4 Prometheus Alert Rules** — `monitoring/alerts.yml`: Six production-grade alert
  rules across three groups:
  - `agentfabric.service_health`: ServiceDown, CollectorNotReceivingSpans, HighProcessorQueueDepth
  - `agentfabric.api_gateway`: HighErrorRate (>5% 5xx), HighP95Latency (>2s), HighRateLimitRate
  - `agentfabric.cost`: UnexpectedCostSpike (>$100/hour projected)
  All alerts include runbook URLs. `monitoring/prometheus.yml` updated to reference the rules
  file and alerting config block.

- **S4-5 mTLS Certificate Generation Script** — `scripts/generate-dev-certs.sh`:
  Generates a self-signed dev CA, collector server cert (with SAN for DNS:collector,
  DNS:localhost, IP:127.0.0.1), and client cert using OpenSSL. Certs land in
  `deploy/certs/` which is gitignored. Comment block in the script explains how to
  enable `AF_TLS_ENABLED: "true"` in docker-compose once certs are generated.
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
  Falls back to unsafe claim-only parsing when `AF_OIDC_ISSUER` is not configured (dev mode).

### New Features
- **S2-2 Per-tenant Rate Limiting** — `api-gateway/internal/middleware/ratelimit.go`: sliding-window
  rate limiter backed by Redis INCR + EXPIRE pipeline. Key: `rl:{tenantID}:{windowMinute}`.
  Returns 429 with `Retry-After`, `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset`.
  Configured via `AF_RATE_LIMIT_RPM` env var (default 1000 req/min). Fails open on Redis
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
- `api-gateway/cmd/server/main.go`: Added `strconv` import, `AF_RATE_LIMIT_RPM` env var,
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
  Configure via env: `AF_OIDC_ISSUER`, `AF_OIDC_CLIENT_ID`, `AF_OIDC_CLIENT_SECRET`,
  `AF_OIDC_REDIRECT_URI`, `AF_OIDC_LOGOUT_URL`.

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

[Unreleased]: https://github.com/agentfabric/agentfabric/compare/v0.4.0...HEAD
[0.4.0]: https://github.com/agentfabric/agentfabric/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/agentfabric/agentfabric/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/agentfabric/agentfabric/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/agentfabric/agentfabric/compare/v0.0.1...v0.1.0
[0.0.1]: https://github.com/agentfabric/agentfabric/releases/tag/v0.0.1
