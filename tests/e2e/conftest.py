"""
conftest.py — Shared pytest fixtures for AgentFabric E2E pipeline tests.

Provides:
  - mock_collector     : in-process OTLP HTTP stub (no real network needed)
  - mock_api_gateway   : in-process REST API stub returning realistic responses
  - tenant_a_headers   : Authorization header for tenant-alpha
  - tenant_b_headers   : Authorization header for tenant-beta
  - sample_otlp_span() : factory that builds a well-formed OTLP ExportTraceServiceRequest
"""
import base64
import hashlib
import hmac
import json
import threading
import time
import uuid
from http.server import BaseHTTPRequestHandler, HTTPServer
from typing import Any, Dict, Optional

import pytest


# ─── JWT helper (matches collector dev-secret convention) ─────────────────────

def _make_jwt(tenant_id: str, secret: str = "test-jwt-secret") -> str:
    """Produce a minimal HS256 JWT with tenant_id claim — no third-party deps."""
    header = base64.urlsafe_b64encode(
        json.dumps({"alg": "HS256", "typ": "JWT"}, separators=(",", ":")).encode()
    ).rstrip(b"=").decode()
    payload = base64.urlsafe_b64encode(
        json.dumps(
            {
                "tenant_id": tenant_id,
                "sub": "test-agent",
                "exp": int(time.time()) + 86400,
            },
            separators=(",", ":"),
        ).encode()
    ).rstrip(b"=").decode()
    sig_input = f"{header}.{payload}".encode()
    sig = base64.urlsafe_b64encode(
        hmac.new(secret.encode(), sig_input, hashlib.sha256).digest()
    ).rstrip(b"=").decode()
    return f"{header}.{payload}.{sig}"


# ─── OTLP span factory ────────────────────────────────────────────────────────

def sample_otlp_span(
    trace_id: Optional[str] = None,
    span_id: Optional[str] = None,
    framework: str = "crewai",
    model: str = "gpt-4o-mini",
    input_tokens: int = 100,
    output_tokens: int = 50,
    extra_attributes: Optional[Dict[str, Any]] = None,
) -> Dict[str, Any]:
    """
    Build a minimal OTLP/HTTP ExportTraceServiceRequest dict.

    The structure follows the OTLP protobuf-JSON mapping:
    https://opentelemetry.io/docs/specs/otlp/

    trace_id and span_id are 16-byte / 8-byte hex strings respectively.
    """
    trace_id = trace_id or uuid.uuid4().hex + uuid.uuid4().hex  # 32 hex chars = 16 bytes
    span_id = span_id or uuid.uuid4().hex[:16]  # 16 hex chars = 8 bytes

    attributes = [
        {"key": "gen_ai.system", "value": {"stringValue": framework}},
        {"key": "gen_ai.request.model", "value": {"stringValue": model}},
        {"key": "gen_ai.usage.input_tokens", "value": {"intValue": str(input_tokens)}},
        {"key": "gen_ai.usage.output_tokens", "value": {"intValue": str(output_tokens)}},
    ]

    # Framework-specific detection attributes
    framework_attrs = {
        "crewai": [{"key": "crewai.agent.role", "value": {"stringValue": "researcher"}}],
        "langgraph": [{"key": "langgraph.graph.id", "value": {"stringValue": "graph-abc"}}],
        "openai_agents": [{"key": "openai.run_id", "value": {"stringValue": "run-xyz"}}],
        "anthropic": [{"key": "anthropic.model", "value": {"stringValue": model}}],
        "google_adk": [{"key": "google.adk.agent_id", "value": {"stringValue": "adk-001"}}],
    }
    if framework in framework_attrs:
        attributes.extend(framework_attrs[framework])

    if extra_attributes:
        for k, v in extra_attributes.items():
            attributes.append({"key": k, "value": {"stringValue": str(v)}})

    start_ns = int(time.time() * 1e9)
    return {
        "resourceSpans": [
            {
                "resource": {
                    "attributes": [
                        {"key": "service.name", "value": {"stringValue": "af-test-e2e"}},
                        {"key": "deployment.environment", "value": {"stringValue": "test"}},
                    ]
                },
                "scopeSpans": [
                    {
                        "scope": {"name": "agentfabric", "version": "1.0.0"},
                        "spans": [
                            {
                                "traceId": trace_id,
                                "spanId": span_id,
                                "name": f"{framework}.agent.run",
                                "kind": 1,  # SPAN_KIND_INTERNAL
                                "startTimeUnixNano": str(start_ns),
                                "endTimeUnixNano": str(start_ns + 500_000_000),
                                "attributes": attributes,
                                "status": {"code": 1},  # STATUS_CODE_OK
                            }
                        ],
                    }
                ],
            }
        ]
    }


