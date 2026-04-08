# Govagn Quickstart

## Purpose

This guide is the fastest way to get Govagn running locally so you can validate the product shape end to end.

Use it when you want to:

- stand up the local stack quickly
- sign in to the portal
- send instrumented or proxied traffic
- see traces, cost, policy, and audit behavior without a full production deployment

For production-oriented installation, use:

- [SETUP_AND_ONBOARDING.md](SETUP_AND_ONBOARDING.md)
- [INSTALL_SINGLE_TENANT.md](INSTALL_SINGLE_TENANT.md)
- [INSTALL_MULTI_TENANT.md](INSTALL_MULTI_TENANT.md)

## What You Get Locally

The local stack gives you a self-hosted Govagn control plane with:

- gateway APIs
- collector ingest
- portal UI
- live pricing and policy rules
- demo governance and pricing seed data

## Fastest Local Start

### Windows PowerShell

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\bootstrap_local.ps1
```

### macOS / Linux

```bash
bash scripts/setup-local.sh
```

Both commands:

- create `.env.local` if needed
- start the local Docker stack
- wait for gateway and collector health and readiness
- seed demo pricing and policy rules

## Local URLs

- Portal: `http://localhost:3000`
- Gateway: `http://localhost:8080`
- Collector OTLP HTTP: `http://localhost:4318`
- Swagger UI: `http://localhost:8080/docs/swagger`
- Prometheus: `http://localhost:9090`
- Grafana: `http://localhost:9091`
- Jaeger: `http://localhost:16686`
- Gateway readiness: `http://localhost:8080/readyz`
- Collector readiness: `http://localhost:4318/readyz`

## First 10-Minute Validation Flow

Once the stack is up:

1. open the portal
2. sign in with the local admin account
3. verify traces, policies, cost, and prompts pages load
4. register a provider key if you want to test proxied traffic
5. send one instrumented or proxied request
6. confirm a trace appears
7. inspect cost and policy behavior in trace detail

## Demo Seed Content

The local bootstrap seeds:

- tenant pricing overrides for demo models
- traffic policy examples
- DLP and guardrail examples
- prompt and governance-friendly baseline data

Seed file:

- [../deploy/sql/demo_seed.sql](../deploy/sql/demo_seed.sql)

Manual reseed:

### Windows PowerShell

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\seed-demo-data.ps1
```

### macOS / Linux

```bash
bash scripts/seed-demo-data.sh
```

## Local Validation Scripts

If you want stronger proof than just "the UI loaded," run:

- [../scripts/run_release_candidate_validation.ps1](../scripts/run_release_candidate_validation.ps1)
- [../scripts/probe_stack_health.ps1](../scripts/probe_stack_health.ps1)
- [../scripts/probe_proxy_path.ps1](../scripts/probe_proxy_path.ps1)

Shell equivalents are available in the same [../scripts](../scripts) directory.

### Windows PowerShell

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\run_release_candidate_validation.ps1
```

With admin validation:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\run_release_candidate_validation.ps1 `
  -AdminUser admin `
  -AdminPassword <password>
```

### macOS / Linux

```bash
bash scripts/run-release-candidate-validation.sh
```

## What Gets Covered

You will see centralized AI traffic only for workloads you onboard through one of these paths:

- Python SDK auto-instrumentation
- OTLP spans sent to the collector
- LLM calls sent through proxy or netproxy

The local stack validates AI runtime observability and governance. It is not intended to be full generic infrastructure monitoring.

## Stop The Stack

```bash
docker compose -f docker-compose.yml down
```

## Where To Go Next

After Quickstart, use:

- [ARCHITECTURE.md](ARCHITECTURE.md)
- [API_GUIDE.md](API_GUIDE.md)
- [SETUP_AND_ONBOARDING.md](SETUP_AND_ONBOARDING.md)
- [PRODUCTION_CHECKLIST.md](PRODUCTION_CHECKLIST.md)
