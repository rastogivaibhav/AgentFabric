# AgentFabric Tool & Vendor Configuration Guide

This document provides a quick reference for configuring AgentFabric with various AI tools and services.

## Supported Tools Overview

### IDEs & Editors

| Tool | Type | Status | Guide |
|---|---|---|---|
| **Cursor** | AI IDE | ✅ Phase 3 Complete | [SETUP_CURSOR.md](./SETUP_CURSOR.md) |
| **VSCode Extensions** | Editor Extensions | ✅ Phase 3 Complete | [SETUP_VSCODE.md](./SETUP_VSCODE.md) |
| - GitHub Copilot | Code assistant | ✅ | Included in SETUP_VSCODE.md |
| - Continue | Code assistant | ✅ | Included in SETUP_VSCODE.md |
| - Roo Codemod | Code transformation | ✅ | Included in SETUP_VSCODE.md |
| - Cline | AI coding companion | ✅ | Included in SETUP_VSCODE.md |

### Collaboration & Pairing

| Tool | Type | Status | Guide |
|---|---|---|---|
| **Cowork** | Paired programming | ✅ Phase 3 Complete | [SETUP_COWORK.md](./SETUP_COWORK.md) |

### API Services

| Service | Type | Status | Guide |
|---|---|---|---|
| **Anthropic API** | Direct API integration | ✅ Phase 3 Complete | [SETUP_ANTHROPIC_API.md](./SETUP_ANTHROPIC_API.md) |
| **OpenAI API** | Proxy via Gateway | ✅ | See Proxy Configuration |
| **Google Gemini API** | Proxy via Gateway | ✅ | See Proxy Configuration |
| **Bedrock** | AWS AI service | ✅ | See Proxy Configuration |

### Frameworks (Observability Only)

AgentFabric automatically detects and observes:

- **CrewAI** — Multi-agent orchestration framework
- **LangGraph** — LLM computation graphs
- **OpenAI Agents** — OpenAI agent framework
- **Claude Agents** — Anthropic agent framework
- **Google ADK** — Google Agentive AI

No configuration needed for frameworks — AgentFabric SDK auto-instruments them.

## Configuration Checklist

### Step 1: Set Up Gateway

```bash
# Start AgentFabric gateway and collector
docker-compose up -d api-gateway collector

# Verify health
curl http://localhost:8080/healthz
```

### Step 2: Generate Webhook Token

```bash
# Create a webhook token for your organization
WEBHOOK_TOKEN=$(openssl rand -hex 32)
export AGENTFABRIC_WEBHOOK_TOKEN=$WEBHOOK_TOKEN

# Store securely in your secrets manager
```

### Step 3: Configure Each Tool

For each tool you use, follow the specific setup guide:

```bash
# Cursor
source docs/SETUP_CURSOR.md

# VSCode
source docs/SETUP_VSCODE.md

# Cowork
source docs/SETUP_COWORK.md

# Anthropic API
source docs/SETUP_ANTHROPIC_API.md
```

## Environment Variables (Common to All)

Set these in your shell profile or CI/CD system:

```bash
# Core configuration
export AGENTFABRIC_GATEWAY_URL=http://localhost:8080
export AGENTFABRIC_WEBHOOK_TOKEN=your-secure-token-here
export AGENTFABRIC_TENANT_ID=your-organization-id

# Optional
export AGENTFABRIC_ENVIRONMENT=development  # development|staging|production
export AGENTFABRIC_DEBUG=false              # Enable verbose logging
export AGENTFABRIC_REDACT_SECRETS=true      # Redact API keys in logs
```

## Quick Start by Role

### Developer Using Cursor

1. Install Cursor from https://cursor.com
2. Set environment variables (see above)
3. Follow [SETUP_CURSOR.md](./SETUP_CURSOR.md) steps 2-3
4. Start coding; events appear automatically in AgentFabric

### Team Using VSCode + Copilot + Continue

1. Install extensions from VSCode Marketplace
2. Set environment variables
3. Follow [SETUP_VSCODE.md](./SETUP_VSCODE.md) Extension-Specific Configuration
4. Open any project; sessions appear in **Live Stream**

### Team Using Cowork for Pair Programming

1. Install Cowork CLI (see [SETUP_COWORK.md](./SETUP_COWORK.md))
2. Run `cowork login` and `cowork config agentfabric init`
3. Start a session: `cowork session create --name "feature-xyz"`
4. Monitor in AgentFabric **Traces** filtered by `framework = cowork`

### Application Using Anthropic API

1. Install SDK: `pip install anthropic agentfabric-sdk`
2. Set `ANTHROPIC_API_KEY` and AgentFabric env vars
3. Instrument client with `instrument_anthropic()` (see [SETUP_ANTHROPIC_API.md](./SETUP_ANTHROPIC_API.md))
4. Each API call is automatically tracked

## Monitoring & Governance

