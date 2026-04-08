# Govagn Local Demo Guide

Get Govagn running in 5 minutes and trace your first AI agent.

## Prerequisites

- Docker & Docker Compose
- Python 3.10+
- curl or Postman (for manual span testing)
- Git

## Step 1: Start the Platform (2 minutes)

```bash
cd C:\Users\vrast\Documents\Agentic Code\files

# Start all services
docker compose up -d

# Wait for services to be healthy
docker compose ps
```

Expected output:
```
NAME                    STATUS
govagn-postgres    Up (healthy)
govagn-redis       Up (healthy)
govagn-collector   Up (healthy)
govagn-api-gateway Up (healthy)
govagn-portal      Up (healthy)
```

If any service shows "unhealthy", wait 10 seconds and check again:
```bash
docker compose ps
```

## Step 2: Open the Portal (1 minute)

```bash
# Open the portal in your browser
start http://localhost:3000
```

Or manually navigate to: **http://localhost:3000**

You should see:
- **Dashboard** — KPI cards for traces, cost, tokens, error rate
- **Traces** tab — Currently empty (no spans yet)
- **Agents** tab — Currently empty
- **Cost** tab — Currently empty
- **Live Stream** tab — Real-time event feed

---

## Step 3A: Send Your First Span (Manual Test)

Send a test span via curl:

```bash
curl -X POST http://localhost:4318/v1/traces \
  -H 'Content-Type: application/json' \
  -d '{
    "resourceSpans": [{
      "resource": {
        "attributes": [
          {"key":"service.name","value":{"stringValue":"demo-agent"}}
        ]
      },
      "scopeSpans": [{
        "spans": [{
          "traceId": "aabbccddeeff00112233445566778899",
          "spanId": "aabbccddaabbccdd",
          "name": "llm.invoke",
          "startTimeUnixNano": "1700000000000000000",
          "endTimeUnixNano": "1700000001500000000",
          "attributes": [
            {"key":"gen_ai.system","value":{"stringValue":"crewai"}},
            {"key":"gen_ai.usage.prompt_tokens","value":{"intValue":120}},
            {"key":"gen_ai.usage.completion_tokens","value":{"intValue":80}},
            {"key":"gen_ai.usage.cost_usd","value":{"doubleValue":0.0042}}
          ]
        }]
      }]
    }]
  }'
```

Expected response:
```
{}
```

**Check the portal:**
- Go to **http://localhost:3000/traces**
- You should see your trace with ID `aabbccddeeff00112233445566778899`
- Click it to see the span details

---

## Step 3B: Trace a CrewAI Agent (Full Demo)

### 1. Install the SDK

```bash
cd C:\Users\vrast\Documents\Agentic Code\files
pip install govagn crewai crewai-tools
```

### 2. Create a demo script

Create `demo_crew.py`:

```python
import os
from govagn import instrument
from crewai import Agent, Task, Crew
from crewai_tools import tool

# 1. Instrument (one line!)
instrument(
    endpoint="http://localhost:4318",
    service_name="demo-crew"
)

# 2. Define a tool
@tool
def search_market(query: str) -> str:
    """Search for market information"""
    return f"Market data for: {query} - Growth: 45%, Market Size: $2.3B"

# 3. Create an agent
analyst = Agent(
    role="Market Analyst",
    goal="Analyze market trends",
    backstory="Expert analyst with 10 years experience",
    tools=[search_market],
    model="gpt-4"
)

# 4. Define a task
task = Task(
    description="Analyze the AI agent market",
    agent=analyst,
    expected_output="Market analysis report"
)

# 5. Create and run the crew
crew = Crew(agents=[analyst], tasks=[task], verbose=True)

# 6. Execute — automatic tracing happens here!
print("🚀 Starting crew execution (with automatic tracing)...\n")
result = crew.kickoff()

print(f"\n✅ Crew execution complete!\n")
print(f"Result:\n{result}\n")
print("📊 View traces at http://localhost:3000/traces")
print("🤖 View agent performance at http://localhost:3000/agents")
```

