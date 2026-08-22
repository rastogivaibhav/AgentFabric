#!/usr/bin/env python3
from __future__ import annotations
import argparse,json,random,subprocess
from collections import Counter
from pathlib import Path
from typing import Any

from a15_arc_dialectic_adapter import invoke_native,stable_digest
from generic_counterfactual_plugin_v1 import node,relation
from generic_goal_discovery_plugin_v1 import GenericGoalDiscoveryPluginV1,FIELDS

ACTIONS=("A0","A1","A2","A3","A4")
GOAL_SPECS=tuple((field,direction) for field in FIELDS for direction in (+1,-1))
WORLDS={
    "W-X2":{"goal_field":"X2","goal_direction":+1,"expected_action":"A2"},
    "W-X3":{"goal_field":"X3","goal_direction":+1,"expected_action":"A3"},
}


def hidden_transition(state:dict[str,float],action:str)->dict[str,float]:
    s=dict(state)
    if action=="A0": s["X0"]+=2
    elif action=="A1": s["X1"]+=2
    elif action=="A2": s["X2"]+=2
    elif action=="A3": s["X3"]+=2
    elif action=="A4": s["X0"]+=1; s["X2"]+=1
    else: raise ValueError(action)
    return s


def hidden_success(state:dict[str,float],world:dict[str,Any])->bool:
    # Hidden from the plugin. Only the boolean terminal outcome is exposed.
    return float(state[world["goal_field"]])>=6.0


def collect_interactions(world_name:str,count:int=80)->tuple[list[dict[str,Any]],list[dict[str,Any]]]:
    world=WORLDS[world_name]; transitions=[]; outcomes=[]
    for i in range(count):
        rng=random.Random(70000+(0 if world_name=="W-X2" else 10000)+i)
        before={f:float(rng.randint(0,8)) for f in FIELDS}
        # Start below the unknown terminal boundary so success is produced by interaction.
        before[world["goal_field"]]=float(rng.randint(3,5))
        action=ACTIONS[i%len(ACTIONS)]
        after=hidden_transition(before,action)
        success=hidden_success(after,world)
        transitions.append({"episode":i,"action":action,"before":before,"after":after,"success":success})
        outcomes.append({"episode":i,"final_state":after,"success":success})
    if not any(o['success'] for o in outcomes) or all(o['success'] for o in outcomes):
        raise RuntimeError('goal evidence must contain successful and unsuccessful episodes')
    return transitions,outcomes


def bootstrap(args,scope:str)->str:
    db=f"{args.db_prefix}.{scope}.graphenedb"
    subprocess.run([args.bootstrap,db],check=True,capture_output=True,text=True)
    return db


def build_goal_request(outcomes:list[dict[str,Any]],ordered:list[tuple[str,int]],turn:int,scope:str):
    nodes=[]; rels=[]; goal_by_hid={}
    for o in outcomes:
        eid=f"p6d-outcome-{o['episode']:03d}"
        nodes.append(node(eid,'Fact','Active',f"Observed episode terminal result: success={bool(o['success'])}; final opaque state={json.dumps(o['final_state'],sort_keys=True)}.",'Observed',turn,{"kind":"terminal_outcome","success":bool(o['success']),"episode":o['episode']}))
    for idx,(field,direction) in enumerate(ordered,1):
        label='increase' if direction>0 else 'decrease'
        hid=f"p6d-goal-{idx:03d}-{field}-{'up' if direction>0 else 'down'}"
        goal_by_hid[hid]={"field":field,"direction":direction,"hypothesis_id":hid}
        nodes.append(node(hid,'Hypothesis','Proposed',f"Latent goal hypothesis: successful terminal states prefer {label} {field}.",'Hypothetical',turn,{"kind":"candidate_goal_hypothesis","field":field,"direction":direction}))
    return {"protocol":"agentfabric-p6d-goal-discovery","operation":"ingest_and_reason","world_scope":scope,"turn":turn,"nodes":nodes,"relations":rels},goal_by_hid


def discover_goal(args,plugin,outcomes,ordered,scope,turn):
    req,gmap=build_goal_request(outcomes,ordered,turn,scope)
    diag=plugin.enrich_goal_request(turn=turn,request=req,goal_by_hypothesis=gmap,outcomes=outcomes)
    resp=invoke_native(args.native_helper,req,bootstrap(args,scope))
    pid=str(resp.get('primary_hypothesis_id') or '')
    selected=gmap.get(pid)
    return {"selected_goal":selected,"primary_hypothesis_id":pid,"confidence":resp.get('confidence'),"diag":diag,"native":resp,"request_digest":stable_digest(req)}


def build_action_request(state,ordered,turn,scope):
    sid=f"p6d-state-{turn}"; nodes=[node(sid,'Fact','Active',f"Current opaque state={json.dumps(state,sort_keys=True)}.",'Observed',turn,{"kind":"opaque_state"})]; rels=[]; amap={}
    for idx,action in enumerate(ordered,1):
        hid=f"p6d-action-{turn}-{idx:03d}-{action}"; amap[hid]=action
        nodes.append(node(hid,'Hypothesis','Proposed',f"Opaque action {action} is a candidate under the discovered goal model.",'Hypothetical',turn,{"kind":"candidate_action","action":action}))
        rels.append(relation(sid,hid,'Supports',0.55,'Observed'))
    return {"protocol":"agentfabric-p6d-action","operation":"ingest_and_reason","world_scope":scope,"turn":turn,"nodes":nodes,"relations":rels},amap


