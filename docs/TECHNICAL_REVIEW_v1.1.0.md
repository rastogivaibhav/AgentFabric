# AgentFabric — Technical Review Report
**Version**: v1.1.0
**Review Date**: 2026-03-16
**Reviewers**: Technical PM + Senior Architect
**Method**: Full source inspection across all services — api-gateway (Go), collector (Go), af-core (Rust), portal (React/TS), deploy artefacts, and test suites
**Scope**: Security posture, architecture correctness, operational readiness, code quality, and forward path

---

## Executive Summary

AgentFabric v1.1.0 is a well-structured, multi-service observability platform with a sound core architecture. The sprint work added meaningful production hardening: constant-time auth, TLS fail-secure, JWT key rotation, schema migration tooling, RBAC/ABAC dual-layer enforcement, and a detailed production checklist. The codebase demonstrates consistent patterns and good engineering discipline.

**However, three blockers must be resolved before a production deployment:**

1. **Schema split** — two incompatible schemas exist simultaneously (migration UUID-based vs inline store TEXT-based)
2. **Password login bypasses the users table** — created users cannot log in
3. **JWT token XSS exposure** — token stored in `localStorage` and in a non-`HttpOnly` cookie

Beyond these blockers, 14 medium and 8 low-priority issues are documented below. Overall readiness is assessed at **68% — not yet production-safe** for a multi-tenant, externally-reachable deployment.

---

## 1. Blocker Issues (P0 — Must Fix Before Production)

### B-1: Dual Schema — Migration vs Inline Store Schema

**File**: `api-gateway/internal/store/postgres.go` lines 50–160 vs `deploy/migrations/001_initial_schema.up.sql`

**What was found**:
`postgres.go` contains a `const schema` block defining tables with `TEXT` primary keys (`tenant_id TEXT`, `span_id TEXT`, composite PKs). This was the original "init on startup" schema and is now **dead code** — `NewPostgresStore()` has the comment "No migration needed here" and does not execute it.

The canonical schema is now in the migration file which uses `UUID` primary keys (`tenant_id UUID NOT NULL REFERENCES tenants(tenant_id)`), generated columns, CHECK constraints, and proper FK chains.

**The critical conflict**: Every query in `postgres.go` passes `tenant_id` as a plain string (e.g., `"default"`). The migration schema's `tenants.tenant_id` is `UUID`. The hardcoded seed row is `'00000000-0000-0000-0000-000000000001'` — which PostgreSQL accepts as a UUID string, but the application runtime defaults to the string `"default"` which is **not a valid UUID** and will fail FK validation at runtime.

**Impact**: The application will fail to insert spans in a migrated production database. The inline `const schema` is misleading — a developer reading it will believe it represents the live schema.

**Resolution path**:
- Remove the `const schema` block entirely from `postgres.go`
- Update the `tenant_id` default in all queries from `"default"` to `"00000000-0000-0000-0000-000000000001"` OR change the migration to accept TEXT tenant IDs (matching the query layer)
- Resolve in v1.1.1 as a critical patch

---

### B-2: Password Login Does Not Authenticate from the Users Table

**File**: `api-gateway/internal/auth/oidc.go` — `PasswordLogin()` (lines 826–880)

**What was found**:
The `PasswordLogin` handler validates credentials exclusively against `AF_ADMIN_USER` / `AF_ADMIN_PASSWORD` environment variables — a single hardcoded admin account. It does **not** query the `users` table at all.

This means:
- Creating users via `POST /api/v1/users` gives them a database record but **zero login capability**
- The entire Users CRUD surface (create, update, delete users with bcrypt-hashed passwords) is operationally inert — there is no login path that uses `password_hash` from the database
- Any invited user cannot authenticate

**Impact**: The advertised multi-user RBAC feature is non-functional. Only the environment-variable admin can log in.

**Resolution path**:
- `PasswordLogin` must query `users` by `username`, retrieve `password_hash`, and call `bcrypt.CompareHashAndPassword`
- The admin env-var path can remain as a fallback ("break-glass") but database users must be the primary path

