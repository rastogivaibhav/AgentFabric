# Enterprise AI Telemetry Platform — Expanded Vision

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build **AgentFabric** as the vendor-neutral control plane for AI development tools. Ingest, normalize, govern, and visualize telemetry from:
- **Terminal agents** (Codex CLI, Claude Code)
- **AI editors** (Cursor)
- **IDE agents** (VSCode Copilot, Continue, Roo Code, Cline)
- **Anthropic ecosystem** (Claude API, Cowork)
- **CI/CD agents** (PR reviewers, test bots)

**Architecture:** Multi-source telemetry (OTLP, VSCode extensions, webhooks, direct APIs) → Unified Collector → Flexible Normalization → PostgreSQL → Enterprise Dashboards.

**Outcome:** Single pane of glass for all AI development tool usage, cost, risk, and governance across your entire engineering org.

---

## Implementation Phases

### **PHASE 1: Foundation — Codex + Claude Code** (✅ COMPLETED — 5 commits)
- Framework detection (collector)
- Canonical schema (PostgreSQL)
- Source mappers (api-gateway)
- User documentation

### **PHASE 2: Unified Collection & Governance** (NEXT — ~8 tasks)
- Multi-source ingestion layer
- Flexible canonical schema
- Pluggable normalisation framework
- Risk scoring engine
- Admin redaction controls

### **PHASE 3: Extended Tool Support** (FOLLOW — ~10 tasks)
- Cursor telemetry mapper
- VSCode Extension API integration
- Cowork agent mapper
- Anthropic API direct instrumentation
- Webhook/proxy ingestion

### **PHASE 4: Enterprise Features** (~7 tasks)
- Multi-tool dashboards
- Productivity metrics
- Governance workflows
- Tool comparison analytics
- Final documentation & README

---

## Phase 2: Unified Collection & Governance

### Task 5: Implement Multi-Source Ingestion Layer

**Files:**
- Create: `collector/internal/receiver/multi_source.go` (HTTP, VSCode extension webhook, direct API)
- Create: `collector/internal/receiver/multi_source_test.go`
- Modify: `collector/cmd/collector/main.go` (register routes)
- Test: Manual verification of all receiver types

**Responsibility:** Handle OTLP, VSCode extensions, webhook events, and direct API calls in a unified way.

- [ ] **Step 1: Create multi-source receiver interface**

```go
package receiver

type EventReceiver interface {
    Receive(ctx context.Context, event interface{}) error
    Name() string
}

// OTLPReceiver handles OpenTelemetry data
type OTLPReceiver struct { ... }

// VSCodeExtensionReceiver handles VSCode webhook events
type VSCodeExtensionReceiver struct { ... }

// WebhookReceiver handles generic webhooks
type WebhookReceiver struct { ... }
```

- [ ] **Step 2: Write test for VSCode extension webhook**

```go
func TestVSCodeExtensionReceiver_AcceptsTelemetry(t *testing.T) {
    receiver := NewVSCodeExtensionReceiver()
    event := map[string]interface{}{
        "source": "vscode-copilot",
        "event_type": "suggestion.accepted",
        "timestamp": time.Now(),
    }
    err := receiver.Receive(context.Background(), event)
    assert.NoError(t, err)
}
```

- [ ] **Step 3: Implement VSCode extension receiver**

Handles events like:
- `suggestion.accepted`
- `command.executed`
- `chat.started`
- `file.saved`

- [ ] **Step 4: Register routes in main.go**

```go
// OTLP routes (existing)
r.Post("/v1/traces", otlpReceiver.ReceiveTraces)
r.Post("/v1/metrics", otlpReceiver.ReceiveMetrics)
r.Post("/v1/logs", otlpReceiver.ReceiveLogs)

// VSCode extension webhook
r.Post("/api/v1/telemetry/vscode", vscodeReceiver.Receive)

// Generic webhook
r.Post("/api/v1/telemetry/webhook", webhookReceiver.Receive)

// Direct API ingestion
r.Post("/api/v1/telemetry/events", directReceiver.Receive)
```

- [ ] **Step 5: Run tests**

Run: `cd collector && go test ./internal/receiver -v`

