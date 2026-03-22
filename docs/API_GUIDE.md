# API Guide

## Purpose

This guide explains the API surface of AgentFabric at a product and integration level.

It is designed to help:

- platform teams integrating the gateway
- application teams sending proxied or instrumented traffic
- administrators managing governance and prompts
- architects reviewing capability boundaries

## API Model

AgentFabric exposes a control-plane API rather than a single narrow service API.

The API surface is organized around these domains:

- authentication and session management
- traces and observability
- live runtime events
- pricing and cost governance
- budgets and usage controls
- policy, guardrails, and simulation
- prompts and releases
- evals and regressions
- administrative and audit workflows
- provider key and runtime mediation

## Canonical API Definition

The OpenAPI specification should be treated as the canonical low-level contract.

Use:

- `docs/openapi.yaml` for the current route-level contract
- this guide for product-level workflow understanding

## Authentication

Most administrative and control-plane APIs require authenticated access.

Common patterns:

- admin login for portal and admin workflows
- bearer token or session-backed access for authenticated routes
- controlled provider access for proxy traffic

Recommended principle:

- separate administrative identities from runtime application identities

## Core API Domains

### 1. Traces and Observability

These APIs support runtime inspection and debugging.

Typical capabilities:

- list traces
- filter traces
- fetch trace detail
- inspect spans and lineage
- inspect trace-level cost, policy, and prompt context
- compare traces

Typical use cases:

- debugging a failed runtime path
- reviewing policy and pricing effects on a request
- comparing two release behaviors

### 2. Live Runtime Events

These APIs support live operational visibility.

Typical capabilities:

- subscribe to live runtime events
- inspect near-real-time traffic and operator signals

Typical use cases:

- monitoring active traffic during a rollout
- observing incidents or policy spikes in real time

### 3. Pricing and Cost Governance

These APIs support pricing rules, cost explanation, and tenant-aware cost control.

Typical capabilities:

- list pricing rules
- preview pricing behavior
- inspect cost breakdowns
- analyze rule matches and cost provenance

Typical use cases:

- validating provider cost attribution
- creating tenant-specific pricing behavior
- explaining why a request cost what it did

### 4. Budgets and Usage Controls

These APIs support budget configuration and usage visibility.

Typical capabilities:

- configure budget limits
- inspect usage
- review enforcement or alert conditions

Typical use cases:

- per-tenant budget governance
- release and platform cost oversight

### 5. Policy and Guardrails

These APIs support runtime governance and policy explainability.

Typical capabilities:

- create and update policies
- preview decisions
- inspect explanation output
- simulate policy behavior
- inspect guardrail actions

Typical use cases:

- validating a new policy before rollout
- explaining why allow, deny, redact, or warn happened
- comparing policy behavior across environments

### 6. Prompt Lifecycle

These APIs support prompt versioning and release linkage.

Typical capabilities:

- list prompts
- create or update prompt versions
- promote releases across environments
- link prompt identifiers to runtime traces

Typical use cases:

- tracing a production issue back to a prompt release
- managing prompt changes with environment-aware control

### 7. Evaluations and Regressions

These APIs support release-readiness and runtime quality workflows.

Typical capabilities:

- score traces
- create eval runs
- compare release tags
- inspect regression outcomes
- review policy effectiveness metrics

Typical use cases:

- determining whether a release is better or worse than baseline
- validating policy and cost effects before rollout

### 8. Provider Keys and Runtime Mediation

These APIs support governed access to upstream model providers.

Typical capabilities:

- register provider access
- inspect provider metadata
- route traffic through governed provider paths

Typical use cases:

- centralizing provider access
- avoiding unmanaged direct model usage
- enforcing policy, pricing, and audit through one runtime path

### 9. Audit and Administrative Workflows

These APIs support governance visibility and platform operations.

Typical capabilities:

- inspect audit records
- review administrative actions
- validate release and readiness workflows

Typical use cases:

- compliance review
- internal governance evidence
- platform operations review

## Common Integration Flows

### Flow A: Application sends proxied runtime traffic

1. application authenticates to the gateway
2. request is routed through provider mediation
3. pricing and policy logic run
4. trace and cost data are persisted
5. portal surfaces runtime outcome

### Flow B: Team promotes a prompt release

1. create or update a prompt version
2. assign release metadata
3. promote to target environment
4. runtime traces carry prompt release context
5. compare behavior across releases if needed

### Flow C: Administrator validates policy

1. create or update a policy
2. preview the policy decision
3. simulate policy against sample requests
4. review explanation output
5. promote to active use

### Flow D: Operator evaluates release readiness

1. score relevant traces
2. compare candidate versus baseline release tags
3. review regressions and policy effectiveness
4. run validation scripts and GA gate
5. make go or no-go decision

## API Design Principles

The AgentFabric API is designed around these principles:

- control-plane first, not just data retrieval
- explainability over opaque outcomes
- enterprise governance as a first-class concern
- product workflows reflected in API structure
- separation between runtime mediation and administrative control

## Integration Recommendations

### For application teams

- prefer governed runtime paths over unmanaged direct provider calls
- include clear service and environment metadata
- propagate prompt metadata where available

### For platform teams

- define a standard tenant, service, and release-tag convention
- use pricing, policy, and prompt APIs together
- validate new environments with proof scripts, not only manual checks

### For security and governance teams

- treat audit, policy simulation, and budget APIs as a single governance surface
- use preview and simulation before enabling stricter policies globally

## Error Handling Expectations

Clients should expect normal API failure patterns such as:

- authentication or authorization failures
- validation failures
- policy denials or redactions
- missing configuration errors
- budget or governance enforcement responses
- upstream provider failures surfaced through controlled runtime paths

Consumers should treat responses as explainable control-plane outcomes, not just pass or fail transport events.

## Versioning Guidance

Use the versioned API path structure exposed by the gateway.

For stable integrations:

- pin to documented route versions
- validate against the OpenAPI contract
- treat release-boundary docs as the supportability boundary

## What This API Is Not

The AgentFabric API is not primarily designed as:

- an end-user chat API
- a generic playground API
- a research notebook interface
- a standalone model abstraction library

It is designed as an enterprise operational API for governed AI runtime systems.

## Related Documents

Use this guide with:

- [ARCHITECTURE.md](ARCHITECTURE.md)
- [SETUP_AND_ONBOARDING.md](SETUP_AND_ONBOARDING.md)
- [PRODUCTION_CHECKLIST.md](PRODUCTION_CHECKLIST.md)
- [RELEASE_BOUNDARIES.md](RELEASE_BOUNDARIES.md)
- [openapi.yaml](openapi.yaml)
