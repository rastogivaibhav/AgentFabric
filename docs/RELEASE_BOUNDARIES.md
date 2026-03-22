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
- broader managed-key provider scope beyond `openai` and `anthropic`

## Operational caveats

- Windows local Go test execution can still be affected by application-control policies on generated `*.test.exe` files.
- Linux CI or a less restricted environment should remain the authoritative release gate for full Go execution.
- Local bootstrap uses demo-friendly defaults and seeded pricing/policy rules; production must override secrets and auth settings.

## Release gate summary

Do not ship a release unless:
- packaging boots cleanly
- health checks pass
- portal tests and build pass
- focused Go validation passes
- docs, provider scope, and release claims match code