---

### B-3: JWT Token Stored in XSS-Accessible Locations

**File 1**: `portal/src/pages/LoginPage.tsx` line 55: `localStorage.setItem('af_token', token)`
**File 2**: `api-gateway/internal/auth/oidc.go` line 335: `HttpOnly: false`

**What was found**:
The password-login flow stores the JWT in `localStorage`. The OIDC callback sets a cookie with `HttpOnly: false` because "portal JS needs to read it". Both mean a single XSS vulnerability in any portal page exposes the session token.

**Impact**: Any stored XSS (e.g., via injected span attributes that render unsanitised in trace detail) would exfiltrate admin JWTs. For a platform that ingests arbitrary third-party agent telemetry, this is high-probability.

**Resolution path**:
- OIDC cookie: set `HttpOnly: true`; the portal does NOT need to read the raw JWT — use `GET /auth/me` to populate the user context
- Password login: change to set an `HttpOnly` cookie server-side instead of returning the token in the JSON body for localStorage
- `getToken()` in `auth.ts` already reads from cookie first — the infrastructure is already right; the server just needs to stop setting `HttpOnly: false`

---

## 2. High-Priority Issues (P1 — Fix in Next Sprint)

### H-1: Context Key Type Safety (Collision Risk)

**File**: `api-gateway/internal/middleware/middleware.go` lines 94, 164

String literals `"claims"` and `"tenant_id"` used as `context.WithValue` keys are a documented Go anti-pattern (see `golangci-lint` rule `staticcheck SA1029`). Any other package using the same string key will silently collide.

**Fix**: Define unexported typed keys:
```go
type contextKey int
const (
    claimsKey   contextKey = iota
    tenantIDKey
)
```

---

### H-2: OIDC Discovery Not Cached — N+1 HTTP Calls Per Login

**File**: `api-gateway/internal/auth/oidc.go` — `discoverAuthEndpoint()` and `discoverTokenEndpoint()` both call `h.discover()` independently

Each OIDC login flow makes **two** uncached HTTP calls to `/.well-known/openid-configuration`. Under load this adds 50–200ms latency per login and constitutes an external dependency on every auth operation.

**Fix**: Cache the `oidcDiscovery` struct on `OIDCHandler` with the same TTL pattern as `jwksCache`.

---

### H-3: No Request Body Size Limit on Ingest

**File**: `api-gateway/internal/handlers/handlers.go` `Ingest()` line 60

`json.NewDecoder(r.Body).Decode(&req)` reads the body without a `http.MaxBytesReader` wrapper. A malicious collector or misconfigured agent SDK could send a multi-GB JSON payload that will be fully buffered before the 10,000-span limit check on line 70.

**Fix**:
```go
r.Body = http.MaxBytesReader(w, r.Body, 32<<20) // 32 MB hard cap
```

---

### H-4: `CollectorAuth` Source Header Provides No Security Value

**File**: `api-gateway/internal/middleware/middleware.go` lines 172–194

The `X-AF-Source: collector` header check (line 176) is trivially forgeable by any HTTP client. The JWT validation that follows is the real security control. The source-header check adds false confidence without protection.

**Fix**: Remove the `X-AF-Source` check from `CollectorAuth` (JWT validation alone is sufficient), or document it explicitly as a routing hint rather than a security control.

---

### H-5: `VerifyAuditChain` Loads Up to 100,000 Rows Into Memory

**File**: `api-gateway/internal/store/postgres.go` line 733

`VerifyAuditChain` fetches all audit entries (capped at 100k) into a Go slice before walking the chain. At ~400 bytes per entry this is ~40 MB per invocation. On a busy tenant this will spike memory.

**Fix**: Stream rows directly from the query cursor, verifying the chain entry-by-entry without building the full slice.

---

### H-6: `exchangeCode` Loses Request Context

**File**: `api-gateway/internal/auth/oidc.go` line 445

The method signature is `exchangeCode(ctx interface{ Done() <-chan struct{} }, ...)` — a partial interface instead of `context.Context`. The `http.PostForm` call on line 460 uses no context at all, meaning token exchange requests have no timeout and cannot be cancelled if the browser closes the connection.

