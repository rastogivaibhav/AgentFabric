#!/usr/bin/env python3
from __future__ import annotations

import json
from typing import Any


EVALUATOR_ONLY_FIELDS = {
    "game_id", "guid", "state", "levels_completed", "win_levels", "score",
    "score_delta", "level_delta", "scorecard", "official_success", "level_scores",
}

SYSTEM_RULES = """You propose testable interpretations of an unknown interactive integer-grid world.
You do NOT know the objective or action semantics. Do not invent target-specific rules, object identities, a player, or movement unless an observed transition supports them.
The runtime supplies an EVIDENCE CATALOG of observed facts. Do not invent or rewrite observations.
Preserve at least two genuinely different competing hypotheses. Every hypothesis must explicitly cite catalog observation IDs, including at least one grid or transition observation.
ACTIONn availability is affordance evidence only; it cannot by itself justify a causal/world hypothesis.
Candidate goals must follow from evidence and hypotheses; never use 'win the game'.
Give falsification questions, but DO NOT choose convergence, challenged hypotheses, or reopen sets; the native GrapheneDB/HypoKosh controller owns those decisions.
Every action is an experiment. Avoid repeating a no-effect action in an unchanged state when another opaque action is available.
Use only exposed ACTIONn identifiers/parameters. Do not infer meaning from action names.
Semantic text must come from CURRENT EVIDENCE. Contract/validator/repair text is not world evidence.
Return one compact JSON object only."""


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


def _grid_catalog_statement(observation: Any) -> str:
    if isinstance(observation, dict):
        digest = str(observation.get("grid_digest") or "")
        rle = str(observation.get("grid_rle") or "")
        headline = rle.splitlines()[0].strip() if rle else ""
        if headline and digest:
            return f"Current observed integer grid: {headline}; digest={digest[:16]}."
        if headline:
            return f"Current observed integer grid: {headline}."
        if "grid" in observation:
            raw = json.dumps(observation["grid"], separators=(",", ":"))
            return f"Current observed integer grid={raw[:600]}."
    return "A current integer grid observation is available in CURRENT EVIDENCE."


def build_evidence_catalog(*, turn: int, observation: Any,
                           available_actions: list[str],
                           recent_outcomes: list[dict[str, Any]]) -> list[dict[str, Any]]:
    """Create observed evidence deterministically; the LLM does not author facts."""
    _reject_evaluator_metadata(observation, "observation")
    _reject_evaluator_metadata(recent_outcomes, "recent_outcomes")
    catalog: list[dict[str, Any]] = [{
        "id": f"t{turn}-o-grid",
        "statement": _grid_catalog_statement(observation),
        "evidence_ref": f"grid:turn:{turn}",
        "evidence_kind": "grid",
    }]
    if recent_outcomes:
        last = recent_outcomes[-1]
        previous_turn = int(last.get("turn", max(0, turn - 1)))
        action = str(last.get("action") or "opaque action")
        changed = int(last.get("changed_cells", 0) or 0)
        persistent = bool(last.get("persistent_change", False))
        effect = str(last.get("observed_effect") or "No additional interpretation recorded.")
        catalog.append({
            "id": f"t{turn}-o-transition",
            "statement": (
                f"Observed prior transition after {action}: changed_cells={changed}; "
                f"persistent_change={str(persistent).lower()}; effect={effect[:300]}"
            ),
            "evidence_ref": f"transition:turn:{previous_turn}",
            "evidence_kind": "transition",
        })
    catalog.append({
        "id": f"t{turn}-o-affordance",
        "statement": "Opaque actions currently available: " + ", ".join(map(str, available_actions)),
        "evidence_ref": f"affordance:turn:{turn}",
        "evidence_kind": "affordance",
    })
    return catalog


def _output_contract(turn: int) -> str:
    """Compact model-language contract; canonical IDs are assigned by code."""
    return f"""COMPACT FIELD CONTRACT. Do not emit observations or canonical IDs; the runtime already owns them.
Required top-level keys: "hypotheses", "candidate_goals", "falsification_questions", "experiment", "residual_uncertainty".
Hypothesis[] 2..5: "statement"; "basis_observation_ids" non-empty catalog IDs with >=1 grid/transition ID; "prediction". Do not emit status/id.
CandidateGoal[] 1..3: "statement"; "basis" non-empty GoalBasis[]. GoalBasis has integer "hypothesis_index" (0-based index into hypotheses) plus catalog "observation_id". Do not emit status/id.
"falsification_questions": 1..3 concrete strings.
Experiment: "tests_hypothesis_indices" non-empty 0-based hypothesis indices; "information_goal"; "predicted_observation"; "action" exactly one available ACTIONn; "action_params" matching exposed schema.
"residual_uncertainty": concrete string array.
The runtime will assign t{turn}-h* and t{turn}-g* IDs and will materialize the supplied evidence references exactly. JSON only."""


