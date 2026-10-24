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

from mission_broker import MissionBroker, MissionJournal, MissionStateError, Permit, _space_key
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
        self.broker = MissionBroker(self.journal, self.provider,
                                    authorize_launch=lambda *_: Permit(True, "ags_fixture_launch"))
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
        self.assertTrue(all("at_utc" in x["metadata"] for x in self.journal.events("tenant_a", "run_001")))
        self.assertEqual(self.journal.events("tenant_a", "run_001")[2]["metadata"]["decision_ref"],
                         "ags_fixture_launch")
        raw = self.path.read_bytes()
        self.assertNotIn(b"test-management-token", raw)
        self.assertNotIn(b"Read the support ticket", raw)
        self.assertNotIn(b"fixture-output", raw)


    def test_idempotency_key_namespaces_tenant_run_and_mutation(self):
        key = _space_key("tenant_a", "run_001", "create", "same-key")
        self.assertTrue(key.startswith("ak-"))
        self.assertEqual(key, _space_key("tenant_a", "run_001", "create", "same-key"))
        self.assertNotEqual(key, _space_key("tenant_b", "run_001", "create", "same-key"))
        self.assertNotEqual(key, _space_key("tenant_a", "run_002", "create", "same-key"))
        self.assertNotEqual(key, _space_key("tenant_a", "run_001", "command", "same-key"))
        self.assertNotEqual(key, _space_key("tenant_a", "run_001", "create", "other-key"))
        self.assertNotIn("tenant_a", key)

    def test_vm_requests_use_stable_namespaced_idempotency_key(self):
        self._launch()
        create_calls = [r for r in self.server.calls if r["path"] == "/v1/sandboxes"]
        self.assertEqual(len(create_calls), 1)
        self.assertEqual(create_calls[0]["key"], _space_key(
            "tenant_a", "run_001", "create", "create-001"))
        self.assertTrue(self.journal.verify_chain("tenant_a", "run_001"))

    def test_no_launch_authorizer_fails_closed_before_space_api(self):
        self.broker.authorize_launch = None
        with self.assertRaisesRegex(MissionStateError, "No trusted launch authorization"):
            self._launch()
        self.assertEqual(self.server.calls, [])
        self.assertEqual(self.journal.get("tenant_a", "run_001").state, "new")
        self.assertEqual(self.journal.events("tenant_a", "run_001")[-1]["event"], "create.denied")

    def test_explicit_vm_launch_denial_is_recorded_without_network_call(self):
        self.broker.authorize_launch = lambda *_: Permit(False, "ags_no_vm_permission")
        with self.assertRaisesRegex(MissionStateError, "Launch explicitly denied"):
            self._launch()
        self.assertEqual(self.server.calls, [])
        self.assertEqual(
            self.journal.events("tenant_a", "run_001")[-1]["metadata"]["decision_ref"],
            "ags_no_vm_permission",
        )
        self.assertTrue(self.journal.verify_chain("tenant_a", "run_001"))

    def test_launch_authorizer_outage_fails_closed(self):
        def unavailable(*_):
            raise OSError("simulated authorization outage")
        self.broker.authorize_launch = unavailable
        with self.assertRaisesRegex(MissionStateError, "Launch authorization unavailable"):
            self._launch()
        self.assertEqual(self.server.calls, [])
        self.assertEqual(self.journal.events("tenant_a", "run_001")[-1]["event"], "create.denied")

    def test_vm_launch_policy_digest_changes_with_requested_egress(self):
        digests = []
        def capture(tenant, run, fingerprint):
            digests.append(fingerprint)
            return Permit(False, "ags_denied")
        self.broker.authorize_launch = capture
        with self.assertRaises(MissionStateError):
            self._launch()
        with self.assertRaises(MissionStateError):
            self.broker.launch(
                "tenant_a", "run_001", template="seed:latest", request_key="create-002",
                network_policy={"version": "v1", "egress": {
                    "default_action": "allow", "allow": [], "deny": []}})
        self.assertEqual(len(digests), 2)
        self.assertNotEqual(digests[0], digests[1])
        self.assertEqual(self.server.calls, [])

    def test_command_authorizer_outage_does_not_reach_vm(self):
        self._launch()
        def unavailable(*_):
            raise TimeoutError("test only")
        self.broker.authorize_command = unavailable
        before = len(self.server.calls)
        with self.assertRaisesRegex(MissionStateError, "Authorization unavailable"):
            self.broker.execute("tenant_a", "run_001", argv=["true"], request_key="auth-down")
        self.assertEqual(len(self.server.calls), before)
        self.assertEqual(self.journal.events("tenant_a", "run_001")[-1]["event"], "command.denied")

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
        self.assertEqual(
            self.journal.events("tenant_a", "run_001")[-1]["metadata"]["decision_ref"],
            "ags_deny_13",
        )
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
        self.broker = MissionBroker(self.journal, self.provider,
                                    authorize_launch=lambda *_: Permit(True, "ags_fixture_launch"))
        with self.assertRaisesRegex(MissionStateError, "not new"):
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
        with self.assertRaisesRegex(MissionStateError, "not new"):
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
