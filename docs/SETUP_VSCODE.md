# Setting Up AgentFabric with VSCode AI Extensions

AgentFabric integrates with popular VSCode AI extensions to provide observability and governance over your AI-assisted development workflow.

## Supported Extensions

- **GitHub Copilot** — Official GitHub AI assistant
- **Continue** — Open-source code assistant
- **Roo Codemod** — AI-powered code transformation
- **Cline** — AI coding companion with tool use

## Prerequisites

- VSCode 1.60+
- One or more of the supported extensions installed
- AgentFabric collector accessible via webhook endpoint

## Installation

### 1. Install VSCode Extension(s)

From VSCode Marketplace:

```
GitHub Copilot: search "GitHub Copilot"
Continue: search "Continue"
Roo Codemod: search "Roo"
Cline: search "Cline"
```

Or via CLI:

```bash
code --install-extension GitHub.Copilot
code --install-extension Continue.continue
code --install-extension RooCline.roo
code --install-extension saoudrizwan.claude-dev
```

### 2. Configure AgentFabric Extension

Create or update `.vscode/settings.json`:

```json
{
  "agentfabric": {
    "enabled": true,
    "gatewayUrl": "http://localhost:8080",
    "webhookToken": "your-webhook-token",
    "tenantId": "your-tenant-id",
    "environment": "development",
    "extensions": {
      "copilot": { "enabled": true, "trackTokens": true },
      "continue": { "enabled": true, "trackTokens": true },
      "roo": { "enabled": true, "trackTokens": true },
      "cline": { "enabled": true, "trackTokens": true }
    },
    "privacy": {
      "redactSecrets": true,
      "redactCredentials": true,
      "redactPaths": false
    }
  }
}
```

### 3. Set Environment Variables

In your shell profile (`.bashrc`, `.zshrc`, etc.):

```bash
export AGENTFABRIC_GATEWAY_URL=http://localhost:8080
export AGENTFABRIC_WEBHOOK_TOKEN=your-webhook-token
export VSCODE_TENANT_ID=your-tenant-id
```

## Extension-Specific Configuration

### GitHub Copilot

```json
{
  "github.copilot.enable": {
    "*": true,
    "plaintext": false,
    "markdown": false
  },
  "agentfabric.copilot": {
    "trackSuggestions": true,
    "trackCompletions": true,
    "anonymizeFilePaths": false
  }
}
```

**Events tracked:**
- Code suggestions accepted/rejected
- Chat interactions
- Token usage per session

### Continue

```json
{
  "continue.telemetry": {
    "enabled": true,
    "sendCrashReports": true,
    "sendUsageData": true
  },
  "agentfabric.continue": {
    "trackMessages": true,
    "trackRefactorings": true,
    "trackEdits": true
  }
}
```

**Events tracked:**
- User messages and AI responses
- Code refactoring operations
- File edits triggered by Continue

### Roo Codemod

```json
{
  "agentfabric.roo": {
    "trackCodemods": true,
    "trackApprovals": true,
    "captureTransformations": true
  }
}
```

**Events tracked:**
- Codemod proposals and approvals
- Transformation scope (files affected)
- Token usage for analysis

### Cline

```json
{
  "agentfabric.cline": {
    "trackToolUse": true,
    "trackMCPCalls": true,
    "trackFileOperations": true,
    "flagDangerousOperations": true
  }
}
```

**Events tracked:**
- Tool calls and results
- MCP server interactions
- File system operations (create, edit, delete)
- Dangerous patterns (e.g., `rm -rf`, shell commands in prod)

## Monitoring in AgentFabric

### 1. View Live Activity

Navigate to **Live Stream** to see real-time events from VSCode:

```
http://localhost:3000/live
```

Filter by extension:
- `framework: "vscode_copilot"`
- `framework: "vscode_continue"`
- `framework: "vscode_roo"`
- `framework: "vscode_cline"`

### 2. Analyze Cost & Tokens

**Cost & Budget** page shows:
- Tokens used per extension
- Cost by AI model (GPT-4, Claude, etc.)
- Budget tracking and alerts

### 3. Review Governance

**Governance** page flags:
- Dangerous commands (Cline running `rm -rf`)
- Secret exposure in code generations
- High token usage anomalies
- Production file modifications

Approve or reject flagged operations to enforce governance policies.

## Troubleshooting

### No Events Appearing

```bash
# Check extension is installed
code --list-extensions | grep -i agentfabric

# Enable debug logs
echo "agentfabric.debug=true" >> .vscode/settings.json

# Restart VSCode
code .
```

### Authentication Failed

```bash
# Verify webhook token
curl -X POST http://localhost:8080/webhook/health \
  -H "Authorization: Bearer YOUR_TOKEN"

# Check gateway is accessible
curl http://localhost:8080/healthz
```

### Missing Token Tracking

Ensure each extension has `trackTokens: true` in settings:

```json
{
  "agentfabric.extensions": {
    "copilot": { "trackTokens": true }
  }
}
```

## Best Practices

1. **Use separate tenant IDs** for dev, staging, production
2. **Enable secret redaction** to prevent credential leakage
3. **Set budget alerts** to catch unexpected token usage spikes
4. **Review governance decisions** weekly to tune governance policies
5. **Keep extensions updated** to get latest AgentFabric features

## Advanced Configuration

### Custom Webhook Endpoint

```json
{
  "agentfabric": {
    "webhookUrl": "https://my-gateway.example.com/webhook/telemetry",
    "webhookBatch": true,
    "batchSize": 50,
    "flushInterval": 5000
  }
}
```

### Rate Limiting

```json
{
  "agentfabric": {
    "rateLimit": {
      "enabled": true,
      "requestsPerMinute": 1000,
      "eventsPerSecond": 100
    }
  }
}
```

## Support

For issues:

1. Check logs: **View** → **Output** → **AgentFabric**
2. File an issue: https://github.com/govagn/agentfabric/issues
3. Contact support: support@agentfabric.dev
