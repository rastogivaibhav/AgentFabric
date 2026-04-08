# Govagn â€” Execution North Star
**The single source of truth for all development decisions**

Last updated: 2025-03-13
Status: ACTIVE â€” All PRs must reference this document

---

## **I. ONE-LINE MISSION**

> **Govagn is the mandatory governance layer for multi-agent enterprises: we make cost, compliance, and quality observable and enforceable at the server-side semantic layer (not OTEL-layer).**

---

## **II. CORE ARCHITECTURAL PRINCIPLES**

These are immutable. If a feature violates one, it fails review.

### **Principle 1: Server-Side Semantic Truth**
- **The rule**: All critical decisions (framework detection, cost, compliance) are recomputed server-side. Never trust client claims.
- **Why**: A developer could patch their SDK to claim "no PII" when there is. The CISO needs proof, not hope.
- **Example**: Client sends `af.policy.decision=allow`. Collector deletes it (`agent_processor.go:239`) and recomputes.
- **Consequence**: Code that reads/trusts client-submitted attributes like `ai.policy.decision` or `af.policy.trusted` is a bug report, not a feature.

### **Principle 2: Multi-Tenant Isolation is Non-Negotiable**
- **The rule**: Every query in every service includes tenant context. Row-level security at the DB layer.
- **Why**: A CISO customer's data must never be visible to another tenant, even by accident.
- **Example**: `SET LOCAL app.tenant_id = $1` in af-core before every query. API gateway extracts tenant from JWT claims.
- **Consequence**: Any new endpoint that doesn't filter by tenant is a security bug.

### **Principle 3: Async Governance, Hot Observability**
- **The rule**:
  - **Hot path** (collector â†’ api-gateway â†’ PostgreSQL): <100ms, serves live UI
  - **Governance path** (Kafka â†’ af-core â†’ audit log): eventual consistency, policy enforcement
- **Why**: Users need live traces immediately. Policy decisions can be slightly delayed.
- **Example**: Span appears in portal dashboard within 500ms. Policy violation email arrives within 5 minutes.
- **Consequence**: Moving policy enforcement to the hot path (e.g., synchronous collector-side decision) is an anti-pattern; use Kafka queues instead.

### **Principle 4: Immutable Audit Trail with Cryptographic Integrity**
- **The rule**: Every policy decision is written to audit log with SHA-256 hash chain. Chain can be verified.
- **Why**: HIPAA/SOX auditors ask: "Prove you evaluated and logged every decision." Hash chain is the proof.
- **Example**: CISO runs `verify_audit_chain()` on PostgreSQL; if any entry is deleted or reordered, verification fails at that index.
- **Consequence**: New code that logs policy decisions without proper chaining is incomplete.

### **Principle 5: Framework Detection is Server-Computed, Not Attribute-Based**
- **The rule**: Client sends OTLP spans. Collector detects framework via heuristics. Framework attribute is always server-set.
- **Why**: Prevents a malicious agent from claiming it's CrewAI when it's not.
- **Example**: SDK emits `gen_ai.system=openai` + span name + attributes. Collector cross-references all three, decides framework.
- **Consequence**: Any feature that uses `framework` attribute without verifying it came from af-core is a bug.

### **Principle 6: Cost is Computed Once at Ingestion, Never Adjusted**
- **The rule**: Collector computes cost from model name + token counts. That cost is final. No retroactive adjustments.
- **Why**: Cost is the basis for chargeback and policy decisions. It must be immutable and auditable.
- **Example**: If pricing changes, new spans use new prices. Old spans keep their old cost. Report shows "pricing change on 2025-03-15 14:00 UTC".
- **Consequence**: Code that modifies `cost_usd` after ingestion (even to "correct" the 60/40 bug) is retroactive falsification.

### **Principle 7: PII Scrubbing is Defense-in-Depth**
- **The rule**: Scrub at collector (prevent leakage). Scrub at af-core (2x insurance). Never store PII in readable form.
- **Why**: If any component fails, PII still doesn't leak.
- **Example**: Collector regex catches `email=alice@company.com`. af-core regex catches it again. Portal never shows it.
- **Consequence**: New code paths that skip scrubbing (e.g., "this field is safe, skip regex") are security bugs.

