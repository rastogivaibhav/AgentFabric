from __future__ import annotations

import json
import unittest
from pathlib import Path

import chess

from chess_counterfactual_plugin import ChessCounterfactualPlugin

ROOT = Path(__file__).resolve().parents[1]


class ChessCounterfactualPluginTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.rules = json.loads((ROOT / "arc_competition/chess_rules.json").read_text())

    def _base_request(self) -> dict:
        return {
            "protocol": "test",
            "operation": "ingest_and_reason",
            "world_scope": "test",
            "turn": 0,
            "nodes": [
                {
                    "external_id": "h-e4",
                    "node_type": "Hypothesis",
                    "status": "Proposed",
                    "statement": "e4",
                    "origin": "Hypothetical",
                    "metadata": {},
                },
                {
                    "external_id": "h-nf3",
                    "node_type": "Hypothesis",
                    "status": "Proposed",
                    "statement": "Nf3",
                    "origin": "Hypothetical",
                    "metadata": {},
                },
                {
                    "external_id": "h-h3",
                    "node_type": "Hypothesis",
                    "status": "Proposed",
                    "statement": "h3",
                    "origin": "Hypothetical",
                    "metadata": {},
                },
            ],
            "relations": [],
        }

    def test_initial_position_produces_differentiated_evidence_without_selection(self) -> None:
        board = chess.Board()
        moves = {
            "h-e4": chess.Move.from_uci("e2e4"),
            "h-nf3": chess.Move.from_uci("g1f3"),
            "h-h3": chess.Move.from_uci("h2h3"),
        }
        request = self._base_request()
        plugin = ChessCounterfactualPlugin(self.rules)
        diagnostics = plugin.enrich(
            board=board,
            turn=0,
            request=request,
            move_by_hypothesis=moves,
            seed_principles=True,
        )

        self.assertIsNone(diagnostics["selected_move"])
        rows = {r["hypothesis_id"]: r for r in diagnostics["candidates"]}
        e4_signals = {c["signal"] for c in rows["h-e4"]["consequences"]}
        nf3_signals = {c["signal"] for c in rows["h-nf3"]["consequences"]}
        self.assertIn("center_control_delta", e4_signals)
        self.assertIn("minor_piece_developed", nf3_signals)
        self.assertNotEqual(e4_signals, {c["signal"] for c in rows["h-h3"]["consequences"]})

        ids = {n["external_id"] for n in request["nodes"]}
        self.assertIn("cf-principle-development", ids)
        self.assertIn("cf-principle-center_control", ids)
        for rel in request["relations"]:
            self.assertIn(rel["from"], ids)
            self.assertIn(rel["to"], ids)

    def test_plugin_emits_support_and_contradiction_edges_but_no_move_ranking(self) -> None:
        # A position where a pawn move can alter structure and where several board
        # features differ. The plugin may derive positive and/or negative evidence,
        # but it must never attach a scalar move score or selection.
        board = chess.Board("rnbqkbnr/pppp1ppp/8/4p3/3P4/8/PPP1PPPP/RNBQKBNR w KQkq - 0 2")
        move = chess.Move.from_uci("d4e5")
        self.assertIn(move, board.legal_moves)
        request = {
            "protocol": "test",
            "operation": "ingest_and_reason",
            "world_scope": "test",
            "turn": 1,
            "nodes": [{
                "external_id": "h-capture",
                "node_type": "Hypothesis",
                "status": "Proposed",
                "statement": "dxe5",
                "origin": "Hypothetical",
                "metadata": {},
            }],
            "relations": [],
        }
        plugin = ChessCounterfactualPlugin(self.rules)
        diagnostics = plugin.enrich(
            board=board,
            turn=1,
            request=request,
            move_by_hypothesis={"h-capture": move},
            seed_principles=True,
        )
        self.assertNotIn("score", diagnostics["candidates"][0])
        self.assertNotIn("rank", diagnostics["candidates"][0])
        self.assertIsNone(diagnostics["selected_move"])
        roles = {r["role"] for r in request["relations"] if r["to"] == "h-capture"}
        self.assertIn("Supports", roles)

    def test_source_contains_no_engine_or_opening_book_dependency(self) -> None:
        source = (ROOT / "arc_competition/chess_counterfactual_plugin.py").read_text().lower()
        self.assertNotIn("stockfish", source.replace("no stockfish", ""))
        self.assertNotIn("chess.engine", source)
        self.assertNotIn("polyglot", source)
        self.assertNotIn("opening book", source.replace("opening book", "", 1))


if __name__ == "__main__":
    unittest.main()
