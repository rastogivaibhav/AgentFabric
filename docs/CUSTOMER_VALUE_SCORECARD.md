# Customer Value Scorecard

Use this scorecard after every pilot.

## Pilot metadata

- Customer or team:
- Environment:
- Pilot window:
- Primary workload:
- Coverage mode:
  - SDK
  - proxy
  - netproxy

## Outcome areas

Score each area as `green`, `yellow`, or `red`.

### 1. Cost visibility

- Did the team identify spend by model, trace, or workflow step from the UI?
- Were high-cost traces discoverable without database access?
- Did pricing previews help explain or predict spend?

### 2. Policy and guardrail explainability

- Could operators explain why traffic was allowed, denied, warned, or redacted?
- Were policy events and trace details enough for normal triage?
- Did simulation or preview reduce policy rollout anxiety?

### 3. Debugging and incident response

- Could the team investigate a slow, failed, or blocked request from the trace UI?
- Did timeline, policy, cost, and prompt-release linkage reduce investigation time?
- Was the trace compare flow useful for understanding regressions?

### 4. Operational trust

- Did audit logs cover the expected control-plane changes?
- Did prompt release pointers make deployed configuration understandable?
- Did the team trust the platform enough to keep it in the request path?

## Quantitative notes

- Total pilot traces reviewed:
- Total LLM calls observed:
- Total blocked or redacted events:
- Total cost observed:
- Number of incidents/debugging sessions where Govagn was used:
- Average time to find root cause before pilot:
- Average time to find root cause during pilot:

## Qualitative evidence

- Best debugging story:
- Best governance story:
- Best cost-control story:
- Strongest user quote:
- Biggest objection or friction:

## Recommendation

- Keep in pilot
- Expand to another team
- Fix major issues before expansion

## Minimum evidence for market-facing proof

Before using pilot results in external positioning, try to have:

- at least one completed scorecard per pilot
- at least one operator quote
- at least one quantified improvement in debugging or governance workflow
- at least one example trace that demonstrates policy + cost + prompt linkage clearly
