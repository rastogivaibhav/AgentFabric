import json
import os
import time
import uuid

import pytest

try:
    import urllib.request as _urlreq
    import urllib.error as _urlerr
except ImportError:
    pass


_INTEGRATION = os.environ.get("AF_TEST_MODE", "unit") == "integration"
_BASE_URL = os.environ.get("AF_API", "http://localhost:8080")
_ADMIN_USER = os.environ.get("AF_ADMIN_USER", "")
_ADMIN_PASSWORD = os.environ.get("AF_ADMIN_PASSWORD", "")


def _json_request(url: str, method: str = "GET", body: dict | None = None, headers: dict | None = None):
    payload = None if body is None else json.dumps(body).encode()
    req = _urlreq.Request(url, data=payload, headers=headers or {}, method=method)
    try:
        with _urlreq.urlopen(req, timeout=10) as resp:
            raw = resp.read()
            parsed = json.loads(raw) if raw else {}
            return resp.status, parsed, resp.headers
    except _urlerr.HTTPError as e:
        raw = e.read()
        try:
            parsed = json.loads(raw) if raw else {}
        except Exception:
            parsed = {}
        return e.code, parsed, e.headers


def _require_integration():
    if not _INTEGRATION:
        pytest.skip("Set AF_TEST_MODE=integration to run this test")


def _admin_cookie() -> str:
    login_status, _, login_headers = _json_request(
        f"{_BASE_URL}/auth/login",
        method="POST",
        body={"username": _ADMIN_USER, "password": _ADMIN_PASSWORD},
        headers={"Content-Type": "application/json"},
    )
    assert login_status in (200, 204)
    cookie = login_headers.get("Set-Cookie")
    assert cookie
    return cookie


def test_policy_outputs_visible_in_trace_and_audit(mock_api_gateway, tenant_a_headers):
    trace_id = uuid.uuid4().hex + uuid.uuid4().hex
    mock_api_gateway.clear()
    mock_api_gateway.seed_trace({
        "id": trace_id,
        "tenant_id": "tenant-alpha",
        "framework": "proxy",
        "status": "error",
        "policy_events": [
            {
                "decision_id": "decision-1",
                "trace_id": trace_id,
                "policy_name": "deny-gpt-4o",
                "result": "deny",
                "reason": "model blocked in production",
                "provider": "openai",
                "model": "gpt-4o",
            }
        ],
        "spans": [
            {
                "id": "span-1",
                "trace_id": trace_id,
                "blocked": True,
                "blocked_reason": "model blocked in production",
                "pricing_rule_id": 44,
                "cost_usd": 0.0,
            }
        ],
    })
    mock_api_gateway.seed_audit({
        "id": 1,
        "tenant_id": "tenant-alpha",
        "category": "policy",
        "action": "deny",
        "target_type": "policy_rule",
        "target_id": "44",
        "outcome": "success",
    })

    trace_status, trace_body = _get_json(f"{mock_api_gateway.url}/api/v1/traces/{trace_id}", headers=tenant_a_headers)
    assert trace_status == 200
    assert trace_body["policy_events"][0]["result"] == "deny"
    assert trace_body["spans"][0]["blocked"] is True

    audit_status, audit_body = _get_json(f"{mock_api_gateway.url}/api/v1/audit", headers=tenant_a_headers)
    assert audit_status == 200
    assert audit_body["count"] >= 1
    assert audit_body["items"][0]["category"] == "policy"


def _get_json(url: str, headers: dict | None = None):
    req = _urlreq.Request(url, headers=headers or {}, method="GET")
    try:
        with _urlreq.urlopen(req, timeout=5) as resp:
            return resp.status, json.loads(resp.read())
    except _urlerr.HTTPError as e:
        try:
            return e.code, json.loads(e.read())
        except Exception:
            return e.code, {}


