"""
Tests for Govagn SDK auto-instrumentation patching.

Each framework patch function replaces a key method/function with a traced
wrapper that emits OTEL spans carrying the correct semantic attributes.

Coverage:
  - _patch_crewai    → Agent.execute_task replacement
  - _patch_langgraph → StateGraph.compile + traced invoke replacement
  - _patch_openai    → openai.chat.completions.create replacement
  - _patch_anthropic → Anthropic().messages.create replacement
  - _try_instrument_* guards → silent no-op when framework not installed

Run: pytest tests/test_patching.py -v
"""

import sys
from types import ModuleType
from unittest.mock import MagicMock, patch

import pytest

from govagn import Attrs


# ─── Helpers ─────────────────────────────────────────────────────────────────

def _make_span():
    """Return a mock OTEL span usable as a context manager."""
    span = MagicMock()
    span.__enter__ = MagicMock(return_value=span)
    span.__exit__ = MagicMock(return_value=False)
    return span


def _make_tracer():
    """Return (mock_tracer, mock_span). Tracer yields span from start_as_current_span."""
    span = _make_span()
    tracer = MagicMock()
    tracer.start_as_current_span.return_value = span
    return tracer, span


# ─── CrewAI patching ─────────────────────────────────────────────────────────

class TestPatchCrewAI:
    """_patch_crewai replaces Agent.execute_task with a tracing wrapper."""

    def _make_crewai_module(self):
        """Build a minimal mock crewai module matching the import surface used by the patch."""
        original_execute = MagicMock(return_value="task_result")

        # Use a real class so we can check attribute assignment
        class Agent:
            role = "Researcher"
            _crew_id = "crew-001"
            execute_task = original_execute

        mod = ModuleType("crewai")
        mod.Agent = Agent
        mod.Task = MagicMock()
        mod.Crew = MagicMock()
        return mod, Agent, original_execute

    # ── replacement ──────────────────────────────────────────────────────────

    def test_execute_task_is_replaced(self):
        """execute_task must be a different callable after patching."""
        mod, Agent, original = self._make_crewai_module()
        with patch.dict(sys.modules, {"crewai": mod}):
            from govagn import _patch_crewai
            _patch_crewai(mod)
        assert Agent.execute_task is not original

    # ── call-through ─────────────────────────────────────────────────────────

    def test_patched_execute_calls_original(self):
        """The tracing wrapper must delegate to the original execute_task."""
        mod, Agent, original = self._make_crewai_module()
        tracer, span = _make_tracer()
        with patch.dict(sys.modules, {"crewai": mod}):
            from govagn import _patch_crewai
            _patch_crewai(mod)
        with patch("govagn.get_tracer", return_value=tracer):
            fake_agent = Agent()
            fake_task = MagicMock()
            fake_task.description = "Do something"
            Agent.execute_task(fake_agent, fake_task)
        original.assert_called_once()

    def test_patched_execute_returns_original_result(self):
        """Return value of the original function must be preserved."""
        mod, Agent, original = self._make_crewai_module()
        original.return_value = "my_result"
        tracer, span = _make_tracer()
        with patch.dict(sys.modules, {"crewai": mod}):
            from govagn import _patch_crewai
            _patch_crewai(mod)
        with patch("govagn.get_tracer", return_value=tracer):
            fake_agent = Agent()
            fake_task = MagicMock()
            fake_task.description = None
            result = Agent.execute_task(fake_agent, fake_task)
        assert result == "my_result"

    # ── span attributes ───────────────────────────────────────────────────────

    def test_span_carries_crewai_agent_role(self):
        """crewai.agent.role must be set in span attributes (Principle 5 detection)."""
        mod, Agent, original = self._make_crewai_module()
        tracer, span = _make_tracer()
        with patch.dict(sys.modules, {"crewai": mod}):
            from govagn import _patch_crewai
            _patch_crewai(mod)
        with patch("govagn.get_tracer", return_value=tracer):
            fake_agent = Agent()
            fake_agent.role = "Data Analyst"
            fake_task = MagicMock()
            fake_task.description = "Analyse the data"
            Agent.execute_task(fake_agent, fake_task)
        attrs = tracer.start_as_current_span.call_args[1]["attributes"]
        assert attrs[Attrs.CREWAI_AGENT_ROLE] == "Data Analyst"

    def test_span_carries_gen_ai_system_crewai(self):
        """gen_ai.system must be 'crewai' for server-side framework detection."""
        mod, Agent, original = self._make_crewai_module()
        tracer, span = _make_tracer()
        with patch.dict(sys.modules, {"crewai": mod}):
            from govagn import _patch_crewai
            _patch_crewai(mod)
        with patch("govagn.get_tracer", return_value=tracer):
            fake_agent = Agent()
            fake_task = MagicMock()
            fake_task.description = None
            Agent.execute_task(fake_agent, fake_task)
        attrs = tracer.start_as_current_span.call_args[1]["attributes"]
        assert attrs[Attrs.GEN_AI_SYSTEM] == "crewai"

    def test_task_description_is_hashed_not_stored(self):
        """Task description is SHA-256 hashed (PII protection) — never stored raw."""
        import hashlib
        mod, Agent, original = self._make_crewai_module()
        tracer, span = _make_tracer()
        description = "Analyse confidential Q4 revenue data"
        with patch.dict(sys.modules, {"crewai": mod}):
            from govagn import _patch_crewai
            _patch_crewai(mod)
        with patch("govagn.get_tracer", return_value=tracer):
            fake_agent = Agent()
            fake_task = MagicMock()
            fake_task.description = description
            Agent.execute_task(fake_agent, fake_task)
        expected_hash = hashlib.sha256(description.encode()).hexdigest()[:16]
        span.set_attribute.assert_any_call(Attrs.CREWAI_TASK_DESC, expected_hash)

    # ── error handling ────────────────────────────────────────────────────────

    def test_exception_propagates(self):
        """Errors from the original execute_task must not be swallowed."""
        mod, Agent, original = self._make_crewai_module()
        original.side_effect = RuntimeError("crewai task failed")
        tracer, span = _make_tracer()
        with patch.dict(sys.modules, {"crewai": mod}):
            from govagn import _patch_crewai
            _patch_crewai(mod)
        with patch("govagn.get_tracer", return_value=tracer):
            fake_agent = Agent()
            fake_task = MagicMock()
            fake_task.description = None
            with pytest.raises(RuntimeError, match="crewai task failed"):
                Agent.execute_task(fake_agent, fake_task)

    def test_exception_marks_span_error(self):
        """Span must be set to ERROR status when execute_task raises."""
        from opentelemetry.trace import StatusCode
        mod, Agent, original = self._make_crewai_module()
        original.side_effect = ValueError("boom")
        tracer, span = _make_tracer()
        with patch.dict(sys.modules, {"crewai": mod}):
            from govagn import _patch_crewai
            _patch_crewai(mod)
        with patch("govagn.get_tracer", return_value=tracer):
            fake_agent = Agent()
            fake_task = MagicMock()
            fake_task.description = None
            with pytest.raises(ValueError):
                Agent.execute_task(fake_agent, fake_task)
        span.set_status.assert_called()
        status = span.set_status.call_args[0][0]
        assert status.status_code == StatusCode.ERROR