def build_proposal_prompt(*, turn: int, game_id: str, observation: Any,
                          available_actions: list[str], recent_outcomes: list[dict[str, Any]],
                          governed_context: dict[str, Any] | None = None,
                          max_chars: int = 6500) -> str:
    del game_id
    _reject_evaluator_metadata(governed_context or {}, "governed_context")
    if any(not str(a).startswith("ACTION") for a in available_actions):
        raise ValueError("available_actions must remain opaque ACTIONn identifiers")
    catalog = build_evidence_catalog(
        turn=turn,
        observation=observation,
        available_actions=available_actions,
        recent_outcomes=recent_outcomes,
    )
    context = {
        "turn": int(turn),
        "current_observation": observation,
        "available_actions": available_actions,
        "evidence_catalog": catalog,
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


def extract_prompt_context(prompt: str) -> dict[str, Any]:
    try:
        encoded = prompt.split("\n\nCURRENT EVIDENCE:\n", 1)[1].split("\n\nOUTPUT CONTRACT:\n", 1)[0]
        value = json.loads(encoded)
    except Exception as exc:
        raise ValueError("unable to recover deterministic proposal context") from exc
    if not isinstance(value, dict):
        raise ValueError("proposal context must be an object")
    return value


def canonicalize_compact_proposal(compact: dict[str, Any], *, turn: int,
                                  evidence_catalog: list[dict[str, Any]],
                                  available_actions: list[str]) -> dict[str, Any]:
    """Translate explicit compact references into canonical state-contract objects.

    This function assigns IDs and expands indices only. It never invents a
    missing hypothesis/evidence link or semantic statement.
    """
    required = {"hypotheses", "candidate_goals", "falsification_questions", "experiment", "residual_uncertainty"}
    missing = sorted(required - set(compact))
    if missing:
        raise ValueError(f"compact proposal missing required keys {missing}")
    hypotheses_raw = compact["hypotheses"]
    goals_raw = compact["candidate_goals"]
    questions = compact["falsification_questions"]
    experiment_raw = compact["experiment"]
    uncertainty = compact["residual_uncertainty"]
    if not isinstance(hypotheses_raw, list) or not (2 <= len(hypotheses_raw) <= 5):
        raise ValueError("compact hypotheses must contain 2..5 items")
    if not isinstance(goals_raw, list) or not (1 <= len(goals_raw) <= 3):
        raise ValueError("compact candidate_goals must contain 1..3 items")
    if not isinstance(questions, list) or not (1 <= len(questions) <= 3):
        raise ValueError("compact falsification_questions must contain 1..3 items")
    if not isinstance(experiment_raw, dict):
        raise ValueError("compact experiment must be an object")
    if not isinstance(uncertainty, list):
        raise ValueError("compact residual_uncertainty must be an array")

    catalog_ids = {str(item["id"]) for item in evidence_catalog}
    hypotheses: list[dict[str, Any]] = []
    for index, raw_h in enumerate(hypotheses_raw):
        if not isinstance(raw_h, dict):
            raise ValueError(f"compact hypotheses[{index}] must be an object")
        for key in ["statement", "basis_observation_ids", "prediction"]:
            if key not in raw_h:
                raise ValueError(f"compact hypotheses[{index}] missing {key}")
        basis_ids = raw_h["basis_observation_ids"]
        if not isinstance(basis_ids, list) or not basis_ids:
            raise ValueError(f"compact hypotheses[{index}].basis_observation_ids must be non-empty")
        basis: list[dict[str, str]] = []
        for oid in basis_ids:
            oid = str(oid)
            if oid not in catalog_ids:
                raise ValueError(f"compact hypotheses[{index}] references unknown observation {oid}")
            basis.append({"observation_id": oid})
        hypotheses.append({
            "id": f"t{turn}-h{index + 1}",
            "statement": raw_h["statement"],
            "basis": basis,
            "prediction": raw_h["prediction"],
            "status": "proposed",
        })

    goals: list[dict[str, Any]] = []
    for goal_index, raw_g in enumerate(goals_raw):
        if not isinstance(raw_g, dict):
            raise ValueError(f"compact candidate_goals[{goal_index}] must be an object")
        for key in ["statement", "basis"]:
            if key not in raw_g:
                raise ValueError(f"compact candidate_goals[{goal_index}] missing {key}")
        raw_basis = raw_g["basis"]
        if not isinstance(raw_basis, list) or not raw_basis:
            raise ValueError(f"compact candidate_goals[{goal_index}].basis must be non-empty")
        basis: list[dict[str, str]] = []
        for basis_index, link in enumerate(raw_basis):
            if not isinstance(link, dict) or "hypothesis_index" not in link or "observation_id" not in link:
                raise ValueError(f"compact candidate_goals[{goal_index}].basis[{basis_index}] invalid")
            h_index = int(link["hypothesis_index"])
            oid = str(link["observation_id"])
            if not (0 <= h_index < len(hypotheses)):
                raise ValueError(f"compact goal basis hypothesis_index {h_index} out of range")
            if oid not in catalog_ids:
                raise ValueError(f"compact goal basis references unknown observation {oid}")
            basis.append({"hypothesis_id": hypotheses[h_index]["id"], "observation_id": oid})
        goals.append({
            "id": f"t{turn}-g{goal_index + 1}",
            "statement": raw_g["statement"],
            "basis": basis,
            "status": "proposed",
        })

    for key in ["tests_hypothesis_indices", "information_goal", "predicted_observation", "action", "action_params"]:
        if key not in experiment_raw:
            raise ValueError(f"compact experiment missing {key}")
    test_indices = experiment_raw["tests_hypothesis_indices"]
    if not isinstance(test_indices, list) or not test_indices:
        raise ValueError("compact experiment.tests_hypothesis_indices must be non-empty")
    tests: list[str] = []
    for value in test_indices:
        idx = int(value)
        if not (0 <= idx < len(hypotheses)):
            raise ValueError(f"compact experiment hypothesis index {idx} out of range")
        tests.append(hypotheses[idx]["id"])
    action = str(experiment_raw["action"])
    if action not in set(map(str, available_actions)):
        raise ValueError(f"compact experiment action {action!r} unavailable")

    return {
        "turn": int(turn),
        "observations": evidence_catalog,
        "hypotheses": hypotheses,
        "candidate_goals": goals,
        "opposition": {"falsification_questions": questions},
        "experiment": {
            "tests_hypothesis_ids": tests,
            "information_goal": experiment_raw["information_goal"],
            "predicted_observation": experiment_raw["predicted_observation"],
            "action": action,
            "action_params": experiment_raw["action_params"],
        },
        "residual_uncertainty": uncertainty,
    }


def parse_and_canonicalize_model_proposal(text: str, *, prompt: str) -> dict[str, Any]:
    context = extract_prompt_context(prompt)
    compact = parse_json_object(text)
    return canonicalize_compact_proposal(
        compact,
        turn=int(context["turn"]),
        evidence_catalog=list(context["evidence_catalog"]),
        available_actions=list(map(str, context["available_actions"])),
    )


def build_proposal_repair_prompt(*, original_prompt: str, invalid_output: str,
                                 validation_error: str, turn: int,
                                 available_actions: list[str], max_chars: int = 10000) -> str:
    if any(not str(a).startswith("ACTION") for a in available_actions):
        raise ValueError("proposal repair actions must remain opaque ACTIONn identifiers")
    payload = {
        "turn": int(turn),
        "validation_error": str(validation_error)[:800],
        "available_actions": [str(a) for a in available_actions],
        "rejected_output": str(invalid_output)[:3600],
    }
    rules = f"""Turn {turn} compact proposal failed deterministic validation. Repair that SAME compact proposal using ONLY ORIGINAL PROPOSAL REQUEST -> CURRENT EVIDENCE.
The rejection, validation error, contract and repair instructions are NOT world evidence. Never mention or paraphrase them in semantic fields.
Do not emit observations, canonical IDs, status fields, score, level, terminal state, game identity, hidden semantics, or unsupported facts.
Keep 2..5 genuinely different hypotheses. Each must explicitly list non-empty basis_observation_ids from EVIDENCE CATALOG, including grid or transition evidence.
Each goal basis item must contain a 0-based hypothesis_index plus a catalog observation_id. Experiment tests must use tests_hypothesis_indices.
Give 1..3 falsification questions. Do not output native-controller-owned convergence/challenge/reopen fields.
Use only listed ACTIONn values/parameters. Return corrected compact JSON only."""
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
