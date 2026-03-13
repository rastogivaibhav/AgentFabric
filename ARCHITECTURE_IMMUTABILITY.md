# AgentFabric — Architecture Immutability Map
**What can change, what can't, and what requires a major version bump**

---

## **SACRED ARCHITECTURE (Cannot Change)**

These are the load-bearing walls of AgentFabric. Changing them requires v2.0 and major customer communication.

### **1. The Five-Service Pipeline**

```
Agent SDK → Collector → API Gateway → Portal
                ↓
           af-core → PostgreSQL
```

**What's sacred:**
- ✅ Collector receives OTLP spans (gRPC :4317, HTTP :4318)
- ✅ Collector enriches and forwards to API Gateway
- ✅ API Gateway stores in PostgreSQL + broadcasts to WebSocket
- ✅ af-core processes via Kafka (eventual consistency path)
- ✅ Portal queries API Gateway (no direct DB access)

**What can change:**
- Transport protocol (gRPC → gRPC/HTTP dual, fine; adding GraphQL to gateway, fine)
- Storage backend (PostgreSQL → PostgreSQL + backup store, fine; removing PostgreSQL, breaking)
- Kafka → replaced with RabbitMQ (breaking, requires v2.0)
- API Gateway internals (how it processes, doesn't matter; what it returns, it's a contract)

**Red lines:**
- ❌ Removing any of the five services
- ❌ Adding a sixth service without PRB approval
- ❌ Skipping af-core (policy decisions must go through governance path)
- ❌ Collector directly writing to ClickHouse (bypasses API Gateway)

---

### **2. The Seven Principles (Architectural Law)**

These are enforced in every service. Violating one requires a major version.

| Principle | Level | Enforcement | Violation Cost |
|---|---|---|---|
| Server-side semantic truth | Core | Code review + tests | Major version |
| Multi-tenant isolation (RLS) | Core | Code review + tests | Major version |
| Async governance + hot observability | Core | Architecture review | Major version |
| Immutable audit trail with hash chain | Core | Code review + compliance | Major version |
| Server-computed framework | Core | Code review | Minor version (config) |
| Cost immutability | Core | Code review + audit | Major version |
| Defense-in-depth PII scrubbing | Security | Code review + tests | Major version |

---

### **3. The Data Contract (Immutable Types)**

These types define what flows between services. Changing them requires version negotiation.

**Collector → API Gateway**
```json
{
  "trace_id": "string",
  "span_id": "string",
  "span_kind": "INTERNAL|SERVER|CLIENT|PRODUCER|CONSUMER|UNSPECIFIED",
  "framework": "crewai|langgraph|openai_agents|claude_agents|google_adk",
  "start_time_unix_nano": "number",
  "end_time_unix_nano": "number",
  "attributes": { "key": "value" },
  "events": [ { "name": "string", "timestamp": "number" } ],
  "status_code": "0|1|2",
  "status_message": "string",
  "cost_usd": "number",
  "input_tokens": "number",
  "output_tokens": "number",
  "tenant_id": "string"
}
```

**Changes allowed (backward-compatible):**
- ✅ Adding optional fields (`cache_tokens`, `latency_percentile`)
- ✅ Widening types (`cost_usd` stays number)

**Changes breaking:**
- ❌ Removing fields
- ❌ Renaming fields
- ❌ Changing field types (`cost_usd` from number to string)

**API Gateway → Portal**

```typescript
interface Page<T> { items: T[]; total: number; has_more: boolean }
interface Trace { id, root_span_name, framework, start_time, duration_ns, span_count, error_count, total_cost_usd, total_tokens, status, spans? }
interface OverviewStats { total_traces, active_agents, total_cost_usd, total_tokens, error_rate, avg_latency_ms, spans_per_second, framework_counts }
interface LiveEvent { type: 'span'|'run_start'|'run_end'|'error'|'policy'; ts: number; data: any }
```

**These types are **locked**. If Portal expects `root_span_name` and Gateway returns `name`, it's a bug (not a feature). The gateway MUST return exactly this shape, forever.**

---

### **4. The Deployment Model**

Immutable facts about how AgentFabric deploys:

- ✅ Single Kubernetes cluster (or Docker Compose locally)
- ✅ All services in same VPC (no cross-region yet)
- ✅ PostgreSQL is single source of truth (no sharding)
- ✅ Kafka topic name is `agent-spans` (immutable)
- ✅ Redis is used only for WebSocket session store (stateless; can be replaced)
- ✅ ClickHouse is analytics-only (not queried by Portal in v1)

**Changing:**
- ❌ Multi-region deployment (future version, requires major overhaul)
- ❌ Sharded PostgreSQL (future version, requires RLS redesign)
- ❌ Distributed audit chain in the hot path (breaks async governance principle)

