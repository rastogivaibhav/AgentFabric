# AgentFabric — 18-Month Execution Roadmap
**Concrete milestones, owners, and success metrics**

---

## **PHASE 1: PRODUCTION-READY (Q2-Q3 2025)**
**Goal**: Ship an enterprise-grade product that CISO + Ops teams will deploy

### **Deliverables**

| Work Package | Owner | Effort | Status | Success Criteria |
|---|---|---|---|---|
| **WP-1: Test Suite** | QA Lead | 8w | 🔴 Not started | 60%+ code coverage (unit + integration + E2E) |
| **WP-2: OIDC Login** | Frontend Lead | 3w | 🔴 Not started | OAuth2 flow works, CISO can log in, tokens rotate |
| **WP-3: Audit Dashboard** | Frontend + Backend Lead | 2w | 🔴 Not started | Policy violations visible, filterable by type/date |
| **WP-4: mTLS Config** | Backend Lead | 1w | 🔴 Not started | Collector can enforce mTLS in production |
| **WP-5: Helm Charts** | DevOps Lead | 2w | 🔴 Not started | `helm install agentfabric -f values.yaml` works end-to-end |
| **WP-6: Perf Tuning** | Platform Lead | 3w | 🔴 Not started | p99 latency <100ms @ 10K spans/sec |
| **WP-7: Slack Alerting** | Backend Lead | 2w | 🔴 Not started | Policy violations → Slack webhook with trace link |

**Timeline:**
- Week 1-6 (Apr-May): WP-1, WP-2 in parallel
- Week 5-8 (May-Jun): WP-3, WP-4, WP-5 in parallel
- Week 7-10 (Jun-Jul): WP-6, WP-7
- Week 11: Hardening + customer onboarding

**Staffing**: 3 engineers + 1 DevOps + 1 QA lead

**Release date**: 2025-07-15 (v1.0.0)

### **Success Criteria**
- ✅ One enterprise customer in production
- ✅ CISO can: detect PII, enforce policies, verify audit chain
- ✅ All Principles 1-7 verified in code
- ✅ Zero architectural deviations pending
- ✅ 60%+ code coverage

---

## **PHASE 2: GOVERNANCE PLATFORM (Q4 2025–Q1 2026)**
**Goal**: Move from reactive (showing violations) to proactive (preventing them)

### **Deliverables**

| Work Package | Owner | Effort | Status | Success Criteria |
|---|---|---|---|---|
| **WP-8: Distributed Audit Chain** | Backend Lead | 4w | 🔴 Not started | Multi-instance safe, cryptographically verified |
| **WP-9: Custom Policy Uploads** | Backend Lead | 4w | 🔴 Not started | WASM module upload, hot-reload, versioning |
| **WP-10: Budget Enforcement** | Backend Lead | 3w | 🔴 Not started | Cost caps, hourly alerts, API blocking |
| **WP-11: Policy Marketplace** | Product Lead | 3w | 🔴 Not started | 10 pre-built policies, 1-click deploy |
| **WP-12: Capacity Planner** | Data Scientist | 4w | 🔴 Not started | "3x load = 2.8x cost" simulations, ±15% accuracy |
| **WP-13: Cost Optimization** | Data Scientist | 4w | 🔴 Not started | "Use Claude Haiku, save $500K/year" recommendations |

**Timeline:**
- Week 1-4: WP-8, WP-9 in parallel (hardest parts first)
- Week 3-6: WP-10, WP-11 in parallel
- Week 5-8: WP-12, WP-13 in parallel
- Week 9: Integration + testing

**Staffing**: 3 engineers + 1 data scientist + 1 product manager

**Release date**: 2026-02-15 (v1.2.0)

### **Success Criteria**
- ✅ 3+ customers on distributed audit chain
- ✅ 2+ customers using custom policies (WASM)
- ✅ Budget blocking prevents >$1M overspend across customer base
- ✅ Capacity planner accurate to ±15%
- ✅ 5+ customers using policy marketplace

---

## **PHASE 3: INTELLIGENCE LAYER (Q2+ 2026)**
**Goal**: Autonomous cost/quality optimization

### **Deliverables**

| Work Package | Owner | Effort | Status | Success Criteria |
|---|---|---|---|---|
| **WP-14: Anomaly Alerts** | Data Scientist | 2w | 🔴 Not started | Latency/cost spikes → Slack alerts with context |
| **WP-15: A/B Testing Framework** | Backend Lead | 3w | 🔴 Not started | Side-by-side agent comparison, statistical rigor |
| **WP-16: Quality Scorecards** | Backend Lead | 3w | 🔴 Not started | Integrate LLMeval/OpenCompass, correlate with cost |
| **WP-17: CI/CD Regression Gates** | DevOps Lead | 3w | 🔴 Not started | Fail PR if cost +20% or error_rate +2% |
| **WP-18: Regulatory Reporting** | Compliance Lead | 4w | 🔴 Not started | Pre-fill SOX/GDPR evidence sheets, automated |

