# How to Run the AgentFabric Agent Team

**Quick start guide for orchestrating a team of Claude Sonnet agents to build Phase 1 with 90%+ coverage**

---

## **TL;DR**

1. Read `AGENT_TEAM_ORCHESTRATION_PROMPT.md`
2. Copy the "System Prompt" section
3. Paste it into a new Claude conversation with Sonnet 4.6
4. Type: `Kick off Phase 1. Present your Sprint 1 plan to the Architect.`
5. The agent team will present a plan
6. You approve or send back with feedback
7. Team executes, reports weekly
8. Repeat for sprints 2, 3, 4

---

## **STEP-BY-STEP SETUP**

### **Step 1: Open a New Claude Conversation**

- Go to claude.com
- Start a new conversation
- Set model to **Claude Sonnet 4.6** (or latest available)

### **Step 2: Paste the System Prompt**

Copy this section from `AGENT_TEAM_ORCHESTRATION_PROMPT.md`:

```markdown
You are the Orchestrator for an AgentFabric development team consisting of:

1. Developer Agent 1 (Backend/af-core)
2. Developer Agent 2 (Collector/API Gateway)
3. DevOps Agent
4. QA Agent
5. Architect Agent (you will delegate to this agent for review)

[... entire SYSTEM PROMPT section ...]
```

Paste it as the first message or as a custom instruction if Claude Code supports it.

### **Step 3: Provide Context Files**

Upload or reference these files (Claude can read them from the repo):

```
- README_NORTH_STAR.md
- NORTH_STAR.md
- DEVELOPER_QUICK_REFERENCE.md
- ARCHITECTURE_IMMUTABILITY.md
- CRITICAL_ISSUES_BLOCKING_PRODUCTION.md
- EXECUTION_ROADMAP.md
- AGENTFABRIC_REVIEW.md
```

Tell Claude: `These are our North Star documents. Reference them for all decisions.`

### **Step 4: Kick Off Phase 1**

Send this message:

```
You are the Orchestrator for AgentFabric's Phase 1 delivery.

Your mission: Deliver v1.0.0 by 2025-07-15 with 90%+ code coverage.

Before executing anything, present your Sprint 1 plan to the Architect Agent.
The Architect will review against our North Star documents and give go-ahead or feedback.

Present the plan now (use the template from AGENT_TEAM_ORCHESTRATION_PROMPT.md).
```

### **Step 5: Architect Reviews Plan**

The agent team will present a Sprint 1 plan. Review it:

**Questions to ask:**
- Does this plan address all P0 blockers from CRITICAL_ISSUES_BLOCKING_PRODUCTION.md?
- Does the test strategy achieve 90%+ coverage?
- Are all 7 Principles addressed?
- Are the resource allocations realistic?
- What's the critical path?

**Respond with:**
- ✅ `APPROVED. Start execution immediately.`
- ⚠️ `APPROVED WITH CONDITIONS: [list conditions]`
- ❌ `REJECTED: [specific feedback]`

### **Step 6: Monitor Weekly Progress**

Every Friday (or weekly), ask:

```
Team, present your weekly status report for Week [N].

Include:
- Completed features with test coverage %
- P0/P1 issues fixed
- Blockers and mitigations
- Metrics (coverage, test count, Principles violations)
- Next week goals

Then: Architect Agent, please verify compliance with North Star documents
and sign off on progress.
```

### **Step 7: Sprint Completion Review**

At end of each sprint (4 weeks), request:

```
Team, present your Sprint [N] Completion Report.

Architect Agent, verify:
1. All 7 Principles maintained in merged code
2. Coverage ≥90% for assigned modules
3. All P0s resolved or mitigated
4. No architectural deviations
5. Ready for next sprint: YES/NO

Provide detailed sign-off.
```

---

## **CONVERSATION FLOW**

