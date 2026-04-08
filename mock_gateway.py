"""
Govagn Mock API Gateway
Accepts OTLP spans from the collector, stores in memory,
serves all portal API endpoints + WebSocket live stream.

Run: python mock_gateway.py
     (or: uvicorn mock_gateway:app --port 8080 --reload)
"""
import base64
import hashlib
import hmac
import json
import os
import time
import uuid
import asyncio
import random
from collections import defaultdict, deque
from datetime import datetime, timezone
from typing import Any

from fastapi import FastAPI, WebSocket, WebSocketDisconnect, Request, Response
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse
import uvicorn

# ─── Basic-auth credentials (override via env for staging/prod) ───────────────
ADMIN_USER     = os.environ.get("GV_ADMIN_USER",     "admin")
ADMIN_PASSWORD = os.environ.get("GV_ADMIN_PASSWORD", "admin")
JWT_SECRET     = os.environ.get("GV_JWT_SECRET",     "dev-secret-changeme")

app = FastAPI(title="Govagn Mock Gateway")

# ─── CORS (portal runs on :3000 or :5173) ────────────────────────────────────
# allow_credentials=True requires explicit origins — wildcard ("*") is rejected
# by browsers when credentials are included.
app.add_middleware(
    CORSMiddleware,
    allow_origins=["http://localhost:3000", "http://localhost:5173"],
    allow_methods=["*"],
    allow_headers=["*"],
    allow_credentials=True,
)

# ─── In-Memory Store ─────────────────────────────────────────────────────────
spans: deque = deque(maxlen=5000)          # ring buffer, newest last
ws_clients: list[WebSocket] = []

def now_ms() -> int:
    return int(time.time() * 1000)

def ts_str(ns: int) -> str:
    if not ns:
        return datetime.now(timezone.utc).isoformat()
    return datetime.fromtimestamp(ns / 1e9, tz=timezone.utc).isoformat()

def make_span(raw: dict) -> dict:
    """Normalise a raw collector EnrichedSpan into a portal Span shape."""
    attrs = raw.get("attributes") or raw.get("Attributes") or {}
    status_code = raw.get("status_code") or raw.get("StatusCode", 0)
    duration_ns = raw.get("duration_ns") or raw.get("DurationNs") or 0
    start_ns    = raw.get("start_time_ns") or raw.get("StartTimeNs") or 0
    return {
        # ── Span interface ────────────────────────────────────────────────
        "id":            raw.get("span_id")        or raw.get("SpanID")        or str(uuid.uuid4())[:16],
        "trace_id":      raw.get("trace_id")       or raw.get("TraceID")       or str(uuid.uuid4()),
        "parent_id":     raw.get("parent_span_id") or raw.get("ParentSpanID")  or "",
        "run_id":        raw.get("run_id")         or raw.get("RunID")         or attrs.get("af.agent.run_id") or "",
        "name":          raw.get("name")           or raw.get("Name")          or "unknown",
        "framework":     raw.get("framework")      or raw.get("Framework")     or attrs.get("af.agent.framework", "unknown"),
        "start_time_ns": start_ns,
        "duration_ns":   duration_ns,
        "status_code":   2 if status_code == 2 else 0,
        "status_msg":    raw.get("status_message") or raw.get("StatusMessage") or "",
        "attributes":    attrs,
        "events":        raw.get("events") or [],
        "input_tokens":  int(raw.get("input_tokens") or raw.get("InputTokens") or attrs.get("gen_ai.usage.input_tokens") or 0),
        "output_tokens": int(raw.get("output_tokens") or raw.get("OutputTokens") or attrs.get("gen_ai.usage.output_tokens") or 0),
        "cost_usd":      float(raw.get("cost_usd") or raw.get("CostUSD") or attrs.get("af.cost.total_usd") or 0),
        # ── extras kept for internal use ─────────────────────────────────
        "agent_name":    attrs.get("af.agent.name") or "",
        "model":         attrs.get("gen_ai.request.model") or attrs.get("gen_ai.model", ""),
        "environment":   "development",
        "received_at":   datetime.now(timezone.utc).isoformat(),
        "start_time":    ts_str(start_ns),
    }

