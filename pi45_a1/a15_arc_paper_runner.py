#!/usr/bin/env python3
from __future__ import annotations

import hashlib
import json
import sys
from pathlib import Path
from typing import Any

import a15_arc_diagnostic_runner as base
from a15_outcome_prompt import build_outcome_repair_prompt
from a15_proposal_prompt import (
    build_proposal_repair_prompt,
    extract_prompt_context,
    parse_and_canonicalize_model_proposal,
)


_ORIGINAL_CALL_LLM = base.call_llm
_ORIGINAL_PARSE_JSON = base.parse_json_object
_ORIGINAL_VALIDATE_TURN = base.validate_turn
_ORIGINAL_VALIDATE_OUTCOME = base.validate_outcome_interpretation
_ORIGINAL_INVOKE_NATIVE = base.invoke_native
_ORIGINAL_GOVERNED_CONTEXT = base.governed_context

_STATE: dict[str, Any] = {
    "last_proposal_prompt": None,
    "last_proposal_raw": None,
    "last_proposal_usage": {},
    "last_outcome_prompt": None,
    "last_outcome_raw": None,
    "last_outcome_usage": {},
    "last_endpoint": None,
    "last_proposal_max_tokens": None,
    "last_outcome_max_tokens": None,
    "proposal_repairs_attempted": 0,
    "proposal_repairs_succeeded": 0,
    "outcome_repairs_attempted": 0,
    "outcome_repairs_succeeded": 0,
    "native_opposition_rounds": 0,
    "native_reopen_events": 0,
    "alternatives_reopened": 0,
    "native_primary_hypothesis_selections": 0,
    "native_controller_log": None,
    "proposal_repair_log": None,
    "outcome_repair_log": None,
}


def _append_jsonl(path_value: str | None, row: dict[str, Any]) -> None:
    if not path_value:
        return
    path = Path(path_value)
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("a", encoding="utf-8") as handle:
        handle.write(json.dumps(row, sort_keys=True, default=str) + "\n")


def tracked_call_llm(endpoint: str, prompt: str, *, max_tokens: int, timeout: int = 360):
    raw, usage = _ORIGINAL_CALL_LLM(endpoint, prompt, max_tokens=max_tokens, timeout=timeout)
    _STATE["last_endpoint"] = endpoint
    if "\n\nCURRENT EVIDENCE:\n" in prompt and "COMPACT FIELD CONTRACT" in prompt:
        _STATE["last_proposal_prompt"] = prompt
        _STATE["last_proposal_raw"] = raw
        _STATE["last_proposal_usage"] = usage
        _STATE["last_proposal_max_tokens"] = int(max_tokens)
    elif "\n\nEVIDENCE:\n" in prompt and "classifications" in prompt:
        _STATE["last_outcome_prompt"] = prompt
        _STATE["last_outcome_raw"] = raw
        _STATE["last_outcome_usage"] = usage
        _STATE["last_outcome_max_tokens"] = int(max_tokens)
    return raw, usage


def tracked_parse_json_object(text: str) -> dict[str, Any]:
    """Defer malformed model outputs to the bounded repair validators."""
    if text == _STATE.get("last_proposal_raw"):
        try:
            return _ORIGINAL_PARSE_JSON(text)
        except Exception as exc:
            return {"__proposal_parse_error__": repr(exc)}
    if text == _STATE.get("last_outcome_raw"):
        try:
            return _ORIGINAL_PARSE_JSON(text)
        except Exception as exc:
            return {"__outcome_parse_error__": repr(exc)}
    return _ORIGINAL_PARSE_JSON(text)