### 3. Run the demo

```bash
python demo_crew.py
```

Expected output:
```
🚀 Starting crew execution (with automatic tracing)...

Agent: analyst
Task: Analyze the AI agent market
...
✅ Crew execution complete!

Result:
Market analysis report...

📊 View traces at http://localhost:3000/traces
🤖 View agent performance at http://localhost:3000/agents
```

### 4. View the traces in the portal

**In a new terminal:**
```bash
start http://localhost:3000/traces
```

You should see:
- ✅ New trace from your crew execution
- Framework: `crewai`
- Duration: ~2-5 seconds
- Cost: ~$0.01-$0.05
- Status: `ok`

**Click the trace to see:**
- Span timeline (LLM call, tool invocation)
- Cost breakdown
- Token usage
- Agent execution flow

---

## Step 4: Explore the Dashboard

### Dashboard (Home)

```
http://localhost:3000
```

Shows:
- **Total Traces (24h)**: Number of agent runs
- **Total Cost**: $ spent on LLM calls
- **Total Tokens**: Input + output tokens consumed
- **Error Rate**: % of failed traces
- **Framework Breakdown**: Pie chart (CrewAI vs LangGraph, etc.)

### Agents Tab

```
http://localhost:3000/agents
```

Shows:
- **Agent Name** → Role (Market Analyst)
- **Total Runs** → 1
- **Avg Cost** → $0.02/run
- **Error Rate** → 0%
- **Recent Traces** → Table of recent executions

### Cost Tab

```
http://localhost:3000/cost
```

Shows:
- **Total Cost (24h)** → $0.02
- **Total Tokens** → ~200
- **Avg Cost/Trace** → $0.02
- **Cost by Framework** → Bar chart (CrewAI: $0.02)
- **Framework Share** → Progress bars

### Live Stream Tab

```
http://localhost:3000/live
```

Shows:
- Real-time span events as they arrive
- Can pause/resume/clear
- Filters by type (span, error, policy, etc.)

---

## Advanced Demo: Multi-Agent Crew

Create `demo_crew_advanced.py`:

```python
from govagn import instrument
from crewai import Agent, Task, Crew
from crewai_tools import tool

instrument(endpoint="http://localhost:4318", service_name="research-crew")

@tool
def web_search(query: str) -> str:
    return "Search results for AI agents market..."

@tool
def database_query(query: str) -> str:
    return "Database results with historical trends..."

# Agent 1
researcher = Agent(
    role="Research Analyst",
    goal="Find market information",
    backstory="Expert researcher",
    tools=[web_search],
    model="gpt-4"
)

# Agent 2
analyst = Agent(
    role="Data Analyst",
    goal="Analyze trends",
    backstory="Statistical expert",
    tools=[database_query],
    model="gpt-4"
)

# Tasks
research_task = Task(
    description="Research AI agent market",
    agent=researcher,
    expected_output="Market research report"
)

analysis_task = Task(
    description="Analyze research data",
    agent=analyst,
    expected_output="Analysis with forecasts"
)

# Crew
crew = Crew(
    agents=[researcher, analyst],
    tasks=[research_task, analysis_task],
    verbose=True,
    memory=True
)

result = crew.kickoff()
print(f"✅ Multi-agent result:\n{result}")
print("📊 View multi-agent trace at http://localhost:3000/traces")
```

Run it:
```bash
python demo_crew_advanced.py
```

**In the portal, you'll see:**
- Single trace with 2 agent spans
- Tool calls from both agents
- Cost breakdown per agent
- Task dependency graph

---

## Demo Scenarios

### 1. Cost Attribution

Send multiple crew runs:
```bash
for i in {1..5}; do
  python demo_crew.py
done
```

**In portal:**
- **Cost tab** shows cumulative cost
- **Agents tab** shows 5 runs per agent
- Can see cost trend over time