def span_status(s: dict) -> str:
    return "error" if s["status_code"] == 2 else "ok"

def build_trace(trace_spans: list[dict]) -> dict:
    """Aggregate a list of Span dicts into a Trace dict matching the TS Trace interface."""
    root = next((s for s in trace_spans if not s["parent_id"]), trace_spans[0])
    total_tokens = sum((s.get("input_tokens") or 0) + (s.get("output_tokens") or 0) for s in trace_spans)
    error_count  = sum(1 for s in trace_spans if s["status_code"] == 2)
    return {
        # ── Trace interface ───────────────────────────────────────────────
        "id":             root["trace_id"],
        "root_span_name": root["name"],
        "framework":      root["framework"],
        "start_time":     root["start_time"],
        "duration_ns":    root["duration_ns"],
        "span_count":     len(trace_spans),
        "error_count":    error_count,
        "total_cost_usd": round(sum(s.get("cost_usd") or 0 for s in trace_spans), 6),
        "total_tokens":   total_tokens,
        "status":         "error" if error_count > 0 else "ok",
        "spans":          trace_spans,
    }


# ─── Ingest endpoint (collector POSTs enriched spans here) ───────────────────
@app.post("/internal/ingest")
async def ingest(request: Request):
    try:
        body = await request.json()
    except Exception:
        return JSONResponse({"error": "bad json"}, status_code=400)

    # Collector wraps: {"spans": [...]}  — unwrap if present
    if isinstance(body, dict) and "spans" in body:
        batch = body["spans"]
    elif isinstance(body, list):
        batch = body
    else:
        batch = [body]

    new_spans = []

    for raw in batch:
        if not isinstance(raw, dict):
            continue
        s = make_span(raw)
        spans.append(s)
        new_spans.append(s)

    # Broadcast to live stream WebSocket clients as individual LiveEvents
    if new_spans and ws_clients:
        dead = []
        for ws in ws_clients:
            try:
                for s in new_spans:
                    event = {"type": "span", "ts": now_ms(), "data": s}
                    await ws.send_text(json.dumps(event))
            except Exception:
                dead.append(ws)
        for ws in dead:
            ws_clients.remove(ws)

    return JSONResponse({"accepted": len(new_spans)})


# ─── Health ──────────────────────────────────────────────────────────────────
@app.get("/healthz")
def health():
    return {"status": "ok", "spans_stored": len(spans), "version": "mock-1.0"}


# ─── Analytics: Overview ─────────────────────────────────────────────────────
@app.get("/api/v1/analytics/overview")
def overview():
    all_spans  = list(spans)
    errors     = [s for s in all_spans if s["status_code"] == 2]
    durations  = [s["duration_ns"] / 1e6 for s in all_spans if s["duration_ns"] > 0]
    total_tokens = sum((s.get("input_tokens") or 0) + (s.get("output_tokens") or 0) for s in all_spans)

    framework_counts: dict[str, int] = defaultdict(int)
    for s in all_spans:
        framework_counts[s["framework"]] += 1

    # spans_per_second based on window of last 60 s
    now = now_ms()
    recent_60s = [s for s in all_spans if (now - (s["start_time_ns"] / 1e6 if s["start_time_ns"] else now)) <= 60_000]
    spans_per_second = round(len(recent_60s) / 60, 2)

    return {
        # ── OverviewStats interface ───────────────────────────────────────
        "total_traces":   len(set(s["trace_id"] for s in all_spans)),
        "active_agents":  len(set(s["agent_name"] for s in all_spans if s["agent_name"])),
        "total_cost_usd": round(sum(s.get("cost_usd") or 0 for s in all_spans), 6),
        "total_tokens":   total_tokens,
        "error_rate":     round(len(errors) / max(len(all_spans), 1) * 100, 2),
        "avg_latency_ms": round(sum(durations) / max(len(durations), 1), 1),
        "spans_per_second": spans_per_second,
        "framework_counts": dict(framework_counts),
    }


