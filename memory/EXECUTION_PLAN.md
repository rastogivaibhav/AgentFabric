# Govagn — Execution Plan

## Purpose of This Document

This document is a self-contained implementation brief for a new Claude Code session. Read this alongside `memory/MEMORY.md` (project state and architecture). No prior conversation context is assumed.

**Repo root:** `C:\Users\vrast\Documents\Agentic Code\files\`

---

## Problem Being Solved

Govagn's SDK requires developers to opt in by calling `govagn.instrument()`. Any developer who skips those two lines is invisible: their LLM calls bypass all tracking, budget enforcement, and governance. A single rogue engineer or contractor can exhaust an enterprise API quota silently.

This plan implements three layers of defence, from easiest to deploy to most comprehensive.

---

## Three-Layer Defence Overview

| Layer | Priority | Effort | What it catches |
|-------|----------|--------|-----------------|
| 1a — sitecustomize.py auto-instrumentation | P0 | 1 day | Any Python process on a machine with govagn installed |
| 1b — Budget hard-limit enforcement | P0 | 1 day | Over-budget ingest requests blocked at api-gateway |
| 2 — API Key Proxy (virtual keys) | P1 | 1 week | Any SDK that can point at a base URL |
| 3 — Network transparent proxy | P2 | 2 weeks | Everything, including curl, CLI tools, non-Python runtimes |

---

## Layer 1a — P0: sitecustomize.py Auto-Instrumentation

### What it does

Python automatically imports `sitecustomize.py` from site-packages at the start of every Python process — before user code runs. Installing govagn's patching there means every Python process is instrumented with zero developer action.

### Files to create / modify

```
agent-sdk/govagn/auto_instrument.py     NEW
agent-sdk/govagn/sitecustomize.py       NEW
agent-sdk/install_hooks.py                   NEW
agent-sdk/tests/test_auto_instrument.py      NEW
agent-sdk/pyproject.toml                     MODIFY — add post-install hook
```

### High-level class structure

#### `agent-sdk/govagn/auto_instrument.py`

```python
class AutoInstrumentor:
    """
    Called once at Python startup from sitecustomize.py.
    Reads environment, decides which frameworks to patch, bootstraps tracer.
    """
    ENDPOINT_ENV  = "GV_ENDPOINT"        # e.g. http://localhost:4318
    TENANT_ENV    = "GV_TENANT_ID"       # e.g. acme-corp
    DISABLED_ENV  = "GV_AUTO_INSTRUMENT" # set to "0" to opt out

    def __init__(self):
        self.endpoint  : str | None
        self.tenant_id : str | None
        self.enabled   : bool

    def run(self) -> None:
        """Entry point called by sitecustomize.py"""
        # 1. read_config()
        # 2. if not enabled: return
        # 3. setup_tracer()
        # 4. patch_all_available()

    def read_config(self) -> None:
        """Read GV_ENDPOINT, GV_TENANT_ID, GV_AUTO_INSTRUMENT from env"""

    def setup_tracer(self) -> None:
        """
        Create OTLPSpanExporter pointing at self.endpoint.
        Create TracerProvider + BatchSpanProcessor.
        Inject into govagn._tracer + govagn._initialized = True.
        """

    def patch_all_available(self) -> None:
        """
        For each framework, try import — if available, call govagn patch fn.
        Never raises — failures are silently logged to stderr.
        Frameworks: openai, anthropic, langgraph, crewai, google.adk
        """

    def _try_patch(self, module_name: str, patch_fn_name: str) -> bool:
        """
        importlib.import_module(module_name)
        getattr(govagn, patch_fn_name)(module)
        return True on success, False on ImportError/AttributeError
        """
```

#### `agent-sdk/govagn/sitecustomize.py`

```python
# Auto-installed into site-packages by pip post-install hook.
# Python calls this automatically before any user code.

def _boot():
    try:
        from govagn.auto_instrument import AutoInstrumentor
        AutoInstrumentor().run()
    except Exception:
        pass  # never break user's process

