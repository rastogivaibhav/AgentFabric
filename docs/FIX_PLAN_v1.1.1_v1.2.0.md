# Govagn — Detailed Fix Plan: v1.1.1 + v1.2.0
**Prepared**: 2026-03-16
**Covers**: Every blocker and hardening item from the v1.1.0 technical review
**Format**: Each fix has exact file, exact line(s) affected, root cause, and the precise change required

---

## How to Use This Plan

Work top-to-bottom within each version. Each item is self-contained — a single engineer can pick one ticket, read the "Change required" block, make the edit, add the listed tests, and submit a PR. Items marked **SEQUENTIAL** must wait for the item above to land first (they depend on it). All others are parallelisable.

---

# Version v1.1.1 — Correctness Patch

**Goal**: Fix the three production blockers + four high-risk correctness issues.
**Target**: 1 sprint (~5 days), ~2 engineers in parallel.

---

## Fix 1.1.1-A — Schema Unification (BLOCKER / SEQUENTIAL: must land first)

**Priority**: P0 — everything else depends on a single, consistent schema.

### Root Cause

Two schemas exist simultaneously:

| Location | Tenant ID type | Table names | Status |
|----------|---------------|-------------|--------|
| `api-gateway/internal/store/postgres.go` lines 50–160 | `TEXT DEFAULT 'default'` | `spans`, `runs`, `policy_audit_log`, `feedback`, `environments`, `users` | **Dead code** — never executed |
| `deploy/migrations/001_initial_schema.up.sql` | `UUID REFERENCES tenants(tenant_id)` | `tenants`, `agent_runs`, `span_metadata`, `policy_audit_log`, `agent_topology`, `run_feedback`, `model_pricing`, `api_keys`, `users`, `environments` | **Canonical** — what runs in production |

Every query in `postgres.go` references tables from the dead schema (`spans`, `runs`) that **do not exist** in the migration. The migration creates `span_metadata` and `agent_runs` with `UUID` tenant references.

### Files to Change

**File 1**: `api-gateway/internal/store/postgres.go`

**Action A — Remove dead schema constant** (lines 50–160):
Delete the entire `const schema = \`` block. It was the original "init on startup" approach, fully superseded by golang-migrate.

**Action B — Update all table names to match migration**:

| Current query uses | Migration table | Fix |
|-------------------|----------------|-----|
| `spans` | `span_metadata` | Rename in all queries |
| `runs` | `agent_runs` | Rename in all queries |
| `span_id` (TEXT PK) | `span_id VARCHAR(16)` | Column name stays, type compatible |
| `tenant_id = $1` (TEXT 'default') | `tenant_id UUID` | Change default value |

**Action C — Fix tenant ID default**:

Change every occurrence of the string literal `"default"` used as a tenant ID to the UUID:
```go
const defaultTenantID = "00000000-0000-0000-0000-000000000001"
```
Update `tenantFromCtx()` in `handlers/handlers.go` and `TenantInjector` in `middleware/middleware.go` to return this constant instead of `"default"`.

**Action D — Fix `BulkInsertSpans` column mapping**:

The `spans` table columns map to `span_metadata` with these name changes:
```
span_id          → span_id          (same)
trace_id         → trace_id         (same)
parent_span_id   → parent_span_id   (same)
run_id           → run_id           (TEXT → UUID: parse or make nullable)
name             → name             (same)
framework        → framework        (same)
start_time_ns    → start_ns         (renamed)
duration_ns      → (remove: span_metadata stores end_ns, duration_ms is computed)
status_code      → status_code      (same)
status_msg       → status_message   (renamed)
attributes       → (remove: not in migration schema — store in JSONB metadata or separate table)
events           → (remove: not in migration schema)
input_tokens     → input_tokens     (same)
output_tokens    → output_tokens    (same)
cost_usd         → cost_total_usd   (renamed)
tenant_id        → tenant_id        (TEXT → UUID)
```

**File 2**: `api-gateway/internal/middleware/middleware.go`

