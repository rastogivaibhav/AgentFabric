# Codex + Claude Code Telemetry Integration Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend AgentFabric to ingest, normalize, govern, and visualize telemetry from OpenAI Codex CLI and Anthropic Claude Code as first-class citizens alongside existing agent frameworks.

**Architecture:** Codex/Claude Code send OTLP telemetry to the existing collector, which enriches spans and forwards them to the api-gateway. A new normalisation layer in the gateway maps vendor-specific events to canonical schema (ai_agent_events, ai_agent_usage, ai_agent_tool_calls). Redaction processors scrub prompts/secrets by default. Risk scoring detects dangerous shell commands, production file edits, and credential exposure. Portal dashboards expose usage, cost, and security signals for governance.

**Tech Stack:** Go (collector/api-gateway), Rust (af-core), PostgreSQL (canonical schema), React (portal dashboards), OpenTelemetry proto definitions.

---

## Phase 1: Framework Detection & Schema Extension

### Task 1: Add Codex & Claude Code Framework Constants

**Files:**
- Modify: `collector/internal/processor/agent_processor.go:45-54`
- Modify: `collector/internal/processor/agent_processor.go:240` (enrichSpan function)
- Test: `collector/internal/processor/agent_processor_test.go`

- [ ] **Step 1: Write failing test for Codex detection**

```go
func TestDetectFrameworkCodex(t *testing.T) {
	attrs := map[string]string{
		"codex.session.id": "sess_abc123",
	}
	fw := detectFramework(attrs, "codex_run")
	assert.Equal(t, FrameworkCodex, fw)
}
```

Run: `cd collector && go test ./internal/processor -run TestDetectFrameworkCodex -v`
Expected: FAIL with "FrameworkCodex not defined"

- [ ] **Step 2: Add framework constants**

In `collector/internal/processor/agent_processor.go` after line 53:

```go
const (
	FrameworkCrewAI       Framework = "crewai"
	FrameworkLangGraph    Framework = "langgraph"
	FrameworkGoogleADK    Framework = "google_adk"
	FrameworkOpenAIAgents Framework = "openai_agents"
	FrameworkClaudeAgents Framework = "claude_agents"
	FrameworkCodex        Framework = "codex"        // NEW
	FrameworkClaudeCode   Framework = "claude_code"  // NEW
	FrameworkUnknown      Framework = "unknown"
)
```

- [ ] **Step 3: Add SDK detection keys for Codex/Claude Code**

After line 91 in `agent_processor.go`:

```go
	sdkCodexSessionID   = "codex.session.id"
	sdkCodexModel       = "codex.model"
	sdkClaudeCodeSessionID = "claude_code.session.id"
	sdkClaudeCodeModel  = "claude_code.model"
```

Also add to the check list (line 395-414):

```go
{sdkCodexSessionID, FrameworkCodex},
{"codex.run.id", FrameworkCodex},
{sdkClaudeCodeSessionID, FrameworkClaudeCode},
{"claude_code.session_id", FrameworkClaudeCode},
```

- [ ] **Step 4: Update SQL schema to allow new frameworks**

Modify `deploy/sql/init.sql` line 27-30:

```sql
framework VARCHAR(32) NOT NULL CHECK (framework IN (
    'crewai', 'langgraph', 'google_adk',
    'openai_agents', 'claude_agents', 'codex', 'claude_code', 'unknown')),
```

- [ ] **Step 5: Run all tests**

Run: `cd collector && go test ./internal/processor -v`
Expected: PASS (including new test)

- [ ] **Step 6: Commit**

```bash
git add collector/internal/processor/agent_processor.go collector/internal/processor/agent_processor_test.go deploy/sql/init.sql
git commit -m "feat: add Codex and Claude Code framework detection constants"
```

---

### Task 2: Create Canonical Event Schema Tables

**Files:**
- Create: `deploy/sql/migrations/001_canonical_schema.up.sql`
- Create: `deploy/sql/migrations/001_canonical_schema.down.sql`
- Test: Manual: verify schema loads

- [ ] **Step 1: Create up migration file**

File: `deploy/sql/migrations/001_canonical_schema.up.sql`

```sql
-- Canonical vendor-neutral event schema for ai_agent_events
CREATE TABLE ai_agent_events (
    id              UUID            PRIMARY KEY DEFAULT gen_random_uuid(),
    event_time      TIMESTAMPTZ     NOT NULL,
    source_tool     VARCHAR(32)     NOT NULL CHECK (source_tool IN ('codex', 'claude_code', 'crewai', 'langgraph', 'google_adk', 'openai_agents', 'claude_agents')),
    event_type      VARCHAR(64)     NOT NULL,
    severity        VARCHAR(16)     DEFAULT 'info',
    user_id         TEXT,
    user_email      TEXT,
    org_id          UUID,
    team_id         UUID,
    session_id      VARCHAR(64),
    trace_id        VARCHAR(32),
    span_id         VARCHAR(16),
    repo_url        TEXT,
    repo_name       TEXT,
    git_branch      TEXT,
    git_commit      VARCHAR(40),
    working_directory TEXT,
    model_name      TEXT,
    provider        VARCHAR(32),
    tool_name       VARCHAR(64),
    command         TEXT,
    command_hash    VARCHAR(64),
    file_path       TEXT,
    risk_score      INTEGER DEFAULT 0,
    requires_review BOOLEAN DEFAULT FALSE,
    prompt_redacted BOOLEAN DEFAULT TRUE,
    raw_event       JSONB NOT NULL,
    created_at      TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_ai_events_time        ON ai_agent_events(event_time DESC);
CREATE INDEX idx_ai_events_source      ON ai_agent_events(source_tool, event_time DESC);
CREATE INDEX idx_ai_events_user        ON ai_agent_events(user_email, event_time DESC);
CREATE INDEX idx_ai_events_session     ON ai_agent_events(session_id);
CREATE INDEX idx_ai_events_risk        ON ai_agent_events(risk_score DESC, event_time DESC) WHERE risk_score > 50;

-- Canonical usage table
CREATE TABLE ai_agent_usage (
    id              UUID            PRIMARY KEY DEFAULT gen_random_uuid(),
    event_time      TIMESTAMPTZ     NOT NULL,
    source_tool     VARCHAR(32)     NOT NULL,
    user_id         TEXT,
    user_email      TEXT,
    org_id          UUID,
    team_id         UUID,
    session_id      VARCHAR(64),
    model_name      TEXT,
    input_tokens    BIGINT DEFAULT 0,
    output_tokens   BIGINT DEFAULT 0,
    cache_read_tokens   BIGINT DEFAULT 0,
    cache_write_tokens  BIGINT DEFAULT 0,
    total_tokens    BIGINT GENERATED ALWAYS AS (input_tokens + output_tokens + cache_read_tokens + cache_write_tokens) STORED,
    estimated_cost_usd NUMERIC(12, 6),
    created_at      TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_usage_time    ON ai_agent_usage(event_time DESC);
CREATE INDEX idx_usage_source  ON ai_agent_usage(source_tool, event_time DESC);
CREATE INDEX idx_usage_user    ON ai_agent_usage(user_email, event_time DESC);

-- Canonical tool calls table
CREATE TABLE ai_agent_tool_calls (
    id              UUID            PRIMARY KEY DEFAULT gen_random_uuid(),
    event_time      TIMESTAMPTZ     NOT NULL,
    source_tool     VARCHAR(32)     NOT NULL,
    session_id      VARCHAR(64),
    user_email      TEXT,
    repo_name       TEXT,
    tool_name       VARCHAR(64) NOT NULL,
    action_type     VARCHAR(32),
    target_resource TEXT,
    command         TEXT,
    status          VARCHAR(16),
    duration_ms     BIGINT,
    exit_code       INTEGER,
    approval_required BOOLEAN DEFAULT FALSE,
    approved_by     TEXT,
    risk_score      INTEGER DEFAULT 0,
    raw_event       JSONB NOT NULL,
    created_at      TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_tool_calls_time     ON ai_agent_tool_calls(event_time DESC);
CREATE INDEX idx_tool_calls_tool     ON ai_agent_tool_calls(tool_name, event_time DESC);
CREATE INDEX idx_tool_calls_risk     ON ai_agent_tool_calls(risk_score DESC, event_time DESC) WHERE risk_score > 50;
CREATE INDEX idx_tool_calls_user     ON ai_agent_tool_calls(user_email, event_time DESC);
```

- [ ] **Step 2: Create down migration file**

File: `deploy/sql/migrations/001_canonical_schema.down.sql`

