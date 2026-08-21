from __future__ import annotations

import unittest

from a15_outcome_prompt import build_outcome_prompt, validate_outcome_interpretation


class A15OutcomePromptTests(unittest.TestCase):
    def test_grid_only_outcome_prompt(self) -> None:
        prompt = build_outcome_prompt(
            turn=2,
            experiment={"tests_hypothesis_ids": ["h1", "h2"], "action": "ACTION6", "action_params": {"x": 4, "y": 5}, "information_goal": "test", "predicted_observation": "grid changes"},
            hypotheses=[
                {"id": "h1", "statement": "region responds", "prediction": "grid changes"},
                {"id": "h2", "statement": "region inert", "prediction": "grid unchanged"},
            ],
            grid_outcome={"before_grid_digest": "a", "after_grid_digest": "b", "changed_cells": 2, "changed_regions": [[4,5,0,1]], "persistent_change": True},
        )
        self.assertIn("observable before/after grid evidence", prompt)
        self.assertNotIn("score", prompt.lower())
        self.assertNotIn("level", prompt.lower())

    def test_evaluator_metadata_rejected(self) -> None:
        with self.assertRaises(ValueError):
            build_outcome_prompt(
                turn=0,
                experiment={"tests_hypothesis_ids": ["h1"], "action": "ACTION1"},
                hypotheses=[{"id": "h1", "statement": "x", "prediction": "y"}],
                grid_outcome={"changed_cells": 0, "score_delta": 1},
            )

    def test_interpretation_must_only_classify_tested_hypotheses(self) -> None:
        with self.assertRaises(ValueError):
            validate_outcome_interpretation({
                "observed_effect": "none",
                "supports_hypothesis_ids": ["h3"],
                "contradicts_hypothesis_ids": [],
                "unresolved_hypothesis_ids": [],
            }, {"h1", "h2"})


if __name__ == "__main__":
    unittest.main()
