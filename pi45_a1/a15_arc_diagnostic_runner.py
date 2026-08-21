#!/usr/bin/env python3
from __future__ import annotations

import argparse
import hashlib
import json
import os
import subprocess
import time
import urllib.request
from collections import Counter, deque
from pathlib import Path
from typing import Any, Iterable

from a15_arc_dialectic_adapter import invoke_native, outcome_to_native_request, proposal_to_native_request
from a15_contract_gate import ContractError, load_json, validate_outcome, validate_turn
from a15_outcome_prompt import build_outcome_prompt, validate_outcome_interpretation
from a15_proposal_prompt import build_proposal_prompt, parse_json_object

MODEL_ID = "Qwen/Qwen2.5-1.5B-Instruct-GGUF"
MODEL_SHA256 = "6a1a2eb6d15622bf3c96857206351ba97e1af16c30d7a74ee38970e434e9407e"
TEMPERATURE = 0
SEED = 4242
PROMPT_MAX_CHARS = 9000
OUTCOME_PROMPT_MAX_CHARS = 6000


def _tolist(x: Any) -> Any:
    return x.tolist() if hasattr(x, "tolist") else x


def last_grid(obs: Any) -> list[list[int]]:
    frames = _tolist(getattr(obs, "frame", []) or [])
    if not frames:
        return []
    grid = _tolist(frames[-1])
    return [[int(v) for v in row] for row in grid]


def grid_hash(grid: list[list[int]]) -> str:
    return hashlib.sha256(json.dumps(grid, separators=(",", ":")).encode()).hexdigest()


def rle_row(row: Iterable[int]) -> str:
    row = list(row)
    if not row:
        return ""
    out: list[str] = []
    value, count = row[0], 1
    for cur in row[1:]:
        if cur == value:
            count += 1
        else:
            out.append(f"{value}x{count}")
            value, count = cur, 1
    out.append(f"{value}x{count}")
    return " ".join(out)


def compact_grid(grid: list[list[int]]) -> str:
    if not grid:
        return "<empty>"
    counts = Counter(v for row in grid for v in row)
    rows = "\n".join(f"{i:02d}:{rle_row(row)}" for i, row in enumerate(grid))
    return f"size={len(grid[0])}x{len(grid)};palette={dict(sorted(counts.items()))}\n{rows}"


def diff_summary(a: list[list[int]], b: list[list[int]], cap: int = 64) -> dict[str, Any]:
    if not a or not b or len(a) != len(b) or len(a[0]) != len(b[0]):
        return {"changed_cells": -1, "changed_regions": [], "transition_counts": {}}
    sample: list[dict[str, int]] = []
    transitions: Counter[tuple[int, int]] = Counter()
    for y, (ra, rb) in enumerate(zip(a, b)):
        for x, (before, after) in enumerate(zip(ra, rb)):
            if before != after:
                transitions[(before, after)] += 1
                if len(sample) < cap:
                    sample.append({"x": x, "y": y, "before": before, "after": after})
    return {
        "changed_cells": sum(transitions.values()),
        "changed_regions": sample,
        "transition_counts": {f"{x}->{y}": n for (x, y), n in sorted(transitions.items())},
    }


def action_affordances(available: list[str]) -> dict[str, Any]:
    out: dict[str, Any] = {}
    for action in available:
        if action == "ACTION6":
            out[action] = {"parameters": {"x": "integer 0..63", "y": "integer 0..63"}}
        else:
            out[action] = {"parameters": {}}
    return out


def call_llm(endpoint: str, prompt: str, *, max_tokens: int, timeout: int = 360) -> tuple[str, dict[str, Any]]:
    body = {
        "model": os.getenv("MODEL_ALIAS", "pi45a15-qwen25-15b"),
        "messages": [
            {"role": "system", "content": "Return exactly the requested JSON object and no additional text."},
            {"role": "user", "content": prompt},
        ],
        "temperature": TEMPERATURE,
        "seed": SEED,
        "max_tokens": max_tokens,
        "stream": False,
    }
    request = urllib.request.Request(endpoint, data=json.dumps(body).encode(), headers={"Content-Type": "application/json"}, method="POST")
    with urllib.request.urlopen(request, timeout=timeout) as response:
        data = json.loads(response.read().decode())
    return data["choices"][0]["message"].get("content") or "", data.get("usage") or {}


