from __future__ import annotations

import copy
import json
import unittest

from a15_arc_dialectic_adapter import OPAQUE_WORLD_SCOPE, outcome_to_native_request, proposal_to_native_request, stable_digest
from a15_contract_gate import ContractError, load_json, validate_outcome, validate_turn


CONTRACT = load_json("pi45_a1/a15_state_contract.json")


def proposal() -> dict:
    return {
        "turn": 3,
        "observations": [
            {"id": "t3-o-a", "statement": "A region is visible in the grid.", "evidence_ref": "grid:3"},
            {"id": "t3-o-b", "statement": "The previous action left the grid unchanged.", "evidence_ref": "transition:2-3"},
        ],
        "hypotheses": [
            {"id": "t3-h-a", "statement": "The region may respond to interaction.", "support_observation_ids": ["t3-o-a"], "prediction": "A suitable interaction alters the grid.", "status": "active"},
            {"id": "t3-h-b", "statement": "The region may be inert in this context.", "support_observation_ids": ["t3-o-a", "t3-o-b"], "prediction": "Interaction leaves the grid invariant.", "status": "proposed"},
        ],
        "candidate_goals": [
            {"id": "t3-g-a", "statement": "Determine whether the visible region participates in the world rules.", "basis": [{"hypothesis_id": "t3-h-a", "observation_id": "t3-o-a"}, {"hypothesis_id": "t3-h-b", "observation_id": "t3-o-b"}], "status": "active"},
        ],
        "opposition": {"falsification_questions": ["What observable grid outcome would make inertness more plausible?"]},
        "experiment": {"tests_hypothesis_ids": ["t3-h-a", "t3-h-b"], "information_goal": "Discriminate responsive from inert.", "predicted_observation": "The grid changes or remains invariant.", "action": "ACTION2", "action_params": {}},
        "residual_uncertainty": ["Control semantics remain uncertain."],
    }


def outcome() -> dict:
    return {
        "turn": 3,
        "experiment_id": "turn-3-experiment",
        "action": "ACTION2",
        "before_grid_digest": "before",
        "after_grid_digest": "after",
        "changed_cells": 1,
        "changed_regions": ["r0"],
        "persistent_change": True,
        "observed_effect": "A persistent cell changed.",
        "supports_hypothesis_ids": ["t3-h-a"],
        "contradicts_hypothesis_ids": ["t3-h-b"],
    }


