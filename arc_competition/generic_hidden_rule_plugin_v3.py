#!/usr/bin/env python3
"""P6C plugin-only contradiction-driven world-model revision.

Learns a validated transition rule from observed episodes, detects when later
observations contradict that rule, quarantines the stale rule, and learns a new
post-contradiction rule. The replacement becomes predictive only after two new
training observations plus one held-out validation observation.

No hidden transition function or oracle counterfactuals are supplied. The plugin
never chooses an action. GrapheneDB/HypoKosh core remains unchanged.
"""
from __future__ import annotations
from collections import defaultdict
from typing import Any

from generic_counterfactual_plugin_v1 import node, relation, conf

FIELDS=("goal_distance","resource","hazard","information")
ORIENTATION={"goal_distance":-1.0,"resource":1.0,"hazard":-1.0,"information":1.0}
TRAIN=2
HOLDOUT=1
REQUIRED=3
TOL=1e-9


class GenericHiddenRulePluginV3:
    name="generic-hidden-rule-dialectical-revision-v3"

    @staticmethod
    def synthesize(positive:list[float],negative:list[float])->float:
        pos=sum(positive)/len(positive) if positive else 0.50
        neg=sum(negative)/len(negative) if negative else 0.0
        return max(0.05,min(0.99,pos-0.25*neg))

    @staticmethod
    def delta(event:dict[str,Any])->dict[str,float]:
        return {f:float(event['after'][f])-float(event['before'][f]) for f in FIELDS}

    @classmethod
    def fit3(cls,events:list[dict[str,Any]])->dict[str,Any]:
        row={"validated":False,"observations":len(events)}
        if len(events)<TRAIN:
            return row
        train=events[:TRAIN]
        mean={f:sum(cls.delta(e)[f] for e in train)/TRAIN for f in FIELDS}
        row.update({"mean_delta":mean,"training_samples":TRAIN})
        hold=events[TRAIN:TRAIN+HOLDOUT]
        row["validation_samples"]=len(hold)
        if hold:
            residuals=[]
            for e in hold:
                d=cls.delta(e)
                residuals.append({f:d[f]-mean[f] for f in FIELDS})
            mx=max(abs(v) for r in residuals for v in r.values())
            row.update({"validation_residuals":residuals,"max_abs_validation_residual":mx,"validated":mx<=TOL,"validation_confidence":0.95 if mx<=TOL else max(0.05,0.95/(1+mx))})
        return row

    @classmethod
    def learn_revisable(cls,history:list[dict[str,Any]])->dict[str,dict[str,Any]]:
        grouped:dict[str,list[dict[str,Any]]]=defaultdict(list)
        for e in history: grouped[str(e['action'])].append(e)
        out={}
        for action,events in grouped.items():
            initial=cls.fit3(events[:REQUIRED])
            row={"action":action,"observations":len(events),"initial_model":initial,"revision_count":0,"contested":False,"validated":False,"status":"needs_initial_evidence"}
            if not initial.get('validated'):
                row.update(initial); out[action]=row; continue
            old_mean=dict(initial['mean_delta'])
            row.update({"validated":True,"status":"validated_initial","mean_delta":old_mean,"validation_confidence":initial.get('validation_confidence',0.95)})
            contradiction_index=None; contradiction_residual=None
            for idx,e in enumerate(events[REQUIRED:],start=REQUIRED):
                d=cls.delta(e); residual={f:d[f]-old_mean[f] for f in FIELDS}
                if max(abs(v) for v in residual.values())>TOL:
                    contradiction_index=idx; contradiction_residual=residual; break
            if contradiction_index is None:
                out[action]=row; continue
            post=events[contradiction_index:]
            row.update({"contested":True,"validated":False,"status":"contested_relearning","stale_mean_delta":old_mean,"contradiction_observation_index":contradiction_index,"contradiction_residual":contradiction_residual,"post_contradiction_observations":len(post)})
            revised=cls.fit3(post)
            row['revised_model']=revised
            if revised.get('validated'):
                row.update({"validated":True,"status":"revised_validated","revision_count":1,"mean_delta":revised['mean_delta'],"validation_confidence":revised.get('validation_confidence',0.95)})
            out[action]=row
        return out

    def enrich(self,*,state:dict[str,float],actions:list[str],turn:int,request:dict[str,Any],action_by_hypothesis:dict[str,str],history:list[dict[str,Any]])->dict[str,Any]:
        nodes=request.setdefault('nodes',[]); rels=request.setdefault('relations',[])
        anchor=f"hidden-v3-{turn}-comparison-anchor"
        nodes.insert(0,node(anchor,'Outcome','Proposed',f'Dialectical rule-revision comparison anchor at turn {turn}.','Hypothetical',turn,{"kind":"rule_revision_comparison_anchor","plugin":self.name}))
        models=self.learn_revisable(history)
        diag={"plugin":self.name,"turn":turn,"selected_action":None,"ranked_actions":None,"models":models,"candidates":[]}
        for hid,action in action_by_hypothesis.items():
            m=models.get(action,{"observations":0,"validated":False,"status":"unseen","contested":False})
            row={"hypothesis_id":hid,"action":action,"model_status":m.get('status'),"validated":bool(m.get('validated')),"contested":bool(m.get('contested')),"revision_count":int(m.get('revision_count',0)),"consequences":[]}
            if m.get('contested'):
                cid=f"hidden-v3-{turn}-{action}-stale-rule-contradiction"
                residual=m.get('contradiction_residual') or {}
                severity=max([abs(float(v)) for v in residual.values()] or [1.0])
                cc=conf(-severity)
                nodes.append(node(cid,'Outcome','Contested',f'Observed transition contradicts the previously validated rule for {action}; stale rule is quarantined. residual={residual}.','Observed',turn,{"kind":"rule_contradiction","action":action,"residual":residual,"stale_rule_quarantined":True}))
                rels.append(relation(hid,anchor,'Contradicts',cc,'Observed'))
                row['stale_rule_contradiction_confidence']=cc
            if not m.get('validated'):
                # Do not use stale predictions. Only represent need for new evidence.
                post=int(m.get('post_contradiction_observations',0)) if m.get('contested') else int(m.get('observations',0))
                support=max(0.20,0.58-0.08*min(post,REQUIRED)) if m.get('contested') else max(0.60,0.96-0.08*min(post,REQUIRED))
                cid=f"hidden-v3-{turn}-{action}-uncertainty"
                nodes.append(node(cid,'Outcome','Proposed',f'Action {action} rule status={m.get("status")}; additional observed transition evidence is required before predictive reuse.','Hypothetical',turn,{"kind":"revision_uncertainty","action":action,"status":m.get('status'),"stale_prediction_used":False}))
                rels.append(relation(hid,cid,'Predictive',support,'Hypothetical')); rels.append(relation(hid,anchor,'Supports',support,'Hypothetical'))
                row.update({"aggregate_support_confidence":support,"predicted_after":None,"stale_prediction_used":False})
            else:
                mean=m['mean_delta']; predicted={f:float(state[f])+float(mean[f]) for f in FIELDS}; predicted['goal_distance']=max(0.0,predicted['goal_distance']); predicted['hazard']=max(0.0,predicted['hazard'])
                pos=[]; neg=[]
                for field in FIELDS:
                    d=float(mean[field]); oriented=d*ORIENTATION[field]
                    if abs(d)<1e-12: continue
                    good=oriented>0; c=conf(oriented); (pos if good else neg).append(c)
                    cid=f"hidden-v3-{turn}-{action}-{field}"
                    nodes.append(node(cid,'Outcome','Proposed',f'{m.get("status")} rule for {action} predicts {field} delta={d:g}; predicted next {field}={predicted[field]:g}.','Hypothetical',turn,{"kind":"revised_rule_counterfactual" if m.get('revision_count') else "validated_rule_counterfactual","action":action,"field":field,"delta":d,"revision_count":m.get('revision_count',0),"derivation_provenance":"observed_transition_revision"}))
                    rels.append(relation(hid,cid,'Predictive',c,'Hypothetical'))
                    if not good: rels.append(relation(hid,anchor,'Contradicts',c,'Hypothetical'))
                    row['consequences'].append({"field":field,"delta":d,"polarity":"support" if good else "contradiction","confidence":c})
                agg=self.synthesize(pos,neg); rels.append(relation(hid,anchor,'Supports',agg,'Hypothetical'))
                row.update({"aggregate_support_confidence":agg,"predicted_after":predicted,"stale_prediction_used":False})
            diag['candidates'].append(row)
        request['protocol']='agentfabric-generic-hidden-rule-revision-p6c'
        request.setdefault('plugin_metadata',{}).update({"counterfactual_plugin":self.name,"comparison_anchor_id":anchor,"required_query_mode":"Theoretical","plugin_selects_action":False,"oracle_next_states_supplied":False,"stale_rule_quarantine":True,"aggregation":"mean_positive_minus_0.25_mean_contradiction"})
        return diag
