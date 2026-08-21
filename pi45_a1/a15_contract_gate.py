#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
from pathlib import Path
from typing import Any


class ContractError(ValueError):
    pass


def load_json(path: str | Path) -> dict[str, Any]:
    return json.loads(Path(path).read_text(encoding="utf-8"))


def require_keys(obj: dict[str, Any], keys: list[str], where: str) -> None:
    missing = [key for key in keys if key not in obj]
    if missing:
        raise ContractError(f"{where}: missing required keys {missing}")


def reject_forbidden_keys(obj: Any, forbidden: set[str], where: str) -> None:
    if isinstance(obj, dict):
        bad = sorted(str(k) for k in obj if str(k) in forbidden)
        if bad:
            raise ContractError(f"{where}: evaluator-only fields entered epistemic payload: {bad}")
        for key, value in obj.items():
            reject_forbidden_keys(value, forbidden, f"{where}.{key}")
    elif isinstance(obj, list):
        for index, value in enumerate(obj):
            reject_forbidden_keys(value, forbidden, f"{where}[{index}]")


def require_unique_ids(items: list[dict[str, Any]], where: str) -> set[str]:
    ids: list[str] = []
    for idx, item in enumerate(items):
        value = str(item.get("id", "")).strip()
        if not value:
            raise ContractError(f"{where}[{idx}]: id must be non-empty")
        ids.append(value)
    if len(ids) != len(set(ids)):
        raise ContractError(f"{where}: ids must be unique")
    return set(ids)


def evaluator_only_fields(contract: dict[str, Any]) -> set[str]:
    return {str(x) for x in (contract.get("perception_contract", {}).get("evaluator_only_fields") or [])}


def validate_constitution(constitution: dict[str, Any]) -> None:
    required_sequence = [
        "non_convergent_expansion",
        "convergent_compression",
        "opposition",
        "re_expansion",
        "synthesis",
        "implementation",
        "outcome_feedback",
        "model_world_update",
    ]
    if constitution.get("control_sequence") != required_sequence:
        raise ContractError("constitution: dialectical control sequence changed")
    principles = constitution.get("principles") or []
    ids = {str(p.get("id")) for p in principles}
    required = {
        "preserve-alternatives", "separate-observation-inference",
        "goals-emerge-from-model-world", "actions-are-experiments",
        "outcomes-update-beliefs", "opposition-before-closure",
        "contradictions-are-information", "lyapunov-pattern-lock",
        "abstain-without-evidence", "provenance-governs-promotion",
    }
    if ids != required:
        raise ContractError(f"constitution: expected principle ids {sorted(required)}, got {sorted(ids)}")
    raw = json.dumps(constitution, sort_keys=True).lower()
    for token in constitution.get("forbidden_tokens") or []:
        if str(token).lower() in raw.replace(json.dumps(constitution.get("forbidden_tokens"), sort_keys=True).lower(), ""):
            raise ContractError(f"constitution: target-game contamination token present: {token}")
    mapping = constitution.get("model_world_mapping") or {}
    if mapping.get("candidate_goal", {}).get("node_type") != "Decision":
        raise ContractError("constitution: candidate goals must map to native ModelWorld Decision nodes")
    if mapping.get("candidate_goal", {}).get("origin") != "Hypothetical":
        raise ContractError("constitution: candidate goals must remain hypothetical")
    if mapping.get("outcome", {}).get("origin") != "Observed":
        raise ContractError("constitution: outcomes must be observed")