**Timeline:**
- Week 1-3: WP-14, WP-15 in parallel
- Week 2-5: WP-16, WP-17 in parallel
- Week 4-8: WP-18

**Staffing**: 3 engineers + 1 data scientist + 1 compliance lead

**Release date**: 2026-06-15 (v1.4.0)

### **Success Criteria**
- ✅ 5+ customers using anomaly alerts
- ✅ 2+ customers using A/B testing framework
- ✅ 3+ customers using CI/CD regression gates
- ✅ 1 Fortune 500 customer uses product for regulatory audits

---

## **OWNERSHIP MATRIX**

| Role | Owner | Accountability |
|---|---|---|
| **Principal Tech Lead** | [To be assigned] | Architecture immutability, Principles enforcement, Code review gate |
| **Product Manager** | [To be assigned] | Customer feedback, Feature prioritization, Release cadence |
| **Backend Lead** | [To be assigned] | af-core quality, policy engine, audit trail |
| **Frontend Lead** | [To be assigned] | Portal UX, auth flow, performance |
| **DevOps Lead** | [To be assigned] | Helm charts, production deployments, monitoring |
| **QA Lead** | [To be assigned] | Test suite, coverage targets, regression testing |
| **Data Scientist** | [To be assigned] | ML models (anomaly, cost optimization), accuracy metrics |
| **Security Lead** | [To be assigned] | PII scrubbing, audit trail, compliance proof |

---

## **QUARTERLY MILESTONES**

### **Q2 2025: Foundation**
- [ ] Test suite at 40%+ coverage
- [ ] OIDC login working
- [ ] v1.0.0-beta released to early access customer
- [ ] Zero Principle violations in code

### **Q3 2025: Production**
- [ ] Test suite at 60%+ coverage
- [ ] v1.0.0 released
- [ ] 1 enterprise customer in production
- [ ] All WP-1 through WP-7 complete

### **Q4 2025: Governance**
- [ ] Distributed audit chain design complete
- [ ] Custom policy framework working
- [ ] Budget enforcement in production
- [ ] v1.1.0 released (OIDC polish, alerting)

### **Q1 2026: Scale**
- [ ] Distributed audit chain live in production
- [ ] 3+ customers on custom policies
- [ ] Policy marketplace with 10 pre-built rules
- [ ] v1.2.0 released

### **Q2 2026: Intelligence**
- [ ] Anomaly detection working at >85% precision
- [ ] A/B testing framework live
- [ ] Cost optimization recommendations at ±15% accuracy
- [ ] v1.4.0 released

### **Q3 2026: Enterprise**
- [ ] 10+ enterprise customers
- [ ] $1M ARR (annual recurring revenue)
- [ ] Regulatory attestation (SOX/GDPR) automated
- [ ] CI/CD integration available

---

## **SUCCESS METRICS (Track Quarterly)**

| Metric | Q2 2025 | Q3 2025 | Q4 2025 | Q1 2026 | Q2 2026 | Target |
|---|---|---|---|---|---|---|
| **Code Coverage** | 20% | 60% | 65% | 70% | 75% | 75%+ |
| **Enterprise Customers** | 0 | 1 | 2 | 3 | 5 | 10+ by Q3 2026 |
| **Annual Recurring Revenue** | $0 | $50K | $120K | $300K | $600K | $1M+ by Q3 2026 |
| **Customer NPS (CISO)** | N/A | 40 | 50 | 60 | 70 | 70+ |
| **Uptime (99%+)** | N/A | 99.0% | 99.5% | 99.9% | 99.9% | 99.9%+ |
| **Incident Response Time** | N/A | <1h | <30m | <15m | <10m | <10m |
| **Feature Adoption Rate** | N/A | 60% | 75% | 85% | 90% | 90%+ |

---

## **RESOURCE ALLOCATION**

### **Phase 1 (Q2-Q3 2025): 16 weeks**
- **Backend**: 1.5 engineers (cost computation bug, mTLS, perf tuning)
- **Frontend**: 1 engineer (OIDC, audit dashboard)
- **DevOps**: 0.5 engineer (Helm, monitoring)
- **QA**: 1 engineer (test suite)
- **Product**: 0.5 PM (customer sync)
- **Total**: 4.5 engineers

