#!/usr/bin/env python3
from __future__ import annotations

import argparse
import hashlib
import json
import subprocess
from pathlib import Path
from typing import Any

from a15_contract_gate import ContractError, load_json, validate_constitution, validate_outcome, validate_turn


class NativeRuntimeError(RuntimeError):
    pass


def stable_digest(obj: Any) -> str:
    raw = json.dumps(obj, sort_keys=True, separators=(",", ":")).encode("utf-8")
    return hashlib.sha256(raw).hexdigest()


def _node(external_id: str, node_type: str, status: str, statement: str, origin: str,
          turn: int, metadata: dict[str, Any] | None = None) -> dict[str, Any]:
    md = {"turn": str(turn), **{str(k): str(v) for k, v in (metadata or {}).items()}}
    return {
        "external_id": external_id,
        "node_type": node_type,
        "status": status,
        "statement": statement,
        "origin": origin,
        "metadata": md,
    }


def proposal_to_native_request(proposal: dict[str, Any], game_id: str) -> dict[str, Any]:
    turn = int(proposal["turn"])
    nodes: list[dict[str, Any]] = []
    relations: list[dict[str, Any]] = []

    for obs in proposal["observations"]:
        oid = str(obs["id"])
        nodes.append(_node(oid, "Fact", "Active", str(obs["statement"]), "Observed", turn,
                           {"game_id": game_id, "evidence_ref": obs["evidence_ref"]}))

    for hyp in proposal["hypotheses"]:
        hid = str(hyp["id"])
        status = {"proposed": "Proposed", "active": "Active", "contested": "Contested"}[hyp["status"]]
        nodes.append(_node(hid, "Hypothesis", status, str(hyp["statement"]), "Hypothetical", turn,
                           {"game_id": game_id, "prediction": hyp["prediction"]}))
        for oid in hyp["support_observation_ids"]:
            relations.append({"from": str(oid), "to": hid, "role": "Supports", "origin": "Inferred", "confidence": 0.55})

    for goal in proposal["candidate_goals"]:
        gid = str(goal["id"])
        status = {"proposed": "Proposed", "active": "Active", "contested": "Contested"}[goal["status"]]
        nodes.append(_node(gid, "Decision", status, str(goal["statement"]), "Hypothetical", turn,
                           {"game_id": game_id, "kind": "candidate_goal"}))
        for hid in goal["implied_by_hypothesis_ids"]:
            relations.append({"from": str(hid), "to": gid, "role": "Predictive", "origin": "Hypothetical", "confidence": 0.45})
        for oid in goal["evidence_observation_ids"]:
            relations.append({"from": str(oid), "to": gid, "role": "Supports", "origin": "Inferred", "confidence": 0.40})

    opposition = proposal["opposition"]
    opp_id = f"turn-{turn}-opposition"
    opp_statement = " | ".join(str(x) for x in opposition["falsification_questions"])
    nodes.append(_node(opp_id, "Opposition", "Active", opp_statement, "Hypothetical", turn,
                       {"game_id": game_id, "reopen": ",".join(map(str, opposition["reopen_hypothesis_ids"]))}))
    relations.append({"from": opp_id, "to": str(opposition["challenged_hypothesis_id"]), "role": "Contradicts", "origin": "Hypothetical", "confidence": 0.50})

    exp = proposal["experiment"]
    exp_id = f"turn-{turn}-experiment"
    nodes.append(_node(exp_id, "Experiment", "Active", str(exp["information_goal"]), "Hypothetical", turn,
                       {"game_id": game_id, "action": exp["action"], "predicted_observation": exp["predicted_observation"],
                        "action_params": json.dumps(exp["action_params"], sort_keys=True)}))
    for hid in exp["tests_hypothesis_ids"]:
        relations.append({"from": exp_id, "to": str(hid), "role": "Mechanistic", "origin": "Hypothetical", "confidence": 0.50})

    return {
        "protocol": "agentfabric-a15-native-v1",
        "operation": "ingest_and_reason",
        "game_id": game_id,
        "turn": turn,
        "nodes": nodes,
        "relations": relations,
        "provisional_hypothesis_id": str(proposal["provisional_hypothesis_id"]),
        "provisional_goal_id": str(proposal["provisional_goal_id"]),
        "reopen_hypothesis_ids": list(map(str, opposition["reopen_hypothesis_ids"])),
        "residual_uncertainty": list(map(str, proposal["residual_uncertainty"])),
        "experiment_id": exp_id,
        "action": str(exp["action"]),
        "action_params": exp["action_params"],
    }


