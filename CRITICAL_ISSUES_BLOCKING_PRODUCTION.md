# Critical Issues Blocking Production Deployment
**Must be fixed before v1.0.0 release (Q3 2025)**

---

## **BLOCKING ISSUES (Fix Before Release)**

### **P0: Cost Computation Bug (REVENUE IMPACT)**
**File**: `af-core/src/storage/postgres.rs:131`

**Issue**: The collector computes separate `af.cost.input_usd` and `af.cost.output_usd`, but af-core discards them and uses a hardcoded 60/40 split:

```rust
// ❌ WRONG: Throws away input/output split from collector
let input_cost = (cost_usd * 0.6).into();
let output_cost = (cost_usd * 0.4).into();
```

**Impact**:
- Claude Haiku with cache hits: **$18K/year overcharge** (per 100M spans)
- GPT-4 with high output ratio: **$10K/year undercharge** (per 100M spans)
- Customer on $500K/year spend: **$50–100K billing discrepancy**

**Fix**: Use actual values from collector:
```rust
let input_cost = enriched_span.input_cost_usd;
let output_cost = enriched_span.output_cost_usd;
```

**Effort**: 2 hours
**Owners**: Backend Lead (code) + QA (test case)
**Test**:
```python
def test_input_output_cost_split():
    # Claude Haiku: 1000 input tokens @ $0.08/1M = $0.00008
    # 500 output tokens @ $0.24/1M = $0.00012
    # Expected: input=$0.00008, output=$0.00012
    # Current 60/40: input=$0.00012, output=$0.00008 (WRONG!)
    assert db.spans[0].input_cost_usd == 0.00008
    assert db.spans[0].output_cost_usd == 0.00012
```

**MUST FIX BEFORE**: v1.0.0 release

---

### **P0: N+1 Query in Topology API (PERFORMANCE)**
**File**: `api-gateway/internal/handlers/handlers.go:270`

**Issue**: The endpoint `GET /api/v1/agents/{agentId}/topology` fetches recent runs, then makes a separate query per run:

```go
runs, _ := store.GetRecentRuns(agentID, 10)
for _, run := range runs {
    graph, _ := store.GetTraceGraph(run.TraceID)  // ← 10 separate queries!
}
```

**Impact**:
- Single agent topology load: **5+ seconds** (should be <200ms)
- Dashboard loads "/agents" page → 5s latency → UX feels broken
- CISO abandons product thinking it's slow

**Fix**: Batch query:
```go
runs, _ := store.GetRecentRuns(agentID, 10)
graphs, _ := store.GetTraceGraphsBatch(getTraceIDs(runs))  // 1 query
```

**Effort**: 4 hours (includes testing batching correctness)
**Owners**: Backend Lead
**Test**:
```go
func TestGetAgentTopologyBatching(t *testing.T) {
    agent := createAgent(10_runs)
    start := time.Now()
    graph := api.GetAgentTopology(agent.ID)
    assert.Less(t, time.Since(start), 200*time.Millisecond)
    assert.Equal(t, len(graph.Runs), 10)
}
```

**MUST FIX BEFORE**: v1.0.0 release

---

### **P0: SQL Injection in af-core (SECURITY)**
**File**: `af-core/src/storage/postgres.rs:221`

**Issue**: Dynamic SQL via string formatting:

```rust
// ❌ VULNERABLE: User input not parameterized
let query = format!(
    "SELECT * FROM spans WHERE framework = '{}' AND timestamp > '{}'",
    framework, time_filter
);
```

If `framework` is `' OR '1'='1`, the query breaks.

**Impact**:
- Customer with public API using this parameter: **SQL injection risk**
- Even if not exposed yet, code review will catch it

**Fix**: Use parameterized queries:

```rust
// ✅ SAFE: Parameterized
let query = "SELECT * FROM spans WHERE framework = $1 AND timestamp > $2";
db.query(query, &[&framework, &time_filter])
```

**Effort**: 3 hours
**Owners**: Backend Lead + Security Lead (review)
**Test**: `assert!(execute_sql_injection_test_fails())`

**MUST FIX BEFORE**: v1.0.0 release

---

### **P1: In-Memory Audit Chain Doesn't Scale (ARCHITECTURE)**
**File**: `af-core/src/policy/audit.rs:45`

