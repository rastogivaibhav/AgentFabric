#!/usr/bin/env python3
"""Plugin-only FiberBundle adapter for the Graphene/HypoKosh chess experiment.

This module does not modify GrapheneDB or HypoKosh core and never chooses/ranks a
move.  It reuses deterministic board-delta consequences from the v1 plugin, but
maps them into the path orientation expected by the native dialectic engine:

    move Hypothesis(root) -> predicted Consequence -> current-turn comparison anchor

The native engine starts at the comparison anchor, walks incoming edges backwards,
and can therefore reach each move hypothesis as a root. Positive consequence->anchor
edges are Supports; negative edges are Contradicts. All counterfactual relations
remain Hypothetical and are intended to be queried in Theoretical mode.
"""
from __future__ import annotations

from typing import Any

import chess

from chess_counterfactual_plugin import _confidence, _derive, _node, _relation


class ChessCounterfactualPluginV2:
    """Expose counterfactual move branches to HypoKosh without selecting a move."""

    name = "chess-counterfactual-fiber-adapter-v2"

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

    def enrich(
        self,
        *,
        board: chess.Board,
        turn: int,
        request: dict[str, Any],
        move_by_hypothesis: dict[str, chess.Move],
        seed_principles: bool,
    ) -> dict[str, Any]:
        nodes = request.setdefault("nodes", [])
        relations = request.setdefault("relations", [])
        existing = {str(n.get("external_id")) for n in nodes}

        anchor_id = f"cf2-ply-{turn}-comparison-anchor"
        anchor = _node(
            anchor_id,
            "Outcome",
            "Proposed",
            (
                f"Current-turn theoretical comparison anchor for ply {turn}; "
                "candidate move consequences converge here only for dialectical comparison."
            ),
            "Hypothetical",
            turn,
            {"kind": "counterfactual_comparison_anchor", "plugin": self.name},
        )
        # Put the anchor first as a transport invariant. The plugin-specific native
        # helper also gives it a dedicated retrieval vector, so this is not relied on
        # for semantic ranking, but makes the evidence bundle easier to inspect.
        nodes.insert(0, anchor)
        existing.add(anchor_id)

        if seed_principles:
            for key, statement in sorted(self.principles.items()):
                pid = f"cf-principle-{key}"
                if pid not in existing:
                    nodes.append(_node(
                        pid,
                        "Concept",
                        "Active",
                        statement,
                        "Observed",
                        turn,
                        {"kind": "counterfactual_principle", "principle": key},
                    ))
                    existing.add(pid)

        diagnostics: dict[str, Any] = {
            "plugin": self.name,
            "turn": turn,
            "comparison_anchor_id": anchor_id,
            "selected_move": None,
            "ranked_moves": None,
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

            # Every legal candidate must retain at least one traversable branch. A
            # neutral branch says only that no tracked immediate strategic delta was
            # detected; it does not imply the move is good.
            if not consequences:
                cid = f"cf2-ply-{turn}-{move.uci()}-00-neutral"
                nodes.append(_node(
                    cid,
                    "Outcome",
                    "Proposed",
                    (
                        f"After {move.uci()}, the plugin detected no immediate delta in its "
                        "tracked strategic/tactical features. This remains an unverified "
                        "counterfactual, not evidence that the move is preferable."
                    ),
                    "Hypothetical",
                    turn,
                    {
                        "kind": "counterfactual_consequence",
                        "principle": "neutral",
                        "signal": "no_tracked_immediate_delta",
                        "delta": 0.0,
                        "polarity": "neutral",
                        "uci": move.uci(),
                    },
                ))
                relations.append(_relation(hypothesis_id, cid, "Predictive", 0.55))
                relations.append(_relation(cid, anchor_id, "Supports", 0.50))
                row["consequences"].append({
                    "id": cid,
                    "principle": "neutral",
                    "signal": "no_tracked_immediate_delta",
                    "delta": 0.0,
                    "polarity": "neutral",
                    "confidence": 0.50,
                })

            for index, consequence in enumerate(consequences, start=1):
                cid = f"cf2-ply-{turn}-{move.uci()}-{index:02d}-{consequence.signal}"
                polarity = "support" if consequence.positive else "contradiction"
                nodes.append(_node(
                    cid,
                    "Outcome",
                    "Proposed",
                    consequence.statement,
                    "Hypothetical",
                    turn,
                    {
                        "kind": "counterfactual_consequence",
                        "principle": consequence.principle,
                        "signal": consequence.signal,
                        "delta": consequence.delta,
                        "polarity": polarity,
                        "uci": move.uci(),
                    },
                ))

                # Forward causal/predictive orientation. Native dialectic traversal
                # starts at the anchor and follows these incoming edges backwards.
                relations.append(_relation(hypothesis_id, cid, "Predictive", 0.82))
                role = "Supports" if consequence.positive else "Contradicts"
                confidence = _confidence(consequence.delta)
                relations.append(_relation(cid, anchor_id, role, confidence))

                # Principles remain knowledge context. They do not select the move.
                pid = f"cf-principle-{consequence.principle}"
                if consequence.principle in self.principles:
                    relations.append(_relation(pid, cid, "Mechanistic", 0.78))

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
                    "statement": consequence.statement,
                })

            diagnostics["candidates"].append(row)

        request["protocol"] = "agentfabric-chess-counterfactual-fiber-v2"
        request.setdefault("plugin_metadata", {})["counterfactual_plugin"] = self.name
        request["plugin_metadata"]["comparison_anchor_id"] = anchor_id
        request["plugin_metadata"]["required_query_mode"] = "Theoretical"
        return diagnostics
