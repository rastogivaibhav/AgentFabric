# AgentFabric — Deep-Dive Test Report
**Version**: v1.1.0
**Date**: 2026-03-16
**Method**: Full source analysis + live test suite execution across all services
**Verdict**: **NOT PRODUCTION-READY** — 1 build failure, 3 blockers, 14 additional defects

---

## Current-State Update (2026-03-21)

This report reflects the March 16 test and inspection pass. It should not be read as the latest code truth without this addendum.

Closed since this report:
- auth cookies are now `HttpOnly`, and the portal no longer persists JWTs in `localStorage`
- password login now supports database-backed users
- `parseIDTokenUnsafe(...)` is now referenced correctly in auth tests
- portal cost-page drift has been fixed
- the health endpoint now checks PostgreSQL and Redis
- the ingest handler now enforces a request-body size cap
- pricing is now DB-backed and GUI-editable through `pricing_rules`

Still unresolved from a release perspective:
- this repo still needs a clean, fully trusted end-to-end release smoke
- some Go test execution remains environment-sensitive on the current Windows machine
- gateway, collector, proxy, and portal still need stronger release-grade verification as a whole

Treat the detailed findings below as historical baseline findings unless they are explicitly superseded by this update.

## 1. Test Suite Execution Results

### 1.1 Go — api-gateway (`go test ./... -count=1 -v`)

| Package | Tests | Result | Notes |
|---------|-------|--------|-------|
| `cmd/server` | 8 | **PASS** | TLS fail-secure (3), parseSecrets (5) — all green |
| `internal/auth` | 46 | **BUILD FAIL** | Compile error — `parseIDToken` undefined |
| `internal/handlers` | 16 | **PASS** | buildTrace (7), buildTopologyGraph (6), helpers (3) |
| `internal/middleware` | 37 | **PASS** | JWTAuth (7), TenantInjector (3), CollectorAuth (6), RateLimit (11), RequireRole (7), ABAC (4), multi-secret (3) |
| `internal/models` | 0 | skipped | No test file |
| `internal/store` | 0 | skipped | No test file |
| `internal/ws` | 0 | skipped | No test file |

**Overall Go: 61 PASS / 0 FAIL / 1 package BUILD FAILED**

#### Build Failure Detail — `internal/auth` Package

```
internal/auth/oidc_test.go:126:17: undefined: parseIDToken
internal/auth/oidc_test.go:143:12: undefined: parseIDToken
internal/auth/oidc_test.go:162:12: undefined: parseIDToken
internal/auth/oidc_test.go:169:12: undefined: parseIDToken
internal/auth/oidc_test.go:176:12: undefined: parseIDToken
```

**Root cause**: `oidc_test.go` calls `parseIDToken()` but the implementation function was renamed to `parseIDTokenUnsafe()` (the "Unsafe" suffix was added to signal it skips signature verification). The test file was never updated to match. This means **46 auth package tests have never compiled or run** — including all 11 `TestPasswordLogin_*` tests and all 6 `TestRefresh_*` tests.

**Affected tests** (never executed):
```
TestParseIDToken_ValidToken
TestParseIDToken_ExpiredToken
TestParseIDToken_MissingSubject
TestParseIDToken_InvalidFormat_TwoPartToken
TestParseIDToken_InvalidFormat_EmptyString
TestGeneratePKCE_* (5 tests)
TestGenerateNonce_* (2 tests)
TestIssueAFToken_* (4 tests)
TestStateCookieRoundTrip
TestStateCookieVerify_* (3 tests)
TestIsHTTPS_* (2 tests)
TestBearerToken_* (3 tests)
TestNewOIDCHandler_* (2 tests)
TestPasswordLogin_* (11 tests)
TestRefresh_* (8 tests)
```

**Fix required** (1-line): In `oidc_test.go`, rename all calls from `parseIDToken(` to `parseIDTokenUnsafe(`. This is a test file change only — no production code change.

---

### 1.2 Portal — Vitest (`npm run test -- --run`)