**Issue**: Hash chain is stored in `Mutex<String>`:

```rust
pub struct AuditWriter {
    chain: Mutex<String>,  // ← In-memory only!
}
```

This works for 1 instance. If you run 2 af-core instances, they produce diverged chains.

**Impact**:
- Can't scale to HA (high availability)
- CISO: "You claim audit trail is tamper-proof, but if you have 2 instances, there are 2 chains?"
- Enterprise won't deploy

**Fix**: Implement PostgreSQL-backed chain:
```rust
// Each af-core instance appends transactionally:
// 1. SELECT previous_hash FROM audit_log ORDER BY id DESC LIMIT 1 FOR UPDATE
// 2. INSERT INTO audit_log (entry, current_hash) VALUES (...)
// 3. COMMIT
```

**Effort**: 3 weeks (includes transaction lock testing)
**Owners**: Backend Lead
**Test**: Deploy 2 af-core instances, verify single coherent chain

**Deadline**: Before Phase 2 (Q4 2025) start
**Can ship v1.0.0 with mitigation**: Deployment docs forbid >1 af-core replica; Prometheus alert fires if 2+ detected

---

### **P1: No Authentication UI (BLOCKER FOR CISO)**
**File**: None (missing entirely)

**Issue**: Portal reads JWT from `localStorage.getItem('af_token')`. There is no login page.

**Impact**:
- CISO cannot log in
- IT must manually generate tokens and inject them
- Non-starter for enterprise deployment

**Fix**: Implement OIDC login page:
```typescript
// portal/src/pages/LoginPage.tsx
// 1. Redirect to OIDC provider
// 2. Handle callback, extract token
// 3. Store in localStorage
// 4. Redirect to dashboard
```

**Effort**: 3 weeks (including OIDC backend support)
**Owners**: Frontend Lead + Backend Lead
**Test**: Login via Okta/Azure AD, access portal, logout

**MUST FIX BEFORE**: v1.0.0 release

---

### **P1: Zero Test Coverage (RISK)**
**File**: Every service

**Issue**: No tests exist in the repository.

**Impact**:
- Can't prove the code works
- Any refactor risks breaking something silently
- Customer asks: "What's your test coverage?" Answer: "0%"

**Fix**: Phase 1 goal is 60%+ coverage.
- Unit tests: SDK + Collector + af-core (6 weeks)
- Integration tests: Hot path + async path (3 weeks)
- E2E tests: Full agent → portal flow (2 weeks)

**Effort**: 8 weeks
**Owners**: QA Lead + all engineers

**MUST FIX BEFORE**: v1.0.0 release

---

### **P2: Hardcoded Pricing Table (TECH DEBT)**
**File**: `collector/internal/processor/agent_processor.go:80`

**Issue**: Model pricing is hardcoded:

```go
modelPricing := map[string][2]float64{
    "gpt-4":        {0.03, 0.06},
    "claude-3-opus": {0.015, 0.075},
    // ...
}
```

When new models launch (gpt-4.5, claude-3.7), code must be updated and redeployed.

**Impact**:
- Customer uses gpt-4.5: Cost computed as gpt-4 pricing until code is deployed
- Billing inaccuracy

**Fix**: Move to config or database:
```go
// Load from ConfigMap
var modelPricing map[string][2]float64
viper.UnmarshalKey("MODEL_PRICING", &modelPricing)
```

**Effort**: 1 week
**Owners**: Backend Lead
**Test**: Update pricing via ConfigMap, verify new prices are used

**Deadline**: Phase 1, after authentication is done

---

### **P2: TopologyGraph Component is Unimplemented (UX)**
**File**: `portal/src/pages/TraceDetail.tsx` references `TopologyGraph.tsx` which doesn't exist

**Issue**: The "Graph" tab on TraceDetail shows a placeholder. The component is missing.

**Impact**:
- Customer clicks on "Graph" tab → sees "Not implemented"
- Looks unfinished

**Fix**: Implement D3 force-directed graph of spans:
```typescript
// portal/src/components/TopologyGraph.tsx
export function TopologyGraph({ spans }: { spans: Span[] }) {
  // Build D3 graph from spans (nodes=spans, edges=parent/child)
  // Render force-directed layout
  // Show span names, costs, latencies
}
```

