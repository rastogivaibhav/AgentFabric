"""SPACE API transport contract tests with a local HTTP fixture.

These tests exercise our client logic; fixture responses are NOT Firecracker
execution and MUST NEVER count as AgentKraft GA or real-microVM proof.
"""
import base64
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import threading
import unittest

from space_client import SpaceComputerProvider, SpaceError, SandboxRef


class _Fixture(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def _send(self, code, body, *, content_type="application/json", location=None):
        value = (json.dumps(body).encode() if content_type == "application/json" else body)
        self.send_response(code)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(value)))
        if location:
            self.send_header("Location", location)
        self.end_headers()
        self.wfile.write(value)

    def _handle(self):
        length = int(self.headers.get("Content-Length", "0"))
        data = self.rfile.read(length) if length else b""
        request = {"method": self.command, "path": self.path, "key": self.headers.get("Idempotency-Key"),
                   "auth": self.headers.get("Authorization"), "body": json.loads(data) if data else None}
        self.server.calls.append(request)
        mode = self.server.mode
        if mode == "http_failure":
            self._send(503, {"error": "provider unavailable"})
            return
        if mode == "redirect":
            self._send(302, {}, location="https://example.invalid/steal")
            return
        if mode == "bad_create_response" and self.command == "POST" and self.path == "/v1/sandboxes":
            self._send(202, {"sandbox_id": "sbx_fixture"})
            return
        if self.command == "POST" and self.path == "/v1/sandboxes":
            self._send(202, {"sandbox_id": "sbx_fixture", "operation_id": "op_create"})
        elif self.command == "GET" and self.path.startswith("/v1/operations/"):
            self._send(200, {"state": "succeeded" if mode != "operation_failed" else "failed"})
        elif self.command == "GET" and self.path == "/v1/sandboxes/sbx_fixture":
            self._send(200, {"state": "running"})
        elif self.command == "GET" and self.path == "/v1/sandboxes/sbx_fixture/processes?limit=1":
            self._send(200, {"data": []})
        elif self.command == "POST" and self.path == "/v1/sandboxes/sbx_fixture/commands":
            base = f"http://127.0.0.1:{self.server.server_port}"
            url = "https://example.invalid/steal" if mode == "evil_attach" else base + "/v1/sessions/proc_fixture/events"
            self._send(201, {"session_id": "proc_fixture", "process_id": "proc_run",
                             "attach": {"events_url": url, "token": "attachment-token"}})
        elif self.command == "GET" and self.path == "/v1/sessions/proc_fixture/events":
            encoded = base64.b64encode(b"fixture-output").decode()
            frames = [b"data: " + json.dumps({"type": "stdout", "data_base64": encoded}).encode() + b"\n\n"]
            if mode == "nonzero":
                frames.append(b'data: {"type":"process_exited","exit_code":4}\n\n')
            elif mode == "invalid_base64":
                frames = [b'data: {"type":"stderr","data_base64":"!!!"}\n\n']
            elif mode == "stream_error":
                frames.append(b'data: {"type":"resync_required"}\n\n')
            elif mode == "truncated_stream":
                pass
            else:
                frames.append(b'data: {"type":"process_exited","exit_code":0}\n\n')
            self._send(200, b"".join(frames), content_type="text/event-stream")
        elif self.command == "DELETE" and self.path == "/v1/sandboxes/sbx_fixture":
            self._send(202, {"operation_id": "op_deleted"})
        else:
            self._send(404, {"error": "unknown route"})

    do_GET = _handle
    do_POST = _handle
    do_DELETE = _handle


