# AgentFabric

> The compliance-ready observability platform for enterprise AI agents — full-stack tracing, cost governance, and tamper-evident audit for CrewAI, LangGraph, OpenAI Agents, Google ADK, and Claude Agents in production.

![Version](https://img.shields.io/badge/version-v1.1.0-blue)
![Build Status](https://img.shields.io/badge/build-passing-brightgreen)
![License](https://img.shields.io/badge/license-proprietary-blue)
![Go](https://img.shields.io/badge/go-1.22-blue)
![Rust](https://img.shields.io/badge/rust-1.78-orange)
![Python](https://img.shields.io/badge/python-3.10+-green)
![Tests](https://img.shields.io/badge/tests-121%20passing-brightgreen)

---

## Why AgentFabric?

Multi-agent systems are notoriously difficult to observe. You get spans from OTLP, but:

- **No framework semantics** — is that span from CrewAI, LangGraph, or a raw API call?
- **No cost attribution** — which agent used $500 of your LLM budget?
- **No compliance trail** — auditors ask: "who accessed this agent's memory?"
- **No policy enforcement** — PII leaks from agent outputs go undetected.

AgentFabric solves this end-to-end. It is a production-grade observability platform purpose-built for AI agents.

---

## What's New in v1.1.0

- **Constant-time authentication** — timing-safe credential comparison prevents username enumeration
- **Zero-downtime JWT key rotation** — `AF_JWT_SECRETS` comma-separated multi-key with signing/verification split
- **TLS fail-secure** — server refuses to start as plain HTTP when `AF_TLS_ENABLED=true` with missing certs
- **Schema migration tooling** — golang-migrate/v4 wired into startup; `make migrate/up`, `make migrate/down`, `make migrate/status`
- **Portal RBAC/ABAC UI** — `RequireRole` component, admin-only Create/Delete in Users page, role badge in nav, production credential hint suppression
- **Kubernetes hardening** — resource limits, liveness/readiness probes, PodDisruptionBudgets on all three deployments, HPA for gateway and af-core
- **Helm alignment** — probe paths, image pull policy, and security context improvements
- **Production checklist** — `docs/PRODUCTION_CHECKLIST.md` with runbook, SLOs, secret rotation procedures, rollback decision tree

See [CHANGELOG.md](CHANGELOG.md) for the full diff.

---

## Features

### Core Capabilities

- **Protocol-Native Tracing** — OTLP gRPC (`:4317`) + HTTP (`:4318`) receivers. Instrument any framework without custom SDKs.
- **5 Frameworks Out of the Box** — CrewAI, LangGraph, OpenAI Agents, Google ADK, Anthropic Claude Agents.
- **LLM Cost Attribution** — Token counts + model pricing. Real-time cost per agent, per trace, per model.
- **Policy Engine** — 5 built-in policies: Sovereignty (tool blocklist), Cost Thresholds, Tool Allowlists, PII Output Detection, Rate Limiting.
- **Live Dashboard** — React portal with trace timeline, span waterfall, agent topology, cost breakdown.
- **Tamper-Evident Audit Log** — SHA-256 hash-chained entries with cryptographic verification endpoint. Regulatory-grade compliance trail.
- **Multi-Tenancy** — Row-level security (RLS) on PostgreSQL. Tenant isolation at the database layer.
- **WebSocket Live Stream** — Real-time span events with pause/resume/filter.
- **User Management** — Full CRUD with RBAC (admin/editor/viewer) + ABAC self-service (users can edit their own profile).

### Enterprise Ready

- **SOC 2 Architecture** — Immutable audit trail, RBAC/ABAC, secrets manager guidance.
- **HIPAA-Compatible** — PII redaction built-in (regex + contextual rules in collector).
- **Kubernetes Deployment** — DaemonSet collector, Deployment api-gateway/af-core/portal, PodDisruptionBudgets, HPA, Helm chart.
- **OIDC/SSO** — PKCE + nonce flow for Okta, Azure AD, Auth0. Password login for self-hosted.
- **Self-Hosted & Cloud** — Docker Compose, Kubernetes, or managed cloud service.

---

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│               Python / Node Agent Code                       │
│       (CrewAI / LangGraph / OpenAI / ADK / Claude)           │
└────────────────────┬─────────────────────────────────────────┘
                     │ OTLP gRPC :4317  or  HTTP :4318
                     ▼
        ┌────────────────────────┐
        │     Collector (Go)     │
        │  • OTLP receiver       │
        │  • Framework detection │
        │  • PII scrubbing       │
        │  • Cost computation    │
        │  • JWT auth to gateway │
        └────────┬───────────────┘
                 │  POST /internal/ingest
        ┌────────▼───────────────────────────────────┐
        │           API Gateway (Go / Chi)            │
        │  • JWT auth (OIDC + password login)         │
        │  • RBAC/ABAC middleware                     │
        │  • Per-tenant rate limiting (Redis)         │
        │  • WebSocket hub (live stream)              │
        │  • Prometheus metrics                       │
        └────────┬───────────────────────────────────┘
        ┌────────▼────────┐    ┌──────────────────────┐
        │   PostgreSQL    │    │      af-core (Rust)   │
        │  • Spans/Runs   │    │  • Kafka consumer     │
        │  • Audit log    │    │  • Policy engine      │
        │  • Users/Tenants│    │  • ClickHouse writes  │
        │  • RLS enforced │    │  • SHA-256 audit chain│
        └─────────────────┘    └──────────────────────┘
                 ▲                     ▲
                 │                     │
        ┌────────┴─────────┐  ┌───────┴───────┐
        │  Portal (React)  │  │     Redis      │
        │  • Dashboard     │  │  Rate limiting │
        │  • Traces/Agents │  │  WS pub/sub    │
        │  • Cost analysis │  │  Session cache │
        │  • Live stream   │  └───────────────┘
        │  • User admin    │
        └──────────────────┘
```

**Data pipeline** (high-throughput path):
```
Agent SDK → Collector → API Gateway → PostgreSQL (hot)
                                    → Kafka → af-core → ClickHouse (analytics)
```

---

## Quick Start (5 Minutes)

### Prerequisites

| Tool | Minimum Version |
|------|----------------|
| Docker + Docker Compose | 24+ |
| Go | 1.22+ |
| Node | 20+ |
| Python | 3.10+ |

### Option A: Full Dev Stack (Recommended for First Run)

```bash
git clone https://github.com/rastogivaibhav/AgentFabric.git
cd AgentFabric

# Copy environment template and review defaults
cp .env.example .env

# Start all services (Postgres, Redis, Kafka, ClickHouse, Collector, API Gateway)
docker compose up -d

# Start the portal dev server (hot-reload)
cd portal
npm install --legacy-peer-deps
npm run dev
```

Visit **http://localhost:3000** — portal is live.
Default credentials: `admin` / `admin` *(change via `AF_ADMIN_PASSWORD` before any production use)*

### Option B: Minimal Stack (Dev / CI)

```bash
docker compose -f deploy/docker/docker-compose.yml up -d
```

Runs PostgreSQL, Redis, Collector, and API Gateway only (no Kafka, ClickHouse, or Grafana).

### Send Your First Span

```bash
curl -X POST http://localhost:4318/v1/traces \
  -H 'Content-Type: application/json' \
  -d '{
    "resourceSpans": [{
      "resource": {
        "attributes": [{"key":"service.name","value":{"stringValue":"demo-agent"}}]
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

Check **http://localhost:3000/traces** — your span appears within seconds.

---

## Python SDK: Automatic Instrumentation

### Install

```bash
pip install agentfabric
```

### One-Line Instrumentation (CrewAI)

```python
from agentfabric import instrument
from crewai import Agent, Task, Crew

# Instrument once at app startup — all agents traced automatically
instrument(
    endpoint="http://localhost:4318",
    service_name="my-research-platform",
    headers={"X-AF-Tenant": "acme-corp"}  # optional: tenant isolation
)

# Define agents normally — no changes needed
researcher = Agent(
    role="Market Research Analyst",
    goal="Gather comprehensive market intelligence",
    backstory="Expert researcher with 10 years experience",
    tools=[web_search],
    model="gpt-4",
)

# Execute — tracing is automatic
crew = Crew(agents=[researcher], tasks=[research_task])
result = crew.kickoff()
```

### Supported Frameworks

| Framework | Version | Status | Auto-traced |
|-----------|---------|--------|-------------|
| **CrewAI** | 0.30+ | GA | Agent roles, tasks, tools, memory |
| **LangGraph** | 0.1+ | GA | State graphs, cycles, memory |
| **OpenAI Agents** | 1.0+ | GA | Chat completions, tool use |
| **Anthropic Claude** | 0.7+ | GA | Claude models, tool_use |
| **Google ADK** | 0.1+ | GA | Generative agents, looping |

### Advanced: Custom Spans

```python
from agentfabric import trace_tool_call, agent_span

@trace_tool_call("custom_validator")
def validate_research(data: dict):
    return data  # Creates a span automatically

with agent_span("data_processing", {"stage": "preprocessing"}):
    processed = preprocess(raw_data)
```

---

## Configuration

### Environment Variables

#### API Gateway (`api-gateway/cmd/server/main.go`)

| Variable | Default | Description |
|----------|---------|-------------|
| `AF_JWT_SECRET` | `dev-secret-change-in-production` | **Required in prod** — primary JWT signing key |
| `AF_JWT_SECRETS` | *(uses AF_JWT_SECRET)* | Comma-separated key rotation list. First = signing, all = verify |
| `AF_ADMIN_USER` | `admin` | Admin username for password login |
| `AF_ADMIN_PASSWORD` | `admin` | **Change before deployment** |
| `AF_AUTH_DISABLED` | `false` | Disable auth entirely (CI/e2e only) |
| `AF_TLS_ENABLED` | `false` | Enable HTTPS. Fail-secure: server refuses to start without cert/key |
| `AF_TLS_CERT_FILE` | *(empty)* | Path to TLS certificate |
| `AF_TLS_KEY_FILE` | *(empty)* | Path to TLS private key |
| `DATABASE_URL` | `postgres://fabric:fabric@localhost:5432/agentfabric?sslmode=disable` | PostgreSQL DSN |
| `REDIS_URL` | `redis://localhost:6379` | Redis URL |
| `LISTEN_ADDR` | `:8080` | HTTP/HTTPS listen address |
| `AF_RATE_LIMIT_RPM` | `1000` | Per-tenant requests-per-minute cap |
| `AF_MIGRATE_ON_STARTUP` | `true` | Run `migrate/up` at startup. Set `false` for read replicas |
| `AF_MIGRATIONS_PATH` | `deploy/migrations` | Path to SQL migration files |
| `AF_OIDC_ISSUER` | *(empty)* | OIDC provider issuer URL (Okta/Azure/Auth0). Leave empty for password-only |
| `AF_OIDC_CLIENT_ID` | *(empty)* | OIDC application client ID |
| `AF_OIDC_CLIENT_SECRET` | *(empty)* | OIDC application client secret |
| `AF_OIDC_REDIRECT_URI` | `http://localhost:8080/auth/callback` | OIDC callback URL |

#### Collector (`collector/internal/config/config.go`)

| Variable | Default | Description |
|----------|---------|-------------|
| `AF_GRPC_ADDR` | `:4317` | OTLP gRPC listener |
| `AF_HTTP_ADDR` | `:4318` | OTLP HTTP listener |
| `AF_GATEWAY_ENDPOINT` | `http://localhost:8080` | API gateway ingest URL |
| `AF_AUTH_REQUIRE_AUTH` | `true` | Require JWT bearer on OTLP ingest |
| `AF_JWT_SECRET` | *(required when auth=true)* | Must match gateway's signing secret |
| `AF_PII_ENABLED` | `true` | Enable PII scrubbing |
| `AF_PII_REDACT` | `true` | Redact (true) vs flag-only (false) |
| `AF_RATE_LIMIT_SPANS_PER_SECOND` | `50000` | Ingest rate cap |
| `AF_PROCESSOR_WORKERS` | `4` | Parallel span processing goroutines |
| `AF_PROCESSOR_BATCH_SIZE` | `512` | Spans per flush batch |

#### Portal (`.env.local` / build args)

| Variable | Description |
|----------|-------------|
| `VITE_API_URL` | API gateway base URL (default: `http://localhost:8080`) |
| `VITE_WS_URL` | WebSocket base URL (default: `ws://localhost:8080`) |
| `VITE_SSO_ENABLED` | Show SSO button on login page (`true`/`false`) |
| `VITE_SHOW_DEFAULT_CREDS` | Show `admin/admin` hint on login page. **Never `true` in production** |
| `VITE_AUTH_DISABLED` | Skip login entirely for CI/e2e (`true`/`false`) |

---

## API Reference

### Authentication

```bash
# Password login
TOKEN=$(curl -sf -X POST http://localhost:8080/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin"}' | jq -r .token)

# Use token
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/environments
```

### REST Endpoints

**Auth:**
- `POST /auth/login` — Password login → `{"token":"<JWT>"}`
- `GET  /auth/login` — OIDC SSO redirect (when `AF_OIDC_ISSUER` set)
- `GET  /auth/callback` — OIDC callback
- `GET  /auth/logout` — Clear session
- `GET  /auth/me` — Current user identity
- `POST /auth/refresh` — Refresh JWT (returns new token with extended expiry)

**Traces:**
- `GET /api/v1/traces` — List traces (`?since=24h&framework=crewai&limit=50`)
- `GET /api/v1/traces/{traceId}` — Full trace with all spans
- `GET /api/v1/traces/{traceId}/graph` — Span DAG topology
- `GET /api/v1/traces/{traceId}/timeline` — Waterfall timeline data
- `GET /api/v1/traces/{traceId}/cost` — Per-span cost breakdown

**Agents:**
- `GET /api/v1/agents` — List agents with run count, cost, error rate
- `GET /api/v1/agents/{agentId}` — Agent detail
- `GET /api/v1/agents/{agentId}/runs` — Run history
- `GET /api/v1/agents/{agentId}/metrics` — 24h metrics snapshot
- `GET /api/v1/agents/{agentId}/topology` — Call graph

**Runs:**
- `GET  /api/v1/runs` — List runs (`?agent=x&framework=langgraph`)
- `GET  /api/v1/runs/{runId}` — Run detail with metadata
- `GET  /api/v1/runs/{runId}/children` — Child runs (nested execution)
- `POST /api/v1/runs/{runId}/feedback` — Submit human feedback `{"score":5,"comment":"..."}`

**Analytics:**
- `GET /api/v1/analytics/overview?since=24h` — Dashboard KPIs
- `GET /api/v1/analytics/frameworks` — Trace count by framework
- `GET /api/v1/analytics/cost?since=24h` — Cost breakdown by model
- `GET /api/v1/analytics/errors?since=24h` — Error frequency by framework

**Users (RBAC-gated):**
- `GET    /api/v1/users` — List users (any authenticated user)
- `POST   /api/v1/users` — Create user (admin only)
- `GET    /api/v1/users/{userId}` — Get user (any authenticated user)
- `PUT    /api/v1/users/{userId}` — Update user (admin OR self)
- `DELETE /api/v1/users/{userId}` — Delete user (admin only)

**Other:**
- `GET /api/v1/environments` — List environments
- `GET /api/v1/audit` — Paginated audit log (`?limit=100&offset=0`)
- `GET /api/v1/audit/verify` — Verify SHA-256 hash chain integrity
- `WS  /api/v1/stream/live` — Real-time span events
- `GET /healthz` — Health check
- `GET /metrics` — Prometheus metrics

### Query Parameters

```
?since=1h|6h|24h|7d|30d    Time window for aggregations
?framework=crewai           Filter by framework
?agent=my-agent             Filter by agent name
?model=gpt-4                Filter by LLM model
?status=ok|error            Filter by trace status
?limit=50                   Page size (1–200, default 50)
?offset=0                   Pagination offset
```

---

## Database Migrations

AgentFabric uses [golang-migrate/v4](https://github.com/golang-migrate/migrate) for versioned SQL migrations.

```bash
# Check current migration version
make migrate/status DATABASE_URL=postgres://...

# Apply all pending migrations
make migrate/up DATABASE_URL=postgres://...

# Roll back one step
make migrate/down DATABASE_URL=postgres://...
```

Migrations run automatically at API gateway startup (`AF_MIGRATE_ON_STARTUP=true` by default).
Set `AF_MIGRATE_ON_STARTUP=false` for read-only replicas or external migration pipelines.

Migration files live in `deploy/migrations/`:
```
deploy/migrations/
  001_initial_schema.up.sql    # Full schema: 10 tables, RLS, audit rules, seed data
  001_initial_schema.down.sql  # Reverse in FK-safe dependency order
```

---

## Policy Engine

Five built-in policies evaluated per span by `af-core`:

| Policy | Trigger | Action |
|--------|---------|--------|
| **Sovereignty** | Tool name in forbidden list | DENY execution |
| **Cost Threshold** | Cumulative cost > limit | DENY / alert |
| **Tool Allowlist** | Tool not in allowed set | DENY execution |
| **PII Output** | SSN / credit card / email / phone pattern in output | REDACT |
| **Rate Limit** | Requests > per-tenant cap | DENY with 429 |

All policy decisions are written to the immutable `policy_audit_log` with SHA-256 hash chaining.

---

## Deployment

### Docker Compose (Development)

```bash
# Full stack with Kafka + ClickHouse + Grafana
docker compose up -d

# Minimal stack
docker compose -f deploy/docker/docker-compose.yml up -d
```

### Kubernetes (Production)

```bash
# Apply manifests directly
kubectl apply -f deploy/k8s/agentfabric.yaml

# Or use Helm
helm install agentfabric deploy/helm/ \
  --namespace agentfabric \
  --create-namespace \
  --values deploy/helm/values.yaml
```

See [docs/PRODUCTION_CHECKLIST.md](docs/PRODUCTION_CHECKLIST.md) for the full pre-deployment gate — 20 required checks covering security, infrastructure, database, and observability.

### Docker Compose (Production-like)

```bash
export AF_JWT_SECRET=$(openssl rand -base64 32)
export AF_ADMIN_PASSWORD=$(openssl rand -base64 24)
export AF_CORS_ORIGINS="https://app.yourdomain.com"
export DATABASE_URL="postgres://fabric:<strong_pass>@postgres:5432/agentfabric?sslmode=require"
export REDIS_URL="redis://:<strong_pass>@redis:6379"
export POSTGRES_PASSWORD="<strong_pass>"
export VITE_API_URL="https://api.yourdomain.com"
export VITE_WS_URL="wss://api.yourdomain.com"

make prod-up
```

---

## Service Level Objectives (v1.1.0)

| SLO | Target |
|-----|--------|
| API Gateway availability | 99.9% (43.8 min downtime/month) |
| API P99 latency | < 300 ms |
| API P50 latency | < 50 ms |
| Span ingest throughput | > 10,000 spans/s per collector node |
| Error rate (5xx) | < 0.1% of requests |
| PII redaction coverage | 100% |

PagerDuty alert mapping: `ServiceDown` → P1, `HighErrorRate` / `HighP95Latency` → P2, `UnexpectedCostSpike` → P3.

---

## Security

### Credential Requirements

| Secret | Minimum | Generate |
|--------|---------|---------|
| `AF_JWT_SECRET` | 32 chars | `openssl rand -base64 32` |
| `AF_ADMIN_PASSWORD` | 20 chars | `openssl rand -base64 24` |
| `POSTGRES_PASSWORD` | 24 chars | `openssl rand -base64 24` |
| `REDIS_URL` password | 20 chars | `openssl rand -base64 20` |

### JWT Key Rotation (Zero-Downtime)

```bash
# 1. Generate new key
NEW_SECRET=$(openssl rand -base64 32)

# 2. Add as first key (old key stays for verification of in-flight sessions)
kubectl patch secret agentfabric-secrets -n agentfabric \
  --type='json' \
  -p='[{"op":"replace","path":"/data/jwt-secrets","value":"'"$(echo -n "${NEW_SECRET},${OLD_SECRET}" | base64 -w0)"'"}]'

# 3. Rolling restart gateway
kubectl rollout restart deployment/agentfabric-gateway -n agentfabric

# 4. After AF_SESSION_MAX_AGE (default 8h), remove old key
# See docs/PRODUCTION_CHECKLIST.md §2.2 for the full 6-step procedure
```

---

## Development

### Running Tests

```bash
# Go server tests
cd api-gateway && go test ./... -count=1

# Portal tests (113 Vitest tests)
cd portal && npm test

# Go lint
make lint

# Full build
make build
```

### Make Targets

```bash
make build          # Build Go binaries + portal bundle
make test           # Run all tests
make lint           # golangci-lint + tsc --noEmit + ESLint
make migrate/up     # Apply pending migrations
make migrate/down   # Roll back one migration
make migrate/status # Show current migration version
make certs          # Generate dev self-signed TLS certs
make prod-up        # Start production Docker Compose stack
make prod-down      # Stop production stack
```

---

## Troubleshooting

### Traces Not Appearing

```bash
# 1. Check collector is reachable
curl -v http://localhost:4318/v1/traces

# 2. Check API gateway logs
docker compose logs api-gateway --tail 50

# 3. Verify database migration ran
make migrate/status DATABASE_URL="postgres://fabric:fabric@localhost:5432/agentfabric?sslmode=disable"
```

### Auth Failures (401 Storm)

```bash
# Check AF_JWT_SECRET matches between collector and gateway
docker compose exec api-gateway printenv AF_JWT_SECRET
docker compose exec collector printenv AF_JWT_SECRET
```

### High Portal Latency

```bash
# Check DB connections
docker exec agentfabric-postgres psql -U fabric -d agentfabric \
  -c "SELECT count(*), state FROM pg_stat_activity GROUP BY state"

# Check Redis
redis-cli -u $REDIS_URL INFO stats | grep instantaneous_ops
```

---

## Known Limitations (v1.1.0)

| Item | Risk | Target |
|------|------|--------|
| Password login authenticates only the env-var admin, not DB users | High | v1.1.1 |
| Tenant ID type mismatch between migration (UUID) and query layer (TEXT) | High | v1.1.1 |
| JWT stored in localStorage (XSS-accessible) | High | v1.1.1 |
| `af_audit_writer` DB role password must be changed manually after first migration | High | v1.2.0 |
| mTLS between collector pods not enabled in docker-compose | Medium | v1.2.0 |
| Grafana alert notification channels not configured | Medium | v1.2.0 |
| No automated certificate rotation (cert-manager not configured) | Medium | v1.2.0 |
| Edit-user modal links to unimplemented route | Low | v1.2.0 |
| Per-tenant audit retention policy not enforced | Low | v1.3.0 |

---

## Roadmap

### v1.1.1 (Patch — in progress)
- [ ] Password login queries `users` table with bcrypt verification
- [ ] Resolve tenant ID type (UUID vs TEXT)
- [ ] HttpOnly cookie for JWT — remove localStorage storage
- [ ] `MaxBytesReader` on ingest endpoint
- [ ] Health endpoint checks Postgres + Redis

### v1.2.0
- [ ] Keyset (cursor-based) pagination for traces and runs
- [ ] HTTP security headers middleware (CSP, X-Frame-Options, etc.)
- [ ] Admin action audit trail (user create/delete/role-change to policy_audit_log)
- [ ] cert-manager integration for automatic TLS renewal
- [ ] Grafana alert notification channels (PagerDuty, Slack)
- [ ] af-core integration test harness
- [ ] Wire `AF_CORS_ORIGINS` env var

### v1.3.0
- [ ] Per-tenant audit log retention policy with automated purge
- [ ] Span search with SQL-like DSL
- [ ] Cost optimisation recommendations (LLM router suggestions)
- [ ] Policy marketplace (share custom policies)
- [ ] OpenTelemetry Collector distribution packaging

---

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/my-feature`)
3. Run tests (`make test && make lint`)
4. Commit with a descriptive message
5. Open a Pull Request against `main`

All PRs require at least one peer review approval before merge.

---

## Support

- **Issues** — GitHub Issues for bugs and feature requests
- **Discussions** — GitHub Discussions for questions and ideas
- **Docs** — [Full documentation](https://docs.agentfabric.io) *(coming soon)*
- **Email** — support@agentfabric.io *(coming soon)*

---

## License

Proprietary — All rights reserved. See LICENSE file for details.

---

**Built for AI agents in production.**