_boot()
```

#### `agent-sdk/install_hooks.py`

```python
class SitecustomizeInstaller:
    """
    Called by pyproject.toml [tool.hatch.build.hooks] or setup.py post_install.
    Copies govagn/sitecustomize.py into sys.prefix/lib/.../site-packages/sitecustomize.py.
    Handles merging if a sitecustomize.py already exists (appends govagn block).
    """
    GUARD = "# govagn-auto-instrument"

    def install(self) -> None:
        # find_site_packages_dir()
        # read existing sitecustomize if present
        # if GUARD already in content: skip (idempotent)
        # append _boot() block guarded by GUARD comment
        # write back

    def uninstall(self) -> None:
        # remove govagn block from sitecustomize.py
        # if file is now empty: delete it
```

### Pseudo-code flow

```
pip install govagn
    → post_install hook runs SitecustomizeInstaller.install()
    → site-packages/sitecustomize.py now contains _boot()

python any_script.py
    → CPython loads site-packages/sitecustomize.py
    → _boot() runs
    → AutoInstrumentor().run()
        → reads GV_ENDPOINT from env (default: http://localhost:4318)
        → reads GV_TENANT_ID from env
        → if GV_AUTO_INSTRUMENT=0: return early
        → creates OTLPSpanExporter(endpoint=GV_ENDPOINT)
        → creates TracerProvider(BatchSpanProcessor(exporter))
        → govagn._tracer = provider.get_tracer("govagn", "1.0.0")
        → govagn._initialized = True
        → _try_patch("openai",     "_patch_openai")
        → _try_patch("anthropic",  "_patch_anthropic")
        → _try_patch("langgraph",  "_patch_langgraph")
        → _try_patch("crewai",     "_patch_crewai")
        → _try_patch("google.adk", "_patch_google_adk")
    → your_script.py code runs — already fully patched
```

### Environment variables

```
GV_ENDPOINT          OTLP HTTP endpoint           default: http://localhost:4318
GV_TENANT_ID         Tenant identifier            default: "default"
GV_AUTO_INSTRUMENT   Set to "0" to disable        default: enabled
GV_SERVICE_NAME      OTel service.name attribute  default: auto-detected from sys.argv[0]
```

### Tests to write (`agent-sdk/tests/test_auto_instrument.py`)

1. `test_disabled_when_env_var_zero` — GV_AUTO_INSTRUMENT=0 → no patches applied
2. `test_patches_openai_when_installed` — openai present → patched
3. `test_skips_missing_frameworks` — missing package → no crash, continues
4. `test_reads_endpoint_from_env` — GV_ENDPOINT overrides default
5. `test_idempotent_install` — calling install() twice does not duplicate sitecustomize block
6. `test_merges_with_existing_sitecustomize` — existing sitecustomize.py content preserved

### Definition of done

- [ ] `pip install govagn` installs sitecustomize.py into site-packages
- [ ] `python test.py` (with openai imported, zero govagn lines) → span appears in portal
- [ ] `GV_AUTO_INSTRUMENT=0 python test.py` → no spans (opt-out works)
- [ ] Existing sitecustomize.py is preserved (merge, not overwrite)
- [ ] All `test_auto_instrument.py` tests pass

---

## Layer 1b — P0: Budget Hard-Limit Enforcement

### What it does

The api-gateway checks every ingest request against a per-tenant token/cost budget stored in PostgreSQL. If the tenant is over-limit, the gateway returns HTTP 429 and triggers an alert. The collector's LLM proxy endpoints (Layer 2) enforce the same limit before forwarding.

### Files to create / modify

```
deploy/migrations/0003_budgets.up.sql              NEW
deploy/migrations/0003_budgets.down.sql            NEW
api-gateway/internal/budget/budget.go              NEW
api-gateway/internal/budget/budget_test.go         NEW
api-gateway/internal/handlers/budget_handler.go    NEW
api-gateway/internal/handlers/handlers.go          MODIFY — add budget check in Ingest()
api-gateway/internal/store/postgres.go             MODIFY — add budget query methods
api-gateway/internal/router/router.go              MODIFY — mount budget routes
portal/src/pages/Cost.tsx                          MODIFY — add budget configuration UI
```

### Database schema

```sql
-- deploy/migrations/0003_budgets.up.sql

CREATE TABLE tenant_budgets (
    tenant_id        TEXT          PRIMARY KEY,
    monthly_tokens   BIGINT        NOT NULL DEFAULT 0,     -- 0 = unlimited
    monthly_cost_usd NUMERIC(10,4) NOT NULL DEFAULT 0,
    alert_threshold  NUMERIC(5,2)  NOT NULL DEFAULT 0.80,  -- 80% triggers alert
    hard_limit       BOOLEAN       NOT NULL DEFAULT true,  -- false = alert only
    reset_day        INT           NOT NULL DEFAULT 1,     -- day of month to reset
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE TABLE usage_alerts (
    id              BIGSERIAL     PRIMARY KEY,
    tenant_id       TEXT          NOT NULL,
    alert_type      TEXT          NOT NULL,  -- 'threshold_80', 'threshold_100', 'hard_limit'
    tokens_used     BIGINT,
    cost_used_usd   NUMERIC(10,4),
    budget_tokens   BIGINT,
    budget_cost_usd NUMERIC(10,4),
    triggered_at    TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);
CREATE INDEX ON usage_alerts(tenant_id, triggered_at DESC);
```

### High-level class structure

#### `api-gateway/internal/budget/budget.go`

```go
type Budget struct {
    TenantID       string
    MonthlyTokens  int64   // 0 = unlimited
    MonthlyCostUSD float64 // 0 = unlimited
    AlertThreshold float64 // 0.80 = alert at 80%
    HardLimit      bool    // true = block at 100%, false = alert only
}

type UsageSummary struct {
    TokensUsed  int64
    CostUsedUSD float64
    PeriodStart time.Time
}

type BudgetEnforcer struct {
    store   Store   // interface — queries PostgreSQL + Redis cache
    alerter Alerter // interface — sends webhook / email / Slack
}

// CheckAndRecord is called on every ingest request BEFORE writing spans.
// Returns (allowed bool, err error).
// If allowed=false, caller returns HTTP 429 to the SDK.
func (e *BudgetEnforcer) CheckAndRecord(
    ctx context.Context,
    tenantID string,
    incomingTokens int64,
    incomingCostUSD float64,
) (allowed bool, err error)
// Flow:
// 1. budget = store.GetBudget(tenantID)
// 2. if budget is nil or both limits are 0: return true (unlimited)
// 3. usage = store.GetMonthlyUsage(tenantID, currentPeriodStart())
// 4. check token limit:
//      if budget.MonthlyTokens > 0:
//          projected = usage.TokensUsed + incomingTokens
//          if projected > budget.MonthlyTokens && budget.HardLimit:
//              alerter.Fire(tenantID, "hard_limit", ...)
//              return false, nil
//          if projected > budget.MonthlyTokens * budget.AlertThreshold:
//              alerter.Fire(tenantID, "threshold_80", ...)  // deduped
// 5. same check for cost
// 6. return true, nil

type Store interface {
    GetBudget(ctx context.Context, tenantID string) (*Budget, error)
    GetMonthlyUsage(ctx context.Context, tenantID string, since time.Time) (*UsageSummary, error)
    UpsertBudget(ctx context.Context, b *Budget) error
    RecordAlert(ctx context.Context, tenantID, alertType string, usage UsageSummary, budget Budget) error
}

type Alerter interface {
    Fire(ctx context.Context, tenantID, alertType string, usage UsageSummary, budget Budget) error
}

// WebhookAlerter implements Alerter.
// Sends JSON POST to the tenant's configured webhook URL.
// Deduped: only fires once per (tenantID, alertType, billing period).
type WebhookAlerter struct { ... }
```

### Integration into ingest flow

```go
// api-gateway/internal/handlers/handlers.go — Ingest()
func (h *Handler) Ingest(w http.ResponseWriter, r *http.Request) {
    // ... existing decode logic ...

    // NEW: count incoming tokens from spans
    totalTokens := sumTokens(req.Spans)
    totalCost   := sumCost(req.Spans)
    tenantID    := r.Header.Get("X-Tenant-ID")  // or from JWT

    // NEW: budget check BEFORE writing to DB
    allowed, err := h.budgetEnforcer.CheckAndRecord(r.Context(), tenantID, totalTokens, totalCost)
    if err != nil {
        // log but don't block — fail open on enforcer errors
    }
    if !allowed {
        writeError(w, http.StatusTooManyRequests, "monthly budget exceeded")
        return
    }

    // ... existing store.SaveSpans logic ...
}
```

### New REST endpoints (`budget_handler.go`)

```
GET    /api/v1/budgets/:tenant_id          — get current budget + usage
PUT    /api/v1/budgets/:tenant_id          — set/update budget limits
DELETE /api/v1/budgets/:tenant_id          — remove budget (unlimited)
GET    /api/v1/budgets/:tenant_id/alerts   — list triggered alerts
GET    /api/v1/budgets/:tenant_id/usage    — current period usage breakdown
```

### Tests to write (`api-gateway/internal/budget/budget_test.go`)

1. `TestBudgetEnforcer_UnderLimit` — tokens within budget → allowed=true
2. `TestBudgetEnforcer_OverTokenLimit` — exceeds token limit → allowed=false
3. `TestBudgetEnforcer_OverCostLimit` — exceeds cost limit → allowed=false
4. `TestBudgetEnforcer_SoftLimitOnly` — hard_limit=false → allowed=true + alert fired
5. `TestBudgetEnforcer_AlertThreshold` — at 80% → alert fired, still allowed
6. `TestBudgetEnforcer_Unlimited` — budget=0 → always allowed
7. `TestBudgetEnforcer_AlertDeduplication` — same alert not fired twice per period
8. `TestIngest_Returns429WhenBudgetExceeded` — full HTTP handler integration test

### Definition of done

- [ ] `PUT /api/v1/budgets/tenant1` sets a 1000-token limit
- [ ] After 1000 tokens ingested, next ingest returns 429
- [ ] Portal Cost page shows % used + hard limit bar
- [ ] Alert recorded in `usage_alerts` table at 80% threshold
- [ ] All `budget_test.go` tests pass

---

## Layer 2 — P1: API Key Proxy (Virtual Keys)

### What it does

Customers register their real LLM API keys with Govagn. Govagn issues virtual keys (`af-vk-*`). Developers point their SDKs at Govagn's collector and use virtual keys. The collector authenticates the virtual key, checks budget, records the span, then forwards the request to the real LLM API with the real key — transparently. Real keys never leave the vault.

### Files to create / modify

```
deploy/migrations/0004_virtual_keys.up.sql       NEW
deploy/migrations/0004_virtual_keys.down.sql     NEW
api-gateway/internal/vault/vault.go              NEW
api-gateway/internal/vault/vault_test.go         NEW
api-gateway/internal/proxy/proxy.go              NEW
api-gateway/internal/proxy/parsers_openai.go     NEW
api-gateway/internal/proxy/parsers_anthropic.go  NEW
api-gateway/internal/proxy/proxy_test.go         NEW
api-gateway/internal/handlers/budget_handler.go  NEW (if not done in Layer 1b)
api-gateway/internal/router/router.go            MODIFY — mount proxy + key routes
portal/src/pages/ApiKeys.tsx                     NEW
portal/src/App.tsx                               MODIFY — add /api-keys route
```

### Database schema

```sql
-- deploy/migrations/0004_virtual_keys.up.sql

CREATE TABLE virtual_keys (
    id           BIGSERIAL   PRIMARY KEY,
    tenant_id    TEXT        NOT NULL,
    virtual_key  TEXT        NOT NULL UNIQUE,  -- af-vk-<random32>
    display_name TEXT        NOT NULL,
    provider     TEXT        NOT NULL,         -- 'openai' | 'anthropic' | 'google'
    real_key_enc BYTEA       NOT NULL,         -- AES-256-GCM encrypted real key
    key_id       TEXT        NOT NULL,         -- first 8 chars of real key (for display)
    team_id      TEXT,
    created_by   TEXT,
    expires_at   TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    revoked      BOOLEAN     NOT NULL DEFAULT false,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX ON virtual_keys(virtual_key) WHERE NOT revoked;
CREATE INDEX ON virtual_keys(tenant_id);
```

### High-level class structure

#### `api-gateway/internal/vault/vault.go`

```go
type Vault struct {
    db        *pgxpool.Pool
    masterKey []byte   // 32-byte AES key, loaded from env GV_VAULT_KEY
}

// Store encrypts realKey and inserts into DB. Returns generated virtual key.
func (v *Vault) Store(tenantID, provider, realKey, displayName string) (virtualKey string, err error)
// Generates virtualKey = "af-vk-" + secureRandom(32)
// Encrypts realKey with AES-256-GCM using masterKey + random nonce
// INSERTs into virtual_keys
// Returns virtualKey

// Resolve looks up the virtual key, decrypts the real key, and updates last_used_at.
func (v *Vault) Resolve(virtualKey string) (provider, realKey, tenantID string, err error)
// SELECT from virtual_keys WHERE virtual_key = $1 AND NOT revoked
// if not found: return ErrInvalidKey
// decrypt real_key_enc
// UPDATE last_used_at
// return provider, decrypted realKey, tenantID

func (v *Vault) Revoke(virtualKey string) error
```

#### `api-gateway/internal/proxy/proxy.go`

```go
// LLMProxy implements http.Handler.
// Mounts at /proxy/openai/v1/*  /proxy/anthropic/v1/*  /proxy/google/v1/*
// Developers set:
//   OPENAI_BASE_URL=http://localhost:8080/proxy/openai/v1
//   OPENAI_API_KEY=af-vk-teamA-001

type LLMProxy struct {
    vault          *vault.Vault
    budgetEnforcer *budget.BudgetEnforcer
    store          store.Store
    httpClient     *http.Client  // used for forwarding
}

func (p *LLMProxy) ServeHTTP(w http.ResponseWriter, r *http.Request)
// Flow:
// 1. extractVirtualKey(r)  — from Authorization: Bearer <key>
// 2. provider, realKey, tenantID = vault.Resolve(virtualKey)
//    if err: 401 Unauthorized
// 3. body = readAndBuffer(r.Body, maxSize)
//    request = parseRequest(provider, body)  — extract model, messages
// 4. estimatedTokens = estimateTokens(request)
//    allowed = budgetEnforcer.CheckAndRecord(tenantID, estimatedTokens, 0)
//    if !allowed: 429 TooManyRequests
// 5. forwardRequest(provider, realKey, r, body) → upstreamResp
// 6. responseBody = readResponse(upstreamResp)
//    actualTokens, cost = parseUsage(provider, responseBody)
// 7. recordSpan(tenantID, provider, model, actualTokens, cost, duration)
// 8. writeResponse(w, upstreamResp.StatusCode, responseBody)
// NOTE: supports streaming (SSE / chunked) — forward chunks as they arrive

type RequestParser interface {
    Parse(body []byte) (model string, inputTokens int, messages []Message, err error)
}
// OpenAIRequestParser implements RequestParser
// AnthropicRequestParser implements RequestParser
// GoogleRequestParser implements RequestParser

type ResponseParser interface {
    ParseUsage(body []byte) (inputTokens, outputTokens int, costUSD float64, err error)
    IsStreaming(resp *http.Response) bool
    ParseStreamingUsage(chunks [][]byte) (inputTokens, outputTokens int, costUSD float64)
}
```

### Virtual key lifecycle

```
Admin registers real key:
  POST /api/v1/keys
  { "provider": "openai", "real_key": "sk-...", "display_name": "Prod OpenAI", "team_id": "engineering" }
  → vault.Store() → returns { "virtual_key": "af-vk-abc123..." }
  → real key encrypted, never returned again

Dev configures their env:
  OPENAI_BASE_URL=https://govagn.company.com/proxy/openai/v1
  OPENAI_API_KEY=af-vk-abc123...

Dev makes API call (standard openai SDK, zero code change):
  openai.chat.completions.create(model="gpt-4o", messages=[...])
  → SDK sends to OPENAI_BASE_URL
  → proxy receives, resolves virtual key → real key
  → checks budget → records span → forwards to api.openai.com
  → streams response back
  → developer sees normal response

Admin revokes key:
  DELETE /api/v1/keys/af-vk-abc123...
  → revoked=true in DB
  → next request with that key → 401 immediately
  → real API key still valid but dev no longer has a route to use it
```

### New REST endpoints

```
POST   /api/v1/keys                        — register real key, receive virtual key
GET    /api/v1/keys                        — list virtual keys for tenant (no real keys)
DELETE /api/v1/keys/:virtual_key           — revoke a virtual key
GET    /api/v1/keys/:virtual_key/usage     — token/cost usage for this key
```

### Definition of done

- [ ] `POST /api/v1/keys` registers a real OpenAI key → returns virtual key
- [ ] `OPENAI_BASE_URL=http://localhost:8080/proxy/openai/v1` + virtual key → `openai.chat.completions.create()` works transparently
- [ ] Span recorded in portal with correct tokens/cost
- [ ] Over-budget request → 429 before forwarding to real API
- [ ] Revoked key → 401 immediately
- [ ] Streaming responses work (SSE chunks forwarded correctly)
- [ ] All `proxy_test.go` tests pass

---

## Layer 3 — P2: Network Transparent Proxy

### What it does

All outbound HTTPS to LLM API domains is intercepted at the network level — no SDK, no env vars, no code changes needed. Works for Python, Node.js, Go, curl, Claude Code CLI, any tool.

### Implementation approach

Deploy an Envoy proxy as a sidecar in the Docker stack. Use iptables rules (Linux) or `HTTP_PROXY`/`HTTPS_PROXY` env vars to route all LLM-bound traffic through it.

### Files to create / modify

```
deploy/envoy/envoy.yaml             NEW — Envoy proxy config
deploy/envoy/Dockerfile             NEW
collector/internal/tlsproxy/        NEW — TLS interception + span recording
deploy/certs/ca.crt + ca.key        NEW — local CA for TLS termination
scripts/install-ca-cert.sh          NEW — installs local CA into OS trust store
docker-compose.yml                  MODIFY — add envoy service, add HTTP_PROXY env to all services
```

### Architecture

```
Any process on the machine
  └── makes HTTPS request to api.openai.com
        │
        │  iptables rule:
        │  -A OUTPUT -d api.openai.com -p tcp --dport 443 -j REDIRECT --to-port 8443
        │  (or HTTP_PROXY=http://localhost:8443 in env)
        ▼
  Envoy Proxy (:8443)
    └── TLS termination with local CA cert (machine trusts local CA)
    └── inspects request:
          Host header / SNI → identifies provider (openai / anthropic / google)
    └── calls Collector gRPC to record span
    └── re-encrypts with real TLS → forwards to actual api.openai.com
    └── intercepts response → records token usage
    └── streams response back to caller

Envoy config targets:
  api.openai.com                          → openai handler
  api.anthropic.com                       → anthropic handler
  generativelanguage.googleapis.com       → google handler
```

### Key implementation challenges and solutions

```
Challenge: TLS certificate validation
Solution:  Generate local CA cert at install time.
           install-ca-cert.sh runs:
           → adds CA to /etc/ssl/certs (Linux) or Keychain (macOS) or certstore (Windows)
           → all processes on machine trust local CA
           → Envoy presents certs signed by local CA

Challenge: Non-proxy-aware tools (ignore HTTP_PROXY)
Solution:  iptables REDIRECT rule (Linux only)
           Works for all TCP connections regardless of app behaviour

Challenge: Identifying tenant from intercepted request
Solution:  Map API key prefix to tenant:
           sk-abc... → registered in vault as tenant X
           Bearer key header parsed and looked up

Challenge: Performance (adds latency)
Solution:  Envoy is designed for this — sub-1ms overhead
           Async span recording (don't wait for DB write before forwarding)
```

### Definition of done

- [ ] `curl https://api.openai.com/v1/chat/completions` → intercepted → span in portal
- [ ] Python script with NO govagn import → span in portal
- [ ] Claude Code CLI → span in portal
- [ ] CA cert trusted by OS, no TLS errors

---

## Implementation Order and Dependency Graph

```
Week 1
───────
Day 1-2:  Layer 1a — sitecustomize.py auto-instrumentation
          Files: auto_instrument.py, sitecustomize.py, install_hooks.py
          Tests: test_auto_instrument.py
          Validates: any python script tracked with zero code change

Day 3:    Layer 1b — Budget hard-limit
          Files: 0003_budgets.up.sql, budget.go, budget_handler.go
          Modify: handlers.go (add CheckAndRecord call)
          Tests: budget_test.go, integration test for 429 response
          Validates: over-budget requests blocked at ingest

Day 4-5:  Layer 1b continued — portal budget UI
          Files: portal/src/pages/Cost.tsx (add budget config panel)
          New endpoint: PUT /api/v1/budgets/:tenant_id

Week 2
───────
Day 6-7:  Layer 2 — Vault
          Files: vault.go, 0004_virtual_keys.up.sql
          Tests: vault_test.go (encrypt/decrypt, resolve, revoke)

Day 8-10: Layer 2 — LLM Proxy
          Files: proxy.go, request parsers, response parsers
          Streaming support: SSE forwarding for OpenAI, Anthropic
          Tests: proxy_test.go with httptest.Server mocking real APIs

Day 11-12: Layer 2 — Portal ApiKeys page
           Files: portal/src/pages/ApiKeys.tsx
           Features: list keys, create key, revoke key, usage per key

Week 3-4
─────────
Day 13-15: Layer 3 — Envoy config + TLS setup
Day 16-18: Layer 3 — iptables rules + CA cert installer
Day 19-20: Layer 3 — Integration testing, Windows support (WinDivert)
```

---

## Complete File List

```
agent-sdk/
  govagn/auto_instrument.py         NEW
  govagn/sitecustomize.py           NEW
  install_hooks.py                       NEW
  tests/test_auto_instrument.py          NEW
  pyproject.toml                         MODIFY (add post-install hook)

api-gateway/
  internal/budget/budget.go             NEW
  internal/budget/budget_test.go        NEW
  internal/vault/vault.go               NEW
  internal/vault/vault_test.go          NEW
  internal/proxy/proxy.go               NEW
  internal/proxy/parsers_openai.go      NEW
  internal/proxy/parsers_anthropic.go   NEW
  internal/proxy/proxy_test.go          NEW
  internal/handlers/budget_handler.go   NEW
  internal/handlers/handlers.go         MODIFY (add budget check in Ingest)
  internal/store/postgres.go            MODIFY (add GetBudget, GetMonthlyUsage, UpsertBudget)
  internal/router/router.go             MODIFY (mount proxy + budget routes)

deploy/
  migrations/0003_budgets.up.sql        NEW
  migrations/0003_budgets.down.sql      NEW
  migrations/0004_virtual_keys.up.sql   NEW
  migrations/0004_virtual_keys.down.sql NEW
  envoy/envoy.yaml                      NEW  (Layer 3)
  envoy/Dockerfile                      NEW  (Layer 3)

portal/src/
  pages/ApiKeys.tsx                     NEW
  pages/Cost.tsx                        MODIFY (add budget panel)
  App.tsx                               MODIFY (add /api-keys route)

scripts/
  install-ca-cert.sh                    NEW  (Layer 3)

docker-compose.yml                      MODIFY (add envoy service — Layer 3)
```

---

## Key Environment Variables Added by This Work

```
# Layer 1 — auto-instrumentation
GV_ENDPOINT           = http://localhost:4318   # where spans go
GV_TENANT_ID          = default                 # tenant identifier
GV_AUTO_INSTRUMENT    = 1                       # set 0 to disable
GV_SERVICE_NAME       = (auto from argv[0])     # OTel service.name

# Layer 1 — budget
GV_BUDGET_ENABLED     = true                    # global on/off

# Layer 2 — vault
GV_VAULT_KEY          = <32-byte hex>           # AES master key for key encryption
GV_PROXY_ENABLED      = true

# Layer 3 — network proxy
GV_NET_PROXY_ENABLED  = false                   # opt-in (requires iptables/CA install)
GV_CA_CERT_PATH       = ./deploy/certs/ca.crt
```

---

## Starting Prompt for New Claude Code Session

Paste this as the first message in the new session:

```
Project: Govagn — enterprise AI observability platform
Repo: C:\Users\vrast\Documents\Agentic Code\files\
Context files:
  - memory/MEMORY.md          (project state, architecture, what's been built)
  - memory/EXECUTION_PLAN.md  (this file — detailed plan for next features)

Task: Implement the features in EXECUTION_PLAN.md in priority order.
Start with Layer 1a (sitecustomize.py auto-instrumentation) then Layer 1b (budget enforcement).

The Docker stack is running (docker compose up).
PostgreSQL is on :5432, api-gateway on :8080, portal on :3000.
All 175 integration tests in integrationTests/ currently pass.

Begin by reading both memory files, then implement Layer 1a.
```
