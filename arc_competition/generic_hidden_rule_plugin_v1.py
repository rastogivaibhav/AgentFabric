#!/usr/bin/env python3
"""P6B plugin-only hidden-rule learner for an opaque deterministic world.

The plugin never receives the environment transition function or oracle next states.
It sees only observed (before, action, after) transitions. For each opaque action it
learns an empirical mean delta over generic state fields and derives predicted
counterfactual consequences from that learned model. Untried actions receive only
an uncertainty/information-gain hypothesis. The plugin never chooses an action.

The whole-evidence aggregation is frozen from P5/P6A:
    mean(positive confidences) - 0.25 * mean(contradiction confidences)
"""
from __future__ import annotations
from collections import defaultdict
from typing import Any

from generic_counterfactual_plugin_v1 import node, relation, conf

FIELDS = ("goal_distance", "resource", "hazard", "information")
ORIENTATION = {"goal_distance": -1.0, "resource": 1.0, "hazard": -1.0, "information": 1.0}


class GenericHiddenRulePluginV1:
    name = "generic-hidden-rule-learner-v1"

    @staticmethod
    def synthesize(positive: list[float], negative: list[float]) -> float:
        pos = sum(positive) / len(positive) if positive else 0.50
        neg = sum(negative) / len(negative) if negative else 0.0
        return max(0.05, min(0.99, pos - 0.25 * neg))

    @staticmethod
    def learn(history: list[dict[str, Any]]) -> dict[str, dict[str, Any]]:
        sums: dict[str, dict[str, float]] = defaultdict(lambda: {f: 0.0 for f in FIELDS})
        counts: dict[str, int] = defaultdict(int)
        for event in history:
            action = str(event["action"])
            before = event["before"]; after = event["after"]
            counts[action] += 1
            for field in FIELDS:
                sums[action][field] += float(after[field]) - float(before[field])
        out: dict[str, dict[str, Any]] = {}
        for action, count in counts.items():
            out[action] = {
                "observations": count,
                "mean_delta": {f: sums[action][f] / count for f in FIELDS},
            }
        return out

    def enrich(
        self,
        *,
        state: dict[str, float],
        actions: list[str],
        turn: int,
        request: dict[str, Any],
        action_by_hypothesis: dict[str, str],
        history: list[dict[str, Any]],
    ) -> dict[str, Any]:
        nodes = request.setdefault("nodes", [])
        relations = request.setdefault("relations", [])
        anchor_id = f"hidden-ply-{turn}-comparison-anchor"
        nodes.insert(0, node(anchor_id, "Outcome", "Proposed", f"Hidden-rule comparison anchor at turn {turn}.", "Hypothetical", turn, {"kind":"hidden_rule_comparison_anchor","plugin":self.name}))
        learned = self.learn(history)
        diagnostics: dict[str, Any] = {"plugin":self.name,"turn":turn,"selected_action":None,"ranked_actions":None,"learned_rules":learned,"candidates":[]}

        for hid, action in action_by_hypothesis.items():
            model = learned.get(action)
            positive: list[float] = []
            negative: list[float] = []
            row: dict[str, Any] = {"hypothesis_id":hid,"action":action,"observations":0 if model is None else model["observations"],"consequences":[]}

            if model is None:
                # Pure epistemic exploration signal: no outcome is predicted for an
                # untried action.  The high confidence represents expected information
                # gain, not utility or a hidden preference for the action identity.
                info_conf = 0.90
                cid = f"hidden-{turn}-{action}-untried"
                nodes.append(node(cid,"Outcome","Proposed",f"Opaque action {action} is untried; executing it would resolve uncertainty about its transition rule.","Hypothetical",turn,{"kind":"hidden_rule_uncertainty","action":action,"signal":"information_gain_from_untried_action"}))
                relations.append(relation(hid,cid,"Predictive",info_conf))
                relations.append(relation(hid,anchor_id,"Supports",info_conf))
                row["aggregate_support_confidence"] = info_conf
                row["model_status"] = "untried"
                row["predicted_after"] = None
            else:
                predicted_after = {f: float(state[f]) + float(model["mean_delta"][f]) for f in FIELDS}
                predicted_after["goal_distance"] = max(0.0, predicted_after["goal_distance"])
                predicted_after["hazard"] = max(0.0, predicted_after["hazard"])
                for field in FIELDS:
                    delta = float(model["mean_delta"][field])
                    oriented = delta * ORIENTATION[field]
                    if abs(delta) < 1e-12:
                        continue
                    positive_polarity = oriented > 0
                    c = conf(oriented)
                    (positive if positive_polarity else negative).append(c)
                    signal = f"learned_{field}_{'benefit' if positive_polarity else 'cost'}"
                    cid = f"hidden-{turn}-{action}-{field}"
                    nodes.append(node(cid,"Outcome","Proposed",f"From {model['observations']} observed transition(s), learned action {action} mean {field} delta={delta:g}; predicted next {field}={predicted_after[field]:g}.","Inferred",turn,{"kind":"learned_counterfactual_consequence","action":action,"field":field,"delta":delta,"signal":signal,"observations":model["observations"]}))
                    relations.append(relation(hid,cid,"Predictive",c,"Inferred"))
                    if not positive_polarity:
                        relations.append(relation(hid,anchor_id,"Contradicts",c,"Inferred"))
                    row["consequences"].append({"field":field,"delta":delta,"polarity":"support" if positive_polarity else "contradiction","confidence":c})
                aggregate = self.synthesize(positive, negative)
                relations.append(relation(hid,anchor_id,"Supports",aggregate,"Inferred"))
                row["aggregate_support_confidence"] = aggregate
                row["model_status"] = "learned"
                row["predicted_after"] = predicted_after
            diagnostics["candidates"].append(row)

        request["protocol"] = "agentfabric-generic-hidden-rule-transfer-v1"
        request.setdefault("plugin_metadata", {})["counterfactual_plugin"] = self.name
        request["plugin_metadata"]["comparison_anchor_id"] = anchor_id
        request["plugin_metadata"]["required_query_mode"] = "Theoretical"
        request["plugin_metadata"]["plugin_selects_action"] = False
        request["plugin_metadata"]["oracle_next_states_supplied"] = False
        request["plugin_metadata"]["aggregation"] = "mean_positive_minus_0.25_mean_contradiction"
        return diagnostics