```go
// Before
ctx := context.WithValue(r.Context(), "tenant_id", "default")

// After — use the canonical seed tenant UUID
const defaultTenantUUID = "00000000-0000-0000-0000-000000000001"
ctx := context.WithValue(r.Context(), "tenant_id", defaultTenantUUID)
```

**File 3**: `api-gateway/internal/handlers/handlers.go`

```go
// Before
func tenantFromCtx(r *http.Request) string {
    if t, ok := r.Context().Value("tenant_id").(string); ok && t != "" {
        return t
    }
    return "default"
}

// After
func tenantFromCtx(r *http.Request) string {
    if t, ok := r.Context().Value("tenant_id").(string); ok && t != "" {
        return t
    }
    return "00000000-0000-0000-0000-000000000001"
}
```

**Tests to add**: `store/postgres_test.go` — table-driven integration test confirming `BulkInsertSpans` writes to `span_metadata` with a real test-container PostgreSQL instance. At minimum: one span round-trip (insert → list traces → see span).

---

## Fix 1.1.1-B — Wire Password Login to Users Table (BLOCKER / SEQUENTIAL: after A)

**Priority**: P0

### Root Cause

`oidc.go` `PasswordLogin()` (lines 826–880) only checks `h.cfg.AdminUser` / `h.cfg.AdminPassword` env vars. The `users` table with bcrypt-hashed passwords is never queried. Created users cannot log in.

### Files to Change

**File 1**: `api-gateway/internal/auth/oidc.go`

The `OIDCHandler` needs a reference to the user store. Add a store interface:

```go
// UserLookup is the minimal store interface PasswordLogin needs.
// Defined here (not in store package) to avoid circular imports.
type UserLookup interface {
    GetUserByUsername(ctx context.Context, username, tenantID string) (*UserRecord, error)
}

// UserRecord carries only the fields PasswordLogin needs.
type UserRecord struct {
    UserID       string
    PasswordHash string
    Role         string
    Email        string
    DisplayName  string
}
```

Add `store UserLookup` field to `OIDCHandler`. Update `NewOIDCHandler` to accept it:
```go
func NewOIDCHandler(cfg OIDCConfig, store UserLookup, logger *zap.Logger) *OIDCHandler
```

**Updated `PasswordLogin` logic**:
```
1. Decode JSON body (same as now)
2. Validate non-empty (same as now)
3. Try database lookup first:
   a. Call store.GetUserByUsername(ctx, req.Username, defaultTenantUUID)
   b. If found: bcrypt.CompareHashAndPassword([]byte(record.PasswordHash), []byte(req.Password))
   c. On match: build idClaims from record (use record.Role, not hardcoded "admin")
   d. On no match: fall through to env-var admin check (break-glass)
4. Env-var admin fallback (existing constant-time logic):
   a. Only if DB lookup failed (user not found) OR bcrypt mismatch after DB lookup
   b. Compare against wantUser/wantPass with subtle.ConstantTimeCompare
   c. On match: build idClaims with role "admin"
5. If both fail: return 401 (same generic message, no leaking which path failed)
```

**File 2**: `api-gateway/internal/store/postgres.go`

Add `GetUserByUsername` method:
```go
func (s *PostgresStore) GetUserByUsername(ctx context.Context, username, tenantID string) (*auth.UserRecord, error) {
    var r auth.UserRecord
    err := s.pool.QueryRow(ctx, `
        SELECT user_id, password_hash, role, email, display_name
        FROM users
        WHERE username = $1 AND tenant_id = $2 AND is_active = true
    `, username, tenantID).Scan(
        &r.UserID, &r.PasswordHash, &r.Role, &r.Email, &r.DisplayName,
    )
    if err != nil {
        return nil, err
    }
    return &r, nil
}
```

Note: `auth.UserRecord` is defined in the auth package (as above) to avoid a circular import. The store implements the `UserLookup` interface implicitly.

**File 3**: `api-gateway/cmd/server/main.go`

Update `NewOIDCHandler` call to pass `pgStore`:
```go
oidcHandler := auth.NewOIDCHandler(auth.OIDCConfig{...}, pgStore, logger)
```