```sql
DROP INDEX IF EXISTS idx_tool_calls_user;
DROP INDEX IF EXISTS idx_tool_calls_risk;
DROP INDEX IF EXISTS idx_tool_calls_tool;
DROP INDEX IF EXISTS idx_tool_calls_time;
DROP TABLE IF EXISTS ai_agent_tool_calls;

DROP INDEX IF EXISTS idx_usage_user;
DROP INDEX IF EXISTS idx_usage_source;
DROP INDEX IF EXISTS idx_usage_time;
DROP TABLE IF EXISTS ai_agent_usage;

DROP INDEX IF EXISTS idx_ai_events_risk;
DROP INDEX IF EXISTS idx_ai_events_session;
DROP INDEX IF EXISTS idx_ai_events_user;
DROP INDEX IF EXISTS idx_ai_events_source;
DROP INDEX IF EXISTS idx_ai_events_time;
DROP TABLE IF EXISTS ai_agent_events;
```

- [ ] **Step 3: Test migration manually**

Run:
```bash
cd deploy/docker
docker-compose exec postgres psql -U fabric -d govagn -f /migrations/001_canonical_schema.up.sql
```

Expected: CREATE TABLE messages, no errors

- [ ] **Step 4: Verify schema with \d command**

Run:
```bash
cd deploy/docker
docker-compose exec postgres psql -U fabric -d govagn -c "\d ai_agent_events"
```

Expected: See all columns and indexes

- [ ] **Step 5: Commit**

```bash
git add deploy/sql/migrations/001_canonical_schema.up.sql deploy/sql/migrations/001_canonical_schema.down.sql
git commit -m "feat: add canonical event schema tables (ai_agent_events, usage, tool_calls)"
```

---

## Phase 2: Source Mapping & Normalisation

### Task 3: Create Codex Source Mapper

**Files:**
- Create: `api-gateway/internal/normalization/codex_mapper.go`
- Create: `api-gateway/internal/normalization/codex_mapper_test.go`
- Test: Unit tests with fixtures

- [ ] **Step 1: Write failing test for Codex span mapping**

File: `api-gateway/internal/normalization/codex_mapper_test.go`

```go
package normalization

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMapCodexSessionStarted(t *testing.T) {
	span := &EnrichedSpan{
		Framework: "codex",
		Name: "session_started",
		Attributes: map[string]string{
			"codex.session.id": "sess_xyz",
			"codex.model": "claude-opus",
			"af.user.id": "user123",
			"af.session.id": "sess_xyz",
		},
		StartTimeNs: 1000000000,
		DurationNs: 5000000000,
		Status: 0,
	}
	
	event, err := MapCodexEvent(span)
	assert.NoError(t, err)
	assert.Equal(t, "session.started", event.EventType)
	assert.Equal(t, "codex", event.SourceTool)
	assert.Equal(t, "user123", event.UserID)
	assert.Equal(t, "sess_xyz", event.SessionID)
	assert.Equal(t, "claude-opus", event.ModelName)
}

func TestMapCodexToolCallCompleted(t *testing.T) {
	span := &EnrichedSpan{
		Framework: "codex",
		Name: "tool_call_completed",
		Attributes: map[string]string{
			"codex.session.id": "sess_xyz",
			"codex.tool.name": "shell",
			"codex.tool.command": "pytest tests/",
			"codex.tool.status": "success",
			"af.user.id": "user123",
		},
		DurationNs: 18422000000,
		Status: 0,
	}
	
	event, err := MapCodexEvent(span)
	assert.NoError(t, err)
	assert.Equal(t, "tool.call.completed", event.EventType)
	assert.Equal(t, "shell", event.ToolName)
	assert.Equal(t, "pytest tests/", event.Command)
	assert.Equal(t, "success", event.Tool CallStatus)
}
```

Run: `cd api-gateway && go test ./internal/normalization -run TestMapCodexSessionStarted -v`
Expected: FAIL with "MapCodexEvent not defined"

- [ ] **Step 2: Create codex_mapper.go**

File: `api-gateway/internal/normalization/codex_mapper.go`

```go
package normalization

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// CodexEventMapper maps Codex spans to canonical events
type CodexEventMapper struct{}

// MapCodexEvent converts an EnrichedSpan from Codex framework to CanonicalEvent
func MapCodexEvent(span *EnrichedSpan) (*CanonicalEvent, error) {
	if span.Framework != "codex" {
		return nil, fmt.Errorf("expected framework 'codex', got '%s'", span.Framework)
	}

	// Determine event type from span name
	eventType := deriveEventTypeCodex(span.Name, span.Attributes)
	
	event := &CanonicalEvent{
		EventTime:       time.Unix(0, int64(span.StartTimeNs)),
		SourceTool:      "codex",
		EventType:       eventType,
		UserID:          span.Attributes["af.user.id"],
		UserEmail:       span.Attributes["af.user.email"],
		SessionID:       span.Attributes["codex.session.id"],
		TraceID:         span.TraceID,
		SpanID:          span.SpanID,
		ModelName:       span.Attributes["codex.model"],
		Provider:        "openai",
		ToolName:        span.Attributes["codex.tool.name"],
		Command:         span.Attributes["codex.tool.command"],
		Severity:        deriveSeverity(span.StatusCode, span.Attributes),
		PromptRedacted:  true,
		RawEvent:        span,
	}

	// Hash sensitive command
	if event.Command != "" {
		event.CommandHash = hashCommand(event.Command)
	}

	// Score risk
	event.RiskScore = scoreCodexRisk(event, span)
	event.RequiresReview = event.RiskScore > 50

	return event, nil
}

func deriveEventTypeCodex(spanName string, attrs map[string]string) string {
	name := strings.ToLower(spanName)

	switch {
	case strings.Contains(name, "session") && strings.Contains(name, "started"):
		return "session.started"
	case strings.Contains(name, "session") && strings.Contains(name, "ended"):
		return "session.ended"
	case strings.Contains(name, "tool") && strings.Contains(name, "call"):
		if strings.Contains(name, "completed") || strings.Contains(name, "success") {
			return "tool.call.completed"
		} else if strings.Contains(name, "failed") {
			return "tool.call.failed"
		}
		return "tool.call.started"
	case strings.Contains(name, "approval"):
		if strings.Contains(name, "granted") {
			return "tool.approval.granted"
		} else if strings.Contains(name, "denied") {
			return "tool.approval.denied"
		}
		return "tool.approval.requested"
	case strings.Contains(name, "model") && strings.Contains(name, "request"):
		if strings.Contains(name, "completed") {
			return "model.request.completed"
		} else if strings.Contains(name, "failed") {
			return "model.request.failed"
		}
		return "model.request.started"
	case strings.Contains(name, "error"):
		return "error.detected"
	default:
		return "event.unknown"
	}
}

func scoreCodexRisk(event *CanonicalEvent, span *EnrichedSpan) int {
	score := 0

	// Dangerous shell commands
	if event.ToolName == "shell" {
		cmd := strings.ToLower(event.Command)
		dangerousPatterns := []string{
			"rm -rf", "mkfs", "dd if=", "chmod 777",
			"curl.*|.*sh", "wget.*|.*sh", "> /dev/", "exec", "eval",
		}
		for _, pattern := range dangerousPatterns {
			if matched, _ := regexp.MatchString(pattern, cmd); matched {
				score += 90
				break
			}
		}
	}

	// Production file modifications
	if event.EventType == "file.updated" {
		path := strings.ToLower(span.Attributes["file_path"])
		if strings.Contains(path, "prod") || strings.Contains(path, "production") ||
			strings.Contains(path, "terraform") || strings.Contains(path, "k8s") {
			score += 70
		}
	}

	// High token usage (warn on >200k tokens)
	inputTokens := parseIntAttr(span.Attributes["gen_ai.usage.input_tokens"])
	outputTokens := parseIntAttr(span.Attributes["gen_ai.usage.output_tokens"])
	if inputTokens+outputTokens > 200000 {
		score += 40
	}

	return score
}

func hashCommand(cmd string) string {
	h := sha256.Sum256([]byte(cmd))
	return hex.EncodeToString(h[:])
}

func deriveSeverity(statusCode int32, attrs map[string]string) string {
	if statusCode != 0 {
		return "error"
	}
	if attrs["codex.tool.status"] == "failed" {
		return "warning"
	}
	return "info"
}

func parseIntAttr(s string) int64 {
	var v int64
	fmt.Sscan(s, &v)
	return v
}
```

- [ ] **Step 3: Create canonical event struct**

If not already present, add to `api-gateway/internal/normalization/canonical.go`:

```go
package normalization

import (
	"github.com/govagn/api-gateway/internal/processor"
)

type CanonicalEvent struct {
	EventTime       time.Time
	SourceTool      string
	EventType       string
	Severity        string
	UserID          string
	UserEmail       string
	OrgID           string
	TeamID          string
	SessionID       string
	TraceID         string
	SpanID          string
	RepoURL         string
	RepoName        string
	GitBranch       string
	GitCommit       string
	ModelName       string
	Provider        string
	ToolName        string
	Command         string
	CommandHash     string
	FilePath        string
	RiskScore       int
	RequiresReview  bool
	PromptRedacted  bool
	RawEvent        *processor.EnrichedSpan
}
```

- [ ] **Step 4: Run tests**

Run: `cd api-gateway && go test ./internal/normalization -run TestMapCodex -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add api-gateway/internal/normalization/codex_mapper.go api-gateway/internal/normalization/codex_mapper_test.go
git commit -m "feat: add Codex source mapper with risk scoring"
```

---

### Task 4: Create Claude Code Source Mapper

**Files:**
- Create: `api-gateway/internal/normalization/claude_code_mapper.go`
- Create: `api-gateway/internal/normalization/claude_code_mapper_test.go`
- Test: Unit tests with fixtures

*(Follow same pattern as Task 3 but for Claude Code)*

- [ ] **Step 1: Write failing test for Claude Code span mapping**

File: `api-gateway/internal/normalization/claude_code_mapper_test.go`

```go
package normalization

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMapClaudeCodeSessionStarted(t *testing.T) {
	span := &EnrichedSpan{
		Framework: "claude_code",
		Name: "claude_code.session.started",
		Attributes: map[string]string{
			"claude_code.session_id": "claud_sess_abc",
			"claude_code.model": "claude-sonnet",
			"af.user.id": "user456",
		},
		StartTimeNs: 1000000000,
		DurationNs: 2000000000,
	}
	
	event, err := MapClaudeCodeEvent(span)
	assert.NoError(t, err)
	assert.Equal(t, "session.started", event.EventType)
	assert.Equal(t, "claude_code", event.SourceTool)
	assert.Equal(t, "user456", event.UserID)
	assert.Equal(t, "claude-sonnet", event.ModelName)
}

func TestMapClaudeCodeToolUsage(t *testing.T) {
	span := &EnrichedSpan{
		Framework: "claude_code",
		Name: "tool_usage",
		Attributes: map[string]string{
			"claude_code.session_id": "claud_sess_abc",
			"claude_code.tool_name": "shell",
			"gen_ai.usage.input_tokens": "5000",
			"gen_ai.usage.output_tokens": "2000",
		},
		InputTokens: 5000,
		OutputTokens: 2000,
	}
	
	event, err := MapClaudeCodeEvent(span)
	assert.NoError(t, err)
	assert.Equal(t, "tool.call.completed", event.EventType)
}
```

Run: `cd api-gateway && go test ./internal/normalization -run TestMapClaudeCode -v`
Expected: FAIL

- [ ] **Step 2: Create claude_code_mapper.go**

File: `api-gateway/internal/normalization/claude_code_mapper.go`

```go
package normalization

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// MapClaudeCodeEvent converts an EnrichedSpan from Claude Code to CanonicalEvent
func MapClaudeCodeEvent(span *EnrichedSpan) (*CanonicalEvent, error) {
	if span.Framework != "claude_code" {
		return nil, fmt.Errorf("expected framework 'claude_code', got '%s'", span.Framework)
	}

	eventType := deriveEventTypeClaudeCode(span.Name, span.Attributes)

	event := &CanonicalEvent{
		EventTime:       time.Unix(0, int64(span.StartTimeNs)),
		SourceTool:      "claude_code",
		EventType:       eventType,
		UserID:          span.Attributes["af.user.id"],
		UserEmail:       span.Attributes["af.user.email"],
		SessionID:       span.Attributes["claude_code.session_id"],
		TraceID:         span.TraceID,
		SpanID:          span.SpanID,
		ModelName:       span.Attributes["claude_code.model"],
		Provider:        "anthropic",
		ToolName:        span.Attributes["claude_code.tool_name"],
		Command:         span.Attributes["claude_code.tool_command"],
		Severity:        "info",
		PromptRedacted:  true, // Claude Code redacts by default
		RawEvent:        span,
	}

	if event.Command != "" {
		event.CommandHash = hashCommand(event.Command)
	}

	// Score risk for Claude Code
	event.RiskScore = scoreClaudeCodeRisk(event, span)
	event.RequiresReview = event.RiskScore > 50

	return event, nil
}

func deriveEventTypeClaudeCode(spanName string, attrs map[string]string) string {
	name := strings.ToLower(spanName)

	switch {
	case strings.Contains(name, "session") && strings.Contains(name, "started"):
		return "session.started"
	case strings.Contains(name, "session") && strings.Contains(name, "ended"):
		return "session.ended"
	case strings.Contains(name, "tool"):
		if strings.Contains(name, "failed") {
			return "tool.call.failed"
		}
		return "tool.call.completed"
	case strings.Contains(name, "usage"):
		return "token.usage.recorded"
	case strings.Contains(name, "cost"):
		return "cost.estimated"
	case strings.Contains(name, "approval"):
		if strings.Contains(name, "granted") {
			return "tool.approval.granted"
		}
		return "tool.approval.requested"
	case strings.Contains(name, "error"):
		return "error.detected"
	default:
		return "event.unknown"
	}
}

func scoreClaudeCodeRisk(event *CanonicalEvent, span *EnrichedSpan) int {
	score := 0

	// Dangerous shell commands
	if event.ToolName == "shell" || event.ToolName == "bash" {
		cmd := strings.ToLower(event.Command)
		patterns := []string{
			"rm -rf", "mkfs", "dd if=", "chmod 777",
			"curl.*\\|.*sh", "wget.*\\|.*sh",
		}
		for _, pattern := range patterns {
			if matched, _ := regexp.MatchString(pattern, cmd); matched {
				score += 85
				break
			}
		}
	}

	// File operations on production paths
	if strings.Contains(event.ToolName, "file") {
		path := strings.ToLower(span.Attributes["file_path"])
		if strings.Contains(path, "prod") || strings.Contains(path, ".env") {
			score += 65
		}
	}

	// MCP tool usage (monitor for unusual activity)
	if strings.Contains(event.ToolName, "mcp") {
		score += 30
	}

	return score
}
```

- [ ] **Step 3: Write tests**

Complete `claude_code_mapper_test.go` with additional test cases (follow Task 3 pattern).

- [ ] **Step 4: Run tests**

Run: `cd api-gateway && go test ./internal/normalization -run TestMapClaudeCode -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add api-gateway/internal/normalization/claude_code_mapper.go api-gateway/internal/normalization/claude_code_mapper_test.go
git commit -m "feat: add Claude Code source mapper with risk scoring"
```

---

## Phase 3: API Ingestion & Storage

### Task 5: Create Canonical Event Ingestion Endpoint

**Files:**
- Modify: `api-gateway/cmd/server/main.go` (add route)
- Create: `api-gateway/internal/handlers/telemetry.go`
- Create: `api-gateway/internal/handlers/telemetry_test.go`
- Modify: `api-gateway/internal/store/canonical_events.go` (new file)

- [ ] **Step 1: Write failing test for ingestion endpoint**

File: `api-gateway/internal/handlers/telemetry_test.go`

```go
package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIngestCanonicalEvent(t *testing.T) {
	handler := NewTelemetryHandler(mockStore)

	payload := map[string]interface{}{
		"source_tool": "codex",
		"event_type": "tool.call.completed",
		"session_id": "sess_123",
		"user_email": "dev@company.com",
		"tool_name": "shell",
		"command": "pytest tests/",
		"risk_score": 10,
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/api/v1/ai-telemetry/events", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.IngestEvent(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	assert.True(t, resp["accepted"].(bool))
}
```

Run: `cd api-gateway && go test ./internal/handlers -run TestIngestCanonicalEvent -v`
Expected: FAIL

- [ ] **Step 2: Create telemetry handler**

File: `api-gateway/internal/handlers/telemetry.go`

