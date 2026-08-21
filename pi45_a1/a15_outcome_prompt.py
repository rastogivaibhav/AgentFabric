#!/usr/bin/env python3
from __future__ import annotations

import json
from typing import Any

from a15_proposal_prompt import EVALUATOR_ONLY_FIELDS, _reject_evaluator_metadata, parse_json_object


SYSTEM_RULES = """You are the outcome interpreter inside a governed dialectical reasoning system for an unknown interactive world.
Use only the observable before/after grid evidence, the experiment, and the preserved hypotheses supplied here.
Do not use score, level, terminal state, game identity, or hidden game semantics.
Decide which tested hypotheses are supported, contradicted, or remain unresolved by this single observation.
Do not promote a hypothesis to truth. Return JSON only."""


def build_outcome_prompt(*, turn: int, experiment: dict[str, Any], hypotheses: list[dict[str, Any]],
                         grid_outcome: dict[str, Any], max_chars: int = 5000) -> str:
    _reject_evaluator_metadata(experiment, "experiment")
    _reject_evaluator_metadata(hypotheses, "hypotheses")
    _reject_evaluator_metadata(grid_outcome, "grid_outcome")
    tested = {str(x) for x in experiment.get("tests_hypothesis_ids") or []}
    supplied = {str(x.get("id")) for x in hypotheses}
    if not tested or not tested.issubset(supplied):
        raise ValueError("outcome interpretation requires all tested hypotheses")
    context = {
        "turn": int(turn),
        "experiment": experiment,
        "tested_hypotheses": [h for h in hypotheses if str(h.get("id")) in tested],
        "observable_grid_outcome": grid_outcome,
    }
    schema = {
        "observed_effect": "brief statement grounded only in grid evidence",
        "supports_hypothesis_ids": ["tested hypothesis id"],
        "contradicts_hypothesis_ids": ["tested hypothesis id"],
        "unresolved_hypothesis_ids": ["tested hypothesis id"],
    }
    prompt = SYSTEM_RULES + "\n\nEVIDENCE:\n" + json.dumps(context, sort_keys=True, separators=(",", ":"))
    prompt += "\n\nOUTPUT CONTRACT:\n" + json.dumps(schema, sort_keys=True, separators=(",", ":"))
    prompt += "\nEach tested hypothesis ID may appear in AT MOST ONE of supports_hypothesis_ids, contradicts_hypothesis_ids, or unresolved_hypothesis_ids."
    if len(prompt) > max_chars:
        raise ValueError(f"A1.5 outcome prompt exceeds dedicated budget: {len(prompt)} > {max_chars}")
    return prompt


def build_outcome_repair_prompt(*, original_prompt: str, invalid_output: str,
                                validation_error: str, tested_ids: set[str],
                                max_chars: int = 6000) -> str:
    """Build one bounded repair request without adding new world evidence.

    The repair step is deliberately syntactic/epistemic: it receives only the
    already-authorized outcome prompt, the model's rejected JSON, the validator
    error, and the tested IDs. It may not add observations or reinterpret hidden
    evaluator metadata. A second invalid result remains a hard failure.
    """
    payload = {
        "validation_error": str(validation_error)[:600],
        "tested_hypothesis_ids": sorted(str(x) for x in tested_ids),
        "rejected_output": str(invalid_output)[:2400],
    }
    repair_rules = """Your previous outcome JSON was rejected by the deterministic epistemic validator.
Repair the JSON using ONLY the exact evidence in ORIGINAL OUTCOME REQUEST.
Do not add new observations, hypotheses, game semantics, score, level, terminal state, or game identity.
Every referenced ID must be one of TESTED_HYPOTHESIS_IDS.
The supported, contradicted, and unresolved ID sets MUST be pairwise disjoint.
Do not promote any hypothesis to truth. Return one corrected JSON object only."""
    prompt = repair_rules + "\n\nORIGINAL OUTCOME REQUEST:\n" + original_prompt
    prompt += "\n\nREPAIR CONTEXT:\n" + json.dumps(payload, sort_keys=True, separators=(",", ":"))
    if len(prompt) > max_chars:
        raise ValueError(f"A1.5 outcome repair prompt exceeds dedicated budget: {len(prompt)} > {max_chars}")
    return prompt


def validate_outcome_interpretation(obj: dict[str, Any], tested_ids: set[str]) -> dict[str, Any]:
    required = {"observed_effect", "supports_hypothesis_ids", "contradicts_hypothesis_ids", "unresolved_hypothesis_ids"}
    missing = sorted(required - set(obj))
    if missing:
        raise ValueError(f"outcome interpretation missing {missing}")
    groups = {
        "supports": {str(x) for x in obj["supports_hypothesis_ids"]},
        "contradicts": {str(x) for x in obj["contradicts_hypothesis_ids"]},
        "unresolved": {str(x) for x in obj["unresolved_hypothesis_ids"]},
    }
    union = groups["supports"] | groups["contradicts"] | groups["unresolved"]
    if not union.issubset(tested_ids):
        raise ValueError("outcome interpretation references untested hypothesis")
    if groups["supports"] & groups["contradicts"] or groups["supports"] & groups["unresolved"] or groups["contradicts"] & groups["unresolved"]:
        raise ValueError("outcome interpretation classifications must be disjoint")
    return {
        "observed_effect": str(obj["observed_effect"])[:600],
        "supports_hypothesis_ids": sorted(groups["supports"]),
        "contradicts_hypothesis_ids": sorted(groups["contradicts"]),
        "unresolved_hypothesis_ids": sorted(groups["unresolved"]),
    }