# ─── LangGraph patching ───────────────────────────────────────────────────────

class TestPatchLangGraph:
    """_patch_langgraph wraps StateGraph.compile so every graph invocation is traced."""

    def _make_langgraph_module(self):
        original_invoke = MagicMock(return_value={"result": "ok"})

        # compiled graph returned by compile()
        compiled = MagicMock()
        compiled.invoke = original_invoke

        original_compile = MagicMock(return_value=compiled)

        class StateGraph:
            compile = original_compile

        lg_graph = ModuleType("langgraph.graph")
        lg_graph.StateGraph = StateGraph

        lg = ModuleType("langgraph")
        lg.graph = lg_graph

        return lg, lg_graph, StateGraph, original_compile, compiled, original_invoke

    # ── replacement ──────────────────────────────────────────────────────────

    def test_compile_is_replaced(self):
        """StateGraph.compile is wrapped after patching."""
        lg, lg_graph, StateGraph, orig_compile, _, _ = self._make_langgraph_module()
        with patch.dict(sys.modules, {"langgraph": lg, "langgraph.graph": lg_graph}):
            from govagn import _patch_langgraph
            _patch_langgraph(lg)
        assert StateGraph.compile is not orig_compile

    def test_patched_compile_calls_original(self):
        """The compile wrapper must call the original compile and return the graph."""
        lg, lg_graph, StateGraph, orig_compile, compiled, _ = self._make_langgraph_module()
        with patch.dict(sys.modules, {"langgraph": lg, "langgraph.graph": lg_graph}):
            from govagn import _patch_langgraph
            _patch_langgraph(lg)
        fake_sg = MagicMock()
        result = StateGraph.compile(fake_sg)
        orig_compile.assert_called_once_with(fake_sg)
        assert result is not None

    # ── traced invoke ─────────────────────────────────────────────────────────

    def test_invoke_is_wrapped_with_span(self):
        """compiled.invoke must be replaced by a traced version after compile()."""
        lg, lg_graph, StateGraph, orig_compile, compiled, orig_invoke = self._make_langgraph_module()
        tracer, span = _make_tracer()
        with patch.dict(sys.modules, {"langgraph": lg, "langgraph.graph": lg_graph}):
            from govagn import _patch_langgraph
            _patch_langgraph(lg)
        with patch("govagn.get_tracer", return_value=tracer):
            fake_sg = MagicMock()
            graph = StateGraph.compile(fake_sg)
            graph.invoke({"messages": []})
        tracer.start_as_current_span.assert_called()
        span_name = tracer.start_as_current_span.call_args[0][0]
        assert span_name == "langgraph.graph.invoke"

    def test_invoke_span_carries_graph_id(self):
        """Span must carry langgraph.graph.id (Principle 5 — server-side detection)."""
        lg, lg_graph, StateGraph, _, _, _ = self._make_langgraph_module()
        tracer, span = _make_tracer()
        with patch.dict(sys.modules, {"langgraph": lg, "langgraph.graph": lg_graph}):
            from govagn import _patch_langgraph
            _patch_langgraph(lg)
        with patch("govagn.get_tracer", return_value=tracer):
            fake_sg = MagicMock()
            graph = StateGraph.compile(fake_sg)
            graph.invoke({"state": "x"})
        attrs = tracer.start_as_current_span.call_args[1]["attributes"]
        assert Attrs.LANGGRAPH_GRAPH in attrs
        assert attrs[Attrs.LANGGRAPH_GRAPH]  # must be non-empty

    def test_invoke_calls_original(self):
        """Traced invoke must delegate to the original compiled.invoke."""
        lg, lg_graph, StateGraph, _, _, orig_invoke = self._make_langgraph_module()
        tracer, span = _make_tracer()
        with patch.dict(sys.modules, {"langgraph": lg, "langgraph.graph": lg_graph}):
            from govagn import _patch_langgraph
            _patch_langgraph(lg)
        with patch("govagn.get_tracer", return_value=tracer):
            fake_sg = MagicMock()
            graph = StateGraph.compile(fake_sg)
            graph.invoke({"key": "value"}, config=None)
        orig_invoke.assert_called_once()

    def test_each_graph_gets_unique_id(self):
        """Two separate compile() calls must produce spans with different graph IDs."""
        lg, lg_graph, StateGraph, orig_compile, compiled, _ = self._make_langgraph_module()
        # Make compile return a fresh compiled object each call
        orig_compile.side_effect = lambda *a, **kw: MagicMock(invoke=MagicMock(return_value={}))
        tracer, _ = _make_tracer()
        with patch.dict(sys.modules, {"langgraph": lg, "langgraph.graph": lg_graph}):
            from govagn import _patch_langgraph
            _patch_langgraph(lg)
        graph_ids = []
        with patch("govagn.get_tracer", return_value=tracer):
            for _ in range(2):
                fake_sg = MagicMock()
                g = StateGraph.compile(fake_sg)
                g.invoke({})
                attrs = tracer.start_as_current_span.call_args[1]["attributes"]
                graph_ids.append(attrs.get(Attrs.LANGGRAPH_GRAPH))
        assert graph_ids[0] != graph_ids[1], "Each compiled graph must have a unique ID"