# ─── Analytics: Frameworks ───────────────────────────────────────────────────
@app.get("/api/v1/analytics/frameworks")
def frameworks():
    counts: dict[str, int] = defaultdict(int)
    for s in spans:
        counts[s["framework"]] += 1
    return [{"framework": k, "count": v} for k, v in sorted(counts.items(), key=lambda x: -x[1])]


# ─── Analytics: Cost ─────────────────────────────────────────────────────────
@app.get("/api/v1/analytics/cost")
def cost():
    by_model: dict[str, dict] = defaultdict(lambda: {"input_tokens": 0, "output_tokens": 0, "cost_usd": 0.0, "span_count": 0})
    for s in spans:
        m = s["model"] or "unknown"
        by_model[m]["input_tokens"]  += s.get("input_tokens") or 0
        by_model[m]["output_tokens"] += s.get("output_tokens") or 0
        by_model[m]["cost_usd"]      += s.get("cost_usd") or 0
        by_model[m]["span_count"]    += 1
    return [{"model": k, **v, "cost_usd": round(v["cost_usd"], 6)} for k, v in sorted(by_model.items(), key=lambda x: -x[1]["cost_usd"])]


# ─── Analytics: Errors ───────────────────────────────────────────────────────
@app.get("/api/v1/analytics/errors")
def errors_report():
    errs = [s for s in spans if s["status_code"] == 2]
    return {
        "total_errors": len(errs),
        "error_rate":   round(len(errs) / max(len(spans), 1) * 100, 2),
        "by_framework": {fw: sum(1 for e in errs if e["framework"] == fw)
                         for fw in set(e["framework"] for e in errs)},
        "recent": [{"span_id": e["id"], "name": e["name"],
                    "framework": e["framework"], "at": e["start_time"]} for e in list(errs)[-10:]],
    }


# ─── Traces: List  →  Page<Trace> ────────────────────────────────────────────
@app.get("/api/v1/traces")
def list_traces(limit: int = 50, framework: str = None, status: str = None, cursor: str = None):
    # Group all spans by trace_id
    by_trace: dict[str, list] = defaultdict(list)
    for s in spans:
        by_trace[s["trace_id"]].append(s)

    # Build Trace objects
    trace_list = [build_trace(v) for v in by_trace.values()]

    # Sort newest first (by start_time string — ISO strings sort lexicographically)
    trace_list.sort(key=lambda t: t["start_time"], reverse=True)

    if framework:
        trace_list = [t for t in trace_list if t["framework"] == framework]
    if status:
        trace_list = [t for t in trace_list if t["status"] == status]

    # Strip embedded spans from list view (keep lightweight)
    items = [{k: v for k, v in t.items() if k != "spans"} for t in trace_list[:limit]]

    return {
        "items":   items,
        "total":   len(trace_list),
        "has_more": len(trace_list) > limit,
    }


# ─── Traces: Detail  →  Trace ────────────────────────────────────────────────
@app.get("/api/v1/traces/{trace_id}")
def get_trace(trace_id: str):
    trace_spans = [s for s in spans if s["trace_id"] == trace_id]
    if not trace_spans:
        return JSONResponse({"error": "not found"}, status_code=404)
    return build_trace(trace_spans)


# ─── Traces: Timeline ────────────────────────────────────────────────────────
@app.get("/api/v1/traces/{trace_id}/timeline")
def trace_timeline(trace_id: str):
    return [s for s in spans if s["trace_id"] == trace_id]


# ─── Traces: Graph ───────────────────────────────────────────────────────────
@app.get("/api/v1/traces/{trace_id}/graph")
def trace_graph(trace_id: str):
    trace_spans = [s for s in spans if s["trace_id"] == trace_id]
    nodes = [{"id": s["id"], "name": s["name"], "status": span_status(s),
               "duration_ms": round(s["duration_ns"] / 1e6, 2)} for s in trace_spans]
    edges = [{"from": s["parent_id"], "to": s["id"]}
              for s in trace_spans if s["parent_id"]]
    return {"nodes": nodes, "edges": edges}


