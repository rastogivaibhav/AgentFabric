#!/usr/bin/env python3
"""Counterfactual consequence plugin for the Graphene/HypoKosh chess experiment.

This module is intentionally outside GrapheneDB/HypoKosh core.  It does not select,
rank, or authorize a chess move.  It deterministically simulates each already-legal
candidate move, derives observable board consequences, and appends hypothetical
nodes/relations to the request consumed by the existing native runtime.

No Stockfish, opening book, LLM, learned evaluator, or move-specific preference is
used.  The only domain knowledge is the strategy/tactics corpus supplied by the
experiment and state deltas computable from python-chess.
"""
from __future__ import annotations

from dataclasses import dataclass
from typing import Any

import chess

CENTER = (chess.D4, chess.E4, chess.D5, chess.E5)
PIECE_VALUE = {
    chess.PAWN: 1,
    chess.KNIGHT: 3,
    chess.BISHOP: 3,
    chess.ROOK: 5,
    chess.QUEEN: 9,
    chess.KING: 0,
}
MINOR_STARTS = {
    chess.WHITE: {chess.B1, chess.G1, chess.C1, chess.F1},
    chess.BLACK: {chess.B8, chess.G8, chess.C8, chess.F8},
}


@dataclass(frozen=True)
class Consequence:
    principle: str
    signal: str
    delta: float
    statement: str

    @property
    def positive(self) -> bool:
        return self.delta > 0


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


def _relation(source: str, target: str, role: str, confidence: float) -> dict[str, Any]:
    return {
        "from": source,
        "to": target,
        "role": role,
        "origin": "Hypothetical",
        "confidence": round(max(0.01, min(0.99, confidence)), 4),
    }


def _material(board: chess.Board, color: chess.Color) -> int:
    own = sum(len(board.pieces(pt, color)) * value for pt, value in PIECE_VALUE.items())
    opp = sum(len(board.pieces(pt, not color)) * value for pt, value in PIECE_VALUE.items())
    return own - opp


def _center_score(board: chess.Board, color: chess.Color) -> int:
    score = 0
    for square in CENTER:
        piece = board.piece_at(square)
        if piece and piece.color == color:
            score += 2
        score += min(2, len(board.attackers(color, square)))
    return score


def _activity(board: chess.Board, color: chess.Color) -> int:
    attacked: set[int] = set()
    for square in chess.SQUARES:
        piece = board.piece_at(square)
        if piece and piece.color == color:
            attacked.update(board.attacks(square))
    return len(attacked)


def _space(board: chess.Board, color: chess.Color) -> int:
    target_ranks = range(4, 8) if color == chess.WHITE else range(0, 4)
    target = {chess.square(file_idx, rank_idx) for rank_idx in target_ranks for file_idx in range(8)}
    attacked: set[int] = set()
    for square in chess.SQUARES:
        piece = board.piece_at(square)
        if piece and piece.color == color:
            attacked.update(s for s in board.attacks(square) if s in target)
    return len(attacked)


def _pawn_weaknesses(board: chess.Board, color: chess.Color) -> int:
    pawns = list(board.pieces(chess.PAWN, color))
    by_file = {file_idx: 0 for file_idx in range(8)}
    for square in pawns:
        by_file[chess.square_file(square)] += 1
    doubled = sum(max(0, n - 1) for n in by_file.values())
    isolated = 0
    for square in pawns:
        f = chess.square_file(square)
        neighbours = [nf for nf in (f - 1, f + 1) if 0 <= nf < 8]
        if not any(by_file[nf] for nf in neighbours):
            isolated += 1
    return doubled + isolated


def _king_shield(board: chess.Board, color: chess.Color) -> int:
    king = board.king(color)
    if king is None:
        return 0
    kf, kr = chess.square_file(king), chess.square_rank(king)
    count = 0
    for df in (-1, 0, 1):
        for dr in (-1, 0, 1):
            if df == 0 and dr == 0:
                continue
            f, r = kf + df, kr + dr
            if 0 <= f < 8 and 0 <= r < 8:
                p = board.piece_at(chess.square(f, r))
                if p and p.color == color and p.piece_type == chess.PAWN:
                    count += 1
    return count


def _pinned_count(board: chess.Board, color: chess.Color) -> int:
    return sum(
        1
        for square in chess.SQUARES
        if (piece := board.piece_at(square)) is not None
        and piece.color == color
        and piece.piece_type != chess.KING
        and board.is_pinned(color, square)
    )


def _mobility_for(board: chess.Board, color: chess.Color) -> int:
    probe = board.copy(stack=False)
    probe.turn = color
    return probe.legal_moves.count()