### **Phase 2 (Q4 2025–Q1 2026): 12 weeks**
- **Backend**: 2 engineers (audit chain, policies, budget)
- **Data**: 1 scientist (capacity planner, cost models)
- **DevOps**: 0.5 engineer (deployment, scaling)
- **QA**: 1 engineer (test maintenance)
- **Product**: 0.5 PM (marketplace curation)
- **Total**: 5 engineers

### **Phase 3 (Q2–Q3 2026): 12 weeks**
- **Backend**: 2 engineers (anomaly, A/B testing, CI/CD)
- **Data**: 1 scientist (ML models, accuracy)
- **DevOps**: 0.5 engineer (pipeline integration)
- **Compliance**: 0.5 engineer (regulatory reports)
- **QA**: 1 engineer (test maintenance)
- **Product**: 1 PM (customer success)
- **Total**: 6 engineers

**Total headcount by Q3 2026**: 6–8 engineers (+ PM, DevOps, Data)

---

## **BUDGET ESTIMATE**

| Phase | Salaries (6 months) | Infrastructure | Contractor Support | Total |
|---|---|---|---|---|
| **Phase 1** (Q2-Q3 2025) | $200K | $30K | $20K | **$250K** |
| **Phase 2** (Q4-Q1 2026) | $250K | $50K | $30K | **$330K** |
| **Phase 3** (Q2-Q3 2026) | $280K | $80K | $40K | **$400K** |
| **Contingency (10%)** | — | — | — | **$98K** |
| **TOTAL** | $730K | $160K | $90K | **$1,078K** (~$1.1M) |

**Cost per customer (at 10 customers by Q3 2026)**: $110K acquisition cost

---

## **RISK MITIGATION**

| Risk | Impact | Probability | Mitigation |
|---|---|---|---|
| **Test suite takes 12w instead of 8w** | Delays Phase 1 | 40% | Start tests in Week 1; hire contract QA |
| **Distributed audit chain is harder than planned** | Delays Phase 2 | 30% | Prototype in Week 1; pre-allocate 6w instead of 4w |
| **Customer churn from auth migration** | Revenue loss | 10% | Offer 6-month token extension during OIDC rollout |
| **Kubernetes deployment complexity** | Delays Phase 1 | 20% | Use Helm as gold standard; test in staging early |
| **Data science models underperform** | Delays Phase 3 | 25% | MVP with simpler heuristics; iterate to ML later |

---

## **DEPENDENCY CHAIN**

```
Phase 1 (Production)
  ├─ Test suite (blocks WP-5, WP-6)
  ├─ OIDC login (blocks customer adoption)
  └─ Audit dashboard (CISO requirement)

Phase 2 (Governance)
  ├─ Distributed audit chain (blocks scaling)
  ├─ Custom policies (customer request)
  ├─ Budget enforcement (ops requirement)
  └─ Capacity planner (prerequisite for cost optimization)

Phase 3 (Intelligence)
  ├─ Capacity planner (from Phase 2)
  ├─ A/B testing (prerequisite for quality scoring)
  └─ Anomaly detection (prerequisite for auto-remediation)

v2.0 (Future)
  ├─ All Phase 3 features mature
  ├─ Customer feedback: "We need multi-region"
  └─ Architecture redesign: Sharded PostgreSQL
```

---

## **DEFINITION OF DONE**

A phase is complete when:

1. **All WPs deployed to production** (Kubernetes)
2. **All tests passing** (automated CI/CD)
3. **Metrics are green** (coverage, uptime, customer satisfaction)
4. **Zero P0/P1 bugs** outstanding (escalation review)
5. **Customer success interview** conducted (at least 1 customer per WP)
6. **Documentation complete** (README, runbook, migration guide)
7. **On-call team trained** (incident response playbook)
8. **Release notes published** (public changelog)

---

## **COMMUNICATION CADENCE**

- **Weekly**: Engineering standup (15 min, per-WP status)
- **Bi-weekly**: Steering committee (1h, blockers + decisions)
- **Monthly**: All-hands (30 min, progress + customer testimonials)
- **Quarterly**: Board review (metrics + next quarter plan)

---

## **THE NORTH STAR (One Sentence)**

> **By Q3 2026, AgentFabric is the mandatory governance layer for enterprises running agent fleets, with 10+ customers, $1M ARR, and automated regulatory compliance proof.**

---

**This roadmap is frozen. Changes require steering committee approval.**

**Last updated**: 2025-03-13
**Next review**: 2025-04-01 (kickoff meeting)