def validate_turn(proposal: dict[str, Any], contract: dict[str, Any], available_actions: set[str] | None = None) -> None:
    reject_forbidden_keys(proposal, evaluator_only_fields(contract), "turn_proposal")
    spec = contract["turn_proposal"]
    require_keys(proposal, spec["required"], "turn_proposal")
    limits = spec["limits"]
    observations = list(proposal["observations"])
    hypotheses = list(proposal["hypotheses"])
    goals = list(proposal["candidate_goals"])
    if not limits["observations_min"] <= len(observations) <= limits["observations_max"]:
        raise ContractError("turn_proposal: observation count outside contract")
    if not limits["hypotheses_min"] <= len(hypotheses) <= limits["hypotheses_max"]:
        raise ContractError("turn_proposal: hypothesis count outside contract")
    if not limits["candidate_goals_min"] <= len(goals) <= limits["candidate_goals_max"]:
        raise ContractError("turn_proposal: candidate-goal count outside contract")

    obj_specs = spec["objects"]
    for idx, item in enumerate(observations):
        require_keys(item, obj_specs["observation"]["required"], f"observations[{idx}]")
    for idx, item in enumerate(hypotheses):
        require_keys(item, obj_specs["hypothesis"]["required"], f"hypotheses[{idx}]")
        if item["status"] not in obj_specs["hypothesis"]["allowed_status"]:
            raise ContractError(f"hypotheses[{idx}]: invalid status {item['status']}")
    for idx, item in enumerate(goals):
        require_keys(item, obj_specs["candidate_goal"]["required"], f"candidate_goals[{idx}]")
        if item["status"] not in obj_specs["candidate_goal"]["allowed_status"]:
            raise ContractError(f"candidate_goals[{idx}]: invalid status {item['status']}")
        if item["statement"].strip().lower() in {"win", "win the game", "complete the game", "beat the game"}:
            raise ContractError("candidate_goal: content-free win goal is forbidden")

    observation_ids = require_unique_ids(observations, "observations")
    hypothesis_ids = require_unique_ids(hypotheses, "hypotheses")
    goal_ids = require_unique_ids(goals, "candidate_goals")
    for idx, hypothesis in enumerate(hypotheses):
        support = set(map(str, hypothesis["support_observation_ids"]))
        if not support.issubset(observation_ids):
            raise ContractError(f"hypotheses[{idx}]: references unknown observation")
    for idx, goal in enumerate(goals):
        implied = set(map(str, goal["implied_by_hypothesis_ids"]))
        evidence = set(map(str, goal["evidence_observation_ids"]))
        if not implied or not implied.issubset(hypothesis_ids):
            raise ContractError(f"candidate_goals[{idx}]: must be implied by known hypothesis")
        if not evidence.issubset(observation_ids):
            raise ContractError(f"candidate_goals[{idx}]: references unknown observation")

    provisional_h = str(proposal["provisional_hypothesis_id"])
    provisional_g = str(proposal["provisional_goal_id"])
    if provisional_h not in hypothesis_ids:
        raise ContractError("turn_proposal: provisional hypothesis is not preserved in FiberBundle input")
    if provisional_g not in goal_ids:
        raise ContractError("turn_proposal: provisional goal is unknown")

    opposition = proposal["opposition"]
    require_keys(opposition, obj_specs["opposition"]["required"], "opposition")
    if str(opposition["challenged_hypothesis_id"]) != provisional_h:
        raise ContractError("opposition: must challenge the provisional convergence")
    questions = list(opposition["falsification_questions"])
    if not limits["falsification_questions_min"] <= len(questions) <= limits["falsification_questions_max"]:
        raise ContractError("opposition: falsification-question count outside contract")
    reopened = set(map(str, opposition["reopen_hypothesis_ids"]))
    if not reopened.issubset(hypothesis_ids):
        raise ContractError("opposition: attempted to reopen unknown hypothesis")

    experiment = proposal["experiment"]
    require_keys(experiment, obj_specs["experiment"]["required"], "experiment")
    tests = set(map(str, experiment["tests_hypothesis_ids"]))
    if not tests or not tests.issubset(hypothesis_ids):
        raise ContractError("experiment: every action must test a preserved hypothesis")
    action = str(experiment["action"])
    if available_actions is not None and action not in available_actions:
        raise ContractError(f"experiment: action {action!r} not in available action set")


def validate_outcome(outcome: dict[str, Any], contract: dict[str, Any], known_hypotheses: set[str]) -> None:
    spec = contract["outcome_record"]
    forbidden = set(map(str, spec.get("forbidden_evaluator_fields") or [])) | evaluator_only_fields(contract)
    reject_forbidden_keys(outcome, forbidden, "outcome")
    require_keys(outcome, spec["required"], "outcome")
    supports = set(map(str, outcome["supports_hypothesis_ids"]))
    contradicts = set(map(str, outcome["contradicts_hypothesis_ids"]))
    if not supports.issubset(known_hypotheses) or not contradicts.issubset(known_hypotheses):
        raise ContractError("outcome: references unknown hypothesis")
    if supports & contradicts:
        raise ContractError("outcome: same hypothesis cannot be both supported and contradicted by one atomic interpretation")
    changed_cells = int(outcome["changed_cells"])
    if changed_cells < 0:
        raise ContractError("outcome: changed_cells cannot be negative")
    if not isinstance(outcome["changed_regions"], list):
        raise ContractError("outcome: changed_regions must be a list of observable regions")
    informative = changed_cells > 0 or bool(outcome["changed_regions"]) or bool(outcome["persistent_change"]) or bool(str(outcome["observed_effect"]).strip())
    if informative and not (supports or contradicts):
        raise ContractError("outcome: informative result must update at least one hypothesis")


