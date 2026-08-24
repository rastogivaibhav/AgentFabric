#!/usr/bin/env python3
from __future__ import annotations
import argparse,json,random,subprocess
from collections import Counter
from pathlib import Path
from a15_arc_dialectic_adapter import invoke_native,stable_digest
from generic_counterfactual_plugin_v1 import node,relation
from generic_hidden_rule_plugin_v3 import GenericHiddenRulePluginV3,FIELDS
ACTIONS=("A0","A1","A2","A3","A4")
SAFE=(
 {"goal_distance":20.0,"resource":10.0,"hazard":5.0,"information":0.0},
 {"goal_distance":17.0,"resource":8.0,"hazard":4.0,"information":3.0},
 {"goal_distance":13.0,"resource":12.0,"hazard":6.0,"information":5.0},
)
def transition_v1(s,a):
 s=dict(s)
 if a=="A0": s['goal_distance']-=1;s['resource']-=1;s['information']+=1
 elif a=="A1": s['goal_distance']-=2;s['resource']-=1
 elif a=="A2": s['goal_distance']-=2;s['resource']-=1;s['hazard']+=1;s['information']+=2
 elif a=="A3": s['resource']+=1;s['information']+=1
 elif a=="A4": s['goal_distance']-=1;s['hazard']-=1;s['information']+=2
 s['goal_distance']=max(0,s['goal_distance']);s['hazard']=max(0,s['hazard']);return s
def transition_v2(s,a):
 if a!="A4": return transition_v1(s,a)
 s=dict(s);s['goal_distance']+=1;s['hazard']+=1;return s
def build(state,turn,ordered,scope):
 sid=f'p6c-state-{turn}';ns=[node(sid,'Fact','Active',f'Current opaque state={json.dumps(state,sort_keys=True)}','Observed',turn,{"kind":"hidden_world_state"}),node('p6c-objective','Decision','Active','Prefer smaller goal_distance, more resource, less hazard, and more information.','Observed',turn,{"kind":"objective"})];rs=[];amap={}
 for i,a in enumerate(ordered,1):
  hid=f'p6c-turn-{turn}-action-{i:03d}-{a}';amap[hid]=a;ns.append(node(hid,'Hypothesis','Proposed',f'Opaque action {a} candidate.','Hypothetical',turn,{"action":a}));rs.append(relation(sid,hid,'Supports',0.55,'Observed'))
 return {"protocol":"agentfabric-p6c","operation":"ingest_and_reason","world_scope":scope,"turn":turn,"nodes":ns,"relations":rs},amap
def reason(args,plugin,state,history,ordered,scope,turn):
 req,amap=build(state,turn,ordered,scope);diag=plugin.enrich(state=state,actions=ordered,turn=turn,request=req,action_by_hypothesis=amap,history=history);db=f'{args.db_prefix}.{scope}.graphenedb';subprocess.run([args.bootstrap,db],check=True,capture_output=True,text=True);resp=invoke_native(args.native_helper,req,db);pid=str(resp.get('primary_hypothesis_id') or '');return {"action":amap.get(pid),"confidence":resp.get('confidence'),"primary":pid,"diag":diag,"native":resp,"digest":stable_digest(req)}
def observe(history,state,a,fn,turn):
 after=fn(state,a);history.append({"turn":turn,"action":a,"before":dict(state),"after":dict(after)});return after
