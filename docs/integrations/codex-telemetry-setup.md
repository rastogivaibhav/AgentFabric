# Codex OpenTelemetry Integration

## Overview

AgentFabric now ingests telemetry from OpenAI Codex CLI via OpenTelemetry (OTLP). This integration enables:
- **Observability**: Track session duration, model usage, tool calls, and errors
- **Governance**: Flag dangerous commands, monitor production changes, detect secrets
- **Cost Monitoring**: Measure token usage and estimated costs by user/project
- **Compliance**: Audit trail with redaction enabled by default

## Prerequisites

- Codex CLI installed: `pip install openai-codex`
- AgentFabric collector running (port 4318 for OTLP HTTP)
- Network access from Codex client to collector

## Quick Start

### Step 1: Configure Codex

Create or edit `~/.codex/config.toml`:

**Development (local collector):**
```toml
[otel]
environment = "dev"
exporter = "otlp-http"
log_user_prompt = false

[otel.otlp_http]
endpoint = "http://localhost:4318"
headers = { "x-api-key" = "dev-token" }
```

**Production (remote collector):**
```toml
[otel]
environment = "production"
exporter = "otlp-http"
log_user_prompt = false
batch_size = 50
batch_timeout_ms = 5000

[otel.otlp_http]
endpoint = "https://otel.your-company.com"
headers = { "authorization" = "Bearer ${CODEX_OTEL_TOKEN}" }
```

### Step 2: Set Environment Variables

```bash
export CODEX_OTEL_TOKEN="your-api-token-here"
```

### Step 3: Start Codex with Telemetry

```bash
codex --telemetry-enabled ls
```

You should see telemetry enabled confirmation in the logs.

## Telemetry Events

Codex emits the following events:

| Event | When | What's Tracked |
|-------|------|-----------------|
| `session.started` | Session begins | Session ID, user, model, environment |
| `session.ended` | Session completes | Duration, token totals, status |
| `tool.call.started` | Tool invoked | Tool name, command preview |
| `tool.call.completed` | Tool finishes | Duration, exit code, status |
| `tool.call.failed` | Tool fails | Error type, error message |
| `tool.approval.requested` | Dangerous action | Command, risk score, user decision |
| `tool.approval.granted` | User approved | Approver ID, timestamp |
| `token.usage.recorded` | Token counted | Input/output/cache tokens |
| `error.detected` | Exception | Error type, stack trace (redacted) |

## Privacy & Redaction

**By default, Codex telemetry:**
- ✓ Does NOT send raw user prompts
- ✓ Does NOT send tool arguments
- ✓ Does NOT send file contents
- ✓ Sends only: event type, tool name, model, token counts, timestamps

**Stored securely:**
- Session ID (for correlation)
- Model name
- Tool name (e.g., "shell", "file", "browser")
- Command category/hash (not full command)
- Token counts
- Risk score & governance decisions

## Risk Scoring

Events are automatically assigned risk scores (0-100):

| Pattern | Score | Action |
|---------|-------|--------|
| `rm -rf`, `mkfs`, `dd if=` | 95+ | Blocked + review required |
| `chmod 777`, `> /dev/null` | 85+ | High risk flag |
| Production file edit | 70-80 | Medium risk flag |
| High token usage (>200k) | 40-60 | Cost warning |

Events with score > 50 require review before execution in strict mode.

## Verification

After configuration:

```bash
# Check Codex sees telemetry config
codex config show

# Run a simple command with telemetry
codex --telemetry-enabled "ls -la"

# Check AgentFabric portal
# Open http://localhost:3000 → "AI Tools" → "Codex Usage"
```

## Troubleshooting

**Telemetry not appearing:**
1. Check `codex config show` includes `[otel]` section
2. Verify collector is running: `curl http://localhost:4318/status`
3. Check firewall: port 4318 must be accessible from Codex client
4. Review collector logs for auth errors

**Authentication failed:**
```bash
# Check token in headers
curl -H "x-api-key: dev-token" http://localhost:4318/v1/logs
```

**High latency:**
- Increase batch_timeout_ms in config
- Reduce batch_size for lower memory usage
- Use gRPC protocol for lower overhead

## Advanced Configuration

### Enable Prompt Logging (Admin Only)

**Not recommended for production.** Only when explicitly needed for debugging:

```toml
[otel]
log_user_prompt = true  # WARNING: stores user prompts encrypted
```

Requires admin to decrypt and audit access. All access is logged to `redacted_content_audit` table.

### Custom Headers

```toml
[otel.otlp_http]
headers = {
  "authorization" = "Bearer ${CODEX_OTEL_TOKEN}",
  "x-client-id" = "codex-prod-1",
  "x-region" = "us-west-2"
}
```

### TLS Certificate

```toml
[otel.otlp_http]
endpoint = "https://otel.secure.example.com"
ca_cert_path = "/etc/ssl/certs/company-ca.crt"
```

## Dashboard Access

Log into AgentFabric portal to view:

- **Usage Dashboard** — Sessions, tokens, costs by time period
- **Risk Events** — Commands flagged for review with scores
- **User Activity** — Sessions per user, models used, tool breakdown
- **Cost Analysis** — Spend by user, model, time period
- **Audit Trail** — Admin access to redacted content (if enabled)

## Support

Issues or questions?
- Check collector logs: `docker-compose logs collector`
- Verify schema: `docker-compose exec postgres psql -U fabric -d govagn -d "SELECT * FROM ai_agent_events LIMIT 1;"`
- Contact platform team with session ID for investigation
