# Claude Code OpenTelemetry Integration

## Overview

AgentFabric ingests telemetry from Anthropic Claude Code via OpenTelemetry (OTLP). This integration enables complete observability and governance of Claude Code usage across your organization.

**Capabilities:**
- **Session Tracking** — Duration, models, cost per session
- **Tool Monitoring** — Track file operations, shell commands, MCP usage
- **Token Accounting** — Input/output/cache token tracking
- **Security** — Flag risky operations, detect production changes
- **Audit** — Complete access logs with admin controls

## Prerequisites

- Claude Code installed and configured
- AgentFabric collector running (default: http://localhost:4318)
- Network connectivity from Claude Code client to collector

## Quick Start

### Step 1: Set Environment Variables

Before running Claude Code, set these environment variables:

**Development (local collector):**
```bash
export CLAUDE_CODE_ENABLE_TELEMETRY=1
export OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:4318"
export OTEL_EXPORTER_OTLP_HEADERS="x-api-key=dev-token"
export OTEL_METRICS_EXPORTER="otlp"
export OTEL_LOGS_EXPORTER="otlp"
export OTEL_TRACES_EXPORTER="otlp"
export OTEL_SERVICE_NAME="claude-code"
export OTEL_RESOURCE_ATTRIBUTES="deployment.environment=dev,team=my-team"
```

**Production (secure collector):**
```bash
export CLAUDE_CODE_ENABLE_TELEMETRY=1
export OTEL_EXPORTER_OTLP_ENDPOINT="https://otel.your-company.com"
export OTEL_EXPORTER_OTLP_HEADERS="authorization=Bearer $(cat /var/run/secrets/otel-token)"
export OTEL_SERVICE_NAME="claude-code"
export OTEL_RESOURCE_ATTRIBUTES="deployment.environment=production,team=engineering,host=$(hostname)"
```

### Step 2: Use Claude Code

Simply run Claude Code as normal; telemetry is sent automatically:

```bash
claude-code "What are the most common patterns in this codebase?"
```

### Step 3: View in AgentFabric

Open http://localhost:3000 and navigate to "AI Tools" → "Claude Code Usage" to see:
- Sessions and duration
- Token usage trends
- Cost estimates
- Risk events requiring review

## Telemetry Events

Claude Code emits these standardized events:

| Event | Triggered When |
|-------|-----------------|
| `session.started` | Claude Code session begins |
| `session.ended` | Claude Code exits |
| `tool.call.started` | File operation, shell command, or MCP call begins |
| `tool.call.completed` | Tool execution succeeds |
| `tool.call.failed` | Tool execution fails |
| `token.usage.recorded` | Token usage metrics reported |
| `cost.estimated` | Cost calculated |
| `tool.approval.requested` | High-risk operation needs approval |
| `error.detected` | Exception or runtime error |

## Privacy by Default

Claude Code **does not send:**
- User prompts
- Tool arguments or parameters
- File contents
- Environment variables
- Secrets or API keys

Claude Code **does send:**
- Session metadata (start time, duration, model used)
- Tool names (e.g., "shell", "file", "browser")
- Token counts (input, output, cache read/write)
- Event types and timestamps
- Risk scores and governance decisions

### Why This Matters

Your prompts and code remain private while AgentFabric tracks usage for:
- **Cost accountability** — Accurate billing by user/team/project
- **Governance** — Risk detection without exposing content
- **Compliance** — Audit trails without data leakage
- **Performance** — Token/cost trends for optimization

## Risk Scoring

Every tool invocation is scored 0-100 for risk:

| Risk Factor | Score | Example |
|------------|-------|---------|
| Dangerous shell commands | 85-95 | `rm -rf`, `mkfs`, `curl | sh` |
| Production file modifications | 70-80 | Edits to `prod/`, `.env`, `terraform/`, `k8s/` |
| MCP tool usage | 30+ | File system, browser, GitHub access |
| Unusual activity | 40-60 | High token usage, repeated errors |
| Normal operations | 0-20 | Safe file reads, code generation |

Events flagged >50 appear in "High Risk Events" dashboard for review.

## Environment Variables Reference

| Variable | Required | Purpose |
|----------|----------|---------|
| `CLAUDE_CODE_ENABLE_TELEMETRY` | Yes | Enable telemetry (must be "1") |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | Yes | Collector HTTP endpoint |
| `OTEL_EXPORTER_OTLP_HEADERS` | No | Auth header (e.g., "authorization=Bearer TOKEN") |
| `OTEL_METRICS_EXPORTER` | No | Set to "otlp" to enable metrics (default) |
| `OTEL_LOGS_EXPORTER` | No | Set to "otlp" to enable logs (default) |
| `OTEL_TRACES_EXPORTER` | No | Set to "otlp" to enable traces (default) |
| `OTEL_SERVICE_NAME` | No | Service name in logs (default: "claude-code") |
| `OTEL_RESOURCE_ATTRIBUTES` | No | Key=value pairs for resource context |

## Persistent Configuration

To avoid setting variables every session, add to your shell profile:

**`~/.bashrc` or `~/.zshrc`:**
```bash
# Claude Code OpenTelemetry
source ~/.claude-code-telemetry.sh
```

**`~/.claude-code-telemetry.sh`:**
```bash
export CLAUDE_CODE_ENABLE_TELEMETRY=1
export OTEL_EXPORTER_OTLP_ENDPOINT="${OTEL_ENDPOINT:-http://localhost:4318}"
export OTEL_EXPORTER_OTLP_HEADERS="${OTEL_HEADERS:-x-api-key=dev-token}"
export OTEL_SERVICE_NAME="claude-code"
export OTEL_RESOURCE_ATTRIBUTES="deployment.environment=dev,team=$(whoami)"
```

Then:
```bash
source ~/.claude-code-telemetry.sh
claude-code --help
```

## Verification

Confirm telemetry is active:

```bash
# These variables should be set
echo $CLAUDE_CODE_ENABLE_TELEMETRY  # Should print: 1
echo $OTEL_EXPORTER_OTLP_ENDPOINT   # Should print: http://localhost:4318

# Run a simple Claude Code command
claude-code "Tell me about this directory"

# Check AgentFabric portal for the new session
# http://localhost:3000 → "AI Tools" → "Claude Code Usage"
```

## Troubleshooting

**Telemetry not appearing:**
1. Verify `CLAUDE_CODE_ENABLE_TELEMETRY=1` is set: `echo $CLAUDE_CODE_ENABLE_TELEMETRY`
2. Check collector is running: `curl http://localhost:4318/v1/logs`
3. Verify network connectivity: `ping $(echo $OTEL_EXPORTER_OTLP_ENDPOINT | cut -d/ -f3)`
4. Review collector logs: `docker-compose logs collector | grep -i "claude"`

**Authentication errors:**
```bash
# Test with auth header
curl -H "x-api-key: dev-token" http://localhost:4318/v1/logs

# Check token format in OTEL_EXPORTER_OTLP_HEADERS
echo $OTEL_EXPORTER_OTLP_HEADERS
```

**High latency:**
- Increase OTLP batch timeout: add to env file
- Use local collector instead of remote: `OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4318`
- Monitor network: `tcpdump -i any -n 'port 4318'`

## Admin Features

### View Redacted Content (Admin Only)

If your admin has enabled prompt logging, admins can decrypt and review prompts:

1. Go to AgentFabric portal → "Admin" → "Redaction Audit"
2. Search for session ID
3. Click "Reveal" to decrypt prompt (audit logged)
4. Each access is recorded with admin email, timestamp, reason

**This is only available if your admin explicitly enables it.** By default, prompts are not stored.

### Cost Analysis by Model

Dashboard shows costs broken down by:
- Model (Claude 3.5 Sonnet, Haiku, etc.)
- Time period (daily, weekly, monthly)
- User or team
- Tool usage patterns

### Session Replay

Click any session to see:
- Timeline of events (start → end)
- Tool calls in order
- Token usage breakdown
- Any errors that occurred
- Risk flags and decisions

## Integration with CI/CD

Use in automation pipelines:

```yaml
# GitHub Actions example
- name: Run Claude Code analysis
  env:
    CLAUDE_CODE_ENABLE_TELEMETRY: "1"
    OTEL_EXPORTER_OTLP_ENDPOINT: "https://otel.company.com"
    OTEL_EXPORTER_OTLP_HEADERS: "authorization=Bearer ${{ secrets.OTEL_TOKEN }}"
    OTEL_RESOURCE_ATTRIBUTES: "ci_job_id=${{ github.run_id }}"
  run: |
    claude-code "Analyze this PR for security issues"
```

Telemetry automatically includes CI context in resource attributes.

## Support

Need help?

1. **Check collector health:** `docker-compose ps collector`
2. **View collector logs:** `docker-compose logs collector -f`
3. **Verify schema:** `psql -c "SELECT COUNT(*) FROM ai_agent_events;"`
4. **Contact:** File an issue with session ID from AgentFabric portal

## Next Steps

- [API Reference](../api/ai-telemetry.md) — Query events programmatically
- [Telemetry Guide](../TELEMETRY_GUIDE.md) — Detailed architecture and governance
- [Codex Integration](./codex-telemetry-setup.md) — Set up OpenAI Codex
