#!/usr/bin/env python3
"""Plugin-only Graphene/HypoKosh chess counterfactual test v2.

Core GrapheneDB/HypoKosh sources are not modified. The harness reuses the basic chess
request builder and legality environment, enriches each ply with the v2 plugin, and
asks a plugin-specific native helper to reason in Theoretical mode.
"""
from __future__ import annotations

import argparse
import json
from pathlib import Path
from typing import Any

import chess

from a15_arc_dialectic_adapter import invoke_native, stable_digest
from chess_counterfactual_plugin_v2 import ChessCounterfactualPluginV2
from graphene_chess_basic import build_request, choose_move, flatten_rules, load_rules, write_json

WORLD_SCOPE = "graphene-chess-counterfactual-v2"


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--native-helper", default="/tmp/chess_counterfactual_native_helper")
    ap.add_argument("--db", default="/tmp/graphene_chess_counterfactual_v2.graphenedb")
    ap.add_argument("--rules", default="arc_competition/chess_rules.json")
    ap.add_argument("--plies", type=int, default=8)
    ap.add_argument("--out", default="/tmp/graphene-chess-counterfactual-v2")
    args = ap.parse_args()

    out = Path(args.out)
    out.mkdir(parents=True, exist_ok=True)
    raw_rules: dict[str, Any] = load_rules(Path(args.rules))
    rules = flatten_rules(raw_rules)
    plugin = ChessCounterfactualPluginV2(raw_rules)
    board = chess.Board()
    game: list[dict[str, Any]] = []

    for ply in range(args.plies):
        if board.is_game_over(claim_draw=True):
            break

        request, move_map, variations = build_request(
            board, ply, rules, seed_rules=(ply == 0)
        )
        # Isolate this experiment's world scope from the basic/control harness.
        request["world_scope"] = WORLD_SCOPE
        diagnostics = plugin.enrich(
            board=board,
            turn=ply,
            request=request,
            move_by_hypothesis=move_map,
            seed_principles=(ply == 0),
        )

        write_json(out / f"ply_{ply+1:02d}_request.json", request)
        write_json(out / f"ply_{ply+1:02d}_variations.json", variations)
        write_json(out / f"ply_{ply+1:02d}_plugin.json", diagnostics)

        response = invoke_native(args.native_helper, request, args.db)
        write_json(out / f"ply_{ply+1:02d}_native_response.json", response)

        try:
            selected_id, move = choose_move(response, move_map)
        except Exception as exc:
            write_json(out / "failure.json", {
                "ply": ply + 1,
                "error": str(exc),
                "primary_node": response.get("primary_node"),
                "primary_hypothesis_id": response.get("primary_hypothesis_id"),
                "confidence": response.get("confidence"),
                "epistemic_status": response.get("epistemic_status"),
                "reopened_hypothesis_ids": response.get("reopened_hypothesis_ids") or [],
                "challenged_claims": response.get("challenged_claims") or [],
                "native_falsification_questions": response.get("native_falsification_questions") or [],
                "residual_uncertainty": response.get("residual_uncertainty") or [],
                "reasoning_receipt": response.get("reasoning_receipt") or {},
                "plugin": diagnostics,
            })
            raise

        san = board.san(move)
        before_fen = board.fen()
        board.push(move)
        entry = {
            "ply": ply + 1,
            "side": "White" if ply % 2 == 0 else "Black",
            "selected_hypothesis_id": selected_id,
            "uci": move.uci(),
            "san": san,
            "before_fen": before_fen,
            "after_fen": board.fen(),
            "legal_candidate_count": len(move_map),
            "primary_node": response.get("primary_node"),
            "primary_hypothesis_id": response.get("primary_hypothesis_id"),
            "confidence": response.get("confidence"),
            "epistemic_status": response.get("epistemic_status"),
            "reopened_hypothesis_ids": response.get("reopened_hypothesis_ids") or [],
            "challenged_claims": response.get("challenged_claims") or [],
            "reasoning_receipt": response.get("reasoning_receipt") or {},
            "plugin_comparison_anchor_id": diagnostics["comparison_anchor_id"],
            "request_digest": stable_digest(request),
        }
        game.append(entry)
        write_json(out / f"ply_{ply+1:02d}.json", entry)
        print(
            f"{ply+1}. {entry['side']}: {san} [{move.uci()}] "
            f"primary={selected_id} confidence={entry['confidence']}"
        )

    summary = {
        "experiment": "Graphene Chess Counterfactual Plugin v2",
        "world_scope": WORLD_SCOPE,
        "plugin": plugin.name,
        "required_query_mode": "Theoretical",
        "rules_count": len(rules),
        "requested_plies": args.plies,
        "completed_plies": len(game),
        "moves": [
            {k: row[k] for k in ("ply", "side", "uci", "san", "selected_hypothesis_id", "confidence")}
            for row in game
        ],
        "final_fen": board.fen(),
        "game_over": board.is_game_over(claim_draw=True),
        "result": board.result(claim_draw=True) if board.is_game_over(claim_draw=True) else "*",
        "all_moves_legal": len(game) == args.plies,
        "plugin_selected_move": False,
        "core_graphenedb_modified": False,
        "core_hypokosh_modified": False,
        "llm_move_selector": False,
        "opening_book": False,
        "stockfish": False,
        "engine_guidance_during_reasoning": False,
    }
    write_json(out / "summary.json", summary)
    print(json.dumps(summary, indent=2, sort_keys=True))


if __name__ == "__main__":
    main()