### Where to Monitor Each Tool

| Tool | Traces | Analytics | Cost | Governance |
|---|---|---|---|---|
| Cursor | `cursor-ide` | ✅ | ✅ | ✅ |
| VSCode | `vscode-*` | ✅ | ✅ | ✅ |
| Cowork | `cowork` | ✅ | ✅ | ✅ |
| Anthropic API | `anthropic-api` | ✅ | ✅ | ✅ |

Navigate to:
- **Traces** — See all events from all tools
- **Analytics** — Token usage, cost trends, error rates
- **Cost & Budget** — Spending by tool, user, model
- **Governance** — Flag risky operations, approve/reject decisions

### Example Filters

```
# See all Cursor activity
framework = "cursor-ide"

# See all VSCode Copilot completions
framework = "vscode-copilot"

# See high-cost Cowork sessions (>$10)
framework = "cowork" AND cost_usd > 10

# See all Anthropic API calls
framework = "anthropic-api"

# See all secret exposures
risk_category = "secret_exposure"

# See all production file edits
risk_category = "prod_edit"
```

## Troubleshooting Matrix

| Symptom | Solution |
|---|---|
| No events appearing | Check AGENTFABRIC_GATEWAY_URL is accessible; verify WEBHOOK_TOKEN is correct |
| High latency | Increase batch_size in tool config; check network to gateway |
| Cost seems wrong | Verify pricing rules in **Pricing & Rules** match vendor rates |
| Secrets exposed in logs | Enable AGENTFABRIC_REDACT_SECRETS=true in config |
| Token counts don't match | Compare with vendor's API response; ensure no batching/retries skew counts |
| Governance policies not triggering | Check policy conditions match event fields; test with manual events |

## Advanced Topics

### Multi-Tenant Setup

If you have multiple teams/organizations:

```bash
# Create separate tenant IDs
export AGENTFABRIC_TENANT_ID=team-a-production
export AGENTFABRIC_TENANT_ID=team-b-production

# Each tenant has isolated:
# - Cost tracking
# - Governance policies
# - Budget limits
# - Access controls
```

### Custom Pricing Rules

Update model pricing in **Pricing & Rules**:

```json
{
  "vendor": "anthropic",
  "models": [
    {
      "name": "claude-3-opus",
      "input_per_million": 15,
      "output_per_million": 75
    }
  ]
}
```

### Governance Policy Examples

```bash
# Flag expensive API calls
agentfabric policy create \
  --name "expensive_calls" \
  --condition "framework = 'anthropic-api' AND cost_usd > 0.10" \
  --action alert_team

# Require approval for production edits
agentfabric policy create \
  --name "prod_edits_approval" \
  --condition "environment = 'production' AND risk_category = 'prod_edit'" \
  --action requires_approval

# Track all secret exposures
agentfabric policy create \
  --name "secret_audit_log" \
  --condition "risk_category = 'secret_exposure'" \
  --action log_to_audit_trail
```

## Proxy Configuration (LLM Provider Routing)

AgentFabric can intercept and route API calls through a policy engine. See [Proxy Configuration](./PROXY_CONFIGURATION.md) for:

- OpenAI API interception
- Anthropic API client routing
- Google Gemini routing
- Budget enforcement at the proxy layer
- Request/response modification

## Support & Documentation

| Resource | Link |
|---|---|
| AgentFabric Docs | https://docs.agentfabric.dev |
| GitHub Repo | https://github.com/govagn/agentfabric |
| Issue Tracker | https://github.com/govagn/agentfabric/issues |
| Community Slack | https://slack.agentfabric.dev |

## Version Compatibility

Ensure minimum versions:

- **Cursor**: 0.20+
- **VSCode**: 1.60+
- **Cowork CLI**: 0.5+
- **Anthropic SDK**: 0.7+
- **AgentFabric Collector**: 3.0+
- **AgentFabric Gateway**: 3.0+

## FAQ

**Q: Can I use multiple tools simultaneously?**
A: Yes! AgentFabric aggregates telemetry from all configured tools. Use filters in **Traces** to isolate specific tools.

**Q: What if my tool isn't listed?**
A: Use the Webhook Ingestion API to send custom events. See [SETUP_ANTHROPIC_API.md](./SETUP_ANTHROPIC_API.md) → Method 2.

**Q: How do I disable a tool's monitoring?**
A: Set `enabled: false` in the tool's config, or set the env var `TOOL_NAME_TELEMETRY=false`.

**Q: What data is collected?**
A: Token counts, latency, models used, cost, timestamps, and risk flags. See Privacy section in each tool's guide.

**Q: Is my data encrypted?**
A: In transit: TLS/HTTPS. At rest: Database encryption (PostgreSQL pgcrypto). API keys stored encrypted in vault.

**Q: How do I export usage data?**
A: Use Analytics export or query the API. See docs.agentfabric.dev/api/export.
