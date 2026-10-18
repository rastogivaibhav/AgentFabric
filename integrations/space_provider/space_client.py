"""Perplexity SPACE native-API computer provider for AgentKraft.

Contract integration only. A green unit test does not prove a real Firecracker
microVM, customer permissions, or production security. SPACE API baseline:
https://github.com/perplexityai/SPACE/blob/09e922efd23b3c0d109ab64de7e567a901bcc61e/examples/first_sandbox.py

No third-party dependencies and no implicit provisioning on module import.
"""
from __future__ import annotations

import base64
import binascii
from dataclasses import dataclass
import json
import re
import time
from typing import Callable
from urllib.error import HTTPError, URLError
from urllib.parse import urlsplit
from urllib.request import HTTPRedirectHandler, ProxyHandler, Request, build_opener


MAX_RESPONSE_BYTES = 1_048_576
MAX_STREAM_BYTES = 8_388_608
ID_RE = re.compile(r"^(sbx_|op_|proc_)[A-Za-z0-9_-]+$")
KEY_RE = re.compile(r"^[A-Za-z0-9_.:-]{1,128}$")
TERMINAL_FAILURE = frozenset({"failed", "canceled", "expired", "deleted", "deleting", "stopped"})


class SpaceError(RuntimeError):
    """Credential-free SPACE API or execution failure."""


class _NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        return None


@dataclass(frozen=True)
class SandboxRef:
    sandbox_id: str
    create_operation_id: str


@dataclass(frozen=True)
class CommandResult:
    process_id: str
    stdout: bytes
    stderr: bytes
    exit_code: int


def _origin(value: str) -> str:
    if not isinstance(value, str):
        raise ValueError("SPACE origin must be a string")
    parsed = urlsplit(value)
    if (
        parsed.scheme not in ("https", "http")
        or not parsed.hostname
        or parsed.username is not None
        or parsed.password is not None
        or parsed.path not in ("", "/")
        or parsed.query
        or parsed.fragment
    ):
        raise ValueError("SPACE origin must be a bare HTTP(S) origin without credentials")
    try:
        parsed.port
    except ValueError as exc:
        raise ValueError("SPACE origin has an invalid port") from exc
    if parsed.scheme == "http" and parsed.hostname not in ("localhost", "127.0.0.1", "::1"):
        raise ValueError("SPACE requires HTTPS except on loopback")
    return value.rstrip("/")


def _id(value: object, prefix: str) -> str:
    if not isinstance(value, str) or not ID_RE.fullmatch(value) or not value.startswith(prefix):
        raise SpaceError("SPACE returned an invalid " + prefix + " identifier")
    return value


def _key(value: str) -> str:
    if not isinstance(value, str) or not KEY_RE.fullmatch(value):
        raise ValueError("Supply a stable, non-secret idempotency key (1-128 safe ASCII chars)")
    return value


