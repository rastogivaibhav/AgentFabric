"""
Staging governance validation tests for AgentFabric.

These tests are integration-only and are intended for a deployed candidate
environment where admin credentials are available.
"""

import json
import os

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


@pytest.mark.integration
def test_gateway_readiness_endpoint():
    _require_integration()
    status, body, _ = _json_request(f"{_BASE_URL}/readyz")
    assert status == 200
    assert body.get("status") == "ok"


@pytest.mark.integration
def test_pricing_preview_endpoint():
    _require_integration()
    if not (_ADMIN_USER and _ADMIN_PASSWORD):
        pytest.skip("Set AF_ADMIN_USER and AF_ADMIN_PASSWORD to run admin integration tests")

    login_status, _, login_headers = _json_request(
        f"{_BASE_URL}/auth/login",
        method="POST",
        body={"username": _ADMIN_USER, "password": _ADMIN_PASSWORD},
        headers={"Content-Type": "application/json"},
    )
    assert login_status in (200, 204)
    cookie = login_headers.get("Set-Cookie")
    assert cookie

    status, body, _ = _json_request(
        f"{_BASE_URL}/api/v1/pricing/preview",
        method="POST",
        body={"provider": "openai", "model": "gpt-4o", "input_tokens": 100, "output_tokens": 50},
        headers={"Content-Type": "application/json", "Cookie": cookie},
    )
    assert status == 200
    assert body.get("total_cost_usd", 0) >= 0
    assert "matched" in body


@pytest.mark.integration
def test_policy_preview_endpoint():
    _require_integration()
    if not (_ADMIN_USER and _ADMIN_PASSWORD):
        pytest.skip("Set AF_ADMIN_USER and AF_ADMIN_PASSWORD to run admin integration tests")

    login_status, _, login_headers = _json_request(
        f"{_BASE_URL}/auth/login",
        method="POST",
        body={"username": _ADMIN_USER, "password": _ADMIN_PASSWORD},
        headers={"Content-Type": "application/json"},
    )
    assert login_status in (200, 204)
    cookie = login_headers.get("Set-Cookie")
    assert cookie

    status, body, _ = _json_request(
        f"{_BASE_URL}/api/v1/policies/preview",
        method="POST",
        body={
            "provider": "openai",
            "model": "gpt-4o",
            "environment": "staging",
            "estimated_tokens": 64,
            "request_body": "email someone@example.com",
            "response_body": "safe response",
        },
        headers={"Content-Type": "application/json", "Cookie": cookie},
    )
    assert status == 200
    assert "traffic" in body
    assert "request_dlp" in body
    assert "response_dlp" in body