---

## **III. EXECUTION FRAMEWORK**

### **Phase Gates (What Must Be True Before Shipping)**

| Phase | Gate 0 | Gate 1 | Gate 2 |
|---|---|---|---|
| **Production-Ready** (Q2-Q3 2025) | Pass code review | 60% test coverage | CISO sign-off |
| **Governance Platform** (Q4 2025â€“Q1 2026) | Pass code review | 70% test coverage | 3+ customers using feature |
| **Intelligence Layer** (Q2+ 2026) | Pass code review | 75% test coverage | Data accuracy within Â±15% |

### **Code Review Checklist (Every PR)**

Before approving any PR, confirm all of these:

- [ ] **Principle 1**: Does it trust client attributes? (ðŸ”´ fail if yes, unless explicitly server-recomputed)
- [ ] **Principle 2**: Does every new query include tenant filtering? (ðŸ”´ fail if no)
- [ ] **Principle 3**: Is policy logic in the hot path (collector)? (ðŸ”´ flag if yes; move to Kafka â†’ af-core)
- [ ] **Principle 4**: Do policy decisions get logged to audit trail? (ðŸ”´ fail if no)
- [ ] **Principle 5**: Does it assume client-provided framework is correct? (ðŸ”´ fail if yes)
- [ ] **Principle 6**: Does it modify historical cost data? (ðŸ”´ fail if yes; immutability is sacred)
- [ ] **Principle 7**: Does it skip PII scrubbing for "safe" fields? (ðŸ”´ fail if yes)
- [ ] **Tests**: Is there a unit test + integration test? (ðŸ”´ fail if <60% coverage)
- [ ] **Docs**: Does the PR update the affected service's README? (ðŸ”´ fail if no)
- [ ] **Backwards compat**: Will this break existing deployments? (ðŸ”´ fail if yes, unless in CHANGELOG under breaking)

---

## **IV. ALLOWED DEVIATIONS FROM ARCHITECTURE**

A deviation is only allowed if **all three** of these are true:

1. **Documented**: The PR explains why the architectural principle must be broken
2. **Temporary**: It has a removal date (e.g., "remove this temporary mock in Q3 2025")
3. **Risk-mitigated**: A compensating control prevents the risk from being realized

### **Example of a Valid Deviation:**

```go
// TEMPORARY: Until af-core is fully multi-instance (Q1 2026),
// we use in-memory Mutex for audit chain instead of PostgreSQL-backed.
// Risk: Chain diverges if multiple instances run concurrently.
// Mitigation: Deployment docs forbid >1 af-core replica; Prometheus alert fires if 2+ detected.
// Removal: https://github.com/rastogivaibhav/Govagn/issues/XXX (target: Q1 2026)
type AuditWriter struct {
    chain Mutex<String>  // â† This is a deviation
}
```

### **Example of an Invalid Deviation:**

```go
// Skip PII scrubbing for "internal_id" field (it's just a UUID, not PII)
// Risk: Maybe it's a UUID, maybe someone adds a customer name; inconsistent
// Mitigation: None (relying on developer good intentions)
// â† REJECTED: No compensating control, no timeline, violates Principle 7
```

---

## **V. ANTI-PATTERNS TO AVOID**

### **Anti-Pattern 1: "It Works in Development"**
- **Definition**: Shipping code that only works when there's one instance, or when timing is perfect
- **Example**: In-memory audit chain, N+1 queries, synchronous blocking writes
- **Fix**: Ask: "Does this work with 10 af-core instances in production?"

### **Anti-Pattern 2: "Trust the Client"**
- **Definition**: Reading attributes submitted by the SDK without server-side verification
- **Example**: Trusting `af.policy.decision` sent by the client
- **Fix**: Always recompute on the server

