#!/usr/bin/env python3
from __future__ import annotations
import argparse,json,random,subprocess
from collections import Counter
from pathlib import Path
from typing import Any
from a15_arc_dialectic_adapter import invoke_native,stable_digest
from generic_counterfactual_plugin_v1 import node,relation
from generic_hidden_rule_plugin_v2 import GenericHiddenRulePluginV2,FIELDS,REQUIRED_SAMPLES

ACTIONS=("A0","A1","A2","A3","A4")
SAFE_STATES=(
 {"goal_distance":20.0,"resource":10.0,"hazard":5.0,"information":0.0},
 {"goal_distance":17.0,"resource":8.0,"hazard":4.0,"information":3.0},
 {"goal_distance":13.0,"resource":12.0,"hazard":6.0,"information":5.0},
)

def hidden_transition(state:dict[str,float],action:str)->dict[str,float]:
 s=dict(state)
 if action=="A0": s["goal_distance"]=max(0.0,s["goal_distance"]-1); s["resource"]-=1; s["information"]+=1
 elif action=="A1": s["goal_distance"]=max(0.0,s["goal_distance"]-2); s["resource"]-=1
 elif action=="A2": s["goal_distance"]=max(0.0,s["goal_distance"]-2); s["resource"]-=1; s["hazard"]+=1; s["information"]+=2
 elif action=="A3": s["resource"]+=1; s["information"]+=1
 elif action=="A4": s["goal_distance"]=max(0.0,s["goal_distance"]-1); s["hazard"]=max(0.0,s["hazard"]-1); s["information"]+=2
 else: raise ValueError(action)
 return s

def build_request(state,turn,ordered,scope):
 sid=f"p6b1-state-{turn}"; nodes=[node(sid,"Fact","Active",f"Current opaque state={json.dumps(state,sort_keys=True)}","Observed",turn,{"kind":"hidden_world_state"}),node("p6b1-objective","Decision","Active","Prefer smaller goal_distance, more resource, less hazard, and more information, while reducing uncertainty about unknown action rules.","Observed",turn,{"kind":"hidden_world_objective"})]; rels=[]; amap={}
 for idx,a in enumerate(ordered,1):
  hid=f"p6b1-turn-{turn}-action-{idx:03d}-{a}"; amap[hid]=a; nodes.append(node(hid,"Hypothesis","Proposed",f"Opaque action {a} is a candidate under the learned world model.","Hypothetical",turn,{"kind":"opaque_candidate","action":a})); rels.append(relation(sid,hid,"Supports",0.55,"Observed"))
 return {"protocol":"agentfabric-p6b1","operation":"ingest_and_reason","world_scope":scope,"turn":turn,"nodes":nodes,"relations":rels},amap

def run_reason(args,plugin,state,history,ordered,scope,turn):
 req,amap=build_request(state,turn,ordered,scope); diag=plugin.enrich(state=state,actions=ordered,turn=turn,request=req,action_by_hypothesis=amap,history=history); db=f"{args.db_prefix}.{scope}.graphenedb"; subprocess.run([args.bootstrap,db],check=True,capture_output=True,text=True); resp=invoke_native(args.native_helper,req,db); pid=str(resp.get('primary_hypothesis_id') or ''); return {"selected_action":amap.get(pid),"confidence":resp.get('confidence'),"primary_hypothesis_id":pid,"plugin":diag,"native":resp,"request_digest":stable_digest(req)}

def true_delta(action):
 base={"goal_distance":30.0,"resource":20.0,"hazard":10.0,"information":10.0}; aft=hidden_transition(base,action); return {f:aft[f]-base[f] for f in FIELDS}