# ─── MockCollector ────────────────────────────────────────────────────────────

class _CollectorRequestHandler(BaseHTTPRequestHandler):
    """Handles OTLP HTTP POST /v1/traces and records received payloads."""

    def log_message(self, fmt, *args):  # suppress access log noise in tests
        pass

    def do_POST(self):
        length = int(self.headers.get("Content-Length", 0))
        body = self.rfile.read(length)
        try:
            payload = json.loads(body)
        except Exception:
            payload = {}

        self.server.received_spans.append(
            {
                "path": self.path,
                "headers": dict(self.headers),
                "payload": payload,
                "received_at": time.time(),
            }
        )

        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(b"{}")

    def do_GET(self):
        if self.path == "/healthz":
            self.send_response(200)
            self.end_headers()
            self.wfile.write(b'{"status":"ok"}')
            return
        self.send_response(404)
        self.end_headers()


class _MockCollectorServer(HTTPServer):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.received_spans: list = []

    def clear(self):
        self.received_spans.clear()


@pytest.fixture(scope="session")
def mock_collector():
    """
    Spin up a real TCP server on an ephemeral port that acts as an OTLP HTTP
    collector.  Yields a helper object with:
      .url          — "http://127.0.0.1:<port>"
      .received     — list of dicts, each with keys: path, headers, payload
      .clear()      — reset received list between tests
      .wait_for(n)  — block until at least n spans have arrived (timeout=5s)
    """
    server = _MockCollectorServer(("127.0.0.1", 0), _CollectorRequestHandler)
    port = server.server_address[1]
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()

    class _Collector:
        url = f"http://127.0.0.1:{port}"
        received = server.received_spans

        @staticmethod
        def clear():
            server.clear()

        @staticmethod
        def wait_for(n: int, timeout: float = 5.0) -> bool:
            deadline = time.time() + timeout
            while time.time() < deadline:
                if len(server.received_spans) >= n:
                    return True
                time.sleep(0.05)
            return False

    yield _Collector()
    server.shutdown()


# ─── MockAPIGateway ───────────────────────────────────────────────────────────