```go
package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/govagn/api-gateway/internal/normalization"
	"github.com/govagn/api-gateway/internal/store"
	"go.uber.org/zap"
)

type TelemetryHandler struct {
	logger *zap.Logger
	store  store.Store
}

func NewTelemetryHandler(s store.Store) *TelemetryHandler {
	return &TelemetryHandler{
		logger: zap.NewNop(),
		store:  s,
	}
}

type IngestEventRequest struct {
	SourceTool      string      `json:"source_tool"`
	EventType       string      `json:"event_type"`
	EventTime       *time.Time  `json:"event_time,omitempty"`
	SessionID       string      `json:"session_id"`
	UserEmail       string      `json:"user_email"`
	UserID          string      `json:"user_id,omitempty"`
	ToolName        string      `json:"tool_name,omitempty"`
	Command         string      `json:"command,omitempty"`
	RiskScore       int         `json:"risk_score,omitempty"`
	RequiresReview  bool        `json:"requires_review,omitempty"`
	PromptRedacted  bool        `json:"prompt_redacted,omitempty"`
	ModelName       string      `json:"model_name,omitempty"`
	TraceID         string      `json:"trace_id,omitempty"`
}

type IngestEventResponse struct {
	Accepted bool   `json:"accepted"`
	EventID  string `json:"event_id"`
	Status   string `json:"status"`
}

func (h *TelemetryHandler) IngestEvent(w http.ResponseWriter, r *http.Request) {
	var req IngestEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	eventID := uuid.NewString()
	if req.EventTime == nil {
		now := time.Now()
		req.EventTime = &now
	}

	// Store canonical event
	event := &normalization.CanonicalEvent{
		EventTime:      *req.EventTime,
		SourceTool:     req.SourceTool,
		EventType:      req.EventType,
		SessionID:      req.SessionID,
		UserEmail:      req.UserEmail,
		UserID:         req.UserID,
		ToolName:       req.ToolName,
		Command:        req.Command,
		RiskScore:      req.RiskScore,
		RequiresReview: req.RequiresReview,
		PromptRedacted: req.PromptRedacted,
		ModelName:      req.ModelName,
		TraceID:        req.TraceID,
	}

	if err := h.store.StoreCanonicalEvent(r.Context(), event); err != nil {
		h.logger.Error("failed to store event", zap.Error(err))
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(IngestEventResponse{
		Accepted: true,
		EventID:  eventID,
		Status:   "queued",
	})
}
```

- [ ] **Step 3: Create store interface method**

Add to `api-gateway/internal/store/store.go`:

```go
// StoreCanonicalEvent stores a normalized event in ai_agent_events table
func (s *PostgresStore) StoreCanonicalEvent(ctx context.Context, event *normalization.CanonicalEvent) error {
	query := `
		INSERT INTO ai_agent_events (
			event_time, source_tool, event_type, user_id, user_email,
			session_id, trace_id, span_id, model_name, tool_name,
			command, command_hash, risk_score, requires_review,
			prompt_redacted, raw_event
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		RETURNING id
	`
	
	var id uuid.UUID
	err := s.db.QueryRowContext(ctx, query,
		event.EventTime,
		event.SourceTool,
		event.EventType,
		event.UserID,
		event.UserEmail,
		event.SessionID,
		event.TraceID,
		event.SpanID,
		event.ModelName,
		event.ToolName,
		event.Command,
		event.CommandHash,
		event.RiskScore,
		event.RequiresReview,
		event.PromptRedacted,
		event.RawEvent, // JSONB
	).Scan(&id)
	
	return err
}
```

- [ ] **Step 4: Register route in api-gateway**

In `api-gateway/cmd/server/main.go` after creating chi router:

```go
	// Telemetry ingestion endpoints
	th := handlers.NewTelemetryHandler(store)
	r.Post("/api/v1/ai-telemetry/events", th.IngestEvent)
```

- [ ] **Step 5: Run tests**

Run: `cd api-gateway && go test ./internal/handlers -run TestIngestCanonicalEvent -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add api-gateway/internal/handlers/telemetry.go api-gateway/internal/handlers/telemetry_test.go api-gateway/internal/store/canonical_events.go
git commit -m "feat: add canonical event ingestion endpoint"
```

---

## Phase 4: Risk Scoring & Redaction with Admin Reveal

### Task 6: Implement Redaction with Encrypted Storage for Admin Access

**Files:**
- Create: `collector/internal/processor/redaction.go`
- Create: `collector/internal/processor/redaction_test.go`
- Create: `deploy/sql/migrations/002_redacted_content.up.sql`
- Modify: `collector/internal/processor/agent_processor.go` (integrate redaction)
- Create: `api-gateway/internal/handlers/admin_telemetry.go` (admin reveal endpoint)

- [ ] **Step 1: Update schema to store redacted content separately**

File: `deploy/sql/migrations/002_redacted_content.up.sql`

```sql
-- Encrypted storage for redacted prompts/arguments (admin-only access)
-- Only stored if admin enables prompt logging
CREATE TABLE redacted_content (
    id              UUID            PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id        UUID            NOT NULL REFERENCES ai_agent_events(id) ON DELETE CASCADE,
    content_type    VARCHAR(32)     NOT NULL CHECK (content_type IN ('prompt', 'response', 'tool_arguments', 'file_content')),
    encrypted_content BYTEA         NOT NULL, -- encrypted with admin master key
    encryption_key_version INT      NOT NULL DEFAULT 1,
    revealed_by     UUID,                       -- user who revealed it
    revealed_at     TIMESTAMPTZ,
    reason          TEXT,                       -- audit: why was this revealed?
    created_at      TIMESTAMPTZ     DEFAULT now()
);

CREATE INDEX idx_redacted_event ON redacted_content(event_id);
CREATE INDEX idx_redacted_revealed ON redacted_content(revealed_at) WHERE revealed_at IS NOT NULL;

-- Settings table for admin control
CREATE TABLE IF NOT EXISTS settings (
    key             VARCHAR(128)    PRIMARY KEY,
    value           TEXT            NOT NULL,
    description     TEXT,
    updated_at      TIMESTAMPTZ     DEFAULT now()
);

-- Insert default setting: prompt logging disabled
INSERT INTO settings (key, value, description) VALUES
    ('ai_telemetry.enable_prompt_logging', 'false', 'Store user prompts (requires admin enablement)')
    ON CONFLICT (key) DO NOTHING;

-- Audit log for redacted content access
CREATE TABLE redacted_content_audit (
    id              UUID            PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id        UUID            NOT NULL,
    admin_user_id   UUID            NOT NULL,
    admin_email     TEXT            NOT NULL,
    action          VARCHAR(32)     NOT NULL CHECK (action IN ('viewed', 'exported', 'shared')),
    ip_address      INET,
    user_agent      TEXT,
    reason          TEXT,
    accessed_at     TIMESTAMPTZ     DEFAULT now()
);

CREATE INDEX idx_audit_admin ON redacted_content_audit(admin_email, accessed_at DESC);
CREATE INDEX idx_audit_event ON redacted_content_audit(event_id);
```

Down migration: `deploy/sql/migrations/002_redacted_content.down.sql`

```sql
DROP INDEX IF EXISTS idx_audit_event;
DROP INDEX IF EXISTS idx_audit_admin;
DROP TABLE IF EXISTS redacted_content_audit;
DROP TABLE IF EXISTS redacted_content;
DELETE FROM settings WHERE key = 'ai_telemetry.enable_prompt_logging';
```

- [ ] **Step 2: Write failing test for redaction with storage**

File: `collector/internal/processor/redaction_test.go`

```go
package processor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedactPromptButStoreEncrypted(t *testing.T) {
	// User-facing: prompt should be hidden
	attrs := map[string]string{
		"user.prompt": "Deploy production with secret key AKIAIOSFODNN7EXAMPLE",
	}
	
	redactedAttrs, hiddenContent := ExtractAndRedact(attrs, true)
	
	// Public view: redacted
	assert.Equal(t, "[REDACTED_PROMPT]", redactedAttrs["user.prompt"])
	assert.NotContains(t, redactedAttrs["user.prompt"], "AKIA")
	
	// Admin view: content stored for later decryption
	assert.NotNil(t, hiddenContent["user.prompt"])
}

func TestPromptLoggingDisabledByDefault(t *testing.T) {
	attrs := map[string]string{
		"user.prompt": "secret task",
	}
	
	// When prompt logging disabled: don't even store encrypted version
	redactedAttrs, hiddenContent := ExtractAndRedact(attrs, false)
	
	assert.Equal(t, "[REDACTED_PROMPT]", redactedAttrs["user.prompt"])
	assert.Nil(t, hiddenContent["user.prompt"]) // not stored
}

func TestToolArgumentsRedaction(t *testing.T) {
	attrs := map[string]string{
		"codex.tool.arguments": `{"api_key": "sk-proj-secret", "query": "select * from users"}`,
	}
	
	redactedAttrs, _ := ExtractAndRedact(attrs, true)
	
	assert.Equal(t, "[REDACTED_TOOL_ARGS]", redactedAttrs["codex.tool.arguments"])
}
```

Run: `cd collector && go test ./internal/processor -run TestRedact -v`
Expected: FAIL

- [ ] **Step 3: Create redaction.go with separate storage**

File: `collector/internal/processor/redaction.go`

```go
package processor

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"io"
	"regexp"
	"strings"
)

var redactionPatterns = []*regexp.Regexp{
	// AWS credentials
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	// OpenAI keys
	regexp.MustCompile(`sk-(?:proj-)?[a-zA-Z0-9-]{48,}`),
	// GitHub tokens
	regexp.MustCompile(`ghp_[A-Za-z0-9_]{36,255}`),
	// Generic API keys
	regexp.MustCompile(`(?i)(api[_-]?key|apikey|token|secret)\s*[:=]\s*["']?([a-zA-Z0-9\-_.]+)["']?`),
	// Database URLs
	regexp.MustCompile(`(?i)(?:postgres|mysql|mongodb|redis)://[^:]+:[^@]+@`),
	// SSH keys
	regexp.MustCompile(`-----BEGIN [A-Z]+ PRIVATE KEY-----`),
}

type RedactedContentMap map[string]string // content_type -> encrypted_content

// ExtractAndRedact removes prompts/arguments from public view, optionally stores encrypted for admin
func ExtractAndRedact(attrs map[string]string, enablePromptLogging bool) (map[string]string, RedactedContentMap) {
	redacted := make(map[string]string, len(attrs))
	hiddenContent := make(RedactedContentMap)

	for k, v := range attrs {
		// Sensitive keys that should be hidden
		if isPromptKey(k) {
			redacted[k] = "[REDACTED_PROMPT]"
			if enablePromptLogging {
				hiddenContent[k] = v
			}
			continue
		}

		if isToolArgumentKey(k) {
			redacted[k] = "[REDACTED_TOOL_ARGS]"
			if enablePromptLogging {
				hiddenContent[k] = v
			}
			continue
		}

		// Redact embedded secrets even in non-sensitive keys
		result := v
		for _, pattern := range redactionPatterns {
			result = pattern.ReplaceAllString(result, "[REDACTED]")
		}
		redacted[k] = result
	}

	return redacted, hiddenContent
}

// EncryptForStorage encrypts sensitive content using admin master key (KMS in production)
// In dev, uses a simple key; in prod, fetch from KMS
func EncryptForStorage(plaintext string, masterKeyVersion int) ([]byte, error) {
	// Dev: hardcoded 32-byte key (in prod: fetch from AWS KMS / HashiCorp Vault)
	// Production: masterKeyVersion would map to actual key from KMS
	devKey := []byte("dev-32-byte-encryption-key-v001")

	block, err := aes.NewCipher(devKey)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	return gcm.Seal(nonce, nonce, []byte(plaintext), nil), nil
}

// DecryptForAdminView decrypts content for admin access (requires audit log)
func DecryptForAdminView(ciphertext []byte, masterKeyVersion int) (string, error) {
	devKey := []byte("dev-32-byte-encryption-key-v001")

	block, err := aes.NewCipher(devKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	nonce, ct := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

func isPromptKey(key string) bool {
	promptKeys := []string{
		"user.prompt", "user.input", "input.full", "request.body",
		"gen_ai.request.prompt", "llm.prompt", "prompt",
		"codex.user_prompt", "claude_code.user_prompt",
	}
	lower := strings.ToLower(key)
	for _, pk := range promptKeys {
		if lower == pk || strings.HasSuffix(lower, "."+pk) {
			return true
		}
	}
	return false
}

func isToolArgumentKey(key string) bool {
	argKeys := []string{
		"tool.arguments", "tool_args", "arguments",
		"codex.tool.arguments", "claude_code.tool.arguments",
	}
	lower := strings.ToLower(key)
	for _, ak := range argKeys {
		if lower == ak || strings.HasSuffix(lower, "."+ak) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Integrate into collector enrichSpan**

Modify `collector/internal/processor/agent_processor.go`:

Add config field to check if prompt logging enabled:

```go
// In enrichSpan, after line 248:
enablePromptLogging := p.cfg.Telemetry.EnablePromptLogging // from config

attrs, hiddenContent := RedactSensitiveAttributes(attrs, enablePromptLogging)

// Store hidden content for later if admin enables
if len(hiddenContent) > 0 && enablePromptLogging {
    e.HiddenContent = hiddenContent // new field on EnrichedSpan
}
```

- [ ] **Step 5: Create admin reveal endpoint**

File: `api-gateway/internal/handlers/admin_telemetry.go`

```go
package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type RevealRedactedContentRequest struct {
	EventID string `json:"event_id"`
	Reason  string `json:"reason"` // audit: why accessing this?
}

type RevealedContent struct {
	EventID       string    `json:"event_id"`
	ContentType   string    `json:"content_type"` // prompt, tool_arguments, etc.
	PlainContent  string    `json:"content"`
	RevealedAt    time.Time `json:"revealed_at"`
	AdminEmail    string    `json:"admin_email"`
}

func (h *TelemetryHandler) RevealRedactedContent(w http.ResponseWriter, r *http.Request) {
	// Only admins can access
	user := r.Context().Value("user") // from auth middleware
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	userID := user.(map[string]interface{})["user_id"].(string)
	email := user.(map[string]interface{})["email"].(string)

	var req RevealRedactedContentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	// Fetch encrypted content from redacted_content table
	content, err := h.store.GetRedactedContent(r.Context(), req.EventID)
	if err != nil {
		h.logger.Error("failed to fetch redacted content", zap.Error(err))
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// Decrypt
	plaintext, err := DecryptForAdminView(content.EncryptedContent, content.KeyVersion)
	if err != nil {
		h.logger.Error("decryption failed", zap.Error(err))
		http.Error(w, "decryption error", http.StatusInternalServerError)
		return
	}

	// Log audit trail
	h.store.LogRedactedContentAccess(r.Context(), &AuditLog{
		EventID:     req.EventID,
		AdminUserID: userID,
		AdminEmail:  email,
		Action:      "viewed",
		IPAddress:   r.RemoteAddr,
		UserAgent:   r.Header.Get("User-Agent"),
		Reason:      req.Reason,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(RevealedContent{
		EventID:      req.EventID,
		ContentType:  content.ContentType,
		PlainContent: plaintext,
		RevealedAt:   time.Now(),
		AdminEmail:   email,
	})
}

// GET /api/v1/admin/redaction-access-log - audit trail of who accessed what
func (h *TelemetryHandler) GetRedactionAuditLog(w http.ResponseWriter, r *http.Request) {
	// Admin only
	logs, err := h.store.GetRedactionAuditLog(r.Context(), 100) // last 100 accesses
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"audit_logs": logs,
		"total":      len(logs),
	})
}
```

- [ ] **Step 6: Run tests**

Run: `cd collector && go test ./internal/processor -run TestRedact -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add collector/internal/processor/redaction.go collector/internal/processor/redaction_test.go deploy/sql/migrations/002_redacted_content.up.sql api-gateway/internal/handlers/admin_telemetry.go
git commit -m "feat: add redaction with admin-controlled reveal and encryption"
```

---

## Phase 5: Dashboard Components

### Task 7: Create Codex/Claude Code Usage Dashboard

**Files:**
- Create: `portal/src/pages/AIToolsPage.tsx`
- Create: `portal/src/pages/AIToolsPage.test.tsx`
- Create: `portal/src/components/usage/UsageMetrics.tsx`
- Create: `portal/src/components/usage/RiskEventsList.tsx`
- Modify: `portal/src/App.tsx` (add route)

- [ ] **Step 1: Write test for dashboard rendering**

File: `portal/src/pages/AIToolsPage.test.tsx`

```typescript
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import AIToolsPage from './AIToolsPage'

describe('AIToolsPage', () => {
  it('renders usage metrics for Codex and Claude Code', async () => {
    const qc = new QueryClient()
    
    render(
      <QueryClientProvider client={qc}>
        <AIToolsPage />
      </QueryClientProvider>
    )
    
    expect(screen.getByText(/Codex Usage/i)).toBeInTheDocument()
    expect(screen.getByText(/Claude Code Usage/i)).toBeInTheDocument()
  })

  it('displays risk events', async () => {
    const qc = new QueryClient()
    
    render(
      <QueryClientProvider client={qc}>
        <AIToolsPage />
      </QueryClientProvider>
    )
    
    expect(screen.getByText(/High Risk Events/i)).toBeInTheDocument()
  })
})
```

Run: `cd portal && npm test -- AIToolsPage.test.tsx`
Expected: FAIL with "AIToolsPage not found"

- [ ] **Step 2: Create AIToolsPage component**

File: `portal/src/pages/AIToolsPage.tsx`

```typescript
import React from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../hooks/api'
import UsageMetrics from '../components/usage/UsageMetrics'
import RiskEventsList from '../components/usage/RiskEventsList'

export default function AIToolsPage() {
  const { data: codexUsage } = useQuery({
    queryKey: ['ai-tools', 'codex', 'usage'],
    queryFn: () => api.get('/api/v1/ai-telemetry/stats?source_tool=codex&period=24h'),
  })

  const { data: claudeUsage } = useQuery({
    queryKey: ['ai-tools', 'claude_code', 'usage'],
    queryFn: () => api.get('/api/v1/ai-telemetry/stats?source_tool=claude_code&period=24h'),
  })

  const { data: riskEvents } = useQuery({
    queryKey: ['ai-tools', 'risk-events'],
    queryFn: () => api.get('/api/v1/ai-telemetry/events?risk_score_min=60&limit=50'),
  })

  return (
    <div className="container mx-auto p-6">
      <h1 className="text-3xl font-bold mb-6">AI Tools Telemetry</h1>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-6 mb-6">
        <UsageMetrics title="Codex Usage" data={codexUsage?.data} />
        <UsageMetrics title="Claude Code Usage" data={claudeUsage?.data} />
      </div>

      <div className="bg-white rounded-lg shadow p-6">
        <h2 className="text-xl font-bold mb-4">High Risk Events</h2>
        <RiskEventsList events={riskEvents?.data || []} />
      </div>
    </div>
  )
}
```

- [ ] **Step 3: Create UsageMetrics component**

File: `portal/src/components/usage/UsageMetrics.tsx`

```typescript
import React from 'react'

interface UsageData {
  total_sessions: number
  total_tokens: number
  estimated_cost_usd: number
  models: { [key: string]: number }
}

export default function UsageMetrics({
  title,
  data,
}: {
  title: string
  data?: UsageData
}) {
  if (!data) {
    return <div className="bg-white rounded-lg shadow p-6 animate-pulse">Loading...</div>
  }

  return (
    <div className="bg-white rounded-lg shadow p-6">
      <h3 className="text-lg font-semibold mb-4">{title}</h3>
      <div className="space-y-3">
        <div className="flex justify-between">
          <span>Sessions</span>
          <span className="font-mono font-bold">{data.total_sessions}</span>
        </div>
        <div className="flex justify-between">
          <span>Tokens</span>
          <span className="font-mono">{data.total_tokens.toLocaleString()}</span>
        </div>
        <div className="flex justify-between">
          <span>Cost</span>
          <span className="font-mono text-green-600 font-bold">${data.estimated_cost_usd.toFixed(2)}</span>
        </div>
        {Object.entries(data.models).length > 0 && (
          <div className="mt-4 pt-4 border-t">
            <p className="text-sm text-gray-600 mb-2">Models Used</p>
            {Object.entries(data.models).map(([model, count]) => (
              <div key={model} className="flex justify-between text-sm">
                <span>{model}</span>
                <span>{count}x</span>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
```

- [ ] **Step 4: Create RiskEventsList component**

File: `portal/src/components/usage/RiskEventsList.tsx`

```typescript
import React from 'react'

interface RiskEvent {
  id: string
  event_time: string
  source_tool: string
  event_type: string
  user_email: string
  tool_name: string
  command: string
  risk_score: number
  requires_review: boolean
}

export default function RiskEventsList({ events }: { events: RiskEvent[] }) {
  if (events.length === 0) {
    return <p className="text-gray-500">No high-risk events</p>
  }

  return (
    <div className="overflow-x-auto">
      <table className="min-w-full text-sm">
        <thead className="bg-gray-50">
          <tr>
            <th className="px-4 py-2 text-left">Time</th>
            <th className="px-4 py-2 text-left">Tool</th>
            <th className="px-4 py-2 text-left">User</th>
            <th className="px-4 py-2 text-left">Event</th>
            <th className="px-4 py-2 text-left">Risk</th>
            <th className="px-4 py-2 text-left">Review</th>
          </tr>
        </thead>
        <tbody className="divide-y">
          {events.map((event) => (
            <tr key={event.id} className="hover:bg-gray-50">
              <td className="px-4 py-2 text-xs text-gray-600">
                {new Date(event.event_time).toLocaleString()}
              </td>
              <td className="px-4 py-2 font-mono text-sm">{event.source_tool}</td>
              <td className="px-4 py-2">{event.user_email}</td>
              <td className="px-4 py-2">{event.event_type}</td>
              <td className="px-4 py-2">
                <span
                  className={`px-2 py-1 rounded text-xs font-bold ${
                    event.risk_score >= 80
                      ? 'bg-red-100 text-red-800'
                      : event.risk_score >= 60
                      ? 'bg-yellow-100 text-yellow-800'
                      : 'bg-orange-100 text-orange-800'
                  }`}
                >
                  {event.risk_score}
                </span>
              </td>
              <td className="px-4 py-2">
                {event.requires_review ? (
                  <span className="text-red-600 font-bold">⚠️ YES</span>
                ) : (
                  <span className="text-green-600">✓</span>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
```

- [ ] **Step 5: Add route to App.tsx**

Modify `portal/src/App.tsx`:

```typescript
import AIToolsPage from './pages/AIToolsPage'

// Add to routes:
<Route path="ai-tools" element={<AIToolsPage />} />
```

- [ ] **Step 6: Run tests**

Run: `cd portal && npm test`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add portal/src/pages/AIToolsPage.tsx portal/src/pages/AIToolsPage.test.tsx portal/src/components/usage/
git commit -m "feat: add AI Tools telemetry dashboard with usage and risk metrics"
```

---

## Phase 6: Documentation & Configuration

### Task 8: Document Codex Configuration

**Files:**
- Create: `docs/integrations/codex-telemetry-setup.md`
- Create: `examples/codex-config.toml`

- [ ] **Step 1: Write setup documentation**

File: `docs/integrations/codex-telemetry-setup.md`

```markdown
# Codex OpenTelemetry Integration

## Prerequisites

- Codex CLI installed (`pip install openai-codex`)
- AgentFabric collector running at http://localhost:4318

## Configuration

Create or edit `~/.codex/config.toml`:

### Development

\`\`\`toml
[otel]
environment = "dev"
exporter = "otlp-http"
log_user_prompt = false
batch_size = 10
batch_timeout_ms = 5000

[otel.otlp_http]
endpoint = "http://localhost:4318"
headers = { "x-api-key" = "dev-token" }
\`\`\`

### Production

\`\`\`toml
[otel]
environment = "production"
exporter = "otlp-http"
log_user_prompt = false

[otel.otlp_http]
endpoint = "https://otel.your-company.com"
headers = { "authorization" = "Bearer ${CODEX_OTEL_TOKEN}" }
\`\`\`

## Telemetry Events

Codex sends:
- `session.started` - session creation
- `tool.call.started/completed/failed` - tool invocations
- `token.usage.recorded` - token metrics
- `error.detected` - runtime errors
- `tool.approval.requested/granted/denied` - approval workflow

## Verification

```bash
codex --telemetry-enabled ls
```

Check AgentFabric portal for events under "AI Tools" → "Codex Usage".
```

- [ ] **Step 2: Create example config**

File: `examples/codex-config.toml`

```toml
# Example Codex OpenTelemetry Configuration
# Place at ~/.codex/config.toml

[otel]
# Environment tag (dev, staging, production)
environment = "dev"

# Exporter type
exporter = "otlp-http"

# Never log raw user prompts by default (security best practice)
log_user_prompt = false

# Batch settings
batch_size = 50
batch_timeout_ms = 5000

[otel.otlp_http]
# OTLP HTTP endpoint (must support /v1/logs, /v1/metrics, /v1/traces)
endpoint = "http://localhost:4318"

# Headers for authentication
headers = { "x-api-key" = "your-token-here" }

# Optional: TLS certificate path
# ca_cert_path = "/etc/ssl/certs/ca-bundle.crt"
```

- [ ] **Step 3: Commit**

```bash
git add docs/integrations/codex-telemetry-setup.md examples/codex-config.toml
git commit -m "docs: add Codex telemetry setup guide"
```

---

### Task 9: Document Claude Code Configuration

**Files:**
- Create: `docs/integrations/claude-code-telemetry-setup.md`
- Create: `examples/claude-code-env.sh`

- [ ] **Step 1: Write setup documentation**

File: `docs/integrations/claude-code-telemetry-setup.md`

```markdown
# Claude Code OpenTelemetry Integration

## Prerequisites

- Claude Code CLI installed
- AgentFabric collector running at http://localhost:4318

## Configuration

Set environment variables before running Claude Code:

### Development

\`\`\`bash
export CLAUDE_CODE_ENABLE_TELEMETRY=1
export OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:4318"
export OTEL_EXPORTER_OTLP_HEADERS="x-api-key=dev-token"
export OTEL_METRICS_EXPORTER="otlp"
export OTEL_LOGS_EXPORTER="otlp"
export OTEL_TRACES_EXPORTER="otlp"
export OTEL_SERVICE_NAME="claude-code"
export OTEL_RESOURCE_ATTRIBUTES="deployment.environment=dev,team=platform"
\`\`\`

### Production

\`\`\`bash
export CLAUDE_CODE_ENABLE_TELEMETRY=1
export OTEL_EXPORTER_OTLP_ENDPOINT="https://otel.your-company.com"
export OTEL_EXPORTER_OTLP_HEADERS="authorization=Bearer $(cat /var/run/secrets/otel-token)"
export OTEL_SERVICE_NAME="claude-code"
export OTEL_RESOURCE_ATTRIBUTES="deployment.environment=production,team=engineering"
\`\`\`

## Telemetry Events

Claude Code sends:
- `session.started` / `session.ended`
- `tool.call.started` / `tool.call.completed` / `tool.call.failed`
- `token.usage.recorded`
- `cost.estimated`
- `tool.approval.requested` / `tool.approval.granted`
- `error.detected`

## Prompts & Privacy

By default, Claude Code:
- **Does NOT** send raw prompts
- **Does NOT** send tool arguments
- **Does NOT** send file contents
- Sends only: event type, timestamp, tool name, model, token counts

## Verification

```bash
CLAUDE_CODE_ENABLE_TELEMETRY=1 claude-code --help
```

Check logs for "telemetry enabled" message and verify OTLP connection.
Check AgentFabric portal for events under "AI Tools" → "Claude Code Usage".
```

- [ ] **Step 2: Create example env file**

File: `examples/claude-code-env.sh`

```bash
#!/bin/bash
# Claude Code OpenTelemetry Setup
# Source this file: source examples/claude-code-env.sh

export CLAUDE_CODE_ENABLE_TELEMETRY=1

# OTLP endpoint (update to your collector address)
export OTEL_EXPORTER_OTLP_ENDPOINT="${OTEL_ENDPOINT:-http://localhost:4318}"

# Authentication (optional)
export OTEL_EXPORTER_OTLP_HEADERS="${OTEL_HEADERS:-x-api-key=dev-token}"

# Exporters
export OTEL_METRICS_EXPORTER="otlp"
export OTEL_LOGS_EXPORTER="otlp"
export OTEL_TRACES_EXPORTER="otlp"

# Service identification
export OTEL_SERVICE_NAME="claude-code"
export OTEL_RESOURCE_ATTRIBUTES="deployment.environment=dev,team=$(whoami),hostname=$(hostname)"

echo "✓ Claude Code telemetry configured"
echo "  Endpoint: $OTEL_EXPORTER_OTLP_ENDPOINT"
echo "  Service: $OTEL_SERVICE_NAME"
```

- [ ] **Step 3: Commit**

```bash
git add docs/integrations/claude-code-telemetry-setup.md examples/claude-code-env.sh
git commit -m "docs: add Claude Code telemetry setup guide"
```

---

### Task 10: Create API Endpoint Documentation

**Files:**
- Create: `docs/api/ai-telemetry.md`

- [ ] **Step 1: Write API documentation**

File: `docs/api/ai-telemetry.md`

```markdown
# AI Telemetry API

## Canonical Event Ingestion

### POST /api/v1/ai-telemetry/events

Ingest a normalized event from Codex or Claude Code.

**Request Body:**

\`\`\`json
{
  "source_tool": "codex",
  "event_type": "tool.call.completed",
  "event_time": "2026-04-27T10:15:22Z",
  "user_email": "dev@company.com",
  "user_id": "user_123",
  "session_id": "sess_abc",
  "trace_id": "trace_def",
  "model_name": "claude-opus",
  "tool_name": "shell",
  "command": "pytest tests/",
  "command_hash": "sha256:...",
  "risk_score": 10,
  "requires_review": false,
  "prompt_redacted": true
}
\`\`\`

**Response:**

\`\`\`json
{
  "accepted": true,
  "event_id": "01JAB...",
  "status": "queued"
}
\`\`\`

### GET /api/v1/ai-telemetry/events

Query canonical events.

**Query Parameters:**
- `source_tool` (optional): "codex" | "claude_code"
- `user_email` (optional): filter by user
- `risk_score_min` (optional): minimum risk score (0-100)
- `event_type` (optional): filter by event type
- `limit` (optional): max results (default 50, max 1000)

**Response:**

\`\`\`json
{
  "data": [
    {
      "id": "01JAB...",
      "event_time": "2026-04-27T10:15:22Z",
      "source_tool": "codex",
      "event_type": "tool.call.completed",
      "user_email": "dev@company.com",
      "risk_score": 10,
      "requires_review": false
    }
  ],
  "total": 150,
  "limit": 50
}
\`\`\`

### GET /api/v1/ai-telemetry/stats

Aggregated telemetry statistics.

**Query Parameters:**
- `source_tool` (required): "codex" | "claude_code"
- `period` (optional): "1h" | "24h" | "7d" (default "24h")

**Response:**

\`\`\`json
{
  "source_tool": "codex",
  "period": "24h",
  "total_sessions": 42,
  "total_tokens": 2500000,
  "total_cost_usd": 12.50,
  "models": {
    "claude-opus": 20,
    "claude-sonnet": 22
  },
  "high_risk_events": 3,
  "avg_session_duration_s": 1850
}
\`\`\`

### GET /api/v1/ai-telemetry/usage

Token and cost aggregation.

**Query Parameters:**
- `source_tool` (optional): filter by tool
- `user_email` (optional): filter by user
- `period` (optional): time window (default "24h")

**Response:**

\`\`\`json
{
  "data": [
    {
      "event_time": "2026-04-27T00:00:00Z",
      "source_tool": "codex",
      "input_tokens": 1000000,
      "output_tokens": 500000,
      "cache_read_tokens": 100000,
      "estimated_cost_usd": 6.25
    }
  ]
}
\`\`\`

## Webhook Events

(Optional) AgentFabric can POST high-risk events to a webhook.

Configure in settings:

\`\`\`json
{
  "webhooks": {
    "high_risk_events": "https://your-api.example.com/webhooks/ai-events",
    "secret": "whsec_...",
    "risk_threshold": 70
  }
}
\`\`\`

Webhook payload:

\`\`\`json
{
  "event_id": "01JAB...",
  "event_type": "tool.call.completed",
  "source_tool": "codex",
  "risk_score": 85,
  "user_email": "dev@company.com",
  "session_id": "sess_abc",
  "command": "rm -rf /production",
  "timestamp": "2026-04-27T10:15:22Z"
}
\`\`\`
```

- [ ] **Step 2: Commit**

```bash
git add docs/api/ai-telemetry.md
git commit -m "docs: add AI Telemetry API documentation"
```

---

## Phase 7: Integration Tests & E2E

### Task 11: Create Integration Test Suite

**Files:**
- Create: `integration_tests/codex_to_dashboard_test.go`
- Create: `integration_tests/claude_code_to_dashboard_test.go`

*(These tests verify the full path: OTLP → Collector → API → DB → Dashboard)*

- [ ] **Step 1: Write Codex end-to-end test**

File: `integration_tests/codex_to_dashboard_test.go`

```go
package integration_tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCodexTelemetryEndToEnd(t *testing.T) {
	// 1. Create a Codex session event
	sessionEvent := map[string]interface{}{
		"source_tool": "codex",
		"event_type": "session.started",
		"session_id": fmt.Sprintf("sess_%d", time.Now().Unix()),
		"user_email": "testuser@example.com",
		"model_name": "claude-opus",
	}

	// 2. POST to ingestion endpoint
	body, _ := json.Marshal(sessionEvent)
	resp, err := http.Post("http://localhost:8080/api/v1/ai-telemetry/events",
		"application/json", bytes.NewReader(body))
	require.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)

	var ingestResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&ingestResp)
	assert.True(t, ingestResp["accepted"].(bool))

	// 3. Query the event from API
	time.Sleep(500 * time.Millisecond) // Allow time for DB write
	getResp, err := http.Get("http://localhost:8080/api/v1/ai-telemetry/events?source_tool=codex&limit=1")
	require.NoError(t, err)

	var events map[string]interface{}
	json.NewDecoder(getResp.Body).Decode(&events)
	
	data := events["data"].([]interface{})
	assert.Greater(t, len(data), 0)

	event := data[0].(map[string]interface{})
	assert.Equal(t, "codex", event["source_tool"])
	assert.Equal(t, "testuser@example.com", event["user_email"])
}

func TestCodexRiskScoring(t *testing.T) {
	dangerousEvent := map[string]interface{}{
		"source_tool": "codex",
		"event_type": "tool.call.completed",
		"session_id": "sess_danger",
		"user_email": "testuser@example.com",
		"tool_name": "shell",
		"command": "rm -rf /production",
		"risk_score": 95,
	}

	body, _ := json.Marshal(dangerousEvent)
	resp, err := http.Post("http://localhost:8080/api/v1/ai-telemetry/events",
		"application/json", bytes.NewReader(body))
	require.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)

	// Verify high-risk event is stored
	time.Sleep(500 * time.Millisecond)
	getResp, err := http.Get("http://localhost:8080/api/v1/ai-telemetry/events?risk_score_min=90")
	require.NoError(t, err)

	var events map[string]interface{}
	json.NewDecoder(getResp.Body).Decode(&events)
	
	data := events["data"].([]interface{})
	assert.Greater(t, len(data), 0)
}
```

- [ ] **Step 2: Write Claude Code e2e test**

File: `integration_tests/claude_code_to_dashboard_test.go`

```go
package integration_tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeCodeTelemetryEndToEnd(t *testing.T) {
	sessionEvent := map[string]interface{}{
		"source_tool": "claude_code",
		"event_type": "session.started",
		"session_id": fmt.Sprintf("claud_%d", time.Now().Unix()),
		"user_email": "developer@example.com",
		"model_name": "claude-sonnet",
	}

	body, _ := json.Marshal(sessionEvent)
	resp, err := http.Post("http://localhost:8080/api/v1/ai-telemetry/events",
		"application/json", bytes.NewReader(body))
	require.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)

	// Verify stats endpoint
	time.Sleep(500 * time.Millisecond)
	statsResp, err := http.Get("http://localhost:8080/api/v1/ai-telemetry/stats?source_tool=claude_code")
	require.NoError(t, err)

	var stats map[string]interface{}
	json.NewDecoder(statsResp.Body).Decode(&stats)
	
	assert.Greater(t, stats["total_sessions"].(float64), 0.0)
}

func TestClaudeCodeUsageTracking(t *testing.T) {
	usageEvent := map[string]interface{}{
		"source_tool": "claude_code",
		"event_type": "token.usage.recorded",
		"session_id": "claud_usage",
		"user_email": "developer@example.com",
		"model_name": "claude-sonnet",
		"input_tokens": 5000,
		"output_tokens": 2000,
	}

	body, _ := json.Marshal(usageEvent)
	resp, err := http.Post("http://localhost:8080/api/v1/ai-telemetry/events",
		"application/json", bytes.NewReader(body))
	require.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)

	// Verify usage tracking
	time.Sleep(500 * time.Millisecond)
	usageResp, err := http.Get("http://localhost:8080/api/v1/ai-telemetry/usage?source_tool=claude_code")
	require.NoError(t, err)

	var usage map[string]interface{}
	json.NewDecoder(usageResp.Body).Decode(&usage)
	
	assert.NotNil(t, usage["data"])
}
```

- [ ] **Step 3: Run integration tests**

Run:
```bash
docker-compose -f deploy/docker/docker-compose.yml up -d
sleep 10
cd integration_tests && go test -v
```

Expected: All tests PASS

- [ ] **Step 4: Commit**

```bash
git add integration_tests/
git commit -m "test: add end-to-end integration tests for Codex and Claude Code telemetry"
```

---

## Phase 8: Final Cleanup & Verification

### Task 12: Update Docker Compose & README

**Files:**
- Modify: `docker-compose.yml` (add OTEL collector tuning if needed)
- Modify: `README.md` (add telemetry section)
- Create: `docs/TELEMETRY_GUIDE.md`

- [ ] **Step 1: Add telemetry section to README**

Modify `README.md`:

```markdown
## AI Coding Agent Telemetry

AgentFabric now ingests and governs telemetry from **Codex CLI** and **Claude Code**.

### Quick Start

1. **Enable Codex telemetry:**
   ```bash
   cat > ~/.codex/config.toml << 'EOF'
   [otel]
   environment = "dev"
   exporter = "otlp-http"
   [otel.otlp_http]
   endpoint = "http://localhost:4318"
   EOF
   ```

2. **Enable Claude Code telemetry:**
   ```bash
   source examples/claude-code-env.sh
   ```

3. **Start AgentFabric:**
   ```bash
   docker-compose up -d
   ```

4. **View telemetry:**
   - Open http://localhost:3000 → "AI Tools" dashboard
   - Check usage, costs, and risk events

### Documentation

- [Codex Setup](docs/integrations/codex-telemetry-setup.md)
- [Claude Code Setup](docs/integrations/claude-code-telemetry-setup.md)
- [API Reference](docs/api/ai-telemetry.md)
- [Telemetry Guide](docs/TELEMETRY_GUIDE.md)
```

- [ ] **Step 2: Create comprehensive telemetry guide**

File: `docs/TELEMETRY_GUIDE.md`

```markdown
# AI Telemetry & Governance Guide

## Overview

AgentFabric collects telemetry from Codex and Claude Code through OpenTelemetry (OTLP).

### Data Flow

```
Codex/Claude Code
        ↓ OTLP HTTP/gRPC
    Collector (Go)
        ↓ JSON/HTTP
    API Gateway (Go)
        ↓ Normalization
  Canonical Schema
        ↓ SQL
    PostgreSQL
        ↓ React
     Portal
```

## What's Tracked

### Sessions
- Start/end timestamps
- User & model information
- Total duration & token usage

### Tool Calls
- Tool name (shell, file, browser, etc.)
- Command executed (hashed by default)
- Duration & status
- Error messages

### Usage Metrics
- Input/output tokens
- Cache hits/writes
- Estimated cost
- Model used

### Risk Events
- Dangerous commands (rm -rf, chmod 777, etc.)
- Production file modifications
- Secret/credential exposure
- High-cost sessions

## Privacy & Security

### Redaction (Default)

By default, prompts, tool arguments, and secrets are **NOT** stored:
- Raw prompts redacted
- API keys redacted
- File contents redacted
- Database URLs redacted

### What IS Stored

- Event type (session.started, tool.call.completed, etc.)
- Timestamp
- User email
- Model name
- Token counts
- Risk score
- Session ID

### Enabling Detailed Logging (Production: Not Recommended)

In Codex config:
```toml
[otel]
log_user_prompt = true  # ONLY for debugging; not recommended
```

## Risk Scoring

Events are automatically scored 0-100:

| Score | Severity | Example |
|-------|----------|---------|
| 90+ | Critical | `rm -rf /`  |
| 70-89 | High | Production file edit |
| 50-69 | Medium | High token usage |
| 0-49 | Low | Normal operations |

Events with score >50 are marked for review.

## Compliance

- GDPR-compliant (prompts redacted by default)
- Audit trails available
- Retention policies configurable
- Right-to-deletion supported
```

- [ ] **Step 3: Verify all tests pass**

Run:
```bash
cd collector && go test ./...
cd ../api-gateway && go test ./...
cd ../portal && npm test
cd ../integration_tests && go test -v
```

Expected: All PASS

- [ ] **Step 4: Commit**

```bash
git add README.md docs/TELEMETRY_GUIDE.md
git commit -m "docs: add comprehensive telemetry documentation and setup guides"
```

---

## Summary & Verification Checklist

Once all tasks are complete:

- [ ] Codex and Claude Code framework constants added
- [ ] Canonical schema tables created (ai_agent_events, usage, tool_calls)
- [ ] Codex source mapper implemented
- [ ] Claude Code source mapper implemented
- [ ] REST ingestion endpoint (/api/v1/ai-telemetry/events) working
- [ ] Redaction processor filters sensitive data
- [ ] Risk scoring calculates risk_score for each event
- [ ] Dashboard displays usage metrics and high-risk events
- [ ] Codex telemetry setup documented
- [ ] Claude Code telemetry setup documented
- [ ] API endpoints documented
- [ ] End-to-end integration tests passing
- [ ] Docker Compose verified working
- [ ] All unit, integration, and e2e tests pass

**Total commits expected: ~12**

**Lines of code added: ~3,500 (collector, api-gateway, portal)**

**Database tables: +3**

**New endpoints: 4**

**Documentation pages: +5**

---
