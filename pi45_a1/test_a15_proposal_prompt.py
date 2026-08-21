from __future__ import annotations

import json
import unittest

from a15_proposal_prompt import SYSTEM_RULES, build_proposal_prompt, parse_json_object


class A15ProposalPromptTests(unittest.TestCase):
    def test_prompt_is_bounded_and_contains_dialectical_guardrails(self) -> None:
        prompt = build_proposal_prompt(
            turn=5,
            game_id="unknown",
            observation={"grid": [[0, 1], [1, 0]], "score": 0},
            available_actions=["ACTION1", "ACTION2"],
            recent_outcomes=[{"turn": i, "effect": "none", "large": "x" * 1500} for i in range(6)],
            governed_context={"status": "Contested", "reopen_hypothesis_ids": ["h2"], "noise": "y" * 4000},
            max_chars=6500,
        )
        self.assertLessEqual(len(prompt), 6500)
        self.assertIn("Preserve at least two competing hypotheses", prompt)
        self.assertIn("never use 'win the game'", prompt)
        self.assertIn("GrapheneDB runtime owns convergence", prompt)
        self.assertIn('"ACTION1"', prompt)
        self.assertIn('"ACTION2"', prompt)

    def test_parser_accepts_json_and_fenced_json(self) -> None:
        expected = {"turn": 0, "x": 1}
        self.assertEqual(parse_json_object(json.dumps(expected)), expected)
        self.assertEqual(parse_json_object("```json\n" + json.dumps(expected) + "\n```"), expected)

    def test_system_rules_do_not_claim_known_goal(self) -> None:
        lower = SYSTEM_RULES.lower()
        self.assertIn("do not know the objective", lower)
        self.assertNotIn("goal is to", lower)


if __name__ == "__main__":
    unittest.main()
