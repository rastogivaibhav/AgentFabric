#!/usr/bin/env python3
from __future__ import annotations

import json
from typing import Any


SYSTEM_RULES = """You are the proposal generator inside a governed dialectical reasoning system for an unknown interactive world.
You do NOT know the objective or action semantics. Do not invent target-specific rules.
Separate observations from hypotheses. Preserve at least two competing hypotheses.
Candidate goals must emerge from those hypotheses and current evidence; never use 'win the game' as a goal.
Choose one provisional hypothesis and goal only for the next experiment, not as truth.
Oppose the provisional hypothesis with a falsification question and preserve at least one alternative when possible.
Every action is an experiment chosen to discriminate hypotheses or reduce important uncertainty.
Return JSON only. The external GrapheneDB runtime owns convergence, epistemic promotion, opposition/reopening, Lyapunov stability and model-world state."""


def build_proposal_prompt(*, turn: int, game_id: str, observation: Any,
                          available_actions: list[str], recent_outcomes: list[dict[str, Any]],
                          governed_context: dict[str, Any] | None = None,
                          max_chars: int = 6500) -> str:
    context = {
        "turn": int(turn),
        "game_id": game_id,
        "current_observation": observation,
        "available_actions": available_actions,
        "recent_outcomes": recent_outcomes[-4:],
        "governed_context": governed_context or {},
    }
    schema = {
        "turn": turn,
        "observations": [{"id": "o...", "statement": "observable fact only", "evidence_ref": "state/transition ref"}],
        "hypotheses": [
            {"id": "h...", "statement": "possible interpretation", "support_observation_ids": ["o..."],
             "prediction": "testable prediction", "status": "proposed|active|contested"}
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
        # Preserve rules, live observation/actions, and contract. Governed/recent context is expendable first.
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
            "residual_uncertainty", "escape_required", "lyapunov_goal_reached"]
    return {k: value[k] for k in keys if k in value}


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