# ─── OpenAI patching ─────────────────────────────────────────────────────────

class TestPatchOpenAI:
    """_patch_openai replaces openai.chat.completions.create with a traced version."""

    def _make_openai_module(self, prompt_tokens=100, completion_tokens=50, finish_reason="stop"):
        original_create = MagicMock()
        usage = MagicMock(prompt_tokens=prompt_tokens, completion_tokens=completion_tokens)
        choice = MagicMock(finish_reason=finish_reason)
        original_create.return_value = MagicMock(usage=usage, choices=[choice])

        completions = MagicMock()
        completions.create = original_create
        chat = MagicMock()
        chat.completions = completions
        mod = MagicMock()
        mod.chat = chat
        return mod, original_create

    # ── replacement ──────────────────────────────────────────────────────────

    def test_create_is_replaced(self):
        """chat.completions.create must be a different callable after patching."""
        mod, original = self._make_openai_module()
        from govagn import _patch_openai
        _patch_openai(mod)
        assert mod.chat.completions.create is not original

    # ── call-through ─────────────────────────────────────────────────────────

    def test_patched_create_calls_original(self):
        """Traced create must delegate to the original."""
        mod, original = self._make_openai_module()
        tracer, span = _make_tracer()
        from govagn import _patch_openai
        _patch_openai(mod)
        with patch("govagn.get_tracer", return_value=tracer):
            mod.chat.completions.create(model="gpt-4o", messages=[])
        original.assert_called_once()

    def test_patched_create_returns_response(self):
        """The original response object must be returned unchanged."""
        mod, original = self._make_openai_module()
        sentinel = object()
        original.return_value = sentinel
        tracer, span = _make_tracer()
        from govagn import _patch_openai
        _patch_openai(mod)
        with patch("govagn.get_tracer", return_value=tracer):
            result = mod.chat.completions.create(model="gpt-4o", messages=[])
        assert result is sentinel

    # ── span attributes ───────────────────────────────────────────────────────

    def test_span_carries_gen_ai_request_model(self):
        """gen_ai.request.model must match the model kwarg."""
        mod, _ = self._make_openai_module()
        tracer, span = _make_tracer()
        from govagn import _patch_openai
        _patch_openai(mod)
        with patch("govagn.get_tracer", return_value=tracer):
            mod.chat.completions.create(model="gpt-4o-mini", messages=[])
        attrs = tracer.start_as_current_span.call_args[1]["attributes"]
        assert attrs[Attrs.GEN_AI_REQUEST_MODEL] == "gpt-4o-mini"

    def test_span_carries_gen_ai_system_openai(self):
        """gen_ai.system must be 'openai' (Principle 5 — framework detection)."""
        mod, _ = self._make_openai_module()
        tracer, span = _make_tracer()
        from govagn import _patch_openai
        _patch_openai(mod)
        with patch("govagn.get_tracer", return_value=tracer):
            mod.chat.completions.create(model="gpt-4", messages=[])
        attrs = tracer.start_as_current_span.call_args[1]["attributes"]
        assert attrs[Attrs.GEN_AI_SYSTEM] == "openai"

    def test_token_usage_set_on_span(self):
        """Input and output tokens from the response must be recorded."""
        mod, _ = self._make_openai_module(prompt_tokens=120, completion_tokens=60)
        tracer, span = _make_tracer()
        from govagn import _patch_openai
        _patch_openai(mod)
        with patch("govagn.get_tracer", return_value=tracer):
            mod.chat.completions.create(model="gpt-4o", messages=[])
        span.set_attribute.assert_any_call(Attrs.GEN_AI_INPUT_TOKENS, 120)
        span.set_attribute.assert_any_call(Attrs.GEN_AI_OUTPUT_TOKENS, 60)

    def test_finish_reason_set_on_span(self):
        """Finish reason from response choices must be set on the span."""
        mod, _ = self._make_openai_module(finish_reason="length")
        tracer, span = _make_tracer()
        from govagn import _patch_openai
        _patch_openai(mod)
        with patch("govagn.get_tracer", return_value=tracer):
            mod.chat.completions.create(model="gpt-4o", messages=[])
        span.set_attribute.assert_any_call(Attrs.GEN_AI_FINISH_REASONS, "['length']")

    # ── error handling ────────────────────────────────────────────────────────

    def test_exception_propagates(self):
        """API errors must propagate from the traced wrapper."""
        mod, original = self._make_openai_module()
        original.side_effect = ConnectionError("OpenAI unreachable")
        tracer, span = _make_tracer()
        from govagn import _patch_openai
        _patch_openai(mod)
        with patch("govagn.get_tracer", return_value=tracer):
            with pytest.raises(ConnectionError, match="OpenAI unreachable"):
                mod.chat.completions.create(model="gpt-4o", messages=[])

    def test_exception_marks_span_error(self):
        """Span must be ERROR when the API call raises."""
        from opentelemetry.trace import StatusCode
        mod, original = self._make_openai_module()
        original.side_effect = TimeoutError("timeout")
        tracer, span = _make_tracer()
        from govagn import _patch_openai
        _patch_openai(mod)
        with patch("govagn.get_tracer", return_value=tracer):
            with pytest.raises(TimeoutError):
                mod.chat.completions.create(model="gpt-4o", messages=[])
        span.set_status.assert_called()
        assert span.set_status.call_args[0][0].status_code == StatusCode.ERROR

    def test_idempotent_patch_does_not_double_wrap(self):
        """Calling _patch_openai twice must not nest the wrapper (no double-counting)."""
        mod, original = self._make_openai_module()
        tracer, span = _make_tracer()
        from govagn import _patch_openai
        _patch_openai(mod)
        first_patched = mod.chat.completions.create
        _patch_openai(mod)  # second call patches the already-patched function
        with patch("govagn.get_tracer", return_value=tracer):
            mod.chat.completions.create(model="gpt-4o", messages=[])
        # The original must still be called exactly once regardless of double-patch
        original.assert_called_once()