**Tests to add**:
- `oidc_test.go`: `TestPasswordLogin_DatabaseUser_Succeeds` — mock `UserLookup` returning a bcrypt hash of "correctpass", verify 200 + token
- `TestPasswordLogin_DatabaseUser_WrongPassword_Returns401`
- `TestPasswordLogin_UserNotFound_FallsBackToEnvAdmin`
- `TestPasswordLogin_UserInactive_Returns401`

---

## Fix 1.1.1-C — HttpOnly Cookie + Remove localStorage Token (BLOCKER)

**Priority**: P0

### Root Cause

- `oidc.go` line 335: `HttpOnly: false` — cookie readable by JavaScript → XSS-stealable
- `LoginPage.tsx` line 55: `localStorage.setItem('af_token', token)` — XSS-stealable

The portal does NOT actually need to read the raw JWT. It uses `GET /auth/me` to populate user context. The `getToken()` function reads cookies already (and correctly prefers them over localStorage).

### Files to Change

**File 1**: `api-gateway/internal/auth/oidc.go`

**OIDC callback cookie** (line 331–339): change `HttpOnly` from `false` to `true`:
```go
http.SetCookie(w, &http.Cookie{
    Name:     "af_token",
    Value:    afJWT,
    Path:     "/",
    HttpOnly: true,    // WAS: false
    Secure:   isHTTPS(r),
    SameSite: http.SameSiteLaxMode,
    MaxAge:   int(h.cfg.SessionMaxAge.Seconds()),
})
```

**`PasswordLogin` response** (line 877): Instead of returning the token in the JSON body, set it as a cookie AND return the body (for backwards compat during transition):
```go
// Set HttpOnly cookie
http.SetCookie(w, &http.Cookie{
    Name:     "af_token",
    Value:    token,
    Path:     "/",
    HttpOnly: true,
    Secure:   isHTTPS(r),
    SameSite: http.SameSiteLaxMode,
    MaxAge:   int(h.cfg.SessionMaxAge.Seconds()),
})
// Still return JSON body so portals on older versions keep working during rollout
writeJSON(w, http.StatusOK, map[string]string{"token": token})
```

**File 2**: `portal/src/pages/LoginPage.tsx`

Remove localStorage storage. After a successful login the cookie is set by the server — no client-side storage needed:
```typescript
// Remove line 55:
// localStorage.setItem('af_token', token)

// Replace the success block with just a navigation:
navigate('/dashboard', { replace: true })
```

**File 3**: `portal/src/hooks/auth.ts` — `getToken()`

The function already prefers cookies over localStorage. After this fix, the localStorage branch becomes dead code. Add a comment noting it is kept only for backwards compatibility with pre-1.1.1 tokens already stored:
```typescript
// localStorage fallback: kept for pre-v1.1.1 tokens stored before HttpOnly migration.
// New tokens are always set as HttpOnly cookies by the server.
```

**Tests to add**:
- `oidc_test.go`: `TestPasswordLogin_SetsHttpOnlyCookie` — verify the response header contains `Set-Cookie: af_token=...; HttpOnly`
- `oidc_test.go`: `TestOIDCCallback_SetsHttpOnlyCookie`

---

## Fix 1.1.1-D — MaxBytesReader on Ingest Endpoint

**Priority**: P1

### Root Cause

`handlers.go` `Ingest()` line 60: `json.NewDecoder(r.Body).Decode(&req)` — no body size cap before reading. A 1 GB payload is read into memory before the 10,000-span check triggers.

### File to Change

**`api-gateway/internal/handlers/handlers.go`** — `Ingest()` function, first line of the body:

```go
func (h *Handler) Ingest(w http.ResponseWriter, r *http.Request) {
    // Cap body at 32 MB — a 10,000-span batch at ~2KB avg is ~20 MB.
    // Anything beyond 32 MB is definitionally malformed or a DoS attempt.
    r.Body = http.MaxBytesReader(w, r.Body, 32<<20)

    var req ingestRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        // http.MaxBytesReader sets the status to 413 before returning the error
        if err.Error() == "http: request body too large" {
            writeError(w, http.StatusRequestEntityTooLarge, "request body too large (max 32 MB)")
            return
        }
        writeError(w, http.StatusBadRequest, "invalid body")
        return
    }
    // ... rest unchanged
```

