# Setting Up AgentFabric with Cursor

Cursor is an AI-powered IDE that integrates with AgentFabric for observability and governance of AI-assisted coding sessions.

## Prerequisites

- Cursor IDE (https://cursor.com) version 0.20+
- AgentFabric collector running and accessible
- Webhook endpoint configured in AgentFabric gateway

## Installation

### 1. Configure AgentFabric Gateway

Ensure your gateway is listening on the webhook endpoint:

```bash
# Default webhook endpoint
POST http://localhost:8080/webhook/telemetry
POST http://localhost:8080/webhook/telemetry/batch
```

### 2. Set Environment Variables

Add these to your Cursor environment or `.cursorrc`:

```bash
# AgentFabric gateway endpoint
AGENTFABRIC_GATEWAY_URL=http://localhost:8080
AGENTFABRIC_WEBHOOK_TOKEN=your-token-here

# Cursor event metadata
CURSOR_TENANT_ID=your-tenant-id
CURSOR_ENVIRONMENT=development
```

### 3. Install Cursor Extension (if available)

If AgentFabric provides a Cursor extension:

```bash
# Via marketplace
cursor --install-extension agentfabric.cursor-telemetry
```

## Configuration

### Cursor Settings (`cursor_settings.json`)

```json
{
  "agentfabric": {
    "enabled": true,
    "gateway": "http://localhost:8080",
    "webhook_path": "/webhook/telemetry",
    "capture": {
      "code_edits": true,
      "ai_generations": true,
      "refactoring_operations": true,
      "command_execution": false
    },
    "privacy": {
      "redact_secrets": true,
      "redact_api_keys": true,
      "redact_emails": true
    }
  }
}
```

## Monitoring

Once configured, you can monitor Cursor activity in AgentFabric:

1. **Dashboard** → Traces: See all Cursor editing sessions
2. **Analytics**: Token usage, cost, error rates by session
3. **Governance**: Flag risky operations (e.g., production file edits, secret exposure)

## Troubleshooting

### No Events Appearing

- Verify gateway URL is correct and accessible
- Check webhook token matches gateway configuration
- Review Cursor logs: `Cmd+Shift+P` → "View Logs"

### High Latency

- Ensure network connection to gateway is stable
- Check gateway is not rate-limited (default: 1000 req/min)

### Secrets Being Exposed

- Enable `redact_secrets` in cursor_settings.json
- Run `cursor --clear-cache` to restart event capture

## Supported Events

| Event Type | Description |
|---|---|
| `code_edit` | User edits code directly |
| `ai_generation` | Cursor AI generates code |
| `chat_message` | User sends message in Cursor chat |
| `refactor` | Apply refactoring suggestion |
| `test_run` | Run tests via Cursor |

## Cost Calculation

Cursor integration tracks:
- **Tokens**: Input/output tokens from Cursor API calls
- **Cost**: Estimated based on model pricing rules
- **Category**: "cursor-ide" for segmentation

Costs are visible in **Cost & Budget** page.
