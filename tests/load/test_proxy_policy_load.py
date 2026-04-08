import json
import os
from concurrent.futures import ThreadPoolExecutor
import urllib.request as _urlreq
import urllib.error as _urlerr

import pytest


_INTEGRATION = os.environ.get("GV_TEST_MODE", "unit") == "integration"
_BASE_URL = os.environ.get("GV_API", "http://localhost:8080")
_ADMIN_USER = os.environ.get("GV_ADMIN_USER", "")
_ADMIN_PASSWORD = os.environ.get("GV_ADMIN_PASSWORD", "")
_VIRTUAL_KEY = os.environ.get("GV_GOVERNANCE_VIRTUAL_KEY", "")
_PROXY_PATH = os.environ.get("GV_GOVERNANCE_PROXY_PATH", "/proxy/openai/v1/chat/completions")
_PROXY_BODY = os.environ.get("GV_GOVERNANCE_PROXY_BODY", '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hello"}],"stream":false}')


def _json_request(url: str, method: str = "GET", body: dict | None = None, headers: dict | None = None):
    payload = None if body is None else json.dumps(body).encode()
    req = _urlreq.Request(url, data=payload, headers=headers or {}, method=method)
    try:
        with _urlreq.urlopen(req, timeout=15) as resp:
            raw = resp.read()
            return resp.status, json.loads(raw) if raw else {}
    except _urlerr.HTTPError as e:
        raw = e.read()
        try:
            parsed = json.loads(raw) if raw else {}
        except Exception:
            parsed = {}
        return e.code, parsed


def _require_integration():
    if not _INTEGRATION:
        pytest.skip("Set GV_TEST_MODE=integration to run this test")


def _login_cookie() -> str:
    req = _urlreq.Request(
        f"{_BASE_URL}/auth/login",
        data=json.dumps({"username": _ADMIN_USER, "password": _ADMIN_PASSWORD}).encode(),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    with _urlreq.urlopen(req, timeout=10) as resp:
        cookie = resp.headers.get("Set-Cookie")
    assert cookie
    return cookie


@pytest.mark.integration
def test_policy_preview_load_path():
    _require_integration()
    if not (_ADMIN_USER and _ADMIN_PASSWORD):
        pytest.skip("Set GV_ADMIN_USER and GV_ADMIN_PASSWORD to run policy load validation")

    cookie = _login_cookie()

    def run_preview(index: int):
        if _VIRTUAL_KEY:
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
                    raw = resp.read()
                    return resp.status, json.loads(raw) if raw else {}
            except _urlerr.HTTPError as e:
                raw = e.read()
                try:
                    parsed = json.loads(raw) if raw else {}
                except Exception:
                    parsed = {}
                return e.code, parsed

        return _json_request(
            f"{_BASE_URL}/api/v1/policies/preview",
            method="POST",
            body={
                "provider": "openai",
                "model": "gpt-4o-mini",
                "environment": "staging",
                "estimated_tokens": 64 + index,
                "request_body": f"hello {index}",
                "response_body": "safe response",
            },
            headers={"Content-Type": "application/json", "Cookie": cookie},
        )

    with ThreadPoolExecutor(max_workers=8) as pool:
        results = list(pool.map(run_preview, range(24)))

    if _VIRTUAL_KEY:
        assert all(status in (200, 403, 429) for status, _ in results)
    else:
        assert all(status == 200 for status, _ in results)
        assert all("traffic" in body for _, body in results)