def validate_action_params(proposal: dict[str, Any], available: list[str]) -> dict[str, Any]:
    experiment = proposal["experiment"]
    action = str(experiment["action"])
    params = dict(experiment.get("action_params") or {})
    if action not in available:
        raise ContractError(f"proposed action {action} is unavailable")
    if action == "ACTION6":
        if set(params) != {"x", "y"}:
            raise ContractError("ACTION6 requires exactly x,y action_params")
        try:
            x, y = int(params["x"]), int(params["y"])
        except Exception as exc:
            raise ContractError("ACTION6 x,y must be integers") from exc
        if not (0 <= x <= 63 and 0 <= y <= 63):
            raise ContractError("ACTION6 x,y outside 0..63")
        return {"x": x, "y": y}
    if params:
        raise ContractError(f"{action} must not carry action_params")
    return {}


def model_world_snapshot(dumper: str, modelworld_path: Path) -> dict[str, Any]:
    if not modelworld_path.exists():
        return {"event_log_hash": 0, "nodes": []}
    proc = subprocess.run([dumper, str(modelworld_path)], check=True, capture_output=True, text=True, timeout=30)
    data = json.loads(proc.stdout)
    nodes = data.get("nodes") or []
    # Keep compact epistemic state only. No metadata is returned by the native dumper.
    return {"event_log_hash": int(data.get("event_log_hash", 0)), "nodes": nodes[-16:]}


def governed_context(native_response: dict[str, Any], snapshot: dict[str, Any]) -> dict[str, Any]:
    nodes = snapshot.get("nodes") or []
    return {
        "status": native_response.get("epistemic_status"),
        "residual_uncertainty": list(native_response.get("residual_uncertainty") or [])[-8:],
        "lyapunov_goal_reached": bool((native_response.get("reasoning_receipt") or {}).get("lyapunov_goal_reached", False)),
        "model_world_nodes": [
            {"external_id": n.get("external_id"), "type": n.get("type"), "status": n.get("status"), "statement": n.get("statement")}
            for n in nodes
        ],
    }


def append_jsonl(path: Path, obj: dict[str, Any]) -> None:
    with path.open("a", encoding="utf-8") as handle:
        handle.write(json.dumps(obj, sort_keys=True, default=str) + "\n")


