# AgentFabric Development — North Star Documents

This directory contains the **source of truth** for all AgentFabric development decisions. Read these first.

---

## **📋 Documents (Read in Order)**

### **1. AGENTFABRIC_REVIEW.md** (80 pages)
**What**: Comprehensive product & technical review from Microsoft Principal Tech Lead + Product Manager perspective

**Contains**:
- What the product does and its market potential ($500M opportunity)
- Current state vs. future state impact analysis
- Deep technical assessment of all 5 services
- 15+ specific capabilities for future extension
- Easter eggs and hidden value in the codebase

**Read if**: You're new to the project, need strategic context, or are evaluating investment

**Time**: 45 minutes

---

### **2. NORTH_STAR.md** (Immutable)
**What**: The single source of truth for development decisions. Sacred principles that cannot change without a major version bump.

**Contains**:
- The 7 Core Architectural Principles (law, not suggestion)
- Execution framework with phase gates
- Code review checklist (paste into every PR)
- Anti-patterns to avoid
- Decision tree: "Is this aligned?"
- Measurement criteria (quarterly metrics)

**Read if**: You're writing code, reviewing PRs, or making architectural decisions

**Time**: 20 minutes (reference, not memorize)

---

### **3. DEVELOPER_QUICK_REFERENCE.md** (Print This)
**What**: One-page cheatsheet for developers. Keep at your desk.

**Contains**:
- 7 Principles in a table
- Before-you-commit checklist
- The decision tree (one page)
- Code review checklist (copy-paste)
- Common pitfalls & fixes
- Testing patterns
- PR templates

**Read if**: You're about to commit code

**Time**: 5 minutes (reference only)

---

### **4. ARCHITECTURE_IMMUTABILITY.md** (Governance)
**What**: Defines what parts of the architecture are sacred vs. flexible. Required for all architectural decisions.

**Contains**:
- The five sacred services (cannot change)
- The seven sacred principles (cannot violate)
- Data contracts (types that flow between services)
- Versioning scheme (when is it a breaking change?)
- What breaks in a major version
- How to propose architectural changes

**Read if**: You're proposing a new service, refactoring architecture, or questioning a design decision

**Time**: 25 minutes

---

### **5. EXECUTION_ROADMAP.md** (Concrete)
**What**: 18-month roadmap with concrete milestones, owners, and success metrics.

**Contains**:
- Phase 1 (Q2-Q3 2025): Production-Ready
- Phase 2 (Q4 2025–Q1 2026): Governance Platform
- Phase 3 (Q2+ 2026): Intelligence Layer
- Ownership matrix
- Quarterly milestones
- Success metrics
- Resource allocation & budget
- Risk mitigation
- Definition of done

**Read if**: You're planning the next quarter, allocating resources, or tracking progress

**Time**: 15 minutes

---

## **🚀 Quick Start: Which Document Do I Need?**

| Situation | Read This | In 5 mins |
|---|---|---|
| **"I'm new to the project"** | AGENTFABRIC_REVIEW.md | ❌ (read full) |
| **"I'm about to commit code"** | DEVELOPER_QUICK_REFERENCE.md | ✅ |
| **"I'm reviewing a PR"** | NORTH_STAR.md (checklist section) | ✅ |
| **"I want to change the architecture"** | ARCHITECTURE_IMMUTABILITY.md | ✅ (skim) |
| **"I want to add a new service"** | NORTH_STAR.md + ARCHITECTURE_IMMUTABILITY.md | ❌ (escalate to PRB) |
| **"What's our 18-month plan?"** | EXECUTION_ROADMAP.md | ✅ |
| **"What's the market opportunity?"** | AGENTFABRIC_REVIEW.md (section 1-2) | ⏱️ (15 min) |
| **"I'm a CISO evaluating this"** | AGENTFABRIC_REVIEW.md (section 5) | ⏱️ (20 min) |

---

## **🎯 The One-Liner**

> **AgentFabric is the mandatory governance layer for multi-agent enterprises: server-side semantic intelligence about cost, compliance, and quality.**

---

## **✅ The Seven Principles (Know These Cold)**

1. **Server-side semantic truth** — Never trust client claims; always recompute
2. **Multi-tenant isolation** — Every query has `WHERE tenant_id = $1`
3. **Async governance, hot observability** — Policy enforcement can be delayed; spans must appear in <100ms
4. **Immutable audit trail** — SHA-256 hash chain proves decisions were logged
5. **Server-computed framework** — Client can claim "CrewAI" but collector verifies
6. **Cost immutability** — Cost is computed once at ingestion and never changes
7. **Defense-in-depth PII scrubbing** — Multiple layers catch PII; no "safe" fields

**Violating any principle = code review rejection.**

---

## **🚨 The Decision Tree (30 Seconds)**

Before starting a feature:

1. Does it store/process agent data?
   - ✅ Recompute server-side → Has tenant filter? → Logs to audit trail?
   - ❌ Trust client claim → **REJECTED**

2. Does it process OTLP spans?
   - ✅ In collector? → <50ms? → **APPROVED**
   - ✅ In af-core? → **APPROVED**
   - ❌ Elsewhere → **Move it**

3. Does it modify historical data?
   - ❌ **REJECTED** (immutability is sacred)

4. Does it touch PII?
   - ✅ Scrubbed → **APPROVED**
   - ❌ Not scrubbed → **REJECTED**

**Full tree**: See DEVELOPER_QUICK_REFERENCE.md

---

## **📊 Current Status (as of 2025-03-13)**

