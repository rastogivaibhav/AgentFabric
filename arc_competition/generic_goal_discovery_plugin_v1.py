#!/usr/bin/env python3
"""P6D plugin-only autonomous goal discovery for an opaque world.

The environment never supplies an objective field/direction to this plugin. It sees:
  * observed state transitions (before, action, after),
  * episode outcomes containing only final opaque state + success terminal flag.

It creates competing goal hypotheses for every opaque field and both directions,
uses successful-vs-unsuccessful outcome contrasts as dialectical evidence, and lets
native HypoKosh select the goal hypothesis. A second native reasoning call then
uses only the selected goal plus empirically learned transition deltas to choose an
action. The plugin never selects a goal or action itself.
"""
from __future__ import annotations
from collections import defaultdict
from typing import Any

from generic_counterfactual_plugin_v1 import node, relation, conf

FIELDS=("X0","X1","X2","X3")
DIRECTIONS=(+1,-1)


class GenericGoalDiscoveryPluginV1:
    name="generic-dialectical-goal-discovery-v1"

    @staticmethod
    def learn_transitions(history:list[dict[str,Any]])->dict[str,dict[str,Any]]:
        grouped:dict[str,list[dict[str,Any]]]=defaultdict(list)
        for e in history:
            grouped[str(e['action'])].append(e)
        out={}
        for action,events in grouped.items():
            mean={f:sum(float(e['after'][f])-float(e['before'][f]) for e in events)/len(events) for f in FIELDS}
            max_residual=max(abs((float(e['after'][f])-float(e['before'][f]))-mean[f]) for e in events for f in FIELDS)
            out[action]={"observations":len(events),"mean_delta":mean,"validated":max_residual<=1e-9,"max_abs_residual":max_residual,"provenance":"observed_transitions"}
        return out

    @staticmethod
    def goal_contrast(outcomes:list[dict[str,Any]],field:str,direction:int)->dict[str,float]:
        success=[float(o['final_state'][field]) for o in outcomes if bool(o['success'])]
        failure=[float(o['final_state'][field]) for o in outcomes if not bool(o['success'])]
        if not success or not failure:
            return {"auc":0.5,"support":0.5,"contradiction":0.5,"aggregate":0.375,"pairs":0}
        wins=0.0; pairs=0
        for s in success:
            for f in failure:
                d=direction*(s-f); pairs+=1
                wins += 1.0 if d>0 else (0.5 if abs(d)<=1e-12 else 0.0)
        auc=wins/pairs
        support=0.50+0.47*auc
        contradiction=0.50+0.47*(1.0-auc)
        aggregate=max(0.05,min(0.99,support-0.25*contradiction))
        return {"auc":auc,"support":support,"contradiction":contradiction,"aggregate":aggregate,"pairs":pairs}

    def enrich_goal_request(self,*,turn:int,request:dict[str,Any],goal_by_hypothesis:dict[str,dict[str,Any]],outcomes:list[dict[str,Any]])->dict[str,Any]:
        nodes=request.setdefault('nodes',[]); rels=request.setdefault('relations',[])
        anchor=f"hidden-v4-{turn}-comparison-anchor"
        nodes.insert(0,node(anchor,'Outcome','Proposed',f'Latent-goal comparison anchor at turn {turn}.','Hypothetical',turn,{"kind":"latent_goal_comparison_anchor","plugin":self.name}))
        diag={"plugin":self.name,"phase":"goal_discovery","selected_goal":None,"ranked_goals":None,"candidates":[]}
        for hid,spec in goal_by_hypothesis.items():
            field=str(spec['field']); direction=int(spec['direction']); ev=self.goal_contrast(outcomes,field,direction)
            label='increase' if direction>0 else 'decrease'
            cid=f"hidden-v4-{turn}-{field}-{label}-outcome-contrast"
            nodes.append(node(cid,'Outcome','Proposed',f'Observed successful versus unsuccessful terminal states provide contrast evidence for hypothesis: successful outcomes prefer {label} {field}. pairwise_auc={ev["auc"]:.6f}.','Inferred',turn,{"kind":"goal_outcome_contrast","field":field,"direction":direction,"pairwise_auc":ev['auc'],"success_count":sum(bool(o['success']) for o in outcomes),"failure_count":sum(not bool(o['success']) for o in outcomes)}))
            rels.append(relation(hid,cid,'Predictive',ev['support'],'Inferred'))
            rels.append(relation(hid,anchor,'Supports',ev['aggregate'],'Inferred'))
            rels.append(relation(hid,anchor,'Contradicts',ev['contradiction'],'Inferred'))
            diag['candidates'].append({"hypothesis_id":hid,"field":field,"direction":direction,**ev})
        request['protocol']='agentfabric-p6d-goal-discovery-v1'
        request.setdefault('plugin_metadata',{}).update({"goal_discovery_plugin":self.name,"phase":"goal_discovery","comparison_anchor_id":anchor,"required_query_mode":"Theoretical","plugin_selects_goal":False,"explicit_objective_supplied":False,"goal_evidence_source":"observed_terminal_success_contrast"})
        return diag

    def enrich_action_request(self,*,turn:int,state:dict[str,float],request:dict[str,Any],action_by_hypothesis:dict[str,str],transition_history:list[dict[str,Any]],selected_goal:dict[str,Any])->dict[str,Any]:
        nodes=request.setdefault('nodes',[]); rels=request.setdefault('relations',[])
        anchor=f"hidden-v4-{turn}-comparison-anchor"
        nodes.insert(0,node(anchor,'Outcome','Proposed',f'Action comparison under discovered goal at turn {turn}.','Hypothetical',turn,{"kind":"discovered_goal_action_anchor","plugin":self.name}))
        models=self.learn_transitions(transition_history)
        field=str(selected_goal['field']); direction=int(selected_goal['direction']); label='increase' if direction>0 else 'decrease'
        goal_id=f"hidden-v4-{turn}-active-discovered-goal"
        nodes.append(node(goal_id,'Decision','Active',f'Discovered goal hypothesis: successful outcomes prefer {label} {field}.','Inferred',turn,{"kind":"discovered_goal","field":field,"direction":direction,"source_hypothesis_id":selected_goal.get('hypothesis_id','')}))
        diag={"plugin":self.name,"phase":"goal_conditioned_action","selected_action":None,"ranked_actions":None,"selected_goal":{"field":field,"direction":direction},"transition_models":models,"candidates":[]}
        for hid,action in action_by_hypothesis.items():
            model=models.get(action); row={"hypothesis_id":hid,"action":action,"validated_model":bool(model and model.get('validated'))}
            if not model or not model.get('validated'):
                agg=0.50
                rels.append(relation(hid,anchor,'Supports',agg,'Hypothetical'))
                row.update({"aggregate_support_confidence":agg,"predicted_after":None,"goal_delta":None})
            else:
                delta=float(model['mean_delta'][field]); oriented=direction*delta
                predicted={f:float(state[f])+float(model['mean_delta'][f]) for f in FIELDS}
                if abs(oriented)<=1e-12:
                    agg=0.50
                elif oriented>0:
                    agg=conf(oriented)
                else:
                    cc=conf(oriented); agg=max(0.05,0.50-0.25*cc); rels.append(relation(hid,anchor,'Contradicts',cc,'Hypothetical'))
                cid=f"hidden-v4-{turn}-{action}-{field}-goal-consequence"
                nodes.append(node(cid,'Outcome','Proposed',f'Learned action {action} predicts {field} delta={delta:g}; under discovered goal direction={direction:+d}, oriented goal effect={oriented:g}.','Hypothetical',turn,{"kind":"discovered_goal_counterfactual","action":action,"field":field,"direction":direction,"delta":delta,"oriented_goal_effect":oriented,"model_observations":model['observations']}))
                rels.append(relation(hid,cid,'Predictive',conf(oriented) if abs(oriented)>1e-12 else 0.50,'Hypothetical'))
                rels.append(relation(hid,goal_id,'Predictive',agg,'Inferred'))
                rels.append(relation(hid,anchor,'Supports',agg,'Hypothetical'))
                row.update({"aggregate_support_confidence":agg,"predicted_after":predicted,"goal_delta":delta,"oriented_goal_effect":oriented})
            diag['candidates'].append(row)
        request['protocol']='agentfabric-p6d-goal-conditioned-action-v1'
        request.setdefault('plugin_metadata',{}).update({"goal_discovery_plugin":self.name,"phase":"goal_conditioned_action","comparison_anchor_id":anchor,"required_query_mode":"Theoretical","plugin_selects_action":False,"explicit_objective_supplied":False,"selected_goal_source":"native_hypokosh_primary_goal_hypothesis"})
        return diag
