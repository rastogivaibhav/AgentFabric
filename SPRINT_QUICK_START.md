# Sprint Quick Start (Copy-Paste Ready)
**Use this file to quickly kick off each sprint. No manual editing needed.**

---

## SPRINT 1 KICKOFF (Copy and paste into Claude Sonnet 4.6)

```
# GOVAGN SPRINT 1 EXECUTOR

## CURRENT STATE (Baseline)

Coverage:
- Overall: 0%
- af-core: 0%
- collector: 0%
- api-gateway: 0%
- portal: 0%

P0 Status:
- P0-1 (Cost bug): BLOCKED
- P0-2 (N+1 query): BLOCKED
- P0-3 (SQL injection): BLOCKED
- P0-4 (OIDC): BLOCKED
- P0-5 (Test infrastructure): BLOCKED

Last completed: Baseline (no previous sprints)
Blockers carried over: None

## SPRINT 1 SCOPE (4 weeks)

### Primary Goals
- [ ] Fix all 5 P0 blockers
- [ ] Establish test infrastructure
- [ ] Set up CI/CD pipeline
- [ ] Coverage: 0% â†’ 60%

### Definition of Done (Must have ALL)
- [ ] Coverage â‰¥60% (measured, not estimated)
- [ ] All tests passing (unit, integration, E2E)
- [ ] Zero Principle violations
- [ ] P0s fixed (all 5, or mitigated with docs)
- [ ] Deployment: Internal (staging ready)
- [ ] Architect sign-off obtained

## TEAM ROLES

**Backend Developer 1** (af-core, policy engine, audit trail)
**Backend Developer 2** (collector, API gateway, framework detection)
**DevOps Engineer** (CI/CD pipeline, Kubernetes, monitoring setup)
**QA Engineer** (test coverage tracking, E2E tests, metrics)
**Architect** (Principles compliance, code review gate, sign-off)

## EXECUTION (Follow SPRINT_EXECUTOR_REPEATABLE.md workflow)

### STEP 1: Plan (30 min)

Developers present Sprint 1 plan with:
- Which P0s will be fixed (all 5) and in what order
- Test strategy for 60% coverage
- Week-by-week task breakdown with owners
- How all 7 Principles are maintained
- Success criteria

### STEP 2: Review (1 hour)

Architect reviews plan:
- âœ… Approves â†’ Proceed to execution
- âš ï¸ Approves with conditions â†’ Fix before execution
- âŒ Rejects â†’ Return with feedback, iterate

### STEP 3: Execute (4 weeks)

- Daily async standup (Slack)
- Friday metrics reports (coverage, P0s, blockers)
- PRs: peer review â†’ QA gate (90%+) â†’ Architect check â†’ merge
- All code merges to main
- Staging deployment by week 4

### STEP 4: Sprint Review (End of week 4)

Team presents completion report:
- Metrics: coverage, P0s fixed, tests added
- Features shipped
- Architect sign-off

**Architect decision**:
- âœ… APPROVED â†’ Proceed to Sprint 2
- âš ï¸ CONDITIONS â†’ Fix and resubmit
- âŒ REJECTED â†’ Continue Sprint 1, resubmit

---

**NOW**: Developers present your Sprint 1 plan.
```

---

## SPRINT 2 KICKOFF (Copy and paste into Claude Sonnet 4.6)

**Use this after Sprint 1 is complete and metrics are available.**

