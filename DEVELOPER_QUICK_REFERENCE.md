# Govagn — Developer Quick Reference
**Print this. Keep it by your desk. Reference before every PR.**

---

## **THE 7 CORE PRINCIPLES (Copy This)**

| # | Principle | Violation = | Test It |
|---|-----------|-----------|---------|
| 1️⃣ | **Server-side semantic truth** | Trusting client attributes | Does collector recompute? |
| 2️⃣ | **Multi-tenant isolation** | Queries without tenant filter | Add `WHERE tenant_id = $1` |
| 3️⃣ | **Async governance, hot observability** | Policy enforcement in collector | Move to Kafka → af-core |
| 4️⃣ | **Immutable audit trail** | Policy decisions not logged | Add to hash chain |
| 5️⃣ | **Server-computed framework** | Trusting client framework attr | Verify at collector |
| 6️⃣ | **Cost immutability** | Modifying historical cost | Cost is set once, forever |
| 7️⃣ | **Defense-in-depth PII scrubbing** | Skipping regex for "safe" fields | Scrub at collector + af-core |

---

## **BEFORE YOU COMMIT**

```bash
# 1. Run test suite
$ pytest tests/unit tests/integration -v --cov

# 2. Check for anti-patterns
$ grep -r "client.*policy\|trust.*framework\|hardcoded.*pricing" src/ | grep -v test

# 3. Add unit test
# (Your code has no test if it doesn't have a corresponding test file)

# 4. Add integration test
# (If you touch collector, api-gateway, or af-core)

# 5. Reference this checklist in your PR description
# Add: "✅ Principles: [list which ones apply]"
#      "✅ Tests: [coverage %]"
#      "✅ BREAKING: [yes/no, note in CHANGELOG if yes]"
```

---

## **THE DECISION TREE (One Page)**

**Q1: Does it involve storing/processing agent data?**

- **YES** → Q2
- **NO** → Q3

**Q2: Does it compute/verify attributes server-side?**

- **YES, it recomputes** → Q4
- **NO, it trusts client** → ❌ REJECTED: Recompute server-side
- **N/A, it just reads** → ❌ REJECTED: Add server-side verification

**Q4: Does every query filter by tenant?**

- **YES** → Q5
- **NO** → ❌ REJECTED: Add `WHERE tenant_id = $1`
- **N/A, not a query** → Q5

**Q5: Does it log to audit trail?**

- **YES, it logs decisions** → ✅ APPROVED
- **NO, but it doesn't make decisions** → ✅ APPROVED
- **NO, and it should log** → ❌ REJECTED: Add audit logging

---

**Q3: Does it process OTLP spans?**

- **YES, it's in collector** → Q6
- **YES, it's in af-core** → ✅ APPROVED
- **NO** → Q7

**Q6: Is latency <50ms?**

- **YES** → ✅ APPROVED
- **NO, or unknown** → ❌ Move logic to af-core (via Kafka)

---

**Q7: Does it modify historical data?**

- **YES** → ❌ REJECTED: Immutability is sacred
- **NO** → Q8

**Q8: Does it touch credentials/tokens/PII?**

- **YES** → Q9
- **NO** → ✅ APPROVED

**Q9: Is it scrubbed before logging?**

- **YES, regex + JSON recursion** → ✅ APPROVED
- **NO, or partial scrubbing** → ❌ REJECTED: Add scrubbing

---

## **CODE REVIEW CHECKLIST (Paste Into PR Description)**

