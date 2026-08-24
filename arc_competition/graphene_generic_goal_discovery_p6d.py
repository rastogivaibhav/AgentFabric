#!/usr/bin/env python3
from __future__ import annotations
import argparse,json,random,subprocess
from collections import Counter
from pathlib import Path
from typing import Any
from a15_arc_dialectic_adapter import invoke_native,stable_digest
from generic_counterfactual_plugin_v1 import node,relation
from generic_goal_discovery_plugin_v1 import FIELDS
from generic_goal_discovery_plugin_v2 import GenericGoalDiscoveryPluginV2

ACTIONS=("A0","A1","A2","A3","A4")
GOAL_SPECS=tuple((f,d) for f in FIELDS for d in (+1,-1))
WORLDS={"W-X2":{"goal_field":"X2","goal_direction":+1,"expected_action":"A2"},"W-X3":{"goal_field":"X3","goal_direction":+1,"expected_action":"A3"}}

def hidden_transition(s,a):
 s=dict(s)
 if a=="A0": s["X0"]+=2
 elif a=="A1": s["X1"]+=2
 elif a=="A2": s["X2"]+=2
 elif a=="A3": s["X3"]+=2
 elif a=="A4": s["X0"]+=1;s["X2"]+=1
 else: raise ValueError(a)
 return s

def hidden_success(s,w): return float(s[w['goal_field']])>=6.0

def collect(world_name,count=80):
 w=WORLDS[world_name];hist=[];outs=[]
 for i in range(count):
  rng=random.Random(70000+(0 if world_name=='W-X2' else 10000)+i);before={f:float(rng.randint(0,8)) for f in FIELDS};before[w['goal_field']]=float(rng.randint(3,5));a=ACTIONS[i%5];after=hidden_transition(before,a);ok=hidden_success(after,w);hist.append({"episode":i,"action":a,"before":before,"after":after,"success":ok});outs.append({"episode":i,"final_state":after,"success":ok})
 assert any(o['success'] for o in outs) and not all(o['success'] for o in outs)
 return hist,outs

def db(args,scope):
 p=f"{args.db_prefix}.{scope}.graphenedb";subprocess.run([args.bootstrap,p],check=True,capture_output=True,text=True);return p

def goal_request(outs,ordered,turn,scope):
 ns=[];gmap={}
 for o in outs: ns.append(node(f"p6d-outcome-{o['episode']:03d}",'Fact','Active',f"Observed terminal result success={bool(o['success'])}; final opaque state={json.dumps(o['final_state'],sort_keys=True)}.",'Observed',turn,{"kind":"terminal_outcome","success":bool(o['success']),"episode":o['episode']}))
 for i,(f,d) in enumerate(ordered,1):
  hid=f"p6d-goal-{i:03d}-{f}-{'up' if d>0 else 'down'}";gmap[hid]={"field":f,"direction":d,"hypothesis_id":hid};ns.append(node(hid,'Hypothesis','Proposed',f"Latent goal hypothesis: successful terminal states prefer {'increase' if d>0 else 'decrease'} {f}.",'Hypothetical',turn,{"kind":"candidate_goal_hypothesis","field":f,"direction":d}))
 return {"protocol":"agentfabric-p6d-goal-discovery","operation":"ingest_and_reason","world_scope":scope,"turn":turn,"nodes":ns,"relations":[]},gmap

def discover(args,plugin,outs,ordered,scope,turn):
 req,gmap=goal_request(outs,ordered,turn,scope);diag=plugin.enrich_goal_request(turn=turn,request=req,goal_by_hypothesis=gmap,outcomes=outs);resp=invoke_native(args.native_helper,req,db(args,scope));pid=str(resp.get('primary_hypothesis_id') or '');sel=gmap.get(pid);return {"selected_goal":sel,"primary":pid,"confidence":resp.get('confidence'),"diag":diag,"native":resp,"digest":stable_digest(req)}

def action_request(state,ordered,turn,scope):
 sid=f'p6d-state-{turn}';ns=[node(sid,'Fact','Active',f"Current opaque state={json.dumps(state,sort_keys=True)}.",'Observed',turn,{"kind":"opaque_state"})];rs=[];amap={}
 for i,a in enumerate(ordered,1):
  hid=f'p6d-action-{turn}-{i:03d}-{a}';amap[hid]=a;ns.append(node(hid,'Hypothesis','Proposed',f'Opaque action {a} candidate under discovered goal.','Hypothetical',turn,{"kind":"candidate_action","action":a}));rs.append(relation(sid,hid,'Supports',0.55,'Observed'))
 return {"protocol":"agentfabric-p6d-action","operation":"ingest_and_reason","world_scope":scope,"turn":turn,"nodes":ns,"relations":rs},amap

