# AgentFabric Quickstart

AgentFabric ships as a self-hosted AI runtime control plane with:
- gateway APIs
- collector ingest
- portal UI
- live pricing and policy rules

## Fastest local start

Windows PowerShell:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\bootstrap_local.ps1
```

macOS / Linux:

```bash
bash scripts/setup-local.sh
```

Both commands:
- create `.env.local` if needed
- start the local Docker stack
- wait for gateway and collector health
- seed demo pricing and policy rules

## Local URLs

- Portal: `http://localhost:3000`
- Gateway: `http://localhost:8080`
- Collector OTLP HTTP: `http://localhost:4318`
- Swagger UI: `http://localhost:8080/docs/swagger`
- Prometheus: `http://localhost:9090`
- Grafana: `http://localhost:9091`
- Jaeger: `http://localhost:16686`

## Demo seed content

The local bootstrap seeds:
- tenant pricing overrides for OpenAI demo models
- traffic policy examples
- DLP rules for secret redaction and PII warnings

Seed file:
- [demo_seed.sql](/C:/Users/vrast/Documents/Agentic%20Code/files/deploy/sql/demo_seed.sql)

Manual reseed:

Windows PowerShell:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\seed-demo-data.ps1
```

macOS / Linux:

```bash
bash scripts/seed-demo-data.sh
```

## Stop the stack

```bash
docker compose -f docker-compose.yml down
```

## Release candidate validation

Windows PowerShell:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\run_release_candidate_validation.ps1
```

With admin validation:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\run_release_candidate_validation.ps1 `
  -AdminUser admin `
  -AdminPassword <password>
```

macOS / Linux:

```bash
bash scripts/run-release-candidate-validation.sh
```