```markdown
## Principles Alignment

- [ ] Principle 1 (Server-side truth): Not trusting client attributes
- [ ] Principle 2 (Multi-tenancy): All queries filter by tenant_id
- [ ] Principle 3 (Async governance): Policy logic not in hot path
- [ ] Principle 4 (Immutable audit): Policy decisions are logged
- [ ] Principle 5 (Framework detection): Framework recomputed server-side
- [ ] Principle 6 (Cost immutability): No retroactive cost modifications
- [ ] Principle 7 (PII defense): Scrubbing at both collector and af-core

## Testing

- [ ] Unit tests added (target: 60%+ coverage)
- [ ] Integration tests added (if touching collector/gateway/core)
- [ ] E2E test considered (if end-user-facing)
- [ ] All existing tests pass

## Documentation

- [ ] README updated in affected service directory
- [ ] CHANGELOG entry added (if breaking change)
- [ ] Architecture docs updated (if changing data flow)

## Risk

- [ ] No hardcoded business logic (pricing, thresholds, allowlists)
- [ ] No N+1 queries
- [ ] No silent failures (all errors logged + alerted)
- [ ] BREAKING: [yes/no] — if yes, noted in CHANGELOG
```

---

## **WHAT EACH SERVICE OWNS**

| Service | Responsible For | DON'T DO | DO THIS |
|---|---|---|---|
| **Collector** | Framework detection, PII scrubbing, cost computation, BatchSpanProcessor | Policy enforcement (too slow), Long-lived state | Keep latency <50ms, Recompute all attributes |
| **af-core** | Policy evaluation, audit logging, trace assembly, ClickHouse writes | Direct HTTP responses (use api-gateway), Hardcoding business logic | Async processing, Kafka consumer, Hash chain |
| **API Gateway** | REST API, WebSocket stream, JWT auth, multi-tenancy enforcement | Policy enforcement (wrong layer), PII scrubbing (too late) | Expose af-core queries, Stream telemetry |
| **Portal** | UI for dashboard/traces/live/agents/cost/environments | Store state in localStorage, Trust auth tokens (validate in gateway) | Query via /api/v1 only, Use React Query, No hardcoded URLs |
| **Agent SDK** | Auto-instrument CrewAI/LangGraph/OpenAI/Anthropic/Google ADK | Enforce policies (client-side), Compute costs | Emit standard attributes, Support manual instrumentation |

---

## **ANTI-PATTERNS CHEATSHEET**

| Anti-Pattern | Example | Fix |
|---|---|---|
| "Works in dev" | In-memory state, synchronous writes | Test with 10 instances, async queues |
| "Trust the client" | Reading `af.policy.decision` from SDK | Recompute on server; delete client value |
| "Hardcoded logic" | Pricing table in code | Move to config/DB, zero-downtime updates |
| "Eventual consistency?" | Span in Postgres but not ClickHouse | Define explicit SLA, implement reconciliation |
| "Silent loss" | Kafka parse error, then continue | Dead-letter queue, Prometheus counter, alert |
| "Unowned complexity" | 10-parallel N+1 queries in handlers.go | Refactor to service layer, assign owner |

---

## **WHAT'S OKAY (With Documentation)**

| Change | Allowed If | Deadline |
|---|---|---|
| **Violate Principle X** | PRB approves, documented in code, temp deadline set, compensating control exists | Q[quarter] [year] |
| **Hardcode a value** | It's truly constant (e.g., regex pattern), not business logic | N/A |
| **Skip tests** | Feature is <100 LOC, non-critical, and approved by QA lead | PR review gate only |
| **Modify db schema** | Migration script provided, backwards-compatible, tested in staging | N/A |
| **Add new service** | PRB approves, documented in ARCHITECTURE.md, deployment runbook | N/A |

---

## **COMMON PITFALLS & FIXES**

### **Pitfall 1: N+1 Query**
```go
// ❌ BAD: Loop + query inside loop
for _, run := range runs {
    graph := store.GetTraceGraph(run.TraceID)  // N queries!
}

// ✅ GOOD: Batch query
graphs := store.GetTraceGraphs(run IDs)  // 1 query
```

### **Pitfall 2: Client-Trusted Attribute**
```go
// ❌ BAD: Trusting client
framework := span.Attributes["framework"]

// ✅ GOOD: Recompute server-side
framework := detectFramework(span)  // Recompute
```

