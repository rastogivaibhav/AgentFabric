#!/usr/bin/env python3
"""GCDT-P4: selection-causality / permutation test.

No core GrapheneDB/HypoKosh changes. No engine, opening book or LLM selector.
The starting chess position and counterfactual evidence are held constant while
legal-move enumeration and hypothesis IDs are deterministically permuted.
A reasoning-driven selector should follow semantic evidence rather than index.
"""
from __future__ import annotations
import argparse, json, random, subprocess
from collections import Counter
from pathlib import Path
from typing import Any
import chess
from a15_arc_dialectic_adapter import invoke_native, stable_digest
from chess_counterfactual_plugin_v2 import ChessCounterfactualPluginV2
from graphene_chess_basic import flatten_rules, load_rules, node, position_statement, variation_snapshot, write_json


def build_permuted(board: chess.Board, turn: int, rules: list[str], moves: list[chess.Move]):
    nodes=[]; relations=[]; move_map={}; variations=[]
    pos_id=f"ply-{turn}-position"
    nodes.append(node(pos_id,"Fact","Active",position_statement(board),"Observed",turn,{"kind":"board_position"}))
    nodes.append(node("chess-objective","Decision","Active",rules[0],"Observed",turn,{"kind":"game_objective"}))
    for i,s in enumerate(rules[1:],1):
        nodes.append(node(f"chess-rule-{i:03d}","Fact","Active",s,"Observed",turn,{"kind":"chess_rule"}))
    for idx,move in enumerate(moves,1):
        hid=f"ply-{turn}-move-{idx:03d}-{move.uci()}"; oid=f"ply-{turn}-variation-{idx:03d}-{move.uci()}"
        snap=variation_snapshot(board,move); move_map[hid]=move; variations.append({"hypothesis_id":hid,"outcome_id":oid,**snap})
        nodes.append(node(hid,"Hypothesis","Proposed",f"Candidate White move {move.uci()} ({snap['san']}) is a legal next action and a possible step toward the stored objective.","Hypothetical",turn,{"kind":"candidate_chess_move","uci":move.uci(),"san":snap['san'],"side":"White"}))
        nodes.append(node(oid,"Outcome","Proposed",f"Counterfactual after {move.uci()}: FEN={snap['after_fen']}; capture={snap['capture']}; castling={snap['castling']}; gives_check={snap['gives_check']}; opponent_legal_reply_count={snap['opponent_legal_reply_count']}.","Hypothetical",turn,{"kind":"counterfactual_variation","uci":move.uci()}))
        relations += [
            {"from":pos_id,"to":hid,"role":"Supports","origin":"Observed","confidence":0.55},
            {"from":hid,"to":"chess-objective","role":"Predictive","origin":"Hypothetical","confidence":0.40},
            {"from":hid,"to":oid,"role":"Predictive","origin":"Hypothetical","confidence":0.75},
        ]
    return {"protocol":"agentfabric-chess-p4","operation":"ingest_and_reason","world_scope":"graphene-chess-p4","turn":turn,"nodes":nodes,"relations":relations},move_map,variations


def main():
    ap=argparse.ArgumentParser(); ap.add_argument('--native-helper',required=True); ap.add_argument('--bootstrap',required=True); ap.add_argument('--db-prefix',default='/tmp/graphene_chess_p4'); ap.add_argument('--rules',default='arc_competition/chess_rules.json'); ap.add_argument('--runs',type=int,default=20); ap.add_argument('--out',default='/tmp/graphene-chess-p4'); a=ap.parse_args()
    out=Path(a.out); out.mkdir(parents=True,exist_ok=True)
    raw:dict[str,Any]=load_rules(Path(a.rules)); rules=flatten_rules(raw); plugin=ChessCounterfactualPluginV2(raw)
    canonical=list(chess.Board().legal_moves); rows=[]
    for seed in range(a.runs):
        board=chess.Board(); moves=canonical.copy(); random.Random(seed).shuffle(moves)
        req,move_map,variations=build_permuted(board,0,rules,moves); req['world_scope']=f'graphene-chess-p4-seed-{seed}'
        diag=plugin.enrich(board=board,turn=0,request=req,move_by_hypothesis=move_map,seed_principles=True)
        db=f'{a.db_prefix}.seed-{seed}.graphenedb'; subprocess.run([a.bootstrap,db],check=True,capture_output=True,text=True)
        resp=invoke_native(a.native_helper,req,db); primary=str(resp.get('primary_hypothesis_id') or '')
        selected=move_map.get(primary); uci=selected.uci() if selected else None
        idx=None
        if primary:
            try: idx=int(primary.split('-move-')[1].split('-')[0])
            except Exception: pass
        fibers=resp.get('debug_fibers') or []
        pf=next((f for f in fibers if f.get('hypothesis_id')==primary),{})
        row={"seed":seed,"selected_uci":uci,"selected_index":idx,"primary_hypothesis_id":primary,"confidence":resp.get('confidence'),"primary_support_paths":pf.get('eligible_support_paths',0),"primary_best_support_quality":pf.get('best_support_quality',0.0),"first_candidate_uci":moves[0].uci(),"request_digest":stable_digest(req),"candidate_order":[m.uci() for m in moves]}
        rows.append(row); write_json(out/f'run_{seed:02d}.json',row); write_json(out/f'run_{seed:02d}_plugin.json',diag)
        print(seed,uci,idx,row['first_candidate_uci'],row['confidence'])
    counts=Counter(r['selected_uci'] for r in rows if r['selected_uci']); first_matches=sum(r['selected_uci']==r['first_candidate_uci'] for r in rows)
    dominant_move,dominant_count=(counts.most_common(1)[0] if counts else (None,0)); unique=len(counts)
    semantic_invariant=(dominant_count==a.runs and unique==1); index_one_rate=sum(r['selected_index']==1 for r in rows)/a.runs
    summary={"experiment":"GCDT-P4 selection causality permutation test","runs":a.runs,"all_native_selected":all(r['selected_uci'] for r in rows),"semantic_selection_counts":dict(counts),"dominant_move":dominant_move,"dominant_count":dominant_count,"unique_selected_moves":unique,"semantic_invariant":semantic_invariant,"first_candidate_match_count":first_matches,"first_candidate_match_rate":first_matches/a.runs,"index_one_rate":index_one_rate,"selection_causality_pass":semantic_invariant and first_matches < a.runs,"core_graphenedb_modified":False,"core_hypokosh_modified":False,"plugin_selected_move":False,"stockfish":False,"opening_book":False,"llm_move_selector":False}
    write_json(out/'summary.json',summary); print(json.dumps(summary,indent=2,sort_keys=True))
    if not summary['selection_causality_pass']:
        raise SystemExit(2)

if __name__=='__main__': main()
