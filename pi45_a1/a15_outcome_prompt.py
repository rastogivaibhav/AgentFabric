#!/usr/bin/env python3
from __future__ import annotations

import json
from typing import Any

from a15_proposal_prompt import _reject_evaluator_metadata


VALID_VERDICTS = {"supported", "contradicted", "unresolved"}

SYSTEM_RULES = """You are the outcome interpreter inside a governed dialectical reasoning system for an unknown interactive world.
Use only the observable before/after grid evidence, the experiment, and the preserved hypotheses supplied here.
Do not use score, level, terminal state, game identity, or hidden game semantics.
Classify EVERY tested hypothesis exactly once as supported, contradicted, or unresolved by this single observation.
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
        "classifications": [
            {
                "hypothesis_id": "each tested hypothesis id exactly once",
                "verdict": "supported | contradicted | unresolved",
            }
        ],
    }
    prompt = SYSTEM_RULES + "\n\nEVIDENCE:\n" + json.dumps(context, sort_keys=True, separators=(",", ":"))
    prompt += "\n\nOUTPUT CONTRACT:\n" + json.dumps(schema, sort_keys=True, separators=(",", ":"))
    prompt += "\nThe classifications array MUST contain exactly one row for every tested hypothesis ID, no duplicates, no omissions, and no extra IDs."
    if len(prompt) > max_chars:
        raise ValueError(f"A1.5 outcome prompt exceeds dedicated budget: {len(prompt)} > {max_chars}")
    return prompt


def build_outcome_repair_prompt(*, original_prompt: str, invalid_output: str,
                                validation_error: str, tested_ids: set[str],
                                max_chars: int = 6000) -> str:
    """Build one bounded repair request without adding new world evidence."""
    payload = {
        "validation_error": str(validation_error)[:600],
        "tested_hypothesis_ids": sorted(str(x) for x in tested_ids),
        "rejected_output": str(invalid_output)[:2400],
    }
    repair_rules = """Your previous outcome JSON was rejected by the deterministic epistemic validator.
Repair the JSON using ONLY the exact evidence in ORIGINAL OUTCOME REQUEST.
Do not add new observations, hypotheses, game semantics, score, level, terminal state, or game identity.
Return `classifications` with EXACTLY ONE row for EACH tested hypothesis ID.
Each row must contain only a tested `hypothesis_id` and one `verdict`: supported, contradicted, or unresolved.
Do not duplicate, omit, or add hypothesis IDs. Do not promote any hypothesis to truth.
Return one corrected JSON object only."""
    prompt = repair_rules + "\n\nORIGINAL OUTCOME REQUEST:\n" + original_prompt
    prompt += "\n\nREPAIR CONTEXT:\n" + json.dumps(payload, sort_keys=True, separators=(",", ":"))
    if len(prompt) > max_chars:
        raise ValueError(f"A1.5 outcome repair prompt exceeds dedicated budget: {len(prompt)} > {max_chars}")
    return prompt


def validate_outcome_interpretation(obj: dict[str, Any], tested_ids: set[str]) -> dict[str, Any]:
    required = {"observed_effect", "classifications"}
    missing = sorted(required - set(obj))
    if missing:
        raise ValueError(f"outcome interpretation missing {missing}")

    rows = obj.get("classifications")
    if not isinstance(rows, list):
        raise ValueError("outcome classifications must be a list")
    if len(rows) != len(tested_ids):
        raise ValueError(
            f"outcome classifications must contain exactly one row per tested hypothesis: "
            f"expected {len(tested_ids)}, got {len(rows)}"
        )

    by_id: dict[str, str] = {}
    for index, row in enumerate(rows):
        if not isinstance(row, dict):
            raise ValueError(f"outcome classification row {index} must be an object")
        hypothesis_id = str(row.get("hypothesis_id", ""))
        verdict = str(row.get("verdict", "")).lower()
        if hypothesis_id not in tested_ids:
            raise ValueError(f"outcome classification references untested hypothesis {hypothesis_id!r}")
        if hypothesis_id in by_id:
            raise ValueError(f"outcome classification duplicates hypothesis {hypothesis_id!r}")
        if verdict not in VALID_VERDICTS:
            raise ValueError(f"outcome classification has invalid verdict {verdict!r}")
        by_id[hypothesis_id] = verdict

    if set(by_id) != tested_ids:
        missing_ids = sorted(tested_ids - set(by_id))
        extra_ids = sorted(set(by_id) - tested_ids)
        raise ValueError(f"outcome classification coverage mismatch: missing={missing_ids}, extra={extra_ids}")

    normalized_rows = [
        {"hypothesis_id": hypothesis_id, "verdict": by_id[hypothesis_id]}
        for hypothesis_id in sorted(by_id)
    ]
    supports = [row["hypothesis_id"] for row in normalized_rows if row["verdict"] == "supported"]
    contradicts = [row["hypothesis_id"] for row in normalized_rows if row["verdict"] == "contradicted"]
    unresolved = [row["hypothesis_id"] for row in normalized_rows if row["verdict"] == "unresolved"]

    return {
        "observed_effect": str(obj["observed_effect"])[:600],
        "classifications": normalized_rows,
        # Deterministic compatibility projection used by the existing GrapheneDB adapter.
        "supports_hypothesis_ids": supports,
        "contradicts_hypothesis_ids": contradicts,
        "unresolved_hypothesis_ids": unresolved,
    }
