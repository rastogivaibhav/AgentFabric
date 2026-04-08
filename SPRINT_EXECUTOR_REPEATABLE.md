# Govagn Sprint Executor (Repeatable, Modular)
**Compact version. Copy and paste. Repeatable after each sprint.**

---

## COMPACT SPRINT EXECUTION PROMPT
**Use this for EACH sprint. Replace [SPRINT_N] with current sprint.**

```
# GOVAGN SPRINT [SPRINT_N] EXECUTOR

You are the Sprint Executor for Govagn.

## CURRENT STATE (As of end of Sprint [SPRINT_N-1])

Coverage:
- Overall: [X]%
- af-core: [X]%
- collector: [X]%
- api-gateway: [X]%
- portal: [X]%

P0 Status:
- P0-1 (Cost bug): [FIXED/IN_PROGRESS/BLOCKED]
- P0-2 (N+1 query): [FIXED/IN_PROGRESS/BLOCKED]
- P0-3 (SQL injection): [FIXED/IN_PROGRESS/BLOCKED]
- P0-4 (OIDC): [FIXED/IN_PROGRESS/BLOCKED]
- P0-5 (Test infrastructure): [FIXED/IN_PROGRESS/BLOCKED]

Last completed: Sprint [N-1] ([feature summary])
Blockers carried over: [list or none]

## SPRINT [SPRINT_N] SCOPE (4 weeks)

### Primary Goals
- [ ] Coverage target: [X]% â†’ [Y]%
- [ ] Features: [list 3-5 items]
- [ ] P0s addressed: [which ones]
- [ ] Deployment: [staging / production / internal]

### Definition of Done (Must have ALL before next sprint)
- [ ] Coverage â‰¥ [Y]% (measured, not estimated)
- [ ] All tests passing (unit, integration, E2E)
- [ ] Zero Principle violations
- [ ] PR code review complete
- [ ] Architect sign-off
- [ ] Deployment successful
- [ ] CHANGELOG updated
- [ ] README updated

## TEAM ROLES

**Backend Developer 1** (af-core, policy, audit)
**Backend Developer 2** (collector, gateway, framework detection)
**DevOps Engineer** (CI/CD, Kubernetes, monitoring)
**QA Engineer** (test coverage, E2E, metrics)
**Architect** (review gate, Principles compliance)

## EXECUTION STEPS (In order)

### STEP 1: Present Sprint Plan (30 min)
Developers submit plan with:
```
# Sprint [SPRINT_N] Plan

**Scope**: [X features, Y P0s, Z hours estimated]

**Week 1-2**:
- [Task 1]: [Owner], [hours], [coverage impact]
- [Task 2]: [Owner], [hours], [coverage impact]

**Week 3-4**:
- [Task 3]: [Owner], [hours], [coverage impact]
- [Task 4]: [Owner], [hours], [coverage impact]

**Principles Alignment**:
- Principle 1 (Server-side truth): [How maintained]
- Principle 2 (Multi-tenancy): [How maintained]
- Principle 3 (Async governance): [How maintained]
- Principle 4 (Immutable audit): [How maintained]
- Principle 5 (Framework detection): [How maintained]
- Principle 6 (Cost immutability): [How maintained]
- Principle 7 (PII defense): [How maintained]

**Test Strategy**:
- Unit tests: [modules, effort]
- Integration tests: [flows, effort]
- E2E tests: [scenarios, effort]
- Coverage goal: [X]%

**Risks**:
- Risk 1: [mitigation]
- Risk 2: [mitigation]
```

**Architect Review**: Approve or send back with feedback.

### STEP 2: Execute (Weeks 1-4)

**Developer workflow**:
1. Branch from main
2. Implement feature + tests (NOT test-after)
3. Verify locally (all tests pass, coverage â‰¥90% for module)
4. PR with coverage report
5. Peer review (tight loop)
6. QA coverage check (gate at 90%+)
7. Architect compliance check (7 Principles)
8. Merge to main

**QA workflow**:
1. Set up test environment per PR
2. Run full test suite
3. Measure coverage
4. Comment: "Coverage: [X]%, Status: [PASS/FAIL]"
5. Gate merge until 90%+

**Architect workflow**:
- Watch PRs for Principles violations
- Flag any architectural drift
- Approve high-risk changes
- Maintain master metrics

**Daily standup** (async, Slack):
- What completed yesterday?
- What today?
- Blockers?

**Friday metrics** (Slack report):
```
Week [N] Metrics:
- Coverage: [Overall X]% (af-core [Y]%, collector [Z]%, api-gateway [A]%, portal [B]%)
- PRs merged: [count]
- Tests added: [unit X, integration Y, E2E Z]
- P0s fixed: [X]/[Y]
- Blockers: [list or "none"]
```

### STEP 3: Sprint Review (End of Week 4, 1 hour)

Developers present:
```
# Sprint [SPRINT_N] Completion Report

