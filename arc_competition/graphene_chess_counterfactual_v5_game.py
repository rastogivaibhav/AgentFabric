#!/usr/bin/env python3
"""8-ply chess game using plugin-v5 whole-evidence synthesis and unchanged native HypoKosh."""
from __future__ import annotations
import argparse, subprocess
from pathlib import Path
from typing import Any
import chess
from a15_arc_dialectic_adapter import invoke_native, NativeRuntimeError, stable_digest
from chess_counterfactual_plugin_v5 import ChessCounterfactualPluginV5
from graphene_chess_basic import build_request, flatten_rules, load_rules, write_json

def main():
    ap=argparse.ArgumentParser(); ap.add_argument('--native-helper',required=True); ap.add_argument('--bootstrap',required=True); ap.add_argument('--db-prefix',default='/tmp/graphene_chess_v5_game'); ap.add_argument('--rules',default='arc_competition/chess_rules.json'); ap.add_argument('--plies',type=int,default=8); ap.add_argument('--out',default='/tmp/graphene-chess-v5-game'); a=ap.parse_args(); out=Path(a.out); out.mkdir(parents=True,exist_ok=True)
    raw:dict[str,Any]=load_rules(Path(a.rules)); rules=flatten_rules(raw); plugin=ChessCounterfactualPluginV5(raw); board=chess.Board(); game=[]
    for ply in range(a.plies):
        request,move_map,variations=build_request(board,ply,rules,seed_rules=True); request['world_scope']=f'graphene-chess-v5-game-ply-{ply}'; diag=plugin.enrich(board=board,turn=ply,request=request,move_by_hypothesis=move_map,seed_principles=True); db=f'{a.db_prefix}.ply-{ply}.graphenedb'; subprocess.run([a.bootstrap,db],check=True,capture_output=True,text=True); resp=invoke_native(a.native_helper,request,db); primary=str(resp.get('primary_hypothesis_id') or '')
        if primary not in move_map: raise NativeRuntimeError(f'native primary not current legal move: {primary!r}')
        move=move_map[primary]; san=board.san(move); before=board.fen(); board.push(move); selected_diag=next(c for c in diag['candidates'] if c['hypothesis_id']==primary); row={"ply":ply+1,"side":"White" if ply%2==0 else "Black","uci":move.uci(),"san":san,"primary_hypothesis_id":primary,"confidence":resp.get('confidence'),"aggregate_support_confidence":selected_diag.get('aggregate_support_confidence'),"before_fen":before,"after_fen":board.fen(),"legal_candidate_count":len(move_map),"request_digest":stable_digest(request)}; game.append(row); write_json(out/f'ply_{ply+1:02d}.json',row); write_json(out/f'ply_{ply+1:02d}_native.json',resp); write_json(out/f'ply_{ply+1:02d}_plugin.json',diag); print(ply+1,row['side'],san,move.uci(),row['confidence'],row['aggregate_support_confidence'])
    summary={"experiment":"Graphene Chess v5 8-ply game","completed_plies":len(game),"requested_plies":a.plies,"all_native_selected":len(game)==a.plies,"moves":[{k:r[k] for k in ('ply','side','uci','san','confidence','aggregate_support_confidence')} for r in game],"final_fen":board.fen(),"core_graphenedb_modified":False,"core_hypokosh_modified":False,"plugin_selected_move":False,"stockfish":False,"opening_book":False,"llm_move_selector":False}; write_json(out/'summary.json',summary); print(summary); raise SystemExit(0 if summary['all_native_selected'] else 2)
if __name__=='__main__': main()
