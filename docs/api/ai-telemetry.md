# AI Telemetry API Reference

Base URL: `http://localhost:8080` (or your AgentFabric deployment)

All endpoints require authentication via JWT or API token in `Authorization` header.

---

## POST /api/v1/ai-telemetry/events

Ingest a canonical event from Codex, Claude Code, or other AI tools.

### Request

```http
POST /api/v1/ai-telemetry/events
Authorization: Bearer YOUR_API_TOKEN
Content-Type: application/json
```

**Body:**

```json
{
  "source_tool": "codex",
  "event_type": "tool.call.completed",
  "event_time": "2026-04-27T10:15:22Z",
  "user_email": "dev@company.com",
  "user_id": "user_123",
  "session_id": "sess_abc",
  "trace_id": "trace_def123",
  "model_name": "claude-opus",
  "tool_name": "shell",
  "command": "pytest tests/",
  "command_hash": "sha256:...",
  "risk_score": 10,
  "requires_review": false,
  "prompt_redacted": true
}
```

### Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `source_tool` | string | Yes | `codex`, `claude_code`, `crewai`, etc. |
| `event_type` | string | Yes | Event type (see taxonomy below) |
| `event_time` | ISO 8601 | Yes | When event occurred (UTC) |
| `user_email` | string | Yes | User's email address |
| `user_id` | string | No | Internal user ID |
| `session_id` | string | Yes | Unique session identifier |
| `model_name` | string | No | Model used (e.g., "claude-opus", "gpt-4o") |
| `tool_name` | string | No | Tool invoked (e.g., "shell", "file", "browser") |
| `command` | string | No | Command/action taken (stored hashed) |
| `risk_score` | integer | No | Risk score 0-100 (default: 0) |
| `requires_review` | boolean | No | Flag for manual review (default: false) |
| `prompt_redacted` | boolean | No | Whether prompt is redacted (default: true) |

### Response

**201 Created**

```json
{
  "accepted": true,
  "event_id": "01JAB1234567890ABCDEFGHIJ",
  "status": "queued"
}
```

**400 Bad Request**

```json
{
  "error": "missing required field: event_type"
}
```

**401 Unauthorized**

```json
{
  "error": "invalid or missing authentication"
}
```

---

## GET /api/v1/ai-telemetry/events

Query canonical events with filtering and pagination.

### Request

```http
GET /api/v1/ai-telemetry/events?source_tool=codex&user_email=dev@company.com&risk_score_min=50&limit=50
Authorization: Bearer YOUR_API_TOKEN
```

### Query Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `source_tool` | string | — | Filter by tool: `codex`, `claude_code`, etc. |
| `event_type` | string | — | Filter by event type (e.g., `tool.call.completed`) |
| `user_email` | string | — | Filter by user |
| `session_id` | string | — | Filter by session |
| `risk_score_min` | integer | 0 | Minimum risk score (0-100) |
| `risk_score_max` | integer | 100 | Maximum risk score |
| `requires_review` | boolean | — | Show only events flagged for review |
| `start_time` | ISO 8601 | 24h ago | Start of time range |
| `end_time` | ISO 8601 | now | End of time range |
| `limit` | integer | 50 | Max results (max 1000) |
| `offset` | integer | 0 | Pagination offset |

### Response

**200 OK**

```json
{
  "data": [
    {
      "id": "01JAB...",
      "event_time": "2026-04-27T10:15:22Z",
      "source_tool": "codex",
      "event_type": "tool.call.completed",
      "user_email": "dev@company.com",
      "session_id": "sess_abc",
      "model_name": "claude-opus",
      "tool_name": "shell",
      "risk_score": 10,
      "requires_review": false,
      "created_at": "2026-04-27T10:15:25Z"
    }
  ],
  "total": 150,
  "limit": 50,
  "offset": 0
}
```

---

## GET /api/v1/ai-telemetry/stats

Aggregated statistics for a specific tool.

### Request

```http
GET /api/v1/ai-telemetry/stats?source_tool=codex&period=24h
Authorization: Bearer YOUR_API_TOKEN
```

### Query Parameters

| Parameter | Required | Options | Description |
|-----------|----------|---------|-------------|
| `source_tool` | Yes | `codex`, `claude_code`, etc. | Tool to analyze |
| `period` | No | `1h`, `24h`, `7d`, `30d` | Time period (default: `24h`) |

### Response

**200 OK**

```json
{
  "source_tool": "codex",
  "period": "24h",
  "period_start": "2026-04-26T10:15:22Z",
  "period_end": "2026-04-27T10:15:22Z",
  "total_sessions": 42,
  "total_tokens": 2500000,
  "total_cost_usd": 12.50,
  "models": {
    "claude-opus": 20,
    "claude-sonnet": 22
  },
  "high_risk_events": 3,
  "avg_session_duration_s": 1850,
  "unique_users": 8,
  "top_tools": {
    "shell": 95,
    "file": 24,
    "browser": 8
  }
}
```

---

## GET /api/v1/ai-telemetry/usage

Token and cost breakdown by source tool, user, model, or time.

### Request

```http
GET /api/v1/ai-telemetry/usage?source_tool=codex&period=7d&group_by=user
Authorization: Bearer YOUR_API_TOKEN
```

### Query Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `source_tool` | string | — | Filter by tool |
| `user_email` | string | — | Filter by user |
| `model_name` | string | — | Filter by model |
| `period` | string | `24h` | Time window |
| `group_by` | string | — | Aggregate by: `user`, `model`, `source_tool` |
| `granularity` | string | `hour` | Time bucket: `hour`, `day`, `week` |