### 2. Error Handling

Modify `demo_crew.py` to inject an error:

```python
@tool
def failing_search(query: str) -> str:
    raise Exception("API timeout - simulated failure")
```

Run it:
```bash
python demo_crew.py
```

**In portal:**
- Trace shows status: ❌ error
- Error message visible in span details
- Error rate on dashboard updates

### 3. Policy Governance

Monitor PII redaction:

```python
@tool
def get_customer_info(id: str) -> str:
    # Intentionally leak PII
    return f"Customer SSN: 123-45-6789, Email: john@example.com"
```

**In portal, Policy Audit Log:**
- `policy_name: pii_output`
- `decision: REDACT`
- `reason: "Detected SSN and email patterns"`

---

## Troubleshooting

### Portal shows "Connection refused"

```bash
# Check if API Gateway is running
curl http://localhost:8080/healthz

# Should return: {"status":"ok"}
```

If not:
```bash
# Check logs
docker logs govagn-api-gateway

# Restart
docker compose restart api-gateway
```

### Traces not appearing in portal

1. Check collector is running:
```bash
curl http://localhost:4318/v1/traces -v
# Should return 400 (missing body), not "Connection refused"
```

2. Check API Gateway logs:
```bash
docker logs govagn-api-gateway | grep -i error
```

3. Check database:
```bash
docker exec govagn-postgres psql -U postgres -d govagn -c "SELECT COUNT(*) FROM spans;"
```

### High memory usage

```bash
# Check container sizes
docker compose stats

# If high, restart
docker compose down
docker compose up -d
```

---

## Next Steps

After the demo:

1. **Integrate with your agents** — Add `instrument()` to your CrewAI code
2. **Set up cost alerts** — Configure cost thresholds in Policy tab
3. **Monitor in production** — Deploy collector on your production machines
4. **Explore the API** — Use REST endpoints for custom dashboards

---

## Key Demo Points to Show

✅ **One-line instrumentation** — No code changes to CrewAI
✅ **Automatic framework detection** — CrewAI recognized automatically
✅ **Cost attribution** — Per-agent, per-tool cost tracking
✅ **Multi-agent support** — Crew with multiple agents
✅ **Real-time tracing** — Spans appear in portal as they execute
✅ **Error tracking** — Failed spans logged with stack traces
✅ **Policy enforcement** — PII automatically redacted
✅ **Enterprise features** — Multi-tenancy, audit log, RLS

---

## Demo Script (Copy-Paste)

```bash
# Terminal 1: Start platform
cd C:\Users\vrast\Documents\Agentic Code\files
docker compose up -d
sleep 10

# Terminal 2: Open portal
start http://localhost:3000

# Terminal 3: Send test span
curl -X POST http://localhost:4318/v1/traces \
  -H 'Content-Type: application/json' \
  -d '{
    "resourceSpans": [{
      "resource": {
        "attributes": [
          {"key":"service.name","value":{"stringValue":"demo-agent"}}
        ]
      },
      "scopeSpans": [{
        "spans": [{
          "traceId": "aabbccddeeff00112233445566778899",
          "spanId": "aabbccddaabbccdd",
          "name": "llm.invoke",
          "startTimeUnixNano": "1700000000000000000",
          "endTimeUnixNano": "1700000001500000000",
          "attributes": [
            {"key":"gen_ai.system","value":{"stringValue":"crewai"}},
            {"key":"gen_ai.usage.prompt_tokens","value":{"intValue":120}},
            {"key":"gen_ai.usage.completion_tokens","value":{"intValue":80}},
            {"key":"gen_ai.usage.cost_usd","value":{"doubleValue":0.0042}}
          ]
        }]
      }]
    }]
  }'

# Check portal: http://localhost:3000/traces
# You should see the trace!
```

---

**That's it! You now have a working demo of Govagn.**

Questions? Check the README or run `docker compose logs -f api-gateway` for real-time debugging.
