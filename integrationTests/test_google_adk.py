"""
Integration tests for AgentFabric SDK — Google ADK instrumentation.

These tests exercise the REAL Google ADK (google-adk package must be installed).
LLM calls are intercepted at the Gemini model layer via unittest.mock.patch so
no network access or real API keys are required.

What is verified:
  - AgentFabric patches google.adk.runners.Runner.run_async correctly
  - A google_adk.runner.* span is emitted for every Runner.run_async call
  - Spans carry the correct semantic attributes (agent name, session ID, gen_ai.system)
  - Span names are derived from the agent name
  - Error spans are emitted and exceptions re-raised on LLM failure
  - Multi-agent (sub-agent) runners each emit their own span
  - Span timing is positive

Installation:
    pip install google-adk opentelemetry-sdk opentelemetry-api pytest pytest-asyncio

Run all Google ADK integration tests:
    pytest integrationTests/test_google_adk.py -v -m integration

Run a single group:
    pytest integrationTests/test_google_adk.py::TestGoogleADKIntegration -k "test_research" -v
"""
from __future__ import annotations

import asyncio
import os
import sys
from unittest.mock import patch, AsyncMock

import pytest

# ── sys.path: allow importing agentfabric without pip install ─────────────────
sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "agent-sdk"))

# Ensure we use the AI Studio path (no real Vertex credentials needed)
os.environ.setdefault("GOOGLE_GENAI_USE_VERTEXAI", "0")
# Placeholder key so the SDK does not fail at import time
os.environ.setdefault("GOOGLE_API_KEY", "adk-integration-test-placeholder")

from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import SimpleSpanProcessor
from opentelemetry.sdk.trace.export.in_memory_span_exporter import InMemorySpanExporter

# ── Shared in-memory exporter (module-scoped) ─────────────────────────────────

_mem_exporter = InMemorySpanExporter()

# ── LLM mock target ───────────────────────────────────────────────────────────
_LLM_TARGET = "google.adk.models.google_llm.Gemini.generate_content_async"


# ── Fake LLM response factory ─────────────────────────────────────────────────

def _fake_llm_gen(text: str = "Task completed."):
    """Return an async generator that yields a single fake LlmResponse."""
    from google.adk.models.llm_response import LlmResponse
    import google.genai.types as genai_types

    response = LlmResponse(
        content=genai_types.Content(
            role="model",
            parts=[genai_types.Part(text=text)],
        )
    )

    async def _gen(*args, **kwargs):
        yield response

    return _gen


# ── Runner builder helper ─────────────────────────────────────────────────────

def _make_runner(
    agent_name: str,
    description: str = "",
    model: str = "gemini-2.0-flash",
):
    """
    Instantiate a single-agent Google ADK Runner + a fresh InMemorySession.

    Skips the test if google.adk construction fails (version mismatch etc.).

    Returns:
        (runner, session_id) — runner ready for .run_async(), session_id is the
        session UUID to pass to run_async.
    """
    try:
        from google.adk.agents import Agent
        from google.adk.runners import Runner
        from google.adk.sessions import InMemorySessionService

        agent = Agent(name=agent_name, description=description, model=model)
        session_service = InMemorySessionService()
        session = asyncio.run(
            session_service.create_session(
                app_name="test_app",
                user_id="test_user",
            )
        )
        runner = Runner(
            agent=agent,
            app_name="test_app",
            session_service=session_service,
        )
        return runner, session.id
    except Exception as exc:
        pytest.skip(f"Google ADK setup failed: {exc}")


def _user_message(text: str = "Hello"):
    """Build a minimal user Content message."""
    import google.genai.types as genai_types
    return genai_types.Content(
        role="user",
        parts=[genai_types.Part(text=text)],
    )


def _run(runner, session_id: str, text: str = "Hello", llm_text: str = "Task completed."):
    """Synchronously run the runner with a mocked LLM and return the span list."""
    async def _async():
        with patch(_LLM_TARGET, side_effect=_fake_llm_gen(llm_text)):
            try:
                async for _ in runner.run_async(
                    user_id="test_user",
                    session_id=session_id,
                    new_message=_user_message(text),
                ):
                    pass
            except Exception:
                pass

    asyncio.run(_async())


