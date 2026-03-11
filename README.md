# AgentFabric

> The compliance-ready observability platform for enterprise AI agents — full-stack tracing, cost governance, and tamper-evident audit for CrewAI, LangGraph, and OpenAI Agents in production.

![Build Status](https://img.shields.io/badge/status-beta-yellow)
![License](https://img.shields.io/badge/license-proprietary-blue)
![Go](https://img.shields.io/badge/go-1.22-blue)
![Rust](https://img.shields.io/badge/rust-1.78-orange)
![Python](https://img.shields.io/badge/python-3.10+-green)

## Why AgentFabric?

Multi-agent systems are notoriously difficult to observe. You get spans from OTLP, but:

- **No framework semantics** — is that span from CrewAI, LangGraph, or raw API call?
- **No cost attribution** — which agent used $500 of your LLM budget?
- **No compliance trail** — auditors ask: "who accessed this agent's memory?"
- **No policy enforcement** — PII leaks from agent outputs go undetected.

AgentFabric solves this end-to-end. It's a production-grade observability platform purpose-built for AI agents.

## Features

### Core Capabilities

- **🔍 Protocol-Native Tracing** — OTLP gRPC + HTTP receivers. Instrument any framework without SDKs.
- **🤖 5 Frameworks Out of the Box** — CrewAI, LangGraph, OpenAI Agents, Google ADK, Anthropic Claude Agents.
- **💰 LLM Cost Attribution** — Token counts + model pricing. Real-time cost per agent, per trace, per model.
- **🚨 Policy Engine** — 5 built-in policies: Sovereignty (no tool execution), Cost Thresholds, Tool Allowlists, PII Output Detection, Rate Limiting.
- **📊 Live Dashboard** — React portal with trace timeline, span waterfall, agent topology, cost breakdown.
- **🔐 Tamper-Evident Audit Log** — SHA-256 hash-chained entries with cryptographic verification. Regulatory-grade compliance trail.
- **🏢 Multi-Tenancy** — Row-level security (RLS) on PostgreSQL. Tenant isolation at the database layer.
- **📡 WebSocket Live Stream** — Real-time span events with pause/resume/filter.

### Enterprise Ready

- **SOC 2 Architecture** — Ready for Type II certification.
- **HIPAA-Compatible** — PII redaction built-in (regex + contextual rules).
- **Kubernetes Deployment** — DaemonSet collector, Deployment api-gateway, Helm charts included.
- **Distributed Tracing** — Kafka → af-core → ClickHouse pipeline for petabyte-scale analytics.
- **Self-Hosted & Cloud** — Run anywhere: Docker Compose, Kubernetes, or managed cloud service.

## Quick Start (5 Minutes)

### Prerequisites

- Docker & Docker Compose
- Go 1.22+
- Node 20+
- Python 3.10+

### Option A: Full Stack (Recommended for First Run)

```bash
git clone https://github.com/rastogivaibhav/AgentFabric.git
cd AgentFabric

# Start all services (Postgres, Redis, Collector, API Gateway, Portal)
docker compose up -d

# Start the portal dev server (hot-reload)
cd portal
npm install --legacy-peer-deps
npm run dev
```

Visit **http://localhost:3000** — portal is live with dev API proxy to `:8080`.

### Option B: Minimal Stack (Dev/Testing)

```bash
docker compose -f deploy/docker/docker-compose.yml up -d
```

Runs only PostgreSQL, Redis, Collector, API Gateway (no Kafka, ClickHouse, or Grafana).

### Send Your First Span

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

Check the portal at **http://localhost:3000/traces** — your span should be there.

---

## Real-World Example: Multi-Agent Research Pipeline

Here's a production-ready CrewAI example that demonstrates AgentFabric observability:

```python
import os
from agentfabric import instrument
from crewai import Agent, Task, Crew
from crewai_tools import tool

# 1. One-line instrumentation (automatically traces all agents)
instrument(
    endpoint=os.getenv("AF_ENDPOINT", "http://localhost:4318"),
    service_name="research-platform",
)

# 2. Define your tools
@tool
def web_search(query: str) -> str:
    """Search the web for information"""
    # Implementation here
    return "Search results..."

@tool
def database_query(query: str) -> str:
    """Query internal database"""
    return "Database results..."

# 3. Create your agents
market_researcher = Agent(
    role="Market Research Analyst",
    goal="Gather comprehensive market intelligence",
    backstory="Expert researcher with 10 years of industry experience",
    tools=[web_search],
    model="gpt-4",
)

data_analyst = Agent(
    role="Data Analyst",
    goal="Synthesize research into actionable insights",
    backstory="Statistical expert and business strategist",
    tools=[database_query],
    model="gpt-4",
)

report_writer = Agent(
    role="Technical Writer",
    goal="Create clear, well-structured reports",
    backstory="Award-winning author with tech background",
    tools=[],
    model="gpt-3.5-turbo",  # Cheaper for writing tasks
)

# 4. Define tasks
research_task = Task(
    description="Conduct market research on AI agents in 2026",
    expected_output="Detailed market analysis with size, growth, and key players",
    agent=market_researcher,
    tools=[web_search],
)

analysis_task = Task(
    description="Analyze research data and identify trends",
    expected_output="Statistical analysis with trend forecasts",
    agent=data_analyst,
    tools=[database_query],
)

reporting_task = Task(
    description="Create executive summary report",
    expected_output="Professional report ready for stakeholders",
    agent=report_writer,
    context=[research_task, analysis_task],
)

# 5. Create and execute crew
research_crew = Crew(
    agents=[market_researcher, data_analyst, report_writer],
    tasks=[research_task, analysis_task, reporting_task],
    verbose=True,
    memory=True,  # Enable memory for context carry-over
)

# 6. Kickoff (automatic tracing happens here)
result = research_crew.kickoff()

# 7. Monitor in AgentFabric Portal
print(f"Report: {result}")
print("\n📊 View detailed traces at http://localhost:3000/traces")
print("📈 View cost breakdown at http://localhost:3000/cost")
print("🤖 View agent performance at http://localhost:3000/agents")
```

**What AgentFabric Captures:**

- **Agent Performance**: Each agent's execution time, tokens used, cost
- **Tool Usage**: Which tools were called, success rate, latency
- **Task Dependencies**: How research_task feeds into analysis_task
- **Cost Attribution**: $0.45 for researcher, $0.12 for analyst, $0.03 for writer
- **Error Tracking**: If web_search fails, it's logged with retry attempts
- **Token Efficiency**: See which agent is most token-efficient
- **Model Selection**: Cost savings from using gpt-3.5 for writing vs gpt-4

**View in Portal:**

```
Dashboard
├── Total Cost (24h): $145.32
├── Total Tokens: 890,452
├── Avg Latency: 2.1s per agent run
├── Agents
│   ├── Market Researcher: 42 runs, $67.20, 5.1s avg
│   ├── Data Analyst: 42 runs, $51.30, 2.3s avg
│   └── Report Writer: 42 runs, $26.82, 1.2s avg
└── Tools
    ├── web_search: 145 calls, 94.2% success rate
    └── database_query: 87 calls, 100% success rate
```

## Python SDK: Automatic Instrumentation

### Install

```bash
pip install agentfabric
```

### Quick Start with CrewAI

```python
from agentfabric import instrument
from crewai import Agent, Task, Crew

# 1. Instrument your environment (one line!)
instrument(
    endpoint="http://localhost:4318",
    service_name="my-crew-squad"
)

# 2. Define your agents
researcher = Agent(
    role="Research Analyst",
    goal="Gather comprehensive market research",
    backstory="Expert at finding and analyzing trends",
    tools=[web_search, data_analyzer]
)

analyst = Agent(
    role="Business Analyst",
    goal="Synthesize research into actionable insights",
    backstory="Strategic thinker with 15 years experience",
    tools=[database_query, report_generator]
)

# 3. Define your tasks
research_task = Task(
    description="Research the AI agent market",
    agent=researcher,
    expected_output="Comprehensive market analysis"
)

analysis_task = Task(
    description="Analyze research and create strategy",
    agent=analyst,
    expected_output="Strategic recommendations"
)

# 4. Create your crew
crew = Crew(
    agents=[researcher, analyst],
    tasks=[research_task, analysis_task],
    verbose=True
)

# 5. Execute — automatic tracing happens here
result = crew.kickoff()
print(result)

# 6. View traces at http://localhost:3000/traces
#    Every agent action, tool call, and LLM invocation is traced
```

### CrewAI Observability Features

Once instrumented, AgentFabric automatically captures:

**Agent-Level Metrics:**
- Agent name, role, backstory
- Time spent thinking vs executing
- Tool success/failure rates
- Tokens consumed per agent
- Cost attribution per agent

**Task-Level Tracing:**
- Task description and expected output
- Subtask execution order
- Dependencies between tasks
- Task duration and status

**Tool Instrumentation:**
- Tool name and input/output
- Execution time
- Errors and retries
- Tool-to-agent call graph

**LLM Instrumentation:**
- Model used per call
- Prompt + completion tokens
- Cost per API call
- Latency histogram

**View in Portal:**
```
Dashboard → Agents Tab
├── researcher
│   ├── Total runs: 42
│   ├── Avg cost: $0.18/run
│   ├── Error rate: 2.3%
│   └── Recent tasks: [List of tasks]
└── analyst
    ├── Total runs: 42
    ├── Avg cost: $0.12/run
    ├── Error rate: 0%
    └── Recent tasks: [List of tasks]
```

### CrewAI Team Observability Example

```python
# Track multiple crews in the same tenant
from agentfabric import instrument

instrument(
    endpoint="http://localhost:4318",
    service_name="enterprise-ai-platform",  # Shared service name
    headers={"X-AF-Tenant": "acme-corp"}    # Tenant isolation
)

# Crew 1: Marketing Team
marketing_crew = Crew(agents=[copywriter, seo_expert], tasks=[...])
result1 = marketing_crew.kickoff()  # Traced as "marketing_crew"

# Crew 2: Customer Support Team
support_crew = Crew(agents=[support_agent, escalation_agent], tasks=[...])
result2 = support_crew.kickoff()  # Traced as "support_crew"

# View in Portal:
# - Compare performance across crews
# - Allocate budget per team
# - Monitor error rates by team
# - Track tool usage patterns
```

### Advanced: Custom Spans for Nested Logic

```python
from agentfabric import trace_tool_call, agent_span

# Instrument a custom function
@trace_tool_call("custom_validator")
def validate_research(data: dict):
    # This creates a span under the active trace
    # Automatically tracks execution time and errors
    return data

# Or use context manager for fine-grained control
with agent_span("data_processing", {"stage": "preprocessing"}):
    processed = preprocess(raw_data)
    # Nested spans are automatically linked in the trace DAG
```

### Supported Frameworks

| Framework | Version | Status | Features |
|-----------|---------|--------|----------|
| **CrewAI** | 0.30+ | ✅ GA | Agent roles, tasks, tools, memory |
| **LangGraph** | 0.1+ | ✅ GA | State graphs, cycles, memory |
| **OpenAI** | 1.0+ | ✅ GA | Chat completions, tool use |
| **Anthropic** | 0.7+ | ✅ GA | Claude models, tool_use |
| **Google ADK** | 0.1+ | ✅ GA | Generative agents, looping |

## Table of Contents

- [Why AgentFabric?](#why-agentfabric)
- [Features](#features)
- [Quick Start](#quick-start-5-minutes)
- [Python SDK](#python-sdk-automatic-instrumentation)
- [CrewAI Quick Start](#quick-start-with-crewai)
- [CrewAI Observability](#crewai-observability-features)
- [Configuration](#configuration)
- [API Reference](#api-reference)
- [Policy Engine](#policy-engine)
- [Cost Control](#crewai-team-governance--cost-control)
- [Debugging](#debugging-crewai-teams-with-agentfabric)
- [Best Practices](#best-practices-for-crewai--agentfabric)
- [Troubleshooting](#troubleshooting)
- [Deployment](#deployment)
- [Roadmap](#roadmap)

---

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                    Python Agent Code                         │
│              (CrewAI / LangGraph / OpenAI)                   │
└────────────────────┬─────────────────────────────────────────┘
                     │ OTLP (gRPC or HTTP)
                     ↓
        ┌────────────────────────┐
        │    Collector (Go)      │
        │                        │
        │ • OTLP gRPC :4317      │
        │ • OTLP HTTP :4318      │
        │ • PII Scrubbing        │
        │ • Framework Detection  │
        │ • Cost Computation     │
        └────────────┬───────────┘
                     │
        ┌────────────┴──────────────────┐
        │                               │
        ↓                               ↓
  API Gateway (Go)               af-core (Rust)
  • REST/WebSocket API           • Kafka Consumer
  • JWT Auth                      • ClickHouse Writes
  • Multi-tenancy RLS             • Policy Evaluation
  • Prometheus Metrics            • SHA-256 Audit Log
        │
  ┌─────┴──────────────┐
  ↓                    ↓
PostgreSQL           Redis
(Spans, Runs,        (Cache,
 Traces, Audit)      WebSocket
                      Pub/Sub)

        ↑
        │ GraphQL/REST API
        │
  ┌─────┴──────────────┐
  │  Portal (React)    │
  │                    │
  │ • Dashboard        │
  │ • Traces           │
  │ • Agents           │
  │ • Cost Analysis    │
  │ • Live Stream      │
  └────────────────────┘
```

## Configuration

### Environment Variables

**API Gateway** (`api-gateway/cmd/server/main.go`):
```bash
AF_JWT_SECRET=<32+ byte hex secret>          # REQUIRED in production
AF_AUTH_DISABLED=true|false                  # Default: false (auth required)
AF_CORS_ORIGINS=http://localhost:3000,...    # Comma-separated allowed origins
DATABASE_URL=postgres://user:pass@host/db    # Default: localhost
REDIS_URL=redis://localhost:6379             # Default: localhost
LISTEN_ADDR=:8080                            # Default: :8080
```

**Collector** (`collector/cmd/collector/main.go`):
```bash
AF_GRPC_ADDR=:4317                           # gRPC listener
AF_HTTP_ADDR=:4318                           # HTTP listener
AF_GATEWAY_ENDPOINT=http://api-gateway:8080  # Where to forward spans
AF_AUTH_REQUIRE_AUTH=true|false               # Require JWT for ingest
AF_GATEWAY_JWT_SECRET=<matching api-gateway secret>
```

**Portal** (`.env.local`):
```bash
VITE_API_URL=http://localhost:8080           # Backend URL
```

## API Reference

### REST Endpoints

**Traces:**
- `GET /api/v1/traces` — List traces with filtering
- `GET /api/v1/traces/{traceId}` — Get full trace with spans
- `GET /api/v1/traces/{traceId}/graph` — Span DAG topology
- `GET /api/v1/traces/{traceId}/timeline` — Waterfall timeline
- `GET /api/v1/traces/{traceId}/cost` — Cost breakdown

**Agents:**
- `GET /api/v1/agents` — List all agents
- `GET /api/v1/agents/{agentId}` — Agent details (run count, cost, error rate)
- `GET /api/v1/agents/{agentId}/runs` — Runs for specific agent
- `GET /api/v1/agents/{agentId}/metrics` — 24h metrics snapshot
- `GET /api/v1/agents/{agentId}/topology` — Agent call graph

**Runs:**
- `GET /api/v1/runs` — List runs with filtering
- `GET /api/v1/runs/{runId}` — Run details
- `GET /api/v1/runs/{runId}/children` — Child runs (nested execution)
- `POST /api/v1/runs/{runId}/feedback` — Submit human feedback (score + comment)

**Analytics:**
- `GET /api/v1/analytics/overview?since=24h` — Dashboard KPIs
- `GET /api/v1/analytics/frameworks` — Traces by framework
- `GET /api/v1/analytics/cost?since=24h` — Cost breakdown by model
- `GET /api/v1/analytics/errors?since=24h` — Error frequency by framework

**Live Stream:**
- `WebSocket /api/v1/stream/live` — Real-time span events (JSON)

**System:**
- `GET /healthz` — Health check
- `GET /metrics` — Prometheus metrics

### Query Parameters

```
?since=24h|7d|30d        # Time window for aggregations
?framework=crewai        # Filter by framework
?agent=my-agent          # Filter by agent name
?model=gpt-4             # Filter by LLM model
?status=ok|error         # Filter by trace status
?limit=50                # Pagination (1-200, default 50)
```

### Response Format

All responses are JSON. Errors return `{"error": "message"}` with appropriate HTTP status codes.

**Example Trace:**
```json
{
  "id": "trace-uuid",
  "root_span_name": "agent_kickoff",
  "framework": "crewai",
  "start_time": "2026-03-10T12:34:56Z",
  "duration_ns": 5000000000,
  "span_count": 42,
  "error_count": 0,
  "total_cost_usd": 0.0234,
  "total_tokens": 1850,
  "status": "ok",
  "spans": [...]
}
```

## CrewAI Team Governance & Cost Control

### Cost Attribution by Team

Track spending across multiple CrewAI teams in a single dashboard:

```python
# Production crew (expensive, needs budget controls)
production_crew = Crew(
    agents=[senior_researcher, lead_analyst],
    tasks=[...],
    max_rpm=100  # Rate limit configuration
)

# Experimental crew (learning phase, lower cost threshold)
experimental_crew = Crew(
    agents=[junior_researcher],
    tasks=[...],
    max_rpm=10
)

# Monitor in Portal:
# Cost Breakdown
# ├── Production Team: $2,340.12/month (45% of budget)
# ├── Experimental: $234.01/month (4% of budget)
# └── Support: $1,200.00/month (23% of budget)
```

### Policy Governance for Crews

Enforce team-level constraints:

```json
{
  "policy": "cost_threshold",
  "config": {
    "per_run_limit_usd": 5.00,
    "daily_limit_usd": 500.00,
    "enforcement": "hard_stop"  // Crew fails if exceeded
  },
  "scope": "crew:marketing"  // Apply to specific crew
}
```

### Multi-Agent Task Monitoring

Watch task execution across agents:

```
Portal → Traces Tab
Task: "Research Market Trends"
├── Agent: researcher
│   ├── Duration: 45s
│   ├── Tool calls: 8 (web_search x6, data_analyzer x2)
│   ├── Cost: $0.34
│   └── Status: ✅ Completed
├── Agent: analyst
│   ├── Duration: 23s
│   ├── Tool calls: 4 (database_query x3, report_gen x1)
│   ├── Cost: $0.12
│   └── Status: ✅ Completed
└── Task Total: 68s, $0.46, 12 tool calls
```

---

## Policy Engine

AgentFabric's policy engine evaluates 5 built-in policies per span:

### 1. Sovereignty Policy
Prevents specific tools from executing based on runtime allowlist.

```json
{
  "name": "sovereignty",
  "config": {
    "forbidden_tools": ["delete_user", "modify_system"]
  },
  "decision": "DENY",
  "reason": "tool 'delete_user' is not in allowlist"
}
```

### 2. Cost Threshold Policy
Halts execution if cumulative cost exceeds limit.

```json
{
  "name": "cost_threshold",
  "config": {
    "max_cost_usd": 10.0,
    "window_minutes": 60
  },
  "decision": "ALLOW",
  "reason": "current_cost: $2.34 < limit: $10.00"
}
```

### 3. Tool Allowlist Policy
Restricts execution to a whitelist of tools only.

```json
{
  "name": "tool_allowlist",
  "config": {
    "allowed_tools": ["search", "calculator", "send_email"]
  },
  "decision": "DENY",
  "reason": "tool 'web_scraper' not in allowlist"
}
```

### 4. PII Output Policy
Detects and redacts PII patterns (SSN, credit card, email, phone).

```json
{
  "name": "pii_output",
  "config": {
    "patterns": ["ssn", "credit_card", "email", "phone"]
  },
  "decision": "REDACT",
  "reason": "detected SSN pattern in output"
}
```

### 5. Rate Limit Policy
Per-tenant, per-agent request throttling.

```json
{
  "name": "rate_limit",
  "config": {
    "requests_per_minute": 100,
    "burst_window_sec": 10
  },
  "decision": "ALLOW",
  "reason": "rate: 45 req/min < limit: 100 req/min"
}
```

All policy decisions are logged to the immutable audit trail.

## Deployment

### Docker Compose (Development)

```bash
docker compose up -d
# Access portal at http://localhost:3000
```

### Kubernetes (Production)

```bash
kubectl apply -f deploy/k8s/agentfabric.yaml
# or
helm install agentfabric deploy/helm/
```

See [deploy/helm/values.yaml](deploy/helm/values.yaml) for tuning options.

### Self-Hosted (On-Premise)

```bash
# Set AF_JWT_SECRET before deploying
export AF_JWT_SECRET=$(openssl rand -hex 32)

docker compose -f deploy/docker/docker-compose.yml up -d
```

## Roadmap

- [ ] Web UI login page with OIDC/SSO
- [ ] Policy marketplace (share/sell custom policies)
- [ ] Cost optimization suggestions (LLM router recommendations)
- [ ] Span search with SQL-like DSL
- [ ] Agent DAG visualization (multi-agent orchestration)
- [ ] Kafka topic backpressure handling
- [ ] ClickHouse integration for petabyte-scale analytics
- [ ] OpenTelemetry Collector distribution with AgentFabric processor

## Contributing

1. Fork the repo
2. Create a feature branch (`git checkout -b feature/my-feature`)
3. Commit changes (`git commit -am 'Add feature'`)
4. Push to branch (`git push origin feature/my-feature`)
5. Open a Pull Request

## Debugging CrewAI Teams with AgentFabric

### Find Slow Tasks

```python
# Query dashboard or API
curl 'http://localhost:8080/api/v1/analytics/overview?since=24h' \
  -H 'Authorization: Bearer <token>'

# Response shows:
# {
#   "avg_latency_ms": 3200,
#   "spans_per_second": 12,
#   "total_cost_usd": 452.34,
#   "framework_counts": { "crewai": 156 }
# }

# Then drill down:
# Portal → Traces → Filter by framework=crewai
# Sort by duration → Identify bottleneck agents
```

### Debug Tool Failures

```python
# In Portal, go to a trace with errors
# Trace ID: abc-123-def
#
# Span: "tool_call:web_search"
# Status: ❌ ERROR
# Error Message: "Rate limit exceeded"
#
# Context:
# ├── Tool: web_search
# ├── Input: {"query": "AI agents 2026"}
# ├── Duration: 2.3s
# ├── Attempt: 1/3
# └── Next retry in: 5s
```

### Monitor PII Leakage

AgentFabric automatically detects when agent outputs contain PII:

```python
# Example: Customer support agent accidentally includes SSN
support_agent = Agent(
    role="Customer Service",
    backstory="Helpful and friendly"
)

# In Portal, Policy Audit Log shows:
# Timestamp: 2026-03-10T12:34:56Z
# Policy: pii_output
# Decision: REDACT
# Reason: "Detected SSN pattern (###-##-####)"
# Span: trace-abc123 / span-xyz789
# Agent: support_agent
# Action Taken: Output scrubbed before returning to user
```

### Cost Anomaly Detection

```python
# Portal alerts when cost exceeds baseline
# Example:
# Alert: "Cost spike detected"
# Current: $120/hour (baseline: $45/hour)
# Cause: researcher agent making 3x more API calls
# Recommendation: Check for stuck loops or verbose prompts
```

---

## Best Practices for CrewAI + AgentFabric

### 1. Instrument Once, Trace Everything

```python
# Put this at app startup
from agentfabric import instrument

instrument(
    endpoint=os.getenv("AF_COLLECTOR_ENDPOINT"),
    service_name="my-app",
    headers={"X-API-Key": os.getenv("AF_API_KEY")}
)

# Now ALL CrewAI agents (and other frameworks) are traced automatically
# No need to modify agent definitions
```

### 2. Use Meaningful Agent Roles

```python
# ✅ Good: Specific roles
researcher = Agent(role="Market Research Analyst")
analyst = Agent(role="Financial Analyst")

# ❌ Avoid: Generic names
agent1 = Agent(role="Agent 1")
agent2 = Agent(role="Agent 2")

# In Portal: Easy to identify and correlate by agent role
```

### 3. Set Expectations for Tasks

```python
task = Task(
    description="Research AI agent market trends",
    agent=researcher,
    expected_output="A comprehensive report with market size, growth rate, and key players",
    # AgentFabric captures if actual output matches expected output quality
)
```

### 4. Monitor Tool Success Rates

```python
# Portal automatically calculates:
# Tool Success Rate = (Successful calls) / (Total calls)

# High failure rate (>20%) on a tool suggests:
# - Tool API is down
# - Agent prompts need refinement
# - Rate limiting is too aggressive

# Portal alerts on degradation
```

### 5. Budget by Crew, Not by LLM API

```python
# Instead of:
# - OpenAI budget: $1000/month

# Use:
# - Marketing Crew: $200/month (budget: $250)
# - Support Crew: $150/month (budget: $200)
# - R&D Crew: $500/month (budget: $600)

# This aligns cost accountability with business teams
```

---

## Troubleshooting

### Traces Not Appearing in Portal

**Check 1: Is the collector running?**
```bash
curl http://localhost:4318/v1/traces -v
# Should return 400 or 200, not "Connection refused"
```

**Check 2: Is your app sending spans?**
```python
import logging
logging.basicConfig(level=logging.DEBUG)
# Should see debug logs from agentfabric SDK
```

**Check 3: Check API Gateway logs**
```bash
docker logs agentfabric-api-gateway
# Look for "span submission failed" errors
```

### High Latency on Portal

**Check:**
- Number of spans: `curl http://localhost:8080/metrics | grep span`
- Database connection pool: `docker exec agentfabric-postgres psql -c "SELECT * FROM pg_stat_activity"`
- Redis cache hit rate: `redis-cli INFO stats`

### Cost Attribution Wrong

**Check:**
- Model pricing table: `curl http://localhost:8080/api/v1/analytics/frameworks`
- Token counts in spans: `Portal → Traces → Select trace → Inspect span attributes`

---

## Support

- **Issues** — GitHub Issues for bugs and feature requests
- **Discussions** — GitHub Discussions for questions and ideas
- **Community Slack** — [Join AgentFabric Slack](https://slack.agentfabric.io) (coming soon)
- **Email** — support@agentfabric.io (coming soon)
- **Docs** — [Full documentation](https://docs.agentfabric.io) (coming soon)

## License

Proprietary — All rights reserved. See LICENSE file for details.

---

**Built with ❤️ for AI agents in production.**