- [ ] **Step 6: Commit**

```bash
git add collector/internal/receiver/multi_source.go collector/internal/receiver/multi_source_test.go collector/cmd/collector/main.go
git commit -m "feat: add multi-source telemetry receivers (OTLP, VSCode, webhooks)"
```

---

### Task 6: Flexible Canonical Schema

**Files:**
- Modify: `deploy/sql/init.sql` (replace 3-table schema with unified schema)
- Create: `deploy/sql/migrations/003_unified_schema.up.sql`
- Create: `deploy/sql/migrations/003_unified_schema.down.sql`

**Responsibility:** Replace ai_agent_events, usage, and tool_calls with single flexible table supporting all tools.

- [ ] **Step 1: Create migration for unified schema**

File: `deploy/sql/migrations/003_unified_schema.up.sql`

```sql
-- Drop old tables (or migrate data)
-- CREATE unified table
CREATE TABLE ai_dev_events (
    id              UUID            PRIMARY KEY DEFAULT gen_random_uuid(),
    ts              TIMESTAMPTZ     NOT NULL,
    source_vendor   VARCHAR(32)     NOT NULL, -- codex, cursor, vscode, anthropic
    source_product  VARCHAR(64),               -- codex-cli, cursor-editor, github-copilot
    source_channel  VARCHAR(32),               -- otlp, extension, api, webhook
    user_id         TEXT,
    user_email      TEXT,
    team_id         UUID,
    repo            TEXT,
    model           TEXT,
    event_type      VARCHAR(64)     NOT NULL,
    event_category  VARCHAR(32),               -- session, model_call, tool_call, approval
    action          VARCHAR(64),
    success         BOOLEAN,
    latency_ms      BIGINT,
    input_tokens    BIGINT,
    output_tokens   BIGINT,
    cache_read_tokens BIGINT,
    cache_write_tokens BIGINT,
    estimated_cost_usd NUMERIC(12, 6),
    risk_score      INTEGER DEFAULT 0,
    risk_category   VARCHAR(32),               -- unsafe_command, secret_exposure, prod_edit
    requires_review BOOLEAN DEFAULT FALSE,
    payload         JSONB           NOT NULL,
    redacted        BOOLEAN         DEFAULT TRUE,
    created_at      TIMESTAMPTZ     DEFAULT now()
);

CREATE INDEX idx_ai_dev_events_ts ON ai_dev_events(ts DESC);
CREATE INDEX idx_ai_dev_events_vendor ON ai_dev_events(source_vendor, ts DESC);
CREATE INDEX idx_ai_dev_events_user ON ai_dev_events(user_email, ts DESC);
CREATE INDEX idx_ai_dev_events_risk ON ai_dev_events(risk_score DESC) WHERE risk_score > 50;
CREATE INDEX idx_ai_dev_events_repo ON ai_dev_events(repo, ts DESC) WHERE repo IS NOT NULL;
```

- [ ] **Step 2: Verify schema migration**

Run: `cd deploy/docker && docker-compose exec postgres psql -U fabric -d govagn -c "\d ai_dev_events"`

- [ ] **Step 3: Create down migration**

File: `deploy/sql/migrations/003_unified_schema.down.sql`

```sql
DROP TABLE IF EXISTS ai_dev_events;
```

- [ ] **Step 4: Commit**

```bash
git add deploy/sql/migrations/003_unified_schema.up.sql deploy/sql/migrations/003_unified_schema.down.sql
git commit -m "feat: create unified ai_dev_events schema for all tools"
```

---

### Task 7: Pluggable Normalisation Framework

**Files:**
- Create: `api-gateway/internal/normalization/mapper.go` (factory pattern)
- Modify: `api-gateway/internal/normalization/canonical.go` (extend canonical event)
- Create: `api-gateway/internal/normalization/router.go` (route to correct mapper)
- Test: `api-gateway/internal/normalization/router_test.go`

**Responsibility:** Allow mappers to be registered and selected based on source tool.

- [ ] **Step 1: Create mapper interface**

