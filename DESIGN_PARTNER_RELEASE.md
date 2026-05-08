# Govagn Design-Partner Release

Target date: 2026-05-10

## Positioning

Govagn is a governed release control plane for production AI agents. It links prompts, evals, policies, rollouts, traces, costs, and audit evidence so regulated teams can ship AI changes safely and prove what happened.

This release is intentionally narrow. It is not a generic observability launch and it is not a generic agent governance toolkit launch. The weekend wedge is a single governed release workflow for a regulated support agent.

## Demo Workflow

The design-partner demo proves one path:

1. Create a production prompt candidate for `regulated-support-agent`.
2. Attach a policy gate for PII minimization and prompt-injection handling.
3. Run `evalpack.regulated_support.release.v1` against seeded release datasets.
4. Create a 25 percent prompt-release canary with rollback criteria.
5. Generate a release evidence bundle for audit review.

The portal path is:

```text
/release-control
```

The script path is:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\run_governed_release_demo.ps1
```

## Acceptance Criteria

The weekend release is viable when a buyer can see these controls in under seven minutes:

- Prompt release candidate exists.
- Policy preview shows traffic/DLP decisions.
- Eval execution completes with a release score.
- Canary rollout rule exists.
- Evidence bundle exports release proof.

## Release Narrative

Use this line:

> Govagn gives regulated AI teams one governed path from prompt change to production release evidence.

Avoid these claims in the weekend release:

- "Open-source LLM observability platform"
- "Agent governance toolkit"
- "Full OWASP Agentic Top 10 parity"
- "Framework replacement"

## Design-Partner Buyer

Best first buyer:

- AI platform lead
- security engineering lead
- regulated product team owner
- enterprise support automation owner

Best first use case:

- one production support or document-processing workflow
- one policy pack
- one eval gate
- one rollout
- one audit/evidence bundle

## What To Show

In the portal, open Release Control and run the release gate. Then show:

- Prompts: candidate release history
- Policies: release policy rule
- Evals: release run and score
- Rollouts: canary assignment
- Audit Log: control history and evidence bundle

## What To Say

Govagn sits in the release and runtime path. Langfuse helps teams understand and improve LLM behavior. Agent security toolkits enforce action-level controls. Govagn's wedge is the release decision: approve, enforce, roll out, and prove regulated AI changes.
