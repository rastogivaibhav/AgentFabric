# Release Boundaries

## Purpose

This document defines how AgentFabric should be positioned, described, and released at the current stage of product maturity.

Use it to keep:

- product claims aligned with code
- release messaging aligned with validation evidence
- internal and external positioning aligned with what is actually supportable

## Current Product Position

AgentFabric should currently be positioned as:

**self-hosted AI runtime governance, observability, and control plane**

That means the strongest current story is:

- governed AI runtime traffic
- trace and live operational visibility
- pricing, budget, and cost governance
- policy, guardrails, and DLP-style controls
- prompt lifecycle linkage
- release-readiness and operator evidence

## In Scope For The Current Release Line

- trace and live-stream visibility
- proxy and netproxy mediation
- virtual key management
- budget enforcement workflows
- configurable pricing
- traffic policy enforcement
- DLP-style secret and PII handling
- control-plane and policy audit trails
- multi-tenant admin and operator UI
- prompt version and release linkage
- evaluation and regression workflows
- release-proof and GA gate scripting

## Not The GA Story Yet

The following should not be presented as the main market story today:

- prompt playgrounds
- dataset management
- broad experimentation suites
- research-first evaluation lab workflows
- a generic policy-engine replacement for OPA
- broad managed-key provider claims beyond the current supported release scope

## Provider Release Boundary

The codebase includes broader provider and routing work, but current release claims should remain conservative.

Recommended release-ready provider scope:

- `openai`
- `anthropic`
- `google`

Providers present in code but not necessarily equal in release maturity should be described carefully until they are field-proven:

- `vertexai`
- `bedrock`

## Internal GA vs Market-Facing GA

### Internal GA or controlled platform rollout
For internal GA or controlled enterprise rollout, the release gate can pass without public references if:

- staging proof is clean
- governance validation is clean
- release blockers are zero at the required severity
- deployment and operations evidence is complete

### Broader market-facing release language
For broader external positioning, add:

- at least one completed pilot scorecard
- at least one real operator or platform-user story
- at least one example of measurable value in cost visibility, governance, or debugging workflow
- ideally a second pilot before broader product claims

## Operational Caveats

- Windows local Go test execution can still be affected by application-control policies on generated `*.test.exe` files.
- Linux CI or a less restricted environment should remain the authoritative release gate for full Go execution.
- Local bootstrap uses demo-friendly defaults and seeded pricing and policy rules. Production must override secrets and auth settings.
- Central rollout coverage depends on onboarding:
  - SDK-instrumented applications are covered
  - OTLP-producing services are covered
  - proxied LLM traffic is covered
  - unmanaged hosts are not automatically covered

## Release Gate Summary

Do not ship a release unless:

- CI is green
- packaging renders cleanly
- health and readiness checks pass
- portal tests and build pass
- focused Go validation passes
- staging validation passes against a real candidate environment
- governance scenarios pass against the candidate environment
- docs, provider scope, and release claims match code
- no open P0 or P1 blockers remain

## Final GA Decision

Use the GA gate scripts as the objective release decision:

- [../scripts/run_ga_gate.ps1](../scripts/run_ga_gate.ps1)
- [../scripts/run-ga-gate.sh](../scripts/run-ga-gate.sh)

`ci` mode confirms merge and release evidence in CI.

`ga` mode is the actual release decision and requires:

- confirmed green CI
- successful packaging checks
- successful stack and proxy probes
- successful release-candidate validation with governance scenarios
- explicit blocker counts showing no open P0 or P1 items

For market-facing release decisions, also use pilot proof:

- `-RequirePilotProof -PilotScorecardPath <path>` with the PowerShell GA gate
- `REQUIRE_PILOT_PROOF=true PILOT_SCORECARD_PATH=<path>` with the shell GA gate

Pilot execution and scorecard templates are here:

- [PILOT_PLAYBOOK.md](PILOT_PLAYBOOK.md)
- [CUSTOMER_VALUE_SCORECARD.md](CUSTOMER_VALUE_SCORECARD.md)

Treat any result other than `GO` as `NO-GO`.