```go
package normalization

type EventMapper interface {
    Map(event interface{}) (*CanonicalEvent, error)
    Accepts(sourceVendor, sourceProduct string) bool
}

type MapperRegistry struct {
    mappers map[string]EventMapper
}

func NewMapperRegistry() *MapperRegistry {
    return &MapperRegistry{
        mappers: make(map[string]EventMapper),
    }
}

func (r *MapperRegistry) Register(sourceVendor string, mapper EventMapper) {
    r.mappers[sourceVendor] = mapper
}

func (r *MapperRegistry) Map(sourceVendor string, event interface{}) (*CanonicalEvent, error) {
    mapper, ok := r.mappers[sourceVendor]
    if !ok {
        return nil, fmt.Errorf("no mapper for vendor: %s", sourceVendor)
    }
    return mapper.Map(event)
}
```

- [ ] **Step 2: Write test for mapper registry**

```go
func TestMapperRegistry_SelectsCorrectMapper(t *testing.T) {
    registry := NewMapperRegistry()
    registry.Register("codex", &CodexMapper{})
    registry.Register("cursor", &CursorMapper{})
    
    event, err := registry.Map("codex", mockCodexEvent)
    assert.NoError(t, err)
    assert.Equal(t, "codex", event.SourceTool)
}
```

- [ ] **Step 3: Implement mapper registration**

```go
func init() {
    registry := NewMapperRegistry()
    registry.Register("codex", &CodexMapper{})
    registry.Register("claude_code", &ClaudeCodeMapper{})
    registry.Register("cursor", &CursorMapper{})
    registry.Register("vscode", &VSCodeMapper{})
    registry.Register("cowork", &CoworkMapper{})
}
```

- [ ] **Step 4: Extend canonical event struct**

```go
type CanonicalEvent struct {
    // ... existing fields ...
    SourceVendor   string
    SourceProduct  string
    SourceChannel  string
    EventCategory  string  // session, model_call, tool_call, approval
    RiskCategory   string  // unsafe_command, secret_exposure, prod_edit
    Action         string
    Success        bool
    Payload        map[string]interface{}
    Redacted       bool
}
```

- [ ] **Step 5: Run tests**

Run: `cd api-gateway && go test ./internal/normalization -v`

- [ ] **Step 6: Commit**

```bash
git add api-gateway/internal/normalization/mapper.go api-gateway/internal/normalization/router.go api-gateway/internal/normalization/router_test.go
git commit -m "feat: implement pluggable mapper registry for multi-tool normalization"
```

---

### Task 8: Risk Scoring Engine

**Files:**
- Create: `api-gateway/internal/governance/risk_engine.go`
- Create: `api-gateway/internal/governance/rules.go`
- Create: `api-gateway/internal/governance/risk_engine_test.go`

**Responsibility:** Evaluate events against governance rules, compute risk scores, flag for review.

- [ ] **Step 1: Write test for risk scoring**

```go
func TestRiskEngine_FlagsUnsafeShellCommand(t *testing.T) {
    engine := NewRiskEngine()
    event := &CanonicalEvent{
        SourceVendor: "vscode",
        Action: "shell_command",
        Payload: map[string]interface{}{"command": "rm -rf /"},
    }
    
    score, category := engine.Score(event)
    assert.Greater(t, score, 80)
    assert.Equal(t, "dangerous_command", category)
}

func TestRiskEngine_FlagsSecretExposure(t *testing.T) {
    engine := NewRiskEngine()
    event := &CanonicalEvent{
        SourceVendor: "cursor",
        Payload: map[string]interface{}{"prompt": "aws key AKIA1234567890ABCDEF"},
    }
    
    score, category := engine.Score(event)
    assert.Greater(t, score, 90)
    assert.Equal(t, "secret_exposure", category)
}
```

- [ ] **Step 2: Implement risk engine**

```go
package governance

type RiskRule struct {
    Name        string
    Match       RuleMatcher
    Score       int
    Category    string
}

type RiskEngine struct {
    rules []*RiskRule
}

func NewRiskEngine() *RiskEngine {
    return &RiskEngine{
        rules: initializeDefaultRules(),
    }
}

func (e *RiskEngine) Score(event *CanonicalEvent) (int, string) {
    maxScore := 0
    maxCategory := ""
    
    for _, rule := range e.rules {
        if rule.Match(event) {
            if rule.Score > maxScore {
                maxScore = rule.Score
                maxCategory = rule.Category
            }
        }
    }
    
    return maxScore, maxCategory
}
```

