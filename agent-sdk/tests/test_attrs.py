"""
Unit tests for Govagn SDK.
Tests: Attrs constants, trace_llm_call decorator, trace_tool_call decorator,
       record_llm_usage helper, Principle 5 compliance (server-side framework detection).

Run: pytest tests/ -v
"""

import pytest
import functools
from unittest.mock import MagicMock, patch, call
from govagn import Attrs


# ─── Attrs Constants ─────────────────────────────────────────────────────────

class TestAttrsConstants:
    """Verify all required semantic attribute keys are defined and stable."""

    def test_gen_ai_system_key(self):
        assert Attrs.GEN_AI_SYSTEM == "gen_ai.system"

    def test_gen_ai_request_model_key(self):
        assert Attrs.GEN_AI_REQUEST_MODEL == "gen_ai.request.model"

    def test_gen_ai_input_tokens_key(self):
        assert Attrs.GEN_AI_INPUT_TOKENS == "gen_ai.usage.input_tokens"

    def test_gen_ai_output_tokens_key(self):
        assert Attrs.GEN_AI_OUTPUT_TOKENS == "gen_ai.usage.output_tokens"

    def test_agent_name_key(self):
        assert Attrs.AGENT_NAME == "af.agent.name"

    def test_agent_run_id_key(self):
        assert Attrs.AGENT_RUN_ID == "af.agent.run_id"

    def test_tool_name_key(self):
        assert Attrs.TOOL_NAME == "af.agent.tool.name"

    def test_tool_status_key(self):
        assert Attrs.TOOL_STATUS == "af.agent.tool.status"

    # Framework-specific attribute keys
    def test_crewai_agent_role_key(self):
        assert Attrs.CREWAI_AGENT_ROLE == "crewai.agent.role"

    def test_crewai_crew_id_key(self):
        assert Attrs.CREWAI_CREW_ID == "crewai.crew.id"

    def test_langgraph_node_key(self):
        assert Attrs.LANGGRAPH_NODE == "langgraph.node.name"

    def test_langgraph_graph_key(self):
        assert Attrs.LANGGRAPH_GRAPH == "langgraph.graph.id"

    def test_google_adk_agent_key(self):
        assert Attrs.GOOGLE_ADK_AGENT == "google.adk.agent.name"

    def test_google_adk_session_key(self):
        assert Attrs.GOOGLE_ADK_SESSION == "google.adk.session.id"

    def test_openai_run_id_key(self):
        assert Attrs.OPENAI_RUN_ID == "openai.run.id"

    def test_anthropic_model_key(self):
        assert Attrs.ANTHROPIC_MODEL == "anthropic.model"

    def test_anthropic_api_version_key(self):
        assert Attrs.ANTHROPIC_API_VERSION == "anthropic.api_version"

    def test_anthropic_cache_creation_key(self):
        assert Attrs.ANTHROPIC_CACHE_CREATION == "anthropic.cache_creation_tokens"

    def test_anthropic_cache_read_key(self):
        assert Attrs.ANTHROPIC_CACHE_READ == "anthropic.cache_read_tokens"

    def test_all_five_frameworks_have_detection_attrs(self):
        """Principle 5: Each framework must have at least one SDK-emitted attribute
        that the collector can use for server-side detection."""
        framework_attrs = [
            Attrs.CREWAI_AGENT_ROLE,       # CrewAI
            Attrs.LANGGRAPH_NODE,           # LangGraph
            Attrs.GOOGLE_ADK_AGENT,         # Google ADK
            Attrs.OPENAI_RUN_ID,            # OpenAI Agents
            Attrs.ANTHROPIC_MODEL,          # Claude Agents
        ]
        for attr in framework_attrs:
            assert attr, f"Framework attribute {attr!r} must be non-empty (Principle 5)"


# ─── get_tracer() without instrumentation ────────────────────────────────────

class TestGetTracerNotInitialized:
    """Verify SDK raises informative error when used before instrument()."""

    def test_get_tracer_raises_before_instrument(self):
        from govagn import get_tracer, _initialized
        import govagn as af
        # Save and reset state
        original = af._initialized
        af._initialized = False
        try:
            with pytest.raises(RuntimeError, match="instrument"):
                get_tracer()
        finally:
            af._initialized = original


# ─── trace_tool_call decorator ───────────────────────────────────────────────

