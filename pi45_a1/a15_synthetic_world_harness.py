#!/usr/bin/env python3
from __future__ import annotations

import argparse
import copy
import hashlib
import json
import os
import subprocess
import urllib.request
from collections import Counter, deque
from pathlib import Path
from typing import Any, Iterable

from a15_arc_dialectic_adapter import invoke_native, outcome_to_native_request, proposal_to_native_request
from a15_contract_gate import ContractError, load_json, validate_outcome, validate_turn
from a15_outcome_prompt import (
    build_outcome_prompt,
    build_outcome_repair_prompt,
    validate_outcome_interpretation,
)
from a15_proposal_prompt import build_proposal_prompt, parse_json_object

TEMPERATURE = 0
SEED = 4242
OPAQUE_WORLD_ID = "synthetic-evaluator-only"


def grid_hash(grid: list[list[int]]) -> str:
    return hashlib.sha256(json.dumps(grid, separators=(",", ":")).encode()).hexdigest()


def rle_row(row: Iterable[int]) -> str:
    values = list(row)
    if not values:
        return ""
    out: list[str] = []
    current, count = values[0], 1
    for value in values[1:]:
        if value == current:
            count += 1
        else:
            out.append(f"{current}x{count}")
            current, count = value, 1
    out.append(f"{current}x{count}")
    return " ".join(out)


def compact_grid(grid: list[list[int]]) -> str:
    counts = Counter(v for row in grid for v in row)
    rows = "\n".join(f"{idx:02d}:{rle_row(row)}" for idx, row in enumerate(grid))
    return f"size={len(grid[0])}x{len(grid)};palette={dict(sorted(counts.items()))}\n{rows}"


