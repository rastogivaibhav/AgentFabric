# Sprint Execution Flow (Repeatable, Step-by-Step)

## THE CYCLE (Repeat for each of 4 sprints)

```
SPRINT [N] START
    ↓
[Copy SPRINT_EXECUTOR_REPEATABLE.md, replace [SPRINT_N] with actual number]
    ↓
Paste into Claude Sonnet
    ↓
Teams present plan (30 min)
    ↓
Architect reviews, approves or rejects (1 hour)
    ↓
[IF APPROVED] Execute for 4 weeks
    ├─ Daily async standup (Slack)
    ├─ Friday metrics report
    ├─ PRs: code review → QA → Architect → merge
    └─ Weekly coverage snapshot
    ↓
SPRINT [N] END (Week 4, Friday)
    ↓
Team presents Completion Report
    ↓
Architect reviews, approves or rejects (1 hour)
    ↓
[IF APPROVED] Record metrics, proceed to SPRINT [N+1]
    ↓
[IF REJECTED] Revise, resubmit, continue sprint [N]
    ↓
KICK OFF SPRINT [N+1] using template below
```

---

## EXACT STEPS FOR EACH SPRINT

### **SPRINT 1 KICKOFF**

**You do this now:**

1. Open `SPRINT_EXECUTOR_REPEATABLE.md`
2. Find the section "COPY THIS TO START SPRINT 1"
3. Copy the entire Sprint 1 prompt
4. Go to claude.com, start new conversation
5. Set model to Claude Sonnet 4.6
6. Paste the prompt
7. Send it
8. Team presents plan → Architect reviews → Execute

---

### **SPRINT 2 KICKOFF** (After Sprint 1 ends)

**You do this when Sprint 1 is done:**

1. Get the metrics from Sprint 1 completion report:
   - Coverage: [was 40-60%, should be ~60%]
   - P0s: [how many fixed]
   - What was shipped: [list features]

2. Copy `SPRINT_EXECUTOR_REPEATABLE.md`

3. Replace these placeholders:
   ```
   [SPRINT_N] → 2
   [SPRINT_N-1] → 1
   [Current State / Coverage] → fill from Sprint 1 report
   [Current State / P0 Status] → fill from Sprint 1 report
   [Last completed] → fill from Sprint 1 report
   [SPRINT_N] Scope → fill in Sprint 2 goals
   ```

4. The "SPRINT 2 SCOPE" section should be:
   ```
   ### Primary Goals
   - [ ] Coverage: 60% → 75%
   - [ ] Implement af-core, collector, gateway fully
   - [ ] Framework detection (all 5 frameworks)
   - [ ] Fix remaining P0s
   - [ ] Deployment: Staging ready
   ```

5. Paste into new Claude Sonnet conversation
6. Send: "KICK OFF SPRINT 2" message (see template below)
7. Team presents plan → Architect reviews → Execute

---

### **SPRINT 3 KICKOFF** (After Sprint 2 ends)

Replace:
```
[SPRINT_N] → 3
[SPRINT_N-1] → 2
Coverage → 75% (from Sprint 2 report)
P0 Status → all 5 fixed (expected)
Features → Portal UI, E2E tests, multi-tenant verification
Scope → 75% → 85%
```

---

### **SPRINT 4 KICKOFF** (After Sprint 3 ends)

Replace:
```
[SPRINT_N] → 4
[SPRINT_N-1] → 3
Coverage → 85% (from Sprint 3 report)
P0 Status → all 5 fixed (expected)
Features → Bug fixes, perf tuning, hardening, release v1.0.0
Scope → 85% → 90%+
Deployment → Production v1.0.0
```

---

## TEMPLATE: "KICK OFF SPRINT [N+1]" MESSAGE

After sprint [N] completes and architect approves, send this to start sprint [N+1]:

```
# KICK OFF SPRINT [SPRINT_N+1]

## Metrics from Sprint [SPRINT_N]

Coverage: [INSERT from completion report]
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

Features shipped last sprint:
- [Feature 1]
- [Feature 2]
- [Feature 3]

Blockers carried over (if any):
- [Blocker 1] → mitigation [plan]

---

## Sprint [SPRINT_N+1] Scope

Target: Coverage [X]% → [Y]%
Features:
- [List 3-5 features for this sprint]

P0s:
- [Which ones remaining]

Expected deployment: [internal/staging/production]

---

Developer Team: Present your Sprint [SPRINT_N+1] plan.

Architect: Review and approve.

Execute: 4 weeks.
```

---

## WHAT EACH SPRINT FOCUSES ON