---

## **FLEXIBLE ARCHITECTURE (Can Change)**

These are implementation details. Change them freely within a version.

### **Service Internals**

| Service | Can Change | Cannot Change |
|---|---|---|
| **Collector** | Worker pool size, batch timeout, prometheus port | OTLP protocol, enrichment semantics, PII patterns |
| **af-core** | Kafka consumer batch size, PostgreSQL query structure | Policy decisions, hash chain algorithm, tenant isolation |
| **API Gateway** | HTTP handlers, response formatting, cache headers | Multi-tenancy enforcement, auth middleware, data types |
| **Portal** | React component structure, CSS, recharts config | API contract (types), route names, WebSocket URL |
| **Agent SDK** | Instrumentation hook order, attribute naming (within gen_ai.* namespace) | Framework patches (CrewAI, LangGraph, etc.), OTLP transport |

### **Configuration & Tuning**

These can change without code changes (env vars, ConfigMaps):

- Worker pool size (collector)
- Batch timeout (collector)
- Prometheus scrape interval (any service)
- Database connection pool size
- Kafka consumer group ID
- WebSocket reconnect interval (portal)
- JWT expiration (auth)
- PII regex patterns (with audit trail)
- Policy thresholds (sovereignty region, cost ceiling, rate limit)

---

## **THE VERSIONING SCHEME**

```
v[MAJOR].[MINOR].[PATCH]

v1.0.0 (2025-Q3)  — Production-Ready
├─ v1.1.0 (2025-Q4)  — OIDC login, alerting, audit visualization (backward compatible)
├─ v1.2.0 (2026-Q1)  — Distributed audit chain, custom policies, capacity planner
├─ v1.3.0 (2026-Q2)  — Anomaly alerts, A/B testing, cost optimization engine
└─ v1.4.0 (2026-Q3)  — Regulatory reports, CI/CD integration

v2.0.0 (2027+)  — Reserved for:
├─ Multi-region deployment
├─ Sharded PostgreSQL (if volume demands)
├─ Kafka → RabbitMQ replacement
├─ Custom policy marketplace in production
└─ Agent auto-remediation (autonomous agents)
```

**Rule of thumb:**
- **PATCH** (1.0.1): Bug fix, config change, performance tuning
- **MINOR** (1.1.0): New feature, new policy type, new framework support (additive only)
- **MAJOR** (2.0.0): Removing features, changing protocols, breaking API contracts, changing Principles

---

## **WHAT BREAKS IN A MAJOR VERSION**

If you're planning v2.0, prepare for:

1. **Customer communication** (12 weeks before release)
2. **Migration guide** (for each customer, unique to their config)
3. **Data migration** (if schema changes)
4. **API deprecation period** (v1.x supports both old + new for 6 months)
5. **Support cost** (double for 6 months during transition)

**Example v2.0 scenario:**

```
Current (v1.x): Single PostgreSQL, all tenants, all regions
Desired (v2.0): Sharded PostgreSQL (US/EU/APAC), RLS per shard

Migration:
1. Announce v2.0 in v1.4 release notes (Q3 2026)
2. v1.5 (Q4 2026): Support both v1 (single) and v2 (sharded) deployments
3. v2.0 (Q1 2027): v2 becomes default; v1 is in legacy support mode
4. v2.2 (Q3 2027): v1 reaches end of support

Cost: 4 engineers × 6 months + customer support burden + data migration work
```

**This is why we lock the architecture now. Major versions are expensive.**

---

## **DECISION TREE: "Is This a Breaking Change?"**

```
┌─ Does it change a Principle (1-7)?
│  ├─ YES → MAJOR VERSION
│  └─ NO  → Continue
│
├─ Does it change the data contract (field names, types)?
│  ├─ YES → MAJOR VERSION
│  └─ NO  → Continue
│
├─ Does it remove a service?
│  ├─ YES → MAJOR VERSION
│  └─ NO  → Continue
│
├─ Does it change how tenants are isolated?
│  ├─ YES → MAJOR VERSION
│  └─ NO  → Continue
│
├─ Does it change the API Gateway response format?
│  ├─ YES, breaking response shape → MAJOR VERSION
│  ├─ MAYBE, adding optional field → MINOR VERSION
│  └─ NO  → Continue
│
├─ Does it change the OTLP ingestion contract?
│  ├─ YES, removing required fields → MAJOR VERSION
│  ├─ MAYBE, making new attributes required → MAJOR VERSION
│  └─ NO  → Continue
│
├─ Does it change configuration schema?
│  ├─ YES, removing required config → MAJOR VERSION
│  ├─ MAYBE, adding required config → MINOR VERSION
│  └─ NO  → Continue
│
└─ Everything else is PATCH or MINOR VERSION
```

---

