"""Constrained PUL client for AgentFabric application operations.

PUL owns connector policy, approvals, execution, and receipts. This client is
deliberately separate from the AGS sandbox permit source: permission to run a
command in SPACE is not permission to write to GitHub, Gmail, or another app.
"""
from __future__ import annotations

from dataclasses import dataclass
import json
import re
from typing import Any, Mapping
from urllib.error import HTTPError, URLError
from urllib.parse import urlsplit
from urllib.request import HTTPRedirectHandler, ProxyHandler, Request, build_opener


MAX_BODY_BYTES = 1_048_576
_IDENTITY = re.compile(r"^[A-Za-z0-9][A-Za-z0-9_.:-]{0,199}$")
_OPERATION = re.compile(r"^[a-z][a-z0-9_]*(?:\.[a-z][a-z0-9_]*){2,}$")
_STATUSES = frozenset({"completed", "approval_required", "failed"})


class PULProtocolError(RuntimeError):
    """PUL transport or response failure; callers must treat it as no execution."""


class _NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        return None


def _origin(value: str) -> str:
    if not isinstance(value, str):
        raise ValueError("PUL origin must be a string")
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
        raise ValueError("PUL origin must be a bare HTTP(S) origin without credentials")
    try:
        parsed.port
    except ValueError as exc:
        raise ValueError("PUL origin has an invalid port") from exc
    if parsed.scheme == "http" and parsed.hostname not in ("localhost", "127.0.0.1", "::1"):
        raise ValueError("PUL requires HTTPS except on loopback")
    return value.rstrip("/")


def _identity(value: str, name: str) -> str:
    if not isinstance(value, str) or not _IDENTITY.fullmatch(value):
        raise ValueError(f"{name} must be a stable, nonsecret identifier")
    return value


@dataclass(frozen=True)
class PULExecution:
    status: str
    receipt_id: str
    approval_id: str | None = None
    output: Any = None
    error: str | None = None

    @property
    def completed(self) -> bool:
        return self.status == "completed"

    @property
    def awaiting_approval(self) -> bool:
        return self.status == "approval_required"


class PULClient:
    """Execute an application operation through PUL's app-token boundary."""

    def __init__(
        self,
        origin: str,
        app_id: str,
        app_token: str,
        *,
        user_id: str,
        request_timeout: float = 10.0,
    ):
        self.origin = _origin(origin)
        self.app_id = _identity(app_id, "app_id")
        self.user_id = _identity(user_id, "user_id")
        if not isinstance(app_token, str) or not app_token.strip() or "\r" in app_token or "\n" in app_token:
            raise ValueError("A non-empty PUL app token is required")
        if request_timeout <= 0 or request_timeout > 60:
            raise ValueError("request_timeout must be in (0, 60]")
        self._app_token = app_token
        self._timeout = request_timeout
        self._open = build_opener(ProxyHandler({}), _NoRedirect())

    def execute(
        self,
        operation: str,
        inputs: Mapping[str, Any],
        *,
        idempotency_key: str,
    ) -> PULExecution:
        if not isinstance(operation, str) or not _OPERATION.fullmatch(operation):
            raise ValueError("operation must be a normalized PUL operation name")
        if not isinstance(inputs, Mapping):
            raise ValueError("inputs must be an object")
        idempotency_key = _identity(idempotency_key, "idempotency_key")
        body = {
            "operation": operation,
            "input": dict(inputs),
            "caller": {"app": self.app_id, "user": self.user_id},
            "idempotencyKey": idempotency_key,
        }
        try:
            encoded = json.dumps(body, separators=(",", ":"), ensure_ascii=False).encode("utf-8")
        except (TypeError, ValueError) as exc:
            raise ValueError("inputs must be JSON serializable") from exc
        if len(encoded) > MAX_BODY_BYTES:
            raise ValueError("PUL request exceeds size limit")
        request = Request(
            self.origin + "/v1/execute",
            data=encoded,
            method="POST",
            headers={
                "Accept": "application/json",
                "Authorization": "Bearer " + self._app_token,
                "Content-Type": "application/json",
            },
        )
        http_status = 0
        try:
            with self._open.open(request, timeout=self._timeout) as response:
                http_status = response.status
                raw = response.read(MAX_BODY_BYTES + 1)
        except HTTPError as exc:
            http_status = exc.code
            raw = exc.read(MAX_BODY_BYTES + 1)
            exc.close()
            if http_status != 403:
                raise PULProtocolError(f"PUL execution returned HTTP {http_status}") from None
        except (URLError, TimeoutError, OSError) as exc:
            raise PULProtocolError(
                f"PUL execution transport failed ({type(exc).__name__}); operation not accepted"
            ) from None
        if len(raw) > MAX_BODY_BYTES:
            raise PULProtocolError("PUL response exceeds size limit")
        try:
            result = json.loads(raw)
        except (ValueError, UnicodeDecodeError) as exc:
            raise PULProtocolError("PUL returned non-JSON data") from exc
        if not isinstance(result, dict):
            raise PULProtocolError("PUL returned a non-object response")
        status = result.get("status")
        if status not in _STATUSES:
            raise PULProtocolError("PUL returned an unsupported execution status")
        if http_status == 403 and status != "failed":
            raise PULProtocolError("PUL returned a non-denial result with HTTP 403")
        receipt_id = result.get("receiptId")
        if not isinstance(receipt_id, str) or not _IDENTITY.fullmatch(receipt_id):
            raise PULProtocolError("PUL returned an invalid receipt identifier")
        approval_id = result.get("approvalId")
        if status == "approval_required":
            if not isinstance(approval_id, str) or not _IDENTITY.fullmatch(approval_id):
                raise PULProtocolError("PUL approval response omitted a valid approval identifier")
        elif approval_id is not None:
            raise PULProtocolError("PUL returned an approval identifier for a terminal result")
        error = result.get("error")
        if error is not None and not isinstance(error, str):
            raise PULProtocolError("PUL returned a non-string error")
        return PULExecution(
            status=status,
            receipt_id=receipt_id,
            approval_id=approval_id,
            output=result.get("output"),
            error=error,
        )

