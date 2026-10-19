"""Contract tests for the constrained PUL application-operation client."""
import json
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from pul_client import PULClient, PULProtocolError


class _PULFixture(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def do_POST(self):
        length = int(self.headers.get("Content-Length", "0"))
        body = json.loads(self.rfile.read(length))
        self.server.calls.append({
            "path": self.path,
            "authorization": self.headers.get("Authorization"),
            "body": body,
        })
        if self.server.mode == "failure":
            self.send_response(503)
            payload = {"error": "unavailable"}
        elif self.server.mode == "redirect":
            self.send_response(302)
            self.send_header("Location", "https://example.invalid/steal")
            payload = {}
        elif self.server.mode == "malformed":
            self.send_response(200)
            payload = {"status": "completed", "receiptId": 7}
        elif self.server.mode == "approval_required":
            self.send_response(200)
            payload = {
                "status": "approval_required",
                "receiptId": "rec_123",
                "approvalId": "apr_123",
            }
        elif self.server.mode == "failed":
            self.send_response(403)
            payload = {
                "status": "failed",
                "receiptId": "rec_denied",
                "error": "operation explicitly denied",
            }
        else:
            self.send_response(200)
            payload = {
                "status": "completed",
                "receiptId": "rec_123",
                "output": {"mode": "dry_run"},
            }
        encoded = json.dumps(payload).encode()
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(encoded)))
        self.end_headers()
        self.wfile.write(encoded)


class PULClientTests(unittest.TestCase):
    def setUp(self):
        self.server = ThreadingHTTPServer(("127.0.0.1", 0), _PULFixture)
        self.server.mode = "completed"
        self.server.calls = []
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()
        origin = f"http://127.0.0.1:{self.server.server_port}"
        self.client = PULClient(
            origin, "app_agentfabric", "pul_app_secret", user_id="agentfabric-broker"
        )

    def tearDown(self):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=5)

    def test_completed_operation_binds_app_operation_and_idempotency_key(self):
        execution = self.client.execute(
            "repo.github.search_code",
            {"query": "AgentFabric"},
            idempotency_key="tenant-a.run-1.search-1",
        )
        self.assertTrue(execution.completed)
        self.assertEqual(execution.receipt_id, "rec_123")
        request = self.server.calls[0]
        self.assertEqual(request["path"], "/v1/execute")
        self.assertEqual(request["authorization"], "Bearer pul_app_secret")
        self.assertEqual(request["body"]["caller"]["app"], "app_agentfabric")
        self.assertEqual(request["body"]["caller"]["user"], "agentfabric-broker")
        self.assertEqual(request["body"]["operation"], "repo.github.search_code")
        self.assertEqual(request["body"]["idempotencyKey"], "tenant-a.run-1.search-1")

    def test_approval_required_is_not_reported_as_completed(self):
        self.server.mode = "approval_required"
        execution = self.client.execute(
            "repo.github.create_issue", {}, idempotency_key="approval-1"
        )
        self.assertFalse(execution.completed)
        self.assertTrue(execution.awaiting_approval)
        self.assertEqual(execution.approval_id, "apr_123")

    def test_explicit_denial_preserves_receipt_and_error(self):
        self.server.mode = "failed"
        execution = self.client.execute(
            "repo.github.search_code", {}, idempotency_key="denied-1"
        )
        self.assertFalse(execution.completed)
        self.assertEqual(execution.receipt_id, "rec_denied")
        self.assertEqual(execution.error, "operation explicitly denied")

    def test_transport_failure_and_redirect_do_not_execute_twice(self):
        for mode in ("failure", "redirect"):
            with self.subTest(mode=mode):
                self.server.mode = mode
                before = len(self.server.calls)
                with self.assertRaises(PULProtocolError):
                    self.client.execute(
                        "repo.github.search_code", {}, idempotency_key=f"{mode}-1"
                    )
                self.assertEqual(len(self.server.calls), before + 1)

    def test_malformed_result_fails_closed(self):
        self.server.mode = "malformed"
        with self.assertRaises(PULProtocolError):
            self.client.execute(
                "repo.github.search_code", {}, idempotency_key="malformed-1"
            )

    def test_remote_http_embedded_credentials_and_bad_operation_are_rejected(self):
        with self.assertRaises(ValueError):
            PULClient("http://example.com", "app_x", "token", user_id="broker")
        with self.assertRaises(ValueError):
            PULClient(
                "https://user:secret@example.com", "app_x", "token", user_id="broker"
            )
        with self.assertRaises(ValueError):
            self.client.execute("bad operation", {}, idempotency_key="bad-1")
        self.assertEqual(self.server.calls, [])


if __name__ == "__main__":
    unittest.main()
