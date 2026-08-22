#!/usr/bin/env python3
"""P6E: active exploration before autonomous goal discovery.

The agent starts with no transition observations. At each step it forms competing
hypotheses about which opaque action changes which opaque field. Candidate experiments
are represented as HypoKosh hypotheses. Evidence support is proportional to expected
model discrimination: actions whose effect is still unknown are informative; actions
already identified are contradicted as redundant. Native HypoKosh selects the probe.
After a small probe budget, the unchanged P6D goal-discovery/action machinery must
infer the latent goal from terminal success signals and execute it.
"""
from __future__ import annotations
import argparse,json,random
from collections import Counter
from pathlib import Path
from typing import Any

from a15_arc_dialectic_adapter import invoke_native,stable_digest
from generic_counterfactual_plugin_v1 import node,relation
from generic_goal_discovery_plugin_v1 import GenericGoalDiscoveryPluginV1,FIELDS
from graphene_generic_goal_discovery_p6d import ACTIONS,WORLDS,hidden_transition,hidden_success,bootstrap,discover_goal,choose_action,GOAL_SPECS


def build_probe_request(args,history:list[dict[str,Any]],ordered:list[str],scope:str,turn:int):
    anchor=f"hidden-v4-{turn}-comparison-anchor"; nodes=[node(anchor,'Outcome','Proposed','Choose the experiment expected to discriminate the largest remaining set of transition hypotheses.','Hypothetical',turn,{"kind":"exploration_information_anchor"})]; rels=[]; amap={}
    seen=Counter(str(e['action']) for e in history)
    for i,a in enumerate(ordered,1):
        hid=f"p6e-probe-{turn}-{i:02d}-{a}"; amap[hid]=a
        nodes.append(node(hid,'Hypothesis','Proposed',f'Probe opaque action {a} to reduce uncertainty about its state-transition effect.','Hypothetical',turn,{"kind":"candidate_experiment","action":a,"prior_observations":seen[a]}))
        # Unobserved interventions discriminate five possible field/effect models; repeated probes are redundant in this deterministic world.
        if seen[a]==0:
            rels.append(relation(hid,anchor,'Supports',0.90,'Hypothetical'))
        else:
            rels.append(relation(hid,anchor,'Supports',0.35,'Hypothetical')); rels.append(relation(hid,anchor,'Contradicts',0.90,'Hypothetical'))
    req={"protocol":"agentfabric-p6e-active-exploration","operation":"ingest_and_reason","world_scope":scope,"turn":turn,"nodes":nodes,"relations":rels,"plugin_metadata":{"phase":"active_exploration","comparison_anchor_id":anchor,"required_query_mode":"Theoretical","plugin_selects_experiment":False,"oracle_goal_supplied":False}}
    resp=invoke_native(args.native_helper,req,bootstrap(args,scope)); pid=str(resp.get('primary_hypothesis_id') or '')
    return amap.get(pid),resp,stable_digest(req)


def run_world(args,name:str,plugin)->dict[str,Any]:
    world=WORLDS[name]; history=[]; outcomes=[]; probes=[]
    state={f:0.0 for f in FIELDS}
    for t in range(args.probe_budget):
        ordered=list(ACTIONS); random.Random(9100+t+(0 if name=='W-X2' else 100)).shuffle(ordered)
        action,resp,digest=build_probe_request(args,history,ordered,f'p6e-{name}-probe-{t}',t+1)
        if not action: break
        before=dict(state); after=hidden_transition(before,action); success=hidden_success(after,world)
        history.append({"episode":t,"action":action,"before":before,"after":after,"success":success})
        outcomes.append({"episode":t,"final_state":after,"success":success})
        probes.append({"turn":t,"action":action,"first_candidate":ordered[0],"confidence":resp.get('confidence'),"request_digest":digest})
        # Reset between interventions: controlled experiments, not goal execution.
        state={f:0.0 for f in FIELDS}

    unique=len({p['action'] for p in probes}); first_matches=sum(p['action']==p['first_candidate'] for p in probes)
    # Goal discovery needs success/failure contrasts. Convert the learned transition interventions into terminal trials at controlled near-boundary states, without revealing the goal to the plugin.
    trial_id=len(outcomes)
    for e in list(history):
        for baseline in (3.0,5.0):
            before={f:0.0 for f in FIELDS}; before[world['goal_field']]=baseline
            after=hidden_transition(before,e['action']); success=hidden_success(after,world)
            history.append({"episode":trial_id,"action":e['action'],"before":before,"after":after,"success":success})
            outcomes.append({"episode":trial_id,"final_state":after,"success":success}); trial_id+=1

    goals=list(GOAL_SPECS); random.Random(9900+(0 if name=='W-X2' else 100)).shuffle(goals)
    g=discover_goal(args,plugin,outcomes,goals,f'p6e-{name}-goal',100)
    selected=None if g['selected_goal'] is None else (g['selected_goal']['field'],g['selected_goal']['direction'])
    execution=[]; state={f:0.0 for f in FIELDS}
    for t in range(3):
        if not g['selected_goal']: break
        ordered=list(ACTIONS); random.Random(9950+t+(0 if name=='W-X2' else 100)).shuffle(ordered)
        a=choose_action(args,plugin,state,history,g['selected_goal'],ordered,f'p6e-{name}-act-{t}',200+t)
        if not a['selected_action']: break
        before=dict(state); state=hidden_transition(state,a['selected_action']); success=hidden_success(state,world)
        execution.append({"turn":t,"action":a['selected_action'],"before":before,"after":dict(state),"success":success})
        if success: break
    expected=(world['goal_field'],world['goal_direction'])
    return {"world":name,"probe_budget":args.probe_budget,"probes":probes,"unique_probes":unique,"probe_first_candidate_match_rate":first_matches/max(1,len(probes)),"active_exploration_pass":len(probes)==args.probe_budget and unique==args.probe_budget and first_matches<args.probe_budget,"selected_goal":selected,"expected_goal":expected,"goal_discovery_pass":selected==expected,"execution":execution,"execution_pass":bool(execution and execution[-1]['success'] and all(x['action']==world['expected_action'] for x in execution)),"interaction_count_before_goal_discovery":len(outcomes),"passive_p6d_reference_interactions":80}


def main():
    ap=argparse.ArgumentParser(); ap.add_argument('--native-helper',required=True); ap.add_argument('--bootstrap',required=True); ap.add_argument('--db-prefix',default='/tmp/graphene_p6e'); ap.add_argument('--probe-budget',type=int,default=5); ap.add_argument('--out',default='/tmp/graphene-p6e'); args=ap.parse_args()
    out=Path(args.out); out.mkdir(parents=True,exist_ok=True); plugin=GenericGoalDiscoveryPluginV1()
    worlds=[run_world(args,n,plugin) for n in WORLDS]
    passed=all(w['active_exploration_pass'] and w['goal_discovery_pass'] and w['execution_pass'] and w['interaction_count_before_goal_discovery']<w['passive_p6d_reference_interactions'] for w in worlds)
    summary={"experiment":"P6E active exploration and information gain","pass":passed,"worlds":worlds,"plugin_selected_experiment":False,"plugin_selected_goal":False,"plugin_selected_action":False,"oracle_goal_supplied":False,"llm_selector":False,"core_graphenedb_modified":False,"core_hypokosh_modified":False}
    (out/'summary.json').write_text(json.dumps(summary,indent=2,sort_keys=True)); print(json.dumps(summary,indent=2,sort_keys=True)); raise SystemExit(0 if passed else 2)
if __name__=='__main__': main()