class A15AdapterTests(unittest.TestCase):
    def test_proposal_maps_to_native_epistemic_types_without_controller_seeds(self) -> None:
        p = proposal()
        validate_turn(p, CONTRACT, {"ACTION1", "ACTION2"})
        req = proposal_to_native_request(p, "ft09-secret-evaluator-id")
        by_id = {n["external_id"]: n for n in req["nodes"]}
        self.assertEqual(by_id["t3-o-a"]["node_type"], "Fact")
        self.assertEqual(by_id["t3-o-a"]["origin"], "Observed")
        self.assertEqual(by_id["t3-h-a"]["node_type"], "Hypothesis")
        self.assertEqual(by_id["t3-h-a"]["origin"], "Hypothetical")
        self.assertEqual(by_id["t3-g-a"]["node_type"], "Decision")
        self.assertEqual(by_id["t3-g-a"]["origin"], "Hypothetical")
        self.assertEqual(by_id["t3-g-a"]["metadata"]["basis_count"], "2")
        self.assertIn("turn-3-opposition", by_id)
        self.assertIn("turn-3-experiment", by_id)
        self.assertEqual(req["action"], "ACTION2")
        self.assertEqual(req["candidate_hypothesis_ids"], ["t3-h-a", "t3-h-b"])
        self.assertEqual(req["world_scope"], OPAQUE_WORLD_SCOPE)
        self.assertNotIn("provisional_hypothesis_id", req)
        self.assertNotIn("provisional_goal_id", req)
        self.assertNotIn("reopen_hypothesis_ids", req)
        self.assertNotIn("ft09-secret-evaluator-id", json.dumps(req, sort_keys=True))
        self.assertNotIn("game_id", json.dumps(req, sort_keys=True))
        roles = {(r["from"], r["to"], r["role"]) for r in req["relations"]}
        self.assertIn(("t3-h-a", "t3-g-a", "Predictive"), roles)
        self.assertIn(("t3-h-b", "t3-g-a", "Predictive"), roles)
        self.assertIn(("t3-o-a", "t3-g-a", "Supports"), roles)
        self.assertIn(("t3-o-b", "t3-g-a", "Supports"), roles)
        opposition_relations = [r for r in req["relations"] if r["from"] == "turn-3-opposition"]
        self.assertEqual(opposition_relations, [])

    def test_goal_basis_must_reference_known_hypothesis_and_observation(self) -> None:
        for key, value in [("hypothesis_id", "t3-h-missing"), ("observation_id", "t3-o-missing")]:
            contaminated = proposal()
            contaminated["candidate_goals"][0]["basis"][0][key] = value
            with self.assertRaises(ContractError):
                validate_turn(contaminated, CONTRACT, {"ACTION2"})

    def test_llm_controller_fields_are_rejected(self) -> None:
        for mutate in [
            lambda p: p.update({"provisional_hypothesis_id": "t3-h-a"}),
            lambda p: p.update({"provisional_goal_id": "t3-g-a"}),
            lambda p: p["opposition"].update({"challenged_hypothesis_id": "t3-h-a"}),
            lambda p: p["opposition"].update({"reopen_hypothesis_ids": ["t3-h-b"]}),
        ]:
            contaminated = proposal()
            mutate(contaminated)
            with self.assertRaises(ContractError):
                validate_turn(contaminated, CONTRACT, {"ACTION2"})

    def test_outcome_is_grid_observed_and_updates_beliefs(self) -> None:
        o = outcome()
        validate_outcome(o, CONTRACT, {"t3-h-a", "t3-h-b"})
        req = outcome_to_native_request(o, "bp35-secret-evaluator-id")
        self.assertEqual(req["nodes"][0]["node_type"], "Outcome")
        self.assertEqual(req["nodes"][0]["origin"], "Observed")
        roles = {(r["to"], r["role"], r["origin"]) for r in req["relations"]}
        self.assertIn(("t3-h-a", "Supports", "Observed"), roles)
        self.assertIn(("t3-h-b", "Contradicts", "Observed"), roles)
        encoded = json.dumps(req, sort_keys=True)
        self.assertNotIn("score", encoded)
        self.assertNotIn("level", encoded)
        self.assertNotIn("bp35-secret-evaluator-id", encoded)

    def test_evaluator_fields_are_rejected_from_epistemic_outcome(self) -> None:
        for key, value in [("score_delta", 1), ("level_delta", 1), ("win_levels", 6), ("state", "WIN")]:
            contaminated = copy.deepcopy(outcome())
            contaminated[key] = value
            with self.assertRaises(ContractError, msg=key):
                validate_outcome(contaminated, CONTRACT, {"t3-h-a", "t3-h-b"})

    def test_evaluator_fields_are_rejected_from_proposal(self) -> None:
        contaminated = proposal()
        contaminated["observations"][0]["score"] = 10
        with self.assertRaises(ContractError):
            validate_turn(contaminated, CONTRACT, {"ACTION2"})

    def test_unscoped_ids_are_rejected(self) -> None:
        contaminated = proposal()
        contaminated["hypotheses"][0]["id"] = "h-a"
        with self.assertRaises(ContractError):
            validate_turn(contaminated, CONTRACT, {"ACTION2"})

    def test_content_free_goal_remains_forbidden(self) -> None:
        p = proposal()
        p["candidate_goals"][0]["statement"] = "Win the game"
        with self.assertRaises(ContractError):
            validate_turn(p, CONTRACT, {"ACTION2"})

    def test_serialization_digest_is_deterministic(self) -> None:
        req = proposal_to_native_request(proposal(), "diagnostic")
        self.assertEqual(stable_digest(req), stable_digest(copy.deepcopy(req)))


if __name__ == "__main__":
    unittest.main()
