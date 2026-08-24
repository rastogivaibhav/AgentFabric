#!/usr/bin/env python3
"""P6B.1 plugin-only validated hidden-rule learner.

The plugin never receives the hidden environment transition function or oracle next
states.  It receives only observed (before, action, after) episodes.  For each opaque
action it:
  * learns a mean state delta from the first two observations,
  * validates that rule against a third held-out observed transition,
  * exposes sample count, residuals, and validation confidence,
  * generates counterfactual evidence only from validated rules.

Unvalidated actions receive only uncertainty evidence so the native reasoner is
encouraged to gather more observations.  The plugin never chooses/ranks an action.
The whole-evidence aggregation remains frozen from P5/P6A.
"""
from __future__ import annotations
from collections import defaultdict
from typing import Any

from generic_counterfactual_plugin_v1 import node, relation, conf

FIELDS=("goal_distance","resource","hazard","information")
ORIENTATION={"goal_distance":-1.0,"resource":1.0,"hazard":-1.0,"information":1.0}
TRAIN_SAMPLES=2
VALIDATION_SAMPLES=1
REQUIRED_SAMPLES=TRAIN_SAMPLES+VALIDATION_SAMPLES
TOLERANCE=1e-9


class GenericHiddenRulePluginV2:
    name="generic-hidden-rule-validated-v2"

    @staticmethod
    def synthesize(positive:list[float],negative:list[float])->float:
        pos=sum(positive)/len(positive) if positive else 0.50
        neg=sum(negative)/len(negative) if negative else 0.0
        return max(0.05,min(0.99,pos-0.25*neg))

    @staticmethod
    def _delta(event:dict[str,Any])->dict[str,float]:
        return {f:float(event["after"][f])-float(event["before"][f]) for f in FIELDS}

    @classmethod
    def learn_validated(cls,history:list[dict[str,Any]])->dict[str,dict[str,Any]]:
        grouped:dict[str,list[dict[str,Any]]]=defaultdict(list)
        for event in history: grouped[str(event["action"])].append(event)
        out:dict[str,dict[str,Any]]={}
        for action,events in grouped.items():
            row:dict[str,Any]={"observations":len(events),"required_observations":REQUIRED_SAMPLES,"validated":False,"provenance":"observed_transition_history"}
            if len(events)>=TRAIN_SAMPLES:
                train=events[:TRAIN_SAMPLES]
                mean={f:sum(cls._delta(e)[f] for e in train)/len(train) for f in FIELDS}
                row["mean_delta"]=mean; row["training_samples"]=len(train)
                holdout=events[TRAIN_SAMPLES:TRAIN_SAMPLES+VALIDATION_SAMPLES]
                row["validation_samples"]=len(holdout)
                if holdout:
                    residuals=[]
                    for e in holdout:
                        observed=cls._delta(e)
                        residuals.append({f:observed[f]-mean[f] for f in FIELDS})
                    max_abs=max(abs(v) for r in residuals for v in r.values())
                    row["validation_residuals"]=residuals; row["max_abs_validation_residual"]=max_abs
                    row["validated"]=max_abs<=TOLERANCE
                    row["validation_confidence"]=0.95 if row["validated"] else max(0.05,0.95/(1.0+max_abs))
            out[action]=row
        return out

    def enrich(self,*,state:dict[str,float],actions:list[str],turn:int,request:dict[str,Any],action_by_hypothesis:dict[str,str],history:list[dict[str,Any]])->dict[str,Any]:
        nodes=request.setdefault("nodes",[]); relations=request.setdefault("relations",[])
        anchor=f"hidden-v2-{turn}-comparison-anchor"
        nodes.insert(0,node(anchor,"Outcome","Proposed",f"Validated hidden-rule comparison anchor at turn {turn}.","Hypothetical",turn,{"kind":"hidden_rule_comparison_anchor","plugin":self.name}))
        models=self.learn_validated(history)
        diag={"plugin":self.name,"turn":turn,"selected_action":None,"ranked_actions":None,"learned_rules":models,"candidates":[]}
        for hid,action in action_by_hypothesis.items():
            model=models.get(action,{"observations":0,"validated":False})
            obs=int(model.get("observations",0)); row={"hypothesis_id":hid,"action":action,"observations":obs,"validated":bool(model.get("validated",False)),"consequences":[]}
            if not model.get("validated"):
                # Evidence strength depends only on how much epistemic uncertainty remains.
                remaining=max(0,REQUIRED_SAMPLES-obs)
                support=0.96-0.08*min(obs,REQUIRED_SAMPLES)
                cid=f"hidden-v2-{turn}-{action}-uncertainty"
                nodes.append(node(cid,"Outcome","Proposed",f"Action {action} has {obs}/{REQUIRED_SAMPLES} required observed transitions; another execution would reduce rule uncertainty by one sample.","Hypothetical",turn,{"kind":"hidden_rule_uncertainty","action":action,"observations":obs,"remaining_samples":remaining}))
                relations.append(relation(hid,cid,"Predictive",support,"Hypothetical")); relations.append(relation(hid,anchor,"Supports",support,"Hypothetical"))
                row.update({"model_status":"needs_evidence","aggregate_support_confidence":support,"predicted_after":None})
            else:
                mean=model["mean_delta"]
                predicted={f:float(state[f])+float(mean[f]) for f in FIELDS}
                predicted["goal_distance"]=max(0.0,predicted["goal_distance"]); predicted["hazard"]=max(0.0,predicted["hazard"])
                positive=[]; negative=[]
                for field in FIELDS:
                    delta=float(mean[field]); oriented=delta*ORIENTATION[field]
                    if abs(delta)<1e-12: continue
                    good=oriented>0; c=conf(oriented); (positive if good else negative).append(c)
                    cid=f"hidden-v2-{turn}-{action}-{field}"
                    nodes.append(node(cid,"Outcome","Proposed",f"Validated rule for {action} predicts {field} delta={delta:g} from {model['training_samples']} train + {model['validation_samples']} held-out observed transition(s); predicted next {field}={predicted[field]:g}.","Hypothetical",turn,{"kind":"validated_rule_counterfactual","action":action,"field":field,"delta":delta,"validation_confidence":model.get("validation_confidence"),"sample_count":obs,"derivation_provenance":"trained_and_validated_on_observed_transitions"}))
                    relations.append(relation(hid,cid,"Predictive",c,"Hypothetical"))
                    if not good: relations.append(relation(hid,anchor,"Contradicts",c,"Hypothetical"))
                    row["consequences"].append({"field":field,"delta":delta,"polarity":"support" if good else "contradiction","confidence":c})
                aggregate=self.synthesize(positive,negative)
                relations.append(relation(hid,anchor,"Supports",aggregate,"Hypothetical"))
                row.update({"model_status":"validated","aggregate_support_confidence":aggregate,"predicted_after":predicted,"validation_confidence":model.get("validation_confidence")})
            diag["candidates"].append(row)
        request["protocol"]="agentfabric-generic-hidden-rule-validated-p6b1"
        request.setdefault("plugin_metadata",{})["counterfactual_plugin"]=self.name
        request["plugin_metadata"].update({"comparison_anchor_id":anchor,"required_query_mode":"Theoretical","plugin_selects_action":False,"oracle_next_states_supplied":False,"rule_training_samples":TRAIN_SAMPLES,"rule_validation_samples":VALIDATION_SAMPLES,"aggregation":"mean_positive_minus_0.25_mean_contradiction"})
        return diag
