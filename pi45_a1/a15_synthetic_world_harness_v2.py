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
_STATE: dict[str, Any] = {
    "last_proposal_prompt": None,
    "last_proposal_raw": None,
    "last_proposal_usage": None,
    "last_endpoint": None,
    "last_max_tokens": None,
    "proposal_repairs_attempted": 0,
    "proposal_repairs_succeeded": 0,
    "repair_log_path": None,
}


def _append_repair_record(record: dict[str, Any]) -> None:
    path = _STATE.get("repair_log_path")
    if not path:
        return
    p = Path(path)
    p.parent.mkdir(parents=True, exist_ok=True)
    with p.open("a", encoding="utf-8") as handle:
        handle.write(json.dumps(record, sort_keys=True, default=str) + "\n")


def tracked_call_llm(endpoint: str, prompt: str, *, max_tokens: int, timeout: int = 360):
    raw, usage = _ORIGINAL_CALL_LLM(endpoint, prompt, max_tokens=max_tokens, timeout=timeout)
    # Proposal prompts are the only model calls with CURRENT EVIDENCE and the
    # candidate-goal contract. Outcome and repair prompts have different markers.
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
            # Preserve the repaired model decision exactly; do not auto-fill or
            # synthesize any linkage in code.
            proposal.clear()
            proposal.update(repaired)
            _STATE["proposal_repairs_succeeded"] += 1
            record["repair_succeeded"] = True
            _append_repair_record(record)
            return
        except Exception as repair_error:
            record["repair_succeeded"] = False
            record["repair_validation_error"] = repr(repair_error)
            _append_repair_record(record)
            raise


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
    })
    base.call_llm = tracked_call_llm
    base.validate_turn = validate_turn_with_one_repair


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
    summary["proposal_repair_policy"] = "max_one_same-model-validation-retry"
    (out_dir / "summary.json").write_text(
        json.dumps(summary, indent=2, sort_keys=True) + "\n", encoding="utf-8"
    )

    base.assert_harness_gates(summary)
    attempted = int(inst.get("proposal_repairs_attempted", 0))
    succeeded = int(inst.get("proposal_repairs_succeeded", 0))
    if attempted > int(summary["turns_executed"]) or succeeded != attempted:
        raise AssertionError("bounded proposal repair contract violated")

    print("a15_synthetic_model_in_loop_harness=PASS")
    print(json.dumps(summary, indent=2, sort_keys=True))


if __name__ == "__main__":
    main()
