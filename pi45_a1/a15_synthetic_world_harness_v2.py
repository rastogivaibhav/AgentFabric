#!/usr/bin/env python3
from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path
from typing import Any

import a15_synthetic_world_harness as base
from a15_proposal_prompt import build_proposal_repair_prompt, parse_json_object


_ORIGINAL_CALL_LLM = base.call_llm
_ORIGINAL_VALIDATE_TURN = base.validate_turn
_ORIGINAL_INVOKE_NATIVE = base.invoke_native
_ORIGINAL_GOVERNED_CONTEXT = base.governed_context
_STATE: dict[str, Any] = {
    "last_proposal_prompt": None,
    "last_proposal_raw": None,
    "last_proposal_usage": None,
    "last_endpoint": None,
    "last_max_tokens": None,
    "proposal_repairs_attempted": 0,
    "proposal_repairs_succeeded": 0,
    "repair_log_path": None,
    "controller_log_path": None,
    "native_opposition_rounds": 0,
    "native_reopen_events": 0,
    "alternatives_reopened": 0,
    "native_primary_hypothesis_selections": 0,
    "reopened_ids": set(),
}


def _append_jsonl(path_value: str | None, record: dict[str, Any]) -> None:
    if not path_value:
        return
    path = Path(path_value)
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("a", encoding="utf-8") as handle:
        handle.write(json.dumps(record, sort_keys=True, default=str) + "\n")


def tracked_call_llm(endpoint: str, prompt: str, *, max_tokens: int, timeout: int = 360):
    raw, usage = _ORIGINAL_CALL_LLM(endpoint, prompt, max_tokens=max_tokens, timeout=timeout)
    if "\n\nCURRENT EVIDENCE:\n" in prompt and '"candidate_goals"' in prompt:
        _STATE["last_proposal_prompt"] = prompt
        _STATE["last_proposal_raw"] = raw
        _STATE["last_proposal_usage"] = usage
        _STATE["last_endpoint"] = endpoint
        _STATE["last_max_tokens"] = max_tokens
    return raw, usage


def validate_turn_with_one_repair(proposal: dict[str, Any], contract: dict[str, Any], available_actions: set[str]) -> None:
    try:
        _ORIGINAL_VALIDATE_TURN(proposal, contract, available_actions)
        return
    except Exception as first_error:
        original_prompt = _STATE.get("last_proposal_prompt")
        original_raw = _STATE.get("last_proposal_raw")
        endpoint = _STATE.get("last_endpoint")
        max_tokens = _STATE.get("last_max_tokens")
        if not original_prompt or original_raw is None or not endpoint or not max_tokens:
            raise

        turn = int(proposal.get("turn", -1))
        _STATE["proposal_repairs_attempted"] += 1
        record: dict[str, Any] = {
            "turn": turn,
            "initial_raw": original_raw,
            "initial_usage": _STATE.get("last_proposal_usage") or {},
            "initial_validation_error": repr(first_error),
            "repair_attempted": True,
            "repair_raw": None,
            "repair_usage": {},
            "repair_validation_error": None,
        }
        try:
            repair_prompt = build_proposal_repair_prompt(
                original_prompt=original_prompt,
                invalid_output=original_raw,
                validation_error=str(first_error),
                turn=turn,
                available_actions=sorted(str(a) for a in available_actions),
                max_chars=11500,
            )
            repaired_raw, repaired_usage = _ORIGINAL_CALL_LLM(
                endpoint, repair_prompt, max_tokens=int(max_tokens)
            )
            record["repair_prompt_sha256"] = hashlib.sha256(repair_prompt.encode()).hexdigest()
            record["repair_prompt_chars"] = len(repair_prompt)
            record["repair_raw"] = repaired_raw
            record["repair_usage"] = repaired_usage
            repaired = parse_json_object(repaired_raw)
            _ORIGINAL_VALIDATE_TURN(repaired, contract, available_actions)
            proposal.clear()
            proposal.update(repaired)
            _STATE["proposal_repairs_succeeded"] += 1
            record["repair_succeeded"] = True
            _append_jsonl(_STATE.get("repair_log_path"), record)
            return
        except Exception as repair_error:
            record["repair_succeeded"] = False
            record["repair_validation_error"] = repr(repair_error)
            _append_jsonl(_STATE.get("repair_log_path"), record)
            raise


