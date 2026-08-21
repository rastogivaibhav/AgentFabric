#!/usr/bin/env python3
from __future__ import annotations

import json
from typing import Any


EVALUATOR_ONLY_FIELDS = {
    "game_id", "guid", "state", "levels_completed", "win_levels", "score",
    "score_delta", "level_delta", "scorecard", "official_success", "level_scores",
}

SYSTEM_RULES = """You propose testable interpretations of an unknown interactive grid world.
You do NOT know the objective or action semantics. Do not invent target-specific rules.
Use only the grid, opaque legal actions, prior agent actions, and observed grid transitions.
Separate observations from hypotheses. Preserve at least two genuinely different competing hypotheses.
Observation evidence_kind is grid, transition, affordance, or memory. ACTIONn availability is only affordance evidence.
Every hypothesis needs explicit observation basis and at least one grid or transition observation. Affordance-only support is invalid.
Candidate goals must follow from evidence and hypotheses; never use 'win the game'.
Give falsification questions, but DO NOT choose convergence, challenged hypotheses, or reopen sets; the native GrapheneDB/HypoKosh controller owns those decisions.
Every action is an experiment. Avoid repeating a no-effect action in an unchanged state when another opaque action is available.
Each turn is a new epistemic revision: create fresh tTURN-o, tTURN-h and tTURN-g IDs.
Use only exposed ACTIONn identifiers/parameters. Do not infer meaning from action names.
Semantic text must come from CURRENT EVIDENCE. Contract/validator/repair text is not world evidence.
Return one JSON object only."""


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
    """Field-only shape: no semantic example for the model to copy."""
    return f"""FIELD-ONLY CONTRACT. Key names below are not semantic values.
Top level: "turn"={turn}; "observations" 1..8; "hypotheses" 2..5; "candidate_goals" 1..3; "opposition"; "experiment"; "residual_uncertainty".

Observation: "id" t{turn}-o*; "statement" observable fact; "evidence_ref" prefixed grid:/transition:/affordance:/memory:; "evidence_kind" grid|transition|affordance|memory matching that prefix.
Hypothesis: "id" t{turn}-h*; "statement" possible interpretation; "basis" non-empty HypothesisBasis[]; "prediction" observable prediction; "status" proposed|active|contested. At least one basis observation must be grid or transition evidence. Affordance-only support is invalid.
HypothesisBasis: "observation_id" referencing this turn's observation.
CandidateGoal: "id" t{turn}-g*; "statement" evidence-seeking objective; "basis" non-empty GoalBasis[]; "status" proposed|active|contested.
GoalBasis: "hypothesis_id" plus "observation_id", both from this turn.
Opposition: "falsification_questions" 1..3 concrete questions.
Experiment: "tests_hypothesis_ids" non-empty; "information_goal"; "predicted_observation"; "action" exactly one available ACTIONn; "action_params" matching its exposed schema.
Forbidden controller keys: provisional_hypothesis_id, provisional_goal_id, challenged_hypothesis_id, reopen_hypothesis_ids.
Do not copy contract sentences into semantic fields. JSON only."""


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
    ) + "\n\nOUTPUT CONTRACT:\n" + contract_text
    if len(prompt) > max_chars:
        compact = dict(context)
        compact["recent_outcomes"] = compact["recent_outcomes"][-1:]
        compact["governed_context"] = _compact_governed(compact["governed_context"])
        prompt = SYSTEM_RULES + "\n\nCURRENT EVIDENCE:\n" + json.dumps(
            compact, sort_keys=True, separators=(",", ":")
        ) + "\n\nOUTPUT CONTRACT:\n" + contract_text
    if len(prompt) > max_chars:
        raise ValueError(f"A1.5 proposal prompt exceeds dedicated budget: {len(prompt)} > {max_chars}")
    return prompt


def build_proposal_repair_prompt(*, original_prompt: str, invalid_output: str,
                                 validation_error: str, turn: int,
                                 available_actions: list[str], max_chars: int = 10000) -> str:
    """One same-model repair with no new world evidence."""
    if any(not str(a).startswith("ACTION") for a in available_actions):
        raise ValueError("proposal repair actions must remain opaque ACTIONn identifiers")
    payload = {
        "turn": int(turn),
        "validation_error": str(validation_error)[:800],
        "available_actions": [str(a) for a in available_actions],
        "rejected_output": str(invalid_output)[:4200],
    }
    rules = f"""Turn {turn} proposal failed deterministic validation. Repair that SAME proposal using ONLY ORIGINAL PROPOSAL REQUEST -> CURRENT EVIDENCE.
The rejection, validation error, contract and repair instructions are NOT world evidence. Never mention or paraphrase them in semantic fields.
Do not add score, level, terminal state, game identity, hidden semantics, or unsupported observations.
Keep >=2 genuinely different hypotheses. Every Observation needs evidence_kind and matching evidence_ref. Every Hypothesis MUST contain non-empty `basis` of observation_id items, including grid or transition evidence; affordance-only is invalid.
Every candidate goal MUST contain a non-empty `basis` array of hypothesis_id + observation_id pairs.
Give 1..3 evidence-grounded falsification questions. Do not output native-controller-owned convergence/challenge/reopen fields.
Keep t{turn}-o/t{turn}-h/t{turn}-g IDs and only listed ACTIONn values/parameters. Return corrected JSON only."""
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
