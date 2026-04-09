# Govagn SaaS, Licensing, and Positioning Memo

## Recommendation

Build Govagn as a managed enterprise SaaS plus self-hosted enterprise product, and position it as the AI runtime control plane, not as a generic observability tool.

## Why This Is The Right Lane

The current market already has strong point solutions:

- `Langfuse` is strong in tracing, prompts, evals, and self-hosting.
- `Portkey` is strong in gateway, routing, guardrails, prompt management, and an open-source gateway.
- `Braintrust` is strong in turning traces into evals and release-quality measurement.
- `Datadog` is strong in enterprise observability by tying AI traces to broader infra and APM.

My inference from those products: Govagn should win by combining runtime control, governance, and release workflows in one product, especially for teams that care about policy, auditability, and deployment safety.

## What Customers Should Buy

Not:

- AI observability

Yes:

- `Control`: route, block, redact, budget, fail over
- `Governance`: policies, approvals, audit trails, environment controls
- `Release`: prompt versions, eval gates, staged rollouts, rollback criteria
- `Proof`: scorecards, regressions, cost attribution, compliance evidence

That gives Govagn a clearer reason to exist:

> Ship AI agents safely into production, and prove they are behaving, compliant, and cost-controlled.

## Who Should Buy First

Best initial buyer:

- AI platform leads
- security and platform engineering
- regulated internal product teams
- enterprise teams running support agents, internal copilots, document-processing agents, or workflow agents

Best first use case:

- one production agent or workflow
- one policy pack
- one eval gate
- one rollout path
- one cost dashboard leadership trusts

## Recommended License

This is product strategy, not legal advice. Counsel should review it.

Recommended split:

- `SDKs, instrumentation, sample integrations`: `Apache-2.0`
- `API schemas and client libraries`: `Apache-2.0`
- `Control plane, portal, orchestration backend`: source-available or commercial
- `Enterprise modules`: commercial

Concrete default:

- Use `Apache-2.0` for the SDK and ecosystem surface.
- Use `BSL 1.1` or a commercial source-available model for the platform.
- Offer commercial enterprise terms for hosted and private deployment.

Why this is preferable to full `MIT`:

- It encourages adoption where you need it most: the SDK.
- It protects the hosted control plane from becoming easy to clone.
- It matches your likely moat better than a fully permissive platform license.

Why this is preferable to pure `AGPL`:

- `AGPL` can help protect against uncredited hosting, but it also adds friction for enterprise adoption and partnerships.
- If your near-term goal is selling to enterprises, source-available or commercial is usually the cleaner GTM path.

## SaaS Strategy

Yes, make it SaaS. Launch in this order:

### 1. Managed Single-Tenant

- hosted by you
- dedicated database and queue per customer
- optional VPC or private networking
- easiest enterprise sale

### 2. Hybrid Enterprise

- customer-managed data plane
- Govagn-managed control plane
- good for regulated buyers

### 3. Multi-Tenant SaaS

- for smaller teams and faster self-serve motion
- only after security, RBAC, billing, and noisy-neighbor controls are solid

Why this order:

- Your strongest wedge is trust and governance.
- Shared SaaS is harder to sell first if the product promise is control and compliance.

## What Makes It Comprehensive

Do not add random modules. Build five tightly connected systems.

### 1. Policy Library

- PII
- prompt injection
- jailbreak
- unsafe output
- tenant routing
- region restrictions
- model allowlists
- budget ceilings

### 2. Prompt Library

- approved templates
- versioning
- release labels
- environment promotion
- owner and approval metadata

### 3. Eval Library

- factuality
- grounding
- refusal quality
- schema correctness
- tool-call correctness
- latency and cost quality scorecards

### 4. Rollout Library

- canaries
- prompt and model rollout recipes
- rollback thresholds
- prebuilt release policies

### 5. Evidence Library

- audit exports
- release approval packets
- policy decision history
- incident review packs

If these are linked together, Govagn becomes more than tabs in a portal. It becomes the operating system for production AI changes.

## 12-Month Product Roadmap

### Months 0-3: Make The Core Undeniable

- rock-solid traces, runs, agents, and costs
- reliable policy enforcement
- prompt versioning and release promotion
- eval runs that are easy to understand
- alerting and scorecards that do not break
- simple onboarding: instrument one agent in 10 minutes

### Months 3-6: Productize The Control Plane

- policy library
- prompt library
- eval library
- rollout workflows
- approval flows
- RBAC, SSO, org, project, and environment model

### Months 6-9: Enterprise Hardening

- audit logs
- SLA-worthy uptime
- usage-based billing
- data retention controls
- VPC and private deployment
- export to data lake and SIEM
- evidence and compliance reporting

### Months 9-12: Commercial Expansion

- managed SaaS GA
- industry packs
- design-partner case studies
- partner integrations
- opinionated dashboards for exec, platform, and risk stakeholders

## Launch Strategy

Best launch motion:

- design partners first
- not broad PLG at first
- sell a high-touch pilot around one real workflow

Pilot promise:

- instrument one production workflow
- add one policy pack
- set one eval gate
- run one controlled rollout
- produce one leadership-ready cost and compliance report

That is easier to sell than "replace all your AI tooling."

## Homepage Positioning

Hero:

> Govern every AI agent in production.

Subhead:

> Govagn gives enterprise teams one control plane for policies, prompts, evals, rollouts, traces, and cost.

Three proof points:

- Block unsafe or non-compliant behavior in real time
- Ship prompt and model changes with eval-backed rollout gates
- Trace every agent run with cost, policy, and release context attached

## How To Compete Without Spreading Too Thin

Do not try to beat:

- Langfuse on pure community and open-source momentum
- Portkey on gateway breadth alone
- Braintrust on eval-specialist depth alone
- Datadog on infra observability breadth alone

Instead, beat them on one sentence:

> Govagn is where AI changes get governed before they reach production.

That is the strongest strategic lane.

## Bottom Line

- Yes, make it SaaS.
- Yes, keep self-hosted enterprise as a premium path.
- License the SDK permissively and protect the platform.
- Win on control, governance, and release safety, not on generic observability.

## Sources

- [Langfuse pricing](https://langfuse.com/pricing)
- [Langfuse enterprise](https://langfuse.com/enterprise)
- [Langfuse self-hosting/licensing FAQ](https://github.com/langfuse/langfuse-docs/blob/main/pages/faq/all/self-hosting-langfuse.mdx)
- [Langfuse GitHub](https://github.com/langfuse/langfuse)
- [Portkey pricing](https://portkey.ai/pricing)
- [Portkey guardrails](https://portkey.ai/docs/product/guardrails)
- [Portkey gateway GitHub](https://github.com/Portkey-AI/gateway)
- [Braintrust pricing](https://www.braintrust.dev/pricing)
- [Braintrust evals](https://www.braintrust.dev/docs/guides/evals)
- [Datadog LLM Observability](https://www.datadoghq.com/product/ai/llm-observability/)