**Tests to add**:
- `handlers_test.go`: `TestIngest_OversizedBody_Returns413` — send a body larger than 32 MB, verify 413

---

## Fix 1.1.1-E — Healthz Dependency Checks

**Priority**: P1

### Root Cause

`handlers.go` `Health()` always returns 200 regardless of whether Postgres or Redis are reachable. Kubernetes readiness probes pointing at `/healthz` will report pods as ready when they are not.

### Files to Change

**File 1**: `api-gateway/internal/store/postgres.go`

Add a `Ping` method:
```go
func (s *PostgresStore) Ping(ctx context.Context) error {
    return s.pool.Ping(ctx)
}
```

**File 2**: `api-gateway/internal/store/redis.go`

Add a `Ping` method (if not already present — verify first):
```go
func (c *RedisClient) Ping(ctx context.Context) error {
    return c.client.Ping(ctx).Err()
}
```

**File 3**: `api-gateway/internal/handlers/handlers.go`

Update `Health()`:
```go
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
    defer cancel()

    type depStatus struct {
        Status string `json:"status"`
        Error  string `json:"error,omitempty"`
    }
    type healthResponse struct {
        Status   string               `json:"status"`
        Postgres depStatus            `json:"postgres"`
        Redis    depStatus            `json:"redis"`
    }

    resp := healthResponse{Status: "ok"}

    if err := h.pg.Ping(ctx); err != nil {
        resp.Status = "degraded"
        resp.Postgres = depStatus{Status: "unhealthy", Error: err.Error()}
    } else {
        resp.Postgres = depStatus{Status: "ok"}
    }

    if err := h.redis.Ping(ctx); err != nil {
        resp.Status = "degraded"
        resp.Redis = depStatus{Status: "unhealthy", Error: err.Error()}
    } else {
        resp.Redis = depStatus{Status: "ok"}
    }

    code := http.StatusOK
    if resp.Status != "ok" {
        code = http.StatusServiceUnavailable
    }
    writeJSON(w, code, resp)
}
```

**Tests to add**:
- `handlers_test.go`: `TestHealth_AllHealthy_Returns200`
- `TestHealth_PostgresDown_Returns503`
- `TestHealth_RedisDown_Returns503`

---

## Fix 1.1.1-F — Context Key Type Safety

**Priority**: P1 (low effort, prevents future debugging nightmares)

### Root Cause

String literals `"claims"` and `"tenant_id"` as context keys collide with any other package using the same strings.

### File to Change

**`api-gateway/internal/middleware/middleware.go`**

Add at the top of the file (after imports):
```go
// contextKey is an unexported type for context keys in this package.
// Using a typed key prevents collisions with keys from other packages.
type contextKey int

const (
    claimsKey   contextKey = iota // stores *Claims
    tenantIDKey                   // stores string (tenant UUID)
)
```

Replace all `context.WithValue(r.Context(), "claims", ...)` with `context.WithValue(r.Context(), claimsKey, ...)`.
Replace all `r.Context().Value("claims").(*Claims)` with `r.Context().Value(claimsKey).(*Claims)`.
Replace all `"tenant_id"` with `tenantIDKey`.

Export getters for use in handlers:
```go
// ClaimsFromCtx extracts JWT claims from the request context.
func ClaimsFromCtx(r *http.Request) *Claims {
    c, _ := r.Context().Value(claimsKey).(*Claims)
    return c
}

// TenantIDFromCtx extracts the tenant UUID from the request context.
func TenantIDFromCtx(r *http.Request) string {
    t, _ := r.Context().Value(tenantIDKey).(string)
    if t == "" {
        return "00000000-0000-0000-0000-000000000001"
    }
    return t
}
```