def outcome_to_native_request(outcome: dict[str, Any], game_id: str) -> dict[str, Any]:
    turn = int(outcome["turn"])
    oid = f"turn-{turn}-outcome"
    node = _node(oid, "Outcome", "Active", str(outcome["observed_effect"] or "No observable effect."), "Observed", turn,
                 {"game_id": game_id, "action": outcome["action"], "before_state_digest": outcome["before_state_digest"],
                  "after_state_digest": outcome["after_state_digest"], "meaningful_change": outcome["meaningful_change"],
                  "score_delta": outcome["score_delta"], "level_delta": outcome["level_delta"]})
    relations = [{"from": str(outcome["experiment_id"]), "to": oid, "role": "Causal", "origin": "Observed", "confidence": 1.0}]
    for hid in outcome["supports_hypothesis_ids"]:
        relations.append({"from": oid, "to": str(hid), "role": "Supports", "origin": "Observed", "confidence": 0.80})
    for hid in outcome["contradicts_hypothesis_ids"]:
        relations.append({"from": oid, "to": str(hid), "role": "Contradicts", "origin": "Observed", "confidence": 0.80})
    return {"protocol": "agentfabric-a15-native-v1", "operation": "apply_outcome_and_reason", "game_id": game_id,
            "turn": turn, "nodes": [node], "relations": relations}


def invoke_native(helper: str, request: dict[str, Any], db_path: str) -> dict[str, Any]:
    proc = subprocess.run([helper, "--db", db_path], input=json.dumps(request), text=True,
                          capture_output=True, timeout=60)
    if proc.returncode != 0:
        raise NativeRuntimeError(f"native helper failed rc={proc.returncode}: {proc.stderr.strip()}")
    try:
        response = json.loads(proc.stdout)
    except json.JSONDecodeError as exc:
        raise NativeRuntimeError(f"native helper returned invalid JSON: {exc}") from exc
    receipt = response.get("reasoning_receipt") or {}
    required_receipt = ["graphene_executed", "fiber_bundle_built", "stability_critic_executed",
                        "epistemic_admissibility_executed", "lyapunov_trajectory_executed",
                        "convergence_executed", "opposition_executed", "no_silent_promotion"]
    missing = [k for k in required_receipt if k not in receipt]
    if missing:
        raise NativeRuntimeError(f"native helper receipt missing {missing}")
    if not all(bool(receipt[k]) for k in required_receipt):
        raise NativeRuntimeError(f"native GrapheneDB reasoning gate incomplete: {receipt}")
    return response


def append_jsonl(path: Path, obj: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("a", encoding="utf-8") as f:
        f.write(json.dumps(obj, sort_keys=True) + "\n")


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--mode", choices=["proposal", "outcome"], required=True)
    ap.add_argument("--input", required=True)
    ap.add_argument("--game-id", required=True)
    ap.add_argument("--contract", default="pi45_a1/a15_state_contract.json")
    ap.add_argument("--constitution", default="pi45_a1/a15_reasoning_constitution.json")
    ap.add_argument("--available-actions", default="")
    ap.add_argument("--known-hypotheses", default="")
    ap.add_argument("--native-helper")
    ap.add_argument("--db")
    ap.add_argument("--trace")
    ap.add_argument("--dry-run", action="store_true")
    args = ap.parse_args()

    contract = load_json(args.contract)
    constitution = load_json(args.constitution)
    validate_constitution(constitution)
    payload = load_json(args.input)

    if args.mode == "proposal":
        available = {x for x in args.available_actions.split(",") if x} or None
        validate_turn(payload, contract, available)
        request = proposal_to_native_request(payload, args.game_id)
    else:
        known = {x for x in args.known_hypotheses.split(",") if x}
        if not known:
            raise ContractError("outcome mode requires --known-hypotheses")
        validate_outcome(payload, contract, known)
        request = outcome_to_native_request(payload, args.game_id)

    envelope = {"request_digest": stable_digest(request), "request": request}
    if args.dry_run:
        response = {"dry_run": True, "request_digest": envelope["request_digest"], "request": request}
    else:
        if not args.native_helper or not args.db:
            raise NativeRuntimeError("native mode requires --native-helper and --db; no Python fallback is permitted")
        response = invoke_native(args.native_helper, request, args.db)
        response["request_digest"] = envelope["request_digest"]

    if args.trace:
        append_jsonl(Path(args.trace), {"mode": args.mode, "game_id": args.game_id, "request": request, "response": response})
    print(json.dumps(response, sort_keys=True))


if __name__ == "__main__":
    main()
