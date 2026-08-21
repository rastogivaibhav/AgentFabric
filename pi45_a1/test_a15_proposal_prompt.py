from __future__ import annotations

import json
import unittest

from a15_proposal_prompt import (
    SYSTEM_RULES,
    build_evidence_catalog,
    build_proposal_prompt,
    build_proposal_repair_prompt,
    canonicalize_compact_proposal,
    extract_prompt_context,
    parse_and_canonicalize_model_proposal,
    parse_json_object,
)


class A15ProposalPromptTests(unittest.TestCase):
    def test_prompt_is_bounded_and_uses_runtime_evidence_catalog(self) -> None:
        prompt = build_proposal_prompt(
            turn=5,
            game_id="secret-ft09-evaluator-id",
            observation={"grid_digest": "abc123", "grid_rle": "size=2x2;palette={0:2,1:2}\n00:0x1 1x1\n01:1x1 0x1"},
            available_actions=["ACTION1", "ACTION2"],
            recent_outcomes=[{"turn": i, "action": "ACTION1", "changed_cells": 0, "persistent_change": False, "observed_effect": "No grid cells changed."} for i in range(6)],
            governed_context={
                "status": "Contested",
                "reopened_hypothesis_ids": ["old-h2"],
                "challenged_claims": ["competing causal roots remain"],
                "noise": "y" * 4000,
            },
            max_chars=6500,
        )
        self.assertLessEqual(len(prompt), 6500)
        self.assertIn("runtime supplies an EVIDENCE CATALOG", prompt)
        self.assertIn("at least two genuinely different competing hypotheses", prompt)
        self.assertIn("never use 'win the game'", prompt)
        self.assertIn("native GrapheneDB/HypoKosh controller owns those decisions", prompt)
        self.assertIn('"ACTION1"', prompt)
        self.assertIn('"ACTION2"', prompt)
        self.assertIn("t5-o-grid", prompt)
        self.assertIn("t5-o-transition", prompt)
        self.assertIn("t5-o-affordance", prompt)
        self.assertIn("basis_observation_ids", prompt)
        self.assertIn("hypothesis_index", prompt)
        self.assertIn("tests_hypothesis_indices", prompt)
        self.assertNotIn("secret-ft09-evaluator-id", prompt)
        self.assertNotIn('"game_id"', prompt)

        output_contract = prompt.split("\n\nOUTPUT CONTRACT:\n", 1)[1]
        self.assertIn("COMPACT FIELD CONTRACT", output_contract)
        self.assertIn("Do not emit observations or canonical IDs", output_contract)
        self.assertNotIn('"observations":', output_contract)
        self.assertNotIn('"status":', output_contract)
        self.assertNotIn('"id":', output_contract)

    def test_evidence_catalog_is_deterministic_and_separates_affordance(self) -> None:
        catalog = build_evidence_catalog(
            turn=2,
            observation={"grid_digest": "abcdef0123456789", "grid_rle": "size=6x6;palette={0:31,1:4,4:1}"},
            available_actions=["ACTION1", "ACTION2", "ACTION3"],
            recent_outcomes=[{
                "turn": 1,
                "action": "ACTION1",
                "changed_cells": 0,
                "persistent_change": False,
                "observed_effect": "No grid cells changed.",
            }],
        )
        by_id = {row["id"]: row for row in catalog}
        self.assertEqual(by_id["t2-o-grid"]["evidence_kind"], "grid")
        self.assertTrue(by_id["t2-o-grid"]["evidence_ref"].startswith("grid:"))
        self.assertEqual(by_id["t2-o-transition"]["evidence_kind"], "transition")
        self.assertTrue(by_id["t2-o-transition"]["evidence_ref"].startswith("transition:"))
        self.assertEqual(by_id["t2-o-affordance"]["evidence_kind"], "affordance")
        self.assertIn("ACTION3", by_id["t2-o-affordance"]["statement"])

    def test_compact_proposal_canonicalizes_explicit_links_without_inventing_them(self) -> None:
        catalog = build_evidence_catalog(
            turn=0,
            observation={"grid_digest": "abc", "grid_rle": "size=6x6;palette={0:31,1:4,4:1}"},
            available_actions=["ACTION1", "ACTION2", "ACTION3"],
            recent_outcomes=[],
        )
        compact = {
            "hypotheses": [
                {"statement": "The visible nonzero region may respond to an opaque action.", "basis_observation_ids": ["t0-o-grid"], "prediction": "At least one grid cell changes after a useful probe."},
                {"statement": "The visible nonzero region may remain invariant under the first probe.", "basis_observation_ids": ["t0-o-grid"], "prediction": "The grid remains unchanged after the probe."},
            ],
            "candidate_goals": [
                {"statement": "Determine whether the visible structure participates in a transition rule.", "basis": [{"hypothesis_index": 0, "observation_id": "t0-o-grid"}]}
            ],
            "falsification_questions": ["Would an unchanged grid contradict the responsive interpretation?"],
            "experiment": {
                "tests_hypothesis_indices": [0, 1],
                "information_goal": "Discriminate responsive from invariant interpretations.",
                "predicted_observation": "The grid either changes or remains invariant.",
                "action": "ACTION1",
                "action_params": {},
            },
            "residual_uncertainty": ["Opaque action semantics remain unknown."],
        }
        canonical = canonicalize_compact_proposal(
            compact, turn=0, evidence_catalog=catalog,
            available_actions=["ACTION1", "ACTION2", "ACTION3"],
        )
        self.assertEqual(canonical["observations"], catalog)
        self.assertEqual(canonical["hypotheses"][0]["id"], "t0-h1")
        self.assertEqual(canonical["hypotheses"][0]["basis"], [{"observation_id": "t0-o-grid"}])
        self.assertEqual(canonical["candidate_goals"][0]["id"], "t0-g1")
        self.assertEqual(canonical["candidate_goals"][0]["basis"][0]["hypothesis_id"], "t0-h1")
        self.assertEqual(canonical["experiment"]["tests_hypothesis_ids"], ["t0-h1", "t0-h2"])

        missing_basis = json.loads(json.dumps(compact))
        missing_basis["hypotheses"][0]["basis_observation_ids"] = []
        with self.assertRaises(ValueError):
            canonicalize_compact_proposal(
                missing_basis, turn=0, evidence_catalog=catalog,
                available_actions=["ACTION1", "ACTION2", "ACTION3"],
            )

        unknown_basis = json.loads(json.dumps(compact))
        unknown_basis["hypotheses"][0]["basis_observation_ids"] = ["t0-o-unknown"]
        with self.assertRaises(ValueError):
            canonicalize_compact_proposal(
                unknown_basis, turn=0, evidence_catalog=catalog,
                available_actions=["ACTION1", "ACTION2", "ACTION3"],
            )

    def test_prompt_context_roundtrip_canonicalization(self) -> None:
        prompt = build_proposal_prompt(
            turn=1, game_id="hidden",
            observation={"grid_digest": "abc", "grid_rle": "size=2x2;palette={0:4}"},
            available_actions=["ACTION1", "ACTION2"], recent_outcomes=[], governed_context={}
        )
        context = extract_prompt_context(prompt)
        self.assertEqual(context["turn"], 1)
        raw = json.dumps({
            "hypotheses": [
                {"statement": "The current grid may change after a probe.", "basis_observation_ids": ["t1-o-grid"], "prediction": "At least one cell changes."},
                {"statement": "The current grid may remain invariant after the probe.", "basis_observation_ids": ["t1-o-grid"], "prediction": "No cell changes."},
            ],
            "candidate_goals": [{"statement": "Test whether the grid has a responsive rule.", "basis": [{"hypothesis_index": 0, "observation_id": "t1-o-grid"}]}],
            "falsification_questions": ["Would an unchanged grid favor invariance?"],
            "experiment": {"tests_hypothesis_indices": [0, 1], "information_goal": "Test responsiveness.", "predicted_observation": "Change or invariance.", "action": "ACTION1", "action_params": {}},
            "residual_uncertainty": ["Action semantics remain unknown."],
        })
        canonical = parse_and_canonicalize_model_proposal(raw, prompt=prompt)
        self.assertEqual(canonical["turn"], 1)
        self.assertEqual(canonical["hypotheses"][1]["id"], "t1-h2")

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

    def test_repair_prompt_is_compact_and_adds_no_new_evidence(self) -> None:
        original = "CURRENT EVIDENCE token-clean\nOUTPUT CONTRACT token-contract"
        rejected = json.dumps({"hypotheses": [{"statement": "x", "prediction": "y"}]})
        prompt = build_proposal_repair_prompt(
            original_prompt=original,
            invalid_output=rejected,
            validation_error="compact hypotheses[0] missing basis_observation_ids",
            turn=2,
            available_actions=["ACTION1", "ACTION2", "ACTION3"],
        )
        self.assertIn(original, prompt)
        self.assertIn("SAME compact proposal", prompt)
        self.assertIn("basis_observation_ids", prompt)
        self.assertIn("grid or transition evidence", prompt)
        self.assertIn("hypothesis_index", prompt)
        self.assertIn("tests_hypothesis_indices", prompt)
        self.assertIn("NOT world evidence", prompt)
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
        expected = {"x": 1}
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
        self.assertIn("runtime supplies an evidence catalog", lower)
        self.assertIn("do not invent or rewrite observations", lower)


if __name__ == "__main__":
    unittest.main()