def main():
 ap=argparse.ArgumentParser();ap.add_argument('--native-helper',required=True);ap.add_argument('--bootstrap',required=True);ap.add_argument('--db-prefix',default='/tmp/graphene_p6c');ap.add_argument('--runs',type=int,default=10);ap.add_argument('--out',default='/tmp/graphene-p6c');args=ap.parse_args();out=Path(args.out);out.mkdir(parents=True,exist_ok=True);plugin=GenericHiddenRulePluginV3();rows=[]
 for seed in range(args.runs):
  hist=[];t=0
  for st in SAFE:
   for a in ACTIONS: observe(hist,st,a,transition_v1,t);t+=1
  pre_model=plugin.learn_revisable(hist); pre_ok=all(pre_model[a]['validated'] and pre_model[a]['revision_count']==0 for a in ACTIONS)
  fresh={"goal_distance":7.0,"resource":4.0,"hazard":2.0,"information":1.0};ord1=list(ACTIONS);random.Random(seed).shuffle(ord1);pre=reason(args,plugin,fresh,hist,ord1,f'p6c-pre-{seed}',100)
  observe(hist,SAFE[0],'A4',transition_v2,t);t+=1
  contested=plugin.learn_revisable(hist)['A4']; contest_ok=contested['contested'] and not contested['validated'] and contested['status']=='contested_relearning'
  ord2=list(ACTIONS);random.Random(100+seed).shuffle(ord2);during=reason(args,plugin,fresh,hist,ord2,f'p6c-contested-{seed}',101)
  stale_row=next(x for x in during['diag']['candidates'] if x['action']=='A4'); quarantine_ok=stale_row['contested'] and stale_row['predicted_after'] is None and stale_row['stale_prediction_used'] is False
  observe(hist,SAFE[1],'A4',transition_v2,t);t+=1;observe(hist,SAFE[2],'A4',transition_v2,t);t+=1
  revised=plugin.learn_revisable(hist)['A4']; revised_ok=revised['validated'] and revised['revision_count']==1 and revised['status']=='revised_validated'
  ord3=list(ACTIONS);random.Random(200+seed).shuffle(ord3);post=reason(args,plugin,fresh,hist,ord3,f'p6c-post-{seed}',102)
  post_a4=next(x for x in post['diag']['candidates'] if x['action']=='A4'); prediction_ok=post_a4['predicted_after']==transition_v2(fresh,'A4')
  rows.append({"seed":seed,"pre_action":pre['action'],"pre_confidence":pre['confidence'],"pre_model_ok":pre_ok,"contest_ok":contest_ok,"during_action":during['action'],"quarantine_ok":quarantine_ok,"revised_ok":revised_ok,"post_action":post['action'],"post_confidence":post['confidence'],"prediction_ok":prediction_ok,"first_pre":ord1[0],"first_post":ord3[0]});print('p6c',seed,pre['action'],during['action'],post['action'],contest_ok,revised_ok,prediction_ok)
 pre_counts=Counter(r['pre_action'] for r in rows);post_counts=Counter(r['post_action'] for r in rows);pre_first=sum(r['pre_action']==r['first_pre'] for r in rows);post_first=sum(r['post_action']==r['first_post'] for r in rows)
 post_dominant,post_n=post_counts.most_common(1)[0] if post_counts else (None,0)
 semantic_shift=(post_n==args.runs and post_dominant is not None and post_dominant!='A4')
 pass_all=(all(r['pre_model_ok'] and r['contest_ok'] and r['quarantine_ok'] and r['revised_ok'] and r['prediction_ok'] for r in rows) and pre_counts==Counter({'A4':args.runs}) and semantic_shift and pre_first<args.runs and post_first<args.runs)
 summary={"experiment":"P6C contradiction-driven model revision","runs":args.runs,"pre_selection_counts":dict(pre_counts),"post_selection_counts":dict(post_counts),"post_dominant_action":post_dominant,"semantic_selection_shift_away_from_stale_action":semantic_shift,"pre_first_candidate_match_rate":pre_first/args.runs,"post_first_candidate_match_rate":post_first/args.runs,"contradiction_detected":all(r['contest_ok'] for r in rows),"stale_rule_quarantined":all(r['quarantine_ok'] for r in rows),"replacement_rule_validated":all(r['revised_ok'] for r in rows),"revised_prediction_pass":all(r['prediction_ok'] for r in rows),"revision_selection_pass":pass_all,"oracle_next_states_supplied_to_plugin":False,"plugin_selected_action":False,"llm_selector":False,"core_graphenedb_modified":False,"core_hypokosh_modified":False}
 (out/'summary.json').write_text(json.dumps(summary,indent=2,sort_keys=True));(out/'rows.json').write_text(json.dumps(rows,indent=2,sort_keys=True));print(json.dumps(summary,indent=2,sort_keys=True));raise SystemExit(0 if pass_all else 2)
if __name__=='__main__':main()
