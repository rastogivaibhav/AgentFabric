# API Guide

## Canonical API References

The authoritative machine-readable API description lives in:

- [openapi.yaml](/C:/Users/vrast/Documents/Agentic%20Code/files/docs/openapi.yaml)

The gateway also serves:

- `GET /docs/openapi.yaml`
- `GET /docs/swagger`

This guide is the operator-facing overview of the current API surface.

## Authentication Model

The gateway supports multiple auth patterns.

### Browser and portal usage

- login through `/auth/login`
- session established through an `HttpOnly` cookie
- portal uses browser credentials for authenticated API access

### API clients

- can use `Authorization: Bearer <jwt>` on authenticated routes

### Proxy traffic

- uses AgentFabric virtual keys
- does not use the normal control-plane JWT path

### Collector ingest

- uses the internal ingest path and collector auth configuration

## Major API Areas

### Health and readiness

- `GET /healthz`
- `GET /readyz`

### Authentication

- `POST /auth/login`
- `GET /auth/login`
- `GET /auth/callback`
- `GET /auth/logout`
- `GET /auth/me`
- `POST /auth/refresh`

### Traces and live operations

- `GET /api/v1/traces`
- `GET /api/v1/traces/{id}`
- `GET /api/v1/traces/{id}/timeline`
- `GET /api/v1/traces/{id}/graph`
- `GET /api/v1/traces/compare`
- `GET /api/v1/traces/saved-views`
- `GET /api/v1/stream/live`

### Agents, runs, and analytics

- `GET /api/v1/agents`
- `GET /api/v1/runs`
- analytics endpoints under `/api/v1/analytics/...`

### Environments and users

- `GET /api/v1/environments`
- user APIs under `/api/v1/users`

### Audit

- `/api/v1/audit`
- `/api/v1/audit/control`
- `/api/v1/audit/verify`

### Budgets and pricing

- `/api/v1/budgets/{tenant_id}`
- `/api/v1/pricing`
- `/api/v1/pricing/preview`

### Policies and guardrails

- `/api/v1/policies`
- `/api/v1/policies/preview`

### Keys

- `/api/v1/keys`

### Evals and regressions

- `/api/v1/evals`
- `POST /api/v1/evals/score`
- `POST /api/v1/evals/regressions`

### Prompts and releases

- prompt endpoints under `/api/v1/prompts`

### Proxy

- `/proxy/{provider}/...`
- `GET /api/v1/netproxy/ca.crt`

### Internal ingest

- `POST /internal/ingest`

## Typical API Workflows

### Operator workflow

1. log in
2. inspect traces
3. review live stream
4. review policy or audit evidence

### Governance workflow

1. create or update a policy
2. run policy preview or simulation
3. route traffic through the proxy
4. review resulting trace and audit evidence

### Cost workflow

1. review pricing rules
2. run a pricing preview
3. inspect trace-level cost breakdown
4. review budget behavior

### Prompt lifecycle workflow

1. create prompt metadata
2. create a new version or release
3. promote it to the target environment
4. verify prompt linkage in the trace

## Provider Scope

The current provider registry in code includes:

- `openai`
- `anthropic`
- `google`
- `vertexai`
- `bedrock`

Release and support claims should remain aligned with:
- [RELEASE_BOUNDARIES.md](/C:/Users/vrast/Documents/Agentic%20Code/files/docs/RELEASE_BOUNDARIES.md)

## API Usage Guidance

Recommended operator guidance:

- use the OpenAPI spec for generated clients or endpoint details
- use this document for workflow-level understanding
- use the portal for investigation before building custom control-plane clients

## Validation

Before depending on an API flow in production, validate with:

- [run_release_candidate_validation.ps1](/C:/Users/vrast/Documents/Agentic%20Code/files/scripts/run_release_candidate_validation.ps1)
- [probe_stack_health.ps1](/C:/Users/vrast/Documents/Agentic%20Code/files/scripts/probe_stack_health.ps1)
- [probe_proxy_path.ps1](/C:/Users/vrast/Documents/Agentic%20Code/files/scripts/probe_proxy_path.ps1)