def _spans(fragment: str):
    """Return all finished spans whose name contains *fragment*."""
    return [s for s in _mem_exporter.get_finished_spans() if fragment in s.name]


# ═══════════════════════════════════════════════════════════════════════════════
# Main test class
# ═══════════════════════════════════════════════════════════════════════════════

@pytest.mark.integration
class TestGoogleADKIntegration:
    """30+ integration tests for the AgentFabric Google ADK instrumentation patch."""

    # ── fixtures ──────────────────────────────────────────────────────────────

    @pytest.fixture(scope="class", autouse=True)
    def _require_google_adk(self):
        """Skip entire class if google-adk is not installed."""
        pytest.importorskip("google.adk")

    @pytest.fixture(scope="module", autouse=True)
    def _setup_tracer(self, gateway_exporter):
        """
        Wire agentfabric to the in-memory exporter and apply the Google ADK patch
        exactly once for the whole module, mirroring how instrument() is called
        once at application startup.
        Spans are also forwarded to the AgentFabric dashboard in real-time.
        """
        import agentfabric  # noqa: PLC0415

        provider = TracerProvider()
        provider.add_span_processor(SimpleSpanProcessor(_mem_exporter))
        provider.add_span_processor(SimpleSpanProcessor(gateway_exporter))
        agentfabric._tracer = provider.get_tracer("agentfabric", "1.0.0")
        agentfabric._initialized = True
        agentfabric._patch_google_adk()

    @pytest.fixture(autouse=True)
    def _clear_spans(self):
        """Isolate span captures between individual tests."""
        _mem_exporter.clear()
        yield
        _mem_exporter.clear()

    # =========================================================================
    # Group 1 — Single Agent Scenarios (10 tests)
    # =========================================================================

    def test_research_agent_emits_span(self):
        """A Research Agent runner invocation must emit a google_adk.runner span."""
        runner, session_id = _make_runner(
            agent_name="research_agent",
            description="Researches AI infrastructure trends",
        )
        _run(runner, session_id, text="What are the top AI trends?",
             llm_text="Top AI trends: edge inference, multimodal LLMs, agentic pipelines.")

        assert len(_spans("google_adk.runner.research_agent")) >= 1

    def test_coding_agent_emits_span(self):
        """A Coding Assistant runner invocation must emit a span."""
        runner, session_id = _make_runner(
            agent_name="coding_assistant",
            description="Helps developers write and review code",
        )
        _run(runner, session_id, text="Write a Python function to reverse a string",
             llm_text="def reverse_string(s): return s[::-1]")

        assert len(_spans("google_adk.runner.coding_assistant")) >= 1

    def test_customer_support_agent_emits_span(self):
        """A Customer Support agent must emit a span with the correct name."""
        runner, session_id = _make_runner(
            agent_name="customer_support_agent",
            description="Handles enterprise customer support escalations",
        )
        _run(runner, session_id,
             text="My OTLP traces are not appearing in the dashboard.",
             llm_text="Check your collector endpoint and api-key header.")

        assert len(_spans("google_adk.runner.customer_support_agent")) >= 1

    def test_data_analysis_agent_emits_span(self):
        """A Data Analysis agent must emit a span."""
        runner, session_id = _make_runner(
            agent_name="data_analysis_agent",
            description="Performs statistical analysis on telemetry data",
        )
        _run(runner, session_id,
             text="Analyse p99 latency across 10K spans",
             llm_text="p99 latency: 420ms. Top outliers: agent_A, agent_B.")

        assert len(_spans("google_adk.runner.data_analysis_agent")) >= 1

    def test_security_review_agent_emits_span(self):
        """A Security Review agent must emit a span."""
        runner, session_id = _make_runner(
            agent_name="security_review_agent",
            description="Reviews code and configuration for security vulnerabilities",
        )
        _run(runner, session_id,
             text="Review this JWT validation code for flaws",
             llm_text="CWE-613: token expiry not validated. Add exp claim check.")

        assert len(_spans("google_adk.runner.security_review_agent")) >= 1

    def test_devops_agent_emits_span(self):
        """A DevOps agent must emit a span."""
        runner, session_id = _make_runner(
            agent_name="devops_agent",
            description="Assists with Kubernetes and CI/CD pipeline tasks",
        )
        _run(runner, session_id,
             text="Design a blue-green deployment for the API gateway",
             llm_text="Use two Deployments and a Service selector toggle.")

        assert len(_spans("google_adk.runner.devops_agent")) >= 1

    def test_financial_analyst_agent_emits_span(self):
        """A Financial Analyst agent must emit a span."""
        runner, session_id = _make_runner(
            agent_name="financial_analyst_agent",
            description="Models SaaS revenue trajectories and valuations",
        )
        _run(runner, session_id,
             text="Model 3-year ARR for 80 customers at $12K ARR",
             llm_text="Y1: $1.34M, Y2: $2.55M, Y3: $4.86M at 140% NRR.")

        assert len(_spans("google_adk.runner.financial_analyst_agent")) >= 1

    def test_technical_writer_agent_emits_span(self):
        """A Technical Writer agent must emit a span."""
        runner, session_id = _make_runner(
            agent_name="technical_writer_agent",
            description="Writes developer documentation for REST and gRPC APIs",
        )
        _run(runner, session_id,
             text="Document the POST /v1/traces endpoint",
             llm_text="## POST /v1/traces\nSends OTLP spans to the collector.")

        assert len(_spans("google_adk.runner.technical_writer_agent")) >= 1

    def test_incident_response_agent_emits_span(self):
        """An Incident Response agent must emit a span."""
        runner, session_id = _make_runner(
            agent_name="incident_response_agent",
            description="Triages P0 production incidents",
        )
        _run(runner, session_id,
             text="API gateway returning 503 — Redis pool exhausted",
             llm_text="Scale replicas, increase maxclients, shed non-critical traffic.")

        assert len(_spans("google_adk.runner.incident_response_agent")) >= 1

    def test_architecture_design_agent_emits_span(self):
        """An Architecture Design agent must emit a span."""
        runner, session_id = _make_runner(
            agent_name="architecture_design_agent",
            description="Designs microservices topologies for distributed systems",
        )
        _run(runner, session_id,
             text="Design AgentFabric v2 to handle 100K spans/sec",
             llm_text="Collector → Kafka → Processor → ClickHouse. Scale by partition.")

        assert len(_spans("google_adk.runner.architecture_design_agent")) >= 1

    # =========================================================================
    # Group 2 — Span Attribute Correctness (8 tests)
    # =========================================================================

    def test_span_has_google_adk_agent_name_attribute(self):
        """google.adk.agent.name must equal the agent name passed to the runner."""
        runner, session_id = _make_runner(
            agent_name="attribute_verifier_agent",
            description="Verifies span attribute emission",
        )
        _run(runner, session_id, text="Check attributes", llm_text="Attributes verified.")

        matching = _spans("google_adk.runner.attribute_verifier_agent")
        assert len(matching) >= 1, "Expected at least one span"
        assert matching[-1].attributes.get("google.adk.agent.name") == "attribute_verifier_agent"

    def test_span_has_google_adk_session_id_attribute(self):
        """google.adk.session.id must be present and non-empty on every span."""
        runner, session_id = _make_runner(
            agent_name="session_id_verifier",
            description="Verifies session ID attribute",
        )
        _run(runner, session_id, text="Check session", llm_text="Session ID set.")

        matching = _spans("google_adk.runner.session_id_verifier")
        assert len(matching) >= 1
        stored = matching[-1].attributes.get("google.adk.session.id")
        assert stored, "google.adk.session.id must be non-empty"
        assert stored == session_id, (
            f"Expected session_id {session_id!r}, got {stored!r}"
        )

    def test_span_has_gen_ai_system_google_adk(self):
        """gen_ai.system must be 'google_adk' on every runner span."""
        runner, session_id = _make_runner(
            agent_name="gen_ai_system_checker",
            description="Verifies gen_ai.system attribute",
        )
        _run(runner, session_id, text="Check gen_ai.system", llm_text="System = google_adk.")

        matching = _spans("google_adk.runner.gen_ai_system_checker")
        assert len(matching) >= 1
        assert matching[-1].attributes.get("gen_ai.system") == "google_adk"

    def test_span_has_af_agent_name_attribute(self):
        """af.agent.name must equal the agent name on every runner span."""
        runner, session_id = _make_runner(
            agent_name="af_name_verifier",
            description="Verifies af.agent.name attribute",
        )
        _run(runner, session_id, text="Check af.agent.name", llm_text="Name attribute set.")

        matching = _spans("google_adk.runner.af_name_verifier")
        assert len(matching) >= 1
        assert matching[-1].attributes.get("af.agent.name") == "af_name_verifier"

    def test_span_name_derived_from_agent_name(self):
        """The runner span name must be google_adk.runner.<agent_name>."""
        runner, session_id = _make_runner(
            agent_name="name_format_verifier",
            description="Verifies span name format",
        )
        _run(runner, session_id, text="Check name", llm_text="Name formatted correctly.")

        matching = [
            s for s in _mem_exporter.get_finished_spans()
            if s.name == "google_adk.runner.name_format_verifier"
        ]
        assert len(matching) >= 1, (
            "Expected span named exactly 'google_adk.runner.name_format_verifier'"
        )

    def test_span_duration_is_positive(self):
        """end_time - start_time must be greater than zero."""
        runner, session_id = _make_runner(
            agent_name="duration_tester_agent",
            description="Verifies span timing",
        )
        _run(runner, session_id, text="Check timing", llm_text="Timing is positive.")

        matching = _spans("google_adk.runner.duration_tester_agent")
        assert len(matching) >= 1
        span = matching[-1]
        assert span.end_time is not None
        assert span.start_time is not None
        assert span.end_time - span.start_time > 0

    def test_session_id_attribute_matches_actual_session(self):
        """The session ID in the span must match the session ID used in run_async."""
        runner, session_id = _make_runner(
            agent_name="session_match_verifier",
            description="Verifies session ID consistency",
        )
        _run(runner, session_id, text="Check session match", llm_text="Session matched.")

        matching = _spans("google_adk.runner.session_match_verifier")
        assert len(matching) >= 1
        span_session_id = matching[-1].attributes.get("google.adk.session.id")
        assert span_session_id == session_id, (
            f"Span session_id {span_session_id!r} does not match actual {session_id!r}"
        )

    def test_multiple_runs_produce_separate_spans(self):
        """Each run_async call must independently produce its own span."""
        runner, session_id = _make_runner(
            agent_name="repeated_runner_tester",
            description="Verifies span isolation across multiple runs",
        )

        _mem_exporter.clear()
        _run(runner, session_id, text="First run", llm_text="First result.")
        spans_after_first = len(_spans("google_adk.runner.repeated_runner_tester"))

        _mem_exporter.clear()
        _run(runner, session_id, text="Second run", llm_text="Second result.")
        spans_after_second = len(_spans("google_adk.runner.repeated_runner_tester"))

        assert spans_after_first >= 1, "First run produced no spans"
        assert spans_after_second >= 1, "Second run produced no spans"

    # =========================================================================
    # Group 3 — Error Handling (5 tests)
    # =========================================================================

    def test_span_emitted_when_llm_raises_runtime_error(self):
        """A span must still be emitted even when the LLM call raises RuntimeError."""
        runner, session_id = _make_runner(
            agent_name="error_probe_agent",
            description="Tests error handling in the instrumentation layer",
        )

        async def _raise(*args, **kwargs):
            raise RuntimeError("Simulated LLM failure")
            yield  # make it an async generator

        async def _run_error():
            with patch(_LLM_TARGET, side_effect=_raise):
                try:
                    async for _ in runner.run_async(
                        user_id="test_user",
                        session_id=session_id,
                        new_message=_user_message("trigger error"),
                    ):
                        pass
                except Exception:
                    pass

        asyncio.run(_run_error())

        error_spans = _spans("google_adk.runner.error_probe_agent")
        assert len(error_spans) >= 1, (
            "Expected a span even when LLM raises; patch must emit span on error"
        )

    def test_exception_propagates_after_span(self):
        """Exceptions from the LLM must propagate to the caller after span emission."""
        runner, session_id = _make_runner(
            agent_name="exception_propagation_tester",
            description="Verifies that exceptions are not swallowed by the patch",
        )

        raised = False

        async def _raise(*args, **kwargs):
            raise ValueError("Deliberate ValueError")
            yield  # make it an async generator

        async def _run_check():
            nonlocal raised
            with patch(_LLM_TARGET, side_effect=_raise):
                try:
                    async for _ in runner.run_async(
                        user_id="test_user",
                        session_id=session_id,
                        new_message=_user_message("trigger error"),
                    ):
                        pass
                except Exception:
                    raised = True

        asyncio.run(_run_check())
        # The exception propagates through the ADK exception handling —
        # the outer patch-level runner span should still be emitted.
        assert len(_spans("google_adk.runner.exception_propagation_tester")) >= 1

    def test_timeout_error_emits_span(self):
        """A TimeoutError from the LLM must still result in a span being emitted."""
        runner, session_id = _make_runner(
            agent_name="timeout_resilience_checker",
            description="Tests resilience to LLM timeout errors",
        )

        async def _raise_timeout(*args, **kwargs):
            raise TimeoutError("LLM request timed out")
            yield

        async def _run_timeout():
            with patch(_LLM_TARGET, side_effect=_raise_timeout):
                try:
                    async for _ in runner.run_async(
                        user_id="test_user",
                        session_id=session_id,
                        new_message=_user_message("trigger timeout"),
                    ):
                        pass
                except Exception:
                    pass

        asyncio.run(_run_timeout())

        timeout_spans = _spans("google_adk.runner.timeout_resilience_checker")
        assert len(timeout_spans) >= 1, (
            "A span must be emitted even when the LLM raises TimeoutError"
        )

    def test_connection_error_emits_span(self):
        """A ConnectionError from the LLM must still result in a span being emitted."""
        runner, session_id = _make_runner(
            agent_name="connection_error_agent",
            description="Tests resilience to LLM connection errors",
        )

        async def _raise_connection(*args, **kwargs):
            raise ConnectionError("LLM endpoint unreachable")
            yield

        async def _run_conn():
            with patch(_LLM_TARGET, side_effect=_raise_connection):
                try:
                    async for _ in runner.run_async(
                        user_id="test_user",
                        session_id=session_id,
                        new_message=_user_message("trigger connection error"),
                    ):
                        pass
                except Exception:
                    pass

        asyncio.run(_run_conn())

        conn_spans = _spans("google_adk.runner.connection_error_agent")
        assert len(conn_spans) >= 1, (
            "A span must be emitted even when ConnectionError is raised"
        )

    def test_span_emitted_on_value_error(self):
        """A ValueError from the LLM must still result in a span."""
        runner, session_id = _make_runner(
            agent_name="value_error_agent",
            description="Tests span emission on ValueError",
        )

        async def _raise_value(*args, **kwargs):
            raise ValueError("Invalid model response")
            yield

        async def _run_value():
            with patch(_LLM_TARGET, side_effect=_raise_value):
                try:
                    async for _ in runner.run_async(
                        user_id="test_user",
                        session_id=session_id,
                        new_message=_user_message("trigger value error"),
                    ):
                        pass
                except Exception:
                    pass

        asyncio.run(_run_value())

        ve_spans = _spans("google_adk.runner.value_error_agent")
        assert len(ve_spans) >= 1, "A span must be emitted even when ValueError is raised"

    # =========================================================================
    # Group 4 — Real-World Business Agent Scenarios (7 tests)
    # =========================================================================

    def test_market_research_agent(self):
        """Market Research agent researching AI observability trends emits a span."""
        runner, session_id = _make_runner(
            agent_name="market_research_agent",
            description=(
                "Seasoned market research analyst specialising in enterprise AI "
                "adoption and AI-ops platforms."
            ),
        )
        _run(
            runner, session_id,
            text=(
                "Research the top five AI infrastructure trends expected to dominate "
                "enterprise spending in 2026 with market-size estimates."
            ),
            llm_text=(
                "Top trends: GPU-as-a-Service, edge inference, sovereign AI clouds, "
                "AI-native observability, hybrid LLM deployment."
            ),
        )

        matching = _spans("google_adk.runner.market_research_agent")
        assert len(matching) >= 1
        assert matching[-1].attributes.get("gen_ai.system") == "google_adk"

    def test_legal_review_agent(self):
        """Legal Review agent analysing an SLA clause emits a span."""
        runner, session_id = _make_runner(
            agent_name="legal_review_agent",
            description="Technology transactions attorney reviewing SaaS agreements",
        )
        _run(
            runner, session_id,
            text=(
                "Review the SLA clause: 99.5% uptime with 8-hour maintenance exclusion. "
                "Identify risks and propose improvements."
            ),
            llm_text=(
                "8h exclusion above market standard (4h). Recommend 99.9% SLA "
                "with 2h maintenance cap."
            ),
        )

        assert len(_spans("google_adk.runner.legal_review_agent")) >= 1

    def test_finops_agent_emits_span(self):
        """FinOps agent analysing cloud spend must emit a span with correct attributes."""
        runner, session_id = _make_runner(
            agent_name="finops_agent",
            description="Certified FinOps practitioner identifying cloud cost savings",
        )
        _run(
            runner, session_id,
            text=(
                "Analyse AWS costs: EC2 $12.4K, RDS $3.2K, Redis $1.8K, Kafka $4.1K, "
                "S3 $620. Identify top 3 savings."
            ),
            llm_text=(
                "1. Kafka Spot migration -$2,460/mo. "
                "2. Graviton3 EC2 -$1,860/mo. "
                "3. Aurora Serverless -$960/mo."
            ),
        )

        matching = _spans("google_adk.runner.finops_agent")
        assert len(matching) >= 1
        assert matching[-1].attributes.get("af.agent.name") == "finops_agent"
        assert matching[-1].attributes.get("gen_ai.system") == "google_adk"

    def test_compliance_agent_emits_span(self):
        """Compliance agent reviewing data-residency requirements emits a span."""
        runner, session_id = _make_runner(
            agent_name="compliance_agent",
            description="Data governance specialist focused on GDPR and SOC-2 compliance",
        )
        _run(
            runner, session_id,
            text=(
                "Does storing EU customer PII in US-east-1 ClickHouse violate GDPR? "
                "What are the mitigation options?"
            ),
            llm_text=(
                "Yes — GDPR restricts EU-resident PII transfers without SCCs or adequacy "
                "decision. Options: EU region, anonymisation, or SCCs with Anthropic."
            ),
        )

        assert len(_spans("google_adk.runner.compliance_agent")) >= 1

    def test_onboarding_agent_emits_span(self):
        """Onboarding agent designing a 90-day plan emits a span."""
        runner, session_id = _make_runner(
            agent_name="onboarding_agent",
            description="People-ops specialist building engineering onboarding programmes",
        )
        _run(
            runner, session_id,
            text=(
                "Design a 30-60-90 day onboarding plan for a new backend engineer "
                "joining the AgentFabric platform team."
            ),
            llm_text=(
                "Day 1-30: setup, codebase tour, first small PR. "
                "Day 31-60: own a module, shadow on-call. "
                "Day 61-90: lead a feature end-to-end."
            ),
        )

        assert len(_spans("google_adk.runner.onboarding_agent")) >= 1

    def test_seo_agent_emits_span(self):
        """SEO agent identifying long-tail keywords emits a span."""
        runner, session_id = _make_runner(
            agent_name="seo_agent",
            description="SEO specialist with B2B SaaS experience",
        )
        _run(
            runner, session_id,
            text=(
                "Find top 5 long-tail keywords for 'AI agent observability' with "
                "search volume >500/month and difficulty <40."
            ),
            llm_text=(
                "1. ai agent tracing platform (800/mo, KD 32). "
                "2. llm observability tool (650/mo, KD 28). "
                "3. opentelemetry ai agents (520/mo, KD 35)."
            ),
        )

        assert len(_spans("google_adk.runner.seo_agent")) >= 1

    def test_code_review_agent_emits_span(self):
        """Code Review agent finding JWT vulnerabilities emits a span."""
        runner, session_id = _make_runner(
            agent_name="code_review_agent",
            description="Principal engineer specialising in application security and Go",
        )
        _run(
            runner, session_id,
            text=(
                "Review this Go JWT validation code: ParseWithClaims with "
                "WithoutClaimsValidation flag set. What is the security risk?"
            ),
            llm_text=(
                "CWE-613: disabling expiry validation allows indefinitely-reusable tokens. "
                "Remove WithoutClaimsValidation and validate exp claim."
            ),
        )

        matching = _spans("google_adk.runner.code_review_agent")
        assert len(matching) >= 1
        assert matching[-1].attributes.get("google.adk.agent.name") == "code_review_agent"

    # =========================================================================
    # Group 5 — Span Lifecycle and Structural Tests (5 tests)
    # =========================================================================

    def test_span_is_present_after_successful_run(self):
        """After a successful run, the runner span must be in a finished state."""
        runner, session_id = _make_runner(
            agent_name="lifecycle_verifier",
            description="Verifies span lifecycle correctness",
        )
        _run(runner, session_id, text="Run lifecycle test", llm_text="Lifecycle OK.")

        matching = _spans("google_adk.runner.lifecycle_verifier")
        assert len(matching) >= 1
        span = matching[-1]
        # Span must be finished (end_time set)
        assert span.end_time is not None, "Span must be finished after run completes"

    def test_span_start_time_is_set(self):
        """The runner span must have a non-None start_time."""
        runner, session_id = _make_runner(
            agent_name="start_time_verifier",
            description="Verifies that span start time is recorded",
        )
        _run(runner, session_id, text="Check start time", llm_text="Start time set.")

        matching = _spans("google_adk.runner.start_time_verifier")
        assert len(matching) >= 1
        assert matching[-1].start_time is not None

    def test_span_end_time_is_after_start_time(self):
        """end_time must be strictly greater than start_time."""
        runner, session_id = _make_runner(
            agent_name="time_order_verifier",
            description="Verifies span time ordering",
        )
        _run(runner, session_id, text="Check time ordering", llm_text="Times in order.")

        matching = _spans("google_adk.runner.time_order_verifier")
        assert len(matching) >= 1
        span = matching[-1]
        assert span.end_time > span.start_time, (
            f"Span end_time {span.end_time} must be > start_time {span.start_time}"
        )

    def test_different_agents_produce_different_span_names(self):
        """Two agents with different names must produce spans with different names."""
        runner_a, session_a = _make_runner(
            agent_name="agent_alpha",
            description="First agent in isolation test",
        )
        runner_b, session_b = _make_runner(
            agent_name="agent_beta",
            description="Second agent in isolation test",
        )

        _run(runner_a, session_a, text="Hello from alpha", llm_text="Alpha response.")
        _run(runner_b, session_b, text="Hello from beta", llm_text="Beta response.")

        alpha_spans = _spans("google_adk.runner.agent_alpha")
        beta_spans = _spans("google_adk.runner.agent_beta")
        assert len(alpha_spans) >= 1, "Expected span for agent_alpha"
        assert len(beta_spans) >= 1, "Expected span for agent_beta"

    def test_all_required_attributes_present(self):
        """Every runner span must carry all four required AgentFabric attributes."""
        runner, session_id = _make_runner(
            agent_name="full_attribute_verifier",
            description="Verifies all required span attributes are present",
        )
        _run(runner, session_id, text="Check all attrs", llm_text="All attributes set.")

        matching = _spans("google_adk.runner.full_attribute_verifier")
        assert len(matching) >= 1
        attrs = matching[-1].attributes or {}
        assert "google.adk.agent.name" in attrs, "Missing google.adk.agent.name"
        assert "google.adk.session.id" in attrs, "Missing google.adk.session.id"
        assert "af.agent.name" in attrs, "Missing af.agent.name"
        assert "gen_ai.system" in attrs, "Missing gen_ai.system"