# ─── Traces: Cost ────────────────────────────────────────────────────────────
@app.get("/api/v1/traces/{trace_id}/cost")
def trace_cost(trace_id: str):
    trace_spans = [s for s in spans if s["trace_id"] == trace_id]
    return {
        "total_usd":    round(sum(s.get("cost_usd") or 0 for s in trace_spans), 6),
        "input_tokens": sum(s.get("input_tokens") or 0 for s in trace_spans),
        "output_tokens":sum(s.get("output_tokens") or 0 for s in trace_spans),
        "by_model":     {},
        "by_span":      [{"span_id": s["id"], "name": s["name"],
                           "cost_usd": s.get("cost_usd") or 0,
                           "model": s["model"]} for s in trace_spans],
    }


# ─── Agents ──────────────────────────────────────────────────────────────────
@app.get("/api/v1/agents")
def list_agents():
    by_agent: dict[str, dict] = {}
    for s in spans:
        name = s["agent_name"] or s["name"].split(".")[0]
        if name not in by_agent:
            by_agent[name] = {"name": name, "framework": s["framework"],
                               "run_count": 0, "total_cost": 0.0,
                               "error_count": 0, "last_seen": s["start_time"]}
        by_agent[name]["run_count"]   += 1
        by_agent[name]["total_cost"]  += s.get("cost_usd") or 0
        by_agent[name]["error_count"] += 1 if s["status_code"] == 2 else 0
        by_agent[name]["last_seen"]    = s["start_time"]
    return list(by_agent.values())


@app.get("/api/v1/agents/{agent_id}")
def get_agent(agent_id: str):
    agent_spans = [s for s in spans if (s["agent_name"] or s["name"]) == agent_id]
    return {
        "name":        agent_id,
        "framework":   agent_spans[0]["framework"] if agent_spans else "unknown",
        "run_count":   len(agent_spans),
        "total_cost":  round(sum(s.get("cost_usd") or 0 for s in agent_spans), 6),
        "error_count": sum(1 for s in agent_spans if s["status_code"] == 2),
        "spans":       agent_spans[-20:],
    }


@app.get("/api/v1/agents/{agent_id}/runs")
def agent_runs(agent_id: str):
    return [s for s in spans if s["agent_name"] == agent_id][-50:]


@app.get("/api/v1/agents/{agent_id}/metrics")
def agent_metrics(agent_id: str):
    return overview()


@app.get("/api/v1/agents/{agent_id}/topology")
def agent_topology(agent_id: str):
    return {"nodes": [], "edges": []}


# ─── Runs ─────────────────────────────────────────────────────────────────────
@app.get("/api/v1/runs")
def list_runs():
    by_run: dict[str, dict] = {}
    for s in spans:
        rid = s["run_id"] or s["trace_id"]
        if rid not in by_run:
            by_run[rid] = {**s, "child_count": 0}
        by_run[rid]["child_count"] += 1
    return list(by_run.values())[-50:]


@app.get("/api/v1/runs/{run_id}")
def get_run(run_id: str):
    run_spans = [s for s in spans if s["run_id"] == run_id or s["trace_id"] == run_id]
    if not run_spans:
        return JSONResponse({"error": "not found"}, status_code=404)
    return {**run_spans[0], "spans": run_spans}


@app.get("/api/v1/runs/{run_id}/children")
def run_children(run_id: str):
    return [s for s in spans if s["parent_id"] and s["run_id"] == run_id]


@app.post("/api/v1/runs/{run_id}/feedback")
async def run_feedback(run_id: str, request: Request):
    return {"status": "ok", "run_id": run_id}


# ─── Environments ────────────────────────────────────────────────────────────
@app.get("/api/v1/environments")
def list_environments():
    return [
        {"name": "development", "status": "active", "span_count": len(spans)},
        {"name": "staging",     "status": "inactive", "span_count": 0},
        {"name": "production",  "status": "inactive", "span_count": 0},
    ]