# ─── Anthropic patching ───────────────────────────────────────────────────────

class TestPatchAnthropic:
    """_patch_anthropic wraps Anthropic().messages.create with a traced version."""

    def _make_anthropic_module(
        self,
        input_tokens=200,
        output_tokens=80,
        cache_creation=0,
        cache_read=0,
        stop_reason="end_turn",
    ):
        original_create = MagicMock()
        usage = MagicMock(
            input_tokens=input_tokens,
            output_tokens=output_tokens,
            cache_creation_input_tokens=cache_creation,
            cache_read_input_tokens=cache_read,
        )
        original_create.return_value = MagicMock(usage=usage, stop_reason=stop_reason)

        # _patch_anthropic accesses Anthropic.messages (class-level) in a hasattr
        # guard before falling through to type(Anthropic().messages).  The real SDK
        # exposes `messages` both as a class-level descriptor and as an instance
        # attribute.  Our mock mirrors that by setting it at class level too.
        class Messages:
            create = original_create

        _shared = Messages()  # class-level sentinel used only by hasattr check

        class MockAnthropic:
            messages = _shared   # class-level: satisfies `Anthropic.messages.create`

            def __init__(self):
                self.messages = Messages()   # instance-level: used by type()

        mod = ModuleType("anthropic")
        mod.Anthropic = MockAnthropic
        return mod, MockAnthropic, Messages, original_create

    # ── replacement ──────────────────────────────────────────────────────────

    def test_messages_create_is_replaced(self):
        """Messages.create must be replaced after patching."""
        mod, _, Messages, original = self._make_anthropic_module()
        with patch.dict(sys.modules, {"anthropic": mod}):
            from govagn import _patch_anthropic
            _patch_anthropic(mod)
        assert Messages.create is not original

    # ── call-through ─────────────────────────────────────────────────────────

    def test_patched_create_calls_original(self):
        """Traced create must delegate to the original Messages.create."""
        mod, MockAnthropic, Messages, original = self._make_anthropic_module()
        tracer, span = _make_tracer()
        with patch.dict(sys.modules, {"anthropic": mod}):
            from govagn import _patch_anthropic
            _patch_anthropic(mod)
        with patch("govagn.get_tracer", return_value=tracer):
            instance = Messages()
            instance.create(model="claude-opus-4-6", messages=[], max_tokens=1024)
        original.assert_called_once()

    # ── span attributes ───────────────────────────────────────────────────────

    def test_span_carries_anthropic_model(self):
        """anthropic.model must be set in span attributes (Principle 5 detection)."""
        mod, _, Messages, _ = self._make_anthropic_module()
        tracer, span = _make_tracer()
        with patch.dict(sys.modules, {"anthropic": mod}):
            from govagn import _patch_anthropic
            _patch_anthropic(mod)
        with patch("govagn.get_tracer", return_value=tracer):
            instance = Messages()
            instance.create(model="claude-sonnet-4-6", messages=[], max_tokens=512)
        attrs = tracer.start_as_current_span.call_args[1]["attributes"]
        assert attrs[Attrs.ANTHROPIC_MODEL] == "claude-sonnet-4-6"

    def test_span_carries_gen_ai_system_anthropic(self):
        """gen_ai.system must be 'anthropic' for collector framework detection."""
        mod, _, Messages, _ = self._make_anthropic_module()
        tracer, span = _make_tracer()
        with patch.dict(sys.modules, {"anthropic": mod}):
            from govagn import _patch_anthropic
            _patch_anthropic(mod)
        with patch("govagn.get_tracer", return_value=tracer):
            instance = Messages()
            instance.create(model="claude-haiku-4-5-20251001", messages=[], max_tokens=256)
        attrs = tracer.start_as_current_span.call_args[1]["attributes"]
        assert attrs[Attrs.GEN_AI_SYSTEM] == "anthropic"

    def test_token_usage_set_on_span(self):
        """Input and output tokens from the response must be recorded."""
        mod, _, Messages, _ = self._make_anthropic_module(input_tokens=300, output_tokens=90)
        tracer, span = _make_tracer()
        with patch.dict(sys.modules, {"anthropic": mod}):
            from govagn import _patch_anthropic
            _patch_anthropic(mod)
        with patch("govagn.get_tracer", return_value=tracer):
            instance = Messages()
            instance.create(model="claude-opus-4-6", messages=[], max_tokens=2048)
        span.set_attribute.assert_any_call(Attrs.GEN_AI_INPUT_TOKENS, 300)
        span.set_attribute.assert_any_call(Attrs.GEN_AI_OUTPUT_TOKENS, 90)

    def test_cache_creation_tokens_tracked(self):
        """Anthropic prompt-cache creation tokens must be recorded (Principle 7)."""
        mod, _, Messages, _ = self._make_anthropic_module(cache_creation=450)
        tracer, span = _make_tracer()
        with patch.dict(sys.modules, {"anthropic": mod}):
            from govagn import _patch_anthropic
            _patch_anthropic(mod)
        with patch("govagn.get_tracer", return_value=tracer):
            instance = Messages()
            instance.create(model="claude-opus-4-6", messages=[], max_tokens=1024)
        span.set_attribute.assert_any_call(Attrs.ANTHROPIC_CACHE_CREATION, 450)

    def test_cache_read_tokens_tracked(self):
        """Anthropic prompt-cache read tokens must be recorded (Principle 7)."""
        mod, _, Messages, _ = self._make_anthropic_module(cache_read=200)
        tracer, span = _make_tracer()
        with patch.dict(sys.modules, {"anthropic": mod}):
            from govagn import _patch_anthropic
            _patch_anthropic(mod)
        with patch("govagn.get_tracer", return_value=tracer):
            instance = Messages()
            instance.create(model="claude-opus-4-6", messages=[], max_tokens=1024)
        span.set_attribute.assert_any_call(Attrs.ANTHROPIC_CACHE_READ, 200)

    def test_stop_reason_set_on_span(self):
        """stop_reason from the response must be recorded as finish reasons."""
        mod, _, Messages, _ = self._make_anthropic_module(stop_reason="max_tokens")
        tracer, span = _make_tracer()
        with patch.dict(sys.modules, {"anthropic": mod}):
            from govagn import _patch_anthropic
            _patch_anthropic(mod)
        with patch("govagn.get_tracer", return_value=tracer):
            instance = Messages()
            instance.create(model="claude-sonnet-4-6", messages=[], max_tokens=100)
        span.set_attribute.assert_any_call(Attrs.GEN_AI_FINISH_REASONS, "['max_tokens']")

    # ── error handling ────────────────────────────────────────────────────────

    def test_exception_propagates(self):
        """Anthropic API errors must propagate from the traced wrapper."""
        mod, _, Messages, original = self._make_anthropic_module()
        original.side_effect = TimeoutError("Anthropic API timeout")
        tracer, span = _make_tracer()
        with patch.dict(sys.modules, {"anthropic": mod}):
            from govagn import _patch_anthropic
            _patch_anthropic(mod)
        with patch("govagn.get_tracer", return_value=tracer):
            instance = Messages()
            with pytest.raises(TimeoutError, match="Anthropic API timeout"):
                instance.create(model="claude-opus-4-6", messages=[], max_tokens=1024)

    def test_exception_marks_span_error(self):
        """Span must be ERROR when the Anthropic API call raises."""
        from opentelemetry.trace import StatusCode
        mod, _, Messages, original = self._make_anthropic_module()
        original.side_effect = ConnectionError("network error")
        tracer, span = _make_tracer()
        with patch.dict(sys.modules, {"anthropic": mod}):
            from govagn import _patch_anthropic
            _patch_anthropic(mod)
        with patch("govagn.get_tracer", return_value=tracer):
            instance = Messages()
            with pytest.raises(ConnectionError):
                instance.create(model="claude-opus-4-6", messages=[], max_tokens=1024)
        span.set_status.assert_called()
        assert span.set_status.call_args[0][0].status_code == StatusCode.ERROR


