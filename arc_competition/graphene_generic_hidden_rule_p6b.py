#!/usr/bin/env python3
from __future__ import annotations
import argparse, json, random, subprocess
from collections import Counter
from pathlib import Path
from typing import Any

from a15_arc_dialectic_adapter import invoke_native, stable_digest
from generic_counterfactual_plugin_v1 import node, relation
from generic_hidden_rule_plugin_v1 import GenericHiddenRulePluginV1, FIELDS

ACTIONS=("A0","A1","A2","A3","A4")
START={"goal_distance":5.0,"resource":3.0,"hazard":1.0,"information":0.0}

# Hidden environment. This function is deliberately never passed to the plugin.
def hidden_transition(state:dict[str,float],action:str)->dict[str,float]:
    s=dict(state)
    if action=="A0": s["goal_distance"]=max(0.0,s["goal_distance"]-1); s["resource"]-=1; s["information"]+=1
    elif action=="A1": s["goal_distance"]=max(0.0,s["goal_distance"]-2); s["resource"]-=1
    elif action=="A2": s["goal_distance"]=max(0.0,s["goal_distance"]-2); s["resource"]-=1; s["hazard"]+=1; s["information"]+=2
    elif action=="A3": s["resource"]+=1; s["information"]+=1
    elif action=="A4": s["goal_distance"]=max(0.0,s["goal_distance"]-1); s["hazard"]=max(0.0,s["hazard"]-1); s["information"]+=2
    else: raise ValueError(action)
    return s


def build_request(state:dict[str,float],turn:int,ordered:list[str],scope:str):
    sid=f"hidden-state-{turn}"
    nodes=[
        node(sid,"Fact","Active",f"Current opaque state={json.dumps(state,sort_keys=True)}","Observed",turn,{"kind":"hidden_world_state"}),
        node("hidden-objective","Decision","Active","Prefer smaller goal_distance, more resource, less hazard, and more information, while learning unknown action rules from observed transitions.","Observed",turn,{"kind":"hidden_world_objective"}),
    ]
    rels=[]; action_by_hid={}
    for idx,action in enumerate(ordered,1):
        hid=f"hidden-turn-{turn}-action-{idx:03d}-{action}"; action_by_hid[hid]=action
        nodes.append(node(hid,"Hypothesis","Proposed",f"Opaque action {action} is a candidate experiment/action under the current learned world model.","Hypothetical",turn,{"kind":"opaque_candidate","action":action}))
        rels.append(relation(sid,hid,"Supports",0.55,"Observed"))
    return {"protocol":"agentfabric-generic-hidden-rule-p6b","operation":"ingest_and_reason","world_scope":scope,"turn":turn,"nodes":nodes,"relations":rels},action_by_hid


def run_reason(args,plugin,state,history,ordered,scope,turn):
    req,action_by_hid=build_request(state,turn,ordered,scope)
    diag=plugin.enrich(state=state,actions=ordered,turn=turn,request=req,action_by_hypothesis=action_by_hid,history=history)
    db=f"{args.db_prefix}.{scope}.graphenedb"; subprocess.run([args.bootstrap,db],check=True,capture_output=True,text=True)
    resp=invoke_native(args.native_helper,req,db); primary=str(resp.get("primary_hypothesis_id") or ""); action=action_by_hid.get(primary)
    return {"selected_action":action,"primary_hypothesis_id":primary,"confidence":resp.get("confidence"),"request_digest":stable_digest(req),"plugin":diag,"native":resp}


def true_delta(action:str)->dict[str,float]:
    base={"goal_distance":10.0,"resource":10.0,"hazard":5.0,"information":10.0}
    after=hidden_transition(base,action)
    return {f:after[f]-base[f] for f in FIELDS}


