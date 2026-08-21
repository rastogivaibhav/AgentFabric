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
        self.assertIn("exactly one row", prompt.lower())
        self.assertIn("classifications", prompt)
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

    def test_valid_classification_projects_to_graphene_edges(self) -> None:
        result = validate_outcome_interpretation({
            "observed_effect": "the tested region did not change",
            "classifications": [
                {"hypothesis_id": "h1", "verdict": "contradicted"},
                {"hypothesis_id": "h2", "verdict": "supported"},
            ],
        }, {"h1", "h2"})
        self.assertEqual(result["supports_hypothesis_ids"], ["h2"])
        self.assertEqual(result["contradicts_hypothesis_ids"], ["h1"])
        self.assertEqual(result["unresolved_hypothesis_ids"], [])

    def test_interpretation_must_only_classify_tested_hypotheses(self) -> None:
        with self.assertRaisesRegex(ValueError, "untested"):
            validate_outcome_interpretation({
                "observed_effect": "none",
                "classifications": [
                    {"hypothesis_id": "h1", "verdict": "unresolved"},
                    {"hypothesis_id": "h3", "verdict": "supported"},
                ],
            }, {"h1", "h2"})

    def test_duplicate_hypothesis_is_rejected(self) -> None:
        with self.assertRaisesRegex(ValueError, "duplicates"):
            validate_outcome_interpretation({
                "observed_effect": "none",
                "classifications": [
                    {"hypothesis_id": "h1", "verdict": "supported"},
                    {"hypothesis_id": "h1", "verdict": "unresolved"},
                ],
            }, {"h1", "h2"})

    def test_omitted_hypothesis_is_rejected(self) -> None:
        with self.assertRaisesRegex(ValueError, "exactly one row"):
            validate_outcome_interpretation({
                "observed_effect": "none",
                "classifications": [
                    {"hypothesis_id": "h1", "verdict": "unresolved"},
                ],
            }, {"h1", "h2"})

    def test_invalid_verdict_is_rejected(self) -> None:
        with self.assertRaisesRegex(ValueError, "invalid verdict"):
            validate_outcome_interpretation({
                "observed_effect": "none",
                "classifications": [
                    {"hypothesis_id": "h1", "verdict": "maybe"},
                    {"hypothesis_id": "h2", "verdict": "unresolved"},
                ],
            }, {"h1", "h2"})

    def test_old_overlapping_array_shape_is_rejected(self) -> None:
        with self.assertRaisesRegex(ValueError, "missing"):
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
            "classifications": [
                {"hypothesis_id": "h1", "verdict": "supported"},
                {"hypothesis_id": "h1", "verdict": "unresolved"},
            ],
        })
        prompt = build_outcome_repair_prompt(
            original_prompt=original,
            invalid_output=invalid,
            validation_error="outcome classification duplicates hypothesis 'h1'",
            tested_ids={"h1", "h2"},
        )
        self.assertIn(original, prompt)
        self.assertIn("EXACTLY ONE", prompt)
        self.assertIn("h1", prompt)
        self.assertIn("h2", prompt)
        self.assertNotIn("ft09", prompt)
        self.assertNotIn("bp35", prompt)


if __name__ == "__main__":
    unittest.main()