def self_test(contract: dict[str, Any], constitution: dict[str, Any]) -> None:
    validate_constitution(constitution)
    proposal = {
        "turn": 0,
        "observations": [{"id": "o1", "statement": "A visible region exists.", "evidence_ref": "grid:0"}],
        "hypotheses": [
            {"id": "h1", "statement": "The region may be interactable.", "support_observation_ids": ["o1"], "prediction": "An interaction may alter the grid.", "status": "active"},
            {"id": "h2", "statement": "The region may be inert.", "support_observation_ids": ["o1"], "prediction": "Interaction leaves the grid invariant.", "status": "proposed"}
        ],
        "candidate_goals": [{"id": "g1", "statement": "Determine whether the visible region participates in the world rules.", "implied_by_hypothesis_ids": ["h1", "h2"], "evidence_observation_ids": ["o1"], "status": "active"}],
        "provisional_hypothesis_id": "h1",
        "provisional_goal_id": "g1",
        "opposition": {"challenged_hypothesis_id": "h1", "falsification_questions": ["What grid change would support inertness instead?"], "reopen_hypothesis_ids": ["h2"]},
        "experiment": {"tests_hypothesis_ids": ["h1", "h2"], "information_goal": "Discriminate responsive from inert using observable grid evidence.", "predicted_observation": "The grid changes or remains invariant.", "action": "ACTION1", "action_params": {}},
        "residual_uncertainty": ["Action semantics are not yet known."]
    }
    validate_turn(proposal, contract, {"ACTION1", "ACTION2"})
    outcome = {
        "turn": 0, "experiment_id": "e1", "action": "ACTION1",
        "before_grid_digest": "before", "after_grid_digest": "after",
        "changed_cells": 1, "changed_regions": ["r0"], "persistent_change": True,
        "observed_effect": "One visible cell changed persistently.",
        "supports_hypothesis_ids": ["h1"], "contradicts_hypothesis_ids": ["h2"]
    }
    validate_outcome(outcome, contract, {"h1", "h2"})

    bad_goal = json.loads(json.dumps(proposal))
    bad_goal["candidate_goals"][0]["statement"] = "Win the game"
    try:
        validate_turn(bad_goal, contract, {"ACTION1"})
    except ContractError:
        pass
    else:
        raise AssertionError("self-test failed: content-free goal was accepted")

    bad_action = json.loads(json.dumps(proposal))
    bad_action["experiment"]["action"] = "ACTION99"
    try:
        validate_turn(bad_action, contract, {"ACTION1"})
    except ContractError:
        pass
    else:
        raise AssertionError("self-test failed: unavailable action was accepted")

    contaminated = json.loads(json.dumps(outcome))
    contaminated["score_delta"] = 1
    try:
        validate_outcome(contaminated, contract, {"h1", "h2"})
    except ContractError:
        pass
    else:
        raise AssertionError("self-test failed: evaluator metadata entered epistemic outcome")


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--contract", default="pi45_a1/a15_state_contract.json")
    ap.add_argument("--constitution", default="pi45_a1/a15_reasoning_constitution.json")
    ap.add_argument("--proposal")
    ap.add_argument("--available-actions", default="")
    ap.add_argument("--outcome")
    args = ap.parse_args()
    contract = load_json(args.contract)
    constitution = load_json(args.constitution)
    self_test(contract, constitution)
    if args.proposal:
        proposal = load_json(args.proposal)
        available = {x for x in args.available_actions.split(",") if x} or None
        validate_turn(proposal, contract, available)
        if args.outcome:
            outcome = load_json(args.outcome)
            hypothesis_ids = {str(x["id"]) for x in proposal["hypotheses"]}
            validate_outcome(outcome, contract, hypothesis_ids)
    print("a15_contract_gate=PASS")


if __name__ == "__main__":
    main()