```
# GOVAGN SPRINT 2 EXECUTOR

## CURRENT STATE (From Sprint 1 Completion)

Coverage:
- Overall: 60%
- af-core: 45%
- collector: 55%
- api-gateway: 40%
- portal: 30%

P0 Status:
- P0-1 (Cost bug): FIXED
- P0-2 (N+1 query): FIXED
- P0-3 (SQL injection): FIXED
- P0-4 (OIDC): IN_PROGRESS (backend done, UI in Sprint 3)
- P0-5 (Test infrastructure): FIXED

Last completed: Sprint 1 (P0 blockers, test infra, CI/CD)
Blockers carried over: OIDC UI (deferred to Sprint 3)

## SPRINT 2 SCOPE (4 weeks)

### Primary Goals
- [ ] Implement af-core fully (policy engine, audit, Kafka consumer)
- [ ] Implement collector fully (OTLP, framework detection, PII scrubbing)
- [ ] Implement API gateway fully (20 endpoints, WebSocket)
- [ ] Coverage: 60% â†’ 75%

### Definition of Done (Must have ALL)
- [ ] Coverage â‰¥75% (measured, not estimated)
- [ ] All tests passing (unit, integration, E2E)
- [ ] Zero Principle violations
- [ ] af-core: 90%+ coverage
- [ ] collector: 90%+ coverage
- [ ] api-gateway: 90%+ coverage
- [ ] Framework detection: all 5 frameworks verified (CrewAI, LangGraph, OpenAI, Anthropic, Google)
- [ ] PII scrubbing: all 7 patterns tested (SSN, card, email, credentials, UK phone, UK postcode, names)
- [ ] Deployment: Staging ready, full integration tested
- [ ] Architect sign-off obtained

## TEAM ROLES

Same as Sprint 1:
- Backend Developer 1 (af-core)
- Backend Developer 2 (collector, gateway)
- DevOps Engineer (CI/CD, staging)
- QA Engineer (coverage, integration tests)
- Architect (Principles, sign-off)

## EXECUTION (Follow SPRINT_EXECUTOR_REPEATABLE.md workflow)

### STEP 1: Plan (30 min)

Developers present Sprint 2 plan with:
- af-core implementation tasks (policy, audit, Kafka)
- Collector implementation tasks (OTLP, frameworks, PII)
- Gateway implementation tasks (endpoints, WebSocket)
- Test strategy for 75% coverage
- Week-by-week breakdown with owners and hours

### STEP 2: Review (1 hour)

Architect reviews against 7 Principles:
- âœ… Approves â†’ Execution
- âš ï¸ Conditions â†’ Fix first
- âŒ Rejects â†’ Iterate

### STEP 3: Execute (4 weeks)

Same process as Sprint 1:
- Daily standup
- Friday metrics
- PR workflow: peer â†’ QA â†’ Architect â†’ merge
- Staging deployment by week 4

### STEP 4: Sprint Review (End of week 4)

Completion report must show:
- Coverage: 75%+ (af-core 90%+, collector 90%+, gateway 90%+)
- All 5 frameworks verified
- All 7 PII patterns tested
- Full integration tested in staging

---

**NOW**: Developers present your Sprint 2 plan.
```

---

## SPRINT 3 KICKOFF (Copy and paste into Claude Sonnet 4.6)

**Use this after Sprint 2 is complete.**