### **Anti-Pattern 3: "Hardcoded Business Logic"**
- **Definition**: Pricing tables, policy thresholds, allowlists in code instead of config/DB
- **Example**: `modelPricing` map in `agent_processor.go:80`
- **Fix**: Move to config file or database; enable zero-downtime updates

### **Anti-Pattern 4: "Eventual Consistency Without a Plan"**
- **Definition**: Data reaches different services at different times, with no reconciliation
- **Example**: Span in PostgreSQL but not yet in ClickHouse; user sees it in live stream but not in analytics
- **Fix**: Define explicit consistency guarantees (SLA) and implement reconciliation if violated

### **Anti-Pattern 5: "Silent Data Loss"**
- **Definition**: Spans/decisions are dropped with no logging or alerting
- **Example**: Kafka message fails to parse â†’ silent continue â†’ message lost forever
- **Fix**: Dead-letter queue + Prometheus counter + alert

### **Anti-Pattern 6: "Unowned Complexity"**
- **Definition**: Code that's "too important to delete" but nobody maintains it
- **Example**: The topology graph computation in handlers.go that does 10 parallel N+1 queries
- **Fix**: Every algorithm/computation has an owner; quarterly review of maintenance load

---

## **VI. MEASUREMENT CRITERIA**

### **How We Know We're On Track**

Each quarter, measure these metrics:

| Metric | Target | Current | Owner |
|---|---|---|---|
| **Code coverage** | 70%+ | 0% (start Q2 2025) | QA Lead |
| **Test execution time** | <5 min (full suite) | N/A | QA Lead |
| **CISO sign-off rate** | 100% of enterprise features | TBD | Product Manager |
| **Audit chain verification time** | <10s for 1M entries | N/A | Backend Lead |
| **PII false positive rate** | <1% (redacting non-PII) | N/A | Security Lead |
| **Multi-tenant isolation test pass rate** | 100% | N/A | Backend Lead |
| **Production incident rate** | <1 per month | N/A | DevOps Lead |
| **Customer NPS for CISO features** | >50 | TBD | Product Manager |

---

## **VII. DECISION TREE: "Is This Aligned?"**

Use this tree before starting any feature or refactor:

```
â”Œâ”€ Does it involve storing or processing agent data?
â”‚  â”œâ”€ YES â†’ Does it compute or verify attributes server-side?
â”‚  â”‚  â”œâ”€ YES â†’ Does it filter by tenant?
â”‚  â”‚  â”‚  â”œâ”€ YES â†’ Does it log to audit trail if applicable?
â”‚  â”‚  â”‚  â”‚  â”œâ”€ YES â†’ âœ… APPROVED (proceed)
â”‚  â”‚  â”‚  â”‚  â””â”€ NO  â†’ âŒ REJECTED (add audit logging)
â”‚  â”‚  â”‚  â””â”€ NO  â†’ âŒ REJECTED (add tenant filtering)
â”‚  â”‚  â””â”€ NO  â†’ âŒ REJECTED (add server-side recomputation)
â”‚  â””â”€ NO  â†’ Continue below
â”‚
â”œâ”€ Does it process OTLP spans?
â”‚  â”œâ”€ YES â†’ Is it in the hot path (collector)?
â”‚  â”‚  â”œâ”€ YES â†’ Is it <50ms latency?
â”‚  â”‚  â”‚  â”œâ”€ YES â†’ âœ… APPROVED
â”‚  â”‚  â”‚  â””â”€ NO  â†’ âŒ Move to Kafka â†’ af-core
â”‚  â”‚  â””â”€ NO  â†’ Is it in af-core?
â”‚  â”‚     â”œâ”€ YES â†’ âœ… APPROVED
â”‚  â”‚     â””â”€ NO  â†’ âŒ Move to af-core
â”‚  â””â”€ NO  â†’ Continue below
â”‚
â”œâ”€ Does it modify historical data?
â”‚  â”œâ”€ YES â†’ âŒ REJECTED (immutability is sacred)
â”‚  â””â”€ NO  â†’ âœ… APPROVED
â”‚
â””â”€ Does it involve credentials, tokens, or PII?
   â”œâ”€ YES â†’ Is it scrubbed before logging?
   â”‚  â”œâ”€ YES â†’ âœ… APPROVED
   â”‚  â””â”€ NO  â†’ âŒ REJECTED (add scrubbing)
   â””â”€ NO  â†’ âœ… APPROVED
```