def _fork_targets(after: chess.Board, move: chess.Move, mover: chess.Color) -> int:
    piece = after.piece_at(move.to_square)
    if piece is None or piece.color != mover:
        return 0
    count = 0
    for target in after.attacks(move.to_square):
        victim = after.piece_at(target)
        if victim and victim.color != mover and PIECE_VALUE[victim.piece_type] >= 3:
            count += 1
    return count


def _derive(board: chess.Board, move: chess.Move) -> tuple[chess.Board, list[Consequence]]:
    mover_piece = board.piece_at(move.from_square)
    if mover_piece is None:
        raise ValueError(f"candidate has no moving piece: {move.uci()}")
    mover = mover_piece.color
    san = board.san(move)
    was_capture = board.is_capture(move)
    was_castling = board.is_castling(move)
    gave_check = board.gives_check(move)

    before_material = _material(board, mover)
    before_center = _center_score(board, mover)
    before_activity = _activity(board, mover)
    before_space = _space(board, mover)
    before_weakness = _pawn_weaknesses(board, mover)
    before_shield = _king_shield(board, mover)
    before_pins = _pinned_count(board, not mover)
    before_mobility = _mobility_for(board, mover)

    after = board.copy(stack=False)
    after.push(move)

    consequences: list[Consequence] = []

    developed = (
        mover_piece.piece_type in {chess.KNIGHT, chess.BISHOP}
        and move.from_square in MINOR_STARTS[mover]
        and move.to_square != move.from_square
    )
    if developed:
        consequences.append(Consequence(
            "development", "minor_piece_developed", 1.0,
            f"{san} develops a minor piece from its starting square into active play.",
        ))

    center_delta = _center_score(after, mover) - before_center
    if center_delta:
        consequences.append(Consequence(
            "center_control", "center_control_delta", float(center_delta),
            f"{san} changes control/occupation of d4, e4, d5, e5 by {center_delta:+d} units.",
        ))

    activity_delta = _activity(after, mover) - before_activity
    if activity_delta:
        consequences.append(Consequence(
            "piece_activity", "attacked_square_delta", float(activity_delta),
            f"{san} changes the number of distinct squares influenced by the moving side by {activity_delta:+d}.",
        ))

    mobility_delta = _mobility_for(after, mover) - before_mobility
    if mobility_delta:
        consequences.append(Consequence(
            "mobility", "legal_mobility_delta", float(mobility_delta),
            f"{san} changes the moving side's legal-move mobility in the hypothetical position by {mobility_delta:+d}.",
        ))

    space_delta = _space(after, mover) - before_space
    if space_delta:
        consequences.append(Consequence(
            "space", "opponent_half_control_delta", float(space_delta),
            f"{san} changes controlled squares in the opponent half by {space_delta:+d}.",
        ))

    material_delta = _material(after, mover) - before_material
    if material_delta:
        consequences.append(Consequence(
            "material", "material_balance_delta", float(material_delta),
            f"{san} changes material balance by {material_delta:+d} using conventional piece values 1/3/3/5/9.",
        ))

    weakness_delta = _pawn_weaknesses(after, mover) - before_weakness
    if weakness_delta:
        # More weaknesses are bad, so invert direction for evidence polarity.
        consequences.append(Consequence(
            "pawn_structure", "pawn_weakness_delta", float(-weakness_delta),
            f"{san} changes doubled/isolated pawn weaknesses by {weakness_delta:+d}; fewer weaknesses are preferred.",
        ))

    shield_delta = _king_shield(after, mover) - before_shield
    if shield_delta:
        consequences.append(Consequence(
            "king_safety", "king_pawn_shield_delta", float(shield_delta),
            f"{san} changes the count of friendly pawns adjacent to the king by {shield_delta:+d}.",
        ))
    if was_castling:
        consequences.append(Consequence(
            "king_safety", "castling", 2.0,
            f"{san} castles, directly applying the stored principle that castling improves king safety and activates a rook.",
        ))

    pin_delta = _pinned_count(after, not mover) - before_pins
    if pin_delta:
        consequences.append(Consequence(
            "pin", "enemy_pinned_piece_delta", float(pin_delta),
            f"{san} changes the number of pinned opponent pieces by {pin_delta:+d}.",
        ))

    fork_targets = _fork_targets(after, move, mover)
    if fork_targets >= 2:
        consequences.append(Consequence(
            "fork", "valuable_fork_targets", float(fork_targets),
            f"After {san}, the moved piece attacks {fork_targets} opponent pieces valued at least a minor piece.",
        ))

    if was_capture:
        consequences.append(Consequence(
            "forcing_play", "capture", 1.0,
            f"{san} is a capture and therefore a forcing candidate requiring tactical examination.",
        ))
    if gave_check:
        consequences.append(Consequence(
            "forcing_play", "check", 2.0,
            f"{san} gives check and constrains the opponent's legal replies.",
        ))
    if move.promotion:
        consequences.append(Consequence(
            "forcing_play", "promotion", 3.0,
            f"{san} promotes a pawn, creating an immediate high-impact material consequence.",
        ))
    if after.is_checkmate():
        consequences.append(Consequence(
            "tactical_priority", "checkmate", 10.0,
            f"{san} produces checkmate in the hypothetical next state.",
        ))

    return after, consequences


