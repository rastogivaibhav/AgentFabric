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
Your perceptual evidence is limited to the observable grid, opaque legal actions, and prior outcomes derived from observable grid changes.
Separate observations from hypotheses. Preserve at least two competing hypotheses.
Candidate goals must emerge from those hypotheses and current evidence; never use 'win the game' as a goal.
Provide falsification questions that could challenge the current hypothesis set, but DO NOT choose a provisional convergence, challenged hypothesis, or reopen set. The native GrapheneDB/HypoKosh controller owns those decisions.
Every action is an experiment chosen to discriminate hypotheses or reduce important uncertainty.
Treat each turn as a new epistemic revision. Use prior GOVERNED_CONTEXT as evidence/history, but create fresh turn-scoped IDs for this turn: tTURN-o..., tTURN-h..., and tTURN-g.... Do not mutate an older ModelWorld node by reusing its ID.
Follow the supplied opaque action-affordance parameter schema exactly. Do not infer semantic meaning from it.
Return JSON only. The external GrapheneDB runtime owns convergence, epistemic promotion, opposition targeting/reopening, Lyapunov stability and model-world state."""


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

    obs_id = f"t{turn}-o1"
    h1 = f"t{turn}-h1"
    h2 = f"t{turn}-h2"
    g1 = f"t{turn}-g1"
    context = {
        "turn": int(turn),
        "current_observation": observation,
        "available_actions": available_actions,
        "recent_outcomes": recent_outcomes[-4:],
        "governed_context": governed_context or {},
    }
    schema = {
        "turn": turn,
        "observations": [{"id": obs_id, "statement": "observable fact only", "evidence_ref": f"grid:turn:{turn}"}],
        "hypotheses": [
            {"id": h1, "statement": "possible interpretation A", "support_observation_ids": [obs_id],
             "prediction": "testable grid prediction", "status": "active"},
            {"id": h2, "statement": "competing interpretation B", "support_observation_ids": [obs_id],
             "prediction": "different testable grid prediction", "status": "proposed"}
        ],
        "candidate_goals": [
            {"id": g1, "statement": "evidence-seeking/progress goal", "implied_by_hypothesis_ids": [h1, h2],
             "evidence_observation_ids": [obs_id], "status": "active"}
        ],
        "opposition": {"falsification_questions": ["..."]},
        "experiment": {"tests_hypothesis_ids": [h1, h2], "information_goal": "...", "predicted_observation": "...",
                       "action": "EXACT_AVAILABLE_ACTION", "action_params": {}},
        "residual_uncertainty": ["..."],
    }
    prompt = SYSTEM_RULES + "\n\nCURRENT EVIDENCE:\n" + json.dumps(context, sort_keys=True, separators=(",", ":"))
    prompt += "\n\nOUTPUT CONTRACT:\n" + json.dumps(schema, sort_keys=True, separators=(",", ":"))
    prompt += f"\nConstraints: IDs created this turn MUST start with t{turn}-o, t{turn}-h, or t{turn}-g as appropriate. 1-8 observations; 2-5 genuinely distinct hypotheses; 1-3 candidate goals; 1-3 falsification questions. Every candidate goal MUST include both implied_by_hypothesis_ids and evidence_observation_ids. Do NOT output provisional_hypothesis_id, provisional_goal_id, challenged_hypothesis_id, or reopen_hypothesis_ids. Use only listed actions. JSON only."
    if len(prompt) > max_chars:
        compact = dict(context)
        compact["recent_outcomes"] = compact["recent_outcomes"][-1:]
        compact["governed_context"] = _compact_governed(compact["governed_context"])
        prompt = SYSTEM_RULES + "\n\nCURRENT EVIDENCE:\n" + json.dumps(compact, sort_keys=True, separators=(",", ":"))
        prompt += "\n\nOUTPUT CONTRACT:\n" + json.dumps(schema, sort_keys=True, separators=(",", ":"))
        prompt += f"\nConstraints: use t{turn}-scoped IDs; 1-8 observations; 2-5 distinct hypotheses; 1-3 goals; each goal must include hypothesis and observation links; native controller owns convergence/opposition/reopening; use only listed actions; JSON only."
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
Repair that SAME proposal using ONLY the evidence in ORIGINAL PROPOSAL REQUEST.
Do not add hidden semantics, score, level, terminal state, game identity, or new observations.
Preserve at least two competing hypotheses and one evidence-linked candidate goal.
Every candidate goal MUST contain `implied_by_hypothesis_ids` and `evidence_observation_ids`, referencing IDs present in this turn's proposal.
Provide one to three falsification questions, but do NOT output provisional_hypothesis_id, provisional_goal_id, challenged_hypothesis_id, or reopen_hypothesis_ids; those are native-controller-owned decisions.
Preserve turn-scoped IDs beginning with t{turn}-o, t{turn}-h, and t{turn}-g.
Use only the listed opaque actions and the supplied parameter schema.
Do not treat any hypothesis or goal as truth. Return one corrected JSON object only."""
    prompt = rules + "\n\nORIGINAL PROPOSAL REQUEST:\n" + original_prompt
    prompt += "\n\nREPAIR CONTEXT:\n" + json.dumps(payload, sort_keys=True, separators=(",", ":"))
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