# ─── WebSocket Live Stream ────────────────────────────────────────────────────
@app.websocket("/api/v1/stream/live")
async def live_stream(ws: WebSocket):
    await ws.accept()
    ws_clients.append(ws)
    print(f"[WS] client connected  (total: {len(ws_clients)})")
    try:
        # Replay last 20 spans as individual LiveEvents so UI isn't empty on connect
        recent = list(spans)[-20:]
        for s in recent:
            event = {"type": "span", "ts": now_ms(), "data": s}
            await ws.send_text(json.dumps(event))
        while True:
            await asyncio.sleep(30)
            await ws.send_text(json.dumps({"type": "ping", "ts": now_ms(), "data": {}}))
    except WebSocketDisconnect:
        pass
    finally:
        if ws in ws_clients:
            ws_clients.remove(ws)
        print(f"[WS] client disconnected (total: {len(ws_clients)})")


# ─── Auth helpers ─────────────────────────────────────────────────────────────
def _b64url(data: bytes) -> str:
    return base64.urlsafe_b64encode(data).rstrip(b"=").decode()

def _make_jwt(sub: str, email: str, tenant_id: str = "tenant-default") -> str:
    """Issue a minimal HS256 JWT valid for 8 hours."""
    header  = _b64url(json.dumps({"alg": "HS256", "typ": "JWT"}).encode())
    payload = _b64url(json.dumps({
        "sub": sub,
        "email": email,
        "tenant_id": tenant_id,
        "iat": int(time.time()),
        "exp": int(time.time()) + 28800,  # 8 h
    }).encode())
    sig_input = f"{header}.{payload}".encode()
    sig = _b64url(hmac.new(JWT_SECRET.encode(), sig_input, hashlib.sha256).digest())
    return f"{header}.{payload}.{sig}"


SESSION_MAX_AGE = 8 * 60 * 60  # 8 hours in seconds


def _decode_token(token_str: str) -> dict | None:
    """Decode JWT payload without signature verification (mock only)."""
    try:
        parts = token_str.split(".")
        if len(parts) != 3:
            return None
        padding = 4 - len(parts[1]) % 4
        payload = json.loads(base64.urlsafe_b64decode(parts[1] + "=" * padding))
        if payload.get("exp", 0) < time.time():
            return None
        return payload
    except Exception:
        return None


def _token_from_request(request: Request) -> str | None:
    """Extract JWT from Authorization: Bearer header or af_token cookie."""
    auth = request.headers.get("Authorization", "")
    if auth.startswith("Bearer "):
        return auth[7:]
    return request.cookies.get("af_token")


@app.post("/auth/login")
async def basic_login(request: Request):
    """Username/password login — sets HttpOnly cookie and returns token for CLI callers."""
    try:
        body = await request.json()
    except Exception:
        return JSONResponse({"error": "invalid JSON"}, status_code=400)

    username = (body.get("username") or "").strip()
    password = (body.get("password") or "").strip()

    if username == ADMIN_USER and password == ADMIN_PASSWORD:
        token = _make_jwt(sub=username, email=f"{username}@govagn.local")
        resp = JSONResponse({"token": token})
        # HttpOnly cookie — JS cannot read it; portal uses GET /auth/me with credentials:'include'
        resp.set_cookie(
            key="af_token", value=token,
            httponly=True, samesite="strict",
            max_age=SESSION_MAX_AGE, path="/",
        )
        return resp

    return JSONResponse({"error": "invalid credentials"}, status_code=401)


@app.get("/auth/me")
async def auth_me(request: Request):
    """Return current user identity from af_token cookie or Authorization: Bearer header."""
    token_str = _token_from_request(request)
    if not token_str:
        return JSONResponse({"error": "unauthenticated"}, status_code=401)
    payload = _decode_token(token_str)
    if not payload:
        return JSONResponse({"error": "invalid or expired token"}, status_code=401)
    exp = payload.get("exp", int(time.time()) + SESSION_MAX_AGE)
    return JSONResponse({
        "sub":   payload.get("sub", ""),
        "email": payload.get("email", ""),
        "name":  payload.get("name", payload.get("sub", "")),
        "role":  payload.get("role", "admin"),
        "exp":   exp,
    })