| Test File | Tests | Result |
|-----------|-------|--------|
| `src/hooks/api.test.ts` | 13 | PASS |
| `src/hooks/auth.test.ts` | 37 | PASS |
| `src/pages/Dashboard.test.tsx` | 7 | PASS |
| `src/pages/AgentsPage.test.tsx` | 8 | PASS |
| `src/pages/CostPage.test.tsx` | 12 | PASS |
| `src/pages/LiveStream.test.tsx` | 12 | PASS |
| `src/pages/TraceDetail.test.tsx` | 11 | PASS |
| `src/pages/TracesPage.test.tsx` | 13 | PASS |
| **Total** | **113** | **ALL PASS** |

Portal test suite is clean. All 113 tests pass in 3.94 seconds.

---

### 1.3 Summary Table

```
┌─────────────────────────────────────────────────────────────────────┐
│  TEST SUITE SUMMARY — AgentFabric v1.1.0                            │
├────────────────────┬──────────┬──────────┬──────────────────────────┤
│  Suite             │  Total   │  Pass    │  Status                  │
├────────────────────┼──────────┼──────────┼──────────────────────────┤
│  Go: cmd/server    │       8  │       8  │  GREEN                   │
│  Go: internal/auth │      46  │       0  │  BUILD FAIL (rename bug) │
│  Go: internal/     │      53  │      53  │  GREEN                   │
│  Portal: Vitest    │     113  │     113  │  GREEN                   │
├────────────────────┼──────────┼──────────┼──────────────────────────┤
│  TOTAL             │     220  │     174  │  46 NOT COMPILED         │
└────────────────────┴──────────┴──────────┴──────────────────────────┘
```

---

## 2. Coverage Gap Analysis

### 2.1 Packages With Zero Tests

