# Customer Value Scorecard

- Pilot name: **local-pilot**
- Team: **pilot-team**
- Environment: **staging**
- Timestamp: **2026-04-08 22:12:30 UTC**

## Value Signals

- Cost visibility: total observed spend **$0**
- Runtime activity: **0** traces, **0** LLM calls, **0** tool calls
- Guardrail/policy evidence: **verified**
- Blocked/redacted pressure: **0** blocked requests reported in overview
- Audit completeness: **0** control audit records visible
- Proxy proof: **stack-only**

## Operator Outcome Questions

- Was the team able to identify high-cost or high-latency traces without database access?
- Did policy previews and trace-linked policy events explain why requests were denied, warned, or redacted?
- Did the prompt/release linkage make it obvious which prompt version produced a given trace?
- Did audit and cost views reduce manual investigation time during pilot debugging?

## Suggested Pilot Ratings

- Cost visibility: green if spend anomalies were found from the UI alone
- Policy explainability: green if deny/redact decisions were understandable without logs
- Incident debugging speed: green if at least one trace-driven investigation was completed faster than the prior workflow
- Operator confidence: green if pilot users say they would keep this in the path for their team