@pytest.mark.integration
def test_policy_preview_scenarios_allow_deny_redact_warn():
    _require_integration()
    if not (_ADMIN_USER and _ADMIN_PASSWORD):
        pytest.skip("Set AF_ADMIN_USER and AF_ADMIN_PASSWORD to run admin integration tests")

    cookie = _admin_cookie()
    timestamp = int(time.time())
    created_rule_ids: list[int] = []

    def create_rule(body: dict):
        status, response, _ = _json_request(
            f"{_BASE_URL}/api/v1/policies",
            method="PUT",
            body=body,
            headers={"Content-Type": "application/json", "Cookie": cookie},
        )
        assert status == 200
        created_rule_ids.append(response["id"])
        return response

    try:
        create_rule({
            "name": f"governance-allow-{timestamp}",
            "rule_type": "traffic",
            "decision_mode": "fast",
            "enabled": True,
            "priority": 2000,
            "action": "allow",
            "provider": "openai",
            "model_pattern": "gpt-4o-mini",
            "environment": "staging",
            "description": "allow safe preview traffic",
        })
        create_rule({
            "name": f"governance-deny-{timestamp}",
            "rule_type": "traffic",
            "decision_mode": "rego",
            "enabled": True,
            "priority": 2500,
            "action": "deny",
            "provider": "openai",
            "model_pattern": "gpt-4o",
            "environment": "staging",
            "rego_module": 'deny if input.environment == "staging" && input.estimated_tokens > 100',
            "description": "deny large staging traffic",
        })
        create_rule({
            "name": f"governance-redact-{timestamp}",
            "rule_type": "dlp",
            "decision_mode": "fast",
            "enabled": True,
            "priority": 2600,
            "action": "redact",
            "detector": "secret",
            "scope": "request",
            "description": "redact secrets",
        })
        create_rule({
            "name": f"governance-warn-{timestamp}",
            "rule_type": "dlp",
            "decision_mode": "rego",
            "enabled": True,
            "priority": 2550,
            "action": "warn",
            "scope": "response",
            "rego_module": 'warn if input.scope == "response" && input.response_body contains "@"',
            "description": "warn on pii-like response content",
        })

        allow_status, allow_body, _ = _json_request(
            f"{_BASE_URL}/api/v1/policies/preview",
            method="POST",
            body={
                "provider": "openai",
                "model": "gpt-4o-mini",
                "environment": "staging",
                "estimated_tokens": 20,
                "request_body": "safe request",
                "response_body": "safe response",
            },
            headers={"Content-Type": "application/json", "Cookie": cookie},
        )
        assert allow_status == 200
        assert allow_body["traffic"]["matched"] is True
        assert allow_body["traffic"]["action"] == "allow"
        assert allow_body["traffic"]["engine"] == "fast-path"

        deny_status, deny_body, _ = _json_request(
            f"{_BASE_URL}/api/v1/policies/preview",
            method="POST",
            body={
                "provider": "openai",
                "model": "gpt-4o",
                "environment": "staging",
                "estimated_tokens": 200,
                "request_body": "safe request",
                "response_body": "safe response",
            },
            headers={"Content-Type": "application/json", "Cookie": cookie},
        )
        assert deny_status == 200
        assert deny_body["traffic"]["matched"] is True
        assert deny_body["traffic"]["action"] == "deny"
        assert deny_body["traffic"]["engine"] == "rego-adapter"
        assert len(deny_body["traffic"]["condition_trace"]) > 0

        redact_status, redact_body, _ = _json_request(
            f"{_BASE_URL}/api/v1/policies/preview",
            method="POST",
            body={
                "provider": "openai",
                "model": "gpt-4o-mini",
                "environment": "staging",
                "estimated_tokens": 32,
                "request_body": "secret sk-abcdefghijklmnopqrstuvwxyz12345",
                "response_body": "safe response",
            },
            headers={"Content-Type": "application/json", "Cookie": cookie},
        )
        assert redact_status == 200
        assert redact_body["request_dlp"]["matched"] is True
        assert redact_body["request_dlp"]["action"] == "redact"
        assert redact_body["request_dlp"]["redacted_preview"]

        warn_status, warn_body, _ = _json_request(
            f"{_BASE_URL}/api/v1/policies/preview",
            method="POST",
            body={
                "provider": "openai",
                "model": "gpt-4o-mini",
                "environment": "staging",
                "estimated_tokens": 32,
                "request_body": "safe request",
                "response_body": "contact me at analyst@example.com",
            },
            headers={"Content-Type": "application/json", "Cookie": cookie},
        )
        assert warn_status == 200
        assert warn_body["response_dlp"]["matched"] is True
        assert warn_body["response_dlp"]["action"] == "warn"
        assert warn_body["response_dlp"]["engine"] == "rego-adapter"
    finally:
        for rule_id in created_rule_ids:
            _json_request(
                f"{_BASE_URL}/api/v1/policies/{rule_id}",
                method="DELETE",
                headers={"Cookie": cookie},
            )
