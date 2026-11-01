#!/usr/bin/env python3
"""Plugin-only direct evidence fibers for chess selection-causality testing.

No GrapheneDB/HypoKosh core changes. The plugin derives immediate board consequences
for every legal move, preserves the consequence nodes for audit, and emits one direct
Hypothesis -> comparison-anchor evidence fiber per consequence. Edge confidence is the
confidence of that individual derived consequence. The plugin never selects, ranks, or
returns a preferred move.
"""
from __future__ import annotations
from typing import Any
import chess
from chess_counterfactual_plugin import _confidence, _derive, _node, _relation

class ChessCounterfactualPluginV4:
    name = "chess-counterfactual-direct-fibers-v4"

    def __init__(self, rules: dict[str, Any]):
        self.rules = rules
        self.principles = {}
        for section in ("strategic_principles", "tactical_principles"):
            for key, values in (rules.get(section) or {}).items():
                if isinstance(values, list) and values:
                    self.principles[str(key)] = " ".join(str(v) for v in values)

    def enrich(self, *, board: chess.Board, turn: int, request: dict[str, Any],
               move_by_hypothesis: dict[str, chess.Move], seed_principles: bool) -> dict[str, Any]:
        nodes=request.setdefault("nodes", []); relations=request.setdefault("relations", [])
        existing={str(n.get("external_id")) for n in nodes}
        anchor_id=f"cf2-ply-{turn}-comparison-anchor"
        nodes.insert(0,_node(anchor_id,"Outcome","Proposed",
            f"Current-turn theoretical comparison anchor for ply {turn}.",
            "Hypothetical",turn,{"kind":"counterfactual_comparison_anchor","plugin":self.name}))
        existing.add(anchor_id)
        if seed_principles:
            for key,statement in sorted(self.principles.items()):
                pid=f"cf-principle-{key}"
                if pid not in existing:
                    nodes.append(_node(pid,"Concept","Active",statement,"Observed",turn,
                        {"kind":"counterfactual_principle","principle":key}))
                    existing.add(pid)
        diag={"plugin":self.name,"turn":turn,"comparison_anchor_id":anchor_id,
              "selected_move":None,"ranked_moves":None,"candidate_count":len(move_by_hypothesis),"candidates":[]}
        for hid,move in move_by_hypothesis.items():
            after,consequences=_derive(board,move)
            row={"hypothesis_id":hid,"uci":move.uci(),"after_fen":after.fen(),"consequences":[]}
            if not consequences:
                cid=f"cf4-ply-{turn}-{move.uci()}-00-neutral"
                nodes.append(_node(cid,"Outcome","Proposed",
                    f"After {move.uci()}, no tracked immediate strategic/tactical delta was detected.",
                    "Hypothetical",turn,{"kind":"counterfactual_consequence","signal":"neutral","uci":move.uci()}))
                relations.append(_relation(hid,anchor_id,"Supports",0.50))
                row["consequences"].append({"id":cid,"signal":"neutral","delta":0.0,"polarity":"neutral","confidence":0.50})
            for i,c in enumerate(consequences,1):
                cid=f"cf4-ply-{turn}-{move.uci()}-{i:02d}-{c.signal}"
                polarity="support" if c.positive else "contradiction"; conf=_confidence(c.delta)
                nodes.append(_node(cid,"Outcome","Proposed",c.statement,"Hypothetical",turn,
                    {"kind":"counterfactual_consequence","principle":c.principle,"signal":c.signal,
                     "delta":c.delta,"polarity":polarity,"uci":move.uci()}))
                # Direct evidence fiber: confidence is the individual consequence strength.
                relations.append(_relation(hid,anchor_id,"Supports" if c.positive else "Contradicts",conf))
                # Audit/context links are deliberately not on the anchor traversal path.
                relations.append(_relation(hid,cid,"Predictive",conf))
                pid=f"cf-principle-{c.principle}"
                if c.principle in self.principles:
                    relations.append(_relation(pid,cid,"Mechanistic",0.78))
                row["consequences"].append({"id":cid,"principle":c.principle,"signal":c.signal,
                    "delta":c.delta,"polarity":polarity,"confidence":conf,"statement":c.statement})
            diag["candidates"].append(row)
        request["protocol"]="agentfabric-chess-counterfactual-direct-fibers-v4"
        request.setdefault("plugin_metadata",{})["counterfactual_plugin"]=self.name
        request["plugin_metadata"]["comparison_anchor_id"]=anchor_id
        request["plugin_metadata"]["required_query_mode"]="Theoretical"
        request["plugin_metadata"]["plugin_selects_move"]=False
        return diag
