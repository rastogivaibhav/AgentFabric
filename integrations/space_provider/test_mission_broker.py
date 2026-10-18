"""Deterministic local HTTP contract tests for the AgentKraft mission broker.

The HTTP fixture DOES NOT launch Firecracker or connect to a real LLM, AGS,
PUL, or GovAgn. Passing these tests is explicitly not GA evidence.
"""
import os
from pathlib import Path
import sqlite3
import tempfile
import threading
import unittest
from http.server import ThreadingHTTPServer

from mission_broker import MissionBroker, MissionJournal, MissionStateError, Permit
from space_client import SpaceComputerProvider, SpaceError
from test_space_client import _Fixture


class MissionBrokerTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.path = Path(self.tmp.name) / "missions.sqlite3"
        self.server = ThreadingHTTPServer(("127.0.0.1", 0), _Fixture)
        self.server.mode = "normal"
        self.server.calls = []
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()
        origin = f"http://127.0.0.1:{self.server.server_port}"
        self.provider = SpaceComputerProvider(origin, origin, "test-management-token")
        self.journal = MissionJournal(self.path)
        self.broker = MissionBroker(self.journal, self.provider)
        self.journal.begin("tenant_a", "run_001", "Read the support ticket and prepare a draft")

    def tearDown(self):
        self.journal.close()
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=5)
        self.tmp.cleanup()

    def _launch(self):
        return self.broker.launch("tenant_a", "run_001", template="seed:latest", request_key="create-001")

    def test_launch_command_delete_and_evidence_survive_restart(self):
        mission = self._launch()
        self.assertEqual(mission.state, "ready")
        self.assertEqual(mission.sandbox_id, "sbx_fixture")
        self.broker.authorize_command = lambda tenant, run, digest: Permit(
            tenant == "tenant_a" and run == "run_001" and len(digest) == 64, "ags_demo_approval")
        result = self.broker.execute("tenant_a", "run_001", argv=["true"], request_key="command-001")
        self.assertEqual(result.stdout, b"fixture-output")
        self.broker.terminate("tenant_a", "run_001", request_key="delete-001")
        self.assertEqual(self.journal.get("tenant_a", "run_001").state, "deleted")
        self.assertTrue(self.journal.verify_chain("tenant_a", "run_001"))
        self.journal.close()
        self.journal = MissionJournal(self.path)
        self.assertEqual(self.journal.get("tenant_a", "run_001").state, "deleted")
        self.assertTrue(self.journal.verify_chain("tenant_a", "run_001"))
        event_names = [x["event"] for x in self.journal.events("tenant_a", "run_001")]
        self.assertEqual(event_names, [
            "mission.created", "create.reserved", "create.completed", "sandbox.ready",
            "command.reserved", "command.completed", "delete.reserved", "delete.completed"])
        self.assertEqual(self.path.stat().st_mode & 0o777, 0o600)
        raw = self.path.read_bytes()
        self.assertNotIn(b"test-management-token", raw)
        self.assertNotIn(b"Read the support ticket", raw)
        self.assertNotIn(b"fixture-output", raw)

    def test_no_approval_adapter_denies_command_without_calling_vm(self):
        self._launch()
        count = len(self.server.calls)
        with self.assertRaisesRegex(MissionStateError, "No trusted authorization"):
            self.broker.execute("tenant_a", "run_001", argv=["sh"], request_key="no-gate")
        self.assertEqual(len(self.server.calls), count)
        self.assertEqual(self.journal.get("tenant_a", "run_001").state, "ready")

    def test_explicit_denial_is_audited_without_running_command(self):
        self._launch()
        self.broker.authorize_command = lambda *_: Permit(False, "ags_deny_13")
        before = len(self.server.calls)
        with self.assertRaisesRegex(MissionStateError, "explicitly denied"):
            self.broker.execute("tenant_a", "run_001", argv=["touch", "/tmp/forbidden"],
                                request_key="denied-001")
        self.assertEqual(len(self.server.calls), before)
        self.assertEqual(self.journal.events("tenant_a", "run_001")[-1]["event"], "command.denied")
        self.assertTrue(self.journal.verify_chain("tenant_a", "run_001"))

    def test_unknown_decision_fails_closed(self):
        self._launch()
        self.broker.authorize_command = lambda *_: None
        before = len(self.server.calls)
        with self.assertRaisesRegex(MissionStateError, "Authorization failed closed"):
            self.broker.execute("tenant_a", "run_001", argv=["true"], request_key="invalid-auth")
        self.assertEqual(len(self.server.calls), before)

    def test_cross_tenant_access_rejected(self):
        with self.assertRaisesRegex(MissionStateError, "Unknown mission"):
            self.journal.get("tenant_b", "run_001")
        with self.assertRaisesRegex(MissionStateError, "Unknown mission"):
            self.broker.launch("tenant_b", "run_001", template="seed:latest",
                               request_key="wrong-tenant")
        self.assertEqual(self.server.calls, [])

    def test_ambiguous_create_blocks_duplicate_provisioning_after_restart(self):
        self.server.mode = "http_failure"
        with self.assertRaises(SpaceError):
            self._launch()
        self.assertEqual(self.journal.get("tenant_a", "run_001").state, "needs_reconciliation")
        self.server.mode = "normal"
        self.journal.close()
        self.journal = MissionJournal(self.path)
        self.broker = MissionBroker(self.journal, self.provider)
        with self.assertRaisesRegex(MissionStateError, "needs_reconciliation"):
            self._launch()
        self.assertEqual(len(self.server.calls), 1)
        self.assertEqual(self.journal.events("tenant_a", "run_001")[-1]["event"], "create.uncertain")

    def test_ready_failure_keeps_known_sandbox_not_reprovisioned(self):
        self.server.mode = "operation_failed"
        with self.assertRaises(SpaceError):
            self._launch()
        mission = self.journal.get("tenant_a", "run_001")
        self.assertEqual(mission.state, "created")
        self.assertEqual(mission.sandbox_id, "sbx_fixture")
        with self.assertRaisesRegex(MissionStateError, "disallows create"):
            self._launch()
        self.assertEqual(len([c for c in self.server.calls if c["path"] == "/v1/sandboxes"]), 1)

    def test_ambiguous_command_blocks_replay(self):
        self._launch()
        self.broker.authorize_command = lambda *_: Permit(True, "ags_local_decision")
        self.server.mode = "truncated_stream"
        with self.assertRaises(SpaceError):
            self.broker.execute("tenant_a", "run_001", argv=["true"], request_key="command-uncertain")
        self.assertEqual(self.journal.get("tenant_a", "run_001").state, "needs_reconciliation")
        before = len(self.server.calls)
        with self.assertRaisesRegex(MissionStateError, "not ready"):
            self.broker.execute("tenant_a", "run_001", argv=["true"], request_key="command-two")
        self.assertEqual(len(self.server.calls), before)

    def test_failed_delete_is_not_retried_blindly(self):
        self._launch()
        self.server.mode = "http_failure"
        with self.assertRaises(SpaceError):
            self.broker.terminate("tenant_a", "run_001", request_key="delete-unknown")
        self.assertEqual(self.journal.get("tenant_a", "run_001").state, "needs_reconciliation")
        with self.assertRaisesRegex(MissionStateError, "inspection"):
            self.broker.terminate("tenant_a", "run_001", request_key="delete-2")

    def test_chain_detects_record_tampering(self):
        self._launch()
        with self.journal._db:
            self.journal._db.execute(
                "UPDATE evidence SET event='falsified' WHERE tenant_id=? AND run_id=? AND seq=1",
                ("tenant_a", "run_001"))
        self.assertFalse(self.journal.verify_chain("tenant_a", "run_001"))

    def test_raw_intent_not_written_and_run_ids_scoped(self):
        self.assertTrue(self.journal.verify_chain("tenant_a", "run_001"))
        other = self.journal.begin("tenant_b", "run_001", "Another unrelated task")
        self.assertEqual(other.tenant_id, "tenant_b")
        self.assertNotEqual(self.journal.events("tenant_a", "run_001")[0]["event_hash"],
                            self.journal.events("tenant_b", "run_001")[0]["event_hash"])
        self.assertNotIn(b"Another unrelated task", self.path.read_bytes())

    def test_refuse_ephemeral_journal(self):
        with self.assertRaisesRegex(ValueError, "In-memory"):
            MissionJournal(":memory:")


if __name__ == "__main__":
    unittest.main()