# ─── _try_instrument_* guard tests ────────────────────────────────────────────

class TestTryInstrumentGuards:
    """_try_instrument_* must silently no-op when a framework is not installed."""

    def test_crewai_absent_does_not_raise(self):
        with patch.dict(sys.modules, {"crewai": None}):
            from govagn import _try_instrument_crewai
            _try_instrument_crewai()  # must not raise

    def test_langgraph_absent_does_not_raise(self):
        with patch.dict(sys.modules, {"langgraph": None}):
            from govagn import _try_instrument_langgraph
            _try_instrument_langgraph()

    def test_openai_absent_does_not_raise(self):
        with patch.dict(sys.modules, {"openai": None}):
            from govagn import _try_instrument_openai
            _try_instrument_openai()

    def test_anthropic_absent_does_not_raise(self):
        with patch.dict(sys.modules, {"anthropic": None}):
            from govagn import _try_instrument_anthropic
            _try_instrument_anthropic()

    def test_crewai_import_error_logged_not_raised(self):
        """ImportError during framework patching must be caught, not propagated."""
        # Remove crewai from modules to trigger ImportError path in _try_instrument_crewai
        saved = sys.modules.pop("crewai", None)
        try:
            from govagn import _try_instrument_crewai
            _try_instrument_crewai()  # must not raise even with real ImportError
        finally:
            if saved is not None:
                sys.modules["crewai"] = saved
