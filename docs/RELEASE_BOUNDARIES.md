# Release Boundaries

AgentFabric should currently be positioned and released as:

`self-hosted AI runtime control plane with operational observability`

## In scope for the current release line

- trace and live-stream visibility
- proxy and netproxy mediation
- virtual key management
- budget enforcement
- configurable pricing
- traffic policy enforcement
- DLP-style secret and PII controls
- control-plane and policy audit trails
- multi-tenant admin/operator UI

## Explicitly not the GA story yet

- prompt playgrounds
- dataset management
- broad experiment management
- research-first evaluation workflows
- generic policy-engine replacement for OPA
- broader managed-key provider scope beyond `openai`, `anthropic`, and `google`

## Operational caveats

- Windows local Go test execution can still be affected by application-control policies on generated `*.test.exe` files.
- Linux CI or a less restricted environment should remain the authoritative release gate for full Go execution.
- Local bootstrap uses demo-friendly defaults and seeded pricing/policy rules; production must override secrets and auth settings.
- Central rollout coverage depends on onboarding:
  - SDK-instrumented applications are covered
  - OTLP-producing services are covered
  - proxied LLM traffic is covered
  - unmanaged hosts are not automatically covered

## Release gate summary

Do not ship a release unless:
- CI is green
- packaging renders cleanly
- health and readiness checks pass
- portal tests and build pass
- focused Go validation passes
- staging validation passes against a real candidate environment
- governance scenarios pass against the candidate environment
- docs, provider scope, and release claims match code
- no open P0/P1 blockers remain

## Final GA decision

Use the GA gate scripts as the objective release decision:

- [run_ga_gate.ps1](/C:/Users/vrast/Documents/Agentic%20Code/files/scripts/run_ga_gate.ps1)
- [run-ga-gate.sh](/C:/Users/vrast/Documents/Agentic%20Code/files/scripts/run-ga-gate.sh)

`ci` mode confirms merge/release evidence inside GitHub Actions.

`ga` mode is the actual release decision and requires:
- confirmed green CI
- successful packaging checks
- successful stack and proxy probes
- successful release-candidate validation with governance scenarios
- explicit blocker counts showing no open P0/P1 items

Treat any result other than `GO` as `NO-GO`.