```
# GOVAGN SPRINT 3 EXECUTOR

## CURRENT STATE (From Sprint 2 Completion)

Coverage:
- Overall: 75%
- af-core: 92%
- collector: 91%
- api-gateway: 89%
- portal: 35%

P0 Status:
- P0-1 (Cost bug): FIXED
- P0-2 (N+1 query): FIXED
- P0-3 (SQL injection): FIXED
- P0-4 (OIDC): IN_PROGRESS (backend done, UI this sprint)
- P0-5 (Test infrastructure): FIXED

Last completed: Sprint 2 (af-core, collector, gateway fully implemented)
Blockers carried over: OIDC UI implementation

## SPRINT 3 SCOPE (4 weeks)

### Primary Goals
- [ ] Implement all 7 portal pages (Dashboard, Traces, TraceDetail, Live, Agents, Cost, Environments)
- [ ] Implement OIDC login UI
- [ ] E2E tests (agent â†’ collector â†’ portal)
- [ ] Multi-tenancy verification and security tests
- [ ] Coverage: 75% â†’ 85%

### Definition of Done (Must have ALL)
- [ ] Coverage â‰¥85% (measured, not estimated)
- [ ] Portal pages: 7/7 complete and functional
- [ ] Dashboard: KPIs, framework breakdown, token usage
- [ ] Traces: pagination, filtering, clickable rows
- [ ] TraceDetail: waterfall, spans table, topology graph
- [ ] LiveStream: Wireshark-style real-time table, pause/resume, CSV export
- [ ] Agents: framework cards, recent activity
- [ ] Cost: aggregate stats, framework breakdown, share progress bars
- [ ] Environments: collector list, dynamic environment list
- [ ] OIDC: login page, token generation, logout
- [ ] E2E tests: agent â†’ collector â†’ portal flow verified
- [ ] Multi-tenancy: isolation verified in 5+ security tests
- [ ] WebSocket: live stream working, reconnection tested
- [ ] Deployment: Staging fully tested, production ready
- [ ] Architect sign-off obtained

## TEAM ROLES

Same as Sprints 1-2:
- Backend Developer 1 (OIDC implementation)
- Backend Developer 2 (portal backend endpoints if needed)
- DevOps Engineer (staging, production-readiness)
- QA Engineer (E2E tests, multi-tenancy tests, UI testing)
- Architect (Principles, sign-off)

## EXECUTION (Follow SPRINT_EXECUTOR_REPEATABLE.md workflow)

### STEP 1: Plan (30 min)

Developers present Sprint 3 plan with:
- Portal page implementation tasks (7 pages)
- OIDC login UI
- E2E test strategy
- Multi-tenancy security tests
- Week-by-week breakdown

### STEP 2: Review (1 hour)

Architect reviews:
- âœ… Approves â†’ Execution
- âš ï¸ Conditions â†’ Fix first
- âŒ Rejects â†’ Iterate

### STEP 3: Execute (4 weeks)

Same process as Sprints 1-2:
- Daily standup
- Friday metrics
- PR workflow
- Staging fully operational by week 4

### STEP 4: Sprint Review (End of week 4)

Completion report must show:
- Coverage: 85%+
- Portal pages: 7/7 working
- OIDC: login working
- E2E: critical flows verified
- Multi-tenancy: security tests passing
- Staging: fully tested and ready

---

**NOW**: Developers present your Sprint 3 plan.
```

---

## SPRINT 4 KICKOFF (Copy and paste into Claude Sonnet 4.6)

**Use this after Sprint 3 is complete.**

