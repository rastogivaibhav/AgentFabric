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
Choose one provisional hypothesis and goal only for the next experiment, not as truth.
Oppose the provisional hypothesis with a falsification question and preserve at least one alternative when possible.
Every action is an experiment chosen to discriminate hypotheses or reduce important uncertainty.
If GOVERNED_CONTEXT contains an existing ModelWorld hypothesis or goal that you are continuing, reuse its external_id; create a new id only for a genuinely new interpretation.
Follow the supplied opaque action-affordance parameter schema exactly. Do not infer semantic meaning from it.
Return JSON only. The external GrapheneDB runtime owns convergence, epistemic promotion, opposition/reopening, Lyapunov stability and model-world state."""


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

    context = {
        "turn": int(turn),
        "current_observation": observation,
        "available_actions": available_actions,
        "recent_outcomes": recent_outcomes[-4:],
        "governed_context": governed_context or {},
    }
    schema = {
        "turn": turn,
        "observations": [{"id": "o...", "statement": "observable fact only", "evidence_ref": "grid/transition ref"}],
        "hypotheses": [
            {"id": "h...", "statement": "possible interpretation", "support_observation_ids": ["o..."],
             "prediction": "testable grid prediction", "status": "proposed|active|contested"}
        ],
        "candidate_goals": [
            {"id": "g...", "statement": "evidence-seeking/progress goal", "implied_by_hypothesis_ids": ["h..."],
             "evidence_observation_ids": ["o..."], "status": "proposed|active|contested"}
        ],
        "provisional_hypothesis_id": "h...",
        "provisional_goal_id": "g...",
        "opposition": {"challenged_hypothesis_id": "h...", "falsification_questions": ["..."], "reopen_hypothesis_ids": ["h..."]},
        "experiment": {"tests_hypothesis_ids": ["h..."], "information_goal": "...", "predicted_observation": "...",
                       "action": "EXACT_AVAILABLE_ACTION", "action_params": {}},
        "residual_uncertainty": ["..."],
    }
    prompt = SYSTEM_RULES + "\n\nCURRENT EVIDENCE:\n" + json.dumps(context, sort_keys=True, separators=(",", ":"))
    prompt += "\n\nOUTPUT CONTRACT:\n" + json.dumps(schema, sort_keys=True, separators=(",", ":"))
    prompt += "\nConstraints: 1-8 observations; 2-5 genuinely distinct hypotheses; 1-3 candidate goals; 1-3 falsification questions. Use only listed actions. JSON only."
    if len(prompt) > max_chars:
        compact = dict(context)
        compact["recent_outcomes"] = compact["recent_outcomes"][-1:]
        compact["governed_context"] = _compact_governed(compact["governed_context"])
        prompt = SYSTEM_RULES + "\n\nCURRENT EVIDENCE:\n" + json.dumps(compact, sort_keys=True, separators=(",", ":"))
        prompt += "\n\nOUTPUT CONTRACT:\n" + json.dumps(schema, sort_keys=True, separators=(",", ":"))
        prompt += "\nConstraints: 1-8 observations; 2-5 distinct hypotheses; 1-3 goals; use only listed actions; JSON only."
    if len(prompt) > max_chars:
        raise ValueError(f"A1.5 proposal prompt exceeds dedicated budget: {len(prompt)} > {max_chars}")
    return prompt


def _compact_governed(value: dict[str, Any]) -> dict[str, Any]:
    keys = ["status", "primary_hypothesis_id", "candidate_goal_ids", "reopen_hypothesis_ids",
            "residual_uncertainty", "escape_required", "lyapunov_goal_reached", "model_world_nodes"]
    out = {k: value[k] for k in keys if k in value}
    if isinstance(out.get("model_world_nodes"), list):
        # Keep the most recent compact epistemic objects when context is tight.
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