def choose_action(args,plugin,state,history,goal,ordered,scope,turn):
    req,amap=build_action_request(state,ordered,turn,scope)
    diag=plugin.enrich_action_request(turn=turn,state=state,request=req,action_by_hypothesis=amap,transition_history=history,selected_goal=goal)
    resp=invoke_native(args.native_helper,req,bootstrap(args,scope))
    pid=str(resp.get('primary_hypothesis_id') or '')
    return {"selected_action":amap.get(pid),"primary_hypothesis_id":pid,"confidence":resp.get('confidence'),"diag":diag,"native":resp,"request_digest":stable_digest(req)}


def run_world(args,world_name:str,plugin)->dict[str,Any]:
    world=WORLDS[world_name]; history,outcomes=collect_interactions(world_name)
    goal_rows=[]; action_rows=[]
    for seed in range(args.runs):
        goals=list(GOAL_SPECS); random.Random(1000+seed+(0 if world_name=='W-X2' else 100)).shuffle(goals)
        g=discover_goal(args,plugin,outcomes,goals,f'p6d-{world_name}-goal-{seed}',10+seed)
        sem=None if g['selected_goal'] is None else (g['selected_goal']['field'],g['selected_goal']['direction'])
        goal_rows.append({"seed":seed,"selected_goal":sem,"confidence":g['confidence'],"first_goal":goals[0]})
        if g['selected_goal'] is None:
            action_rows.append({"seed":seed,"selected_action":None,"confidence":None,"first_action":None}); continue
        actions=list(ACTIONS); random.Random(3000+seed+(0 if world_name=='W-X2' else 100)).shuffle(actions)
        state={f:0.0 for f in FIELDS}
        a=choose_action(args,plugin,state,history,g['selected_goal'],actions,f'p6d-{world_name}-action-{seed}',100+seed)
        action_rows.append({"seed":seed,"selected_action":a['selected_action'],"confidence":a['confidence'],"first_action":actions[0]})
    expected_goal=(world['goal_field'],world['goal_direction'])
    goal_counts=Counter(str(r['selected_goal']) for r in goal_rows); action_counts=Counter(r['selected_action'] for r in action_rows)
    goal_first=sum(r['selected_goal']==r['first_goal'] for r in goal_rows); action_first=sum(r['selected_action']==r['first_action'] for r in action_rows)
    discovery_pass=all(r['selected_goal']==expected_goal for r in goal_rows) and goal_first<args.runs
    action_pass=all(r['selected_action']==world['expected_action'] for r in action_rows) and action_first<args.runs

    # Closed-loop execution uses one freshly discovered native goal hypothesis.
    goals=list(GOAL_SPECS); random.Random(999+(0 if world_name=='W-X2' else 100)).shuffle(goals)
    g=discover_goal(args,plugin,outcomes,goals,f'p6d-{world_name}-loop-goal',500)
    state={f:0.0 for f in FIELDS}; loop=[]
    for t in range(3):
        actions=list(ACTIONS); random.Random(6000+t+(0 if world_name=='W-X2' else 100)).shuffle(actions)
        a=choose_action(args,plugin,state,history,g['selected_goal'],actions,f'p6d-{world_name}-loop-{t}',600+t)
        if not a['selected_action']: break
        before=dict(state); state=hidden_transition(state,a['selected_action']); success=hidden_success(state,world)
        loop.append({"turn":t,"action":a['selected_action'],"confidence":a['confidence'],"before":before,"after":dict(state),"success":success})
        if success: break
    loop_pass=bool(loop and loop[-1]['success'] and len(loop)<=3 and all(x['action']==world['expected_action'] for x in loop))
    return {"world":world_name,"hidden_goal_not_supplied_to_plugin":True,"expected_goal":expected_goal,"expected_action":world['expected_action'],"goal_selection_counts":dict(goal_counts),"action_selection_counts":dict(action_counts),"goal_first_candidate_match_rate":goal_first/args.runs,"action_first_candidate_match_rate":action_first/args.runs,"goal_discovery_pass":discovery_pass,"goal_conditioned_action_pass":action_pass,"closed_loop":loop,"closed_loop_pass":loop_pass,"success_observations":sum(bool(o['success']) for o in outcomes),"failure_observations":sum(not bool(o['success']) for o in outcomes)}


def main():
    ap=argparse.ArgumentParser(); ap.add_argument('--native-helper',required=True); ap.add_argument('--bootstrap',required=True); ap.add_argument('--db-prefix',default='/tmp/graphene_p6d'); ap.add_argument('--runs',type=int,default=10); ap.add_argument('--out',default='/tmp/graphene-p6d'); args=ap.parse_args()
    out=Path(args.out); out.mkdir(parents=True,exist_ok=True); plugin=GenericGoalDiscoveryPluginV1()
    worlds=[run_world(args,name,plugin) for name in WORLDS]
    passed=all(w['goal_discovery_pass'] and w['goal_conditioned_action_pass'] and w['closed_loop_pass'] for w in worlds)
    summary={"experiment":"P6D autonomous dialectical goal discovery","runs_per_world":args.runs,"worlds":worlds,"goal_discovery_pass":passed,"explicit_objective_supplied_to_plugin":False,"terminal_success_signal_only":True,"plugin_selected_goal":False,"plugin_selected_action":False,"llm_selector":False,"oracle_goal_field_or_direction_supplied":False,"core_graphenedb_modified":False,"core_hypokosh_modified":False}
    (out/'summary.json').write_text(json.dumps(summary,indent=2,sort_keys=True)); print(json.dumps(summary,indent=2,sort_keys=True)); raise SystemExit(0 if passed else 2)

if __name__=='__main__': main()