def main():
 ap=argparse.ArgumentParser(); ap.add_argument('--native-helper',required=True); ap.add_argument('--bootstrap',required=True); ap.add_argument('--db-prefix',default='/tmp/graphene_generic_p6b1'); ap.add_argument('--runs',type=int,default=10); ap.add_argument('--out',default='/tmp/graphene-generic-p6b1'); a=ap.parse_args(); out=Path(a.out); out.mkdir(parents=True,exist_ok=True); plugin=GenericHiddenRulePluginV2(); runs=[]
 for seed in range(a.runs):
  history=[]; trace=[]; turn=0
  # reset episodes avoid boundary clipping without exposing hidden rules to the plugin
  for sample_idx,state0 in enumerate(SAFE_STATES):
   pending=set(ACTIONS)
   while pending:
    ordered=list(ACTIONS); random.Random(seed*1000+turn).shuffle(ordered); r=run_reason(a,plugin,dict(state0),history,ordered,f'p6b1-explore-{seed}-{turn}',turn); action=r['selected_action']
    if action is None or action not in pending:
     # native reasoner may prefer an already more-sampled action; restrict this reset episode to pending by rebuilding order/candidates
     ordered=list(pending); random.Random(seed*1000+turn+77).shuffle(ordered); r=run_reason(a,plugin,dict(state0),history,ordered,f'p6b1-explore-pending-{seed}-{turn}',turn); action=r['selected_action']
    if action is None: break
    before=dict(state0); after=hidden_transition(before,action); history.append({"turn":turn,"sample_index":sample_idx,"action":action,"before":before,"after":after}); pending.discard(action); trace.append({"turn":turn,"sample_index":sample_idx,"action":action,"confidence":r['confidence'],"first_candidate":ordered[0]}); turn+=1
  models=plugin.learn_validated(history); accuracy={}
  for action in ACTIONS:
   m=models.get(action) or {}; truth=true_delta(action); accuracy[action]=bool(m.get('validated') and all(abs(float(m['mean_delta'][f])-float(truth[f]))<1e-9 for f in FIELDS))
  runs.append({"seed":seed,"history":history,"trace":trace,"models":models,"coverage_pass":all(sum(1 for e in history if e['action']==x)>=REQUIRED_SAMPLES for x in ACTIONS),"rule_accuracy":accuracy,"rule_accuracy_pass":all(accuracy.values())}); print('learn',seed,[t['action'] for t in trace],all(accuracy.values()))
 learning_pass=all(r['coverage_pass'] and r['rule_accuracy_pass'] for r in runs)
 exploit=[]
 for seed,rr in enumerate(runs):
  fresh={"goal_distance":7.0,"resource":4.0,"hazard":2.0,"information":1.0}; ordered=list(ACTIONS); random.Random(9000+seed).shuffle(ordered); r=run_reason(a,plugin,fresh,rr['history'],ordered,f'p6b1-exploit-{seed}',100); action=r['selected_action']; model=plugin.learn_validated(rr['history']).get(action or '') or {}; predicted=None; actual=None
  if action and model.get('validated'):
   predicted={f:fresh[f]+model['mean_delta'][f] for f in FIELDS}; predicted['goal_distance']=max(0.0,predicted['goal_distance']); predicted['hazard']=max(0.0,predicted['hazard']); actual=hidden_transition(fresh,action)
  ok=bool(predicted and all(abs(float(predicted[f])-float(actual[f]))<1e-9 for f in FIELDS)); exploit.append({"seed":seed,"selected_action":action,"first_candidate":ordered[0],"confidence":r['confidence'],"prediction_pass":ok,"predicted_after":predicted,"actual_after":actual}); print('exploit',seed,action,r['confidence'],ok)
 counts=Counter(x['selected_action'] for x in exploit); dom,domn=counts.most_common(1)[0] if counts else (None,0); first=sum(x['selected_action']==x['first_candidate'] for x in exploit); exploit_pass=bool(domn==a.runs and dom=='A4' and first<a.runs and all(x['prediction_pass'] for x in exploit))
 state={"goal_distance":4.0,"resource":4.0,"hazard":1.0,"information":0.0}; loop=[]; hist=runs[0]['history']
 for i in range(4):
  ordered=list(ACTIONS); random.Random(12000+i).shuffle(ordered); r=run_reason(a,plugin,state,hist,ordered,f'p6b1-loop-{i}',200+i); act=r['selected_action']
  if not act: break
  before=dict(state); state=hidden_transition(state,act); loop.append({"turn":i,"action":act,"confidence":r['confidence'],"before":before,"after":dict(state)}); print('loop',i,act,r['confidence'],state)
 loop_pass=len(loop)==4 and all(x['action']=='A4' for x in loop) and state['goal_distance']==0.0
 summary={"experiment":"P6B.1 validated hidden-rule discovery","runs":a.runs,"training_samples_per_action":2,"heldout_validation_samples_per_action":1,"oracle_next_states_supplied_to_plugin":False,"learning_pass":learning_pass,"exploit_selection_counts":dict(counts),"dominant_action":dom,"dominant_count":domn,"first_candidate_match_rate":first/a.runs,"prediction_pass":all(x['prediction_pass'] for x in exploit),"exploit_pass":exploit_pass,"closed_loop":loop,"closed_loop_pass":loop_pass,"hidden_rule_discovery_pass":learning_pass and exploit_pass and loop_pass,"plugin_selected_action":False,"llm_selector":False,"engine":False,"chess_semantics":False,"core_graphenedb_modified":False,"core_hypokosh_modified":False}
 (out/'summary.json').write_text(json.dumps(summary,indent=2,sort_keys=True)); (out/'learning.json').write_text(json.dumps(runs,indent=2,sort_keys=True)); (out/'exploit.json').write_text(json.dumps(exploit,indent=2,sort_keys=True)); print(json.dumps(summary,indent=2,sort_keys=True)); raise SystemExit(0 if summary['hidden_rule_discovery_pass'] else 2)
if __name__=='__main__': main()
