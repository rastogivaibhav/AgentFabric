from __future__ import annotations

import copy
import unittest

from a15_arc_dialectic_adapter import outcome_to_native_request, proposal_to_native_request, stable_digest
from a15_contract_gate import ContractError, load_json, validate_outcome, validate_turn


CONTRACT = load_json("pi45_a1/a15_state_contract.json")


def proposal() -> dict:
    return {
        "turn": 3,
        "observations": [
            {"id": "o3a", "statement": "A region is visible.", "evidence_ref": "state:3"},
            {"id": "o3b", "statement": "The previous action produced no score change.", "evidence_ref": "transition:2-3"},
        ],
        "hypotheses": [
            {"id": "h3a", "statement": "The region may respond to interaction.", "support_observation_ids": ["o3a"], "prediction": "A suitable interaction alters state.", "status": "active"},
            {"id": "h3b", "statement": "The region may be inert in this context.", "support_observation_ids": ["o3a", "o3b"], "prediction": "Interaction leaves state invariant.", "status": "proposed"},
        ],
        "candidate_goals": [
            {"id": "g3a", "statement": "Determine whether the visible region participates in the world rules.", "implied_by_hypothesis_ids": ["h3a", "h3b"], "evidence_observation_ids": ["o3a"], "status": "active"},
        ],
        "provisional_hypothesis_id": "h3a",
        "provisional_goal_id": "g3a",
        "opposition": {"challenged_hypothesis_id": "h3a", "falsification_questions": ["What outcome would make inertness more plausible?"], "reopen_hypothesis_ids": ["h3b"]},
        "experiment": {"tests_hypothesis_ids": ["h3a", "h3b"], "information_goal": "Discriminate responsive from inert.", "predicted_observation": "State changes or remains invariant.", "action": "ACTION2", "action_params": {}},
        "residual_uncertainty": ["Control semantics remain uncertain."],
    }


class A15AdapterTests(unittest.TestCase):
    def test_proposal_maps_to_native_epistemic_types(self) -> None:
        p = proposal()
        validate_turn(p, CONTRACT, {"ACTION1", "ACTION2"})
        req = proposal_to_native_request(p, "diagnostic")
        by_id = {n["external_id"]: n for n in req["nodes"]}
        self.assertEqual(by_id["o3a"]["node_type"], "Fact")
        self.assertEqual(by_id["o3a"]["origin"], "Observed")
        self.assertEqual(by_id["h3a"]["node_type"], "Hypothesis")
        self.assertEqual(by_id["h3a"]["origin"], "Hypothetical")
        self.assertEqual(by_id["g3a"]["node_type"], "Decision")
        self.assertEqual(by_id["g3a"]["origin"], "Hypothetical")
        self.assertIn("turn-3-opposition", by_id)
        self.assertIn("turn-3-experiment", by_id)
        self.assertEqual(req["action"], "ACTION2")
        self.assertEqual(req["reopen_hypothesis_ids"], ["h3b"])

    def test_outcome_is_observed_and_updates_beliefs(self) -> None:
        outcome = {
            "turn": 3,
            "experiment_id": "turn-3-experiment",
            "action": "ACTION2",
            "before_state_digest": "before",
            "after_state_digest": "after",
            "meaningful_change": True,
            "score_delta": 1,
            "level_delta": 0,
            "observed_effect": "A persistent cell changed.",
            "supports_hypothesis_ids": ["h3a"],
            "contradicts_hypothesis_ids": ["h3b"],
        }
        validate_outcome(outcome, CONTRACT, {"h3a", "h3b"})
        req = outcome_to_native_request(outcome, "diagnostic")
        self.assertEqual(req["nodes"][0]["node_type"], "Outcome")
        self.assertEqual(req["nodes"][0]["origin"], "Observed")
        roles = {(r["to"], r["role"], r["origin"]) for r in req["relations"]}
        self.assertIn(("h3a", "Supports", "Observed"), roles)
        self.assertIn(("h3b", "Contradicts", "Observed"), roles)

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
