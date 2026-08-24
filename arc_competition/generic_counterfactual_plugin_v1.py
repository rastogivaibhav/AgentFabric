#!/usr/bin/env python3
"""Domain-neutral counterfactual evidence plugin for P6 transfer.

No chess semantics, model, LLM, engine, or action preference. The plugin receives
opaque candidate next-states and derives generic consequence evidence from four
state dimensions: goal_distance (lower is better), resource (higher is better),
hazard (lower is better), and information (higher is better). It reuses the
frozen P5 whole-evidence aggregation rule exactly:

    mean(positive confidences) - 0.25 * mean(contradiction confidences)

The plugin never selects or ranks an action. The comparison-anchor identifier uses
the already-frozen P5 helper routing contract so native retrieval behaviour is not
changed for this transfer test.
"""
from __future__ import annotations
from typing import Any


def node(eid: str, node_type: str, status: str, statement: str, origin: str, turn: int, metadata: dict[str, Any]) -> dict[str, Any]:
    return {"external_id": eid, "node_type": node_type, "status": status, "statement": statement, "origin": origin, "turn": turn, "metadata": metadata}


def relation(src: str, dst: str, role: str, confidence: float, origin: str = "Hypothetical") -> dict[str, Any]:
    return {"from": src, "to": dst, "role": role, "origin": origin, "confidence": float(confidence)}


def conf(delta: float) -> float:
    return max(0.50, min(0.97, 0.55 + 0.15 * abs(float(delta))))


class GenericCounterfactualPluginV1:
    name = "generic-counterfactual-whole-evidence-v1"

    @staticmethod
    def synthesize(positive: list[float], negative: list[float]) -> float:
        pos = sum(positive) / len(positive) if positive else 0.50
        neg = sum(negative) / len(negative) if negative else 0.0
        return max(0.05, min(0.99, pos - 0.25 * neg))

    def enrich(self, *, state: dict[str, float], turn: int, request: dict[str, Any], next_state_by_hypothesis: dict[str, dict[str, float]]) -> dict[str, Any]:
        nodes = request.setdefault("nodes", [])
        relations = request.setdefault("relations", [])
        anchor_id = f"cf2-ply-{turn}-comparison-anchor"
        nodes.insert(0, node(anchor_id, "Outcome", "Proposed", f"Opaque-action comparison anchor at turn {turn}.", "Hypothetical", turn, {"kind": "generic_comparison_anchor", "plugin": self.name}))
        diagnostics: dict[str, Any] = {"plugin": self.name, "turn": turn, "selected_action": None, "ranked_actions": None, "candidates": []}

        for hid, after in next_state_by_hypothesis.items():
            positive: list[float] = []
            negative: list[float] = []
            consequences: list[dict[str, Any]] = []
            specs = [
                ("goal_distance", -1.0, "goal_progress", "goal_regression"),
                ("resource", +1.0, "resource_gain", "resource_cost"),
                ("hazard", -1.0, "hazard_reduction", "hazard_increase"),
                ("information", +1.0, "information_gain", "information_loss"),
            ]
            for field, good_sign, pos_name, neg_name in specs:
                before = float(state[field]); now = float(after[field]); raw = now - before
                oriented = raw * good_sign
                if oriented == 0:
                    continue
                positive_polarity = oriented > 0
                signal = pos_name if positive_polarity else neg_name
                c = conf(oriented)
                (positive if positive_polarity else negative).append(c)
                cid = f"generic-{turn}-{hid}-{signal}"
                statement = f"Opaque candidate changes {field} from {before:g} to {now:g}; derived signal={signal}, magnitude={abs(raw):g}."
                nodes.append(node(cid, "Outcome", "Proposed", statement, "Hypothetical", turn, {"kind": "generic_counterfactual_consequence", "field": field, "signal": signal, "delta": raw, "polarity": "support" if positive_polarity else "contradiction"}))
                relations.append(relation(hid, cid, "Predictive", c))
                if not positive_polarity:
                    relations.append(relation(hid, anchor_id, "Contradicts", c))
                consequences.append({"field": field, "signal": signal, "delta": raw, "confidence": c, "polarity": "support" if positive_polarity else "contradiction"})
            aggregate = self.synthesize(positive, negative)
            relations.append(relation(hid, anchor_id, "Supports", aggregate))
            diagnostics["candidates"].append({"hypothesis_id": hid, "aggregate_support_confidence": aggregate, "positive_count": len(positive), "contradiction_count": len(negative), "consequences": consequences})

        request["protocol"] = "agentfabric-generic-counterfactual-transfer-v1"
        request.setdefault("plugin_metadata", {})["counterfactual_plugin"] = self.name
        request["plugin_metadata"]["comparison_anchor_id"] = anchor_id
        request["plugin_metadata"]["required_query_mode"] = "Theoretical"
        request["plugin_metadata"]["plugin_selects_action"] = False
        request["plugin_metadata"]["aggregation"] = "mean_positive_minus_0.25_mean_contradiction"
        return diagnostics
