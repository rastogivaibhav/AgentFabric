"""
Integration-oriented regression checks for the eval layer.

These are intentionally lightweight so they can run against a staging stack
without requiring internal test harnesses.
"""

import json
import os
from urllib import error as urlerr
from urllib import request as urlreq

import pytest


_INTEGRATION = os.environ.get("AF_TEST_MODE", "unit") == "integration"
_BASE_URL = os.environ.get("AF_API", "http://localhost:8080")
_ADMIN_USER = os.environ.get("AF_ADMIN_USER", "")
_ADMIN_PASSWORD = os.environ.get("AF_ADMIN_PASSWORD", "")
_TRACE_ID = os.environ.get("AF_EVAL_TRACE_ID", "")


def _require_integration():
    if not _INTEGRATION:
        pytest.skip("Set AF_TEST_MODE=integration to run this test")


def _json_request(url: str, method: str = "GET", body: dict | None = None, headers: dict | None = None):
    payload = None if body is None else json.dumps(body).encode()
    req = urlreq.Request(url, data=payload, headers=headers or {}, method=method)
    try:
        with urlreq.urlopen(req, timeout=10) as resp:
            raw = resp.read()
            return resp.status, json.loads(raw) if raw else {}, resp.headers
    except urlerr.HTTPError as exc:
        raw = exc.read()
        return exc.code, json.loads(raw) if raw else {}, exc.headers


def _login_cookie():
    if not (_ADMIN_USER and _ADMIN_PASSWORD):
        pytest.skip("Set AF_ADMIN_USER and AF_ADMIN_PASSWORD to run eval integration tests")
    status, _, headers = _json_request(
        f"{_BASE_URL}/auth/login",
        method="POST",
        body={"username": _ADMIN_USER, "password": _ADMIN_PASSWORD},
        headers={"Content-Type": "application/json"},
    )
    assert status in (200, 204)
    cookie = headers.get("Set-Cookie")
    assert cookie
    return cookie


@pytest.mark.integration
def test_score_trace_eval_endpoint():
    _require_integration()
    if not _TRACE_ID:
        pytest.skip("Set AF_EVAL_TRACE_ID to a real trace for eval scoring")

    cookie = _login_cookie()
    status, body, _ = _json_request(
        f"{_BASE_URL}/api/v1/evals/score",
        method="POST",
        body={"trace_id": _TRACE_ID, "release_tag": "candidate", "eval_suite": "core-release"},
        headers={"Content-Type": "application/json", "Cookie": cookie},
    )
    assert status == 200
    assert body["trace_id"] == _TRACE_ID
    assert "scores" in body
    assert body["overall_score"] >= 0


@pytest.mark.integration
def test_compare_release_regressions_endpoint():
    _require_integration()
    cookie = _login_cookie()
    status, body, _ = _json_request(
        f"{_BASE_URL}/api/v1/evals/regressions",
        method="POST",
        body={"baseline_tag": "baseline", "candidate_tag": "candidate", "eval_suite": "core-release"},
        headers={"Content-Type": "application/json", "Cookie": cookie},
    )
    assert status == 200
    assert body["baseline_tag"] == "baseline"
    assert body["candidate_tag"] == "candidate"
    assert "metrics" in body
