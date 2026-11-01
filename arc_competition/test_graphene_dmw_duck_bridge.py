from __future__ import annotations

import json
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "arc_competition"))
sys.path.insert(0, str(ROOT / "pi45_a1"))

from graphene_dmw_duck_bridge import GrapheneDMWDuckBridge
from a15_contract_gate import load_json, validate_turn


class GrapheneDMWDuckBridgeTests(unittest.TestCase):
    def test_negative_evidence_persists_and_triggers_dialectic(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            state = Path(td) / "state.json"
            bridge = GrapheneDMWDuckBridge(state, mode="dialectic")
            grid = [[0, 0], [0, 1]]
            bridge.record_transition(turn=0, action="ACTION1", before_grid=grid, after_grid=grid)
            bridge.record_transition(turn=1, action="ACTION1", before_grid=grid, after_grid=grid)
            self.assertEqual(bridge.negative_count(grid, "ACTION1"), 2)
            self.assertTrue(bridge.should_escalate_dialectic())
            context = bridge.prompt_context()
            self.assertIn("DIALECTIC ESCALATION ACTIVE", context)
            self.assertIn("two-consecutive-no-op-transitions", context)
            self.assertIn("repeated-dead-state-action-signature", context)

            reloaded = GrapheneDMWDuckBridge(state, mode="dialectic")
            self.assertEqual(reloaded.negative_count(grid, "ACTION1"), 2)

    def test_exact_state_action_is_blocked_after_one_observed_noop(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            bridge = GrapheneDMWDuckBridge(Path(td) / "state.json", mode="evidence")
            grid = [[0, 0], [0, 1]]
            actions = [{"action": "ACTION1"}]
            label = bridge.canonical_action(actions)
            self.assertIsNone(bridge.dead_action_reason(grid, label))
            bridge.record_transition(turn=0, action=label, before_grid=grid, after_grid=grid)
            reason = bridge.dead_action_reason(grid, label)
            self.assertIsNotNone(reason)
            self.assertIn("exact action", reason or "")
            # Same action in a changed state is not generalized into a block.
            changed_state = [[0, 0], [0, 2]]
            self.assertIsNone(bridge.dead_action_reason(changed_state, label))

    def test_off_mode_never_blocks_action(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            bridge = GrapheneDMWDuckBridge(Path(td) / "state.json", mode="off")
            grid = [[0]]
            label = bridge.canonical_action([{"action": "ACTION1"}])
            bridge.record_transition(turn=0, action=label, before_grid=grid, after_grid=grid)
            self.assertIsNone(bridge.dead_action_reason(grid, label))

    def test_changed_evidence_supersedes_dead_signature(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            bridge = GrapheneDMWDuckBridge(Path(td) / "state.json", mode="evidence")
            before = [[0, 0], [0, 1]]
            bridge.record_transition(turn=0, action="ACTION2", before_grid=before, after_grid=before)
            self.assertEqual(bridge.negative_count(before, "ACTION2"), 1)
            after = [[0, 0], [0, 2]]
            bridge.record_transition(turn=1, action="ACTION2", before_grid=before, after_grid=after)
            self.assertEqual(bridge.negative_count(before, "ACTION2"), 0)

    def test_scientist_note_becomes_compact_memory(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            bridge = GrapheneDMWDuckBridge(Path(td) / "state.json", mode="evidence")
            bridge.ingest_scientist_note({
                "world_model": " blue object may be movable ",
                "goal_model": "reach the marked region",
                "open_questions": "does ACTION2 move the object?",
                "ignored": "must not persist",
            })
            context = bridge.prompt_context()
            self.assertIn("blue object may be movable", context)
            self.assertIn("does ACTION2 move the object?", context)
            raw = json.loads(bridge.state_path.read_text())
            self.assertNotIn("ignored", raw["scientist_note"])

    def test_native_proposal_validates_against_frozen_a15_contract(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            bridge = GrapheneDMWDuckBridge(Path(td) / "state.json", mode="dialectic")
            proposal = bridge.build_native_proposal(
                turn=3,
                observation_statement="ACTION1 produced no observable grid transition from the current state.",
                observation_ref="transition:3",
                hypotheses=[
                    ("ACTION1 is ineffective in this state.", "Repeating ACTION1 leaves the grid unchanged."),
                    ("ACTION1 requires a prerequisite state that is not currently satisfied.", "After changing another object, ACTION1 may alter the grid."),
                ],
                goal_statement="Find a discriminating interaction that reveals the missing prerequisite or rules it out.",
                action="ACTION2",
                predicted_observation="The grid either changes or remains unchanged, distinguishing the alternatives.",
                falsification_questions=["What observation would show that no prerequisite is involved?"],
            )
            contract = load_json(str(ROOT / "pi45_a1/a15_state_contract.json"))
            validate_turn(proposal, contract, {"ACTION1", "ACTION2"})
            self.assertEqual(proposal["observations"][0]["evidence_kind"], "transition")
            self.assertEqual(len(proposal["hypotheses"]), 2)

    def test_duplicate_hypotheses_are_rejected_before_native_runtime(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            bridge = GrapheneDMWDuckBridge(Path(td) / "state.json", mode="dialectic")
            with self.assertRaises(ValueError):
                bridge.build_native_proposal(
                    turn=0,
                    observation_statement="no change",
                    observation_ref="transition:0",
                    hypotheses=[("same", "x"), ("same", "y")],
                    goal_statement="test",
                    action="ACTION1",
                    predicted_observation="something",
                    falsification_questions=["what differs?"],
                )


if __name__ == "__main__":
    unittest.main()