Update `handlers.go` `tenantFromCtx()` to call `middleware.TenantIDFromCtx(r)`.
Update `rbac_test.go` and `middleware_test.go` to use the new typed key for injecting test context values.

---

## v1.1.1 Delivery Checklist

Before tagging v1.1.1:

```
[ ] Fix 1.1.1-A merged: schema unified, const schema removed, table names match migration
[ ] Fix 1.1.1-B merged: password login queries users table, bcrypt compare, env-var fallback
[ ] Fix 1.1.1-C merged: HttpOnly cookie on both login paths, localStorage removed from portal
[ ] Fix 1.1.1-D merged: MaxBytesReader on ingest, 413 test passing
[ ] Fix 1.1.1-E merged: healthz checks Postgres + Redis, returns 503 when degraded
[ ] Fix 1.1.1-F merged: typed context keys, exported getters
[ ] make test passes (all 121+ tests green)
[ ] make lint passes (zero warnings)
[ ] make migrate/status shows version 1 on a fresh db
[ ] Manual smoke test: create user → login as that user → see bearer in cookie (HttpOnly) → no localStorage token
[ ] PRODUCTION_CHECKLIST.md §1.2 checkbox verified: GV_AUTH_DISABLED absent or false
```

---

---

# Version v1.2.0 — Hardening + UX

**Goal**: Production-grade hardening, cursor pagination, admin audit trail, CORS wiring, security headers, and af-core integration tests.
**Target**: 1 sprint (4 weeks), ~3 engineers in parallel across workstreams.

Workstream split:
- **WS-A** (Backend): Cursor pagination, admin audit trail, CORS wiring, context.Context in exchangeCode
- **WS-B** (Security): Security headers, dead code removal, RFC UUID, af-core tests
- **WS-C** (Portal): Edit-user route, version footer, env var consistency fix

---

## Fix 1.2.0-A1 — Cursor-Based (Keyset) Pagination

**Priority**: P2 — offset pagination scans the full table for large offsets

### Root Cause

All list endpoints use `LIMIT $n OFFSET $m`. A request for offset 50,000 causes PostgreSQL to read and discard 50,000 rows. For a high-throughput deployment this will cause P99 latency spikes.

### Files to Change

**File 1**: `api-gateway/internal/models/models.go`

Replace `Page[T]`:
```go
type Page[T any] struct {
    Items      []T    `json:"items"`
    HasMore    bool   `json:"has_more"`
    NextCursor string `json:"next_cursor,omitempty"` // base64-encoded keyset position
}
```

**File 2**: `api-gateway/internal/store/postgres.go` — `ListTraces`

Change pagination from OFFSET to keyset on `(start_time_ns, trace_id)`:
```go
// Accept cursor parameter in TraceQuery
type TraceQuery struct {
    // ... existing fields
    Cursor string // base64( startTimeNs:traceId ) of the last seen item
}

// In ListTraces: decode cursor, add WHERE clause
// WHERE (start_time_ns, trace_id) < ($cursor_time, $cursor_id)
// ORDER BY start_time_ns DESC, trace_id DESC
// LIMIT $n
// Return: encode the last item's (start_time_ns, trace_id) as the next cursor
```

Apply the same pattern to `ListRuns` and `ListAgents`.

**File 3**: `api-gateway/internal/handlers/handlers.go`

Accept `?cursor=<token>` query param and pass it through to `TraceQuery`. Return `next_cursor` in the response.

**Tests to add**:
- `postgres_test.go`: `TestListTraces_CursorPagination` — insert 25 traces, page with limit=10, verify three pages, no duplicates, no gaps

---

## Fix 1.2.0-A2 — Admin Action Audit Trail

**Priority**: P2 — SOC 2 CC6.3 requires logging privileged operations

### Root Cause

User create/update/delete operations are RBAC-gated but not written to `policy_audit_log`. An admin could delete a user with no forensic record.

### Files to Change

**File 1**: `api-gateway/internal/handlers/handlers.go` — user mutation handlers