- [ ] **Step 3: Define default rules**

```go
func initializeDefaultRules() []*RiskRule {
    return []*RiskRule{
        {
            Name: "dangerous_shell_command",
            Match: func(e *CanonicalEvent) bool {
                if e.Action != "shell_command" {
                    return false
                }
                cmd := e.Payload["command"].(string)
                return regexp.MustCompile(`rm -rf|mkfs|chmod 777|curl.*\|.*sh`).MatchString(cmd)
            },
            Score: 90,
            Category: "dangerous_command",
        },
        {
            Name: "secret_exposure",
            Match: func(e *CanonicalEvent) bool {
                payload := fmt.Sprintf("%v", e.Payload)
                return regexp.MustCompile(`AKIA[0-9A-Z]{16}|sk-[A-Za-z0-9]{48}|ghp_[A-Za-z0-9]`).MatchString(payload)
            },
            Score: 100,
            Category: "secret_exposure",
        },
        {
            Name: "production_file_modification",
            Match: func(e *CanonicalEvent) bool {
                if e.Action != "file_edit" {
                    return false
                }
                path := e.Payload["file_path"].(string)
                return regexp.MustCompile(`prod|terraform|k8s`).MatchString(strings.ToLower(path))
            },
            Score: 70,
            Category: "prod_edit",
        },
    }
}
```

- [ ] **Step 4: Run tests**

Run: `cd api-gateway && go test ./internal/governance -v`

- [ ] **Step 5: Commit**

```bash
git add api-gateway/internal/governance/risk_engine.go api-gateway/internal/governance/rules.go api-gateway/internal/governance/risk_engine_test.go
git commit -m "feat: implement configurable risk scoring engine with governance rules"
```

---

### Task 9: Store Integration for Unified Schema

**Files:**
- Create: `api-gateway/internal/store/ai_dev_events.go`
- Modify: `api-gateway/internal/store/store.go` (add interface methods)
- Test: `api-gateway/internal/store/ai_dev_events_test.go`

**Responsibility:** Persist canonical events to ai_dev_events table.

- [ ] **Step 1: Write test for event storage**

```go
func TestStoreCanonicalEvent(t *testing.T) {
    store := NewPostgresStore(testDB)
    event := &CanonicalEvent{
        SourceVendor: "codex",
        SourceProduct: "codex-cli",
        EventType: "tool.call.completed",
        UserEmail: "dev@company.com",
        RiskScore: 10,
        Payload: map[string]interface{}{"command": "pytest"},
    }
    
    err := store.StoreCanonicalEvent(context.Background(), event)
    assert.NoError(t, err)
    
    // Verify in DB
    stored, err := store.GetCanonicalEvent(context.Background(), event.ID)
    assert.NoError(t, err)
    assert.Equal(t, "codex", stored.SourceVendor)
}
```

- [ ] **Step 2: Implement event storage**

```go
func (s *PostgresStore) StoreCanonicalEvent(ctx context.Context, event *CanonicalEvent) error {
    query := `
        INSERT INTO ai_dev_events (
            ts, source_vendor, source_product, source_channel,
            user_id, user_email, team_id, repo, model,
            event_type, event_category, action, success,
            latency_ms, input_tokens, output_tokens,
            cache_read_tokens, cache_write_tokens,
            estimated_cost_usd, risk_score, risk_category,
            requires_review, payload, redacted
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
                  $11, $12, $13, $14, $15, $16, $17, $18,
                  $19, $20, $21, $22, $23, $24)
        RETURNING id
    `
    
    payloadJSON, _ := json.Marshal(event.Payload)
    var id string
    err := s.db.QueryRowContext(ctx, query,
        event.EventTime, event.SourceVendor, event.SourceProduct, event.SourceChannel,
        event.UserID, event.UserEmail, event.TeamID, event.RepoName, event.ModelName,
        event.EventType, event.EventCategory, event.Action, event.Success,
        event.LatencyMs, event.InputTokens, event.OutputTokens,
        event.CacheReadTokens, event.CacheWriteTokens,
        event.EstimatedCost, event.RiskScore, event.RiskCategory,
        event.RequiresReview, payloadJSON, event.Redacted,
    ).Scan(&id)
    
    if err != nil {
        return err
    }
    event.ID = id
    return nil
}
```