### **Pitfall 3: Skipping Scrubbing**
```python
# ❌ BAD: "UUID is safe, skip regex"
for attr in span.attributes:
    if attr.key == "internal_id":
        continue  # Skip scrubbing
    else:
        scrub(attr)

# ✅ GOOD: Scrub everything, no exceptions
for attr in span.attributes:
    scrub(attr)
```

### **Pitfall 4: No Tenant Filter**
```sql
-- ❌ BAD: Global query
SELECT * FROM spans WHERE timestamp > now() - interval '1 hour'

-- ✅ GOOD: Always filter tenant
SELECT * FROM spans WHERE tenant_id = $1 AND timestamp > now() - interval '1 hour'
```

### **Pitfall 5: Modifying Historical Cost**
```go
// ❌ BAD: Retroactive "correction"
UPDATE spans SET cost_usd = new_cost WHERE trace_id = ?

// ✅ GOOD: Cost is immutable; document discrepancy
// If pricing was wrong, new spans use new pricing.
// Add note to CHANGELOG: "Fixed cost computation bug on 2025-03-15;
// spans ingested before this date may have incorrect cost."
```

---

## **TESTING PATTERNS**

### **Unit Test Pattern**
```python
def test_framework_detection():
    # Arrange
    span = create_span(
        attributes={"gen_ai.system": "openai", ...}
    )

    # Act
    framework = detect_framework(span)

    # Assert
    assert framework == "openai_agents"
    assert span.server_attributes["framework"] == "openai_agents"
    assert span.client_attributes["framework"] is None  # Never trust client
```

### **Integration Test Pattern**
```python
def test_pii_scrubbing_end_to_end():
    # Arrange
    collector = start_collector()
    gateway = start_gateway()

    # Act
    collector.ingest(span_with_email="alice@company.com")
    time.sleep(1)  # Batch flush

    # Assert
    stored = gateway.api.get_spans()
    assert "[REDACTED]" in stored[0].attributes["email"]
    assert "alice@company.com" not in json.dumps(stored)
```

---

## **DEPLOYMENT CHECKLIST**

Before deploying to production:

- [ ] All tests pass (unit + integration + E2E)
- [ ] Code coverage ≥60%
- [ ] No hardcoded credentials (search for `password`, `secret`, `api_key`)
- [ ] All Principles 1-7 verified
- [ ] CHANGELOG updated
- [ ] Deployment script tested in staging
- [ ] Rollback plan documented
- [ ] Monitoring alerts configured
- [ ] On-call engineer notified
- [ ] Customer communication drafted (if breaking)

---

## **ESCALATION CHECKLIST**

Reach out to PRB (Product + Tech Lead + CISO) if:

- [ ] Violating one of the 7 Principles
- [ ] Adding a new service
- [ ] Adding a new data store
- [ ] Changing hot path latency
- [ ] Affecting PII/audit
- [ ] Asking for multi-tenancy exception
- [ ] Estimated >4 weeks engineering

---

## **TEMPLATES**

### **PR Title**
```
[feature|fix|refactor]: [service] short description

Examples:
- feat: collector add gpt-4o support
- fix: af-core N+1 query in topology builder
- refactor: api-gateway move cost rollup to service layer
```

### **Commit Message**
```
[service] Short description

- Detailed change 1
- Detailed change 2

Principles: [list which 1-7 apply]
Tests: [coverage % or link to test file]
Fixes: [GitHub issue #]
```

### **PR Description**
```markdown
## What
Brief description of what changed

## Why
Why this change was needed (customer request, bug, tech debt, etc.)

## How
How the change works (architecture, algorithm, etc.)

## Testing
How to verify (manual steps, test file, etc.)

## Principles Alignment
[Paste checklist from above]

## BREAKING
[yes/no] If yes, CHANGELOG entry and migration guide required
```

---

**Keep this page bookmarked. Reference before every commit.**

**Questions? Ask #engineering or escalate to PRB.**

**Last updated**: 2025-03-13