@app.post("/auth/refresh")
async def auth_refresh(request: Request):
    """Rotate the af_token cookie; returns new exp for the portal refresh scheduler."""
    token_str = _token_from_request(request)
    if not token_str:
        return JSONResponse({"error": "missing token"}, status_code=401)
    payload = _decode_token(token_str)
    if not payload:
        return JSONResponse({"error": "invalid or expired token"}, status_code=401)

    new_token = _make_jwt(sub=payload.get("sub", ""), email=payload.get("email", ""))
    new_exp = int(time.time()) + SESSION_MAX_AGE
    resp = JSONResponse({"exp": new_exp, "expires_in": SESSION_MAX_AGE})
    resp.set_cookie(
        key="af_token", value=new_token,
        httponly=True, samesite="strict",
        max_age=SESSION_MAX_AGE, path="/",
    )
    return resp


@app.get("/auth/logout")
async def auth_logout():
    resp = JSONResponse({"ok": True})
    resp.delete_cookie("af_token", path="/", samesite="strict")
    return resp


# ─── Users ────────────────────────────────────────────────────────────────────
MOCK_USERS = [
    {"user_id": "u-001", "tenant_id": "tenant-default", "username": "admin",
     "email": "admin@govagn.local", "display_name": "Admin",
     "role": "admin", "is_active": True, "last_login_at": "2026-03-19T08:00:00Z",
     "created_at": "2026-01-01T00:00:00Z"},
    {"user_id": "u-002", "tenant_id": "tenant-default", "username": "alice",
     "email": "alice@govagn.local", "display_name": "Alice Chen",
     "role": "editor", "is_active": True, "last_login_at": "2026-03-18T14:32:00Z",
     "created_at": "2026-01-15T09:00:00Z"},
    {"user_id": "u-003", "tenant_id": "tenant-default", "username": "bob",
     "email": "bob@govagn.local", "display_name": "Bob Smith",
     "role": "viewer", "is_active": True, "last_login_at": "2026-03-17T11:10:00Z",
     "created_at": "2026-02-01T00:00:00Z"},
    {"user_id": "u-004", "tenant_id": "tenant-default", "username": "ci-bot",
     "email": "ci@govagn.local", "display_name": "CI Bot",
     "role": "viewer", "is_active": True, "last_login_at": None,
     "created_at": "2026-03-01T00:00:00Z"},
]

@app.get("/api/v1/users")
def list_users():
    return MOCK_USERS

@app.get("/api/v1/users/{user_id}")
def get_user(user_id: str):
    u = next((u for u in MOCK_USERS if u["user_id"] == user_id), None)
    if not u:
        return JSONResponse({"error": "not found"}, status_code=404)
    return u

@app.post("/api/v1/users")
async def create_user(request: Request):
    body = await request.json()
    new_user = {
        "user_id": f"u-{str(uuid.uuid4())[:8]}",
        "tenant_id": "tenant-default",
        "username": body.get("username", ""),
        "email": body.get("email", ""),
        "display_name": body.get("display_name", ""),
        "role": body.get("role", "viewer"),
        "is_active": True,
        "last_login_at": None,
        "created_at": datetime.now(timezone.utc).isoformat(),
    }
    MOCK_USERS.append(new_user)
    return JSONResponse(new_user, status_code=201)

@app.delete("/api/v1/users/{user_id}")
def delete_user(user_id: str):
    global MOCK_USERS
    MOCK_USERS = [u for u in MOCK_USERS if u["user_id"] != user_id]
    return JSONResponse({"ok": True})

@app.patch("/api/v1/users/{user_id}")
async def update_user(user_id: str, request: Request):
    body = await request.json()
    for u in MOCK_USERS:
        if u["user_id"] == user_id:
            u.update({k: v for k, v in body.items() if k in u})
            return u
    return JSONResponse({"error": "not found"}, status_code=404)