- [ ] **Step 3: Run tests**

Run: `cd api-gateway && go test ./internal/store -v`

- [ ] **Step 4: Commit**

```bash
git add api-gateway/internal/store/ai_dev_events.go api-gateway/internal/store/ai_dev_events_test.go
git commit -m "feat: implement canonical event persistence to unified schema"
```

---

### Task 10: Extend Redaction Processor for All Tools

**Files:**
- Modify: `collector/internal/processor/redaction.go` (support tool-agnostic redaction)
- Test: Update `collector/internal/processor/redaction_test.go`

**Responsibility:** Redact sensitive data regardless of tool source (Cursor, VSCode, etc.).

- [ ] **Step 1: Extend redaction patterns**

```go
var redactionPatterns = []*regexp.Regexp{
    // AWS credentials
    regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
    // OpenAI keys
    regexp.MustCompile(`sk-(?:proj-)?[a-zA-Z0-9-]{48,}`),
    // GitHub tokens
    regexp.MustCompile(`ghp_[A-Za-z0-9_]{36,255}`),
    // Anthropic keys
    regexp.MustCompile(`sk-ant-[a-zA-Z0-9_]{36,}`),
    // Database URLs
    regexp.MustCompile(`(?i)(?:postgres|mysql|mongodb|redis)://[^:]+:[^@]+@`),
    // Private keys
    regexp.MustCompile(`-----BEGIN (?:RSA |OPENSSH )?PRIVATE KEY-----`),
    // .env patterns
    regexp.MustCompile(`(?i)(password|passwd|secret|api_key|token|access_key|secret_key)\s*=\s*\S+`),
}

// Tool-agnostic sensitive keys that should never be stored
var sensitiveKeys = []string{
    "user.prompt", "prompt",
    "tool.arguments", "arguments",
    "request.body", "response.body",
    "file.content", "code_content",
    "command.full", // Full command including secrets
    "cursor.full_response", "vscode.full_response",
    "cowork.instructions", // Internal AI instructions
}
```

- [ ] **Step 2: Test redaction across tools**

```go
func TestRedactionAcrossTools(t *testing.T) {
    tests := []struct {
        name string
        tool string
        payload map[string]string
    }{
        {
            name: "Cursor prompt with secret",
            tool: "cursor",
            payload: map[string]string{
                "prompt": "Deploy with API key sk-ant-xyz123",
            },
        },
        {
            name: "VSCode command with password",
            tool: "vscode",
            payload: map[string]string{
                "command": "postgres://user:password@localhost/db",
            },
        },
        {
            name: "Cowork instructions with AWS key",
            tool: "cowork",
            payload: map[string]string{
                "instructions": "Use AKIA1234567890ABCDEF for AWS",
            },
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            redacted, _ := ExtractAndRedact(tt.payload, false)
            for _, v := range redacted {
                assert.NotContains(t, v, "password")
                assert.NotContains(t, v, "AKIA")
                assert.NotContains(t, v, "sk-ant-")
            }
        })
    }
}
```

- [ ] **Step 3: Commit**

```bash
git add collector/internal/processor/redaction.go collector/internal/processor/redaction_test.go
git commit -m "feat: extend redaction to cover all vendor tools and formats"
```

---

## Phase 3: Extended Tool Support

### Task 11: Cursor Telemetry Mapper

**Files:**
- Create: `api-gateway/internal/normalization/cursor_mapper.go`
- Create: `api-gateway/internal/normalization/cursor_mapper_test.go`

**Responsibility:** Map Cursor IDE events to canonical schema.

- [ ] **Step 1: Write test for Cursor mapper**

```go
func TestMapCursorSuggestionAccepted(t *testing.T) {
    event := &VSCodeExtensionEvent{
        Source: "cursor",
        EventType: "suggestion.accepted",
        Timestamp: time.Now(),
        Payload: map[string]interface{}{
            "suggestion_length": 42,
            "model": "claude-3-sonnet",
            "file_path": "/app/main.py",
            "latency_ms": 1200,
        },
    }
    
    canonical, err := MapCursorEvent(event)
    assert.NoError(t, err)
    assert.Equal(t, "cursor", canonical.SourceVendor)
    assert.Equal(t, "suggestion.accepted", canonical.EventType)
    assert.Equal(t, "claude-3-sonnet", canonical.Model)
    assert.Equal(t, int64(1200), canonical.LatencyMs)
}
```

- [ ] **Step 2: Implement Cursor mapper**

```go
func MapCursorEvent(event *VSCodeExtensionEvent) (*CanonicalEvent, error) {
    canonical := &CanonicalEvent{
        SourceVendor: "cursor",
        SourceProduct: "cursor-editor",
        SourceChannel: "extension",
        EventTime: event.Timestamp,
        EventType: event.EventType,
        Model: event.Payload["model"].(string),
        LatencyMs: event.Payload["latency_ms"].(int64),
        Payload: event.Payload,
        PromptRedacted: true,
    }
    
    // Map Cursor-specific events to canonical types
    switch event.EventType {
    case "suggestion.accepted":
        canonical.EventCategory = "tool_call"
        canonical.Action = "code_generation_accepted"
    case "suggestion.rejected":
        canonical.EventCategory = "tool_call"
        canonical.Action = "code_generation_rejected"
    case "refactor.executed":
        canonical.EventCategory = "tool_call"
        canonical.Action = "refactor"
    case "chat.started":
        canonical.EventCategory = "session"
        canonical.Action = "chat_initiated"
    }
    
    // Risk scoring for Cursor
    canonical.RiskScore = scoreCursorRisk(canonical)
    canonical.RequiresReview = canonical.RiskScore > 50
    
    return canonical, nil
}