```
# GOVAGN SPRINT 4 EXECUTOR

## CURRENT STATE (From Sprint 3 Completion)

Coverage:
- Overall: 85%
- af-core: 92%
- collector: 91%
- api-gateway: 89%
- portal: 82%

P0 Status:
- P0-1 (Cost bug): FIXED
- P0-2 (N+1 query): FIXED
- P0-3 (SQL injection): FIXED
- P0-4 (OIDC): FIXED
- P0-5 (Test infrastructure): FIXED

Last completed: Sprint 3 (all 7 portal pages, OIDC, E2E tests)
Blockers carried over: None

## SPRINT 4 SCOPE (4 weeks) â€” FINAL SPRINT TO v1.0.0

### Primary Goals
- [ ] Bug fixes from Sprints 1-3
- [ ] Performance tuning (p99 latency <100ms)
- [ ] Security hardening (mTLS, JWT verification)
- [ ] Complete Helm charts for production
- [ ] Complete monitoring dashboards (Prometheus + Grafana)
- [ ] Complete documentation and runbooks
- [ ] Release v1.0.0 to production
- [ ] Coverage: 85% â†’ 90%+

### Definition of Done (Must have ALL)
- [ ] Coverage â‰¥90% (measured, not estimated)
- [ ] All tests passing
- [ ] Zero Principle violations
- [ ] Zero known P1/P2 bugs (or documented with roadmap)
- [ ] Performance: p99 latency <100ms for all critical paths
- [ ] Performance: throughput â‰¥10K spans/sec
- [ ] Security: mTLS enabled and tested
- [ ] Security: JWT signature verification working
- [ ] Security: Multi-tenancy isolation verified in production config
- [ ] Helm charts: Production-ready deployment
- [ ] Kubernetes manifests: All services deployable
- [ ] Monitoring: Prometheus scraping collector + gateway
- [ ] Monitoring: Grafana dashboards live (4 dashboards minimum)
- [ ] Documentation: README updated in each service
- [ ] Documentation: CHANGELOG complete
- [ ] Documentation: Architecture docs updated
- [ ] Documentation: Deployment runbooks documented
- [ ] Documentation: Security best practices documented
- [ ] Deployment: v1.0.0 released to production
- [ ] Deployment: 1 enterprise customer live
- [ ] Deployment: Incident response plan documented
- [ ] Architect sign-off obtained

## TEAM ROLES

Same as Sprints 1-3:
- Backend Developer 1 (security hardening, bug fixes)
- Backend Developer 2 (performance tuning, bug fixes)
- DevOps Engineer (Helm, Kubernetes, Prometheus, Grafana)
- QA Engineer (performance tests, load tests, final verification)
- Architect (final sign-off, Principles verification)

## EXECUTION (Follow SPRINT_EXECUTOR_REPEATABLE.md workflow)

### STEP 1: Plan (30 min)

Developers present Sprint 4 plan with:
- Bug fixes (from bug backlog)
- Performance tuning tasks (identify hotspots, optimize)
- Security hardening (mTLS, JWT, RLS verification)
- Documentation completion
- Release checklist
- Week-by-week breakdown

### STEP 2: Review (1 hour)

Architect reviews:
- âœ… Approves â†’ Execution
- âš ï¸ Conditions â†’ Fix first
- âŒ Rejects â†’ Iterate

### STEP 3: Execute (4 weeks)

Same process as Sprints 1-3:
- Daily standup
- Friday metrics
- PR workflow
- Production deployment by week 4

### STEP 4: Sprint Review (End of week 4)

Completion report must show:
- Coverage: 90%+
- All bugs fixed
- Performance: p99 <100ms, throughput â‰¥10K/sec
- Security: mTLS, JWT, RLS verified
- Helm charts: production-ready
- Monitoring: dashboards live
- Documentation: complete
- v1.0.0: released to production
- Customer: 1 live in production

---

**FINAL GATE: v1.0.0 Release Approval**

If ALL criteria met:
- âœ… APPROVED â†’ v1.0.0 released to production
- âš ï¸ CONDITIONS â†’ Fix and rerelease
- âŒ REJECTED â†’ Hotfix and resubmit

---

**NOW**: Developers present your Sprint 4 plan.
```

---

## HOW TO USE THESE QUICK STARTS

### For Sprint 1 (Right Now)
1. Copy the "SPRINT 1 KICKOFF" section above
2. Go to claude.com, start new conversation
3. Set model to Claude Sonnet 4.6
4. Paste the prompt
5. Send it
6. Team executes

### For Sprint 2 (After Sprint 1 ends)
1. Copy the "SPRINT 2 KICKOFF" section above
2. Go to claude.com, start new conversation
3. Set model to Claude Sonnet 4.6
4. Paste the prompt
5. Send it
6. Team executes

### For Sprint 3 (After Sprint 2 ends)
1. Copy the "SPRINT 3 KICKOFF" section above
2. Paste and execute

### For Sprint 4 (After Sprint 3 ends)
1. Copy the "SPRINT 4 KICKOFF" section above
2. Paste and execute

---

## THAT'S IT

No more "what's the prompt for next sprint". Just copy and paste.

Each sprint prompt is self-contained, knows the current state, and guides execution.

**Total time per sprint kickoff: 5 minutes**

Let's ship v1.0.0.


