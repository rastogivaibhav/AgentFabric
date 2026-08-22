#!/usr/bin/env python3
"""P6D.1 adapter correction: preserve inferred evidence nodes while routing the
hypothesis-to-evidence and hypothesis-to-comparison fibers as Theoretical
(Hypothetical-origin) relations, matching the proven P4/P5/P6 traversal contract.
No goal/action ranking or selection is performed here.
"""
from __future__ import annotations
from typing import Any
from generic_counterfactual_plugin_v1 import node,relation
from generic_goal_discovery_plugin_v1 import GenericGoalDiscoveryPluginV1

class GenericGoalDiscoveryPluginV2(GenericGoalDiscoveryPluginV1):
    name="generic-dialectical-goal-discovery-v2"

    def enrich_goal_request(self,*,turn:int,request:dict[str,Any],goal_by_hypothesis:dict[str,dict[str,Any]],outcomes:list[dict[str,Any]])->dict[str,Any]:
        nodes=request.setdefault('nodes',[]); rels=request.setdefault('relations',[])
        anchor=f"hidden-v4-{turn}-comparison-anchor"
        nodes.insert(0,node(anchor,'Outcome','Proposed',f'Latent-goal comparison anchor at turn {turn}.','Hypothetical',turn,{"kind":"latent_goal_comparison_anchor","plugin":self.name}))
        diag={"plugin":self.name,"phase":"goal_discovery","selected_goal":None,"ranked_goals":None,"candidates":[]}
        success_count=sum(bool(o['success']) for o in outcomes); failure_count=len(outcomes)-success_count
        for hid,spec in goal_by_hypothesis.items():
            field=str(spec['field']); direction=int(spec['direction']); ev=self.goal_contrast(outcomes,field,direction)
            label='increase' if direction>0 else 'decrease'; cid=f"hidden-v4-{turn}-{field}-{label}-outcome-contrast"
            nodes.append(node(cid,'Outcome','Proposed',f'Observed successful versus unsuccessful terminal states imply contrast for {label} {field}; pairwise_auc={ev["auc"]:.6f}.','Inferred',turn,{"kind":"goal_outcome_contrast","field":field,"direction":direction,"pairwise_auc":ev['auc'],"success_count":success_count,"failure_count":failure_count,"derivation_provenance":"observed_terminal_success_contrast"}))
            # The statistic is inferred, but these are theoretical hypothesis fibers.
            # Keeping them Hypothetical-origin makes them eligible in the same
            # Theoretical FiberBundle path used by the proven P4/P5/P6 adapters.
            rels.append(relation(hid,cid,'Predictive',ev['support'],'Hypothetical'))
            rels.append(relation(hid,anchor,'Supports',ev['aggregate'],'Hypothetical'))
            rels.append(relation(hid,anchor,'Contradicts',ev['contradiction'],'Hypothetical'))
            diag['candidates'].append({"hypothesis_id":hid,"field":field,"direction":direction,**ev})
        request['protocol']='agentfabric-p6d-goal-discovery-v2'
        request.setdefault('plugin_metadata',{}).update({"goal_discovery_plugin":self.name,"phase":"goal_discovery","comparison_anchor_id":anchor,"required_query_mode":"Theoretical","plugin_selects_goal":False,"explicit_objective_supplied":False,"goal_evidence_source":"observed_terminal_success_contrast","inferred_statistic_theoretical_fiber":True})
        return diag