---

## **VIII. ROADMAP ALIGNMENT**

### **Phase 1: Production-Ready (Q2-Q3 2025)**
- âœ… Architecture: Unchanged
- âœ… Core: Collector + API Gateway + af-core + Portal
- âœ… Principles: All 7 must be met
- âš ï¸ Auth: OIDC login (new, but non-breaking)
- âš ï¸ Testing: 60%+ coverage required
- âœ… Governance: All 5 built-in policies, no deviations

### **Phase 2: Governance Platform (Q4 2025â€“Q1 2026)**
- âœ… Architecture: Unchanged
- âš ï¸ Audit chain: Distributed (Principle 4 scaling fix)
- âš ï¸ Custom policies: WASM runtime (extensible, not core)
- âœ… Principles: All 7 still apply to new code
- âœ… Testing: 70%+ coverage required

### **Phase 3: Intelligence Layer (Q2+ 2026)**
- âœ… Architecture: Unchanged
- âœ… Principles: All 7 still apply
- âš ï¸ New: ClickHouse analytics backing (observability, not governance)
- âœ… Testing: 75%+ coverage required

**No architectural changes planned through 2026. If a feature requires architectural change, it goes to Principles Review Board (Product + Tech Lead + CISO customer).**

---

## **IX. WHAT SUCCESS LOOKS LIKE**

By end of Phase 1 (Q3 2025):
- âœ… First enterprise customer running Govagn in production
- âœ… CISO can: detect PII, enforce policies, verify audit chain
- âœ… Developer can: instrument their agent in <3 minutes
- âœ… Ops can: track costs per service, get alerted on overage
- âœ… Zero architectural deviations outstanding (all mitigated or fixed)

By end of Phase 2 (Q1 2026):
- âœ… 3+ enterprise customers
- âœ… Distributed audit chain live
- âœ… 5+ custom policies deployed by customers
- âœ… Cost optimization recommendations working at >85% accuracy

By end of Phase 3 (Q2 2026):
- âœ… $1M ARR
- âœ… Autonomous cost optimization (agents switch to cheaper models automatically within bounds)
- âœ… Regulatory attestation (pre-built SOX/GDPR reports)
- âœ… 50+ customers

---

## **X. WHEN TO INVOKE PRINCIPLES REVIEW BOARD**

If any of these are true, escalate to PRB (Product Manager + Principal Tech Lead + CISO advisor):

1. Feature requires violating one of the 7 Principles
2. Feature requires new service (currently: Collector, af-core, API Gateway, Portal, SDK)
3. Feature requires new data storage (currently: PostgreSQL, Redis, ClickHouse, Kafka)
4. Feature requires changes to the hot path latency (currently: p99 < 100ms)
5. Feature affects PII scrubbing or audit trail
6. Customer is asking for exception to multi-tenancy isolation
7. Feature would cost >4 weeks of engineering (estimated)

---

## **XI. REFERENCE DOCUMENTS**

- **Product Review**: `GOVAGN_REVIEW.md` (comprehensive analysis)
- **Architecture Diagram**: (TODO: add to wiki)
- **Security Whitepaper**: (TODO: create for CISO customers)
- **API Contract**: `api-gateway/internal/models/models.go` (source of truth for portal types)
- **Deployment Guide**: `deploy/` (production runbook)

---

## **SIGNOFF**

This North Star was ratified by:
- **Principal Tech Lead**: [Your name]
- **Product Manager**: [Your name]
- **CISO Advisor**: [To be assigned]

**Next review date**: 2025-06-13 (quarterly)

---

**All development after 2025-03-13 must reference this document.**

