from __future__ import annotations

import json
import unittest
from pathlib import Path

import chess

from chess_counterfactual_plugin_v2 import ChessCounterfactualPluginV2

ROOT = Path(__file__).resolve().parents[1]


class ChessCounterfactualPluginV2Tests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.rules = json.loads((ROOT / "arc_competition/chess_rules.json").read_text())

    @staticmethod
    def _base_request() -> dict:
        return {
            "protocol": "test",
            "operation": "ingest_and_reason",
            "world_scope": "test",
            "turn": 0,
            "nodes": [
                {"external_id": "h-e4", "node_type": "Hypothesis", "status": "Proposed", "statement": "e4", "origin": "Hypothetical", "metadata": {}},
                {"external_id": "h-nf3", "node_type": "Hypothesis", "status": "Proposed", "statement": "Nf3", "origin": "Hypothetical", "metadata": {}},
                {"external_id": "h-h3", "node_type": "Hypothesis", "status": "Proposed", "statement": "h3", "origin": "Hypothetical", "metadata": {}},
            ],
            "relations": [],
        }

    def test_v2_builds_backward_traversable_paths_without_selecting(self) -> None:
        board = chess.Board()
        moves = {
            "h-e4": chess.Move.from_uci("e2e4"),
            "h-nf3": chess.Move.from_uci("g1f3"),
            "h-h3": chess.Move.from_uci("h2h3"),
        }
        request = self._base_request()
        plugin = ChessCounterfactualPluginV2(self.rules)
        diagnostics = plugin.enrich(
            board=board,
            turn=0,
            request=request,
            move_by_hypothesis=moves,
            seed_principles=True,
        )

        self.assertIsNone(diagnostics["selected_move"])
        self.assertIsNone(diagnostics["ranked_moves"])
        anchor = diagnostics["comparison_anchor_id"]
        self.assertEqual(request["nodes"][0]["external_id"], anchor)
        self.assertEqual(request["plugin_metadata"]["required_query_mode"], "Theoretical")

        ids = {n["external_id"] for n in request["nodes"]}
        incoming_anchor = [r for r in request["relations"] if r["to"] == anchor]
        self.assertGreater(len(incoming_anchor), 0)
        for rel in incoming_anchor:
            self.assertIn(rel["role"], {"Supports", "Contradicts"})
            self.assertEqual(rel["origin"], "Hypothetical")
            self.assertIn(rel["from"], ids)

        # Every move hypothesis must have at least one forward Predictive branch.
        for hid in moves:
            predicted = [r for r in request["relations"] if r["from"] == hid and r["role"] == "Predictive"]
            self.assertGreater(len(predicted), 0, hid)
            for rel in predicted:
                self.assertIn(rel["to"], ids)
                self.assertEqual(rel["origin"], "Hypothetical")

        # The old consequence -> hypothesis support orientation must not be present.
        consequence_ids = {n["external_id"] for n in request["nodes"] if (n.get("metadata") or {}).get("kind") == "counterfactual_consequence"}
        backwards = [r for r in request["relations"] if r["from"] in consequence_ids and r["to"] in moves]
        self.assertEqual(backwards, [])

    def test_opening_candidates_remain_differentiated_but_unranked(self) -> None:
        board = chess.Board()
        moves = {
            "h-e4": chess.Move.from_uci("e2e4"),
            "h-nf3": chess.Move.from_uci("g1f3"),
            "h-h3": chess.Move.from_uci("h2h3"),
        }
        request = self._base_request()
        diagnostics = ChessCounterfactualPluginV2(self.rules).enrich(
            board=board, turn=0, request=request, move_by_hypothesis=moves, seed_principles=True,
        )
        rows = {r["hypothesis_id"]: r for r in diagnostics["candidates"]}
        e4 = {c["signal"] for c in rows["h-e4"]["consequences"]}
        nf3 = {c["signal"] for c in rows["h-nf3"]["consequences"]}
        h3 = {c["signal"] for c in rows["h-h3"]["consequences"]}
        self.assertIn("center_control_delta", e4)
        self.assertIn("minor_piece_developed", nf3)
        self.assertNotEqual(e4, h3)
        for row in rows.values():
            self.assertNotIn("score", row)
            self.assertNotIn("rank", row)

    def test_source_does_not_embed_engine_or_opening_book(self) -> None:
        source = (ROOT / "arc_competition/chess_counterfactual_plugin_v2.py").read_text().lower()
        self.assertNotIn("chess.engine", source)
        self.assertNotIn("polyglot", source)
        self.assertNotIn("selected_move =", source)


if __name__ == "__main__":
    unittest.main()