### Sprint 1 (Weeks 1-4): Foundation
```
Goals:
- Fix all 5 P0 blockers
- Set up test infrastructure
- Establish CI/CD pipeline
- Coverage: 0% → 60%

Key deliverables:
- P0-1: Cost bug fixed ✅
- P0-2: N+1 query fixed ✅
- P0-3: SQL injection fixed ✅
- P0-4: OIDC backend started
- P0-5: Test infra 40%+ coverage

Deployment: Internal (no customer access yet)
```

### Sprint 2 (Weeks 5-8): Core Services
```
Goals:
- Implement af-core fully (policy engine, audit, Kafka consumer)
- Implement collector fully (OTLP, framework detection, PII scrubbing)
- Implement API gateway fully (20 endpoints, WebSocket)
- Coverage: 60% → 75%

Key deliverables:
- af-core: 90%+ coverage
- collector: 90%+ coverage
- api-gateway: 90%+ coverage
- Framework detection: all 5 frameworks verified
- PII scrubbing: all 7 patterns tested

Deployment: Staging ready, all services tested
```

### Sprint 3 (Weeks 9-12): Portal & Integration
```
Goals:
- Implement all 7 portal pages
- E2E tests (agent → collector → portal)
- Multi-tenancy verification
- Coverage: 75% → 85%

Key deliverables:
- Portal pages: 7/7 complete
- E2E tests: All critical flows
- Multi-tenant isolation: Security tests
- Live stream: WebSocket working
- Coverage: 85%

Deployment: Staging fully tested, production ready
```

### Sprint 4 (Weeks 13-16): Release
```
Goals:
- Bug fixes from Sprints 1-3
- Performance tuning
- Security hardening
- Documentation complete
- Release v1.0.0
- Coverage: 85% → 90%+

Key deliverables:
- v1.0.0 released to production
- 1 enterprise customer live
- Monitoring dashboards live
- Helm charts production-ready
- Documentation complete

Deployment: Production, customer live
```

---

## METRICS TO TRACK WEEKLY

**Every Friday, report:**

```
Week [N]:
Coverage:
- Overall: [X]%
- af-core: [X]%
- collector: [X]%
- api-gateway: [X]%
- portal: [X]%

PRs merged: [count]
Tests added: [unit: X, integration: Y, E2E: Z]
P0s fixed: [X]/5
Blockers: [list or "none"]

On track for Sprint [N] goal? [YES/NO]
Risks? [list or "none"]
```

---

## IF SPRINT PAUSES (Emergency Resume)

If you need to stop mid-sprint and resume later:

1. **Document the checkpoint:**
   ```
   Paused at: Week [N] of Sprint [M]
   Coverage: [X]%
   Completed: [list PRs, features]
   In-progress: [list PRs, owners]
   Blockers: [list]
   Next steps: [list tasks to resume]
   ```

2. **When resuming, send this:**
   ```
   # RESUME SPRINT [M]

   Checkpoint: Week [N] of 4, Coverage [X]%
   Completed: [list]
   In-progress: [list with owners]
   Remaining: [list]

   Team: Resume from checkpoint above.
   Continue execution, targeting [Y]% coverage by week 4.
   ```

---

## FINAL CHECKLIST

Before each sprint starts:

- [ ] Previous sprint metrics documented
- [ ] Current state filled in (coverage, P0s, features)
- [ ] Sprint goals clear and achievable
- [ ] Team assignments clear
- [ ] Success criteria (Definition of Done) written
- [ ] Architect ready to review
- [ ] Prompt pasted into Claude Sonnet

Before each sprint ends:

- [ ] Completion report drafted by team
- [ ] Coverage measured (not estimated)
- [ ] All tests passing
- [ ] Architect reviewed it
- [ ] Metrics recorded for next sprint

---

## TL;DR FLOW

```
Sprint 1: Copy prompt, fill blanks, execute (4 weeks)
          ↓ (Friday week 4)
          Present completion report, get architect approval
          ↓
Sprint 2: Update metrics, fill blanks, execute (4 weeks)
          ↓ (Friday week 8)
          Present completion report, get architect approval
          ↓
Sprint 3: Update metrics, fill blanks, execute (4 weeks)
          ↓ (Friday week 12)
          Present completion report, get architect approval
          ↓
Sprint 4: Update metrics, fill blanks, execute (4 weeks)
          ↓ (Friday week 16)
          Present completion report
          ↓
v1.0.0 SHIPPED ✅
```

---

## NEXT ACTION

1. Open `SPRINT_EXECUTOR_REPEATABLE.md`
2. Find "COPY THIS TO START SPRINT 1"
3. Copy that prompt
4. Paste into Claude Sonnet 4.6
5. Send it
6. Team presents plan
7. Architect approves
8. Execute

**That's it. You're live.**

