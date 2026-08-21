#!/usr/bin/env python3
from __future__ import annotations

import json
from typing import Any


EVALUATOR_ONLY_FIELDS = {
    "game_id", "guid", "state", "levels_completed", "win_levels", "score",
    "score_delta", "level_delta", "scorecard", "official_success", "level_scores",
}

SYSTEM_RULES = """You are the proposal generator inside a governed dialectical reasoning system for an unknown interactive world.
You do NOT know the objective or action semantics. Do not invent target-specific rules.
Your perceptual evidence is limited to the observable grid, opaque legal actions, prior agent actions, and prior outcomes derived from observable grid changes.
Separate observations from hypotheses. Preserve at least two genuinely different competing hypotheses.
Classify each observation by evidence_kind: grid, transition, affordance, or memory. An opaque ACTIONn being available is only affordance evidence; it does not by itself support a causal claim about what that action will do.
Every hypothesis must explicitly link to one or more current-turn observations through its basis. Every causal/world hypothesis must include at least one grid or transition observation in that basis.
Candidate goals must emerge from hypotheses and current evidence; never use 'win the game' as a goal. Every candidate goal must explicitly link at least one current-turn hypothesis to one current-turn observation.
Provide falsification questions that could challenge the current hypothesis set, but DO NOT choose a provisional convergence, challenged hypothesis, or reopen set. The native GrapheneDB/HypoKosh controller owns those decisions.
Every action is an experiment chosen to discriminate hypotheses or reduce important uncertainty. A repeated no-effect action in an unchanged state is weak evidence against repeating the same experiment when another opaque action is available.
Treat each turn as a new epistemic revision. Use prior GOVERNED_CONTEXT as evidence/history, but create fresh turn-scoped IDs for this turn: tTURN-o..., tTURN-h..., and tTURN-g.... Do not mutate an older ModelWorld node by reusing its ID.
Follow the supplied opaque action-affordance parameter schema exactly. Do not infer semantic meaning from action names.
All semantic strings you emit must be concrete statements derived from CURRENT EVIDENCE. Contract labels, validator messages, schema words, and instructions are not world evidence and must never be copied into semantic fields.
Return one JSON object only. The external GrapheneDB runtime owns convergence, epistemic promotion, opposition targeting/reopening, Lyapunov stability and model-world state."""


def _reject_evaluator_metadata(value: Any, where: str) -> None:
    if isinstance(value, dict):
        bad = sorted(str(k) for k in value if str(k) in EVALUATOR_ONLY_FIELDS)
        if bad:
            raise ValueError(f"{where}: evaluator-only metadata is forbidden in reasoning context: {bad}")
        for key, child in value.items():
            _reject_evaluator_metadata(child, f"{where}.{key}")
    elif isinstance(value, list):
        for index, child in enumerate(value):
            _reject_evaluator_metadata(child, f"{where}[{index}]")


def _output_contract(turn: int) -> str:
    """Describe shape without a copyable worked JSON object."""
    return f"""FIELD-ONLY CONTRACT — these are key names and constraints, not values to copy.
Top-level required keys:
- "turn": integer exactly {turn}
- "observations": array of 1..8 Observation objects
- "hypotheses": array of 2..5 Hypothesis objects with genuinely different statements
- "candidate_goals": array of 1..3 CandidateGoal objects
- "opposition": Opposition object
- "experiment": Experiment object
- "residual_uncertainty": array of concrete strings

Observation object required keys:
- "id": current-turn ID beginning t{turn}-o
- "statement": concrete observable fact from CURRENT EVIDENCE
- "evidence_ref": concrete source reference using exactly one prefix: grid:, transition:, affordance:, or memory:
- "evidence_kind": exactly one of grid | transition | affordance | memory and it must match evidence_ref
Use grid for facts directly visible in the current integer grid. Use transition for before/after grid-change facts. Use affordance only for facts such as an opaque ACTIONn being available. Use memory only for prior persisted evidence.

Hypothesis object required keys:
- "id": current-turn ID beginning t{turn}-h
- "statement": concrete possible interpretation of the world
- "basis": non-empty array of HypothesisBasis objects
- "prediction": concrete observable prediction
- "status": one of proposed | active | contested
Every hypothesis basis MUST contain at least one observation whose evidence_kind is grid or transition. Affordance-only support is invalid.

HypothesisBasis object required keys:
- "observation_id": Observation ID from this turn

CandidateGoal object required keys:
- "id": current-turn ID beginning t{turn}-g
- "statement": concrete evidence-seeking/progress objective, never a generic win objective
- "basis": non-empty array of GoalBasis objects
- "status": one of proposed | active | contested

GoalBasis object required keys:
- "hypothesis_id": Hypothesis ID from this turn
- "observation_id": Observation ID from this turn

Opposition object required keys:
- "falsification_questions": array of 1..3 concrete questions grounded in the current hypothesis set
DO NOT emit challenged_hypothesis_id or reopen_hypothesis_ids.

Experiment object required keys:
- "tests_hypothesis_ids": non-empty array of Hypothesis IDs from this turn
- "information_goal": concrete uncertainty the experiment is intended to reduce
- "predicted_observation": concrete observable result that would be informative
- "action": exactly one currently available opaque ACTIONn identifier
- "action_params": object matching that action's exposed parameter schema

Forbidden controller-owned keys anywhere in the proposal:
- provisional_hypothesis_id
- provisional_goal_id
- challenged_hypothesis_id
- reopen_hypothesis_ids

Do not copy any sentence from this contract into a semantic field. JSON only."""