def play_game(arc: Any, game_id: str, scorecard_id: str, args: argparse.Namespace, out_dir: Path) -> dict[str, Any]:
    from arcengine import GameAction, GameState

    env = arc.make(game_id, scorecard_id=scorecard_id, save_recording=True, include_frame_data=True)
    if env is None:
        return {"game_id": game_id, "error": "make_failed"}
    obs = env.reset()
    if obs is None:
        return {"game_id": game_id, "error": "reset_failed"}

    safe_name = game_id.split("-")[0]
    db_path = out_dir / f"{safe_name}.a15.graphenedb"
    modelworld_path = Path(str(db_path) + ".modelworld")
    subprocess.run([args.bootstrap, str(db_path)], check=True, capture_output=True, text=True, timeout=30)

    trace_path = out_dir / f"{safe_name}.a15.turns.jsonl"
    model_trace_path = out_dir / f"{safe_name}.a15.model_calls.jsonl"
    recent_outcomes: deque[dict[str, Any]] = deque(maxlen=4)
    state_action_counts: Counter[tuple[str, str, str]] = Counter()
    action_counts: Counter[str] = Counter()
    totals = Counter()
    distinct_hypotheses: set[str] = set()
    distinct_goals: set[str] = set()
    first_meaningful_change_turn: int | None = None
    last_native: dict[str, Any] = {"epistemic_status": "uninitialized", "reasoning_receipt": {}, "residual_uncertainty": []}
    start_levels = int(getattr(obs, "levels_completed", 0) or 0)

    for turn in range(args.max_actions):
        evaluator_state = getattr(obs, "state", None)
        if evaluator_state == GameState.WIN:
            break
        if evaluator_state == GameState.GAME_OVER:
            totals["resets"] += 1
            obs = env.reset()
            if obs is None:
                break

        grid = last_grid(obs)
        pre_digest = grid_hash(grid)
        available = [a.name for a in list(env.action_space)]
        if not available:
            totals["empty_action_space"] += 1
            break

        snapshot_before = model_world_snapshot(args.modelworld_dump, modelworld_path)
        current_governed = governed_context(last_native, snapshot_before)
        observation = {
            "grid_digest": pre_digest,
            "grid_rle": compact_grid(grid),
            "action_affordances": action_affordances(available),
        }
        try:
            proposal_prompt = build_proposal_prompt(
                turn=turn, game_id=game_id, observation=observation,
                available_actions=available, recent_outcomes=list(recent_outcomes),
                governed_context=current_governed, max_chars=args.proposal_prompt_max_chars,
            )
            raw_proposal, proposal_usage = call_llm(args.endpoint, proposal_prompt, max_tokens=args.proposal_max_tokens)
            proposal = parse_json_object(raw_proposal)
            validate_turn(proposal, args.contract, set(available))
            action_data = validate_action_params(proposal, available)
        except Exception as exc:
            totals["proposal_errors"] += 1
            append_jsonl(trace_path, {"turn": turn, "phase": "proposal_error", "error": repr(exc), "evaluator": {"game_id": game_id}})
            break

        if int(proposal.get("turn", -1)) != turn:
            totals["proposal_errors"] += 1
            append_jsonl(trace_path, {"turn": turn, "phase": "proposal_error", "error": "proposal turn mismatch", "proposal": proposal})
            break

        for h in proposal["hypotheses"]:
            distinct_hypotheses.add(str(h["id"]))
        for g in proposal["candidate_goals"]:
            distinct_goals.add(str(g["id"]))
        totals["hypotheses_generated"] += len(proposal["hypotheses"])
        totals["candidate_goals_generated"] += len(proposal["candidate_goals"])
        totals["opposition_rounds"] += 1

        try:
            native_request = proposal_to_native_request(proposal, game_id)
            native_response = invoke_native(args.native_helper, native_request, str(db_path))
        except Exception as exc:
            totals["native_errors"] += 1
            append_jsonl(trace_path, {"turn": turn, "phase": "native_proposal_error", "error": repr(exc)})
            break
        if not native_response.get("action_authorized") or native_response.get("governed_action") != proposal["experiment"]["action"]:
            totals["governance_denials"] += 1
            append_jsonl(trace_path, {"turn": turn, "phase": "governance_denied", "native": native_response})
            break

        action_name = str(native_response["governed_action"])
        action = GameAction.from_name(action_name)
        action_counts[action_name] += 1
        state_key = (pre_digest, action_name, json.dumps(action_data, sort_keys=True))
        if state_action_counts[state_key] > 0:
            totals["repeated_state_action"] += 1
        state_action_counts[state_key] += 1

        levels_before = int(getattr(obs, "levels_completed", 0) or 0)  # evaluator only
        try:
            next_obs = env.step(action, data=action_data, reasoning={
                "pi": "PI4.5A1.5",
                "dialectical_model_world": True,
                "experiment_id": f"turn-{turn}-experiment",
                "proposal_digest": hashlib.sha256(json.dumps(proposal, sort_keys=True).encode()).hexdigest(),
            })
        except Exception as exc:
            totals["step_errors"] += 1
            append_jsonl(trace_path, {"turn": turn, "phase": "step_error", "error": repr(exc)})
            break
        if next_obs is None:
            totals["null_steps"] += 1
            break

        next_grid = last_grid(next_obs)
        post_digest = grid_hash(next_grid)
        diff = diff_summary(grid, next_grid)
        changed_cells = int(diff["changed_cells"])
        no_op = changed_cells == 0
        if no_op:
            totals["no_ops"] += 1
        elif first_meaningful_change_turn is None:
            first_meaningful_change_turn = turn

        grid_outcome = {
            "before_grid_digest": pre_digest,
            "after_grid_digest": post_digest,
            "changed_cells": changed_cells,
            "changed_regions": diff["changed_regions"],
            "transition_counts": diff["transition_counts"],
            "persistent_change": post_digest != pre_digest,
        }
        tested_ids = {str(x) for x in proposal["experiment"]["tests_hypothesis_ids"]}
        hypotheses = [h for h in proposal["hypotheses"] if str(h["id"]) in tested_ids]
        try:
            outcome_prompt = build_outcome_prompt(
                turn=turn, experiment=proposal["experiment"], hypotheses=hypotheses,
                grid_outcome=grid_outcome, max_chars=args.outcome_prompt_max_chars,
            )
            raw_outcome, outcome_usage = call_llm(args.endpoint, outcome_prompt, max_tokens=args.outcome_max_tokens)
            interpretation = validate_outcome_interpretation(parse_json_object(raw_outcome), tested_ids)
            outcome = {
                "turn": turn,
                "experiment_id": f"turn-{turn}-experiment",
                "action": action_name,
                **grid_outcome,
                "observed_effect": interpretation["observed_effect"],
                "supports_hypothesis_ids": interpretation["supports_hypothesis_ids"],
                "contradicts_hypothesis_ids": interpretation["contradicts_hypothesis_ids"],
            }
            validate_outcome(outcome, args.contract, {str(h["id"]) for h in proposal["hypotheses"]})
            outcome_request = outcome_to_native_request(outcome, game_id)
            outcome_native = invoke_native(args.native_helper, outcome_request, str(db_path))
        except Exception as exc:
            totals["outcome_errors"] += 1
            append_jsonl(trace_path, {"turn": turn, "phase": "outcome_error", "error": repr(exc), "grid_outcome": grid_outcome})
            break

        totals["outcomes_recorded"] += 1
        totals["hypotheses_supported"] += len(outcome["supports_hypothesis_ids"])
        totals["hypotheses_contradicted"] += len(outcome["contradicts_hypothesis_ids"])
        receipt = outcome_native.get("reasoning_receipt") or {}
        totals["lyapunov_checks"] += int(bool(receipt.get("lyapunov_trajectory_executed")))
        totals["escape_considered"] += int(bool(receipt.get("escape_considered")))
        last_native = outcome_native
        snapshot_after = model_world_snapshot(args.modelworld_dump, modelworld_path)

        recent_outcomes.append({
            "turn": turn,
            "action": action_name,
            "action_params": action_data,
            "before_grid_digest": pre_digest,
            "after_grid_digest": post_digest,
            "changed_cells": changed_cells,
            "changed_regions": diff["changed_regions"][:12],
            "persistent_change": post_digest != pre_digest,
            "observed_effect": outcome["observed_effect"],
            "supports_hypothesis_ids": outcome["supports_hypothesis_ids"],
            "contradicts_hypothesis_ids": outcome["contradicts_hypothesis_ids"],
        })

        levels_after = int(getattr(next_obs, "levels_completed", 0) or 0)  # evaluator only
        evaluator_record = {
            "game_id": game_id,
            "state_before": getattr(getattr(obs, "state", None), "name", str(getattr(obs, "state", None))),
            "state_after": getattr(getattr(next_obs, "state", None), "name", str(getattr(next_obs, "state", None))),
            "levels_before": levels_before,
            "levels_after": levels_after,
        }
        append_jsonl(model_trace_path, {
            "turn": turn,
            "proposal_prompt_sha256": hashlib.sha256(proposal_prompt.encode()).hexdigest(),
            "proposal_prompt_chars": len(proposal_prompt),
            "proposal_raw": raw_proposal,
            "proposal_usage": proposal_usage,
            "outcome_prompt_sha256": hashlib.sha256(outcome_prompt.encode()).hexdigest(),
            "outcome_prompt_chars": len(outcome_prompt),
            "outcome_raw": raw_outcome,
            "outcome_usage": outcome_usage,
        })
        append_jsonl(trace_path, {
            "turn": turn,
            "observation": observation,
            "proposal": proposal,
            "native_proposal": native_response,
            "executed_action": {"action": action_name, "params": action_data},
            "grid_outcome": grid_outcome,
            "outcome_interpretation": interpretation,
            "native_outcome": outcome_native,
            "model_world_after": snapshot_after,
            "evaluator": evaluator_record,
        })
        obs = next_obs

    end_levels = int(getattr(obs, "levels_completed", 0) or 0)
    final_snapshot = model_world_snapshot(args.modelworld_dump, modelworld_path)
    return {
        "game_id": game_id,
        "actions": sum(action_counts.values()),
        "action_counts": dict(action_counts),
        "unique_actions": len(action_counts),
        "levels_start": start_levels,
        "levels_completed": end_levels,
        "levels_gained": end_levels - start_levels,
        "terminal_state": getattr(getattr(obs, "state", None), "name", str(getattr(obs, "state", None))),
        "first_meaningful_change_turn": first_meaningful_change_turn,
        "instrumentation": dict(totals),
        "distinct_hypotheses": len(distinct_hypotheses),
        "distinct_goals": len(distinct_goals),
        "model_world_nodes": len(final_snapshot.get("nodes") or []),
        "model_world_event_hash": int(final_snapshot.get("event_log_hash", 0)),
        "graphenedb_path": str(db_path),
    }


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--games", default="ft09,bp35")
    parser.add_argument("--max-actions", type=int, default=10)
    parser.add_argument("--endpoint", default="http://127.0.0.1:9090/v1/chat/completions")
    parser.add_argument("--out", required=True)
    parser.add_argument("--native-helper", required=True)
    parser.add_argument("--bootstrap", required=True)
    parser.add_argument("--modelworld-dump", required=True)
    parser.add_argument("--contract-path", default="pi45_a1/a15_state_contract.json")
    parser.add_argument("--proposal-prompt-max-chars", type=int, default=PROMPT_MAX_CHARS)
    parser.add_argument("--outcome-prompt-max-chars", type=int, default=OUTCOME_PROMPT_MAX_CHARS)
    parser.add_argument("--proposal-max-tokens", type=int, default=700)
    parser.add_argument("--outcome-max-tokens", type=int, default=320)
    args = parser.parse_args()
    args.contract = load_json(args.contract_path)

    from arc_agi import Arcade, OperationMode
    import arc_agi

    out_dir = Path(args.out)
    out_dir.mkdir(parents=True, exist_ok=True)
    requested = [x.strip() for x in args.games.split(",") if x.strip()]
    arc = Arcade(operation_mode=OperationMode.ONLINE)
    envs = arc.get_environments()
    available_ids = [e.game_id for e in envs]
    selected = []
    for prefix in requested:
        match = next((gid for gid in available_ids if gid == prefix or gid.startswith(prefix + "-")), None)
        if match:
            selected.append(match)
    if len(selected) != len(requested):
        raise RuntimeError(f"requested games not resolved: requested={requested} selected={selected}")

    scorecard_id = arc.create_scorecard(
        source_url=None,
        tags=["pi45a15", "dialectical-model-world", "qwen25-15b", "grid-only-perception"],
        opaque={"max_actions": args.max_actions, "perception_contract": "grid+opaque-actions-only"},
    )
    meta = {
        "pi": "PI4.5A1.5-diagnostic",
        "created_unix": time.time(),
        "arc_agi_version": getattr(arc_agi, "__version__", "unknown"),
        "model_id": MODEL_ID,
        "model_sha256": MODEL_SHA256,
        "temperature": TEMPERATURE,
        "seed": SEED,
        "max_actions_per_game": args.max_actions,
        "requested_games": requested,
        "selected_games": selected,
        "perception_contract": "grid+opaque-actions+grid-derived-outcomes-only",
        "evaluator_metadata_in_reasoning": False,
        "native_dialectical_runtime": True,
        "persistent_model_world": True,
    }
    (out_dir / "run_meta.json").write_text(json.dumps(meta, indent=2))

    results = [play_game(arc, gid, scorecard_id, args, out_dir) for gid in selected]
    scorecard = arc.close_scorecard(scorecard_id)
    score_data = scorecard.model_dump() if scorecard is not None and hasattr(scorecard, "model_dump") else None
    summary = {
        **meta,
        "games": results,
        "aggregate": {
            "actions": sum(int(g.get("actions", 0)) for g in results),
            "levels_gained": sum(int(g.get("levels_gained", 0)) for g in results),
            "no_ops": sum(int((g.get("instrumentation") or {}).get("no_ops", 0)) for g in results),
            "repeated_state_action": sum(int((g.get("instrumentation") or {}).get("repeated_state_action", 0)) for g in results),
            "proposal_errors": sum(int((g.get("instrumentation") or {}).get("proposal_errors", 0)) for g in results),
            "outcome_errors": sum(int((g.get("instrumentation") or {}).get("outcome_errors", 0)) for g in results),
            "native_errors": sum(int((g.get("instrumentation") or {}).get("native_errors", 0)) for g in results),
        },
        "scorecard": score_data,
    }
    (out_dir / "summary.json").write_text(json.dumps(summary, indent=2, default=str))
    print(json.dumps(summary, indent=2, default=str))


if __name__ == "__main__":
    main()