class SpaceContractTests(unittest.TestCase):
    def setUp(self):
        self.server = ThreadingHTTPServer(("127.0.0.1", 0), _Fixture)
        self.server.mode = "normal"
        self.server.calls = []
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()
        self.origin = f"http://127.0.0.1:{self.server.server_port}"
        self.events = []
        self.client = SpaceComputerProvider(self.origin, self.origin, "management-test-token",
                                            event_sink=self.events.append)

    def tearDown(self):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=5)

    def test_contract_lifecycle_command_cleanup(self):
        ref = self.client.create(template="seed:latest", request_key="test-create")
        self.assertEqual(ref, SandboxRef("sbx_fixture", "op_create"))
        self.client.wait_ready(ref, timeout=5)
        result = self.client.command(ref.sandbox_id, argv=["sh", "-c", "echo hi"], request_key="test-command")
        self.assertEqual(result.stdout, b"fixture-output")
        self.assertEqual(result.exit_code, 0)
        self.client.delete(ref.sandbox_id, request_key="test-delete", timeout=5)
        self.assertEqual([x["event"] for x in self.events],
                         ["space.create.accepted", "space.sandbox.ready", "space.command.exit",
                          "space.sandbox.deleted"])
        self.assertEqual([r["key"] for r in self.server.calls if r["method"] in ("POST", "DELETE")],
                         ["test-create", "test-command", "test-delete"])
        self.assertEqual(self.server.calls[0]["body"]["runtime"], "fc")
        self.assertEqual(self.server.calls[0]["body"]["network_policy"]["egress"]["default_action"], "deny")

    def test_management_and_attachment_tokens_are_separate(self):
        self.client.command("sbx_fixture", argv=["true"], request_key="separate-auth")
        requests = self.server.calls
        self.assertEqual(requests[0]["auth"], "Bearer management-test-token")
        self.assertEqual(requests[1]["auth"], "Bearer attachment-token")
        self.assertNotIn("attachment-token", str(self.events))

    def test_reject_remote_http(self):
        with self.assertRaises(ValueError):
            SpaceComputerProvider("http://example.com", self.origin, "token")
        with self.assertRaises(ValueError):
            SpaceComputerProvider(self.origin, "http://10.0.0.42:8090", "token")

    def test_reject_credentials_and_embedded_path(self):
        for origin in ("https://user:pw@example.com", "https://example.com/v1", "https://example.com?token=x"):
            with self.subTest(origin=origin):
                with self.assertRaises(ValueError):
                    SpaceComputerProvider(origin, self.origin, "token")

    def test_redirect_refused_without_leaking_token(self):
        self.server.mode = "redirect"
        with self.assertRaises(SpaceError) as ctx:
            self.client.create(template="seed:latest", request_key="redirect")
        self.assertIn("HTTP 302", str(ctx.exception))
        self.assertNotIn("management-test-token", str(ctx.exception))
        self.assertEqual(len(self.server.calls), 1)

    def test_refuse_cross_origin_attachment(self):
        self.server.mode = "evil_attach"
        with self.assertRaisesRegex(SpaceError, "not on the configured"):
            self.client.command("sbx_fixture", argv=["true"], request_key="origin-check")
        self.assertEqual(len(self.server.calls), 1)

    def test_unconfirmed_stream_is_not_success(self):
        self.server.mode = "truncated_stream"
        with self.assertRaisesRegex(SpaceError, "without process_exited"):
            self.client.command("sbx_fixture", argv=["true"], request_key="truncated")

    def test_nonzero_exit_fails(self):
        self.server.mode = "nonzero"
        with self.assertRaisesRegex(SpaceError, "nonzero code 4"):
            self.client.command("sbx_fixture", argv=["false"], request_key="nonzero")
        self.assertEqual(self.events[-1]["exit_code"], 4)

    def test_stream_error_fails(self):
        self.server.mode = "stream_error"
        with self.assertRaisesRegex(SpaceError, "resync_required"):
            self.client.command("sbx_fixture", argv=["true"], request_key="stream-error")

    def test_invalid_stream_base64_fails(self):
        self.server.mode = "invalid_base64"
        with self.assertRaisesRegex(SpaceError, "malformed command output"):
            self.client.command("sbx_fixture", argv=["true"], request_key="invalid-base64")

    def test_failed_operation_blocks_readiness(self):
        self.server.mode = "operation_failed"
        with self.assertRaisesRegex(SpaceError, "reached failed"):
            self.client.wait_ready(SandboxRef("sbx_fixture", "op_create"), timeout=3)

    def test_http_failure_contains_no_credentials(self):
        self.server.mode = "http_failure"
        with self.assertRaises(SpaceError) as ctx:
            self.client.create(template="seed:latest", request_key="broken")
        self.assertIn("HTTP 503", str(ctx.exception))
        self.assertNotIn("management-test-token", str(ctx.exception))

    def test_unsafe_operation_ids_are_rejected(self):
        with self.assertRaises(SpaceError):
            self.client.command("sbx_../evil", argv=["true"], request_key="unsafe")
        self.assertEqual(self.server.calls, [])

    def test_mutations_require_nonsecret_stable_key(self):
        with self.assertRaises(ValueError):
            self.client.create(template="seed:latest", request_key="newline\ninjection")
        self.assertEqual(self.server.calls, [])
        with self.assertRaises(ValueError):
            self.client.delete("sbx_fixture", request_key="")

    def test_explicit_network_policy_is_transmitted(self):
        policy = {"version": "v1", "egress": {"default_action": "deny", "allow": [{"domain": "example.org"}], "deny": []}}
        self.client.create(template="seed:latest", request_key="policy", network_policy=policy)
        self.assertEqual(self.server.calls[0]["body"]["network_policy"], policy)

    def test_create_missing_operation_id_fails_closed(self):
        self.server.mode = "bad_create_response"
        with self.assertRaisesRegex(SpaceError, "invalid op_ identifier"):
            self.client.create(template="seed:latest", request_key="missing-operation")
        self.assertEqual(self.events, [])


if __name__ == "__main__":
    unittest.main()
