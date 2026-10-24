"""Fail-closed AGS Sovereign authorization adapter for the SPACE broker.

Only stable identifiers and action fingerprints cross this boundary. Raw
mission intent, command arguments, application credentials, and SPACE tokens
must never be sent to or retained by this adapter.
"""
from __future__ import annotations

import json
import re
from urllib.error import HTTPError, URLError
from urllib.parse import urlsplit
from urllib.request import HTTPRedirectHandler, ProxyHandler, Request, build_opener

from mission_broker import Permit


MAX_RESPONSE_BYTES = 1_048_576
_IDENTITY = re.compile(r"^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$")
_VERDICTS = frozenset({
    "PERMIT", "PERMIT_LOG", "DENY", "ESCALATE", "REQUIRE_APPROVAL", "HALT", "RESTRICT",
})


class AGSAuthorizationError(RuntimeError):
    """Credential-free AGS protocol or transport failure."""


class _NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        return None


def _origin(value: str) -> str:
    if not isinstance(value, str):
        raise ValueError("AGS origin must be a string")
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
        raise ValueError("AGS origin must be a bare HTTP(S) origin without credentials")
    try:
        parsed.port
    except ValueError as exc:
        raise ValueError("AGS origin has an invalid port") from exc
    if parsed.scheme == "http" and parsed.hostname not in ("localhost", "127.0.0.1", "::1"):
        raise ValueError("AGS requires HTTPS except on loopback")
    return value.rstrip("/")


def _identity(value: str, name: str) -> str:
    if not isinstance(value, str) or not _IDENTITY.fullmatch(value):
        raise ValueError(f"{name} must be a stable, nonsecret identifier")
    return value


def _fingerprint(value: str) -> str:
    if not isinstance(value, str) or not re.fullmatch(r"[0-9a-f]{64}", value):
        raise ValueError("action fingerprint must be a lowercase SHA-256 digest")
    return value


class AGSPermitSource:
    """Map AGS `/evaluate` verdicts to MissionBroker permits.

    AGS is the policy decision point. This adapter never performs the proposed
    operation and never converts an error, escalation, or approval requirement
    into permission.
    """

    def __init__(
        self,
        origin: str,
        api_key: str,
        agent_id: str,
        *,
        request_timeout: float = 5.0,
    ):
        self.origin = _origin(origin)
        if not isinstance(api_key, str) or not api_key.strip() or "\r" in api_key or "\n" in api_key:
            raise ValueError("A non-empty AGS operator key is required")
        if request_timeout <= 0 or request_timeout > 30:
            raise ValueError("request_timeout must be in (0, 30]")
        self.agent_id = _identity(agent_id, "agent_id")
        self._api_key = api_key
        self._timeout = request_timeout
        self._open = build_opener(ProxyHandler({}), _NoRedirect())

    def _evaluate(
        self,
        tenant: str,
        run: str,
        fingerprint: str,
        *,
        action_type: str,
        side_effect: str,
    ) -> Permit:
        tenant = _identity(tenant, "tenant_id")
        run = _identity(run, "run_id")
        fingerprint = _fingerprint(fingerprint)
        body = {
            "agent_id": self.agent_id,
            "action_type": action_type,
            "target": "perplexity-space/firecracker",
            "parameters": {"action_sha256": fingerprint, "runtime": "fc"},
            "context": {
                "tenant_id": tenant,
                "run_id": run,
                "authorization_phase": action_type,
            },
            "side_effect": side_effect,
        }
        request = Request(
            self.origin + "/evaluate",
            data=json.dumps(body, separators=(",", ":")).encode("utf-8"),
            method="POST",
            headers={
                "Accept": "application/json",
                "Content-Type": "application/json",
                "X-AGS-KEY": self._api_key,
            },
        )
        try:
            with self._open.open(request, timeout=self._timeout) as response:
                raw = response.read(MAX_RESPONSE_BYTES + 1)
        except HTTPError as exc:
            status = exc.code
            exc.close()
            raise AGSAuthorizationError(f"AGS evaluation returned HTTP {status}") from None
        except (URLError, TimeoutError, OSError) as exc:
            raise AGSAuthorizationError(
                f"AGS evaluation transport failed ({type(exc).__name__}); action denied"
            ) from None
        if len(raw) > MAX_RESPONSE_BYTES:
            raise AGSAuthorizationError("AGS response exceeds size limit")
        try:
            result = json.loads(raw)
        except (ValueError, UnicodeDecodeError) as exc:
            raise AGSAuthorizationError("AGS returned non-JSON data") from exc
        if not isinstance(result, dict):
            raise AGSAuthorizationError("AGS returned a non-object response")
        if result.get("agent_id") != self.agent_id:
            raise AGSAuthorizationError("AGS response agent identity did not match the request")
        verdict = result.get("verdict")
        if verdict not in _VERDICTS:
            raise AGSAuthorizationError("AGS returned an unsupported verdict")
        decision_id = result.get("id")
        if not isinstance(decision_id, str) or not _IDENTITY.fullmatch(decision_id):
            raise AGSAuthorizationError("AGS returned an invalid decision identifier")
        return Permit(verdict in ("PERMIT", "PERMIT_LOG"), "ags:" + decision_id)

    def authorize_launch(self, tenant: str, run: str, fingerprint: str) -> Permit:
        return self._evaluate(
            tenant,
            run,
            fingerprint,
            action_type="space_sandbox_launch",
            side_effect="REVERSIBLE",
        )

    def authorize_command(self, tenant: str, run: str, fingerprint: str) -> Permit:
        return self._evaluate(
            tenant,
            run,
            fingerprint,
            action_type="space_command_execute",
            side_effect="IRREVERSIBLE",
        )
