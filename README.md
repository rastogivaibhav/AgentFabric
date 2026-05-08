# GOVAGN

> **Governed release control for production AI agents.**

[![GitHub stars](https://img.shields.io/github/stars/govagn/govagn?style=social)](https://github.com/govagn/govagn)
[![Platform License](https://img.shields.io/badge/platform-proprietary-black)](./LICENSE)
[![SDK License](https://img.shields.io/badge/python%20sdk-MIT-green)](./agent-sdk/pyproject.toml)
[![Python SDK](https://img.shields.io/pypi/v/govagn)](https://pypi.org/project/govagn/)
[![Go](https://img.shields.io/badge/Go-1.22-00ADD8?logo=go)](./api-gateway)
[![React](https://img.shields.io/badge/Portal-React%2018-61DAFB?logo=react)](./portal)
[![OpenAPI](https://img.shields.io/badge/API-OpenAPI-6BA539)](./docs/openapi.yaml)
[![Deployment](https://img.shields.io/badge/deployment-self--hosted-2ea44f)](./docker-compose.yml)

**GOVAGN is a self-hosted release control plane for regulated LLM apps and agent systems.**  
Use it to approve, enforce, monitor, and prove AI changes before they reach production: **prompt releases, eval gates, policy packs, rollout controls, traces, costs, and audit evidence** in one place.

**Best for:** platform teams, AI infra teams, security teams, and enterprises running AI in production.

> License note: the **platform is proprietary** today; the **Python SDK is MIT**.

## What Is GOVAGN?

GOVAGN gives teams one governed release path for AI workloads:

- **approve** prompt, model, and policy changes with eval evidence
- **enforce** runtime policy, DLP, budget, and provider controls
- **roll out** AI changes with canaries and rollback criteria
- **prove** what happened with trace-linked audit evidence

It is strongest when AI has already moved beyond demos and into environments where people need answers to questions like:

- What happened?
- Which prompt or model version caused it?
- Why did it cost this much?
- Should this request or agent action have been allowed?
- What evidence do we have for the release, audit, or incident review?

# ✨ Why This Exists

Shipping an AI feature is easy.  
Operating AI across teams, environments, and providers is where things break down.

Typical failure mode:

- apps call OpenAI and Anthropic directly with no shared controls
- prompts change without release discipline
- spend is visible only after the invoice
- policy enforcement is inconsistent
- agent runs are hard to explain during incidents

GOVAGN exists to fix that.

It gives you a **single operational layer** for AI workloads:

- **observe** every run and trace
- **govern** what traffic is allowed
- **control** prompts, budgets, and rollout decisions

If your team has ever asked _“what happened, who changed it, why did it cost this much, and should this have been allowed?”_  
that’s exactly the problem GOVAGN is built to solve.

# ⚡ What You Can Do With It

- **Trace LLM and agent execution end-to-end** across spans, retries, tools, failures, and handoffs
- **Proxy model traffic** across providers with centralized policy, pricing, and audit hooks
- **Enforce guardrails at runtime** with traffic and DLP-style policies
- **Ship GOVAGN-native policy packs** for regulatory, industry, org, and agent-capability controls
- **Attribute cost precisely** by request, span, provider, model, and tenant
- **Version and promote prompts** across environments with release-aware trace linkage
- **Run GOVAGN-native eval packs** against traces and seeded datasets before promoting changes
- **Export audit evidence** for operators, security teams, and architecture reviews
- **Manage long-running agent sessions and tasks** as first-class runtime objects through the API and client hooks
- **Self-host the entire platform** for regulated, internal, or network-controlled environments

> Today, managed-agent execution is strongest in the gateway and API surface. Dedicated portal workflows for managed sessions, tasks, and approvals are still being expanded.

# 🧱 Architecture (High-Level)

GOVAGN is built as a **control plane**, not a framework.

```text
Apps / Agents / Services
        |
        |  SDK instrumentation or proxied LLM traffic
        v
   +-------------+
   |  Collector  |  OTLP ingest + enrichment + scrubbing
   +-------------+
        |
        v
+-------------------+
|   API Gateway     |
|-------------------|
| policy            |
| pricing           |
| budgets           |
| prompts/releases  |
| eval packs        |
| datasets          |
| eval executions   |
| audit             |
| proxy mediation   |
| managed sessions  |
+-------------------+
   |            |
   v            v
PostgreSQL    Redis
   |
   v
+-------------------+
|      Portal       |
| traces, costs,    |
| policy, prompts,  |
| runs, audit, ops  |
+-------------------+
```

**Core services**

- [`api-gateway/`](./api-gateway) — central control plane
- [`collector/`](./collector) — OTLP ingestion and enrichment
- [`portal/`](./portal) — operator UI
- [`agent-sdk/`](./agent-sdk) — Python instrumentation SDK
- [`deploy/`](./deploy) — Docker, Helm, migrations
- [`docs/`](./docs) — architecture, install, API, operations

For the full system view, see [docs/ARCHITECTURE.md](./docs/ARCHITECTURE.md).

# 🚀 Quick Start

Get the full stack running locally in a few minutes.

## 1. Start GOVAGN

### Windows PowerShell

```powershell
git clone https://github.com/govagn/govagn.git
cd govagn
powershell -ExecutionPolicy Bypass -File .\scripts\bootstrap_local.ps1
```

### macOS / Linux

```bash
git clone https://github.com/govagn/govagn.git
cd govagn
bash scripts/setup-local.sh
```

These scripts:

- create local env config if needed
- start PostgreSQL, Redis, gateway, collector, portal, and observability tooling
- run migrations
- seed demo pricing, policy packs, and eval packs
- wait for readiness

## 2. Open the app

- **Portal:** [http://localhost:3000](http://localhost:3000)
- **Gateway:** [http://localhost:8080](http://localhost:8080)
- **Swagger:** [http://localhost:8080/docs/swagger](http://localhost:8080/docs/swagger)
- **Collector OTLP HTTP:** [http://localhost:4318](http://localhost:4318)
- **Jaeger:** [http://localhost:16686](http://localhost:16686)
- **Prometheus:** [http://localhost:9090](http://localhost:9090)
- **Grafana:** [http://localhost:9091](http://localhost:9091)

## 3. Verify it’s healthy

```bash
curl http://localhost:8080/readyz
curl http://localhost:4318/readyz
```

## 4. Send one traced AI request

Install the SDK from this repo:

```bash
pip install -e ./agent-sdk anthropic
```

Run this:

```python
from govagn import instrument
from anthropic import Anthropic

instrument(
    endpoint="http://localhost:4317",
    service_name="quickstart-agent",
    environment="local",
)

client = Anthropic(api_key="YOUR_API_KEY")

resp = client.messages.create(
    model="claude-3-7-sonnet",
    max_tokens=128,
    messages=[{"role": "user", "content": "Write a one-line deployment summary."}],
)

print(resp.content)
```

## 5. Confirm success

In the portal, you should now see:

- a new trace
- model + token usage
- cost attribution
- provider metadata
- execution details in trace view

If you can see that, GOVAGN is working.

## 6. Run a native eval pack

GOVAGN now ships seeded eval packs and datasets. You can execute them directly from the gateway:

```bash
curl -X POST http://localhost:8080/api/v1/evals/execute \
  -H "Content-Type: application/json" \
  -d '{
    "pack_id": "evalpack.retail.commerce.v1",
    "dataset_refs": ["retail.refund_decisions.v1"],
    "mode": "offline"
  }'
```

This returns:

- an `eval_execution`
- a summarized `eval_run`
- per-item evaluator results and evidence links through the execution detail API

Optional local entrypoint:

```bash
make dev
```

For a more guided setup, see [docs/QUICKSTART.md](./docs/QUICKSTART.md).

# 🧪 Example Usage

## Instrument an existing Python agent app

If you already have agent code, you usually only need one line at startup:

```python
from govagn import instrument

instrument(
    endpoint="http://localhost:4317",
    service_name="support-agent",
    service_version="2026.04.0",
    environment="staging",
)
```

After that, your existing framework code runs as usual, and GOVAGN captures runtime telemetry.

## Add custom spans around important workflow steps

```python
from govagn import agent_span, trace_tool_call

@trace_tool_call("fetch_change_request")
def fetch_change_request(change_id: str):
    return {"id": change_id, "risk": "high", "owner": "platform"}

with agent_span(
    name="triage.change_request",
    framework="custom",
    agent_name="release-triage",
    agent_role="reviewer",
):
    result = fetch_change_request("CR-1842")
    print(result)
```

Use this when you want visibility into:

- custom tool calls
- orchestration steps
- internal business logic
- approval or decision boundaries

## Route model traffic through the gateway

```bash
curl -X POST http://localhost:8080/proxy/anthropic/v1/messages \
  -H "Authorization: Bearer af-vk-your-virtual-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-3-7-sonnet",
    "max_tokens": 300,
    "messages": [
      {"role": "user", "content": "Draft a release summary for the last 24 hours of incidents."}
    ]
  }'
```

Before you can use the governed proxy, register a virtual key with the local gateway:

```bash
curl -X POST http://localhost:8080/api/v1/keys \
  -H "Content-Type: application/json" \
  -d '{
    "provider": "anthropic",
    "real_key": "YOUR_REAL_PROVIDER_KEY",
    "display_name": "local-anthropic"
  }'
```

Then use the returned `virtual_key` in the proxy call:

```bash
curl -X POST http://localhost:8080/proxy/anthropic/v1/messages \
  -H "Authorization: Bearer af-vk-your-virtual-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-3-7-sonnet",
    "max_tokens": 300,
    "messages": [
      {"role": "user", "content": "Draft a release summary for the last 24 hours of incidents."}
    ]
  }'
```

This path is useful when you want GOVAGN to act as the **governed gateway** for model traffic, not just the telemetry sink.

## Execute a native eval pack

Use this when you want dataset-backed evaluation instead of just scoring one trace:

```bash
curl -X POST http://localhost:8080/api/v1/evals/execute \
  -H "Content-Type: application/json" \
  -d '{
    "pack_id": "evalpack.gdpr.compliance.v1",
    "dataset_refs": ["regulatory.gdpr.requests.v1"],
    "mode": "offline"
  }'
```

Then inspect executions:

```bash
curl http://localhost:8080/api/v1/evals/executions
curl http://localhost:8080/api/v1/evals/executions/1
```

# 🧩 Core Concepts

## **Trace**

A single execution story.  
One request, run, or workflow seen end-to-end.

## **Span**

A unit of work inside a trace.  
LLM call, tool invocation, policy check, retry, handoff, custom step.

## **Policy**

A runtime rule that can allow, warn, redact, or deny behavior.

## **Policy Pack**

A GOVAGN-native library bundle of policies, controls, defaults, and composition rules.
Examples include GDPR, HIPAA, EU AI Act, retail, banking, org risk, and agent tool-access packs.

## **Eval Pack**

A GOVAGN-native bundle of evaluators, thresholds, datasets, and release/runtime gates.
Eval packs answer whether a system is good enough to trust, not just whether it was allowed.

## **Prompt Release**

A versioned prompt promoted across environments with traceable rollout history.

## **Run**

A summarized execution record used for analytics and operator views.

## **Managed Session / Task**

First-class runtime objects for long-running agent workloads.  
Useful when execution is not just “one model call,” but a session with events, artifacts, approvals, and interruption states.

## **Evidence Bundle**

An exportable audit package for governance review, release validation, or incident analysis.

The mental model is:

**Observe the runtime. Govern the runtime. Ship changes with evidence.**

# 🔌 Integrations / Extensibility

## Frameworks and SDKs

### Direct SDK auto-instrumentation today

- CrewAI
- LangGraph
- OpenAI
- Anthropic
- Google ADK

### Supported adjacent workflows

- LangChain projects that run through LangGraph
- custom Python workflows via OpenTelemetry-compatible spans

GOVAGN does not currently ship a separate first-class LangChain patcher in this repo. LangChain users are best served today through LangGraph flows or explicit instrumentation.

## Providers

- OpenAI
- Anthropic
- Google
- Vertex AI
- AWS Bedrock

## Platform integrations

- OpenTelemetry / OTLP
- Docker Compose
- Helm / Kubernetes
- PostgreSQL
- Redis
- Prometheus
- Grafana
- Jaeger
- OIDC / enterprise SSO

## Extension points

- add custom span attributes through the SDK
- extend provider mediation in the gateway
- define tenant-specific pricing rules
- create policy, eval, and rollout workflows
- integrate managed runtime adapters

# 📊 Real Use Cases

## Central AI gateway for multiple product teams

One team uses OpenAI, another uses Anthropic, and nobody has consistent controls.  
GOVAGN gives the platform team one place to enforce policy, track cost, and inspect runtime behavior.

## Prompt release discipline

A prompt change causes a spike in failures or cost.  
With GOVAGN, you can trace affected runs back to the exact prompt version and release tag.

## Security review for agent systems

An internal agent touches sensitive workflows.  
GOVAGN gives security teams policy enforcement, audit trails, and evidence exports instead of asking them to trust ad hoc logs.

## Enterprise AI FinOps

Leadership wants to know which applications, agents, or teams are driving spend.  
GOVAGN turns model usage into attributable operational data, not just a provider invoice.

## Long-running agent operations

You need visibility into sessions, tasks, artifacts, and approvals, not just one-shot requests.  
GOVAGN adds first-class runtime objects for that operating model in the gateway and API today, with broader portal UX still in progress.

## Native governance libraries

You want first-party starter content instead of building policy and eval logic from scratch.  
GOVAGN now includes native policy packs and eval packs with seeded datasets for regulatory, industry, organization, and agent-runtime enforcement use cases.

# ⚙️ Configuration

You can run GOVAGN locally with the provided scripts and defaults. For a real environment, these are the first settings that matter.

## Required in production

```env
DATABASE_URL=postgres://fabric:CHANGE_ME@localhost:5432/govagn?sslmode=require
REDIS_URL=redis://:CHANGE_ME@localhost:6379/2
GV_JWT_SECRET=CHANGE_ME_minimum_32_random_bytes
GV_VAULT_KEY=CHANGE_ME_64_hex_chars
GV_ADMIN_PASSWORD=CHANGE_ME
```

## Recommended for production

```env
GV_STRICT_CONFIG=true
GV_AUTH_DISABLED=false
GV_BUDGET_ENABLED=true
GV_TLS_ENABLED=true
GV_CORS_ORIGINS=https://app.govagn.io
```

## Optional enterprise SSO

```env
GV_OIDC_ISSUER=
GV_OIDC_CLIENT_ID=
GV_OIDC_CLIENT_SECRET=
GV_OIDC_REDIRECT_URI=https://api.govagn.io/auth/callback
```

## SDK-side instrumentation

```env
GV_ENDPOINT=http://localhost:4317
GV_PROMPT_ID=support-agent
GV_PROMPT_VERSION=12
GV_PROMPT_RELEASE_TAG=prod-2026-04-09
GV_PROMPT_ENVIRONMENT=production
```

For the full reference, see [`.env.example`](./.env.example).

# 🛡️ Security / Governance

GOVAGN is built for teams that need more than logs after the fact.

## Runtime controls

- policy enforcement on AI traffic
- native policy packs for regulatory, industry, org, and agent controls
- dataset-backed eval packs for release and runtime confidence
- DLP-style guardrails
- prompt and release governance
- provider mediation through virtual keys
- budget-aware operational controls

## Access controls

- password login for local and simple environments
- OIDC + PKCE for enterprise SSO
- bearer JWT support for authenticated clients
- tenant-aware role-based access paths

## Data handling

- self-hosted deployment model
- OTLP ingestion with optional auth
- PII scrubbing and redaction support in the collector
- audit trails for runtime and admin actions

## Operational evidence

- control history
- audit log verification
- release validation scripts
- evidence bundles for review and export

This is especially relevant for:

- internal copilots
- workflow automation agents
- regulated enterprise deployments
- multi-team shared AI platforms

# 🗺️ Roadmap

## Next up

- richer operator UI for managed sessions, tasks, and artifacts
- approval inbox and interruption workflows
- deeper Anthropic managed-runtime ingestion
- stronger policy hooks for long-running agent execution

## After that

- broader managed-runtime adapters
- tighter rollout controls across prompts, models, and environments
- better multi-tenant governance ergonomics
- stronger production deployment guidance and HA patterns

## Direction

GOVAGN is opinionated about where this category goes:

**AI systems need a control plane.**  
Not another prompt playground. Not another agent demo framework.  
A real operational layer for production AI.

# 🤝 Contributing

We want contributions from platform engineers, AI infra teams, security engineers, and SDK builders.

## Ways to contribute

- improve instrumentation coverage
- add provider integrations
- tighten policy and audit workflows
- improve docs, examples, and deployment paths
- report bugs and operator pain points
- contribute portal UX improvements for real production workflows

## Local development

```bash
make dev
```

Useful docs:

- [Quickstart](./docs/QUICKSTART.md)
- [Architecture](./docs/ARCHITECTURE.md)
- [API Guide](./docs/API_GUIDE.md)
- [Production Checklist](./docs/PRODUCTION_CHECKLIST.md)

If you’re opening a PR, keep it practical:

- explain the user or operator problem
- include config or migration notes if relevant
- update docs when behavior changes
- prefer small, reviewable changes over broad rewrites

# 📜 License

GOVAGN currently uses a split license model:

- **Platform** (`api-gateway`, `collector`, `portal`, deployment assets): **Proprietary / All rights reserved**
- **Python SDK** (`agent-sdk/`): **MIT**

See [LICENSE](./LICENSE) and [agent-sdk/pyproject.toml](./agent-sdk/pyproject.toml) for the exact terms.
