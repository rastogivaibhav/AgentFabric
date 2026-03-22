# Setup and Onboarding

## Audience

This document is for:

- platform engineers standing up AgentFabric
- administrators configuring the control plane
- application teams onboarding their first workload

## Choose Your Starting Path

Use this guide based on your goal:

- local evaluation: [QUICKSTART.md](/C:/Users/vrast/Documents/Agentic%20Code/files/docs/QUICKSTART.md)
- single-team or single-program production deployment: [INSTALL_SINGLE_TENANT.md](/C:/Users/vrast/Documents/Agentic%20Code/files/docs/INSTALL_SINGLE_TENANT.md)
- shared platform deployment: [INSTALL_MULTI_TENANT.md](/C:/Users/vrast/Documents/Agentic%20Code/files/docs/INSTALL_MULTI_TENANT.md)

## Prerequisites

### Local evaluation

Required:
- Docker Desktop or Docker Engine
- Docker Compose
- PowerShell on Windows, or Bash and curl on macOS or Linux

### Production-oriented setup

Required:
- PostgreSQL
- Redis
- gateway, collector, and portal deployment target
- TLS termination plan
- secret-management plan
- admin authentication plan

## Local Setup

### Windows PowerShell

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\bootstrap_local.ps1
```

### macOS or Linux

```bash
bash scripts/setup-local.sh
```

These commands:
- create `.env.local` if needed
- start the local stack
- wait for health and readiness
- seed demo pricing and policy rules

Local URLs:
- Portal: `http://localhost:3000`
- Gateway: `http://localhost:8080`
- Collector: `http://localhost:4318`
- Swagger UI: `http://localhost:8080/docs/swagger`

## Production Setup Checklist

Before onboarding teams, confirm:

- PostgreSQL and Redis are reachable
- secrets are configured
- gateway and collector readiness probes return `200`
- admin auth is configured
- CORS is restricted to the portal origin
- pricing and policy defaults are loaded

Use:
- [PRODUCTION_CHECKLIST.md](/C:/Users/vrast/Documents/Agentic%20Code/files/docs/PRODUCTION_CHECKLIST.md)
- [REFERENCE_DEPLOYMENT.md](/C:/Users/vrast/Documents/Agentic%20Code/files/docs/REFERENCE_DEPLOYMENT.md)

## First Administrator Tasks

After the stack is up:

1. Log in through the portal or `/auth/login`.
2. Confirm `/auth/me` returns the expected identity.
3. Review pricing defaults.
4. Review policy defaults.
5. Register one provider key.
6. Create or confirm a tenant budget.
7. Verify one trace appears in the portal.

## Onboarding a Workload

AgentFabric supports three primary onboarding paths.

### Path 1: Python SDK

Best for:
- Python agent or application workloads
- teams that want auto-instrumentation

High-level flow:
1. install and initialize the SDK
2. point telemetry at the collector
3. optionally attach prompt metadata
4. send traffic and verify traces in the portal

### Path 2: OTLP Export

Best for:
- services already using OpenTelemetry
- polyglot environments

High-level flow:
1. configure OTLP export to the collector
2. send spans
3. confirm spans arrive in the gateway and portal

### Path 3: Proxy or Netproxy

Best for:
- teams that want governed model access
- workloads needing pricing, policy, and budget controls inline

High-level flow:
1. register a provider key
2. route LLM traffic through `/proxy/{provider}/...` or the netproxy path
3. verify cost, policy, and trace records appear in the portal

## Recommended Onboarding Sequence for a Team

1. Confirm tenant and user access.
2. Choose one provider and one model path.
3. Register the provider key.
4. Apply a basic budget.
5. Apply baseline pricing rules.
6. Apply baseline policies or guardrails.
7. Onboard one application through SDK, OTLP, or proxy.
8. Run one success scenario and one controlled policy scenario.
9. Review trace, cost, and audit evidence in the portal.

## Prompt Lifecycle Onboarding

If the team wants prompt-governed operations:

1. create a prompt record
2. create a version and release tag
3. promote it to the target environment
4. attach prompt identifiers in application instrumentation
5. verify prompt metadata appears in trace detail

## Evaluation Onboarding

If the team wants release or governance scoring:

1. generate candidate traces
2. run eval scoring through the eval APIs or portal
3. compare candidate and baseline release tags
4. review policy-effectiveness and regression results

## Validation After Onboarding

At minimum, validate:

- one successful trace
- one proxied request with cost
- one policy decision visible in trace detail
- one audit event visible in the portal
- one prompt-linked trace, if prompt lifecycle is enabled

Use these scripts:

- [run_release_candidate_validation.ps1](/C:/Users/vrast/Documents/Agentic%20Code/files/scripts/run_release_candidate_validation.ps1)
- [probe_stack_health.ps1](/C:/Users/vrast/Documents/Agentic%20Code/files/scripts/probe_stack_health.ps1)
- [probe_proxy_path.ps1](/C:/Users/vrast/Documents/Agentic%20Code/files/scripts/probe_proxy_path.ps1)

## Coverage Reality

Be explicit with onboarding teams:

- SDK-instrumented workloads are covered
- OTLP-emitting services are covered
- proxied LLM traffic is covered
- unmanaged hosts are not automatically covered

## Troubleshooting Guideposts

Start with:

- gateway `/healthz`
- gateway `/readyz`
- collector `/healthz`
- collector `/readyz`
- portal login flow
- provider key registration
- one proxy request
- one trace lookup in the portal

If production onboarding is blocked, continue with:
- [BACKUP_RESTORE.md](/C:/Users/vrast/Documents/Agentic%20Code/files/docs/BACKUP_RESTORE.md)
- [HA_GUIDE.md](/C:/Users/vrast/Documents/Agentic%20Code/files/docs/HA_GUIDE.md)
- [SSO_RBAC_PLAN.md](/C:/Users/vrast/Documents/Agentic%20Code/files/docs/SSO_RBAC_PLAN.md)
