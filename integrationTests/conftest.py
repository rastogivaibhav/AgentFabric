"""
Shared pytest configuration for Govagn integration tests.

Provides GatewaySpanExporter — a second span processor that forwards every
finished span to the running Govagn gateway (http://localhost:8080) so
traces appear in the dashboard in real-time as tests execute.

If the gateway is not reachable the exporter silently swallows the error so
tests remain green in CI/offline environments.
"""
from __future__ import annotations

import json
import os
import urllib.request

import pytest
from opentelemetry.sdk.trace.export import SpanExporter, SpanExportResult

GATEWAY_URL = os.environ.get("GV_GATEWAY_URL", "http://localhost:8080")


class GatewaySpanExporter(SpanExporter):
    """Forwards finished spans to the Govagn /internal/ingest endpoint."""

    def export(self, spans):
        payload = []
        for s in spans:
            attrs = dict(s.attributes or {})
            dur = (s.end_time - s.start_time) if s.end_time and s.start_time else 0
            payload.append({
                "span_id":        format(s.context.span_id,  "016x"),
                "trace_id":       format(s.context.trace_id, "032x"),
                "parent_span_id": format(s.parent.span_id,   "016x") if s.parent else "",
                "name":           s.name,
                "framework":      attrs.get("gen_ai.system", "unknown"),
                "start_time_ns":  s.start_time or 0,
                "duration_ns":    dur,
                "status_code":    s.status.status_code.value,
                "status_message": s.status.description or "",
                "attributes":     {k: str(v) for k, v in attrs.items()},
                "run_id":         attrs.get("af.agent.run_id", ""),
                "input_tokens":   int(attrs.get("gen_ai.usage.input_tokens",  0)),
                "output_tokens":  int(attrs.get("gen_ai.usage.output_tokens", 0)),
                "cost_usd":       float(attrs.get("af.cost.total_usd", 0.0)),
                "events":         [],
            })
        if not payload:
            return SpanExportResult.SUCCESS
        try:
            data = json.dumps({"spans": payload}).encode()
            req = urllib.request.Request(
                f"{GATEWAY_URL}/internal/ingest",
                data=data,
                headers={"Content-Type": "application/json"},
                method="POST",
            )
            urllib.request.urlopen(req, timeout=3)
        except Exception:
            pass  # gateway unreachable — tests must not fail because of this
        return SpanExportResult.SUCCESS

    def shutdown(self) -> None:
        pass


def pytest_configure(config):
    config.addinivalue_line("markers", "integration: marks tests as integration tests")


@pytest.fixture(scope="session")
def gateway_exporter() -> GatewaySpanExporter:
    """Single shared GatewaySpanExporter instance for the entire test session."""
    return GatewaySpanExporter()