After each successful mutation, write an audit event. Create a helper:
```go
func (h *Handler) writeAdminAudit(ctx context.Context, tenantID, action, subjectID, actorID string) {
    entry := store.AuditEntry{
        DecisionID: generateID(),
        TraceID:    "admin-action",
        SpanID:     subjectID,
        PolicyName: "admin_action",
        Result:     action,   // "user_created" | "user_deleted" | "user_updated" | "role_changed"
        Reason:     fmt.Sprintf("actor=%s subject=%s action=%s", actorID, subjectID, action),
        TenantID:   tenantID,
    }
    if err := h.pg.InsertAuditEntry(ctx, entry); err != nil {
        h.logger.Warn("audit write failed", zap.Error(err))
        // Non-fatal: log and continue — don't fail the request over audit write
    }
}
```

Call `h.writeAdminAudit(...)` in `CreateUser`, `UpdateUser`, `DeleteUser` handlers immediately after the store operation succeeds.

**File 2**: `api-gateway/internal/store/postgres.go`

Add `InsertAuditEntry` method:
```go
func (s *PostgresStore) InsertAuditEntry(ctx context.Context, e AuditEntry) error {
    _, err := s.pool.Exec(ctx, `
        INSERT INTO policy_audit_log
            (decision_id, tenant_id, trace_id, span_id, policy_name, result, reason,
             previous_hash, entry_hash, evaluated_ns)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
    `, e.DecisionID, e.TenantID, e.TraceID, e.SpanID,
        e.PolicyName, e.Result, e.Reason,
        "genesis", computeHash(e), time.Now().UnixNano())
    return err
}
```

Note: hash-chaining of admin audit entries requires reading the last entry hash first (same pattern as af-core). Implement with a SELECT FOR UPDATE or accept genesis hash for simplicity in v1.2.0, proper chaining in v1.3.0.

---

## Fix 1.2.0-A3 — Wire GV_CORS_ORIGINS Environment Variable

**Priority**: P2 — hardcoded origins are a security risk and ops burden

### Root Cause

`main.go` lines 101–107 hardcode CORS origins. `GV_CORS_ORIGINS` is referenced in the README but never read.

### File to Change

**`api-gateway/cmd/server/main.go`**

```go
// Parse CORS origins from env (comma-separated)
corsOrigins := strings.Split(
    envOr("GV_CORS_ORIGINS", "http://localhost:3000,http://localhost:5173"),
    ",",
)
for i := range corsOrigins {
    corsOrigins[i] = strings.TrimSpace(corsOrigins[i])
}

// Replace hardcoded AllowedOrigins:
r.Use(cors.Handler(cors.Options{
    AllowedOrigins:   corsOrigins,
    AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
    AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID", "X-AF-Source", "X-AF-Tenant"},
    AllowCredentials: true,
    MaxAge:           300,
}))
```

---

## Fix 1.2.0-A4 — Fix context.Context in exchangeCode

**Priority**: P1

### Root Cause

`oidc.go` line 445: `exchangeCode(ctx interface{ Done() <-chan struct{} }, ...)` — partial interface, `http.PostForm` on line 460 has no timeout.

### File to Change

**`api-gateway/internal/auth/oidc.go`**

Change signature to accept full `context.Context`:
```go
func (h *OIDCHandler) exchangeCode(ctx context.Context, code, verifier string) (*tokenResponse, error) {
    tokenEndpoint, err := h.discoverTokenEndpoint()
    if err != nil {
        return nil, err
    }

    body := url.Values{ /* same as now */ }

    req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint,
        strings.NewReader(body.Encode()))
    if err != nil {
        return nil, fmt.Errorf("build token request: %w", err)
    }
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

    client := &http.Client{Timeout: 10 * time.Second}
    resp, err := client.Do(req)
    // ... rest same
```

Update the call site in `Callback()` to pass `r.Context()`:
```go
tokenResp, err := h.exchangeCode(r.Context(), code, state.Verifier)
```

---

## Fix 1.2.0-A5 — Cache OIDC Discovery Document

**Priority**: P2

### Root Cause

`discoverAuthEndpoint()` and `discoverTokenEndpoint()` both call `h.discover()` independently, making two uncached HTTP calls to `/.well-known/openid-configuration` per login.

