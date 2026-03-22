# AgentFabric — Project Memory

## What This Is
**AgentFabric** — enterprise AI agent observability & governance platform.
Observes CrewAI, LangGraph, OpenAI Agents, Google ADK, and Claude Agents via OpenTelemetry.

## Tech Stack
- **Collector** (Go 1.22): OTLP receiver (gRPC :4317, HTTP :4318), framework detection, PII scrub, exports to api-gateway
- **af-core** (Rust): Kafka consumer, policy engine, audit log, writes to PostgreSQL + ClickHouse
- **api-gateway** (Go): Chi router REST API + WebSocket, reads PostgreSQL + Redis
- **portal** (React 18 + TypeScript, Vite): Dashboard with Dashboard/LiveStream/Traces/TraceDetail/Agents/Cost/Environments/Users/Audit/API Keys/Pricing pages
- **agent-sdk** (Python): Auto-instrumentation for all 5 frameworks

## Repo Structure (at workspace root)
```
af-core/           Rust processing core
agent-sdk/         Python SDK
api-gateway/       Go REST + WebSocket API
collector/         Go OTLP collector
  cmd/collector/   Entry point (used by Dockerfile)
  internal/        auth, config, exporter, processor, receiver
portal/            React app
  src/
    App.tsx        Router (30 lines)
    components/    Layout.tsx
    hooks/         api.ts (React Query + WebSocket)
    pages/         Dashboard, LiveStream, Traces, TraceDetail, Agents, Cost, Environments, Users, Audit, API Keys, Pricing
deploy/
  docker/          Minimal production compose (uses ../../ relative paths)
  helm/            Helm chart
  k8s/             Kubernetes manifests
  sql/             init.sql + clickhouse_init.sql
docs/              Two report generators (generate-report.js, generate_report.js)
monitoring/        prometheus.yml (scrapes collector + api-gateway)
scripts/           setup-local.sh
docker-compose.yml Full dev stack (all 12 services including Kafka, ClickHouse, Prometheus)
```

## Key Architecture Notes
- Collector uses `cmd/collector/main.go` + `internal/config` (Viper). Dockerfile builds `./cmd/collector`.
- portal/src/App.tsx is the authoritative app entry (App.jsx was deleted — it was an old monolithic version).
- deploy/docker/docker-compose.yml runs from repo root via `docker compose -f deploy/docker/docker-compose.yml up -d`
- scripts/setup-local.sh does `cd deploy/docker` then runs docker-compose from there

## Refactoring Done (2026-03-10)
1. Moved `AgentFabric_Production_Code/agentfabric-src/*` to repo root — eliminated unnecessary nesting
2. Deleted all root-level loose duplicate files (App.jsx, main.rs, main.go, pii.go, etc.)
3. Deleted `portal/src/App.jsx` (monolithic old version; App.tsx + separate pages is the new architecture)
4. Deleted old collector root files (main.go, config.go, pii.go, policy.go, detector.go) — dead code relative to Dockerfile
5. Removed redundant `deploy/docker-compose.yml` (was duplicate of root docker-compose.yml)
6. Fixed root `docker-compose.yml`: api→api-gateway, ./sql/→./deploy/sql/, collector env vars aligned to new config
7. Fixed `deploy/docker/docker-compose.yml`: all build contexts now use `../../` relative paths
8. Created `monitoring/prometheus.yml` (was referenced but missing)
9. Removed empty `portal/src/store/` and `portal/src/utils/` directories

## Layer 1a — sitecustomize.py Auto-Instrumentation (COMPLETED 2026-03-19)
**Status**: ✅ COMPLETE — 21/21 tests passing, 175/175 integration tests passing

**Files Created**:
- `agent-sdk/agentfabric/auto_instrument.py` — AutoInstrumentor class, reads env vars, initializes tracer
- `agent-sdk/agentfabric/sitecustomize.py` — Boot script auto-called by Python at startup
- `agent-sdk/install_hooks.py` — SitecustomizeInstaller class, handles install/merge/uninstall
- `agent-sdk/tests/test_auto_instrument.py` — 21 comprehensive unit tests
- `agent-sdk/setup.py` — Setuptools post-install hook integration

**Files Modified**:
- `agent-sdk/pyproject.toml` — Changed build backend to `setuptools.build_meta`

**Environment Variables Supported**:
- `AF_ENDPOINT` — OTLP HTTP endpoint (default: http://localhost:4318)
- `AF_TENANT_ID` — Tenant identifier (default: "default")
- `AF_AUTO_INSTRUMENT` — Set to "0" to disable (default: enabled)
- `AF_SERVICE_NAME` — OTel service.name (default: auto-detected from sys.argv[0])
- `AF_API_KEY` — Optional API key for authentication
- `AF_INSECURE` — Set to "1" for insecure gRPC (default: true for localhost)

**How it works**:
1. `pip install agentfabric` runs setup.py post-install hook
2. Hook copies sitecustomize.py into site-packages (merges with existing content)
3. Next `python any_script.py` imports sitecustomize.py automatically
4. AutoInstrumentor reads env vars, initializes tracer, patches all 5 frameworks
5. User code runs fully instrumented, zero changes needed

**Definition of Done**: ✅ All met
- [x] `pip install agentfabric` installs sitecustomize.py into site-packages
- [x] `python test.py` (with openai imported, zero agentfabric lines) → span ready to be recorded
- [x] `AF_AUTO_INSTRUMENT=0 python test.py` → no instrumentation (opt-out works)
- [x] Existing sitecustomize.py is preserved (merge, not overwrite)
- [x] All test_auto_instrument.py tests pass (21/21)

## Production Readiness: pre-GA, improving
**Done now in code**: Multi-service architecture, OTLP ingestion, framework detection, PII scrubbing, JWT auth, WebSocket live stream, React portal with dashboard/traces/live/agents/cost/environments/users/audit/keys/pricing pages, Docker Compose, Kubernetes manifests, Helm chart, Layer 1a auto-instrumentation, budget enforcement, Layer 2 virtual key proxy, Layer 3 network proxy, DB-backed pricing rules, and strict production config validation.

**Still missing before GA**: full release smoke confidence, broader automated test coverage across store/collector paths, pricing governance depth (effective dates/priority/audit/preview), stronger policy/security controls, mTLS maturity in collector deployments, and tighter operational validation in CI/staging.

## Current Product Shape
AgentFabric is now best described as an enterprise AI observability and control platform:
- observe agent and LLM activity
- control traffic through proxy/netproxy and virtual keys
- track tokens, budgets, and configurable pricing
- operate the platform through the portal

## Recommended Next Build
Focus next on policy and governance:
- provider/model allowlists and request caps
- pricing governance enhancements
- proxy policy enforcement and DLP-style controls
- release-grade smoke automation