def choose(args,plugin,state,hist,goal,ordered,scope,turn):
 if goal is None: return {"selected_action":None,"confidence":0.0,"native":{},"diag":{}}
 req,amap=action_request(state,ordered,turn,scope);diag=plugin.enrich_action_request(turn=turn,state=state,request=req,action_by_hypothesis=amap,transition_history=hist,selected_goal=goal);resp=invoke_native(args.native_helper,req,db(args,scope));pid=str(resp.get('primary_hypothesis_id') or '');return {"selected_action":amap.get(pid),"confidence":resp.get('confidence'),"native":resp,"diag":diag}

def run_world(args,name,plugin):
 w=WORLDS[name];hist,outs=collect(name);gr=[];ar=[]
 for seed in range(args.runs):
  goals=list(GOAL_SPECS);random.Random(1000+seed+(100 if name=='W-X3' else 0)).shuffle(goals);g=discover(args,plugin,outs,goals,f'p6d-{name}-goal-{seed}',10+seed);sem=None if not g['selected_goal'] else (g['selected_goal']['field'],g['selected_goal']['direction']);gr.append({"seed":seed,"selected_goal":sem,"confidence":g['confidence'],"first_goal":goals[0],"primary":g['primary']});print('goal',name,seed,sem,g['confidence'],goals[0])
  acts=list(ACTIONS);random.Random(3000+seed+(100 if name=='W-X3' else 0)).shuffle(acts);a=choose(args,plugin,{f:0.0 for f in FIELDS},hist,g['selected_goal'],acts,f'p6d-{name}-action-{seed}',100+seed);ar.append({"seed":seed,"selected_action":a['selected_action'],"confidence":a['confidence'],"first_action":acts[0]});print('action',name,seed,a['selected_action'],a['confidence'],acts[0])
 expected=(w['goal_field'],w['goal_direction']);gf=sum(r['selected_goal']==r['first_goal'] for r in gr);af=sum(r['selected_action']==r['first_action'] for r in ar);gp=all(r['selected_goal']==expected for r in gr) and gf<args.runs;ap=all(r['selected_action']==w['expected_action'] for r in ar) and af<args.runs
 goals=list(GOAL_SPECS);random.Random(999+(100 if name=='W-X3' else 0)).shuffle(goals);g=discover(args,plugin,outs,goals,f'p6d-{name}-loop-goal',500);state={f:0.0 for f in FIELDS};loop=[]
 if g['selected_goal'] is not None:
  for t in range(3):
   acts=list(ACTIONS);random.Random(6000+t+(100 if name=='W-X3' else 0)).shuffle(acts);a=choose(args,plugin,state,hist,g['selected_goal'],acts,f'p6d-{name}-loop-{t}',600+t)
   if not a['selected_action']: break
   before=dict(state);state=hidden_transition(state,a['selected_action']);ok=hidden_success(state,w);loop.append({"turn":t,"action":a['selected_action'],"confidence":a['confidence'],"before":before,"after":dict(state),"success":ok});
   if ok: break
 lp=bool(loop and loop[-1]['success'] and len(loop)<=3 and all(x['action']==w['expected_action'] for x in loop))
 return {"world":name,"expected_goal":expected,"expected_action":w['expected_action'],"goal_selection_counts":dict(Counter(str(r['selected_goal']) for r in gr)),"action_selection_counts":dict(Counter(r['selected_action'] for r in ar)),"goal_first_candidate_match_rate":gf/args.runs,"action_first_candidate_match_rate":af/args.runs,"goal_discovery_pass":gp,"goal_conditioned_action_pass":ap,"closed_loop":loop,"closed_loop_pass":lp,"success_observations":sum(bool(o['success']) for o in outs),"failure_observations":sum(not bool(o['success']) for o in outs)}

def main():
 ap=argparse.ArgumentParser();ap.add_argument('--native-helper',required=True);ap.add_argument('--bootstrap',required=True);ap.add_argument('--db-prefix',default='/tmp/graphene_p6d');ap.add_argument('--runs',type=int,default=10);ap.add_argument('--out',default='/tmp/graphene-p6d');args=ap.parse_args();out=Path(args.out);out.mkdir(parents=True,exist_ok=True);plugin=GenericGoalDiscoveryPluginV2();worlds=[run_world(args,n,plugin) for n in WORLDS];passed=all(w['goal_discovery_pass'] and w['goal_conditioned_action_pass'] and w['closed_loop_pass'] for w in worlds);summary={"experiment":"P6D autonomous dialectical goal discovery","runs_per_world":args.runs,"worlds":worlds,"goal_discovery_pass":passed,"explicit_objective_supplied_to_plugin":False,"terminal_success_signal_only":True,"plugin_selected_goal":False,"plugin_selected_action":False,"llm_selector":False,"oracle_goal_field_or_direction_supplied":False,"core_graphenedb_modified":False,"core_hypokosh_modified":False};(out/'summary.json').write_text(json.dumps(summary,indent=2,sort_keys=True));print(json.dumps(summary,indent=2,sort_keys=True));raise SystemExit(0 if passed else 2)
if __name__=='__main__': main()