class _GatewayRequestHandler(BaseHTTPRequestHandler):
    """Lightweight REST stub for api-gateway endpoints used in tests."""

    def log_message(self, fmt, *args):
        pass

    def _tenant_from_request(self) -> str:
        """Extract tenant_id from Authorization JWT (payload claim, no verification)."""
        auth = self.headers.get("Authorization", "")
        if auth.startswith("Bearer "):
            token = auth[len("Bearer "):]
            try:
                parts = token.split(".")
                # Add padding before decoding
                padded = parts[1] + "=" * (4 - len(parts[1]) % 4)
                claims = json.loads(base64.urlsafe_b64decode(padded))
                return claims.get("tenant_id", "default")
            except Exception:
                pass
        return "default"

    def do_GET(self):
        tenant = self._tenant_from_request()

        # Route: GET /api/v1/traces
        if self.path.startswith("/api/v1/traces") and "?" not in self.path.split("/api/v1/traces")[1][:1] or self.path == "/api/v1/traces":
            # List traces — return only traces for this tenant
            traces = [
                t for t in self.server.trace_store.values()
                if t.get("tenant_id") == tenant
            ]
            body = json.dumps({"items": traces, "has_more": False}).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            return

        # Route: GET /api/v1/traces/<traceId>
        parts = self.path.split("/")
        if len(parts) >= 5 and parts[3] == "traces" and parts[4]:
            trace_id = parts[4].split("?")[0]
            trace = self.server.trace_store.get(trace_id)
            if trace and trace.get("tenant_id") == tenant:
                body = json.dumps(trace).encode()
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.end_headers()
                self.wfile.write(body)
            else:
                self.send_response(404)
                self.send_header("Content-Type", "application/json")
                self.end_headers()
                self.wfile.write(b'{"error":"trace not found"}')
            return

        # Route: GET /api/v1/agents
        if self.path.startswith("/api/v1/agents"):
            agents = [
                a for a in self.server.agent_store.values()
                if a.get("tenant_id") == tenant
            ]
            body = json.dumps({"items": agents, "has_more": False}).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(body)
            return

        # Route: GET /api/v1/cost
        if self.path.startswith("/api/v1/cost"):
            costs = [
                c for c in self.server.cost_store
                if c.get("tenant_id") == tenant
            ]
            body = json.dumps(costs).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(body)
            return

        # Route: GET /api/v1/audit
        if self.path.startswith("/api/v1/audit"):
            entries = [
                e for e in self.server.audit_store
                if e.get("tenant_id") == tenant
            ]
            body = json.dumps({"items": entries, "count": len(entries)}).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(body)
            return

        self.send_response(404)
        self.end_headers()

    def do_POST(self):
        length = int(self.headers.get("Content-Length", 0))
        self.rfile.read(length)
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(b'{"status":"ok"}')


class _MockGatewayServer(HTTPServer):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        # In-memory stores keyed by trace_id / agent_id
        self.trace_store: dict = {}
        self.agent_store: dict = {}
        self.cost_store: list = []
        self.audit_store: list = []

    def seed_trace(self, trace: dict):
        self.trace_store[trace["id"]] = trace

    def seed_agent(self, agent: dict):
        self.agent_store[agent["id"]] = agent

    def seed_cost(self, cost: dict):
        self.cost_store.append(cost)

    def seed_audit(self, entry: dict):
        self.audit_store.append(entry)

    def clear(self):
        self.trace_store.clear()
        self.agent_store.clear()
        self.cost_store.clear()
        self.audit_store.clear()


@pytest.fixture(scope="session")
def mock_api_gateway():
    """
    Spin up an in-process HTTP server that mimics the api-gateway REST API.
    Yields a helper object with:
      .url              — "http://127.0.0.1:<port>"
      .seed_trace(dict) — add a trace visible to its tenant
      .seed_agent(dict) — add an agent visible to its tenant
      .seed_cost(dict)  — add a cost row
      .seed_audit(dict) — add an audit entry
      .clear()          — reset all stores
    """
    server = _MockGatewayServer(("127.0.0.1", 0), _GatewayRequestHandler)
    port = server.server_address[1]
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()

    class _Gateway:
        url = f"http://127.0.0.1:{port}"

        @staticmethod
        def seed_trace(trace: dict):
            server.seed_trace(trace)

        @staticmethod
        def seed_agent(agent: dict):
            server.seed_agent(agent)

        @staticmethod
        def seed_cost(cost: dict):
            server.seed_cost(cost)

        @staticmethod
        def seed_audit(entry: dict):
            server.seed_audit(entry)

        @staticmethod
        def clear():
            server.clear()

    yield _Gateway()
    server.shutdown()


# ─── Tenant JWT header fixtures ───────────────────────────────────────────────

@pytest.fixture(scope="session")
def tenant_a_headers() -> Dict[str, str]:
    """Authorization header for tenant 'tenant-alpha'."""
    return {"Authorization": f"Bearer {_make_jwt('tenant-alpha')}"}


@pytest.fixture(scope="session")
def tenant_b_headers() -> Dict[str, str]:
    """Authorization header for tenant 'tenant-beta' (different tenant from A)."""
    return {"Authorization": f"Bearer {_make_jwt('tenant-beta')}"}