def main():
    ap=argparse.ArgumentParser(); ap.add_argument('--native-helper',required=True); ap.add_argument('--bootstrap',required=True); ap.add_argument('--db-prefix',default='/tmp/graphene_generic_p6b'); ap.add_argument('--runs',type=int,default=10); ap.add_argument('--out',default='/tmp/graphene-generic-p6b'); a=ap.parse_args(); out=Path(a.out); out.mkdir(parents=True,exist_ok=True); plugin=GenericHiddenRulePluginV1()

    # Phase 1: hidden-rule exploration. No counterfactual oracle. Untried actions only
    # receive uncertainty/information-gain evidence. We require all 5 actions to be
    # sampled within 5 turns despite candidate-order permutations.
    exploration_runs=[]
    for seed in range(a.runs):
        state=dict(START); history=[]; trace=[]
        for turn in range(5):
            ordered=list(ACTIONS); random.Random(seed*100+turn).shuffle(ordered)
            r=run_reason(a,plugin,state,history,ordered,f'p6b-explore-{seed}-{turn}',turn)
            action=r['selected_action']
            if action is None: break
            before=dict(state); state=hidden_transition(state,action); event={"turn":turn,"action":action,"before":before,"after":dict(state)}; history.append(event)
            trace.append({"turn":turn,"selected_action":action,"confidence":r['confidence'],"first_candidate":ordered[0],"before":before,"after":dict(state),"learned_rules_before":r['plugin']['learned_rules']})
        learned=plugin.learn(history); covered=set(e['action'] for e in history)
        accuracy={}
        for action in ACTIONS:
            est=(learned.get(action) or {}).get('mean_delta'); truth=true_delta(action)
            accuracy[action]=bool(est is not None and all(abs(float(est[f])-float(truth[f]))<1e-9 for f in FIELDS))
        exploration_runs.append({"seed":seed,"trace":trace,"coverage":sorted(covered),"coverage_pass":covered==set(ACTIONS),"rule_accuracy":accuracy,"rule_accuracy_pass":all(accuracy.values()),"history":history})
        print('explore',seed,[x['selected_action'] for x in trace],sorted(covered),all(accuracy.values()))

    exploration_pass=all(r['coverage_pass'] and r['rule_accuracy_pass'] for r in exploration_runs)

    # Phase 2: transfer learned rules to a fresh state. Build predictions only from
    # history, then compare against the hidden environment after selection.
    exploit_rows=[]
    for seed,r0 in enumerate(exploration_runs):
        history=r0['history']; fresh={"goal_distance":7.0,"resource":4.0,"hazard":2.0,"information":1.0}; ordered=list(ACTIONS); random.Random(900+seed).shuffle(ordered)
        r=run_reason(a,plugin,fresh,history,ordered,f'p6b-exploit-{seed}',10)
        action=r['selected_action']; model=plugin.learn(history); predicted=None; actual=None
        if action and action in model:
            predicted={f:fresh[f]+model[action]['mean_delta'][f] for f in FIELDS}; predicted['goal_distance']=max(0.0,predicted['goal_distance']); predicted['hazard']=max(0.0,predicted['hazard'])
            actual=hidden_transition(fresh,action)
        pred_ok=bool(predicted is not None and all(abs(float(predicted[f])-float(actual[f]))<1e-9 for f in FIELDS))
        exploit_rows.append({"seed":seed,"selected_action":action,"first_candidate":ordered[0],"confidence":r['confidence'],"predicted_after":predicted,"actual_after":actual,"prediction_pass":pred_ok})
        print('exploit',seed,action,ordered[0],r['confidence'],pred_ok)

    counts=Counter(x['selected_action'] for x in exploit_rows); dominant,dom_count=counts.most_common(1)[0] if counts else (None,0); first_matches=sum(x['selected_action']==x['first_candidate'] for x in exploit_rows)
    exploit_pass=(dom_count==a.runs and dominant=='A4' and first_matches<a.runs and all(x['prediction_pass'] for x in exploit_rows))

    # Phase 3: closed-loop exploitation from a fresh state using only learned rules.
    seed_history=exploration_runs[0]['history']; state={"goal_distance":4.0,"resource":4.0,"hazard":1.0,"information":0.0}; loop=[]
    for turn in range(4):
        ordered=list(ACTIONS); random.Random(1200+turn).shuffle(ordered); r=run_reason(a,plugin,state,seed_history,ordered,f'p6b-loop-{turn}',20+turn); action=r['selected_action']
        if action is None: break
        before=dict(state); after=hidden_transition(state,action); loop.append({"turn":turn,"action":action,"confidence":r['confidence'],"before":before,"after":after}); state=after
        print('loop',turn,action,r['confidence'],state)
    loop_pass=len(loop)==4 and all(x['action']=='A4' for x in loop) and state['goal_distance']==0.0

    summary={
        "experiment":"P6B hidden-rule discovery",
        "runs":a.runs,
        "oracle_next_states_supplied_to_plugin":False,
        "exploration_pass":exploration_pass,
        "exploration_runs":[{"seed":x['seed'],"actions":[t['selected_action'] for t in x['trace']],"coverage":x['coverage'],"coverage_pass":x['coverage_pass'],"rule_accuracy_pass":x['rule_accuracy_pass']} for x in exploration_runs],
        "exploit_selection_counts":dict(counts),"dominant_action":dominant,"dominant_count":dom_count,"first_candidate_match_rate":first_matches/a.runs,
        "prediction_pass":all(x['prediction_pass'] for x in exploit_rows),"exploit_pass":exploit_pass,"closed_loop":loop,"closed_loop_pass":loop_pass,
        "hidden_rule_discovery_pass":exploration_pass and exploit_pass and loop_pass,
        "plugin_selected_action":False,"llm_selector":False,"engine":False,"chess_semantics":False,"core_graphenedb_modified":False,"core_hypokosh_modified":False,
    }
    (out/'summary.json').write_text(json.dumps(summary,indent=2,sort_keys=True)); (out/'exploration.json').write_text(json.dumps(exploration_runs,indent=2,sort_keys=True)); (out/'exploit.json').write_text(json.dumps(exploit_rows,indent=2,sort_keys=True)); print(json.dumps(summary,indent=2,sort_keys=True)); raise SystemExit(0 if summary['hidden_rule_discovery_pass'] else 2)

if __name__=='__main__': main()
