# Pilot Playbook

Use this playbook when validating Govagn with a real internal team or design partner.

## Pilot goal

Prove three outcomes in a live workflow:

- cost visibility without database access
- explainable policy and guardrail decisions
- faster incident and debugging workflows from trace-driven investigation

## Recommended pilot shape

- Duration: 1-2 weeks
- Team count: 1 team for the first pilot, 2 teams for the second
- Traffic source: one real agent or LLM-enabled service already used by the team
- Coverage mode:
  - SDK instrumentation for agent spans
  - proxy or netproxy for LLM traffic
  - admin portal for operator review

## Preconditions

- candidate stack deployed and reachable
- admin credentials available
- proxy virtual key available
- pricing rules loaded
- policy rules loaded
- prompt registry seeded for the pilot workflow
- stack health probe passes

## Recommended pilot sequence

1. Onboard the team.
2. Register one provider key and one prompt release.
3. Route at least one real workload through the proxy path.
4. Confirm traces, policy events, cost, audit, and prompt linkage appear in the portal.
5. Run one governance scenario:
   deny, warn, redact, or budget-limit.
6. Capture at least one debugging or operations story from the pilot team.
7. Complete the customer value scorecard.

## Validation commands

Windows PowerShell:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\run_local_pilot_validation.ps1 `
  -BaseUrl http://localhost:8080 `
  -CollectorUrl http://localhost:4318 `
  -AdminUser admin `
  -AdminPassword admin `
  -ProxyVirtualKey <virtual-key> `
  -RunGovernanceScenarios `
  -PilotName "pilot-one" `
  -TeamName "ai-platform"
```

Linux/macOS:

```bash
BASE_URL=http://localhost:8080 \
COLLECTOR_URL=http://localhost:4318 \
ADMIN_USER=admin \
ADMIN_PASSWORD=admin \
PROXY_VIRTUAL_KEY=<virtual-key> \
RUN_GOVERNANCE_SCENARIOS=true \
PILOT_NAME=pilot-one \
TEAM_NAME=ai-platform \
./scripts/run-local-pilot-validation.sh
```

## Evidence to capture

- one trace that clearly explains model, prompt release, policy result, and cost
- one example of blocked or redacted traffic
- one example of audit evidence after an admin mutation
- one example of faster investigation compared with the prior workflow
- pilot scorecard with operator comments

## Exit criteria

Treat the pilot as successful if:

- the team can self-serve trace investigation from the portal
- policy and guardrail outcomes are understandable
- cost visibility is actionable
- the team says they would keep the control plane in path

For market-facing release claims, prefer at least two such pilots.