| Package | Risk Level | What's Untested |
|---------|-----------|----------------|
| `internal/store` | **CRITICAL** | All database queries — BulkInsertSpans, ListTraces, ListRuns, CreateUser, GetUserByUsername (doesn't exist), VerifyAuditChain, hashPassword |
| `internal/ws` | Medium | WebSocket hub broadcast, client connect/disconnect, message fan-out |
| `internal/models` | Low | Pure data structs — low value to test directly |
| `collector/` (all) | **HIGH** | OTLP receiver, framework detection, PII scrubbing, rate limiting, exporter |

### 2.2 Critical Uncovered Paths in Existing Packages

| Path | Why It Matters |
|------|---------------|
| `oidc.go:PasswordLogin` — DB user path | **Does not exist yet** (B-2 fix not done) |
| `store.go:BulkInsertSpans` | Core data write — untested against any DB |
| `store.go:VerifyAuditChain` | Hash recomputation — divergence from af-core is silent |
| `store.go:hashPassword` | bcrypt fallback to SHA-256 on error — should be tested |
| `handlers.go:Ingest` — oversized body | **No MaxBytesReader** — DoS vector, untested |
| `handlers.go:Health` | Always returns 200 — dependency failures are invisible |
| `middleware.go:RequireRole` — case sensitivity | Test passes, but `EqualFold` means "Admin" == "admin" — may be unexpected behaviour in the portal where hasRole uses exact match |

---

## 3. Code-Level Defects Found During Deep Inspection

### 3.1 CONFIRMED BUG — `parseIDToken` Rename Not Propagated

**Severity**: High (test failure + reveals rename was made without search-and-replace)
**File**: `api-gateway/internal/auth/oidc_test.go` lines 126, 143, 162, 169, 176

The function `parseIDTokenUnsafe` exists in `oidc.go`. The test file references the old name `parseIDToken`. This means the 5 token-parsing tests — and by extension ALL 46 auth tests — have never run. Production auth behaviour has **zero test coverage**.

---

### 3.2 CONFIRMED BUG — RBAC Case-Sensitivity Mismatch Between Layers

**Severity**: Medium
**Files**: `middleware/middleware.go` (Go server) vs `portal/src/hooks/auth.ts` (TypeScript)

The Go `RequireRole` uses `strings.EqualFold` (case-insensitive):
```go
if strings.EqualFold(claims.Role, allowed) { // "Admin" == "admin" → PASS
```

The TypeScript `hasRole` uses `Array.includes` (exact match):
```typescript
return allowedRoles.includes(user.role) // "Admin" != "admin" → FAIL
```

**Consequence**: A JWT containing `"role": "Admin"` (mixed-case, e.g. from some OIDC providers) would pass the Go API check but the portal would hide admin elements from that user. The user appears to be a non-admin in the UI but an admin to the API. This is an inconsistency that could confuse operators and mask privilege issues.

---

### 3.3 CONFIRMED BUG — `oidc_test.go` `NewOIDCHandler` Signature Now Incorrect

**Severity**: Medium (will block B-2 fix landing)
**File**: `api-gateway/internal/auth/oidc_test.go` lines 23–32

All `testHandler()` and `passwordLoginHandler()` helpers call:
```go
NewOIDCHandler(OIDCConfig{...}, zap.NewNop())
```

After Fix B-2 (`NewOIDCHandler` gains a `UserLookup` parameter), every existing test helper call will fail to compile. All test helpers need a nil or mock `UserLookup` added as the second argument before the logger. This must be planned into the B-2 fix ticket so tests don't regress.

---

### 3.4 CONFIRMED DEFECT — `store.go` Comment Contradicts Reality

**Severity**: Low
**File**: `api-gateway/internal/store/postgres.go` lines 44–47

```go
// Schema is initialized by deploy/sql/init.sql in docker-compose.yml
// No migration needed here
```

This comment is false in two ways:
1. Migrations ARE now done by golang-migrate (`runMigrations()` in main.go)
2. `deploy/sql/init.sql` is separate from `deploy/migrations/001_initial_schema.up.sql` — running both would attempt to create tables twice

The comment must be updated: "Schema is managed by deploy/migrations/ via golang-migrate. See main.go:runMigrations()."

---

### 3.5 CONFIRMED DEFECT — `ingestRequest` Accepts Unauthenticated Tenant Override

**Severity**: Medium
**File**: `api-gateway/internal/handlers/handlers.go` lines 76–81

```go
tenantID := r.Header.Get("X-AF-Tenant")
if tenantID == "" {
    tenantID = "default"
}
```

The `X-AF-Tenant` header is read from the ingest request **with no validation**. Any client that can send to `/internal/ingest` can write spans under any tenant ID. With `authDisabled=true` (dev mode) there is zero protection. With auth enabled, the `CollectorAuth` middleware validates the JWT but does NOT validate that the JWT's `tenant_id` matches the `X-AF-Tenant` header. A compromised collector could poison another tenant's span data.

**Fix**: Extract tenant from the JWT claims inside `CollectorAuth`, not from a header.

---

### 3.6 CONFIRMED DEFECT — Rate Limit Fail-Open Is Undocumented in Production Checklist

**Severity**: Low
**File**: `api-gateway/internal/middleware/ratelimit.go` line 56–59

```go
if err != nil {
    // Redis failure: fail open (do not block legitimate traffic)
    next.ServeHTTP(w, r)
    return
}
```

This is a deliberate design choice (fail-open = no outage during Redis downtime) but it is not documented in the production checklist or runbook. An operator debugging a Redis outage would not know that rate limiting is silently disabled during the outage window. Add a `logger.Warn("rate limiter: Redis unavailable, running without rate limit")` log line.

---

### 3.7 CONFIRMED DEFECT — `GetTraceSpans` Silently Skips Scan Errors

**Severity**: Medium
**File**: `api-gateway/internal/store/postgres.go` lines 298–304

```go
for rows.Next() {
    var sp models.Span
    ...
    if err := rows.Scan(...); err != nil {
        continue  // ← silently drops the row
    }
```

If a row fails to scan (type mismatch, null in non-nullable column), it is silently dropped. The caller receives a partial result with no error. This pattern appears in `GetSpansForTraces`, `ListRuns`, `ListAgents`, and `GetRunChildren` as well (6 occurrences). All should either return an error or log the skip at WARN level so operators know data is being lost.

---

### 3.8 CONFIRMED DEFECT — `srv.Shutdown()` Error Silently Discarded

**Severity**: Low
**File**: `api-gateway/cmd/server/main.go` line 225

```go
srv.Shutdown(ctx)  // return value discarded
```

If graceful shutdown exceeds 15 seconds (configured timeout), active connections are forcibly closed and the error is lost. Should be:
```go
if err := srv.Shutdown(ctx); err != nil {
    logger.Warn("graceful shutdown incomplete", zap.Error(err))
}
```

---

### 3.9 CONFIRMED DEFECT — `VerifyAuditChain` Skips Entries With Empty Hash

**Severity**: Medium
**File**: `api-gateway/internal/store/postgres.go` lines 782–793

```go
if r.entryHash != "" && expected != r.entryHash {
    // chain broken
}
prevHash = r.entryHash  // ← if entryHash is "", prevHash becomes ""
```

If any entry has an empty `entry_hash` (e.g., an audit entry written by a buggy version of af-core), the chain verification silently accepts it AND resets `prevHash` to `""`. The next entry would be hashed against an empty string. A run of entries with empty hashes would pass verification entirely.

The guard should be:
```go
if r.entryHash == "" {
    // treat missing hash as broken chain
}
```

---

## 4. Architecture Deep-Dive — Flow Tracing

### 4.1 Happy-Path Span Ingestion (End-to-End Trace)

```
Agent SDK → OTLP HTTP POST /v1/traces → Collector (:4318)
  collector/internal/receiver → parse OTLP protobuf
  → collector/internal/processor → framework detection, PII scrub, cost computation
  → collector/internal/exporter → POST /internal/ingest to api-gateway:8080
      → Header: X-AF-Source: collector
      → Header: Authorization: Bearer <AF_GATEWAY_AUTH_TOKEN>
      → Body: {"spans":[...]}

api-gateway: POST /internal/ingest
  → CollectorAuth middleware
      → Check X-AF-Source == "collector"     [H-4: this check adds nothing]
      → Constant-time compare against AF_GATEWAY_AUTH_TOKEN
  → Handler.Ingest()
      → No MaxBytesReader                    [D: DoS vector]
      → Read X-AF-Tenant header (unauthenticated) [3.5: tenant injection risk]
      → BulkInsertSpans() → pgx.CopyFrom to spans table  [3.7: scan errors swallowed]
      → WebSocket broadcast to connected clients

RESULT: Span appears in portal within ~100ms of agent SDK call
RISK: No body size limit, no tenant validation, scan errors drop silently
```

### 4.2 Login Flow (Password Path)

```
Portal: POST /auth/login {"username":"admin","password":"admin"}

api-gateway: OIDCHandler.PasswordLogin()
  → Parse JSON body
  → subtle.ConstantTimeCompare against AF_ADMIN_USER/AF_ADMIN_PASSWORD
  → IF match: issue JWT, return {"token":"eyJ..."}
  → Portal: localStorage.setItem('af_token', token)   [B-3: XSS vulnerability]

CRITICAL GAP: users table is never consulted
  → User created via POST /api/v1/users cannot log in
  → bcrypt-hashed passwords are stored but never compared
  → The entire user management feature has no authentication path
```

### 4.3 RBAC Enforcement Flow

```
Portal: DELETE /api/v1/users/uuid-123
  → Authorization: Bearer eyJ...

api-gateway:
  → JWTAuth middleware → verify JWT, extract claims → context["claims"]
  → TenantInjector → set context["tenant_id"]
  → RateLimit → check Redis counter
  → RequireRole("admin") middleware
      → Extract claims from context["claims"]    [H-1: string key, collision risk]
      → EqualFold(claims.Role, "admin")          [3.2: case mismatch with portal]
      → 200 → Handler.DeleteUser()
         → DELETE FROM users WHERE user_id=$1 AND tenant_id=$2
         → No audit trail written               [M-12: compliance gap]

Portal (parallel):
  → hasRole(user, ['admin']) — exact match, NOT EqualFold
  → Hides Delete button for non-admins
```

### 4.4 JWT Rotation Flow

```
Current state (correctly implemented):
  AF_JWT_SECRETS="new-key,old-key"
  → parseSecrets() → ["new-key", "old-key"]
  → JWTAuth(secrets...) tries each in order
  → First match wins → valid
  → New tokens signed with secrets[0] (new-key)
  → Old tokens accepted via secrets[1] (old-key) during rotation window

Session max age: 8 hours (AF_SESSION_MAX_AGE)
  → After 8h, all old-key tokens have expired
  → Remove old-key from AF_JWT_SECRETS

CORRECTLY IMPLEMENTED — no issues found in this path
```

---

## 5. Security Posture Assessment

### 5.1 Security Controls Inventory

| Control | Implementation | Status |
|---------|---------------|--------|
| Password auth timing safety | `crypto/subtle.ConstantTimeCompare` | **PASS** |
| JWT signature validation | `golang-jwt/jwt/v5` with `ParseWithClaims` | **PASS** |
| JWT algorithm pinning | HMAC checked: `if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok` | **PASS** |
| PKCE + nonce (OIDC) | S256 challenge + anti-replay nonce in signed cookie | **PASS** |
| RBAC server enforcement | `RequireRole` middleware | **PASS** |
| ABAC self-service | `RequireRoleOrSelf` middleware | **PASS** |
| TLS fail-secure | `serve()` returns error if TLS=true with no cert | **PASS** |
| JWT key rotation | Multi-secret, comma-separated, zero-downtime | **PASS** |
| Multi-tenancy isolation | `WHERE tenant_id = $1` on all queries | **PASS (manual)** |
| Audit log tamper prevention | PostgreSQL RULE DO INSTEAD NOTHING | **PASS** |
| Session cookie security | HttpOnly: **false** (OIDC) / localStorage (password) | **FAIL** |
| DB user auth path | Password compared against users table | **FAIL — not implemented** |
| Request body size limit | MaxBytesReader on ingest | **FAIL — missing** |
| Security response headers | CSP, X-Frame-Options, etc. | **FAIL — missing** |
| Tenant injection attack | X-AF-Tenant validated against JWT claims | **FAIL — unvalidated header** |
| Admin action audit trail | User CRUD written to policy_audit_log | **FAIL — missing** |

**Security score: 10/16 controls passing**

---

## 6. Performance Risk Assessment

| Path | Current Behaviour | Risk |
|------|------------------|------|
| `GET /api/v1/traces?limit=50&offset=50000` | Full table scan over 50k rows | HIGH — P99 spike at scale |
| `GET /api/v1/audit/verify` | Load 100k rows into memory | HIGH — OOM on busy tenant |
| `POST /internal/ingest` with 1 GB body | Read fully into memory | HIGH — DoS vector |
| OIDC login | 2 x HTTP calls to `/.well-known/openid-configuration` | MEDIUM — 100–200ms latency |
| Redis unavailable | Rate limiter silent fail-open | MEDIUM — unlimited requests |
| 5000 spans per trace (hardcoded) | `GetTraceSpans` LIMIT 5000 | LOW — edge case |

---

## 7. Test Gaps vs Risk Matrix

```
RISK                    TEST COVERAGE     GAP SIZE
────────────────────────────────────────────────────────
Auth (oidc.go)         BUILD FAILS        ALL 46 tests never ran
Store (postgres.go)    ZERO               Every DB query untested
Collector              ZERO               PII scrub, framework detection untested
Ingest DoS             ZERO               No MaxBytesReader test
Password→DB login      N/A (not built)    The feature doesn't exist yet
Healthz deps           ZERO               Health always passes
Security headers        ZERO               Not implemented
Tenant injection        ZERO               No test for X-AF-Tenant bypass
────────────────────────────────────────────────────────
Portal auth hooks      37 tests            GOOD
Middleware (Go)        37 tests            GOOD
Handlers (Go)          16 tests            PARTIAL (no HTTP-level tests)
Server TLS             8 tests             GOOD
```

---

## 8. Final Verdict

### What Is Working (and Tested)

| Area | Confidence |
|------|-----------|
| TLS fail-secure logic | HIGH — 3 tests, well-named, cover edge cases |
| JWT multi-secret rotation (middleware) | HIGH — 3 rotation tests, explicit |
| RBAC/ABAC middleware | HIGH — 12 tests covering all code paths |
| Rate limiting (Redis, isolation, fail-open) | HIGH — 11 tests |
| Portal RBAC/ABAC hooks | HIGH — 37 tests, thorough edge cases |
| PKCE + state cookie | HIGH — once build failure is fixed, 7 dedicated tests exist |
| Password login (env-var path) | HIGH — once build failure is fixed, 11 tests |
| Token refresh | HIGH — once build failure is fixed, 8 tests |

### What Is Broken or Untested

| Area | Severity | Confidence |
|------|----------|-----------|
| `internal/auth` package | BUILD FAILURE | Zero confidence — nothing compiles |
| Password login → users table | NOT BUILT | N/A |
| All store queries | ZERO TESTS | Unknown |
| Ingest DoS protection | NOT BUILT | N/A |
| Security response headers | NOT BUILT | N/A |
| Admin audit trail | NOT BUILT | N/A |
| Tenant injection attack | NOT TESTED | Unknown |
| RBAC case-sensitivity consistency | DEFECT PRESENT | Confirmed |

### Readiness Verdict

```
┌─────────────────────────────────────────────────────────────────────┐
│  AGENTFABRIC v1.1.0 — PRODUCTION READINESS VERDICT                 │
│                                                                     │
│  OVERALL:  NOT READY                                                │
│                                                                     │
│  Tests:    174/220 compiled and passing                             │
│            46 tests in auth package have NEVER run (build failure) │
│                                                                     │
│  Blockers: 3 open (schema, DB login, HttpOnly cookie)               │
│  Build failures: 1 (parseIDToken rename not propagated)            │
│  Additional defects: 9 confirmed                                    │
│                                                                     │
│  Target: v1.1.1 with all items in FIX_PLAN resolved                │
│  Estimated effort: 5 engineering-days                               │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 9. Immediate Action Items (Ordered by Effort × Impact)

| # | Action | Effort | Impact | Owner |
|---|--------|--------|--------|-------|
| 1 | Fix `parseIDToken` → `parseIDTokenUnsafe` in `oidc_test.go` | 5 min | Unblocks 46 tests | Any |
| 2 | `MaxBytesReader` on ingest (Fix D) | 30 min | Closes DoS vector | Backend |
| 3 | Fix `srv.Shutdown()` error check | 5 min | Ops visibility | Any |
| 4 | Fix stale `const schema` comment | 5 min | Prevents confusion | Any |
| 5 | Fix RBAC case-sensitivity mismatch (EqualFold vs includes) | 2h | Consistency | Backend + Frontend |
| 6 | Unify schema + tenant UUID (Fix A) | 1 day | BLOCKER | Backend |
| 7 | Wire password login to DB (Fix B) | 1 day | BLOCKER | Backend |
| 8 | HttpOnly cookie (Fix C) | 3h | BLOCKER | Backend + Frontend |
| 9 | Healthz dependency checks (Fix E) | 2h | Ops readiness | Backend |
| 10 | Add store unit tests | 2 days | Test coverage | Backend |

---

*Deep-dive test report prepared 2026-03-16 — AgentFabric v1.1.0*
*Full fix plan: docs/FIX_PLAN_v1.1.1_v1.2.0.md*
*Architecture review: docs/TECHNICAL_REVIEW_v1.1.0.md*