def build_proposal_prompt(*, turn: int, game_id: str, observation: Any,
                          available_actions: list[str], recent_outcomes: list[dict[str, Any]],
                          governed_context: dict[str, Any] | None = None,
                          max_chars: int = 6500) -> str:
    del game_id
    _reject_evaluator_metadata(observation, "observation")
    _reject_evaluator_metadata(recent_outcomes, "recent_outcomes")
    _reject_evaluator_metadata(governed_context or {}, "governed_context")
    if any(not str(a).startswith("ACTION") for a in available_actions):
        raise ValueError("available_actions must remain opaque ACTIONn identifiers")

    context = {
        "turn": int(turn),
        "current_observation": observation,
        "available_actions": available_actions,
        "recent_outcomes": recent_outcomes[-4:],
        "governed_context": governed_context or {},
    }
    contract_text = _output_contract(turn)
    prompt = SYSTEM_RULES + "\n\nCURRENT EVIDENCE:\n" + json.dumps(
        context, sort_keys=True, separators=(",", ":")
    )
    prompt += "\n\nOUTPUT CONTRACT:\n" + contract_text
    if len(prompt) > max_chars:
        compact = dict(context)
        compact["recent_outcomes"] = compact["recent_outcomes"][-1:]
        compact["governed_context"] = _compact_governed(compact["governed_context"])
        prompt = SYSTEM_RULES + "\n\nCURRENT EVIDENCE:\n" + json.dumps(
            compact, sort_keys=True, separators=(",", ":")
        )
        prompt += "\n\nOUTPUT CONTRACT:\n" + contract_text
    if len(prompt) > max_chars:
        raise ValueError(f"A1.5 proposal prompt exceeds dedicated budget: {len(prompt)} > {max_chars}")
    return prompt


def build_proposal_repair_prompt(*, original_prompt: str, invalid_output: str,
                                 validation_error: str, turn: int,
                                 available_actions: list[str], max_chars: int = 10000) -> str:
    """Build one bounded proposal repair request without adding world evidence."""
    if any(not str(a).startswith("ACTION") for a in available_actions):
        raise ValueError("proposal repair actions must remain opaque ACTIONn identifiers")
    payload = {
        "turn": int(turn),
        "validation_error": str(validation_error)[:800],
        "available_actions": [str(a) for a in available_actions],
        "rejected_output": str(invalid_output)[:4200],
    }
    rules = f"""Your previous proposal JSON for turn {turn} was rejected by the deterministic epistemic contract.
Repair that SAME proposal using ONLY the world evidence in ORIGINAL PROPOSAL REQUEST -> CURRENT EVIDENCE.
The rejection, validation error, contract text and this repair instruction are NOT world evidence. Never mention or paraphrase them in observation statements, hypothesis statements, predictions, goals, falsification questions, experiment text, or residual uncertainty.
Do not add hidden semantics, score, level, terminal state, game identity, or new observations unsupported by CURRENT EVIDENCE.
Preserve at least two genuinely different competing hypotheses and one evidence-linked candidate goal.
Every Observation MUST contain evidence_kind and a matching evidence_ref prefix. Every Hypothesis MUST contain non-empty `basis`, each item containing `observation_id`; at least one linked observation must be grid or transition evidence, not affordance-only evidence.
Every candidate goal MUST contain a non-empty `basis` array. Each goal basis item MUST contain `hypothesis_id` and `observation_id`, using IDs present in this turn's proposal.
Provide one to three evidence-grounded falsification questions, but do NOT output provisional_hypothesis_id, provisional_goal_id, challenged_hypothesis_id, or reopen_hypothesis_ids; those are native-controller-owned decisions.
Preserve turn-scoped IDs beginning with t{turn}-o, t{turn}-h, and t{turn}-g.
Use only the listed opaque actions and the supplied parameter schema.
Return one corrected JSON object only."""
    prompt = rules + "\n\nORIGINAL PROPOSAL REQUEST:\n" + original_prompt
    prompt += "\n\nREPAIR CONTEXT (NOT WORLD EVIDENCE):\n" + json.dumps(
        payload, sort_keys=True, separators=(",", ":")
    )
    if len(prompt) > max_chars:
        raise ValueError(f"A1.5 proposal repair prompt exceeds dedicated budget: {len(prompt)} > {max_chars}")
    return prompt


def _compact_governed(value: dict[str, Any]) -> dict[str, Any]:
    keys = [
        "status", "primary_hypothesis_id", "candidate_goal_ids", "reopened_hypothesis_ids",
        "challenged_claims", "native_falsification_questions", "residual_uncertainty",
        "escape_required", "lyapunov_goal_reached", "model_world_nodes",
    ]
    out = {k: value[k] for k in keys if k in value}
    if isinstance(out.get("model_world_nodes"), list):
        out["model_world_nodes"] = out["model_world_nodes"][-8:]
    return out


def parse_json_object(text: str) -> dict[str, Any]:
    raw = text.strip()
    if raw.startswith("```"):
        lines = raw.splitlines()
        if lines and lines[0].startswith("```"):
            lines = lines[1:]
        if lines and lines[-1].strip() == "```":
            lines = lines[:-1]
        raw = "\n".join(lines).strip()
    try:
        obj = json.loads(raw)
    except json.JSONDecodeError:
        start, end = raw.find("{"), raw.rfind("}")
        if start < 0 or end <= start:
            raise
        obj = json.loads(raw[start:end + 1])
    if not isinstance(obj, dict):
        raise ValueError("proposal output must be a JSON object")
    return obj