```
User: Kick off Phase 1. Present your Sprint 1 plan.
       ↓
Team: [Presents comprehensive Sprint 1 plan]
       ↓
Architect: [Validates against North Star docs]
       ↓
User: APPROVED / APPROVED WITH CONDITIONS / REJECTED
       ↓
Team: [Executes for 4 weeks]
       ↓
User: Weekly check-in
       ↓
Team: [Reports progress]
       ↓
Architect: [Verifies compliance]
       ↓
User: Continue / Fix blockers / Proceed to next sprint
       ↓
[REPEAT for Sprints 2, 3, 4]
```

---

## **KEY COMMANDS TO USE**

### **During Planning:**
- `Show me the detailed breakdown of [feature]. What tests will you write?`
- `How do you ensure 90%+ coverage for [module]?`
- `Which Principles does this implementation maintain?`
- `What are the risks and how are you mitigating them?`

### **During Execution:**
- `What's your progress on [feature]? Coverage update?`
- `Any blockers? How are you unblocking?`
- `Show me the test code for [critical function].`
- `Verify this maintains Principle [X]. Explain.`

### **For Compliance Checks:**
- `Architect Agent, verify this PR against NORTH_STAR.md Principles 1-7.`
- `Does this violate ARCHITECTURE_IMMUTABILITY.md? Explain.`
- `Can this code change be shipped in v1.x or does it require v2.0?`

### **For Metrics:**
- `Show current coverage by module. What's the gap to 90%?`
- `How many P0s remain? Timeline to fix?`
- `Are we on track for v1.0.0 on 2025-07-15?`

---

## **WHAT HAPPENS IN EACH PHASE**

### **Planning Phase (Before Execution)**

**Team presents:**
- Sprint scope (features + fixes)
- Architecture impact analysis
- Principles compliance checklist
- Test coverage plan
- Implementation breakdown (week by week)
- Risks and mitigations
- Success criteria

**You decide:**
- Is this aligned with North Star?
- Is coverage plan realistic (90%)?
- Are P0s being addressed?
- Are resources sufficient?
- Are timelines achievable?

**Architect verifies:**
- Principles 1-7 will be maintained
- No breaking changes without v2.0 discussion
- Coverage targets are clear
- Tests include critical paths
- Mitigations for risks are solid

---

### **Execution Phase (4 Weeks)**

**Team works on:**
- Implementation (tight code review loops, <4h feedback time)
- Testing as you go (not after)
- Coverage measurement (must maintain ≥90%)
- Documentation (README, CHANGELOG)

**You monitor:**
- Are blockers being resolved?
- Is coverage tracking to plan?
- Are Principles being maintained?
- Are tests passing?

**QA focuses on:**
- Coverage measurement (coverage tools + analysis)
- Test stability (no flaky tests)
- Critical path verification
- Security testing (multi-tenancy, PII)

**DevOps focuses on:**
- CI/CD pipeline health
- Staging environment readiness
- Monitoring setup
- Deployment procedures

---

### **Review Phase (After Execution)**

**Team presents:**
- What was shipped (with PR links)
- Coverage metrics (by module)
- Tests added (unit, integration, E2E)
- P0s fixed
- Deployment status
- Next sprint blockers

**Architect verifies:**
- All 7 Principles maintained
- Coverage ≥90% (actual numbers)
- All tests passing
- P0s actually fixed
- No technical debt introduced
- Architecture unchanged

**You decide:**
- Approve and move to next sprint
- Request rework on specific items
- Hold for further investigation

---

## **SUCCESS INDICATORS**

### **Good Signs (On Track)**

✅ Team presents detailed, specific plans (not hand-wavy)
✅ Coverage trending toward 90% week by week
✅ P0s being fixed in parallel with features
✅ All tests passing in CI/CD
✅ No Principle violations detected
✅ Blockers identified early and mitigated
✅ Architect sign-off obtained weekly

### **Red Flags (Off Track)**