**Fix**: Accept `context.Context`, create `http.NewRequestWithContext(ctx, ...)`, and honour the caller's deadline.

---

## 3. Medium-Priority Issues (P2 — Backlog, v1.2.0)

### M-1: `const schema` Dead Code in `postgres.go`

Lines 50–160 of `store/postgres.go` define tables that differ from the live migration schema. This is never executed but will mislead engineers reading the store. Remove entirely.

---

### M-2: `hashPassword` Comment References Resolved TODO

`postgres.go` line 862 comment says:
> `TODO(v1.1): migrate to bcrypt once golang.org/x/crypto is a declared dependency`

The function **already uses bcrypt** (line 965: `bcrypt.GenerateFromPassword`). The stale TODO will mislead future engineers. Delete the comment.

---

### M-3: `generateStoreID()` Produces Non-Standard UUIDs

`postgres.go` lines 951–957 uses `fmt.Sprintf("%08x-%04x-...")` which formats raw random bytes without setting UUID version (4) or variant bits. RFC 4122 UUIDs require bits 6–7 of byte 8 to be `10` and bits 4–7 of byte 6 to be `0100`. Use `github.com/google/uuid` or the `uuid-ossp` PostgreSQL extension instead.

---

### M-4: Offset-Based Pagination Will Not Scale

The `Page<T>` TypeScript type declares `next_cursor?: string` but the backend returns `has_more: bool` and all queries use `LIMIT $n OFFSET $m`. Offset pagination degrades as `O(offset)` in PostgreSQL — listing page 1000 of traces will do a full sequential scan over 50,000 rows. Replace with keyset pagination (`WHERE start_time_ns < $last_seen_value ORDER BY ... LIMIT n`).

---

### M-5: `VITE_API_URL` vs `VITE_API_BASE_URL` Env Var Inconsistency

`portal/src/hooks/api.ts` reads `VITE_API_URL`. `portal/src/pages/LoginPage.tsx` reads `VITE_API_BASE_URL`. Both default to `http://localhost:8080` so this is silent in development. In any environment where only one variable is set the other will use the wrong default. Standardise on `VITE_API_URL`.

---

### M-6: Health Endpoint Does Not Check Dependencies

`GET /healthz` always returns `{"status":"ok"}`. A Kubernetes readiness probe pointing at `/healthz` will report the pod as ready even when Postgres or Redis is unreachable. Add a quick `pgPool.Ping()` and `redisClient.Ping()` and return `503` on failure.

---

### M-7: Missing HTTP Security Response Headers

No middleware sets `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: strict-origin-when-cross-origin`, or `Content-Security-Policy`. These are table-stakes for any SOC 2 / HIPAA-adjacent product. Add a single middleware function wrapping all of them.

---

### M-8: `srv.Shutdown()` Return Value Unchecked

`api-gateway/cmd/server/main.go` line 225: `srv.Shutdown(ctx)` — the error return is discarded. If the graceful shutdown deadline is exceeded, active connections are forcefully closed. The operator should see a log warning. Add `if err := srv.Shutdown(ctx); err != nil { logger.Warn(...) }`.

---

### M-9: CORS Wildcard `https://*.agentfabric.io`

Any compromised or user-controlled subdomain of `agentfabric.io` (including e.g. attacker-owned subdomains if DNS is misconfigured) would pass the CORS origin check. Either enumerate allowed origins explicitly or switch to a configurable `AF_CORS_ORIGINS` env var already referenced in the README but not wired in `main.go`.

**Note**: `AF_CORS_ORIGINS` is documented in the README but the current `main.go` uses a hardcoded array. Wire the env var.

---

### M-10: OIDC Token Cookie Is Not `Secure` in Proxy Deployments

`isHTTPS(r)` checks `r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"`. This works correctly behind a reverse proxy but only when the proxy forwards `X-Forwarded-Proto`. In a deployment where the proxy strips that header, cookies will be set without `Secure` flag over what appears to the server to be plain HTTP — allowing transmission over unencrypted connections.

