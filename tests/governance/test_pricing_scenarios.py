import json
import os
import time
import urllib.request as _urlreq
import urllib.error as _urlerr

import pytest


_INTEGRATION = os.environ.get("GV_TEST_MODE", "unit") == "integration"
_BASE_URL = os.environ.get("GV_API", "http://localhost:8080")
_ADMIN_USER = os.environ.get("GV_ADMIN_USER", "")
_ADMIN_PASSWORD = os.environ.get("GV_ADMIN_PASSWORD", "")


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
        pytest.skip("Set GV_TEST_MODE=integration to run this test")


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
def test_tenant_override_pricing_preview_scenario():
    _require_integration()
    if not (_ADMIN_USER and _ADMIN_PASSWORD):
        pytest.skip("Set GV_ADMIN_USER and GV_ADMIN_PASSWORD to run admin integration tests")

    cookie = _admin_cookie()
    tenant_id = f"governance-pricing-{int(time.time())}"
    created_rule_id = None

    try:
        put_status, put_body, _ = _json_request(
            f"{_BASE_URL}/api/v1/pricing",
            method="PUT",
            body={
                "tenant_id": tenant_id,
                "provider": "openai",
                "model_pattern": "gpt-4o-mini",
                "input_per_million": 9.0,
                "output_per_million": 18.0,
                "active": True,
                "priority": 999,
                "description": "temporary tenant override",
            },
            headers={"Content-Type": "application/json", "Cookie": cookie},
        )
        assert put_status == 200
        created_rule_id = put_body["id"]

        preview_status, preview_body, _ = _json_request(
            f"{_BASE_URL}/api/v1/pricing/preview",
            method="POST",
            body={
                "tenant_id": tenant_id,
                "provider": "openai",
                "model": "gpt-4o-mini",
                "input_tokens": 1000,
                "output_tokens": 1000,
            },
            headers={"Content-Type": "application/json", "Cookie": cookie},
        )
        assert preview_status == 200
        assert preview_body["matched"] is True
        assert preview_body["rule_id"] == created_rule_id
        assert preview_body["pricing_scope"] == "tenant"
        assert preview_body["total_cost_usd"] > 0
    finally:
        if created_rule_id:
            _json_request(
                f"{_BASE_URL}/api/v1/pricing/{created_rule_id}",
                method="DELETE",
                headers={"Cookie": cookie},
            )
