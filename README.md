# AgentFabric

AgentFabric is a multi-service platform for AI telemetry, traffic control, cost accounting, and operational governance.

This repository contains the control-plane pieces around AI agents and LLM traffic:
- telemetry ingestion
- trace and run analytics
- LLM proxying
- virtual API key management
- budget enforcement
- configurable pricing
- operator-facing administration

## Repository Layout

### `agent-sdk/`
Python SDK and auto-instrumentation.

Current responsibilities:
- framework patching
- span creation
- automatic bootstrap through `sitecustomize.py`

### `collector/`
Go OTLP receiver and enrichment service.

Current responsibilities:
- OTLP gRPC and HTTP ingest
- framework detection
- PII scrubbing
- export to gateway

### `api-gateway/`
Primary control plane.

Current responsibilities:
- auth and session handling
- tenant-aware APIs
- traces, runs, analytics, and cost endpoints
- budgets and usage reporting
- pricing rule management
- virtual key registration
- reverse proxy and transparent net proxy

### `portal/`
React operator UI.

Current pages:
- Dashboard
- Traces
- Trace Detail
- Live Stream
- Agents
- Cost
- Environments
- Users
- Audit Log
- API Keys
- Pricing
- Policies

### `deploy/`
Deployment assets:
- SQL migrations
- Docker Compose
- Helm chart
- Kubernetes manifests

### `docs/`
Review documents, production checklist, and historical reports.

### `memory/`
Current-state project notes.

## System Shape

```text
Apps / agents
  -> SDK instrumentation
  -> OTLP spans

Collector
  -> normalize
  -> enrich
  -> scrub
  -> forward

Gateway
  -> persist
  -> price
  -> enforce budgets
  -> proxy model traffic
  -> expose APIs

Portal
  -> query APIs
  -> operate the system
```

The gateway is the operational center of the platform. Today it owns the most important control-plane concerns: auth, budgets, pricing, key mediation, APIs, and proxy behavior.

## Current Capabilities

Implemented in code today:
- OTLP ingest over gRPC and HTTP
- automatic Python instrumentation
- trace, run, agent, and live-stream views
- token and cost reporting
- configurable pricing rules stored in the database
- pricing management through admin APIs and the portal
- traffic policy rules and inline enforcement
- secret and PII-aware DLP actions in proxy and netproxy
- control-plane audit logging for pricing, keys, and policy changes
- budget limits and usage reporting
- virtual API key management
- LLM reverse proxy
- transparent network proxy
- cookie-based browser auth with `HttpOnly` cookies
- RBAC and ABAC-aware admin workflows

## Pricing

Pricing is configurable and no longer lives only in hardcoded tables.

Current pricing precedence:
1. database-backed pricing rules
2. bootstrap pricing from `AF_MODEL_PRICING_FILE` or `AF_MODEL_PRICING_JSON`
3. built-in defaults

Gateway pricing endpoints:
- `GET /api/v1/pricing`
- `PUT /api/v1/pricing`
- `DELETE /api/v1/pricing/{ruleId}`

The portal exposes an admin-only Pricing page backed by the same API.

## Authentication

Browser auth is cookie-based.

Current behavior:
- password login sets an `HttpOnly` session cookie
- OIDC also lands on the same session path
- the portal uses `/auth/me` with `credentials: 'include'`
- the browser does not read or store the raw JWT in `localStorage`

The gateway also supports:
- JWT key rotation through `AF_JWT_SECRETS`
- strict config validation through `AF_ENV=production` or `AF_STRICT_CONFIG=true`

## Provider Scope

The current managed-key registration scope in the gateway is:
- `openai`
- `anthropic`

The SDK supports broader instrumentation than the current gateway key-registration scope. Treat those as separate product surfaces.

## Local Development

Typical local startup:

Windows PowerShell:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\bootstrap_local.ps1
```

macOS / Linux:

```bash
bash scripts/setup-local.sh
```

The bootstrap flow:
- starts the Docker stack
- waits for gateway and collector health
- seeds demo pricing and policy rules

Common local endpoints:
- gateway: `http://localhost:8080`
- collector OTLP HTTP: `http://localhost:4318`
- portal: `http://localhost:3000`
- hosted API docs: `http://localhost:8080/docs/swagger`

## Production Assets

The repository includes:
- Docker Compose for local and production-like setups
- Kubernetes manifests
- Helm chart
- SQL migrations under `deploy/migrations/`
- release validation scripts under `scripts/`

See:
- `docs/QUICKSTART.md`
- `docs/REFERENCE_DEPLOYMENT.md`
- `docs/RELEASE_BOUNDARIES.md`
- `docs/PRODUCTION_CHECKLIST.md`

## Current Maturity

Current status should be described as:
`pre-GA / beta`

What is solid:
- the core control-plane shape
- portal/admin surface
- pricing configurability
- proxy and budget foundations
- SDK auto-instrumentation

What still needs work before GA:
- stronger end-to-end smoke validation
- deeper automated test coverage, especially store and collector paths
- more mature pricing governance
- stronger policy and security controls
- cleaner CI and release confidence

## Recommended Next Work

Highest-value next modules:
- provider and model allowlists
- per-request caps and policy checks
- pricing governance: priority, effective dates, preview, audit
- proxy/netproxy policy enforcement
- DLP-style prompt and response controls
- release-grade smoke automation

## Related Docs

- `docs/openapi.yaml`
- `docs/TECHNICAL_REVIEW_v1.1.0.md`
- `docs/DEEP_DIVE_TEST_REPORT_v1.1.0.md`
- `docs/PRODUCTION_CHECKLIST.md`
- `memory/MEMORY.md`

## License

Proprietary. All rights reserved.