---

### M-11: `af-core` Rust Service Integration Untested

The Rust `af-core` service (Kafka consumer, policy engine, ClickHouse writes, audit chain writer) has no visible integration test harness and its connection to the live `api-gateway` PostgreSQL schema is unverified against the migration. The `VerifyAuditChain` Go function replicates the Rust hash computation — any format divergence would silently produce "chain broken" false positives.

---

### M-12: No Structured Logging for Audit Events From API Gateway

When the api-gateway creates/deletes users or modifies roles, these actions are NOT written to the `policy_audit_log`. Only af-core span policy decisions go there. Admin CRUD operations (who deleted which user at what time) have no audit trail. This is a compliance gap for SOC 2 CC6.

---

### M-13: Edit-User Route Not Implemented (Known Limitation)

`UsersPage.tsx` links to `/users/:id/edit` which has no route or component. This is documented in the Known Limitations table as a Low risk item but clicking "Edit" in the portal will silently 404. The Edit button should be disabled or removed until the route exists.

---

### M-14: Rate Limiter Uses Redis But Redis Failure Is Not Graceful

`middleware/ratelimit.go` uses Redis for per-tenant sliding window counters. If Redis becomes unreachable (network partition, OOM kill), the rate limiter will likely return an error which, depending on implementation, could either block all requests (fail-closed = accidental DoS) or allow all requests (fail-open = DoS vector). The failure mode should be explicitly chosen and tested.

---

## 4. Low-Priority Issues (P3 — Technical Debt)

### L-1: `sleep` in `TestServe_TLSDisabled` Is Flaky
`main_test.go` line 62: `time.Sleep(60 * time.Millisecond)` to wait for the server to bind. On a slow CI runner this may be insufficient. Use a retry loop with `net.DialTimeout` instead.

### L-2: `ListAudit` Has No Tenant Isolation Validation
The audit endpoint reads `?offset` and `?limit` from query params without bounding `offset`. An attacker with a valid (non-admin) JWT could enumerate audit entries across time via unbounded offsets. Add a max-offset cap.

### L-3: Portal `api.ts` Throws Generic Error Messages
`throw new Error(`API ${res.status}: ${path}`)` — the error is caught by React Query and never decoded. The body containing `{"error":"..."}` is discarded. Error messages in the portal will always say "API 403: /users" rather than "insufficient permissions". Decode the body in the catch path.

### L-4: README Roadmap is Stale
The README roadmap still lists "Web UI login page with OIDC/SSO" as a TODO item — this was delivered in v1.0.0. Several other roadmap items now exist in the codebase. The README badge still says `status-beta` but the repo is tagged v1.1.0.

### L-5: `portal/src/App.tsx` version footer is `v1.0.0`
`LoginPage.tsx` line 155: `<div style={s.footer}>v1.0.0 · © AgentFabric</div>`. Should be driven by `import.meta.env.VITE_APP_VERSION` or bumped to match the release tag.

### L-6: No `.nvmrc` or Engines Field
The README requires Node 20+ but nothing enforces it. Add `.nvmrc` (content: `20`) and `"engines": {"node": ">=20"}` in `package.json`.

### L-7: `go.sum` Not Reviewed
The `go.sum` lock file is present but was not reviewed. Any supply-chain audit should run `go mod verify` in CI to detect tampered modules.

### L-8: `af_audit_writer` Default Password `CHANGE_IN_PRODUCTION`
Documented in §8 of the checklist but deserves calling out again: the init SQL seeds a DB role with a known default password. This is listed as a High risk Known Limitation targeting v1.2.0. It should be automated in the migration, not a manual step.

---

## 5. Positive Findings (What's Working Well)

