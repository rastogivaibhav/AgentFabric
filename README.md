# AgentFabric

> Enterprise observability and FinOps platform for production AI agents.
> Zero-code instrumentation. Real-time tracing. Hard budget limits. On-premise deployable.

[![Version](https://img.shields.io/badge/version-v1.2.0-blue)](CHANGELOG.md)
[![Build](https://img.shields.io/badge/build-passing-brightgreen)](#)
[![Tests](https://img.shields.io/badge/tests-121%20passing-brightgreen)](#)
[![Go](https://img.shields.io/badge/go-1.22-blue)](#)
[![Rust](https://img.shields.io/badge/rust-1.76-orange)](#)
[![Python](https://img.shields.io/badge/python-3.10+-green)](#)
[![License](https://img.shields.io/badge/license-proprietary-blue)](#)

---

## What Is AgentFabric?

AgentFabric is an **AI agent observability and cost governance platform** — purpose-built for enterprise teams running autonomous agents in production.

Think Datadog + OpenCost, but designed from the ground up for agentic AI workloads:

- **Observe** every LLM call, tool use, and handoff across all your agent frameworks
- **Attribute costs** to individual agents, traces, models, and tenants in real time
- **Enforce budgets** — hard monthly token/cost limits per tenant with automatic 429 enforcement
- **Govern** with a policy engine that detects PII, evaluates compliance rules, and writes a tamper-evident audit trail
- **Deploy anywhere** — self-hosted Docker Compose, Kubernetes/Helm, or air-gapped on-premise

---

## Why Not LangSmith / Langfuse / Arize?

| Capability | **AgentFabric** | LangSmith | Langfuse | Arize Phoenix | Helicone |
|---|:---:|:---:|:---:|:---:|:---:|
| Zero-code auto-instrumentation | ✅ | ❌ | ❌ | ❌ | ❌ |
| 5 frameworks, one SDK | ✅ | LangChain only | ✅ | ✅ | ✅ |
| Hard budget enforcement (HTTP 429) | ✅ | ❌ | ❌ | ❌ | Partial |
| On-premise / air-gapped | ✅ | ❌ | ✅ | ✅ | ❌ |
| OTel-native (vendor neutral) | ✅ | ❌ | Partial | ✅ | ❌ |
| Real-time WebSocket live stream | ✅ | ❌ | ❌ | ❌ | ❌ |
| Kafka durability + Rust policy engine | ✅ | ❌ | ❌ | ❌ | ❌ |
| ClickHouse analytics tier | ✅ | ❌ | ❌ | ❌ | ❌ |
| Tamper-evident audit chain | ✅ | ❌ | ❌ | ❌ | ❌ |
| RBAC + ABAC + OIDC SSO | ✅ | Partial | Partial | ❌ | ❌ |

**Unique differentiators:**
1. **Zero-code** — `pip install agentfabric` instruments every agent in the environment automatically via Python's `sitecustomize.py`. Developers change nothing.
2. **OTel-native** — standard OTLP gRPC/HTTP. Works with any existing Jaeger, Grafana Tempo, or Datadog pipeline alongside AgentFabric.
3. **Full stack on-premise** — every component ships as a container. No data leaves your network.
4. **FinOps enforcement** — not just reporting. Budget violations block ingest with HTTP 429 before a runaway agent drains your budget.

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                     Your AI Agent Code                                  │
│         (CrewAI · LangGraph · OpenAI Agents · Google ADK · Claude)      │
└──────────────────────────────┬──────────────────────────────────────────┘
                               │  OTLP gRPC :4317  or  HTTP :4318
                               ▼
              ┌────────────────────────────────┐
              │        Collector  (Go)          │
              │  ✦ OTLP gRPC + HTTP receivers   │
              │  ✦ Framework auto-detection     │
              │  ✦ PII scrubbing (7 patterns)   │
              │  ✦ LLM cost computation         │
              │  ✦ Span enrichment + run IDs    │
              └────────────────┬───────────────┘
                               │  POST /internal/ingest  (JSON)
                               ▼
              ┌────────────────────────────────┐
              │      API Gateway  (Go/Chi)      │
              │  ✦ JWT auth · RBAC/ABAC         │
              │  ✦ Per-tenant rate limiting     │
              │  ✦ Budget enforcement (HTTP 429)│
              │  ✦ WebSocket hub (live stream)  │
              │  ✦ Prometheus /metrics          │
              └──────┬──────────────┬───────────┘
                     │              │
          ┌──────────▼───┐    ┌─────▼─────────────────────┐
          │  PostgreSQL   │    │         Kafka              │
          │  Hot storage  │    │   topic: af.spans          │
          │  Spans/Runs   │    └─────────────┬─────────────┘
          │  Audit log    │                  │ consume
          │  Users/Tenants│    ┌─────────────▼─────────────┐
          │  RLS enforced │    │      af-core  (Rust)       │
          └───────┬───────┘    │  ✦ Policy evaluation       │
                  │            │  ✦ PII detection (layer 2)  │
          ┌───────▼───────┐    │  ✦ allow / review / deny   │
          │     Redis      │    └─────────────┬─────────────┘
          │  Rate limiting │                  │ write
          │  WS pub/sub    │    ┌─────────────▼─────────────┐
          │  Trace cache   │    │     ClickHouse             │
          └───────┬───────┘    │  Analytics tier             │
                  │            │  Materialized views         │
          ┌───────▼───────┐    │  90-day TTL · p95 latency  │
          │  Portal (React)│    │  Cost by model/hour        │
          │  Dashboard     │    │  Policy violation counts   │
          │  Live Stream   │    └────────────────────────────┘
          │  Traces/Agents │
          │  Cost / Budget │
          └────────────────┘
```

### Data Flow Summary

```
Agent SDK
  → Collector  [detect · scrub · enrich · compute cost]
  → API Gateway → PostgreSQL          (portal reads — <100ms)
               → WebSocket            (live stream — real-time)
               → Kafka af.spans
                    → af-core         [policy evaluation]
                         → ClickHouse (analytics — p95 queries)
```

### Component Inventory

| Component | Language | Role |
|---|---|---|
| **Collector** | Go 1.22 | OTLP ingestion, framework detection, PII scrub, cost compute |
| **API Gateway** | Go 1.22 / Chi | REST + WebSocket, auth, rate limiting, budget enforcement |
| **af-core** | Rust 1.76 | Kafka consumer, policy engine, ClickHouse writer |
| **Portal** | React 18 + TypeScript | Dashboard, traces, live stream, cost, budget management |
| **Agent SDK** | Python 3.10+ | Auto-instrumentation for 5 frameworks via `sitecustomize.py` |
| **PostgreSQL 16** | — | Hot storage: spans, runs, users, audit log (Row-Level Security) |
| **ClickHouse 24** | — | Analytics: token usage, cost/hour, latency p95/p99, policy violations |
| **Kafka + Zookeeper** | — | Durable span buffer between api-gateway and af-core |
| **Redis 7** | — | Rate limiting, WebSocket pub/sub, trace cache |

---

## Quick Start — Docker Compose (5 minutes)

### Prerequisites

| Tool | Version |
|---|---|
| Docker + Docker Compose | 24+ |
| Python | 3.10+ (for SDK) |

### 1. Start the full stack

```bash
git clone https://github.com/your-org/agentfabric.git
cd agentfabric

# Start all 9 services
docker compose up -d

# Watch startup (takes ~30s for Kafka health check)
docker compose logs -f api-gateway
```

Services started:
- **Portal** → http://localhost:3000 *(dashboard)*
- **API Gateway** → http://localhost:8080 *(REST + WebSocket)*
- **Collector gRPC** → localhost:4317
- **Collector HTTP** → localhost:4318/v1/traces
- **ClickHouse HTTP** → localhost:8123
- **Kafka** → localhost:9092

Default login: `admin` / `admin`

### 2. Install the Python SDK

```bash
pip install agentfabric
```

### 3. Instrument your agent — zero code changes

Set three environment variables before running your agent:

```bash
export AF_ENDPOINT=http://localhost:4318
export AF_TENANT_ID=my-project
export AF_SERVICE_NAME=my-agent

python my_agent.py   # all LLM calls traced automatically
```

That's it. No `import agentfabric`, no decorator, no wrapper. The SDK hooks in via `sitecustomize.py` before your code starts.

### 4. Verify in the portal

Open **http://localhost:3000/traces** — your agent's spans appear within seconds.

---

## Python SDK — Framework Coverage

The SDK auto-instruments all five frameworks without any code changes:

```python
# You write this:
from crewai import Agent, Task, Crew
crew = Crew(agents=[...], tasks=[...])
crew.kickoff()

# AgentFabric captures:
# • Every agent invocation (name, role, model, tokens, cost)
# • Every tool call (name, input, output, latency)
# • Every LLM call (model, prompt tokens, completion tokens, cost_usd)
# • Full trace tree with parent-child span relationships
```

| Framework | Auto-instrumented | Notes |
|---|:---:|---|
| CrewAI | ✅ | Agents, tasks, tool calls |
| LangGraph | ✅ | Node execution, state transitions |
| OpenAI Agents SDK | ✅ | Runs, function calls, handoffs |
| Google ADK | ✅ | Agent sessions, model calls |
| Anthropic Claude | ✅ | Messages API calls |

### Manual instrumentation (optional)

```python
import agentfabric

agentfabric.instrument(
    endpoint="http://localhost:4318",
    tenant_id="production",
    service_name="my-agent",
    frameworks=["crewai", "langgraph"],  # or "all"
)
```

---

## Budget Enforcement

Set a hard monthly limit for any tenant — the gateway returns HTTP 429 when exceeded:

```bash
# Set a $500/month hard limit for tenant "acme-corp"
curl -X PUT http://localhost:8080/api/v1/budgets/acme-corp \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "monthly_cost_usd": 500.00,
    "monthly_tokens": 10000000,
    "alert_threshold": 0.80,
    "hard_limit": true,
    "reset_day": 1
  }'

# Check current usage
curl http://localhost:8080/api/v1/budgets/acme-corp/usage \
  -H "Authorization: Bearer $TOKEN"
# → {"tokens_used": 7420000, "cost_used_usd": 381.20, "cost_pct": 76.24}
```

Alert fires at 80% of the limit (configurable). At 100%, ingest is blocked with:
```json
HTTP 429 {"error": "monthly budget exceeded"}
```

---

## API Reference

### Tracing
| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/traces` | List traces (filter: framework, model, agent, status, time range) |
| `GET` | `/api/v1/traces/{id}` | Full trace with span tree |
| `GET` | `/api/v1/traces/{id}/cost` | Cost breakdown per span |
| `GET` | `/api/v1/traces/{id}/graph` | Topology graph |

### Agents
| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/agents` | List all agents |
| `GET` | `/api/v1/agents/{id}/runs` | Agent run history |
| `GET` | `/api/v1/agents/{id}/metrics` | Error rate, cost, latency |
| `GET` | `/api/v1/agents/{id}/topology` | Agent interaction graph |

### Analytics
| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/analytics/overview` | Dashboard summary stats |
| `GET` | `/api/v1/analytics/cost` | Cost by framework/model |
| `GET` | `/api/v1/analytics/errors` | Error report |

### Live Stream
| Protocol | Path | Description |
|---|---|---|
| `WebSocket` | `/api/v1/stream/live` | Real-time span events |

### Budget & FinOps
| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/budgets/{tenant}` | Get budget config |
| `PUT` | `/api/v1/budgets/{tenant}` | Set budget (admin) |
| `GET` | `/api/v1/budgets/{tenant}/usage` | Current month usage + % |
| `GET` | `/api/v1/budgets/{tenant}/alerts` | Alert history |

### Governance
| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/audit` | Paginated audit log |
| `GET` | `/api/v1/audit/verify` | Cryptographic chain verification |

---

## Kubernetes Deployment

See [HELM_INSTALL.md](HELM_INSTALL.md) for the complete step-by-step guide.

**Quick install:**

```bash
# Add the repo
helm repo add agentfabric https://charts.agentfabric.io
helm repo update

# Install with default values (single namespace, no TLS)
helm install agentfabric agentfabric/agentfabric \
  --namespace agentfabric \
  --create-namespace \
  --set global.environment=production \
  --set postgresql.auth.password=changeme \
  --set api.ingress.hosts[0].host=api.your-domain.com \
  --set portal.ingress.hosts[0].host=app.your-domain.com

# Check rollout
kubectl rollout status deployment/agentfabric-api -n agentfabric
```

---

## Production Checklist

Before going live:

- [ ] Change `AF_JWT_SECRET` (min 32 chars, randomly generated)
- [ ] Change `AF_ADMIN_PASSWORD` from default `admin`
- [ ] Set `AF_AUTH_DISABLED=false`
- [ ] Enable TLS: `AF_TLS_ENABLED=true` with valid certs
- [ ] Set `AF_CORS_ORIGINS` to your actual domain
- [ ] Configure `AF_BUDGET_ENABLED=true` with per-tenant limits
- [ ] Review PII scrubbing patterns for your data types
- [ ] Set up Prometheus alerting on `agentfabric_processed_spans_total` and budget alerts

---

## Monitoring

AgentFabric exposes Prometheus metrics on every service:

| Service | Metrics endpoint | Key metrics |
|---|---|---|
| Collector | `:8888/metrics` | `agentfabric_processed_spans_total`, `agentfabric_pii_scrubbed_total`, queue depth |
| API Gateway | `:8080/metrics` | Request rate, latency, budget checks |
| af-core | `:8889/metrics` | `afcore_spans_consumed_total`, `afcore_ch_errors_total`, policy decisions |

Grafana dashboards are included in the Helm chart (`monitoring.dashboards.enabled=true`).

---

## Repository Structure

```
af-core/           Rust policy engine (Kafka consumer → ClickHouse)
agent-sdk/         Python auto-instrumentation SDK
api-gateway/       Go REST + WebSocket API (Chi router)
collector/         Go OTLP collector (gRPC :4317, HTTP :4318)
deploy/
  docker/          Minimal production Docker Compose
  helm/            Helm chart (Chart.yaml, values.yaml, templates/)
  k8s/             Raw Kubernetes manifests
  migrations/      Versioned SQL migrations (golang-migrate)
  sql/             PostgreSQL + ClickHouse init schemas
docs/              Architecture docs, runbooks
portal/            React 18 + TypeScript dashboard (Vite)
integrationTests/  End-to-end tests for all 5 frameworks
monitoring/        Prometheus config
docker-compose.yml Full dev stack (9 services)
```

---

## v1.2.0 — What's New

- **Full pipeline wired** — Kafka → af-core → ClickHouse now flows end-to-end. af-core (Rust) built from scratch: Kafka consumer, policy evaluation, ClickHouse HTTP API writer, `/health` + `/metrics` server.
- **Budget hard limits** — `tenant_budgets` table, `BudgetEnforcer` in api-gateway, React budget panel in Cost page, HTTP 429 on exceeded limits.
- **Zero-code auto-instrumentation** — `sitecustomize.py` + `install_hooks.py` for drop-in Python instrumentation.
- **WebSocket reconnect** — exponential back-off with jitter in the portal live stream.

See [CHANGELOG.md](CHANGELOG.md) for full history.

---

## License

Proprietary. All rights reserved. Contact [team@agentfabric.io](mailto:team@agentfabric.io) for licensing.