| Metric | Status | Target |
|---|---|---|
| **Code Coverage** | 0% | 60%+ (Phase 1) |
| **Production Ready** | ~70% | 100% (by Q3 2025) |
| **Enterprise Customers** | 0 | 1+ (by Q3 2025) |
| **Test Suite** | None | 60% coverage |
| **Architecture Violations** | 0 | 0 (frozen) |

---

## **📅 Milestones**

| Date | Milestone | Target |
|---|---|---|
| **2025-07-15** | v1.0.0 (Production-Ready) | Test coverage 60%, OIDC login, 1 customer |
| **2026-02-15** | v1.2.0 (Governance Platform) | Distributed audit, custom policies, 3 customers |
| **2026-06-15** | v1.4.0 (Intelligence Layer) | Anomaly alerts, A/B testing, 5 customers |
| **2026-09-30** | v1.final | 10 customers, $1M ARR |

---

## **🔒 Sacred (Cannot Change Without v2.0)**

- ✅ Five-service architecture (Collector, af-core, API Gateway, Portal, SDK)
- ✅ OTLP ingestion (gRPC :4317, HTTP :4318)
- ✅ PostgreSQL as SSOT
- ✅ Kafka for async governance
- ✅ API Gateway data contracts (types returned to Portal)
- ✅ All 7 Principles

---

## **🔧 Flexible (Can Change Within a Version)**

- ✅ Service internals (refactor, optimize, change algorithms)
- ✅ Config/tuning (worker pools, batch sizes, timeouts)
- ✅ Monitoring/logging (add metrics, change alerting)
- ✅ UI/UX (redesign Portal, add pages)
- ✅ New frameworks (add Google ADK, AutoGen instrumentation)

---

## **⚠️ The Red Lines**

These will fail code review immediately:

| Red Line | Example | Fix |
|---|---|---|
| **Trust client attribute** | `framework := span.Attributes["framework"]` | `framework := detectFramework(span)` |
| **Query without tenant** | `SELECT * FROM spans WHERE ...` | Add `AND tenant_id = $1` |
| **Policy in hot path** | Enforce policy in collector | Move to Kafka → af-core |
| **Skip PII scrubbing** | `if key == "internal_id" continue` | Scrub everything |
| **Modify historical cost** | `UPDATE spans SET cost_usd = ...` | ❌ **REJECTED** |
| **Hardcode business logic** | `let input_cost = cost_usd * 0.6` | Move to config or DB |

---

## **🎓 Before Your First PR**

1. **Read** DEVELOPER_QUICK_REFERENCE.md (5 min)
2. **Review** the code review checklist (2 min)
3. **Check** your code against the decision tree (1 min)
4. **Test** locally (make sure it works)
5. **Paste** the checklist into your PR description (30 sec)
6. **Submit** for review

**Expected outcome**: Quick approval or clear feedback on what's missing

---

## **❓ FAQs**

### **Q: Can I change [architecture decision]?**
**A**: Check ARCHITECTURE_IMMUTABILITY.md. If it's in "Sacred," escalate to PRB (Principal Tech Lead + Product Manager + CISO advisor). Otherwise, go ahead.

### **Q: What's a "Principle violation"?**
**A**: Violating one of the 7 principles. Review checklist in NORTH_STAR.md. Examples: trusting client attributes, querying without tenant filter, skipping PII scrubbing.

### **Q: Can I skip tests for [small feature]?**
**A**: Not in code review. All code requires unit tests. If feature is <100 LOC, you might be able to negotiate with QA lead, but it's rare.

### **Q: What's the difference between breaking change vs. compatible change?**
**A**: Read ARCHITECTURE_IMMUTABILITY.md, "The Versioning Scheme" section. Rule of thumb: if it changes a Principle or data contract, it's breaking (v2.0).

### **Q: I found a bug in the architecture. Should I fix it?**
**A**: Document it in a GitHub issue with "Architecture Review Required" label. Don't fix it without PRB approval.

### **Q: Why is cost immutable?**
**A**: Because it's the basis for customer chargeback, policy decisions, and audit trails. If you can change it retroactively, the entire compliance chain is broken. See NORTH_STAR.md, Principle 6.

---

## **📞 Escalation**

If you're unsure whether something is allowed:

1. Check DEVELOPER_QUICK_REFERENCE.md (decision tree)
2. Check ARCHITECTURE_IMMUTABILITY.md (sacred vs. flexible)
3. Ask in #engineering Slack
4. If still unsure, escalate to Principles Review Board:
   - Principal Tech Lead
   - Product Manager
   - CISO advisor

**Expected response time**: 2 business days

---

## **📝 Feedback & Updates**

These documents are quarterly reviewed. If you find something unclear or out of date:

1. File a GitHub issue with label "North Star"
2. Link to the specific section
3. Describe what's unclear
4. Suggest a fix

**All updates require Principal Tech Lead approval.**

---

## **🔗 Related Files**

- **Codebase entry**: `README.md` (product overview)
- **API contract**: `api-gateway/internal/models/models.go` (TypeScript-compatible types)
- **Deployment**: `deploy/` directory (Helm, K8s, Docker Compose)
- **Memory**: `.claude/memory/MEMORY.md` (project context for Claude Code)

---

## **📌 Pinned Reference**

**Keep this accessible:**

```
NORTH_STAR.md          → The immutable source of truth
DEVELOPER_QUICK_REFERENCE.md → Your desk reference (print it)
ARCHITECTURE_IMMUTABILITY.md → When you want to change something
EXECUTION_ROADMAP.md         → When you're planning work
```

---

**All development after 2025-03-13 must align with these documents.**

**No exceptions. No deviations. No assumptions.**

---

**Last Updated**: 2025-03-13
**Next Review**: 2025-06-13 (quarterly)
**Owner**: Principal Tech Lead