# ─── Audit log ────────────────────────────────────────────────────────────────
def _make_hash(prev: str, fields: str) -> str:
    return hashlib.sha256(f"{prev}{fields}".encode()).hexdigest()

_AUDIT_SEED = [
    ("pii-block",   "deny",     "PII detected: email address in prompt"),
    ("rate-limit",  "warn",     "Request rate near limit for tenant"),
    ("pii-block",   "sanitize", "Phone number redacted from output"),
    ("allow-all",   "allow",    "Policy passed — no violations"),
    ("pii-block",   "deny",     "SSN pattern matched in user input"),
    ("cost-guard",  "warn",     "Token budget 80% consumed for this run"),
    ("allow-all",   "allow",    "Policy passed — no violations"),
    ("pii-block",   "deny",     "Credit card number detected in output"),
    ("allow-all",   "allow",    "Policy passed — no violations"),
    ("rate-limit",  "deny",     "Rate limit exceeded — request blocked"),
]

def _build_audit_entries():
    entries = []
    prev_hash = "0" * 64
    base_ts = 1742378400  # 2026-03-19T10:00:00Z
    for i, (policy, result, reason) in enumerate(_AUDIT_SEED):
        fields = f"{i+1}{policy}{result}{reason}"
        entry_hash = _make_hash(prev_hash, fields)
        entries.append({
            "id":            i + 1,
            "decision_id":   f"dec-{i+1:04d}",
            "trace_id":      f"trace-{'abcdef0123456789'[i % 16] * 16}",
            "span_id":       f"span-{'fedcba9876543210'[i % 16] * 8}",
            "policy_name":   policy,
            "result":        result,
            "reason":        reason,
            "tenant_id":     "tenant-default",
            "evaluated_at":  datetime.fromtimestamp(base_ts + i * 37, tz=timezone.utc).isoformat(),
            "previous_hash": prev_hash,
            "entry_hash":    entry_hash,
        })
        prev_hash = entry_hash
    return entries

AUDIT_ENTRIES = _build_audit_entries()

@app.get("/api/v1/audit")
def list_audit(limit: int = 100, offset: int = 0):
    return AUDIT_ENTRIES[offset:offset + limit]

@app.get("/api/v1/audit/verify")
def verify_audit():
    ok = True
    prev = "0" * 64
    for e in AUDIT_ENTRIES:
        fields = f"{e['id']}{e['policy_name']}{e['result']}{e['reason']}"
        expected = _make_hash(prev, fields)
        if expected != e["entry_hash"]:
            ok = False
            break
        prev = e["entry_hash"]
    return {"valid": ok, "entries_checked": len(AUDIT_ENTRIES)}


# ─── Collectors ───────────────────────────────────────────────────────────────
@app.get("/api/v1/collectors")
def list_collectors():
    return [
        {"id": "col-001", "name": "local-collector",
         "endpoint_grpc": "localhost:4317", "endpoint_http": "http://localhost:4318",
         "status": "healthy", "version": "1.2.0",
         "last_checked": datetime.now(timezone.utc).isoformat()},
    ]