❌ Plans are vague or missing details
❌ Coverage declining or stagnating
❌ P0s getting deferred
❌ Flaky tests being ignored
❌ Principle violations detected in code review
❌ Blockers discovered late
❌ Architect requesting rework multiple times

---

## **CONTINGENCY: IF TEAM GETS STUCK**

### **If Coverage Drops Below 90%:**

1. Request: `Show me the [module] with <90% coverage. What tests are missing?`
2. Have QA identify specific gaps
3. Add new tests with team
4. Verify coverage recovers

### **If P0 Takes Longer Than Estimated:**

1. Request: `Is the cost computation bug taking longer than 2h? Why? What's the blocker?`
2. Have team identify root cause
3. Escalate if infrastructure issue (ask DevOps)
4. Reallocate resources if needed
5. Update timeline

### **If Architect Rejects a Plan:**

1. Review the specific feedback
2. Request team revise the plan addressing concerns
3. Re-present to Architect
4. Repeat until approved
5. **Don't proceed without Architect sign-off**

### **If Tests Become Flaky:**

1. Request: `Which tests are flaky? Show me the failure pattern.`
2. Have QA debug the issue
3. Fix the test or the code (likely code issue)
4. Verify stability in CI/CD
5. Track flaky test rate (goal: 0%)

---

## **EXPECTED TIMELINE**

```
Week 1:  Sprint 1 planning + kickoff (1-2 days), P0 fixes (3-4 days)
Week 2:  P0 fixes complete, test infrastructure, 40% coverage
Week 3:  Core services implementation, 50% coverage
Week 4:  Core services finish, 60% coverage, Sprint 1 complete

Week 5:  Sprint 2 planning, collector + af-core focus
Week 6:  Heavy implementation, 70% coverage
Week 7:  Integration testing, audit trail verification
Week 8:  Framework detection + PII scrubbing verified, 80% coverage

Week 9:  Sprint 3 planning, portal implementation
Week 10: Portal pages, E2E tests, 85% coverage
Week 11: Integration tests, multi-tenant isolation verification
Week 12: Live stream + WebSocket, 88% coverage

Week 13: Sprint 4 planning, hardening + release prep
Week 14: Bug fixes, performance tuning, security review
Week 15: Final testing, documentation, monitoring setup
Week 16: v1.0.0 release, 90% coverage achieved ✅
```

---

## **TEMPLATES TO USE IN CONVERSATIONS**

### **For Plan Reviews:**
```
Architect Agent, review this plan against:
1. NORTH_STAR.md (7 Principles)
2. ARCHITECTURE_IMMUTABILITY.md (what can/can't change)
3. CRITICAL_ISSUES_BLOCKING_PRODUCTION.md (P0s)

Approve or reject with specific feedback.
```

### **For Coverage Requests:**
```
QA Agent, provide current coverage report:
- af-core: [X]%
- collector: [X]%
- api-gateway: [X]%
- portal: [X]%
- Overall: [X]%

Which modules are below 90%? What tests are needed?
```

### **For Status Updates:**
```
Team, provide this week's metrics:
- Features completed: [count]
- Test coverage: [X]%
- P0s fixed: [X]/5
- Blockers: [list or none]
- Next week priorities: [list]

Architect, verify compliance.
```

---

## **ONE FINAL NOTE**

**This is not a solo developer.**

You are running a **team of specialized agents** that work together, check each other's work, and report to an architect who ensures every decision aligns with the North Star.

This is how you:
- Achieve 90%+ coverage (not by hoping, but by structure)
- Maintain architectural integrity (every decision verified)
- Detect issues early (weekly architect review)
- Ship production-grade software (zero shortcuts)

**The agents are smart. Give them clear direction. Trust the process. Verify compliance.**

---

**Ready? Let's go.**

```
[Paste SYSTEM PROMPT]

Kick off Phase 1. Present your Sprint 1 plan to the Architect.
```

---

**Last Updated**: 2025-03-13