### File to Change

**`api-gateway/internal/auth/oidc.go`**

Add a discovery cache parallel to `jwksCache`:
```go
type discoveryCache struct {
    mu        sync.RWMutex
    doc       *oidcDiscovery
    fetchedAt time.Time
    ttl       time.Duration
}

func newDiscoveryCache() *discoveryCache {
    return &discoveryCache{ttl: 1 * time.Hour}
}

func (c *discoveryCache) get() *oidcDiscovery { /* same pattern as jwksCache.get() */ }
func (c *discoveryCache) set(d *oidcDiscovery) { /* same pattern as jwksCache.set() */ }
```

Add `discoveryCache *discoveryCache` field to `OIDCHandler`. Update `discover()` to check + populate the cache. `discoverAuthEndpoint()` and `discoverTokenEndpoint()` call the cached `discover()`.

---

## Fix 1.2.0-B1 — HTTP Security Response Headers Middleware

**Priority**: P2

### File to Change

**`api-gateway/internal/middleware/middleware.go`**

Add after the Prometheus middleware:
```go
// SecurityHeaders adds defence-in-depth HTTP response headers.
// These headers are a requirement for SOC 2 CC6.8 / HIPAA §164.312(a)(2)(iv).
func SecurityHeaders(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("X-Content-Type-Options", "nosniff")
        w.Header().Set("X-Frame-Options", "DENY")
        w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
        w.Header().Set("X-XSS-Protection", "1; mode=block")
        // Strict CSP — tighten as portal bundling allows
        w.Header().Set("Content-Security-Policy",
            "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:")
        // Permissions policy
        w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
        next.ServeHTTP(w, r)
    })
}
```

**`api-gateway/cmd/server/main.go`**: Add `r.Use(middleware.SecurityHeaders)` after the Prometheus middleware.

**Tests to add**:
- `middleware_test.go`: `TestSecurityHeaders_Present` — verify all six headers are set on every response

---

## Fix 1.2.0-B2 — Remove Dead `const schema` and Stale TODO

**Priority**: P3 (low effort, zero risk)

**File**: `api-gateway/internal/store/postgres.go`

- Delete lines 50–160 (`const schema = \`...\``)
- Line 862: Delete the comment `// TODO(v1.1): migrate to bcrypt once golang.org/x/crypto is a declared dependency.` — bcrypt is already implemented

---

## Fix 1.2.0-B3 — RFC 4122 UUID Generation

**Priority**: P2

### Root Cause

`generateStoreID()` formats random bytes without setting UUID version 4 / variant bits.

### File to Change

**`api-gateway/go.mod`**: Add `github.com/google/uuid v1.6.0`

**`api-gateway/internal/store/postgres.go`** — replace `generateStoreID()`:
```go
import "github.com/google/uuid"

func generateStoreID() string {
    return uuid.New().String()
}
```

---

## Fix 1.2.0-B4 — af-core Integration Test Harness

**Priority**: P2 — critical for validating the policy engine and audit chain hash computation matches the Go verification code.

### What to Build

Create `tests/integration/af_core_test.go`:

```
Test 1: Audit chain hash format agreement
  - Insert 3 policy_audit_log rows via af-core's Kafka consumer (using test Kafka topic)
  - Call GET /api/v1/audit/verify
  - Assert: valid=true, entries_checked=3

Test 2: PII redaction
  - Send a span with SSN pattern "123-45-6789" in attributes
  - Wait for af-core to process (poll with timeout)
  - Retrieve span from API
  - Assert: SSN pattern is not present in returned span attributes

Test 3: Cost threshold policy
  - Configure policy: per_run_limit_usd=0.001
  - Send span with cost_usd=0.01 (10x limit)
  - Wait for policy evaluation
  - Check audit log: decision=deny, policy=cost_threshold
```

Use `testcontainers-go` to spin up PostgreSQL + Kafka for integration tests. Target: `make test-integration` separate from unit tests.

---