func scoreCursorRisk(event *CanonicalEvent) int {
    score := 0
    
    // File modifications in production paths
    if filePath, ok := event.Payload["file_path"].(string); ok {
        if regexp.MustCompile(`prod|k8s|terraform`).MatchString(strings.ToLower(filePath)) {
            score += 60
        }
    }
    
    // Large code generation (potential for unreviewed changes)
    if length, ok := event.Payload["suggestion_length"].(int); ok && length > 500 {
        score += 40
    }
    
    return score
}
```

- [ ] **Step 3: Register Cursor mapper**

```go
func init() {
    registry.Register("cursor", &CursorMapper{})
}
```

- [ ] **Step 4: Run tests**

Run: `cd api-gateway && go test ./internal/normalization -run Cursor -v`

- [ ] **Step 5: Commit**

```bash
git add api-gateway/internal/normalization/cursor_mapper.go api-gateway/internal/normalization/cursor_mapper_test.go
git commit -m "feat: add Cursor IDE telemetry mapper"
```

---

### Task 12: VSCode Agent Mapper

**Files:**
- Create: `api-gateway/internal/normalization/vscode_mapper.go`
- Create: `api-gateway/internal/normalization/vscode_mapper_test.go`

**Responsibility:** Map VSCode Copilot, Continue, Roo Code, Cline events to canonical.

*(Similar structure to Task 11, covering GitHub Copilot, Continue, Roo Code, Cline)*

---

### Task 13: Cowork Agent Mapper

**Files:**
- Create: `api-gateway/internal/normalization/cowork_mapper.go`
- Create: `api-gateway/internal/normalization/cowork_mapper_test.go`

**Responsibility:** Map Cowork paired assistant events to canonical.

---

### Task 14: Anthropic API Direct Instrumentation

**Files:**
- Create: `api-gateway/internal/normalization/anthropic_api_mapper.go`
- Test: `api-gateway/internal/normalization/anthropic_api_mapper_test.go`

**Responsibility:** Capture Claude API calls directly (for enterprise Claude deployments).

---

### Task 15: Webhook & Proxy Ingestion

**Files:**
- Create: `api-gateway/internal/handlers/webhook_telemetry.go`
- Test: `api-gateway/internal/handlers/webhook_telemetry_test.go`

**Responsibility:** Generic webhook endpoint for custom integrations.

---

## Phase 4: Enterprise Features & Documentation

### Task 16: Multi-Tool Unified Dashboard

**Files:**
- Create: `portal/src/pages/AIToolsDashboard.tsx`
- Create: `portal/src/components/tools/ToolComparison.tsx`
- Create: `portal/src/components/tools/ProductivityMetrics.tsx`
- Modify: `portal/src/App.tsx` (add route)

**Responsibility:** Dashboard showing usage across all tools, cost comparison, adoption trends.

---

### Task 17: Productivity Analytics

**Files:**
- Create: `portal/src/pages/ProductivityPage.tsx`
- Create: `api-gateway/internal/analytics/productivity.go`

**Responsibility:** Track AI-driven velocity metrics:
- Suggestion acceptance rate by tool
- Time to merge (with vs without AI)
- Code quality metrics
- Adoption by team

---

### Task 18: Governance Workflows

**Files:**
- Create: `portal/src/pages/GovernancePage.tsx`
- Create: `api-gateway/internal/handlers/governance.go`

**Responsibility:** Admin UI for:
- Reviewing high-risk events
- Approving/denying actions
- Setting tool policies
- Managing redacted content access

---

### Task 19: Tool Setup Guides

**Files:**
- Create: `docs/integrations/cursor-setup.md`
- Create: `docs/integrations/vscode-agents-setup.md`
- Create: `docs/integrations/cowork-setup.md`
- Create: `docs/integrations/anthropic-api-setup.md`
- Create: `docs/integrations/webhook-setup.md`

---

### Task 20: Extended API Reference

**Files:**
- Create: `docs/api/unified-telemetry.md` (supercedes narrow API docs)
- Document query aggregations by vendor, product, team, model
- Document risk rule engine
- Document webhook contracts

---

### Task 21: Docker Compose & Final README

**Files:**
- Modify: `docker-compose.yml` (ensure all services running)
- Create: `docs/INSTALLATION.md` (step-by-step setup)
- Update: `README.md` (expand with enterprise telemetry vision)

---

## Summary of Expansion

| Aspect | Original | Expanded |
|--------|----------|----------|
| **Tools Supported** | Codex, Claude Code | Codex, Claude Code, Cursor, VSCode agents, Cowork, Anthropic API |
| **Collection Methods** | OTLP only | OTLP, VSCode extensions, webhooks, direct API |
| **Database Schema** | 3 tables (events, usage, tool_calls) | 1 unified flexible table |
| **Mappers** | 2 (Codex, Claude Code) | 6+ (Cursor, VSCode, Cowork, Anthropic, generic) |
| **Governance** | Basic risk scoring | Comprehensive rule engine + admin workflows |
| **Dashboards** | Single-tool views | Multi-tool comparison, productivity, governance |
| **Documentation** | 3 setup guides | 8 guides + comprehensive API reference |
| **Tasks** | 12 | 21 |
| **Estimated LOC** | 3,500 | 12,000+ |

---

## Execution Strategy

**Phase 1:** ✅ Already complete (Codex + Claude Code)
**Phase 2:** ~8 tasks (1-2 days) — Unified collection + governance foundation
**Phase 3:** ~6 tasks (2-3 days) — Each tool mapper is similar, parallel-able
**Phase 4:** ~7 tasks (2-3 days) — Dashboards + docs

**Total:** ~21 tasks, ~5-8 days for experienced team

---

## Key Design Decisions

1. **Single Flexible Table** — Easier to query "all AI tool usage" without joins
2. **Pluggable Mappers** — Add new tools without rewriting normalization logic
3. **Tool-Agnostic Governance** — Rules fire on event patterns, not tool-specific
4. **VSCode Extension Webhooks** — Lowest friction for IDE integration (Cursor, Continue, Roo Code, Cline)
5. **Redaction by Default** — No tool sends prompts/secrets to AgentFabric by default
6. **Operator-Friendly** — Single docker-compose.yml starts entire stack

---

## Next Steps

Ready to implement Phase 2 (Unified Collection & Governance)?

Should I update the branch with these new tasks, or would you prefer to review the expanded plan first?
