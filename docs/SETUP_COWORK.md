# Setting Up AgentFabric with Cowork

Cowork is a paired programming assistant platform that integrates with AgentFabric for full observability of collaborative AI-assisted development sessions.

## Prerequisites

- Cowork account and CLI tool installed
- AgentFabric collector running
- Webhook endpoint accessible from Cowork infrastructure
- API token for Cowork workspace

## Installation

### 1. Install Cowork CLI

```bash
# macOS / Linux
curl -sSL https://cowork.dev/install.sh | bash

# Or with Homebrew
brew install cowork-dev

# Verify installation
cowork --version
```

### 2. Authenticate with Cowork

```bash
cowork login
# Follow the browser prompt to authorize your workspace
```

### 3. Configure AgentFabric Integration

Initialize AgentFabric integration:

```bash
cowork config agentfabric init
```

When prompted, enter:
- **Gateway URL**: `http://localhost:8080` (or your gateway)
- **Webhook Token**: Your AgentFabric webhook token
- **Tenant ID**: Your organization's tenant ID
- **Environment**: `development`, `staging`, or `production`

## Configuration

### Cowork Config File (`~/.cowork/config.yaml`)

```yaml
agentfabric:
  enabled: true
  gateway:
    url: http://localhost:8080
    webhook_token: ${AGENTFABRIC_WEBHOOK_TOKEN}
  tenant:
    id: ${AGENTFABRIC_TENANT_ID}
    environment: development
  telemetry:
    capture_sessions: true
    capture_edits: true
    capture_discussions: true
    batch_events: true
    batch_size: 100
    flush_interval_ms: 5000
  privacy:
    redact_secrets: true
    redact_credentials: true
    anonymize_usernames: false
    anonymize_code: false
```

### Environment Variables

```bash
# Add to your shell profile
export AGENTFABRIC_GATEWAY_URL=http://localhost:8080
export AGENTFABRIC_WEBHOOK_TOKEN=your-token
export AGENTFABRIC_TENANT_ID=your-tenant-id

# Optional
export AGENTFABRIC_DEBUG=false  # Set to true for verbose logging
export COWORK_TELEMETRY=true    # Enable Cowork telemetry collection
```

## Event Types

### Session Events

| Event | Description |
|---|---|
| `session.started` | User starts a Cowork session |
| `session.ended` | Session concludes |
| `session.pause` | Pausing collaborative work |
| `session.resume` | Resuming after pause |

### Interaction Events

| Event | Description |
|---|---|
| `discussion.message` | Participant sends message |
| `code_suggestion` | AI suggests code change |
| `code_review` | Participant reviews/approves suggestion |
| `file_edit` | Code change applied to file |
| `test_execution` | Test run triggered |

### Collaboration Events

| Event | Description |
|---|---|
| `participant.joined` | New participant joins session |
| `participant.left` | Participant leaves |
| `conflict.detected` | Edit conflict detected |
| `resolution.applied` | Conflict resolved |

## Monitoring in AgentFabric

### 1. View Cowork Sessions

Navigate to **Traces**:

```
Filter: framework = "cowork"
```

Each session appears as a trace with:
- Session ID and name
- Participants list
- Duration and timestamps
- Token usage and cost
- Any risk flags

### 2. Analyze Collaboration Metrics

In **Analytics** dashboard:

- **Total tokens by session**: How much compute each session used
- **Cost by participant**: Track AI spend per team member
- **Error rates**: Issues encountered during sessions
- **Framework stats**: Compare Cowork vs other tools

### 3. Enable Governance Policies

Create policies to govern Cowork usage:

```bash
# Flag high-cost sessions (>5000 tokens)
cowork policy create --name "high_cost_sessions" \
  --condition "total_tokens > 5000" \
  --action alert

# Require approval for production edits
cowork policy create --name "prod_edits_require_approval" \
  --condition "environment = 'production' AND event_type = 'file_edit'" \
  --action requires_approval
```

## Usage Example

### Starting a Session with AgentFabric

```bash
# Create a new collaborative session
cowork session create --name "refactoring-auth-module"

# Invite team member
cowork session invite alice@company.com

# Start the session
cowork session start

# Work collaboratively...
# AgentFabric records all activity automatically

# End session
cowork session end

# View summary in AgentFabric
# → Traces, search by session name or ID
```

## Troubleshooting

### Events Not Appearing

```bash
# Check AgentFabric integration status
cowork config agentfabric status

# Verify gateway connectivity
curl -X GET http://localhost:8080/healthz

# Enable debug logging
export AGENTFABRIC_DEBUG=true
cowork session start
```

### Authentication Errors

```bash
# Re-authenticate with Cowork
cowork logout
cowork login

# Re-initialize AgentFabric
cowork config agentfabric init

# Verify webhook token is correct
echo $AGENTFABRIC_WEBHOOK_TOKEN
```

### High Latency

- Check network latency to AgentFabric gateway
- Increase batch size for network efficiency:
  ```yaml
  telemetry:
    batch_size: 200  # Increased from 100
  ```
- Enable compression (if supported):
  ```yaml
  gateway:
    compression: gzip
  ```

## Best Practices

1. **Separate tenant IDs by environment** — dev/staging/prod should have different IDs
2. **Review session costs weekly** — Identify unexpectedly expensive collaborations
3. **Set team budget limits** — Prevent runaway token usage
4. **Archive old sessions** — Keep governance audit trail clean
5. **Enable secret redaction** — Never leak API keys or credentials

## Performance Tuning

For high-traffic sessions (10+ participants, 10k+ events):

```yaml
telemetry:
  batch_size: 500         # Larger batches
  flush_interval_ms: 10000 # Longer flush window
  compression: gzip        # Reduce network traffic
  async_mode: true        # Non-blocking event capture
```

## Cost Analysis

Monitor Cowork usage costs in AgentFabric:

```bash
# View cost report for Cowork sessions
cowork cost report --framework cowork --period "7d"

# Breakdown by participant
cowork cost report --framework cowork --group-by participant

# Breakdown by session type
cowork cost report --framework cowork --group-by session_type
```

## Support

- Documentation: https://docs.cowork.dev
- AgentFabric integration guide: https://docs.agentfabric.dev/integrations/cowork
- Issues: https://github.com/govagn/agentfabric/issues