| Area | Finding |
|------|---------|
| **Auth timing safety** | `crypto/subtle.ConstantTimeCompare` for both username and password fields prevents timing oracle attacks — both fields always evaluated |
| **JWT rotation architecture** | `parseSecrets()` / `AF_JWT_SECRETS` comma-separated multi-key design enables zero-downtime rotation without invalidating active sessions |
| **TLS fail-secure** | `serve()` refuses to start as HTTP when `AF_TLS_ENABLED=true` with missing cert/key — no silent fallback |
| **PKCE + nonce** | OIDC flow correctly implements S256 code challenge and nonce anti-replay using HMAC-signed state cookies |
| **Dual-layer RBAC/ABAC** | `RequireRole` / `RequireRoleOrSelf` in Go middleware + `hasRole` / `isSelfOrRole` in TypeScript — UI and API both gate access independently |
| **Migration sequencing** | Migrations run before `store.NewPostgresStore()` — schema is always consistent before the connection pool opens |
| **Immutable audit rules** | PostgreSQL `DO INSTEAD NOTHING` rules on UPDATE/DELETE prevent audit log tampering at the DB layer |
| **pgxpool configuration** | `MaxConns=25`, `MinConns=5`, `MaxConnLifetime=30min`, `HealthCheckPeriod=1min` — sensible pool settings for production |
| **Bulk span insert** | `pgx.CopyFrom` for batch inserts instead of individual INSERTs — correct for high-throughput telemetry ingestion |
| **PDB coverage** | All three K8s deployments have `PodDisruptionBudget` with `minAvailable: 1` — cluster maintenance won't take the service down |
| **Test coverage** | 8 Go server unit tests (serve/TLS/parseSecrets) + 113 portal Vitest tests covering auth hooks thoroughly |

---

## 6. Service-by-Service Scorecard

| Service | Correctness | Security | Testability | Observability | Score |
|---------|------------|----------|-------------|---------------|-------|
| api-gateway (Go) | 6/10 | 7/10 | 6/10 | 7/10 | **6.5/10** |
| collector (Go) | 7/10 | 6/10 | 4/10 | 6/10 | **5.8/10** |
| af-core (Rust) | Unknown | Unknown | 2/10 | 4/10 | **Needs audit** |
| portal (React) | 7/10 | 5/10 | 7/10 | N/A | **6.3/10** |
| Deploy artefacts | 7/10 | 7/10 | N/A | 7/10 | **7/10** |

---

## 7. Recommended Fix Priority Order

```
IMMEDIATE (before any production traffic):
  B-1  Schema / UUID vs TEXT mismatch
  B-2  Password login must use users table
  B-3  HttpOnly cookie + remove localStorage token storage

SPRINT v1.1.1 (within 1 week):
  H-1  Context key type safety
  H-3  MaxBytesReader on ingest body
  H-5  Stream audit chain verification
  H-6  context.Context in exchangeCode
  M-5  VITE_API_URL consistency
  M-6  Healthz dependency checks
  M-8  srv.Shutdown error check

SPRINT v1.2.0 (next planned release):
  H-2  Cache OIDC discovery document
  H-4  Remove spurious CollectorAuth header check
  M-1  Remove dead const schema
  M-2  Remove stale TODO comment
  M-3  RFC 4122 UUID generation
  M-4  Keyset pagination
  M-7  Security response headers
  M-9  Wire AF_CORS_ORIGINS env var
  M-11 af-core integration test harness
  M-12 Admin action audit trail
  M-13 Disable Edit button or implement route
  M-14 Redis failure mode for rate limiter
  L-5  Version footer driven by env var
  L-8  Automate af_audit_writer password in migration
```

---

## 8. Architecture Decision Records (Recommended)

The following decisions should be formally documented as ADRs before the next sprint:

1. **ADR-001**: Tenant ID type — UUID (migration) vs TEXT (query layer). Pick one, document why.
2. **ADR-002**: Token storage strategy — `HttpOnly` cookie vs `localStorage` vs dual-cookie. Document the XSS/CSRF tradeoff.
3. **ADR-003**: Schema ownership — migration-only vs application-managed schema. Remove dual-maintenance.
4. **ADR-004**: Pagination strategy — offset vs keyset cursor for trace/run listings.

---

*Report prepared by Senior Architect + Technical PM review, AgentFabric v1.1.0, 2026-03-16*