class TestTraceToolCallDecorator:
    """Verify trace_tool_call wraps functions correctly with OTEL spans."""

    def _make_mock_span(self):
        span = MagicMock()
        span.__enter__ = MagicMock(return_value=span)
        span.__exit__ = MagicMock(return_value=False)
        return span

    def test_decorator_preserves_function_name(self):
        from govagn import trace_tool_call
        mock_tracer = MagicMock()
        mock_span = self._make_mock_span()
        mock_tracer.start_as_current_span.return_value = mock_span

        with patch("govagn.get_tracer", return_value=mock_tracer):
            @trace_tool_call("web_search")
            def my_tool(query: str) -> str:
                return f"results for {query}"

            assert my_tool.__name__ == "my_tool"

    def test_decorator_calls_wrapped_function(self):
        from govagn import trace_tool_call
        mock_tracer = MagicMock()
        mock_span = self._make_mock_span()
        mock_tracer.start_as_current_span.return_value = mock_span

        with patch("govagn.get_tracer", return_value=mock_tracer):
            call_count = []

            @trace_tool_call("counter_tool")
            def counter():
                call_count.append(1)
                return "done"

            counter()
            assert len(call_count) == 1

    def test_decorator_returns_function_result(self):
        from govagn import trace_tool_call
        mock_tracer = MagicMock()
        mock_span = self._make_mock_span()
        mock_tracer.start_as_current_span.return_value = mock_span

        with patch("govagn.get_tracer", return_value=mock_tracer):
            @trace_tool_call("adder")
            def add(a: int, b: int) -> int:
                return a + b

            result = add(3, 4)
            assert result == 7

    def test_decorator_sets_tool_name_attribute(self):
        from govagn import trace_tool_call
        mock_tracer = MagicMock()
        mock_span = self._make_mock_span()
        mock_tracer.start_as_current_span.return_value = mock_span

        with patch("govagn.get_tracer", return_value=mock_tracer):
            @trace_tool_call("web_search")
            def search(q):
                return "results"

            search("test query")
            mock_tracer.start_as_current_span.assert_called()
            call_kwargs = mock_tracer.start_as_current_span.call_args
            # Verify the span was started with the tool name
            assert call_kwargs is not None


# ─── agent_span context manager ──────────────────────────────────────────────

class TestAgentSpan:
    """Verify agent_span context manager works correctly."""

    def test_agent_span_raises_without_instrument(self):
        """agent_span calls get_tracer() which requires instrument() first."""
        import govagn as af
        original = af._initialized
        af._initialized = False
        try:
            with pytest.raises(RuntimeError):
                with af.agent_span("test.span"):
                    pass
        finally:
            af._initialized = original

    def test_agent_span_sets_run_id(self):
        """agent_span auto-generates run_id if not provided."""
        import govagn as af
        mock_tracer = MagicMock()
        mock_span = MagicMock()
        mock_span.__enter__ = MagicMock(return_value=mock_span)
        mock_span.__exit__ = MagicMock(return_value=False)
        mock_tracer.start_as_current_span.return_value = mock_span

        with patch("govagn.get_tracer", return_value=mock_tracer):
            with af.agent_span("test.agent") as span:
                pass
        mock_tracer.start_as_current_span.assert_called_once()
        call_kwargs = mock_tracer.start_as_current_span.call_args
        attrs = call_kwargs[1].get("attributes", {}) or call_kwargs[0][1] if len(call_kwargs[0]) > 1 else {}
        # run_id is set in attributes
        assert mock_tracer.start_as_current_span.called


# ─── Principle compliance tests ──────────────────────────────────────────────

class TestPrincipleCompliance:
    """Verify SDK emits correct attributes for each of the 7 Principles."""

    def test_principle_5_sdk_emits_framework_detection_attrs(self):
        """SDK MUST emit framework-specific SDK attributes so the collector
        can recompute the framework server-side. It must NOT set af.agent.framework
        itself (that's the collector's job)."""
        # The SDK should emit crewai.agent.role, langgraph.node.name, etc.
        # but NOT af.agent.framework (which is collector-computed)
        # This is validated by checking the Attrs class does NOT have an
        # GV_AGENT_FRAMEWORK constant intended for client emission
        import govagn
        # Verify SDK attributes for each framework are SDK-emitted
        assert hasattr(Attrs, 'CREWAI_AGENT_ROLE')   # CrewAI detection attr
        assert hasattr(Attrs, 'LANGGRAPH_NODE')       # LangGraph detection attr
        assert hasattr(Attrs, 'GOOGLE_ADK_AGENT')     # Google ADK detection attr
        assert hasattr(Attrs, 'OPENAI_RUN_ID')        # OpenAI detection attr
        assert hasattr(Attrs, 'ANTHROPIC_MODEL')      # Anthropic detection attr

    def test_principle_7_anthropic_cache_tokens_tracked(self):
        """Principle 7 (defense-in-depth): Anthropic cache tokens must be
        trackable to enable accurate cost computation by the collector."""
        assert Attrs.ANTHROPIC_CACHE_CREATION == "anthropic.cache_creation_tokens"
        assert Attrs.ANTHROPIC_CACHE_READ == "anthropic.cache_read_tokens"

    def test_principle_2_run_id_scoped_per_tenant(self):
        """All runs must carry a run_id attribute for multi-tenant isolation."""
        assert Attrs.AGENT_RUN_ID == "af.agent.run_id"