**Effort**: 2 weeks
**Owners**: Frontend Lead

**Deadline**: Phase 1, after auth (v1.0.0)
**Can ship without it**: Mark as "coming soon" and hide the tab

---

## **DEFERRED ISSUES (Phase 2–3)**

| Issue | Impact | Effort | Deadline |
|---|---|---|---|
| Policy violations have no alerting (no Slack/email integration) | CISO can't respond quickly | 2w | Phase 1 (v1.0.0) |
| Cost optimizer not wired (algorithm exists, no UI) | Can't show cost saving recommendations | 4w | Phase 3 (Q2 2026) |
| Google ADK has no auto-instrumentation | Google ADK users must manually instrument | 2w | Phase 2 (v1.2.0) |
| LangGraph node-level tracing missing | No fine-grained insight into node execution | 3w | Phase 2 (v1.2.0) |
| ClickHouse analytics not surfaced | Portal can't do complex analytics queries | 4w | Phase 2 (v1.2.0) |

---

## **PROCESS FOR FIXING BLOCKING ISSUES**

### **Triage & Planning (1 day)**
- [ ] Create GitHub issue for each P0/P1
- [ ] Assign owner
- [ ] Estimate effort
- [ ] Set deadline
- [ ] Link to Phase 1 epic

### **Implementation (per issue effort)**
- [ ] Owner creates feature branch
- [ ] Implement fix
- [ ] Add tests (unit + integration)
- [ ] Self-review against checklist
- [ ] Submit PR

### **Code Review (1-2 days)**
- [ ] Principal Tech Lead reviews
- [ ] Verify fix doesn't violate Principles 1-7
- [ ] Verify tests pass
- [ ] Check for regressions

### **Verification (1 day)**
- [ ] QA tests in staging
- [ ] Performance benchmarks (if applicable)
- [ ] Manual smoke test
- [ ] Merge to main

### **Deployment (1 day)**
- [ ] Deploy to production
- [ ] Monitor metrics
- [ ] Document in CHANGELOG

---

## **DEPENDENCY CHAIN**

```
Cost Bug (P0)
  └─ Must fix before chargebacks start
     └─ Phase 1 release

OIDC Login (P0)
  └─ Must fix before customer signup
     └─ Phase 1 release

N+1 Query (P0)
  └─ Must fix before dashboard demo
     └─ Phase 1 release

Tests (P0)
  └─ Must reach 60% coverage
     └─ Phase 1 release

SQL Injection (P0)
  └─ Must fix before security audit
     └─ Phase 1 release

Distributed Audit (P1)
  └─ Mitigated for Phase 1
     └─ Fixed in Phase 2 release

Topology Graph (P2)
  └─ Can be placeholder for Phase 1
     └─ Full implementation in Phase 1 or Phase 2
```

---

## **SUCCESS CRITERIA FOR PHASE 1**

Before declaring Phase 1 (v1.0.0) complete:

- [ ] Cost bug fixed and verified
- [ ] N+1 query fixed and verified
- [ ] SQL injection fixed and verified
- [ ] OIDC login working end-to-end
- [ ] Audit chain mitigation in place (deployment docs + alert)
- [ ] 60%+ test coverage
- [ ] All P0 issues resolved
- [ ] All P1 issues resolved or mitigated
- [ ] One customer successfully deployed and running

---

## **ESCALATION**

If any blocking issue is:
- **Harder than estimated**: Escalate to Principal Tech Lead + Product Manager
- **Requires architectural change**: Escalate to Principles Review Board
- **Discovered to be more critical**: Re-triage (may move deadline)

---

## **QUICK REFERENCE: Fix Order**

1. **Week 1-2**: Cost bug, SQL injection, N+1 query (3 parallel)
2. **Week 3-6**: OIDC login (includes backend)
3. **Week 7-10**: Test suite (60%+ coverage, ongoing)
4. **Week 11-12**: Audit chain mitigation, polish, hardening

---

**Status**: 🔴 **BLOCKING** — Start immediately upon Phase 1 kickoff

**Owner**: Backend Lead + Frontend Lead + QA Lead

**Next Review**: Weekly standup every Monday

---

**Last Updated**: 2025-03-13
