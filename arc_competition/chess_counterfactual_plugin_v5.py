#!/usr/bin/env python3
"""Plugin-only whole-evidence synthesis for chess hypothesis comparison.

For each legal move the plugin derives the same immediate counterfactual consequences
as earlier versions, then maps the whole evidence set into one support confidence:
mean(positive consequence confidences) - 0.25 * mean(contradiction confidences).
This is domain-generic evidence aggregation: no move name, opening book, engine score,
or final move selection is used. Individual consequence and contradiction fibers are
retained for audit/opposition.
"""
from __future__ import annotations
from typing import Any
import chess
from chess_counterfactual_plugin import _confidence, _derive, _node, _relation

class ChessCounterfactualPluginV5:
    name="chess-counterfactual-whole-evidence-v5"
    def __init__(self,rules:dict[str,Any]):
        self.rules=rules; self.principles={}
        for section in ("strategic_principles","tactical_principles"):
            for key,values in (rules.get(section) or {}).items():
                if isinstance(values,list) and values: self.principles[str(key)]=" ".join(str(v) for v in values)
    @staticmethod
    def synthesize(positive:list[float],negative:list[float])->float:
        pos=sum(positive)/len(positive) if positive else 0.50
        neg=sum(negative)/len(negative) if negative else 0.0
        return max(0.05,min(0.99,pos-0.25*neg))
    def enrich(self,*,board:chess.Board,turn:int,request:dict[str,Any],move_by_hypothesis:dict[str,chess.Move],seed_principles:bool)->dict[str,Any]:
        nodes=request.setdefault("nodes",[]); relations=request.setdefault("relations",[]); existing={str(n.get("external_id")) for n in nodes}
        anchor_id=f"cf2-ply-{turn}-comparison-anchor"; nodes.insert(0,_node(anchor_id,"Outcome","Proposed",f"Current-turn theoretical comparison anchor for ply {turn}.","Hypothetical",turn,{"kind":"counterfactual_comparison_anchor","plugin":self.name})); existing.add(anchor_id)
        if seed_principles:
            for key,statement in sorted(self.principles.items()):
                pid=f"cf-principle-{key}"
                if pid not in existing: nodes.append(_node(pid,"Concept","Active",statement,"Observed",turn,{"kind":"counterfactual_principle","principle":key})); existing.add(pid)
        diag={"plugin":self.name,"turn":turn,"comparison_anchor_id":anchor_id,"selected_move":None,"ranked_moves":None,"candidate_count":len(move_by_hypothesis),"candidates":[]}
        for hid,move in move_by_hypothesis.items():
            after,cs=_derive(board,move); positive=[]; negative=[]; row={"hypothesis_id":hid,"uci":move.uci(),"after_fen":after.fen(),"consequences":[]}
            for i,c in enumerate(cs,1):
                conf=_confidence(c.delta); polarity="support" if c.positive else "contradiction"; (positive if c.positive else negative).append(conf)
                cid=f"cf5-ply-{turn}-{move.uci()}-{i:02d}-{c.signal}"; nodes.append(_node(cid,"Outcome","Proposed",c.statement,"Hypothetical",turn,{"kind":"counterfactual_consequence","principle":c.principle,"signal":c.signal,"delta":c.delta,"polarity":polarity,"uci":move.uci()})); relations.append(_relation(hid,cid,"Predictive",conf))
                pid=f"cf-principle-{c.principle}"
                if c.principle in self.principles: relations.append(_relation(pid,cid,"Mechanistic",0.78))
                if not c.positive: relations.append(_relation(hid,anchor_id,"Contradicts",conf))
                row["consequences"].append({"id":cid,"principle":c.principle,"signal":c.signal,"delta":c.delta,"polarity":polarity,"confidence":conf,"statement":c.statement})
            aggregate=self.synthesize(positive,negative); relations.append(_relation(hid,anchor_id,"Supports",aggregate)); row["aggregate_support_confidence"]=aggregate; row["positive_evidence_count"]=len(positive); row["contradiction_count"]=len(negative); diag["candidates"].append(row)
        request["protocol"]="agentfabric-chess-counterfactual-whole-evidence-v5"; request.setdefault("plugin_metadata",{})["counterfactual_plugin"]=self.name; request["plugin_metadata"]["comparison_anchor_id"]=anchor_id; request["plugin_metadata"]["required_query_mode"]="Theoretical"; request["plugin_metadata"]["plugin_selects_move"]=False; request["plugin_metadata"]["aggregation"]="mean_positive_minus_0.25_mean_contradiction"
        return diag
