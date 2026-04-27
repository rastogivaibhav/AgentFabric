# Setting Up AgentFabric with Anthropic API

This guide covers integrating direct Anthropic API usage (Claude, Claude Instant, etc.) with AgentFabric for full observability and governance.

## Prerequisites

- Anthropic API key from https://console.anthropic.com
- AgentFabric collector and gateway running
- Python 3.8+ (for SDK examples)

## Integration Methods

AgentFabric supports two integration patterns for Anthropic API:

1. **Direct SDK Instrumentation** — Add observability to your own Anthropic SDK calls
2. **Webhook Ingestion** — Send events via HTTP webhook

## Method 1: Direct SDK Instrumentation (Recommended)

### Installation

```bash
# Install Anthropic SDK with AgentFabric support
pip install anthropic agentfabric-sdk
```

### Configuration

Set environment variables:

```bash
export ANTHROPIC_API_KEY=sk-ant-xxxxxxxxxxxx
export AGENTFABRIC_GATEWAY_URL=http://localhost:8080
export AGENTFABRIC_WEBHOOK_TOKEN=your-webhook-token
export AGENTFABRIC_TENANT_ID=your-tenant-id
```

### Example: Instrumented Anthropic API Call

```python
from anthropic import Anthropic
from agentfabric_sdk import instrument_anthropic
import os

# Initialize client
client = Anthropic(api_key=os.environ["ANTHROPIC_API_KEY"])

# Instrument for AgentFabric observability
client = instrument_anthropic(
    client,
    gateway_url=os.environ["AGENTFABRIC_GATEWAY_URL"],
    webhook_token=os.environ["AGENTFABRIC_WEBHOOK_TOKEN"],
    tenant_id=os.environ["AGENTFABRIC_TENANT_ID"],
)

# Your API calls are now observable
response = client.messages.create(
    model="claude-3-sonnet-20240229",
    max_tokens=1024,
    messages=[
        {
            "role": "user",
            "content": "What is the capital of France?"
        }
    ]
)

print(response.content[0].text)
# AgentFabric automatically captures:
# - Model used (claude-3-sonnet-20240229)
# - Input/output tokens
# - Cost ($0.003 for this example)
# - Timestamp and duration
```

### Advanced Configuration

```python
from agentfabric_sdk import instrument_anthropic

client = instrument_anthropic(
    client,
    gateway_url="http://localhost:8080",
    webhook_token="your-token",
    tenant_id="your-tenant-id",
    # Optional settings
    environment="production",
    app_name="my-agent",
    user_id="user@example.com",
    session_id="sess-12345",
    metadata={
        "model_version": "v1.2",
        "feature_flag": "new_prompt_format",
    },
    # Privacy settings
    redact_input=False,
    redact_output=False,
    redact_secrets=True,
)
```

## Method 2: Webhook Ingestion (For REST Clients)

If you're using curl, JavaScript, or other HTTP clients:

### 1. Send Anthropic API Request

```bash
curl https://api.anthropic.com/v1/messages \
  -H "x-api-key: $ANTHROPIC_API_KEY" \
  -H "content-type: application/json" \
  -d '{
    "model": "claude-3-sonnet-20240229",
    "max_tokens": 1024,
    "messages": [
      {"role": "user", "content": "What is 2+2?"}
    ]
  }' > response.json
```

### 2. Capture and Send to AgentFabric

```bash
# Extract relevant data and send to AgentFabric webhook
curl http://localhost:8080/webhook/telemetry \
  -H "Authorization: Bearer $AGENTFABRIC_WEBHOOK_TOKEN" \
  -H "content-type: application/json" \
  -d '{
    "source_vendor": "anthropic",
    "source_product": "anthropic-api",
    "source_channel": "api",
    "event_type": "api.call.completed",
    "event_category": "model_call",
    "action": "completion",
    "model_name": "claude-3-sonnet-20240229",
    "provider": "anthropic",
    "input_tokens": 23,
    "output_tokens": 15,
    "estimated_cost": 0.00024,
    "latency_ms": 1245,
    "success": true,
    "timestamp": "2024-04-27T12:34:56Z",
    "user_id": "user@example.com",
    "session_id": "sess-abc123"
  }'
```

### 3. Webhook Endpoint Reference

**URL:** `POST /webhook/telemetry`

**Headers:**
- `Authorization: Bearer <WEBHOOK_TOKEN>`
- `Content-Type: application/json`

**Body:**

