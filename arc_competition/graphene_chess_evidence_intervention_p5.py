#!/usr/bin/env python3
from __future__ import annotations
import argparse, random, subprocess
from collections import Counter
from pathlib import Path
from typing import Any
import chess
from a15_arc_dialectic_adapter import invoke_native, stable_digest
from chess_counterfactual_plugin_v5 import ChessCounterfactualPluginV5
from graphene_chess_selection_causality_p44 import build_permuted
from graphene_chess_basic import flatten_rules, load_rules, write_json

CONDITIONS = ("baseline", "ablate_e4_positive", "contradict_e4", "boost_d4_positive", "restore")


def find_candidate(diag: dict[str, Any], uci: str) -> dict[str, Any]:
    row = next((r for r in diag["candidates"] if r["uci"] == uci), None)
    if not row:
        raise RuntimeError(f"candidate {uci} missing")
    return row


def evidence_lists(row: dict[str, Any]) -> tuple[list[float], list[float]]:
    pos, neg = [], []
    for c in row.get("consequences", []):
        conf = float(c.get("confidence", 0.0))
        if c.get("polarity") == "support": pos.append(conf)
        elif c.get("polarity") == "contradiction": neg.append(conf)
    return pos, neg


def set_aggregate_support(request: dict[str, Any], hypothesis_id: str, anchor_id: str, confidence: float) -> None:
    matches = [r for r in request["relations"] if r["from"] == hypothesis_id and r["to"] == anchor_id and r["role"] == "Supports"]
    if len(matches) != 1:
        raise RuntimeError(f"expected one aggregate support edge for {hypothesis_id}, found {len(matches)}")
    matches[0]["confidence"] = float(confidence)


def apply_intervention(plugin: ChessCounterfactualPluginV5, condition: str, request: dict[str, Any], diag: dict[str, Any]) -> dict[str, Any]:
    anchor = diag["comparison_anchor_id"]
    target_uci = None
    before = after = None
    details: dict[str, Any] = {"condition": condition, "intervention_level": "counterfactual_evidence_before_aggregation"}
    if condition in ("baseline", "restore"):
        details["changed"] = False
        return details
    if condition in ("ablate_e4_positive", "contradict_e4"):
        target_uci = "e2e4"
    elif condition == "boost_d4_positive":
        target_uci = "d2d4"
    else:
        raise ValueError(condition)
    row = find_candidate(diag, target_uci)
    pos, neg = evidence_lists(row)
    before = plugin.synthesize(pos, neg)
    if condition == "ablate_e4_positive":
        pos = []
        details["operation"] = "remove_all_positive_counterfactual_evidence"
    elif condition == "contradict_e4":
        neg = neg + [0.99, 0.99, 0.99]
        details["operation"] = "inject_three_strong_counterfactual_contradictions"
    elif condition == "boost_d4_positive":
        pos = pos + [0.99] * 5
        details["operation"] = "inject_five_strong_counterfactual_support_observations"
    after = plugin.synthesize(pos, neg)
    set_aggregate_support(request, row["hypothesis_id"], anchor, after)
    details.update({"changed": True, "target_uci": target_uci, "before_aggregate": before, "after_aggregate": after, "positive_count_after": len(pos), "contradiction_count_after": len(neg)})
    return details


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--native-helper", required=True)
    ap.add_argument("--bootstrap", required=True)
    ap.add_argument("--db-prefix", default="/tmp/graphene_chess_p5")
    ap.add_argument("--rules", default="arc_competition/chess_rules.json")
    ap.add_argument("--runs", type=int, default=10)
    ap.add_argument("--out", default="/tmp/graphene-chess-p5")
    a = ap.parse_args()
    out = Path(a.out); out.mkdir(parents=True, exist_ok=True)
    raw: dict[str, Any] = load_rules(Path(a.rules)); rules = flatten_rules(raw); plugin = ChessCounterfactualPluginV5(raw)
    canonical = list(chess.Board().legal_moves)
    summaries: dict[str, Any] = {}
    for condition in CONDITIONS:
        rows = []
        cdir = out / condition; cdir.mkdir(parents=True, exist_ok=True)
        for seed in range(a.runs):
            board = chess.Board(); moves = canonical.copy(); random.Random(seed).shuffle(moves)
            req, move_map = build_permuted(board, 0, rules, moves)
            req["world_scope"] = f"graphene-chess-p5-{condition}-seed-{seed}"
            diag = plugin.enrich(board=board, turn=0, request=req, move_by_hypothesis=move_map, seed_principles=True)
            intervention = apply_intervention(plugin, condition, req, diag)
            db = f"{a.db_prefix}.{condition}.seed-{seed}.graphenedb"
            subprocess.run([a.bootstrap, db], check=True, capture_output=True, text=True)
            resp = invoke_native(a.native_helper, req, db)
            primary = str(resp.get("primary_hypothesis_id") or "")
            selected = move_map.get(primary); uci = selected.uci() if selected else None
            row = {"seed": seed, "condition": condition, "selected_uci": uci, "first_candidate_uci": moves[0].uci(), "confidence": resp.get("confidence"), "intervention": intervention, "request_digest": stable_digest(req)}
            rows.append(row)
            write_json(cdir / f"run_{seed:02d}.json", row); write_json(cdir / f"run_{seed:02d}_native.json", resp)
            print(condition, seed, uci, row["confidence"], intervention)
        counts = Counter(r["selected_uci"] for r in rows if r["selected_uci"])
        dominant, dominant_count = counts.most_common(1)[0] if counts else (None, 0)
        invariant = dominant_count == a.runs and len(counts) == 1
        summaries[condition] = {"selection_counts": dict(counts), "dominant_move": dominant, "dominant_count": dominant_count, "semantic_invariant": invariant, "all_native_selected": all(r["selected_uci"] for r in rows)}
    baseline_ok = summaries["baseline"]["semantic_invariant"] and summaries["baseline"]["dominant_move"] == "e2e4"
    ablation_ok = summaries["ablate_e4_positive"]["semantic_invariant"] and summaries["ablate_e4_positive"]["dominant_move"] != "e2e4"
    contradiction_ok = summaries["contradict_e4"]["semantic_invariant"] and summaries["contradict_e4"]["dominant_move"] != "e2e4"
    boost_ok = summaries["boost_d4_positive"]["semantic_invariant"] and summaries["boost_d4_positive"]["dominant_move"] == "d2d4"
    restore_ok = summaries["restore"]["semantic_invariant"] and summaries["restore"]["dominant_move"] == "e2e4"
    passed = baseline_ok and ablation_ok and contradiction_ok and boost_ok and restore_ok
    summary = {"experiment": "GCDT-P5 causal evidence intervention", "runs_per_condition": a.runs, "conditions": summaries, "baseline_ok": baseline_ok, "ablation_ok": ablation_ok, "contradiction_ok": contradiction_ok, "boost_ok": boost_ok, "restore_ok": restore_ok, "evidence_intervention_pass": passed, "core_graphenedb_modified": False, "core_hypokosh_modified": False, "plugin_selected_move": False, "stockfish": False, "opening_book": False, "llm_move_selector": False}
    write_json(out / "summary.json", summary); print(summary)
    raise SystemExit(0 if passed else 2)

if __name__ == "__main__": main()
