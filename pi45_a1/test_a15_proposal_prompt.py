from __future__ import annotations

import json
import unittest

from a15_proposal_prompt import SYSTEM_RULES, build_proposal_prompt, build_proposal_repair_prompt, parse_json_object


class A15ProposalPromptTests(unittest.TestCase):
    def test_prompt_is_bounded_and_contains_dialectical_guardrails(self) -> None:
        prompt = build_proposal_prompt(
            turn=5,
            game_id="secret-ft09-evaluator-id",
            observation={"grid": [[0, 1], [1, 0]]},
            available_actions=["ACTION1", "ACTION2"],
            recent_outcomes=[{"turn": i, "changed_cells": 0, "effect": "none", "large": "x" * 1500} for i in range(6)],
            governed_context={
                "status": "Contested",
                "reopened_hypothesis_ids": ["old-h2"],
                "challenged_claims": ["competing causal roots remain"],
                "noise": "y" * 4000,
            },
            max_chars=6500,
        )
        self.assertLessEqual(len(prompt), 6500)
        self.assertIn("Preserve at least two genuinely different competing hypotheses", prompt)
        self.assertIn("never use 'win the game'", prompt)
        self.assertIn("native GrapheneDB/HypoKosh controller owns those decisions", prompt)
        self.assertIn('"ACTION1"', prompt)
        self.assertIn('"ACTION2"', prompt)
        self.assertIn("t5-o", prompt)
        self.assertIn("t5-h", prompt)
        self.assertIn("t5-g", prompt)
        self.assertIn("new epistemic revision", prompt)
        self.assertIn('"basis"', prompt)
        self.assertIn('"hypothesis_id"', prompt)
        self.assertIn('"observation_id"', prompt)
        self.assertIn('"evidence_kind"', prompt)
        self.assertIn("grid or transition", prompt)
        self.assertIn("Affordance-only support is invalid", prompt)
        self.assertNotIn("secret-ft09-evaluator-id", prompt)
        self.assertNotIn('"game_id"', prompt)

        output_contract = prompt.split("\n\nOUTPUT CONTRACT:\n", 1)[1]
        self.assertIn("FIELD-ONLY CONTRACT", output_contract)
        self.assertIn('"candidate_goals"', output_contract)
        self.assertIn('"falsification_questions"', output_contract)
        self.assertIn("HypothesisBasis", output_contract)
        self.assertIn("GoalBasis", output_contract)
        self.assertIn("provisional_hypothesis_id", output_contract)
        self.assertIn("reopen_hypothesis_ids", output_contract)
        self.assertIn("Forbidden controller keys", output_contract)
        self.assertNotIn('"statement":""', output_contract.replace(" ", ""))
        self.assertNotIn('"prediction":""', output_contract.replace(" ", ""))
        self.assertNotIn("observable fact only", output_contract.lower())
        self.assertNotIn("possible interpretation a", output_contract.lower())

    def test_evaluator_metadata_is_rejected(self) -> None:
        for bad in [
            {"grid": [[0]], "score": 1},
            {"grid": [[0]], "state": "WIN"},
            {"grid": [[0]], "levels_completed": 1},
            {"grid": [[0]], "win_levels": 6},
        ]:
            with self.assertRaises(ValueError):
                build_proposal_prompt(
                    turn=0, game_id="hidden", observation=bad,
                    available_actions=["ACTION1"], recent_outcomes=[], governed_context={}
                )

    def test_meaningful_action_names_are_rejected(self) -> None:
        with self.assertRaises(ValueError):
            build_proposal_prompt(
                turn=0, game_id="hidden", observation={"grid": [[0]]},
                available_actions=["MOVE_UP"], recent_outcomes=[], governed_context={}
            )

    def test_repair_prompt_is_same_turn_and_adds_no_new_evidence(self) -> None:
        original = "CURRENT EVIDENCE token-clean\nOUTPUT CONTRACT token-contract"
        rejected = json.dumps({
            "turn": 2,
            "hypotheses": [{"id": "t2-h1", "statement": "some possibility", "prediction": "some visible result", "status": "active"}],
            "candidate_goals": [{"id": "t2-g1", "statement": "seek evidence", "status": "active"}],
        })
        prompt = build_proposal_repair_prompt(
            original_prompt=original,
            invalid_output=rejected,
            validation_error="hypotheses[0]: missing required keys ['basis']",
            turn=2,
            available_actions=["ACTION1", "ACTION2", "ACTION3"],
        )
        self.assertIn(original, prompt)
        self.assertIn("SAME proposal", prompt)
        self.assertIn("Every Hypothesis MUST contain non-empty `basis`", prompt)
        self.assertIn("grid or transition evidence", prompt)
        self.assertIn("Every candidate goal MUST contain a non-empty `basis` array", prompt)
        self.assertIn("hypothesis_id", prompt)
        self.assertIn("observation_id", prompt)
        self.assertIn("native-controller-owned", prompt)
        self.assertIn("NOT world evidence", prompt)
        self.assertIn("Never mention or paraphrase", prompt)
        self.assertIn("t2-o", prompt)
        self.assertIn("t2-h", prompt)
        self.assertIn("t2-g", prompt)
        self.assertIn("ACTION3", prompt)
        self.assertNotIn("ft09", prompt)
        self.assertNotIn("bp35", prompt)

    def test_repair_prompt_rejects_semantic_action_names(self) -> None:
        with self.assertRaises(ValueError):
            build_proposal_repair_prompt(
                original_prompt="clean",
                invalid_output="{}",
                validation_error="bad",
                turn=1,
                available_actions=["MOVE_UP"],
            )

    def test_parser_accepts_json_and_fenced_json(self) -> None:
        expected = {"turn": 0, "x": 1}
        self.assertEqual(parse_json_object(json.dumps(expected)), expected)
        self.assertEqual(parse_json_object("```json\n" + json.dumps(expected) + "\n```"), expected)

    def test_system_rules_do_not_claim_known_goal_or_controller_authority(self) -> None:
        lower = SYSTEM_RULES.lower()
        self.assertIn("do not know the objective", lower)
        self.assertNotIn("goal is to", lower)
        self.assertIn("do not choose convergence", lower)
        self.assertIn("controller owns those decisions", lower)
        self.assertIn("not world evidence", lower)
        self.assertIn("affordance evidence", lower)
        self.assertIn("grid or transition observation", lower)


if __name__ == "__main__":
    unittest.main()