class SpaceComputerProvider:
    """Strict management/data-plane adapter; callers own persistent operation journals.

    Reuse the identical idempotency key AND body when resuming an ambiguous
    mutation. Never generate an implicit new retry key. Agent-issued write
    capabilities must still go through AGS/PUL; a VM is not an authorization gate.
    """

    def __init__(
        self,
        api_origin: str,
        data_origin: str,
        token: str,
        *,
        request_timeout: float = 15.0,
        event_sink: Callable[[dict], None] | None = None,
    ):
        self.api_origin = _origin(api_origin)
        self.data_origin = _origin(data_origin)
        if not isinstance(token, str) or not token.strip() or "\r" in token or "\n" in token:
            raise ValueError("A non-empty SPACE management token is required")
        if request_timeout <= 0 or request_timeout > 120:
            raise ValueError("request_timeout must be in (0, 120]")
        self._token = token
        self._timeout = request_timeout
        self._open = build_opener(ProxyHandler({}), _NoRedirect())
        self._emit = event_sink if event_sink is not None else lambda _: None

    def _json(self, method: str, path: str, body: dict | None = None, *, key: str | None = None) -> dict:
        if not path.startswith("/") or path.startswith("//") or not path.startswith("/v1/"):
            raise ValueError("Only SPACE native /v1 paths are supported")
        if method in ("POST", "DELETE") and key is None:
            raise ValueError("Mutating operations require a caller-persisted idempotency key")
        headers = {"Authorization": "Bearer " + self._token, "Accept": "application/json"}
        if key is not None:
            headers["Idempotency-Key"] = _key(key)
        if body is not None:
            headers["Content-Type"] = "application/json"
        data = json.dumps(body, separators=(",", ":")).encode() if body is not None else None
        req = Request(self.api_origin + path, data=data, headers=headers, method=method)
        try:
            with self._open.open(req, timeout=self._timeout) as response:
                raw = response.read(MAX_RESPONSE_BYTES + 1)
        except HTTPError as exc:
            raise SpaceError(f"SPACE {method} {path} returned HTTP {exc.code}") from None
        except (URLError, TimeoutError, OSError) as exc:
            raise SpaceError(f"SPACE {method} {path} transport failed ({type(exc).__name__}); inspect before retry") from None
        if len(raw) > MAX_RESPONSE_BYTES:
            raise SpaceError("SPACE response exceeds size limit")
        try:
            result = json.loads(raw)
        except (ValueError, UnicodeDecodeError) as exc:
            raise SpaceError("SPACE returned non-JSON data") from exc
        if not isinstance(result, dict):
            raise SpaceError("SPACE returned a non-object response")
        return result

    def _wait(self, path: str, target: str, timeout: float = 120.0) -> None:
        if timeout <= 0:
            raise ValueError("timeout must be positive")
        end = time.monotonic() + timeout
        while time.monotonic() < end:
            state = self._json("GET", path).get("state")
            if state == target:
                return
            if state in TERMINAL_FAILURE or (target == "running" and state in ("paused", "suspended")):
                raise SpaceError(f"SPACE {path} reached {state}, expected {target}")
            if not isinstance(state, str) or not state:
                raise SpaceError("SPACE returned a missing lifecycle state")
            time.sleep(min(0.25, max(0.0, end - time.monotonic())))
        raise SpaceError(f"SPACE {path} did not reach {target} before deadline")

    def create(
        self,
        *,
        template: str,
        request_key: str,
        network_policy: dict | None = None,
    ) -> SandboxRef:
        if not isinstance(template, str) or not template or len(template) > 200:
            raise ValueError("A SPACE template reference is required")
        # Deny all external egress by default; use a separately reviewed
        # policy to grant only the destinations required by the user's intent.
        policy = network_policy if network_policy is not None else {
            "version": "v1",
            "egress": {"default_action": "deny", "allow": [], "deny": []},
        }
        if not isinstance(policy, dict):
            raise ValueError("network_policy must be an explicit policy object")
        payload = {"template": template, "runtime": "fc", "network_policy": policy}
        result = self._json("POST", "/v1/sandboxes", payload, key=request_key)
        ref = SandboxRef(_id(result.get("sandbox_id"), "sbx_"), _id(result.get("operation_id"), "op_"))
        self._emit({"event": "space.create.accepted", "sandbox_id": ref.sandbox_id,
                    "operation_id": ref.create_operation_id})
        return ref

    def wait_ready(self, ref: SandboxRef, *, timeout: float = 120.0) -> None:
        sandbox_id = _id(ref.sandbox_id, "sbx_")
        self._wait("/v1/operations/" + _id(ref.create_operation_id, "op_"), "succeeded", timeout)
        self._wait("/v1/sandboxes/" + sandbox_id, "running", timeout)
        processes = self._json("GET", f"/v1/sandboxes/{sandbox_id}/processes?limit=1")
        if not isinstance(processes.get("data"), list):
            raise SpaceError("SPACE process endpoint did not report readiness")
        self._emit({"event": "space.sandbox.ready", "sandbox_id": sandbox_id})

    def command(
        self,
        sandbox_id: str,
        *,
        argv: list[str],
        request_key: str,
        timeout_spec: str = "30s",
    ) -> CommandResult:
        sandbox_id = _id(sandbox_id, "sbx_")
        if not isinstance(argv, list) or not argv or not all(isinstance(v, str) and v for v in argv):
            raise ValueError("argv must be a non-empty list of command arguments")
        if not re.fullmatch(r"[1-9][0-9]{0,3}s", timeout_spec):
            raise ValueError("timeout_spec must be seconds, e.g. 30s")
        result = self._json("POST", f"/v1/sandboxes/{sandbox_id}/commands",
                            {"command": argv, "timeout": timeout_spec}, key=request_key)
        session_id = _id(result.get("session_id"), "proc_")
        process_id = _id(result.get("process_id"), "proc_")
        attach = result.get("attach")
        if not isinstance(attach, dict) or not isinstance(attach.get("token"), str) or not attach["token"]:
            raise SpaceError("SPACE did not supply a session-scoped attachment token")
        path = f"/v1/sessions/{session_id}/events"
        actual = attach.get("events_url")
        if actual not in (path, self.data_origin + path):
            raise SpaceError("SPACE attachment URL is not on the configured data-plane origin")
        req = Request(self.data_origin + path, headers={
            "Authorization": "Bearer " + attach["token"],
            "Accept": "text/event-stream",
        })
        stdout, stderr = bytearray(), bytearray()
        total = 0
        exit_code = None
        pending = []
        try:
            with self._open.open(req, timeout=self._timeout) as response:
                for line in response:
                    total += len(line)
                    if total > MAX_STREAM_BYTES:
                        raise SpaceError("SPACE event stream exceeds size limit")
                    line = line.rstrip(b"\r\n")
                    if not line:
                        if not pending:
                            continue
                        try:
                            frame = json.loads(b"\n".join(pending))
                        except (ValueError, UnicodeDecodeError) as exc:
                            raise SpaceError("SPACE event frame is malformed") from exc
                        pending.clear()
                        if not isinstance(frame, dict):
                            raise SpaceError("SPACE event frame must be an object")
                        kind = frame.get("type")
                        if kind in ("error", "resync_required"):
                            raise SpaceError("SPACE command stream reported " + kind)
                        if kind in ("stdout", "stderr"):
                            try:
                                blob = base64.b64decode(frame["data_base64"], validate=True)
                            except (KeyError, ValueError, binascii.Error) as exc:
                                raise SpaceError("SPACE returned malformed command output") from exc
                            (stdout if kind == "stdout" else stderr).extend(blob)
                        if kind == "process_exited":
                            value = frame.get("exit_code")
                            if type(value) is not int:
                                raise SpaceError("SPACE process exited without an integer exit code")
                            exit_code = value
                            break
                    elif line.startswith(b"data:"):
                        pending.append(line[5:].lstrip())
                if pending and exit_code is None:
                    raise SpaceError("SPACE event stream ended in an incomplete frame")
        except HTTPError as exc:
            raise SpaceError(f"SPACE attachment failed with HTTP {exc.code}") from None
        except (URLError, OSError, TimeoutError) as exc:
            raise SpaceError(f"SPACE attachment transport failed ({type(exc).__name__})") from None
        if exit_code is None:
            raise SpaceError("SPACE stream ended without process_exited; execution not verified")
        self._emit({"event": "space.command.exit", "sandbox_id": sandbox_id,
                    "process_id": process_id, "exit_code": exit_code,
                    "stdout_bytes": len(stdout), "stderr_bytes": len(stderr)})
        if exit_code != 0:
            raise SpaceError(f"SPACE command exited with nonzero code {exit_code}")
        return CommandResult(process_id, bytes(stdout), bytes(stderr), exit_code)

    def delete(self, sandbox_id: str, *, request_key: str, timeout: float = 120.0) -> None:
        sandbox_id = _id(sandbox_id, "sbx_")
        result = self._json("DELETE", "/v1/sandboxes/" + sandbox_id, key=request_key)
        operation_id = _id(result.get("operation_id"), "op_")
        self._wait("/v1/operations/" + operation_id, "succeeded", timeout)
        self._emit({"event": "space.sandbox.deleted", "sandbox_id": sandbox_id,
                    "operation_id": operation_id})
