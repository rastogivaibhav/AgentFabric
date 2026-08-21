#!/usr/bin/env python3
"""Basic GrapheneDB/HypoKosh chess reasoning smoke test.

Experiment:
- Store the complete chess rule corpus in the native GrapheneDB ModelWorld.
- Start from the standard chess initial position.
- At each ply, enumerate legal moves with python-chess. The legality engine is the
  deterministic environment, not a move evaluator.
- Materialize every legal move as a competing Hypothesis node.
- Ask native CompleteHypoKoshRuntime to reason/converge/opposition over them.
- Select only the native runtime's primary hypothesis and apply that move.
- Repeat for four White and four Black moves (8 plies).

No opening book, Stockfish, external move score, or LLM chooses a move.
This test proves native selection/persistence, not chess strength.
"""
from __future__ import annotations

import argparse
import json
import subprocess
from pathlib import Path
from typing import Any

import chess

from a15_arc_dialectic_adapter import NativeRuntimeError, invoke_native, stable_digest

WORLD_SCOPE = "graphene-chess-basic-v1"


def load_rules(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def flatten_rules(rules: dict[str, Any]) -> list[str]:
    statements: list[str] = []
    statements.append(f"Objective: {rules['objective']}")
    for section, value in rules.items():
        if section in {"schema_version", "game", "objective", "notation", "board"}:
            continue
        if isinstance(value, list):
            statements.extend(f"{section}: {item}" for item in value)
        elif isinstance(value, dict):
            for key, items in value.items():
                if isinstance(items, list):
                    statements.extend(f"{section}.{key}: {item}" for item in items)
                else:
                    statements.append(f"{section}.{key}: {items}")
    return statements


def node(external_id: str, node_type: str, status: str, statement: str, origin: str,
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


def position_statement(board: chess.Board) -> str:
    side = "White" if board.turn == chess.WHITE else "Black"
    return (
        f"Current chess position FEN={board.fen()}; side_to_move={side}; "
        f"check={board.is_check()}; legal_move_count={board.legal_moves.count()}."
    )


def build_request(board: chess.Board, turn: int, rules: list[str], seed_rules: bool) -> tuple[dict[str, Any], dict[str, chess.Move]]:
    nodes: list[dict[str, Any]] = []
    relations: list[dict[str, Any]] = []
    pos_id = f"ply-{turn}-position"
    nodes.append(node(pos_id, "Fact", "Active", position_statement(board), "Observed", turn, {"kind": "board_position"}))

    objective_id = "chess-objective"
    if seed_rules:
        nodes.append(node(objective_id, "Decision", "Active", rules[0], "Observed", turn, {"kind": "game_objective"}))
        for idx, statement in enumerate(rules[1:], start=1):
            rid = f"chess-rule-{idx:03d}"
            nodes.append(node(rid, "Fact", "Active", statement, "Observed", turn, {"kind": "chess_rule"}))

    move_by_hypothesis: dict[str, chess.Move] = {}
    side = "White" if board.turn == chess.WHITE else "Black"
    for idx, move in enumerate(list(board.legal_moves), start=1):
        hid = f"ply-{turn}-move-{idx:03d}-{move.uci()}"
        move_by_hypothesis[hid] = move
        san = board.san(move)
        statement = (
            f"Candidate {side} move {move.uci()} ({san}) is a legal next action under the stored chess rules "
            f"and should be considered as a possible step toward the checkmate objective."
        )
        nodes.append(node(hid, "Hypothesis", "Proposed", statement, "Hypothetical", turn, {
            "kind": "candidate_chess_move",
            "uci": move.uci(),
            "san": san,
            "side": side,
        }))
        relations.append({
            "from": pos_id,
            "to": hid,
            "role": "Supports",
            "origin": "Observed",
            "confidence": 0.55,
        })
        # Every candidate is explicitly tied to the game objective. This is not a
        # quality score: all legal moves receive the same relation/confidence.
        relations.append({
            "from": hid,
            "to": objective_id,
            "role": "Predictive",
            "origin": "Hypothetical",
            "confidence": 0.40,
        })

    request = {
        "protocol": "agentfabric-chess-basic-v1",
        "operation": "ingest_and_reason",
        "world_scope": WORLD_SCOPE,
        "turn": turn,
        "nodes": nodes,
        "relations": relations,
    }
    return request, move_by_hypothesis


def choose_move(response: dict[str, Any], move_by_hypothesis: dict[str, chess.Move]) -> tuple[str, chess.Move]:
    primary = str(response.get("primary_hypothesis_id") or "")
    if primary not in move_by_hypothesis:
        reopened = [str(x) for x in response.get("reopened_hypothesis_ids") or []]
        for candidate in reopened:
            if candidate in move_by_hypothesis:
                return candidate, move_by_hypothesis[candidate]
        raise NativeRuntimeError(
            f"native runtime did not select a current legal move hypothesis; primary={primary!r}, "
            f"reopened={reopened[:5]}"
        )
    return primary, move_by_hypothesis[primary]


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--native-helper", default="/tmp/a15_native_runtime_helper")
    ap.add_argument("--db", default="/tmp/graphene_chess_basic.graphenedb")
    ap.add_argument("--rules", default="arc_competition/chess_rules.json")
    ap.add_argument("--plies", type=int, default=8)
    ap.add_argument("--out", default="/tmp/graphene-chess-basic")
    args = ap.parse_args()

    out = Path(args.out)
    out.mkdir(parents=True, exist_ok=True)
    rules = flatten_rules(load_rules(Path(args.rules)))
    board = chess.Board()
    game: list[dict[str, Any]] = []

    for ply in range(args.plies):
        if board.is_game_over(claim_draw=True):
            break
        request, move_map = build_request(board, ply, rules, seed_rules=(ply == 0))
        response = invoke_native(args.native_helper, request, args.db)
        selected_id, move = choose_move(response, move_map)
        san = board.san(move)
        before_fen = board.fen()
        board.push(move)
        receipt = response.get("reasoning_receipt") or {}
        entry = {
            "ply": ply + 1,
            "side": "White" if ply % 2 == 0 else "Black",
            "selected_hypothesis_id": selected_id,
            "uci": move.uci(),
            "san": san,
            "before_fen": before_fen,
            "after_fen": board.fen(),
            "legal_candidate_count": len(move_map),
            "primary_hypothesis_id": response.get("primary_hypothesis_id"),
            "reopened_hypothesis_ids": response.get("reopened_hypothesis_ids") or [],
            "challenged_claims": response.get("challenged_claims") or [],
            "epistemic_status": response.get("epistemic_status"),
            "model_world_nodes": response.get("model_world_nodes"),
            "model_world_events": response.get("model_world_events"),
            "reasoning_receipt": receipt,
            "request_digest": stable_digest(request),
        }
        game.append(entry)
        (out / f"ply_{ply+1:02d}.json").write_text(json.dumps(entry, indent=2, sort_keys=True) + "\n")
        print(f"{ply+1}. {entry['side']}: {san} [{move.uci()}] primary={selected_id}")

    summary = {
        "experiment": "Graphene Chess Dialectic Test basic",
        "world_scope": WORLD_SCOPE,
        "rules_count": len(rules),
        "requested_plies": args.plies,
        "completed_plies": len(game),
        "moves": [{k: row[k] for k in ("ply", "side", "uci", "san", "selected_hypothesis_id")} for row in game],
        "final_fen": board.fen(),
        "game_over": board.is_game_over(claim_draw=True),
        "result": board.result(claim_draw=True) if board.is_game_over(claim_draw=True) else "*",
        "all_moves_legal": len(game) == args.plies,
        "llm_move_selector": False,
        "opening_book": False,
        "stockfish": False,
    }
    (out / "summary.json").write_text(json.dumps(summary, indent=2, sort_keys=True) + "\n")
    print(json.dumps(summary, indent=2, sort_keys=True))


if __name__ == "__main__":
    main()
