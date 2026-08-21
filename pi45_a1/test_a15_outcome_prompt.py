from __future__ import annotations

import json
import unittest

from a15_outcome_prompt import build_outcome_prompt, build_outcome_repair_prompt, validate_outcome_interpretation


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
        self.assertIn("AT MOST ONE", prompt)
        # Guardrail prose is allowed to name forbidden evaluator concepts (e.g.
        # "Do not use score"). What must remain clean is the serialized EVIDENCE
        # payload actually supplied as observations.
        evidence_text = prompt.split("\n\nEVIDENCE:\n", 1)[1].split("\n\nOUTPUT CONTRACT:\n", 1)[0]
        evidence = json.loads(evidence_text)
        encoded = json.dumps(evidence, sort_keys=True).lower()
        for forbidden in ["score", "level", "game_id", "win_levels", "levels_completed", "score_delta", "level_delta"]:
            self.assertNotIn(forbidden, encoded)

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

    def test_overlapping_classification_is_rejected(self) -> None:
        with self.assertRaisesRegex(ValueError, "disjoint"):
            validate_outcome_interpretation({
                "observed_effect": "no visible change",
                "supports_hypothesis_ids": ["h1"],
                "contradicts_hypothesis_ids": [],
                "unresolved_hypothesis_ids": ["h1", "h2"],
            }, {"h1", "h2"})

    def test_repair_prompt_adds_no_new_evidence(self) -> None:
        original = "ORIGINAL GRID EVIDENCE token-abc"
        invalid = json.dumps({
            "observed_effect": "none",
            "supports_hypothesis_ids": ["h1"],
            "contradicts_hypothesis_ids": [],
            "unresolved_hypothesis_ids": ["h1"],
        })
        prompt = build_outcome_repair_prompt(
            original_prompt=original,
            invalid_output=invalid,
            validation_error="outcome interpretation classifications must be disjoint",
            tested_ids={"h1", "h2"},
        )
        self.assertIn(original, prompt)
        self.assertIn("pairwise disjoint", prompt)
        self.assertIn("h1", prompt)
        self.assertIn("h2", prompt)
        self.assertNotIn("ft09", prompt)
        self.assertNotIn("bp35", prompt)


if __name__ == "__main__":
    unittest.main()
