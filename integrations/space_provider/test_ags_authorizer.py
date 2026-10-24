"""Contract tests for the fail-closed AGS authorization adapter."""
import json
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from ags_authorizer import AGSAuthorizationError, AGSPermitSource


class _AGSFixture(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def do_POST(self):
        length = int(self.headers.get("Content-Length", "0"))
        body = json.loads(self.rfile.read(length))
        self.server.calls.append({
            "path": self.path,
            "key": self.headers.get("X-AGS-KEY"),
            "body": body,
        })
        if self.server.mode == "failure":
            self.send_response(503)
            payload = b'{"error":"unavailable"}'
        elif self.server.mode == "redirect":
            self.send_response(302)
            self.send_header("Location", "https://example.invalid/steal")
            payload = b"{}"
        elif self.server.mode == "bad_agent":
            self.send_response(200)
            payload = json.dumps({
                "id": "eval_wrong_agent", "agent_id": "other-agent", "verdict": "PERMIT",
            }).encode()
        elif self.server.mode == "bad_verdict":
            self.send_response(200)
            payload = json.dumps({
                "id": "eval_bad_verdict", "agent_id": "agentkraft-space", "verdict": "MAYBE",
            }).encode()
        else:
            self.send_response(200)
            payload = json.dumps({
                "id": "eval_123", "agent_id": "agentkraft-space", "verdict": self.server.mode,
            }).encode()
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)


class AGSPermitSourceTests(unittest.TestCase):
    def setUp(self):
        self.server = ThreadingHTTPServer(("127.0.0.1", 0), _AGSFixture)
        self.server.mode = "PERMIT"
        self.server.calls = []
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()
        self.origin = f"http://127.0.0.1:{self.server.server_port}"
        self.source = AGSPermitSource(self.origin, "operator-secret", "agentkraft-space")

    def tearDown(self):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=5)

    def test_launch_permit_binds_exact_fingerprint_and_scope(self):
        digest = "a" * 64
        permit = self.source.authorize_launch("tenant_a", "run_001", digest)
        self.assertTrue(permit.allow)
        self.assertEqual(permit.decision_ref, "ags:eval_123")
        request = self.server.calls[0]
        self.assertEqual(request["path"], "/evaluate")
        self.assertEqual(request["key"], "operator-secret")
        self.assertEqual(request["body"]["action_type"], "space_sandbox_launch")
        self.assertEqual(request["body"]["parameters"]["action_sha256"], digest)
        self.assertEqual(request["body"]["context"]["tenant_id"], "tenant_a")
        self.assertEqual(request["body"]["context"]["run_id"], "run_001")
        self.assertEqual(request["body"]["side_effect"], "REVERSIBLE")

    def test_command_is_conservatively_irreversible(self):
        permit = self.source.authorize_command("tenant_a", "run_001", "b" * 64)
        self.assertTrue(permit.allow)
        self.assertEqual(self.server.calls[0]["body"]["action_type"], "space_command_execute")
        self.assertEqual(self.server.calls[0]["body"]["side_effect"], "IRREVERSIBLE")

    def test_deny_escalate_and_approval_requirement_never_permit(self):
        for verdict in ("DENY", "ESCALATE", "REQUIRE_APPROVAL", "HALT", "RESTRICT"):
            with self.subTest(verdict=verdict):
                self.server.mode = verdict
                permit = self.source.authorize_command("tenant_a", "run_001", "c" * 64)
                self.assertFalse(permit.allow)
                self.assertEqual(permit.decision_ref, "ags:eval_123")

    def test_permit_log_is_a_permit(self):
        self.server.mode = "PERMIT_LOG"
        self.assertTrue(self.source.authorize_launch("tenant_a", "run_001", "d" * 64).allow)

    def test_transport_failure_is_not_a_permit(self):
        self.server.mode = "failure"
        with self.assertRaisesRegex(AGSAuthorizationError, "HTTP 503"):
            self.source.authorize_launch("tenant_a", "run_001", "e" * 64)

    def test_redirect_is_refused_without_leaking_key(self):
        self.server.mode = "redirect"
        with self.assertRaises(AGSAuthorizationError) as ctx:
            self.source.authorize_launch("tenant_a", "run_001", "f" * 64)
        self.assertIn("HTTP 302", str(ctx.exception))
        self.assertNotIn("operator-secret", str(ctx.exception))
        self.assertEqual(len(self.server.calls), 1)

    def test_mismatched_agent_and_unknown_verdict_fail_closed(self):
        for mode in ("bad_agent", "bad_verdict"):
            with self.subTest(mode=mode):
                self.server.mode = mode
                with self.assertRaises(AGSAuthorizationError):
                    self.source.authorize_launch("tenant_a", "run_001", "0" * 64)

    def test_invalid_fingerprint_never_reaches_ags(self):
        with self.assertRaises(ValueError):
            self.source.authorize_launch("tenant_a", "run_001", "not-a-digest")
        self.assertEqual(self.server.calls, [])

    def test_remote_plain_http_and_embedded_credentials_are_rejected(self):
        with self.assertRaises(ValueError):
            AGSPermitSource("http://example.com", "key", "agent")
        with self.assertRaises(ValueError):
            AGSPermitSource("https://user:secret@example.com", "key", "agent")


if __name__ == "__main__":
    unittest.main()