## **IMMUTABILITY ENFORCED BY CODE**

These checks run in CI/CD. A PR that violates them is automatically rejected.

### **Drift Detector (Prometheus)**

Every service exports these metrics. If they diverge (versions mismatch), alert fires:

```
agentfabric_service_version{service="collector",version="1.2.3"}
agentfabric_service_version{service="api-gateway",version="1.2.3"}
agentfabric_service_version{service="af-core",version="1.2.3"}
agentfabric_service_version{service="portal",version="1.2.3"}
```

**Alert rule**: If any two services have different MINOR versions, page on-call.

### **Schema Validation**

Before deploying to production, API Gateway must validate:

```
POST /internal/health/validate-contract HTTP/1.1
{
  "expects": {
    "Page<Trace>": ["items", "total", "has_more"],
    "Trace": ["id", "root_span_name", "framework", ...]
  }
}
```

Response must match exactly, or deployment is blocked.

### **Git Hooks (Pre-Commit)**

Every commit runs:

```bash
$ git-hooks/check-breaking-changes.sh
  ✓ No field removals in api.ts
  ✓ No Principle violations in code comments
  ✓ No hardcoded v1.x assumptions in v2 code paths
```

---

## **SACRED DECISION: Why We Won't Change the Core Architecture**

### **Why We Chose This Architecture in the First Place**

1. **Five services** allow **clean separation of concerns**: each team owns one service
2. **Async Kafka path** enables **scalability**: policy engine doesn't block ingestion
3. **PostgreSQL as SSOT** enables **auditing**: all decisions are queryable
4. **API Gateway as facade** enables **evolution**: Portal and external tools use same contract
5. **OTLP ingestion** enables **language agnostic**: any agent framework, any language

### **Why We Won't Abandon This Architecture**

| Temptation | Why We'll Resist |
|---|---|
| "Merge Kafka → af-core into Collector" | Breaks async governance principle; policy enforcement becomes hot path; latency explodes |
| "Use GraphQL instead of REST" | Portal uses REST; changing gateway breaks 3-layer contract; no performance gain |
| "Use DynamoDB instead of PostgreSQL" | DynamoDB has no RLS; multi-tenancy becomes application-level; audit trail becomes expensive |
| "Remove af-core, do policy in Portal" | Client-side policy is untrustworthy; CISO cannot verify; compliance fail |
| "Add real-time streaming (WebRTC, QUIC)" | Current WebSocket is sufficient; adds complexity; no customer request |
| "Support cross-region replication" | Breaks single SSOT principle; audit chain becomes eventual consistent; too risky for v1 |

**Bottom line: Every part of this architecture solves a real problem. Don't change it unless the problem changes.**

---

## **HOW TO PROPOSE AN ARCHITECTURAL CHANGE**

If you believe the architecture should change, follow this process:

1. **Document the problem** (1 page)
   - What's broken about current architecture?
   - How does it impact customers/developers/CISO?
   - Is it a blocker for revenue or compliance?

2. **Propose a solution** (3 pages)
   - Detailed architecture change
   - Migration path (how do we get from v1 to v2?)
   - What breaks? (what customers must change?)
   - Effort estimate (engineering weeks)

3. **Risk analysis** (1 page)
   - What could go wrong?
   - Mitigation for each risk
   - Rollback plan

4. **Business case** (1 page)
   - Revenue impact (customers willing to migrate?)
   - Cost of migration (engineering + customer support)
   - Timeline (when do we need this?)

5. **Submit to Principles Review Board**
   - Principal Tech Lead
   - Product Manager
   - CISO advisor
   - If accepted, becomes a v2.0 epic

**Expect 2-week review cycle. High bar to clear.**

---

## **THE IMMUTABILITY CHECKLIST**

Every quarter, confirm:

- [ ] All services report same MINOR version
- [ ] Data contract (types) unchanged from v1.0
- [ ] All Principles 1-7 verified in code review
- [ ] No pending architectural PRBs (blocked features)
- [ ] No hardcoded assumptions about v1 architecture
- [ ] Migration path documented (if heading to v2)

---

## **GLOSSARY**

- **Hot path**: Collector → API Gateway → Portal (real-time, <100ms SLA)
- **Governance path**: Kafka → af-core → audit trail (eventual consistency, policy enforcement)
- **SSOT**: Single Source of Truth (PostgreSQL holds all truth)
- **RLS**: Row-Level Security (PostgreSQL tenant isolation)
- **Data contract**: Type definitions between services (must be backwards-compatible within a version)
- **Breaking change**: Violates one of the 7 Principles or changes data contract
- **Sacred**: Architectural decision that cannot change without v2.0

---

**This map is the source of truth for what can and cannot change in AgentFabric v1.x.**

**Last updated**: 2025-03-13
**Next review**: 2025-06-13
