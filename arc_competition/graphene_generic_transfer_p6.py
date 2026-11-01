#!/usr/bin/env python3
from __future__ import annotations
import argparse, json, random, subprocess
from collections import Counter
from pathlib import Path
from typing import Any

from a15_arc_dialectic_adapter import invoke_native, stable_digest
from generic_counterfactual_plugin_v1 import GenericCounterfactualPluginV1, node, relation

ACTIONS = ("A0", "A1", "A2", "A3", "A4")
START = {"goal_distance": 5.0, "resource": 3.0, "hazard": 1.0, "information": 0.0}


def transition(state: dict[str, float], action: str) -> dict[str, float]:
    s = dict(state)
    if action == "A0":
        s["goal_distance"] = max(0.0, s["goal_distance"] - 1); s["resource"] -= 1; s["information"] += 1
    elif action == "A1":
        s["goal_distance"] = max(0.0, s["goal_distance"] - 2); s["resource"] -= 1
    elif action == "A2":
        s["goal_distance"] = max(0.0, s["goal_distance"] - 2); s["resource"] -= 1; s["hazard"] += 1; s["information"] += 2
    elif action == "A3":
        s["resource"] += 1; s["information"] += 1
    elif action == "A4":
        s["goal_distance"] = max(0.0, s["goal_distance"] - 1); s["hazard"] = max(0.0, s["hazard"] - 1); s["information"] += 2
    else:
        raise ValueError(action)
    return s


def build_request(state: dict[str, float], turn: int, ordered_actions: list[str], scope: str):
    nodes = [node(f"generic-state-{turn}", "Fact", "Active", f"Current opaque world state={json.dumps(state, sort_keys=True)}", "Observed", turn, {"kind": "generic_state"}), node("generic-objective", "Decision", "Active", "Prefer futures with smaller goal_distance, more resource, less hazard, and more information.", "Observed", turn, {"kind": "generic_objective"})]
    relations = []
    action_by_hid: dict[str, str] = {}; next_by_hid: dict[str, dict[str, float]] = {}
    for idx, action in enumerate(ordered_actions, 1):
        hid = f"generic-turn-{turn}-action-{idx:03d}-{action}"
        action_by_hid[hid] = action; next_by_hid[hid] = transition(state, action)
        nodes.append(node(hid, "Hypothesis", "Proposed", f"Opaque candidate {action} may advance the stored objective.", "Hypothetical", turn, {"kind": "opaque_candidate", "action": action}))
        relations.append(relation(f"generic-state-{turn}", hid, "Supports", 0.55, "Observed"))
    req = {"protocol": "agentfabric-generic-transfer-p6", "operation": "ingest_and_reason", "world_scope": scope, "turn": turn, "nodes": nodes, "relations": relations}
    return req, action_by_hid, next_by_hid


def set_support(req: dict[str, Any], hid: str, value: float):
    anchor = req["plugin_metadata"]["comparison_anchor_id"]
    for r in req["relations"]:
        if r["from"] == hid and r["to"] == anchor and r["role"] == "Supports": r["confidence"] = float(value)


def run_once(args, plugin, state, ordered_actions, scope, turn=0, intervention=None):
    req, action_by_hid, next_by_hid = build_request(state, turn, ordered_actions, scope)
    diag = plugin.enrich(state=state, turn=turn, request=req, next_state_by_hypothesis=next_by_hid)
    if intervention:
        hid = next(h for h,a in action_by_hid.items() if a == intervention[0]); set_support(req, hid, intervention[1])
    db = f"{args.db_prefix}.{scope}.graphenedb"; subprocess.run([args.bootstrap, db], check=True, capture_output=True, text=True)
    resp = invoke_native(args.native_helper, req, db); primary = str(resp.get("primary_hypothesis_id") or ""); action = action_by_hid.get(primary)
    return {"selected_action": action, "primary_hypothesis_id": primary, "confidence": resp.get("confidence"), "request_digest": stable_digest(req), "plugin": diag, "native": resp}


def main():
    ap=argparse.ArgumentParser(); ap.add_argument('--native-helper',required=True); ap.add_argument('--bootstrap',required=True); ap.add_argument('--db-prefix',default='/tmp/graphene_generic_p6'); ap.add_argument('--runs',type=int,default=20); ap.add_argument('--out',default='/tmp/graphene-generic-p6'); a=ap.parse_args(); out=Path(a.out); out.mkdir(parents=True,exist_ok=True); plugin=GenericCounterfactualPluginV1()

    rows=[]
    for seed in range(a.runs):
        ordered=list(ACTIONS); random.Random(seed).shuffle(ordered); row=run_once(a,plugin,START,ordered,f'p6-perm-{seed}'); row.update({"seed":seed,"first_action":ordered[0]}); rows.append(row); print('perm',seed,row['selected_action'],ordered[0],row['confidence'])
    counts=Counter(r['selected_action'] for r in rows); dominant,dom_count=counts.most_common(1)[0] if counts else (None,0); first_matches=sum(r['selected_action']==r['first_action'] for r in rows)
    permutation_pass=(dom_count==a.runs and len(counts)==1 and first_matches<a.runs)

    ablated=[]; restored=[]
    for seed in range(10):
        ordered=list(ACTIONS); random.Random(seed).shuffle(ordered)
        r=run_once(a,plugin,START,ordered,f'p6-ablate-{seed}',intervention=(dominant,0.05)); ablated.append(r['selected_action'])
        rr=run_once(a,plugin,START,ordered,f'p6-restore-{seed}'); restored.append(rr['selected_action'])
    intervention_pass=all(x!=dominant for x in ablated) and all(x==dominant for x in restored)

    state=dict(START); game=[]
    for turn in range(4):
        ordered=list(ACTIONS); random.Random(100+turn).shuffle(ordered); r=run_once(a,plugin,state,ordered,f'p6-loop-{turn}',turn=turn); action=r['selected_action']
        if action is None: break
        before=dict(state); state=transition(state,action); game.append({"turn":turn,"action":action,"confidence":r['confidence'],"before":before,"after":dict(state)})
        print('loop',turn,action,r['confidence'],state)
    loop_pass=len(game)==4 and all(g['action'] in ACTIONS for g in game)

    summary={"experiment":"P6A opaque-world transfer","permutation_runs":a.runs,"selection_counts":dict(counts),"dominant_action":dominant,"dominant_count":dom_count,"first_candidate_match_rate":first_matches/a.runs,"permutation_pass":permutation_pass,"ablation_selected":ablated,"restore_selected":restored,"intervention_pass":intervention_pass,"closed_loop":game,"closed_loop_pass":loop_pass,"transfer_pass":permutation_pass and intervention_pass and loop_pass,"chess_semantics":False,"plugin_selected_action":False,"llm_selector":False,"engine":False,"core_graphenedb_modified":False,"core_hypokosh_modified":False}
    (out/'summary.json').write_text(json.dumps(summary,indent=2,sort_keys=True)); print(json.dumps(summary,indent=2,sort_keys=True)); raise SystemExit(0 if summary['transfer_pass'] else 2)

if __name__=='__main__': main()
