# PI4.5 A1.5 Native Runtime Contract

A1.5 must not implement a second dialectical engine in AgentFabric. The ARC adapter emits typed epistemic objects and invokes the native GrapheneDB/HypoKosh runtime.

## Required native request operations

- `ingest_and_reason`: ingest observations, hypotheses, candidate goals, opposition and experiment nodes/relations; run FiberBundle expansion, convergence, opposition, admissibility, stability/Lyapunov and governed projection.
- `apply_outcome_and_reason`: ingest observed outcome plus support/contradiction relations; update the persistent ModelWorld; rerun governed reasoning.

## Required reasoning receipt

Every successful native response must include `reasoning_receipt` with all of these fields present and true:

- `graphene_executed`
- `fiber_bundle_built`
- `stability_critic_executed`
- `epistemic_admissibility_executed`
- `lyapunov_trajectory_executed`
- `convergence_executed`
- `opposition_executed`
- `no_silent_promotion`

The adapter fails closed when any required field is missing or false. There is deliberately no Python fallback for native execution.

## Epistemic mapping

- ARC observations -> `ModelWorldNodeType::Fact`, origin `Observed`.
- Competing interpretations -> `Hypothesis`, origin `Hypothetical`.
- Candidate goals -> `Decision`, origin `Hypothetical`; never facts.
- Falsification/reopening -> `Opposition`, origin `Hypothetical`.
- Chosen action -> `Experiment`, origin `Hypothetical` until executed.
- Environment result -> `Outcome`, origin `Observed`.
- Outcome support/contradiction -> observed `Supports` / `Contradicts` relations.

## Scientific guardrail

The native runtime is authoritative for preservation of alternatives, convergence, opposition, re-expansion, Lyapunov/stability assessment, escape and no-silent-promotion. The Qwen 1.5B model is only a proposal generator. A target-game-specific rule, action answer key, state signature, known solution or goal label is forbidden.