### Response

**200 OK**

```json
{
  "data": [
    {
      "period": "2026-04-27T00:00:00Z",
      "source_tool": "codex",
      "user_email": "dev@company.com",
      "model_name": "claude-opus",
      "input_tokens": 1000000,
      "output_tokens": 500000,
      "cache_read_tokens": 100000,
      "cache_write_tokens": 50000,
      "total_tokens": 1650000,
      "estimated_cost_usd": 6.25,
      "session_count": 8
    }
  ],
  "total_cost_usd": 87.50,
  "total_tokens": 18750000
}
```

---

## Event Type Taxonomy

Standardized event types emitted by all AI tools:

```
Session Events:
  session.started       — AI tool session begins
  session.ended         — AI tool session ends

Model/LLM Events:
  model.request.started — API call to LLM begins
  model.request.completed — LLM response received
  model.request.failed  — LLM call failed (error, timeout, etc.)

Tool Call Events:
  tool.call.started     — Tool invocation begins
  tool.call.completed   — Tool finishes successfully
  tool.call.failed      — Tool execution fails

Approval/Governance Events:
  tool.approval.requested — High-risk action needs approval
  tool.approval.granted — User approved the action
  tool.approval.denied  — User rejected the action

Token/Cost Events:
  token.usage.recorded  — Token usage metrics
  cost.estimated        — Cost calculation

File Events:
  file.read             — File opened for reading
  file.created          — New file created
  file.updated          — File modified
  file.deleted          — File removed

Git Events:
  git.diff.generated    — Git diff created

Shell/Execution Events:
  shell.command.started — Shell command begins
  shell.command.completed — Shell command succeeds
  shell.command.failed  — Shell command fails

Error/Risk Events:
  policy.violation.detected — Governance policy triggered
  risk.detected         — Risk scoring flagged event
  error.detected        — Exception or error occurred

Trace Events:
  trace.span.started    — Distributed trace span begins
  trace.span.completed  — Distributed trace span ends
```

---

## Error Responses

### 400 Bad Request

```json
{
  "error": "invalid parameter: risk_score_min must be 0-100",
  "details": { "field": "risk_score_min", "value": "150" }
}
```

### 401 Unauthorized

```json
{
  "error": "missing authorization header"
}
```

### 403 Forbidden

```json
{
  "error": "insufficient permissions",
  "details": { "required_role": "admin", "your_role": "user" }
}
```

### 404 Not Found

```json
{
  "error": "event not found",
  "event_id": "01JAB..."
}
```

### 429 Too Many Requests

```json
{
  "error": "rate limit exceeded",
  "retry_after_seconds": 60
}
```

### 500 Internal Server Error

```json
{
  "error": "internal server error",
  "request_id": "req_12345..."
}
```

---

## Admin Endpoints

### POST /api/v1/admin/redaction/reveal

Decrypt and reveal redacted content (admin only, audit logged).

**Request:**

```json
{
  "event_id": "01JAB...",
  "reason": "Security incident investigation"
}
```

**Response:**

```json
{
  "event_id": "01JAB...",
  "content_type": "prompt",
  "content": "Build a function to upload files to S3",
  "revealed_at": "2026-04-27T10:20:00Z",
  "admin_email": "admin@company.com"
}
```

### GET /api/v1/admin/redaction-audit

View audit log of all redacted content access.

**Request:**

```http
GET /api/v1/admin/redaction-audit?admin_email=admin@company.com&limit=100
Authorization: Bearer ADMIN_TOKEN
```

**Response:**

```json
{
  "audit_logs": [
    {
      "id": "audit_...",
      "event_id": "01JAB...",
      "admin_email": "admin@company.com",
      "action": "viewed",
      "reason": "Security audit",
      "accessed_at": "2026-04-27T10:20:00Z",
      "ip_address": "203.0.113.42"
    }
  ],
  "total": 42
}
```

---

## Rate Limiting

- **User endpoints:** 1,000 requests/hour
- **Admin endpoints:** 100 requests/hour
- **Ingestion endpoints:** 10,000 requests/hour

Rate limit headers:
```
X-RateLimit-Limit: 1000
X-RateLimit-Remaining: 999
X-RateLimit-Reset: 1234567890
```

---

## Examples

### Count high-risk events by user

```bash
curl -H "Authorization: Bearer $API_TOKEN" \
  'http://localhost:8080/api/v1/ai-telemetry/events?risk_score_min=60&limit=1000' \
  | jq 'group_by(.user_email) | map({user: .[0].user_email, count: length})'
```

### Get daily cost trend for Codex

```bash
curl -H "Authorization: Bearer $API_TOKEN" \
  'http://localhost:8080/api/v1/ai-telemetry/usage?source_tool=codex&period=7d&granularity=day' \
  | jq '.data | map({day: .period, cost: .estimated_cost_usd})'
```

### Find dangerous commands

```bash
curl -H "Authorization: Bearer $API_TOKEN" \
  'http://localhost:8080/api/v1/ai-telemetry/events?risk_score_min=80&event_type=tool.call.completed' \
  | jq '.data[] | {user: .user_email, tool: .tool_name, risk: .risk_score}'
```

---

## See Also

- [Codex Setup](../integrations/codex-telemetry-setup.md)
- [Claude Code Setup](../integrations/claude-code-telemetry-setup.md)
- [Telemetry Guide](../TELEMETRY_GUIDE.md)
