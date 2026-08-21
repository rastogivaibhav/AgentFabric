# PI4.5 A1.5 — Dialectical Goal Discovery / Model-World Guardrail

## Purpose

A1.5 tests whether an ARC agent can discover a provisional objective and action policy in an unseen interactive world by coupling a compact language/pattern proposal model to the existing GrapheneDB / HypoKosh dialectical reasoning substrate.

It is **not** a stronger prompt treatment and it must not reimplement GrapheneDB reasoning in Python.

## Frozen scientific lineage

A1.3 remains the clean GrapheneDB episodic-memory baseline.
A1.4 remains the generic-prior injection treatment.
A1.5 starts from the A1.4 commit only as repository lineage; its reasoning treatment is independently identified and evidenced.

## Authoritative reasoning contract

The dialectical control sequence is:

`non-convergent expansion -> convergent compression -> opposition -> re-expansion -> synthesis -> implementation/outcome feedback -> model-world update`

A1.5 MUST preserve:

1. multiple competing hypotheses before convergence;
2. alternatives after convergence so opposition can reopen them;
3. explicit observed/discovered/inferred/reinforced/hypothetical provenance;
4. contradiction as information, not noise;
5. no-evidence abstention;
6. no silent promotion of repeated/inferred/hypothetical material to observed/discovered truth;
7. Lyapunov/stability and bounded escape receipts;
8. implementation/action -> outcome -> model-world feedback.

## Division of responsibility

### Compact LLM proposal engine

The 1.5B model may propose:

- descriptions of current observations;
- 2-5 competing hypotheses;
- 1-3 candidate goals implied by those hypotheses;
- falsification questions/opposition;
- one bounded experiment/action candidate;
- concise natural-language summaries.

It does not own epistemic truth state.

### GrapheneDB / HypoKosh

GrapheneDB owns:

- persisted model-world objects and event history;
- provenance and typed relation semantics;
- FiberBundle multiplicity;
- convergence and opposition;
- stability/Lyapunov assessment;
- escape/reopening;
- epistemic admissibility and abstention;
- status promotion/demotion;
- auditability and no-silent-promotion.

### ARC adapter

AgentFabric owns only:

- translating ARC observations/actions/outcomes to the A1.5 contract;
- invoking the compact model for bounded proposals;
- mapping validated objects into GrapheneDB;
- translating a governed experiment into an ARC action;
- collecting proof metrics.

## Model-world mapping

A1.5 semantic object -> native GrapheneDB ModelWorld node:

- observation -> `Fact`, origin `Observed`;
- hypothesis -> `Hypothesis`, origin `Hypothetical`;
- contradiction -> `Contradiction`;
- candidate goal -> `Decision` with metadata `a15_semantic_kind=candidate_goal`, origin `Hypothetical`;
- opposition -> `Opposition`;
- experiment -> `Experiment`;
- executed action -> `Implementation`;
- observed result -> `Outcome`, origin `Observed`;
- failed experiment -> `Failure`;
- synthesised provisional world model -> `Model`.

Candidate goals are never facts. A goal becomes active because it is currently useful for discriminating or explaining evidence, not because the LLM asserts it.

## Prior policy

No target-game-specific information is permitted.

The five A1.4 controller-convention memories may be retained only as weak `Analogical` priors. They cannot be treated as observed/discovered facts and cannot override current-game evidence.

The old A1.4 `goal-setting`, `interaction-design`, and `game-design` prose memories are not the A1.5 reasoning authority. The dialectical contract above is the authority.

## Diagnostic acceptance gates before full ARC

A 10-turn diagnostic is eligible to proceed only if all applicable gates pass:

- at least two genuinely distinct hypotheses are preserved before first convergence;
- at least one candidate goal is linked to a hypothesis/evidence path rather than hard-coded `win the game`;
- at least one action is recorded as an experiment testing a named hypothesis;
- every executed action has an observed outcome record;
- outcome evidence changes support/contradiction/status for at least one hypothesis when informative;
- opposition produces a falsification question and can reopen a discarded alternative;
- a repeated no-information attractor triggers Lyapunov/stability/escape handling when applicable;
- no inferred/hypothetical/reinforced object is silently promoted to observed/discovered truth;
- GrapheneDB model-world/event state validates and is persisted as evidence;
- no target-game-specific prior contamination is present.

Full `ls20/ft09/bp35` jobs MUST NOT run until diagnostics pass.