## Fix 1.2.0-C1 — VITE_API_URL Env Var Consistency

**Priority**: P2

**File**: `portal/src/pages/LoginPage.tsx`

Line 9: Change `VITE_API_BASE_URL` to `VITE_API_URL`:
```typescript
// Before
const BASE = import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080'

// After
const BASE = import.meta.env.VITE_API_URL ?? 'http://localhost:8080'
```

Update `.env.example`, `.env.development`, and all docker-compose `build.args` references.

---

## Fix 1.2.0-C2 — Edit-User Route (Disable Button Until Implemented)

**Priority**: P3

**File**: `portal/src/pages/UsersPage.tsx`

Until `/users/:id/edit` route + component exists, disable the Edit button:
```tsx
// Add disabled state and tooltip
<button
  style={{...s.actionBtn, opacity: canEdit ? 1 : 0.4, cursor: canEdit ? 'pointer' : 'not-allowed'}}
  disabled={!canEdit}
  title={canEdit ? 'Edit user' : 'Edit not yet available'}
  onClick={() => canEdit && navigate(`/users/${u.user_id}/edit`)}
>
  Edit
</button>
```

---

## Fix 1.2.0-C3 — Version Footer Driven by Env Var

**Priority**: P3

**File**: `portal/src/pages/LoginPage.tsx`

Line 155:
```typescript
// Before
<div style={s.footer}>v1.0.0 · © Govagn</div>

// After
<div style={s.footer}>{import.meta.env.VITE_APP_VERSION ?? 'v1.2.0'} · © Govagn</div>
```

Add to `portal/vite.config.ts`:
```typescript
define: {
    'import.meta.env.VITE_APP_VERSION': JSON.stringify(process.env.npm_package_version ?? '1.2.0')
}
```

---

## v1.2.0 Delivery Checklist

```
WS-A (Backend):
[ ] 1.2.0-A1: Cursor pagination on traces, runs, agents — no regression on existing tests
[ ] 1.2.0-A2: Admin action audit trail — user CRUD writes to policy_audit_log
[ ] 1.2.0-A3: GV_CORS_ORIGINS wired — hardcoded origins removed
[ ] 1.2.0-A4: context.Context in exchangeCode — no more fire-and-forget HTTP calls
[ ] 1.2.0-A5: OIDC discovery document cached (1 hour TTL)

WS-B (Security):
[ ] 1.2.0-B1: Security headers middleware — all 6 headers on every response
[ ] 1.2.0-B2: Dead schema removed, stale TODO removed
[ ] 1.2.0-B3: RFC 4122 UUIDs via google/uuid
[ ] 1.2.0-B4: af-core integration test harness — 3 scenarios green

WS-C (Portal):
[ ] 1.2.0-C1: VITE_API_URL consistency across all portal files
[ ] 1.2.0-C2: Edit button disabled until route exists
[ ] 1.2.0-C3: Version footer reads from VITE_APP_VERSION

Cross-cutting:
[ ] make test passes (all tests including new integration tests)
[ ] make lint passes
[ ] PRODUCTION_CHECKLIST.md updated for v1.2.0 additions
[ ] CHANGELOG.md [1.2.0] section written
[ ] git tag v1.2.0, PR to main, merge
```

---

## Dependency Graph (Sequential Constraints)

```
1.1.1-A (schema)
    └── 1.1.1-B (login) — needs GetUserByUsername on the unified schema
        └── 1.1.1-C (HttpOnly) — needs B landed first (cookie carries DB-user role)

1.1.1-D, 1.1.1-E, 1.1.1-F — independent, parallel with A/B/C

1.2.0-A1 (cursor pagination) — independent
1.2.0-A2 (audit trail) — needs 1.1.1-A (correct schema) to be in main
1.2.0-A3, A4, A5 — independent
1.2.0-B1, B2, B3 — independent
1.2.0-B4 (af-core tests) — depends on 1.1.1-A (correct schema) to be in main
1.2.0-C1, C2, C3 — independent
```

---

*Fix plan prepared 2026-03-16 — Govagn v1.1.1 + v1.2.0*