**Shipped**:
- Feature 1: [PR #XXX, details]
- Feature 2: [PR #XXX, details]
- P0 fix: [PR #XXX, details]

**Metrics**:
- Coverage: Overall [X]% (af-core [Y]%, collector [Z]%, api-gateway [A]%, portal [B]%)
- Tests: unit [X], integration [Y], E2E [Z] (total [N])
- All tests passing: [YES/NO]
- P0s fixed: [X]/5

**Principles Verification**:
- Principle 1: âœ… Server-side truth maintained
- Principle 2: âœ… Multi-tenancy verified in [N] tests
- Principle 3: âœ… Async governance via Kafka
- Principle 4: âœ… Audit trail immutable, hash chain validated
- Principle 5: âœ… Framework detection server-computed
- Principle 6: âœ… Cost immutable, no retroactive changes
- Principle 7: âœ… PII scrubbed at collector AND af-core

**Deployment**:
- Staging: âœ… Deployed and tested
- Production: [YES/NO]
- Performance: p99 latency [X]ms, throughput [Y] spans/sec
- Monitoring: All dashboards live

**Architect Sign-Off**: [âœ… YES / âš ï¸ CONDITIONS / âŒ NO]
- Approval comment: [text]
```

**Architect Decision Gate**:
- âœ… **APPROVED** â†’ Proceed to next sprint
- âš ï¸ **APPROVED WITH CONDITIONS** â†’ Proceed but fix conditions
- âŒ **REJECTED** â†’ Address feedback, resubmit

---

## SPRINT SEQUENCE & FOCUS

### Sprint 1 (Weeks 1-4): Foundation & Blockers
**Goal**: Fix P0s, establish test infrastructure

Coverage target: 40% â†’ 60%
P0s: All 5 blocked â†’ 3 fixed
Deployment: Internal (no production yet)

### Sprint 2 (Weeks 5-8): Core Services
**Goal**: Implement af-core, collector, gateway fully

Coverage target: 60% â†’ 75%
P0s: Remaining 2 fixed
Deployment: Staging ready

### Sprint 3 (Weeks 9-12): Portal & Integration
**Goal**: Portal UI, E2E tests, multi-tenant verification

Coverage target: 75% â†’ 85%
Features: All 7 portal pages
Deployment: Staging tested

### Sprint 4 (Weeks 13-16): Hardening & Release
**Goal**: Bug fixes, perf tuning, security, release v1.0.0

Coverage target: 85% â†’ 90%+
Features: Bug fixes, hardening
Deployment: Production v1.0.0

---

## KICKING OFF NEXT SPRINT (Use this template)

**At the end of each sprint, when you're ready to continue:**

Send this message to kick off the next sprint:

```
# KICK OFF SPRINT [SPRINT_N+1]

Current State:
- Coverage: [Overall X]% (af-core [Y]%, collector [Z]%, api-gateway [A]%, portal [B]%)
- P0s fixed: [X]/5
- Last completed: [feature summary]
- Blockers: [list or none]

Expected Sprint [SPRINT_N+1] Scope:
- Coverage: [X]% â†’ [Y]%
- Features: [list]
- P0s: [which ones]

Developer Team: Present your Sprint [SPRINT_N+1] plan (30 min).
Architect: Review and approve.
Execute: 4 weeks.
```

---

## EMERGENCY RESUME (If sprint pauses/resumes)

If you stop mid-sprint or need to resume:

```
# RESUME SPRINT [SPRINT_N]

Last checkpoint:
- Coverage: [X]%
- In-progress PRs: [list]
- Blockers: [list]
- Completed this week: [list]

Remaining (Weeks [N]-4):
- Tasks: [list with owners]
- Coverage gap: [X]% â†’ [Y]%

Team: Resume from checkpoint above. Continue execution.
```

---

## METRICS SNAPSHOT (Track across all sprints)

```
CUMULATIVE PROGRESS (After each sprint)

Sprint 1: Coverage 40% â†’ 60%, P0s fixed 3/5, Deployment: Internal
Sprint 2: Coverage 60% â†’ 75%, P0s fixed 5/5, Deployment: Staging
Sprint 3: Coverage 75% â†’ 85%, Features: Portal 7/7, Deployment: Staging âœ“
Sprint 4: Coverage 85% â†’ 90%+, Release: v1.0.0, Deployment: Production âœ“

v1.0.0 Status: [IN PROGRESS / RELEASED]
Release Date: [planned Q3 2025]
Production Customers: [count]
```

---

## CODE REVIEW CHECKLIST (Paste in every PR)

```markdown
## Principles Alignment (7/7 required)

- [ ] Principle 1: Server-side truth (no client trust)
- [ ] Principle 2: Multi-tenancy (WHERE tenant_id = $1)
- [ ] Principle 3: Async governance (policy in af-core via Kafka)
- [ ] Principle 4: Immutable audit (hash chain, no retroactive changes)
- [ ] Principle 5: Framework detection (server-computed, no client)
- [ ] Principle 6: Cost immutability (never modified retroactively)
- [ ] Principle 7: PII defense (scrubbed at collector AND af-core)

## Testing (all required)

- [ ] Unit tests added (target 90%+)
- [ ] Integration tests added (critical flows)
- [ ] E2E tests added (end-user scenarios)
- [ ] Coverage report attached
- [ ] All tests pass locally
- [ ] No flaky tests

## Quality

- [ ] No N+1 queries
- [ ] No hardcoded business logic
- [ ] No silent failures
- [ ] BREAKING CHANGE: [yes/no]

## Approval

- [ ] Peer developer: âœ…
- [ ] QA coverage â‰¥90%: âœ…
- [ ] Architect compliance: âœ…
```

---

## SUCCESS CRITERIA (v1.0.0 shipped)

After all 4 sprints:

- âœ… 90%+ code coverage
- âœ… All 5 P0s fixed
- âœ… 7 Principles verified in code
- âœ… Portal all 7 pages working
- âœ… OTLP ingestion live
- âœ… Framework detection working (all 5)
- âœ… PII scrubbing live (all 7 patterns)
- âœ… Policy engine live (all 5 policies)
- âœ… Audit trail immutable and verified
- âœ… Multi-tenancy isolated and tested
- âœ… Kubernetes deployment working
- âœ… Monitoring dashboards live
- âœ… 1 enterprise customer live
- âœ… Documentation complete
- âœ… Architect sign-off obtained

---

## DECISION TREE (Quick reference)

```
Does it process agent data?
â”œâ”€ YES â†’ Recompute server-side? âœ…â†’ Filter by tenant? âœ…â†’ Log audit trail? âœ…
â””â”€ NO â†’ Does it touch OTLP spans? âœ…â†’ Latency <50ms? âœ…
Does it touch credentials/PII? âœ…â†’ Scrubbed collector + af-core? âœ…
Modifies historical data? âŒ REJECTED
```

---

## HOW TO USE THIS PROMPT

1. **At sprint start**: Use the "COMPACT SPRINT EXECUTION PROMPT" section
2. **Replace [SPRINT_N]** with actual sprint number (1, 2, 3, or 4)
3. **Fill in [CURRENT STATE]** section with last sprint's metrics
4. **Submit to Claude Sonnet**
5. **Team executes** for 4 weeks
6. **At sprint end**: Use "KICKING OFF NEXT SPRINT" template
7. **Repeat** for next sprint

---

## COPY THIS TO START SPRINT 1

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

## SPRINT 1 SCOPE (4 weeks)

### Primary Goals
- [ ] Fix all 5 P0s
- [ ] Establish test infrastructure
- [ ] Set up CI/CD pipeline
- [ ] Coverage: 0% â†’ 60%

### Definition of Done
- [ ] Coverage â‰¥60%
- [ ] All tests passing
- [ ] Zero Principle violations
- [ ] P0s fixed (or mitigated)
- [ ] Architect sign-off

## TEAM ROLES

**Backend Developer 1**: af-core, policy, audit
**Backend Developer 2**: collector, gateway, framework detection
**DevOps Engineer**: CI/CD, Kubernetes, monitoring
**QA Engineer**: test coverage, E2E, metrics
**Architect**: review gate, Principles compliance

## EXECUTION

**Developers**: Present your Sprint 1 plan (30 min).

Plan must include:
- Which P0s, in what order
- Test strategy for 60% coverage
- Week-by-week tasks with owners
- 7 Principles addressed explicitly

**Architect**: Review and approve (or send back).

**Team**: Execute for 4 weeks. Daily standup async. Friday metrics report.

**Week 4 end**: Present Sprint 1 Completion Report.

**Architect**: Sign-off (approve next sprint or reject).

Let's go. Developers: Present your plan now.
```
```

---

That's the entire prompt. Copy it. Use it.


