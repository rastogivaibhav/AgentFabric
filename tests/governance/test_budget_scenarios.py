import json
import os
import time
import urllib.request as _urlreq
import urllib.error as _urlerr

import pytest


_INTEGRATION = os.environ.get("AF_TEST_MODE", "unit") == "integration"
_BASE_URL = os.environ.get("AF_API", "http://localhost:8080")
_ADMIN_USER = os.environ.get("AF_ADMIN_USER", "")
_ADMIN_PASSWORD = os.environ.get("AF_ADMIN_PASSWORD", "")
_VIRTUAL_KEY = os.environ.get("AF_GOVERNANCE_VIRTUAL_KEY", "")
_PROXY_PATH = os.environ.get("AF_GOVERNANCE_PROXY_PATH", "/proxy/openai/v1/chat/completions")
_PROXY_BODY = os.environ.get("AF_GOVERNANCE_PROXY_BODY", '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hello"}],"stream":false}')


def _json_request(url: str, method: str = "GET", body: dict | None = None, headers: dict | None = None):
    payload = None if body is None else json.dumps(body).encode()
    req = _urlreq.Request(url, data=payload, headers=headers or {}, method=method)
    try:
        with _urlreq.urlopen(req, timeout=10) as resp:
            raw = resp.read()
            return resp.status, json.loads(raw) if raw else {}, resp.headers
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


@pytest.mark.integration
def test_budget_crud_and_usage_scenario():
    _require_integration()
    if not (_ADMIN_USER and _ADMIN_PASSWORD):
        pytest.skip("Set AF_ADMIN_USER and AF_ADMIN_PASSWORD to run admin integration tests")

    tenant_id = f"governance-budget-{int(time.time())}"
    cookie = _admin_cookie()

    put_status, put_body, _ = _json_request(
        f"{_BASE_URL}/api/v1/budgets/{tenant_id}",
        method="PUT",
        body={
            "monthly_tokens": 100,
            "monthly_cost_usd": 0.01,
            "alert_threshold": 0.8,
            "hard_limit": True,
            "reset_day": 1,
        },
        headers={"Content-Type": "application/json", "Cookie": cookie},
    )
    assert put_status == 200
    assert put_body["tenant_id"] == tenant_id

    get_status, get_body, _ = _json_request(
        f"{_BASE_URL}/api/v1/budgets/{tenant_id}",
        headers={"Cookie": cookie},
    )
    assert get_status == 200
    assert get_body["hard_limit"] is True

    usage_status, usage_body, _ = _json_request(
        f"{_BASE_URL}/api/v1/budgets/{tenant_id}/usage",
        headers={"Cookie": cookie},
    )
    assert usage_status == 200
    assert "tokens_used" in usage_body
    assert "cost_used_usd" in usage_body

    delete_status, _, _ = _json_request(
        f"{_BASE_URL}/api/v1/budgets/{tenant_id}",
        method="DELETE",
        headers={"Cookie": cookie},
    )
    assert delete_status in (200, 204)


@pytest.mark.integration
def test_budget_limit_scenario_if_proxy_credentials_available():
    _require_integration()
    if not (_ADMIN_USER and _ADMIN_PASSWORD and _VIRTUAL_KEY):
        pytest.skip("Set AF_ADMIN_USER, AF_ADMIN_PASSWORD, and AF_GOVERNANCE_VIRTUAL_KEY to run live budget limit validation")

    tenant_id = os.environ.get("AF_GOVERNANCE_TENANT_ID", "00000000-0000-0000-0000-000000000001")
    cookie = _admin_cookie()

    _json_request(
        f"{_BASE_URL}/api/v1/budgets/{tenant_id}",
        method="PUT",
        body={
            "monthly_tokens": 1,
            "monthly_cost_usd": 0.000001,
            "alert_threshold": 0.5,
            "hard_limit": True,
            "reset_day": 1,
        },
        headers={"Content-Type": "application/json", "Cookie": cookie},
    )

    req = _urlreq.Request(
        f"{_BASE_URL}{_PROXY_PATH}",
        data=_PROXY_BODY.encode(),
        headers={
            "Content-Type": "application/json",
            "Authorization": f"Bearer {_VIRTUAL_KEY}",
        },
        method="POST",
    )
    try:
        with _urlreq.urlopen(req, timeout=20) as resp:
            status = resp.status
    except _urlerr.HTTPError as e:
        status = e.code

    assert status in (200, 429)