def validate_turn_with_one_repair(
    proposal: dict[str, Any], contract: dict[str, Any], available_actions: set[str] | None = None
) -> None:
    prompt = _STATE.get("last_proposal_prompt")
    raw = _STATE.get("last_proposal_raw")
    endpoint = _STATE.get("last_endpoint")
    max_tokens = _STATE.get("last_proposal_max_tokens")
    available = set(available_actions or set())
    if not prompt or raw is None:
        _ORIGINAL_VALIDATE_TURN(proposal, contract, available_actions)
        return

    try:
        canonical = parse_and_canonicalize_model_proposal(str(raw), prompt=str(prompt))
        _ORIGINAL_VALIDATE_TURN(canonical, contract, available_actions)
        proposal.clear()
        proposal.update(canonical)
        return
    except Exception as first_error:
        if not endpoint or not max_tokens:
            raise

        context = extract_prompt_context(str(prompt))
        turn = int(context["turn"])
        _STATE["proposal_repairs_attempted"] += 1
        record: dict[str, Any] = {
            "turn": turn,
            "initial_raw": raw,
            "initial_usage": _STATE.get("last_proposal_usage") or {},
            "initial_validation_error": repr(first_error),
            "repair_attempted": True,
            "repair_raw": None,
            "repair_usage": {},
            "repair_validation_error": None,
        }
        try:
            repair_prompt = build_proposal_repair_prompt(
                original_prompt=str(prompt),
                invalid_output=str(raw),
                validation_error=str(first_error),
                turn=turn,
                available_actions=sorted(available),
                max_chars=11500,
            )
            repaired_raw, repaired_usage = _ORIGINAL_CALL_LLM(
                str(endpoint), repair_prompt, max_tokens=int(max_tokens)
            )
            record.update({
                "repair_prompt_sha256": hashlib.sha256(repair_prompt.encode()).hexdigest(),
                "repair_prompt_chars": len(repair_prompt),
                "repair_raw": repaired_raw,
                "repair_usage": repaired_usage,
            })
            repaired = parse_and_canonicalize_model_proposal(repaired_raw, prompt=str(prompt))
            _ORIGINAL_VALIDATE_TURN(repaired, contract, available_actions)
            proposal.clear()
            proposal.update(repaired)
            _STATE["proposal_repairs_succeeded"] += 1
            record["repair_succeeded"] = True
            _append_jsonl(_STATE.get("proposal_repair_log"), record)
            return
        except Exception as repair_error:
            record["repair_succeeded"] = False
            record["repair_validation_error"] = repr(repair_error)
            _append_jsonl(_STATE.get("proposal_repair_log"), record)
            raise


def validate_outcome_with_one_repair(obj: dict[str, Any], tested_ids: set[str]) -> dict[str, Any]:
    try:
        return _ORIGINAL_VALIDATE_OUTCOME(obj, tested_ids)
    except Exception as first_error:
        prompt = _STATE.get("last_outcome_prompt")
        raw = _STATE.get("last_outcome_raw")
        endpoint = _STATE.get("last_endpoint")
        max_tokens = _STATE.get("last_outcome_max_tokens")
        if not prompt or raw is None or not endpoint or not max_tokens:
            raise

        _STATE["outcome_repairs_attempted"] += 1
        record: dict[str, Any] = {
            "initial_raw": raw,
            "initial_usage": _STATE.get("last_outcome_usage") or {},
            "initial_validation_error": repr(first_error),
            "repair_attempted": True,
            "repair_raw": None,
            "repair_usage": {},
            "repair_validation_error": None,
        }
        try:
            repair_prompt = build_outcome_repair_prompt(
                original_prompt=str(prompt),
                invalid_output=str(raw),
                validation_error=str(first_error),
                tested_ids=set(map(str, tested_ids)),
                max_chars=9000,
            )
            repaired_raw, repaired_usage = _ORIGINAL_CALL_LLM(
                str(endpoint), repair_prompt, max_tokens=int(max_tokens)
            )
            record.update({
                "repair_prompt_sha256": hashlib.sha256(repair_prompt.encode()).hexdigest(),
                "repair_prompt_chars": len(repair_prompt),
                "repair_raw": repaired_raw,
                "repair_usage": repaired_usage,
            })
            repaired_obj = _ORIGINAL_PARSE_JSON(repaired_raw)
            interpreted = _ORIGINAL_VALIDATE_OUTCOME(repaired_obj, tested_ids)
            _STATE["outcome_repairs_succeeded"] += 1
            record["repair_succeeded"] = True
            _append_jsonl(_STATE.get("outcome_repair_log"), record)
            return interpreted
        except Exception as repair_error:
            record["repair_succeeded"] = False
            record["repair_validation_error"] = repr(repair_error)
            _append_jsonl(_STATE.get("outcome_repair_log"), record)
            raise