def diff_summary(before: list[list[int]], after: list[list[int]], cap: int = 32) -> dict[str, Any]:
    sample: list[dict[str, int]] = []
    transitions: Counter[tuple[int, int]] = Counter()
    for y, (row_before, row_after) in enumerate(zip(before, after)):
        for x, (a, b) in enumerate(zip(row_before, row_after)):
            if a != b:
                transitions[(a, b)] += 1
                if len(sample) < cap:
                    sample.append({"x": x, "y": y, "before": a, "after": b})
    return {
        "changed_cells": sum(transitions.values()),
        "changed_regions": sample,
        "transition_counts": {f"{a}->{b}": n for (a, b), n in sorted(transitions.items())},
    }


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
    request = urllib.request.Request(
        endpoint,
        data=json.dumps(body).encode(),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    with urllib.request.urlopen(request, timeout=timeout) as response:
        data = json.loads(response.read().decode())
    return data["choices"][0]["message"].get("content") or "", data.get("usage") or {}


def append_jsonl(path: Path, value: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("a", encoding="utf-8") as handle:
        handle.write(json.dumps(value, sort_keys=True, default=str) + "\n")


def model_world_snapshot(dumper: str, modelworld_path: Path) -> dict[str, Any]:
    if not modelworld_path.exists():
        return {"event_log_hash": 0, "nodes": []}
    proc = subprocess.run([dumper, str(modelworld_path)], check=True, capture_output=True, text=True, timeout=30)
    data = json.loads(proc.stdout)
    return {
        "event_log_hash": int(data.get("event_log_hash", 0)),
        "nodes": list(data.get("nodes") or [])[-20:],
    }


def governed_context(native_response: dict[str, Any], snapshot: dict[str, Any]) -> dict[str, Any]:
    receipt = native_response.get("reasoning_receipt") or {}
    return {
        "status": native_response.get("epistemic_status"),
        "residual_uncertainty": list(native_response.get("residual_uncertainty") or [])[-8:],
        "lyapunov_goal_reached": bool(receipt.get("lyapunov_goal_reached", False)),
        "model_world_nodes": [
            {
                "external_id": node.get("external_id"),
                "type": node.get("type"),
                "status": node.get("status"),
                "statement": node.get("statement"),
            }
            for node in (snapshot.get("nodes") or [])
        ],
    }


class HiddenSyntheticWorld:
    """Small deterministic evaluator-only world.

    The reasoner sees only the integer grid and three opaque action identifiers.
    Hidden transition rules exist only in this evaluator object and in the
    separate evaluator manifest written after the run.
    """

    ACTIONS = ["ACTION1", "ACTION2", "ACTION3"]

    def __init__(self) -> None:
        self.phase = 0
        self.grid = [
            [0, 0, 0, 0, 0, 0],
            [0, 1, 1, 0, 0, 0],
            [0, 1, 1, 0, 0, 0],
            [0, 0, 0, 4, 0, 0],
            [0, 0, 0, 0, 0, 0],
            [0, 0, 0, 0, 0, 0],
        ]

    def observation(self) -> list[list[int]]:
        return copy.deepcopy(self.grid)

    def step(self, action: str) -> list[list[int]]:
        if action not in self.ACTIONS:
            raise ValueError(f"unknown opaque action {action}")
        if action == "ACTION2" and self.phase == 0:
            for y in (1, 2):
                for x in (1, 2):
                    self.grid[y][x] = 2
            self.phase = 1
        elif action == "ACTION3" and self.phase == 1:
            self.grid[3][3] = 3
            self.phase = 2
        return self.observation()

    def evaluator_state(self) -> dict[str, Any]:
        return {"hidden_phase": self.phase, "hidden_completion": self.phase == 2}


def validate_synthetic_action(proposal: dict[str, Any], available: list[str]) -> str:
    experiment = proposal["experiment"]
    action = str(experiment["action"])
    if action not in available:
        raise ContractError(f"synthetic harness action {action!r} unavailable")
    if dict(experiment.get("action_params") or {}):
        raise ContractError("synthetic opaque actions do not accept parameters")
    return action


def interpret_outcome_with_one_repair(
    args: argparse.Namespace,
    *,
    outcome_prompt: str,
    tested_ids: set[str],
    totals: Counter,
) -> tuple[dict[str, Any], dict[str, Any]]:
    """Run strict interpretation with at most one model self-repair.

    The first invalid answer is not silently normalized. Its raw response and
    validator error are retained in the returned evidence record. The repair
    call receives no new world evidence and must pass the identical validator.
    """
    raw, usage = call_llm(args.endpoint, outcome_prompt, max_tokens=args.outcome_max_tokens)
    evidence: dict[str, Any] = {
        "initial_raw": raw,
        "initial_usage": usage,
        "repair_attempted": False,
        "repair_raw": None,
        "repair_usage": {},
        "initial_validation_error": None,
    }
    try:
        interpreted = validate_outcome_interpretation(parse_json_object(raw), tested_ids)
        return interpreted, evidence
    except Exception as first_error:
        totals["outcome_repairs_attempted"] += 1
        evidence["repair_attempted"] = True
        evidence["initial_validation_error"] = repr(first_error)
        repair_prompt = build_outcome_repair_prompt(
            original_prompt=outcome_prompt,
            invalid_output=raw,
            validation_error=str(first_error),
            tested_ids=tested_ids,
            max_chars=args.outcome_repair_prompt_max_chars,
        )
        repaired_raw, repaired_usage = call_llm(
            args.endpoint, repair_prompt, max_tokens=args.outcome_max_tokens
        )
        evidence["repair_prompt_sha256"] = hashlib.sha256(repair_prompt.encode()).hexdigest()
        evidence["repair_prompt_chars"] = len(repair_prompt)
        evidence["repair_raw"] = repaired_raw
        evidence["repair_usage"] = repaired_usage
        interpreted = validate_outcome_interpretation(parse_json_object(repaired_raw), tested_ids)
        totals["outcome_repairs_succeeded"] += 1
        return interpreted, evidence


def run_harness(args: argparse.Namespace) -> dict[str, Any]:
    world = HiddenSyntheticWorld()
    out = Path(args.out)
    out.mkdir(parents=True, exist_ok=True)
    db_path = out / "synthetic.a15.graphenedb"
    modelworld_path = Path(str(db_path) + ".modelworld")
    subprocess.run([args.bootstrap, str(db_path)], check=True, capture_output=True, text=True, timeout=30)

    turns_path = out / "synthetic.a15.turns.jsonl"
    model_calls_path = out / "synthetic.a15.model_calls.jsonl"
    recent_outcomes: deque[dict[str, Any]] = deque(maxlen=4)
    last_native: dict[str, Any] = {
        "epistemic_status": "uninitialized",
        "reasoning_receipt": {},
        "residual_uncertainty": [],
    }
    action_counts: Counter[str] = Counter()
    state_action_counts: Counter[tuple[str, str]] = Counter()
    totals: Counter = Counter()
    distinct_hypotheses: set[str] = set()
    distinct_goals: set[str] = set()
    first_meaningful_change_turn: int | None = None
    phase_one_turn: int | None = None
    phase_two_turn: int | None = None
    previous_noop: tuple[str, str] | None = None
    max_same_state_action_trials = 0
    prompt_chars: list[int] = []
    outcome_prompt_chars: list[int] = []

    for turn in range(args.max_turns):
        if world.phase == 2 and turn >= args.min_turns:
            break

        grid = world.observation()
        before_digest = grid_hash(grid)
        available = list(HiddenSyntheticWorld.ACTIONS)
        snapshot_before = model_world_snapshot(args.modelworld_dump, modelworld_path)
        observation = {
            "grid_digest": before_digest,
            "grid_rle": compact_grid(grid),
            "action_affordances": {action: {"parameters": {}} for action in available},
        }

        try:
            proposal_prompt = build_proposal_prompt(
                turn=turn,
                game_id=OPAQUE_WORLD_ID,
                observation=observation,
                available_actions=available,
                recent_outcomes=list(recent_outcomes),
                governed_context=governed_context(last_native, snapshot_before),
                max_chars=args.proposal_prompt_max_chars,
            )
            raw_proposal, proposal_usage = call_llm(
                args.endpoint, proposal_prompt, max_tokens=args.proposal_max_tokens
            )
            proposal = parse_json_object(raw_proposal)
            validate_turn(proposal, args.contract, set(available))
            action = validate_synthetic_action(proposal, available)
        except Exception as exc:
            totals["proposal_errors"] += 1
            append_jsonl(turns_path, {
                "turn": turn,
                "phase": "proposal_error",
                "error": repr(exc),
                "proposal_raw": locals().get("raw_proposal"),
            })
            break

        totals["hypotheses_generated"] += len(proposal["hypotheses"])
        totals["candidate_goals_generated"] += len(proposal["candidate_goals"])
        totals["opposition_rounds"] += 1
        distinct_hypotheses.update(str(h["id"]) for h in proposal["hypotheses"])
        distinct_goals.update(str(g["id"]) for g in proposal["candidate_goals"])

        try:
            native_proposal = invoke_native(
                args.native_helper,
                proposal_to_native_request(proposal, OPAQUE_WORLD_ID),
                str(db_path),
            )
        except Exception as exc:
            totals["native_errors"] += 1
            append_jsonl(turns_path, {"turn": turn, "phase": "native_proposal_error", "error": repr(exc)})
            break
        if not native_proposal.get("action_authorized") or native_proposal.get("governed_action") != action:
            totals["governance_denials"] += 1
            append_jsonl(turns_path, {"turn": turn, "phase": "governance_denied", "native": native_proposal})
            break

        proposal_receipt = native_proposal.get("reasoning_receipt") or {}
        totals["proposal_lyapunov_checks"] += int(bool(proposal_receipt.get("lyapunov_trajectory_executed")))
        totals["proposal_escape_considered"] += int(bool(proposal_receipt.get("escape_considered")))

        action_counts[action] += 1
        state_action_key = (before_digest, action)
        state_action_counts[state_action_key] += 1
        max_same_state_action_trials = max(max_same_state_action_trials, state_action_counts[state_action_key])
        if state_action_counts[state_action_key] > 1:
            totals["repeated_state_action"] += 1
        if previous_noop == state_action_key:
            totals["immediate_noop_repeats"] += 1

        hidden_before = world.evaluator_state()
        next_grid = world.step(action)
        hidden_after = world.evaluator_state()
        after_digest = grid_hash(next_grid)
        diff = diff_summary(grid, next_grid)
        changed_cells = int(diff["changed_cells"])
        if changed_cells == 0:
            totals["no_ops"] += 1
            previous_noop = state_action_key
        else:
            totals["meaningful_changes"] += 1
            previous_noop = None
            if first_meaningful_change_turn is None:
                first_meaningful_change_turn = turn
        if hidden_before["hidden_phase"] == 0 and hidden_after["hidden_phase"] == 1 and phase_one_turn is None:
            phase_one_turn = turn
        if hidden_before["hidden_phase"] == 1 and hidden_after["hidden_phase"] == 2 and phase_two_turn is None:
            phase_two_turn = turn

        grid_outcome = {
            "before_grid_digest": before_digest,
            "after_grid_digest": after_digest,
            "changed_cells": changed_cells,
            "changed_regions": diff["changed_regions"],
            "transition_counts": diff["transition_counts"],
            "persistent_change": after_digest != before_digest,
        }
        tested_ids = {str(v) for v in proposal["experiment"]["tests_hypothesis_ids"]}
        tested_hypotheses = [h for h in proposal["hypotheses"] if str(h["id"]) in tested_ids]
        outcome_model_evidence: dict[str, Any] = {}
        try:
            outcome_prompt = build_outcome_prompt(
                turn=turn,
                experiment=proposal["experiment"],
                hypotheses=tested_hypotheses,
                grid_outcome=grid_outcome,
                max_chars=args.outcome_prompt_max_chars,
            )
            interpretation, outcome_model_evidence = interpret_outcome_with_one_repair(
                args,
                outcome_prompt=outcome_prompt,
                tested_ids=tested_ids,
                totals=totals,
            )
            outcome = {
                "turn": turn,
                "experiment_id": f"turn-{turn}-experiment",
                "action": action,
                **grid_outcome,
                "observed_effect": interpretation["observed_effect"],
                "supports_hypothesis_ids": interpretation["supports_hypothesis_ids"],
                "contradicts_hypothesis_ids": interpretation["contradicts_hypothesis_ids"],
            }
            validate_outcome(outcome, args.contract, {str(h["id"]) for h in proposal["hypotheses"]})
            native_outcome = invoke_native(
                args.native_helper,
                outcome_to_native_request(outcome, OPAQUE_WORLD_ID),
                str(db_path),
            )
        except Exception as exc:
            totals["outcome_errors"] += 1
            append_jsonl(model_calls_path, {
                "turn": turn,
                "proposal_prompt_sha256": hashlib.sha256(proposal_prompt.encode()).hexdigest(),
                "proposal_raw": raw_proposal,
                "proposal_usage": proposal_usage,
                "outcome_prompt_sha256": hashlib.sha256(locals().get("outcome_prompt", "").encode()).hexdigest(),
                "outcome_model_evidence": outcome_model_evidence,
            })
            append_jsonl(turns_path, {
                "turn": turn,
                "phase": "outcome_error",
                "error": repr(exc),
                "grid_outcome": grid_outcome,
                "outcome_model_evidence": outcome_model_evidence,
            })
            break

        totals["outcomes_recorded"] += 1
        totals["hypotheses_supported"] += len(outcome["supports_hypothesis_ids"])
        totals["hypotheses_contradicted"] += len(outcome["contradicts_hypothesis_ids"])
        outcome_receipt = native_outcome.get("reasoning_receipt") or {}
        totals["outcome_lyapunov_checks"] += int(bool(outcome_receipt.get("lyapunov_trajectory_executed")))
        totals["outcome_escape_considered"] += int(bool(outcome_receipt.get("escape_considered")))
        last_native = native_outcome
        snapshot_after = model_world_snapshot(args.modelworld_dump, modelworld_path)

        recent_outcomes.append({
            "turn": turn,
            "action": action,
            "action_params": {},
            "before_grid_digest": before_digest,
            "after_grid_digest": after_digest,
            "changed_cells": changed_cells,
            "changed_regions": diff["changed_regions"][:12],
            "persistent_change": after_digest != before_digest,
            "observed_effect": outcome["observed_effect"],
            "supports_hypothesis_ids": outcome["supports_hypothesis_ids"],
            "contradicts_hypothesis_ids": outcome["contradicts_hypothesis_ids"],
        })
        prompt_chars.append(len(proposal_prompt))
        outcome_prompt_chars.append(len(outcome_prompt))
        append_jsonl(model_calls_path, {
            "turn": turn,
            "proposal_prompt_sha256": hashlib.sha256(proposal_prompt.encode()).hexdigest(),
            "proposal_prompt_chars": len(proposal_prompt),
            "proposal_raw": raw_proposal,
            "proposal_usage": proposal_usage,
            "outcome_prompt_sha256": hashlib.sha256(outcome_prompt.encode()).hexdigest(),
            "outcome_prompt_chars": len(outcome_prompt),
            "outcome_model_evidence": outcome_model_evidence,
        })
        append_jsonl(turns_path, {
            "turn": turn,
            "observation": observation,
            "proposal": proposal,
            "native_proposal": native_proposal,
            "executed_action": {"action": action, "params": {}},
            "grid_outcome": grid_outcome,
            "outcome_interpretation": interpretation,
            "native_outcome": native_outcome,
            "model_world_after": snapshot_after,
            "evaluator": {"hidden_before": hidden_before, "hidden_after": hidden_after},
        })

    final_snapshot = model_world_snapshot(args.modelworld_dump, modelworld_path)
    actions = sum(action_counts.values())
    summary = {
        "harness": "A1.5 model-in-loop synthetic dialectical world",
        "turns_executed": actions,
        "min_turns": args.min_turns,
        "max_turns": args.max_turns,
        "action_counts": dict(action_counts),
        "unique_actions": len(action_counts),
        "no_ops": int(totals.get("no_ops", 0)),
        "meaningful_changes": int(totals.get("meaningful_changes", 0)),
        "first_meaningful_change_turn": first_meaningful_change_turn,
        "phase_one_turn": phase_one_turn,
        "phase_two_turn": phase_two_turn,
        "hidden_final_phase": world.phase,
        "hidden_completion": world.phase == 2,
        "distinct_hypotheses": len(distinct_hypotheses),
        "distinct_goals": len(distinct_goals),
        "model_world_nodes": len(final_snapshot.get("nodes") or []),
        "model_world_event_hash": int(final_snapshot.get("event_log_hash", 0)),
        "max_same_state_action_trials": max_same_state_action_trials,
        "instrumentation": dict(totals),
        "proposal_prompt_max_chars_observed": max(prompt_chars, default=0),
        "outcome_prompt_max_chars_observed": max(outcome_prompt_chars, default=0),
        "evaluator_metadata_in_reasoning": False,
        "arc_environment_used": False,
        "native_dialectical_runtime": True,
        "persistent_model_world": True,
    }
    (out / "summary.json").write_text(json.dumps(summary, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    (out / "evaluator_world_manifest.json").write_text(json.dumps({
        "evaluator_only": True,
        "initial_grid": HiddenSyntheticWorld().observation(),
        "opaque_actions": HiddenSyntheticWorld.ACTIONS,
        "hidden_rules": {
            "ACTION1": "always no-op",
            "ACTION2": "phase 0 transforms the 2x2 region and advances to phase 1; otherwise no-op",
            "ACTION3": "phase 1 transforms the marker and advances to phase 2; otherwise no-op",
        },
    }, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    return summary


def assert_harness_gates(summary: dict[str, Any]) -> None:
    inst = summary.get("instrumentation") or {}
    for key in ["proposal_errors", "native_errors", "governance_denials", "outcome_errors"]:
        if int(inst.get(key, 0)) != 0:
            raise AssertionError(f"{key}={inst.get(key)}")
    turns = int(summary["turns_executed"])
    if turns < int(summary["min_turns"]):
        raise AssertionError(f"insufficient turns: {turns}")
    if int(inst.get("outcomes_recorded", 0)) != turns:
        raise AssertionError("every experiment must have an outcome")
    if int(summary["distinct_hypotheses"]) < 2 * turns:
        raise AssertionError("each turn must preserve at least two competing hypotheses")
    if int(summary["distinct_goals"]) < turns:
        raise AssertionError("each turn must derive at least one candidate goal")
    if int(inst.get("opposition_rounds", 0)) != turns:
        raise AssertionError("opposition must execute on every proposal turn")
    if int(inst.get("proposal_lyapunov_checks", 0)) != turns or int(inst.get("outcome_lyapunov_checks", 0)) != turns:
        raise AssertionError("Lyapunov trajectory must execute on proposal and outcome reasoning")
    if int(inst.get("proposal_escape_considered", 0)) != turns or int(inst.get("outcome_escape_considered", 0)) != turns:
        raise AssertionError("escape must be considered on proposal and outcome reasoning")
    attempted = int(inst.get("outcome_repairs_attempted", 0))
    succeeded = int(inst.get("outcome_repairs_succeeded", 0))
    if attempted > turns or succeeded != attempted:
        raise AssertionError("bounded outcome repair contract violated")
    if int(summary["model_world_nodes"]) <= 0 or int(summary["model_world_event_hash"]) == 0:
        raise AssertionError("persistent ModelWorld evidence missing")
    if int(summary["unique_actions"]) < 2:
        raise AssertionError("agent did not adapt its experiment/action choice")
    if int(summary["meaningful_changes"]) < 1:
        raise AssertionError("agent discovered no state-changing interaction")
    if int(summary["max_same_state_action_trials"]) > 2:
        raise AssertionError("agent remained in a repeated same-state/same-action attractor")
    if int(inst.get("hypotheses_supported", 0)) + int(inst.get("hypotheses_contradicted", 0)) <= 0:
        raise AssertionError("observable outcomes never updated belief support/contradiction")
    if summary.get("arc_environment_used") is not False:
        raise AssertionError("synthetic harness must not use live target-game execution")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--endpoint", default="http://127.0.0.1:9090/v1/chat/completions")
    parser.add_argument("--out", required=True)
    parser.add_argument("--native-helper", required=True)
    parser.add_argument("--bootstrap", required=True)
    parser.add_argument("--modelworld-dump", required=True)
    parser.add_argument("--contract-path", default="pi45_a1/a15_state_contract.json")
    parser.add_argument("--min-turns", type=int, default=4)
    parser.add_argument("--max-turns", type=int, default=8)
    parser.add_argument("--proposal-prompt-max-chars", type=int, default=9000)
    parser.add_argument("--outcome-prompt-max-chars", type=int, default=6000)
    parser.add_argument("--outcome-repair-prompt-max-chars", type=int, default=9000)
    parser.add_argument("--proposal-max-tokens", type=int, default=700)
    parser.add_argument("--outcome-max-tokens", type=int, default=320)
    args = parser.parse_args()
    if args.min_turns < 1 or args.max_turns < args.min_turns:
        raise SystemExit("invalid turn bounds")
    args.contract = load_json(args.contract_path)
    summary = run_harness(args)
    assert_harness_gates(summary)
    print("a15_synthetic_model_in_loop_harness=PASS")
    print(json.dumps(summary, indent=2, sort_keys=True))


if __name__ == "__main__":
    main()
