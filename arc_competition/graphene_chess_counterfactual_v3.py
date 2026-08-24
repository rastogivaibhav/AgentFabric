#!/usr/bin/env python3
"""Plugin-only chess counterfactual test v3: query-scoped reasoning projections.

GrapheneDB/HypoKosh core remains unchanged. Each ply is reasoned in a fresh GrapheneDB
projection containing the stored chess knowledge, current board, current legal move
hypotheses and plugin counterfactuals only. This prevents historical hypotheses from
competing with the current action set while preserving the canonical corpus as input.
"""
from __future__ import annotations
import argparse, json, subprocess
from pathlib import Path
from typing import Any
import chess
from a15_arc_dialectic_adapter import invoke_native, stable_digest, NativeRuntimeError
from chess_counterfactual_plugin_v2 import ChessCounterfactualPluginV2
from graphene_chess_basic import build_request, flatten_rules, load_rules, write_json

WORLD_SCOPE="graphene-chess-counterfactual-v3"

def main():
    ap=argparse.ArgumentParser()
    ap.add_argument('--native-helper',default='/tmp/chess_counterfactual_native_helper')
    ap.add_argument('--bootstrap',default='/tmp/a15_graphene_bootstrap')
    ap.add_argument('--db-prefix',default='/tmp/graphene_chess_v3')
    ap.add_argument('--rules',default='arc_competition/chess_rules.json')
    ap.add_argument('--plies',type=int,default=8)
    ap.add_argument('--out',default='/tmp/graphene-chess-plugin-v3')
    a=ap.parse_args(); out=Path(a.out); out.mkdir(parents=True,exist_ok=True)
    raw:dict[str,Any]=load_rules(Path(a.rules)); rules=flatten_rules(raw)
    plugin=ChessCounterfactualPluginV2(raw); board=chess.Board(); game=[]
    for ply in range(a.plies):
        if board.is_game_over(claim_draw=True): break
        # Every query projection contains the same canonical stored rules/principles,
        # but only the current board and current legal move hypotheses.
        request, move_map, variations=build_request(board,ply,rules,seed_rules=True)
        request['world_scope']=f'{WORLD_SCOPE}-ply-{ply}'
        diag=plugin.enrich(board=board,turn=ply,request=request,move_by_hypothesis=move_map,seed_principles=True)
        db=f'{a.db_prefix}.ply-{ply}.graphenedb'
        subprocess.run([a.bootstrap,db],check=True,capture_output=True,text=True)
        write_json(out/f'ply_{ply+1:02d}_request.json',request); write_json(out/f'ply_{ply+1:02d}_plugin.json',diag); write_json(out/f'ply_{ply+1:02d}_variations.json',variations)
        response=invoke_native(a.native_helper,request,db); write_json(out/f'ply_{ply+1:02d}_native_response.json',response)
        primary=str(response.get('primary_hypothesis_id') or '')
        if primary not in move_map:
            raise NativeRuntimeError(f'native primary is not a current legal move: {primary!r}')
        move=move_map[primary]; san=board.san(move); before=board.fen(); board.push(move)
        row={'ply':ply+1,'side':'White' if ply%2==0 else 'Black','uci':move.uci(),'san':san,'selected_hypothesis_id':primary,'primary_hypothesis_id':primary,'primary_node':response.get('primary_node'),'confidence':response.get('confidence'),'epistemic_status':response.get('epistemic_status'),'before_fen':before,'after_fen':board.fen(),'legal_candidate_count':len(move_map),'debug_fibers':response.get('debug_fibers') or [],'reasoning_receipt':response.get('reasoning_receipt') or {},'request_digest':stable_digest(request),'projection_db':db}
        game.append(row); write_json(out/f'ply_{ply+1:02d}.json',row)
        print(f"{ply+1}. {row['side']}: {san} [{move.uci()}] primary={primary} confidence={row['confidence']}")
    summary={'experiment':'Graphene Chess Counterfactual Plugin v3','plugin':plugin.name,'projection_isolation':True,'canonical_rules_reseeded_per_projection':True,'core_graphenedb_modified':False,'core_hypokosh_modified':False,'plugin_selected_move':False,'llm_move_selector':False,'opening_book':False,'stockfish':False,'requested_plies':a.plies,'completed_plies':len(game),'all_moves_legal':len(game)==a.plies,'moves':[{k:r[k] for k in ('ply','side','uci','san','selected_hypothesis_id','confidence')} for r in game],'final_fen':board.fen()}
    write_json(out/'summary.json',summary); print(json.dumps(summary,indent=2,sort_keys=True))
if __name__=='__main__': main()