def tracked_invoke_native(helper: str, request: dict[str, Any], db_path: str) -> dict[str, Any]:
    operation = str(request.get("operation"))
    if operation == "ingest_and_reason":
        for forbidden in [
            "provisional_hypothesis_id", "provisional_goal_id", "reopen_hypothesis_ids"
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
        if str(response.get("primary_hypothesis_id") or ""):
            _STATE["native_primary_hypothesis_selections"] += 1
        _append_jsonl(_STATE.get("native_controller_log"), {
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


def _arg_value(name: str) -> str | None:
    for index, value in enumerate(sys.argv):
        if value == name and index + 1 < len(sys.argv):
            return sys.argv[index + 1]
        if value.startswith(name + "="):
            return value.split("=", 1)[1]
    return None


def _configure(out_dir: Path) -> None:
    out_dir.mkdir(parents=True, exist_ok=True)
    _STATE.update({
        "native_controller_log": str(out_dir / "a15.paper.native_controller.jsonl"),
        "proposal_repair_log": str(out_dir / "a15.paper.proposal_repairs.jsonl"),
        "outcome_repair_log": str(out_dir / "a15.paper.outcome_repairs.jsonl"),
    })
    base.call_llm = tracked_call_llm
    base.parse_json_object = tracked_parse_json_object
    base.validate_turn = validate_turn_with_one_repair
    base.validate_outcome_interpretation = validate_outcome_with_one_repair
    base.invoke_native = tracked_invoke_native
    base.governed_context = governed_context_with_native_controller


def _write_paper_metadata(out_dir: Path) -> None:
    summary_path = out_dir / "summary.json"
    if not summary_path.exists():
        return
    summary = json.loads(summary_path.read_text(encoding="utf-8"))
    wrapper = {
        "experiment_role": "ARC paper treatment",
        "architecture_frozen": True,
        "proposal_language": "compact-semantics-over-runtime-evidence-catalog",
        "observation_authority": "deterministic-runtime",
        "dialectic_control_owner": "CompleteHypoKoshRuntime",
        "proposal_repair_policy": "max-one-same-model-retry",
        "outcome_repair_policy": "max-one-same-model-retry",
        "proposal_repairs_attempted": int(_STATE["proposal_repairs_attempted"]),
        "proposal_repairs_succeeded": int(_STATE["proposal_repairs_succeeded"]),
        "outcome_repairs_attempted": int(_STATE["outcome_repairs_attempted"]),
        "outcome_repairs_succeeded": int(_STATE["outcome_repairs_succeeded"]),
        "native_opposition_rounds": int(_STATE["native_opposition_rounds"]),
        "native_reopen_events": int(_STATE["native_reopen_events"]),
        "alternatives_reopened": int(_STATE["alternatives_reopened"]),
        "native_primary_hypothesis_selections": int(_STATE["native_primary_hypothesis_selections"]),
    }
    summary["paper_wrapper"] = wrapper
    summary["pi"] = "PI4.5A1.5-paper-treatment"
    summary_path.write_text(json.dumps(summary, indent=2, sort_keys=True, default=str) + "\n", encoding="utf-8")
    (out_dir / "a15_paper_wrapper_metrics.json").write_text(
        json.dumps(wrapper, indent=2, sort_keys=True) + "\n", encoding="utf-8"
    )


def main() -> None:
    out_value = _arg_value("--out")
    if not out_value:
        raise SystemExit("--out is required")
    out_dir = Path(out_value)
    _configure(out_dir)
    base.main()
    _write_paper_metadata(out_dir)


if __name__ == "__main__":
    main()