def _confidence(delta: float) -> float:
    magnitude = abs(delta)
    return min(0.97, 0.58 + min(0.39, 0.055 * magnitude))


class ChessCounterfactualPlugin:
    """Enrich a chess reasoning request; never selects a move."""

    name = "chess-counterfactual-consequence-v1"

    def __init__(self, rules: dict[str, Any]):
        self.rules = rules
        self.principles = self._principles(rules)

    @staticmethod
    def _principles(rules: dict[str, Any]) -> dict[str, str]:
        out: dict[str, str] = {}
        for section in ("strategic_principles", "tactical_principles"):
            for key, values in (rules.get(section) or {}).items():
                if isinstance(values, list) and values:
                    out[str(key)] = " ".join(str(v) for v in values)
        return out

    def enrich(self, *, board: chess.Board, turn: int, request: dict[str, Any],
               move_by_hypothesis: dict[str, chess.Move], seed_principles: bool) -> dict[str, Any]:
        nodes = request.setdefault("nodes", [])
        relations = request.setdefault("relations", [])
        existing = {str(n.get("external_id")) for n in nodes}

        if seed_principles:
            for key, statement in sorted(self.principles.items()):
                pid = f"cf-principle-{key}"
                if pid not in existing:
                    nodes.append(_node(
                        pid, "Concept", "Active", statement, "Observed", turn,
                        {"kind": "counterfactual_principle", "principle": key},
                    ))
                    existing.add(pid)

        diagnostics: dict[str, Any] = {
            "plugin": self.name,
            "turn": turn,
            "selected_move": None,
            "candidate_count": len(move_by_hypothesis),
            "candidates": [],
        }

        for hypothesis_id, move in move_by_hypothesis.items():
            after, consequences = _derive(board, move)
            row = {
                "hypothesis_id": hypothesis_id,
                "uci": move.uci(),
                "after_fen": after.fen(),
                "support_count": 0,
                "contradiction_count": 0,
                "consequences": [],
            }
            for index, consequence in enumerate(consequences, start=1):
                cid = f"cf-ply-{turn}-{move.uci()}-{index:02d}-{consequence.signal}"
                polarity = "support" if consequence.positive else "contradiction"
                statement = consequence.statement
                nodes.append(_node(
                    cid, "Outcome", "Proposed", statement, "Hypothetical", turn,
                    {
                        "kind": "counterfactual_consequence",
                        "principle": consequence.principle,
                        "signal": consequence.signal,
                        "delta": consequence.delta,
                        "polarity": polarity,
                        "uci": move.uci(),
                    },
                ))
                # Candidate predicts this hypothetical consequence.
                relations.append(_relation(hypothesis_id, cid, "Predictive", 0.82))
                # Stored principle explains why the consequence matters.
                pid = f"cf-principle-{consequence.principle}"
                if consequence.principle in self.principles:
                    relations.append(_relation(pid, cid, "Mechanistic", 0.78))
                # The consequence is evidence for or against the candidate move.
                role = "Supports" if consequence.positive else "Contradicts"
                confidence = _confidence(consequence.delta)
                relations.append(_relation(cid, hypothesis_id, role, confidence))
                if consequence.positive:
                    row["support_count"] += 1
                else:
                    row["contradiction_count"] += 1
                row["consequences"].append({
                    "id": cid,
                    "principle": consequence.principle,
                    "signal": consequence.signal,
                    "delta": consequence.delta,
                    "polarity": polarity,
                    "confidence": confidence,
                    "statement": statement,
                })
            diagnostics["candidates"].append(row)

        request["protocol"] = "agentfabric-chess-basic-v2+counterfactual-plugin-v1"
        request.setdefault("plugin_metadata", {})["counterfactual_plugin"] = self.name
        return diagnostics