```json
{
  "source_vendor": "anthropic",           // Required
  "source_product": "anthropic-api",      // anthropic-api, anthropic-sdk, etc.
  "source_channel": "api",                // api, sdk, etc.
  "event_type": "api.call.completed",     // Event type
  "event_category": "model_call",         // model_call, function_call, etc.
  "action": "completion",                 // completion, classification, etc.
  "model_name": "claude-3-sonnet-20240229", // Model identifier
  "provider": "anthropic",                // Provider name
  "input_tokens": 100,                    // Input token count
  "output_tokens": 50,                    // Output token count
  "estimated_cost": 0.001,                // Estimated USD cost
  "latency_ms": 1000,                     // Response latency
  "success": true,                        // Success flag
  "error_message": null,                  // Error if applicable
  "timestamp": "2024-04-27T12:34:56Z",   // ISO 8601 timestamp
  "user_id": "user@example.com",         // Optional: user identifier
  "session_id": "sess-123",              // Optional: session ID
  "environment": "production",            // Optional: environment
  "metadata": {                           // Optional: custom metadata
    "prompt_template": "v2",
    "routing_mode": "standard"
  }
}
```

## Monitoring in AgentFabric

### 1. View All Anthropic API Calls

Navigate to **Traces**:

```
Filter: framework = "anthropic_api"
```

### 2. Analyze Token Usage and Cost

In **Cost & Budget** dashboard:

- Token usage over time
- Cost breakdown by model (Sonnet, Opus, Haiku)
- Cost per API call (min/max/avg)
- Budget tracking and alerts

### 3. Enable Risk Governance

Flag risky API usage patterns:

```bash
# Create policy: flag high-cost completions
agentfabric policy create \
  --name "expensive_completions" \
  --condition "output_tokens > 2000" \
  --action alert_team

# Create policy: require approval for production
agentfabric policy create \
  --name "prod_calls_require_approval" \
  --condition "environment = 'production'" \
  --action requires_approval
```

## Cost Model

AgentFabric tracks Anthropic API costs based on:

- **Model**: Sonnet, Opus, Haiku pricing tiers
- **Tokens**: Input and output token counts
- **Feature**: Longer context windows cost more per token

Example:

```
Model: claude-3-sonnet-20240229
Input: 100 tokens @ $0.000003/token = $0.0003
Output: 50 tokens @ $0.000015/token = $0.00075
Total: $0.00105 per call
```

Update pricing rules in **Pricing & Rules** to match current Anthropic rates.

## Examples

### Python: Multi-Turn Conversation

```python
from anthropic import Anthropic
from agentfabric_sdk import instrument_anthropic

client = Anthropic()
client = instrument_anthropic(client, gateway_url="http://localhost:8080", ...)

conversation_history = []

while True:
    user_input = input("You: ")
    conversation_history.append({
        "role": "user",
        "content": user_input
    })
    
    response = client.messages.create(
        model="claude-3-sonnet-20240229",
        max_tokens=1024,
        system="You are a helpful assistant.",
        messages=conversation_history
    )
    
    assistant_message = response.content[0].text
    conversation_history.append({
        "role": "assistant",
        "content": assistant_message
    })
    
    print(f"Assistant: {assistant_message}")
    # Each turn is automatically tracked in AgentFabric
```

### JavaScript: Streaming Responses

```javascript
const Anthropic = require("@anthropic-ai/sdk").default;
const { instrumentAnthropic } = require("agentfabric-sdk");

const client = new Anthropic({
  apiKey: process.env.ANTHROPIC_API_KEY,
});

instrumentAnthropic(client, {
  gatewayUrl: process.env.AGENTFABRIC_GATEWAY_URL,
  webhookToken: process.env.AGENTFABRIC_WEBHOOK_TOKEN,
  tenantId: process.env.AGENTFABRIC_TENANT_ID,
});

const stream = await client.messages.create({
  model: "claude-3-sonnet-20240229",
  max_tokens: 1024,
  stream: true,
  messages: [
    { role: "user", content: "Tell me a story." }
  ],
});

for await (const event of stream) {
  if (event.type === "content_block_delta") {
    process.stdout.write(event.delta.text);
  }
}
// Streaming calls are tracked with complete token counts
```

## Troubleshooting

### No Events Appearing

```python
# Check SDK is instrumented
from agentfabric_sdk import get_status
status = get_status()
print(f"Gateway: {status['gateway_url']}")
print(f"Connected: {status['is_connected']}")

# Enable debug logging
import logging
logging.basicConfig(level=logging.DEBUG)
```

### Token Count Mismatch

Ensure you're passing the same token counts that Anthropic's API returns:

```python
response = client.messages.create(...)
print(f"Input tokens: {response.usage.input_tokens}")
print(f"Output tokens: {response.usage.output_tokens}")
# Use these exact counts in webhook payload
```

### Cost Calculation Wrong

Update Anthropic pricing in **Pricing & Rules** → **Anthropic** to match current rates:

```
claude-3-opus: $0.000015 input, $0.000075 output
claude-3-sonnet: $0.000003 input, $0.000015 output
claude-3-haiku: $0.00000025 input, $0.00000125 output
```

## Support

- Anthropic Docs: https://docs.anthropic.com
- SDK: https://github.com/anthropics/anthropic-sdk-python
- AgentFabric Anthropic Integration: https://docs.agentfabric.dev/integrations/anthropic
- Issues: https://github.com/govagn/agentfabric/issues
