# Changelog

All notable changes to AgentFabric are documented in this file.

Format: [Semantic Versioning](https://semver.org/) — `[MAJOR.MINOR.PATCH] YYYY-MM-DD`

---

## [Unreleased] — Sprint 3 target

### Planned
- mTLS wired in collector dev config
- Secret rotation mechanism
- Monitoring dashboards (Grafana)
- Coverage gate raised to 80% (Sprint 3 target)
- Agent-sdk integration tests for `_patch_crewai`, `_patch_langgraph`, etc.

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

[Unreleased]: https://github.com/agentfabric/agentfabric/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/agentfabric/agentfabric/compare/v0.0.1...v0.1.0
[0.0.1]: https://github.com/agentfabric/agentfabric/releases/tag/v0.0.1