# ─── Integration test run seed data ──────────────────────────────────────────
def _seed_spans():
    """Populate in-memory spans with realistic framework + integration test data."""
    now_ns = int(time.time() * 1e9)
    minute = 60 * 1_000_000_000

    runs = [
        # (framework, agent_name, nodes, status_code, cost, tokens, minutes_ago)
        ("langgraph",   "research-graph",   ["retriever", "summarizer", "writer"],         0, 0.0042, 1820, 2),
        ("crewai",      "analysis-crew",    ["researcher", "analyst", "reporter"],          0, 0.0071, 3200, 5),
        ("langgraph",   "qa-graph",         ["loader", "splitter", "embedder", "ranker"],  0, 0.0018,  740, 8),
        ("google_adk",  "assistant-agent",  ["planner", "executor", "reviewer"],           0, 0.0055, 2400, 12),
        ("openai",      "support-agent",    ["classify", "respond"],                       2, 0.0012,  510, 15),
        ("langgraph",   "pipeline-graph",   ["ingest", "transform", "validate", "emit"],   0, 0.0093, 4100, 20),
        ("crewai",      "content-crew",     ["writer", "editor"],                          0, 0.0031, 1350, 28),
        ("claude",      "claude-agent",     ["think", "act", "observe"],                   0, 0.0066, 2900, 35),
        # integration test runs (emitted by the test harness)
        ("langgraph",   "int-test:node-tracing",  ["node-a", "node-b", "node-c"],           0, 0.0,    0, 40),
        ("google_adk",  "int-test:adk-delegation",["root-agent", "sub-agent-1", "sub-agent-2"], 0, 0.0, 0, 42),
        ("crewai",      "int-test:crew-basic",    ["worker-1", "worker-2"],                 0, 0.0,    0, 44),
    ]

    for fw, agent, nodes, status, cost_total, tokens_total, mins_ago in runs:
        trace_id  = str(uuid.uuid4())
        run_id    = str(uuid.uuid4())
        cost_each = round(cost_total / max(len(nodes), 1), 6)
        tok_each  = tokens_total // max(len(nodes), 1)

        parent_id = ""
        for i, node in enumerate(nodes):
            span_id   = str(uuid.uuid4())[:16]
            start_ns  = now_ns - (mins_ago * minute) + i * (minute // 10)
            dur_ns    = random.randint(200_000_000, 1_200_000_000)
            s_code    = status if i == len(nodes) - 1 else 0

            spans.append({
                "id":            span_id,
                "trace_id":      trace_id,
                "parent_id":     parent_id,
                "run_id":        run_id,
                "name":          f"{fw}.node.{node}" if fw in ("langgraph", "google_adk") else f"{fw}.{node}",
                "framework":     fw,
                "start_time_ns": start_ns,
                "duration_ns":   dur_ns,
                "status_code":   s_code,
                "status_msg":    "internal error" if s_code == 2 else "",
                "attributes":    {
                    "af.agent.name":          agent,
                    "af.agent.framework":     fw,
                    "af.agent.run_id":        run_id,
                    "gen_ai.request.model":   "claude-sonnet-4-6" if fw == "claude" else ("gpt-4o" if fw == "openai" else ""),
                    "gen_ai.usage.input_tokens":  str(tok_each // 2),
                    "gen_ai.usage.output_tokens": str(tok_each // 2),
                    "af.cost.total_usd":      str(cost_each),
                },
                "events":        [],
                "input_tokens":  tok_each // 2,
                "output_tokens": tok_each // 2,
                "cost_usd":      cost_each,
                "agent_name":    agent,
                "model":         "claude-sonnet-4-6" if fw == "claude" else ("gpt-4o" if fw == "openai" else ""),
                "environment":   "integration-test" if agent.startswith("int-test:") else "development",
                "received_at":   datetime.now(timezone.utc).isoformat(),
                "start_time":    ts_str(start_ns),
            })
            parent_id = span_id  # chain spans as parent→child

    print(f"[seed] Loaded {len(spans)} spans across {len(runs)} trace runs")


# ─── Startup banner ──────────────────────────────────────────────────────────
@app.on_event("startup")
async def startup():
    _seed_spans()
    print("\n" + "="*55)
    print("  Govagn Mock Gateway  |  :8080")
    print("  Auth login        -> POST /auth/login  (admin/admin)")
    print("  Auth me           -> GET  /auth/me")
    print("  Collector ingest  -> POST /internal/ingest")
    print("  Portal API        -> GET  /api/v1/*")
    print("  Live stream       -> WS   /api/v1/stream/live")
    print("="*55 + "\n")


if __name__ == "__main__":
    uvicorn.run(
        "mock_gateway:app",
        host="0.0.0.0",
        port=8080,
        reload=False,
        log_level="info",
    )