def tracked_invoke_native(helper: str, request: dict[str, Any], db_path: str) -> dict[str, Any]:
    operation = str(request.get("operation"))
    if operation == "ingest_and_reason":
        for forbidden in [
            "provisional_hypothesis_id", "provisional_goal_id", "reopen_hypothesis_ids",
        ]:
            if forbidden in request:
                raise AssertionError(f"LLM/controller seed leaked into native request: {forbidden}")
    response = _ORIGINAL_INVOKE_NATIVE(helper, request, db_path)
    if operation == "ingest_and_reason":
        _STATE["native_opposition_rounds"] += 1
        reopened = [str(x) for x in response.get("reopened_hypothesis_ids") or [] if str(x)]
        if reopened:
            _STATE["native_reopen_events"] += 1
            _STATE["alternatives_reopened"] += len(reopened)
            _STATE["reopened_ids"].update(reopened)
        if str(response.get("primary_hypothesis_id") or ""):
            _STATE["native_primary_hypothesis_selections"] += 1
        _append_jsonl(_STATE.get("controller_log_path"), {
            "turn": request.get("turn"),
            "candidate_hypothesis_ids": list(request.get("candidate_hypothesis_ids") or []),
            "primary_hypothesis_id": response.get("primary_hypothesis_id"),
            "challenged_claims": list(response.get("challenged_claims") or []),
            "native_falsification_questions": list(response.get("native_falsification_questions") or []),
            "reopened_hypothesis_ids": reopened,
            "opposition_score": response.get("opposition_score"),
            "native_reopen_decision_source": response.get("native_reopen_decision_source"),
            "reasoning_receipt": response.get("reasoning_receipt") or {},
        })
    return response


def governed_context_with_native_controller(native_response: dict[str, Any], snapshot: dict[str, Any]) -> dict[str, Any]:
    context = _ORIGINAL_GOVERNED_CONTEXT(native_response, snapshot)
    context.update({
        "primary_hypothesis_id": native_response.get("primary_hypothesis_id"),
        "reopened_hypothesis_ids": list(native_response.get("reopened_hypothesis_ids") or []),
        "challenged_claims": list(native_response.get("challenged_claims") or [])[-6:],
        "native_falsification_questions": list(native_response.get("native_falsification_questions") or [])[-6:],
    })
    return context


def _configure_patches(out_dir: Path) -> None:
    _STATE.update({
        "last_proposal_prompt": None,
        "last_proposal_raw": None,
        "last_proposal_usage": None,
        "last_endpoint": None,
        "last_max_tokens": None,
        "proposal_repairs_attempted": 0,
        "proposal_repairs_succeeded": 0,
        "repair_log_path": str(out_dir / "synthetic.a15.proposal_repairs.jsonl"),
        "controller_log_path": str(out_dir / "synthetic.a15.native_controller.jsonl"),
        "native_opposition_rounds": 0,
        "native_reopen_events": 0,
        "alternatives_reopened": 0,
        "native_primary_hypothesis_selections": 0,
        "reopened_ids": set(),
    })
    base.call_llm = tracked_call_llm
    base.validate_turn = validate_turn_with_one_repair
    base.invoke_native = tracked_invoke_native
    base.governed_context = governed_context_with_native_controller


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

    args.contract = base.load_json(args.contract_path)
    out_dir = Path(args.out)
    out_dir.mkdir(parents=True, exist_ok=True)
    _configure_patches(out_dir)

    summary = base.run_harness(args)
    inst = summary.setdefault("instrumentation", {})
    inst["proposal_repairs_attempted"] = int(_STATE["proposal_repairs_attempted"])
    inst["proposal_repairs_succeeded"] = int(_STATE["proposal_repairs_succeeded"])
    inst["native_opposition_rounds"] = int(_STATE["native_opposition_rounds"])
    inst["native_reopen_events"] = int(_STATE["native_reopen_events"])
    inst["alternatives_reopened"] = int(_STATE["alternatives_reopened"])
    inst["native_primary_hypothesis_selections"] = int(_STATE["native_primary_hypothesis_selections"])
    summary["native_reopened_hypothesis_ids"] = sorted(_STATE["reopened_ids"])
    summary["proposal_repair_policy"] = "max_one_same-model-validation-retry"
    summary["dialectic_control_owner"] = "CompleteHypoKoshRuntime"
    (out_dir / "summary.json").write_text(
        json.dumps(summary, indent=2, sort_keys=True) + "\n", encoding="utf-8"
    )

    base.assert_harness_gates(summary)
    turns = int(summary["turns_executed"])
    attempted = int(inst.get("proposal_repairs_attempted", 0))
    succeeded = int(inst.get("proposal_repairs_succeeded", 0))
    if attempted > turns or succeeded != attempted:
        raise AssertionError("bounded proposal repair contract violated")
    if int(inst.get("native_opposition_rounds", 0)) != turns:
        raise AssertionError("native opposition must govern every proposal turn")
    if int(inst.get("native_reopen_events", 0)) < 1:
        raise AssertionError("native controller never reopened a competing hypothesis")
    if int(inst.get("alternatives_reopened", 0)) < 1:
        raise AssertionError("no alternative hypothesis survived/reopened through native opposition")
    if summary.get("dialectic_control_owner") != "CompleteHypoKoshRuntime":
        raise AssertionError("dialectic control ownership drifted away from native runtime")

    print("a15_synthetic_model_in_loop_harness=PASS")
    print(json.dumps(summary, indent=2, sort_keys=True))


if __name__ == "__main__":
    main()
