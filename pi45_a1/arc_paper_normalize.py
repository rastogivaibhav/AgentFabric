#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
from pathlib import Path
from typing import Any

MODEL_SHA = "6a1a2eb6d15622bf3c96857206351ba97e1af16c30d7a74ee38970e434e9407e"


def load_json(path: Path) -> dict[str, Any] | None:
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
        return value if isinstance(value, dict) else None
    except Exception:
        return None


def parse_kv_contract(out_dir: Path) -> dict[str, str]:
    for name in ["paper_treatment_contract.txt", "treatment_contract.txt", "control_contract.txt"]:
        path = out_dir / name
        if path.exists():
            result: dict[str, str] = {}
            for line in path.read_text(encoding="utf-8", errors="replace").splitlines():
                if "=" in line:
                    key, value = line.split("=", 1)
                    result[key.strip()] = value.strip()
            return result
    return {}


def first_game(summary: dict[str, Any]) -> dict[str, Any]:
    games = summary.get("games") or []
    if isinstance(games, list) and games and isinstance(games[0], dict):
        return games[0]
    return {}


def score_value(summary: dict[str, Any]) -> Any:
    scorecard = summary.get("scorecard")
    if isinstance(scorecard, dict):
        for key in ["score", "total_score"]:
            if key in scorecard:
                return scorecard[key]
    return summary.get("score")


def int_field(*values: Any, default: int = 0) -> int:
    for value in values:
        if value is None:
            continue
        try:
            return int(value)
        except Exception:
            continue
    return default


def normalize(variant: str, out_dir: Path, expected_game: str, max_actions: int) -> dict[str, Any]:
    summary = load_json(out_dir / "summary.json") or {}
    game = first_game(summary)
    aggregate = summary.get("aggregate") if isinstance(summary.get("aggregate"), dict) else {}
    inst = game.get("instrumentation") if isinstance(game.get("instrumentation"), dict) else {}
    contract = parse_kv_contract(out_dir)
    wrapper = summary.get("paper_wrapper") if isinstance(summary.get("paper_wrapper"), dict) else {}

    validity = load_json(out_dir / "measurement_validity.json")
    runner_exit = None
    for name in ["variant_exit_code.txt", "runner_exit_code.txt", "agent_exit_code.txt"]:
        path = out_dir / name
        if path.exists():
            try:
                runner_exit = int(path.read_text().strip())
            except Exception:
                pass
            break

    invalid_reasons: list[str] = []
    if validity is not None:
        invalid_reasons.extend(map(str, validity.get("invalid_reasons") or []))
        valid = bool(validity.get("valid_measurement", False))
    else:
        valid = bool(summary)
        if runner_exit not in (None, 0):
            valid = False
            invalid_reasons.append(f"runner_exit_code={runner_exit}")
        model_sha = summary.get("model_sha256")
        if model_sha and model_sha != MODEL_SHA:
            valid = False
            invalid_reasons.append("model_sha_mismatch")

    action_counts = game.get("action_counts") if isinstance(game.get("action_counts"), dict) else {}
    actions = int_field(game.get("actions"), aggregate.get("actions"))
    levels_gained = int_field(game.get("levels_gained"), aggregate.get("levels_gained"))
    no_ops = int_field(inst.get("no_ops"), aggregate.get("no_ops"))
    repeated = int_field(inst.get("repeated_state_action"), aggregate.get("repeated_state_action"))

    # A1.4 already writes a normalized behavior artifact; prefer it where useful.
    behavior = load_json(out_dir / "a14_behavior_metrics.json") or {}
    first_change = game.get("first_meaningful_change_turn")
    if first_change is None:
        first_change = behavior.get("first_meaningful_change_turn")

    record = {
        "variant": variant,
        "expected_game": expected_game,
        "game_id": game.get("game_id") or expected_game,
        "valid_measurement": valid,
        "invalid_reasons": sorted(set(invalid_reasons)),
        "runner_exit_code": runner_exit,
        "max_actions": max_actions,
        "actions": actions,
        "action_budget_fraction": (actions / max_actions) if max_actions else None,
        "levels_gained": levels_gained,
        "levels_completed": int_field(game.get("levels_completed")),
        "terminal_state": game.get("terminal_state"),
        "score": score_value(summary),
        "action_counts": action_counts,
        "unique_actions": int_field(game.get("unique_actions"), len(action_counts)),
        "no_ops": no_ops,
        "no_op_rate": (no_ops / actions) if actions else None,
        "repeated_state_action": repeated,
        "repeated_state_action_rate": (repeated / actions) if actions else None,
        "first_meaningful_change_turn": first_change,
        "proposal_errors": int_field(inst.get("proposal_errors"), aggregate.get("proposal_errors")),
        "outcome_errors": int_field(inst.get("outcome_errors"), aggregate.get("outcome_errors")),
        "native_errors": int_field(inst.get("native_errors"), aggregate.get("native_errors")),
        "governance_denials": int_field(inst.get("governance_denials")),
        "hypotheses_generated": int_field(inst.get("hypotheses_generated")),
        "candidate_goals_generated": int_field(inst.get("candidate_goals_generated")),
        "hypotheses_supported": int_field(inst.get("hypotheses_supported")),
        "hypotheses_contradicted": int_field(inst.get("hypotheses_contradicted")),
        "lyapunov_checks": int_field(inst.get("lyapunov_checks")),
        "escape_considered": int_field(inst.get("escape_considered")),
        "distinct_hypotheses": int_field(game.get("distinct_hypotheses")),
        "distinct_goals": int_field(game.get("distinct_goals")),
        "model_world_nodes": int_field(game.get("model_world_nodes")),
        "model_world_event_hash": int_field(game.get("model_world_event_hash")),
        "proposal_repairs_attempted": int_field(wrapper.get("proposal_repairs_attempted")),
        "proposal_repairs_succeeded": int_field(wrapper.get("proposal_repairs_succeeded")),
        "outcome_repairs_attempted": int_field(wrapper.get("outcome_repairs_attempted")),
        "outcome_repairs_succeeded": int_field(wrapper.get("outcome_repairs_succeeded")),
        "native_opposition_rounds": int_field(wrapper.get("native_opposition_rounds")),
        "native_reopen_events": int_field(wrapper.get("native_reopen_events")),
        "alternatives_reopened": int_field(wrapper.get("alternatives_reopened")),
        "native_primary_hypothesis_selections": int_field(wrapper.get("native_primary_hypothesis_selections")),
        "graphenedb_enabled": bool(summary.get("graphenedb_enabled", variant != "a0")),
        "persistent_world_model": bool(summary.get("persistent_world_model", variant == "a15")),
        "dialectical_model_world": variant == "a15",
        "model_sha256": summary.get("model_sha256"),
        "context_size": int_field(contract.get("context_size"), default=0) or None,
        "memory_window": int_field(contract.get("memory_window"), default=0) or None,
        "treatment_contract": contract,
    }
    return record


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--variant", required=True, choices=["a0", "a13", "a14", "a15"])
    ap.add_argument("--out-dir", required=True)
    ap.add_argument("--game", required=True)
    ap.add_argument("--max-actions", type=int, required=True)
    ap.add_argument("--output")
    args = ap.parse_args()
    out_dir = Path(args.out_dir)
    record = normalize(args.variant, out_dir, args.game, args.max_actions)
    output = Path(args.output) if args.output else out_dir / "paper_result.json"
    output.write_text(json.dumps(record, indent=2, sort_keys=True, default=str) + "\n", encoding="utf-8")
    print(json.dumps(record, indent=2, sort_keys=True, default=str))


if __name__ == "__main__":
    main()
