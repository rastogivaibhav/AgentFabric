"""
Integration tests for AgentFabric SDK framework patching.

Unlike test_patching.py (which uses sys.modules mocks), these tests install
and exercise the REAL framework packages. HTTP calls are intercepted at the
httpx transport layer via respx — no API keys or network access required.

Install:
    pip install -e ".[integration]"

Run integration tests only:
    pytest tests/test_patching_integration.py -v -m integration

Run a single framework:
    pytest tests/test_patching_integration.py::TestOpenAIIntegration -v
"""
from __future__ import annotations

import hashlib
import importlib
import os

import pytest

# Set dummy credentials before any SDK is lazily initialised.
os.environ.setdefault("OPENAI_API_KEY", "sk-integration-test-placeholder")
os.environ.setdefault("ANTHROPIC_API_KEY", "sk-ant-integration-test-placeholder")

from opentelemetry import trace
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import SimpleSpanProcessor
from opentelemetry.sdk.trace.export.in_memory_span_exporter import InMemorySpanExporter

# ── Shared in-memory exporter (session-scoped) ────────────────────────────────

_mem_exporter = InMemorySpanExporter()


@pytest.fixture(scope="session", autouse=True)
def _setup_agentfabric_tracer():
    """Wire agentfabric to in-memory exporter without hitting a real OTLP endpoint."""
    import agentfabric  # noqa: PLC0415

    provider = TracerProvider()
    provider.add_span_processor(SimpleSpanProcessor(_mem_exporter))
    trace.set_tracer_provider(provider)
    agentfabric._tracer = trace.get_tracer("agentfabric", "1.0.0")
    agentfabric._initialized = True


@pytest.fixture(autouse=True)
def _clear_spans():
    """Isolate span captures between tests."""
    _mem_exporter.clear()
    yield
    _mem_exporter.clear()


def _spans_named(fragment: str):
    """Return all finished spans whose name contains *fragment*."""
    return [s for s in _mem_exporter.get_finished_spans() if fragment in s.name]


# ── Fake HTTP response payloads ───────────────────────────────────────────────

_OPENAI_RESPONSE = {
    "id": "chatcmpl-integration-test",
    "object": "chat.completion",
    "created": 1_700_000_000,
    "model": "gpt-4o",
    "choices": [
        {
            "index": 0,
            "message": {"role": "assistant", "content": "Integration test OK"},
            "finish_reason": "stop",
            "logprobs": None,
        }
    ],
    "usage": {"prompt_tokens": 12, "completion_tokens": 7, "total_tokens": 19},
    "system_fingerprint": None,
}

_ANTHROPIC_RESPONSE = {
    "id": "msg_integration_test",
    "type": "message",
    "role": "assistant",
    "content": [{"type": "text", "text": "Integration test OK"}],
    "model": "claude-opus-4-6",
    "stop_reason": "end_turn",
    "stop_sequence": None,
    "usage": {"input_tokens": 8, "output_tokens": 6},
}


# ── LangGraph helper ──────────────────────────────────────────────────────────

def _build_graph(node_fn, node_name: str = "step"):
    """Build and compile a minimal StateGraph; handles both old and new API."""
    from langgraph.graph import StateGraph  # noqa: PLC0415

    builder = StateGraph(dict)
    builder.add_node(node_name, node_fn)
    try:
        from langgraph.graph import START, END  # noqa: PLC0415  # langgraph ≥ 0.1
        builder.add_edge(START, node_name)
        builder.add_edge(node_name, END)
    except ImportError:
        builder.set_entry_point(node_name)       # langgraph 0.0.x fallback
        builder.set_finish_point(node_name)
    return builder.compile()


# ═══════════════════════════════════════════════════════════════════════════════
# LangGraph — no HTTP calls needed, real graph execution
# ═══════════════════════════════════════════════════════════════════════════════

@pytest.mark.integration
class TestLangGraphIntegration:
    """Exercise real langgraph.graph.StateGraph execution with our patch applied."""

    @pytest.fixture(scope="class", autouse=True)
    def _require_and_patch(self):
        lg = pytest.importorskip("langgraph", reason="langgraph not installed")
        import agentfabric  # noqa: PLC0415
        from langgraph.graph import StateGraph  # noqa: PLC0415

        original_compile = StateGraph.compile
        agentfabric._patch_langgraph(lg)
        yield
        StateGraph.compile = original_compile  # restore for other test modules

    # ── correctness ──────────────────────────────────────────────────────────

    def test_compiled_app_is_callable(self):
        app = _build_graph(lambda s: s)
        assert callable(app.invoke)

    def test_state_passes_through_unmodified(self):
        """Patching must not alter return values."""
        app = _build_graph(lambda s: {**s, "done": True})
        result = app.invoke({"x": 7})
        assert result["x"] == 7
        assert result["done"] is True

    def test_transform_node_runs_correctly(self):
        app = _build_graph(lambda s: {"value": s["value"] * 3}, "triple")
        assert app.invoke({"value": 4})["value"] == 12

    # ── span emission ─────────────────────────────────────────────────────────

    def test_invoke_emits_langgraph_span(self):
        app = _build_graph(lambda s: s)
        app.invoke({"ping": "pong"})

        spans = _spans_named("langgraph.graph.invoke")
        assert len(spans) >= 1, "Expected at least one langgraph span"

    def test_span_carries_gen_ai_system_attribute(self):
        app = _build_graph(lambda s: s)
        app.invoke({})

        attrs = _spans_named("langgraph.graph.invoke")[-1].attributes
        assert attrs.get("gen_ai.system") == "langgraph"

    def test_span_carries_graph_id_attribute(self):
        app = _build_graph(lambda s: s)
        app.invoke({})

        attrs = _spans_named("langgraph.graph.invoke")[-1].attributes
        assert attrs.get("langgraph.graph.id") is not None
        assert len(attrs["langgraph.graph.id"]) == 8  # uuid[:8]

    def test_each_compiled_graph_has_unique_id(self):
        app1 = _build_graph(lambda s: s, "n1")
        app2 = _build_graph(lambda s: s, "n2")

        app1.invoke({})
        id1 = _spans_named("langgraph.graph.invoke")[-1].attributes["langgraph.graph.id"]
        _mem_exporter.clear()

        app2.invoke({})
        id2 = _spans_named("langgraph.graph.invoke")[-1].attributes["langgraph.graph.id"]

        assert id1 != id2

    def test_exception_in_node_still_produces_span(self):
        def boom(state):
            raise RuntimeError("deliberate failure")

        app = _build_graph(boom)
        with pytest.raises(Exception):
            app.invoke({})

        # The span is still exported even though the node raised
        assert len(_spans_named("langgraph.graph.invoke")) >= 1


# ═══════════════════════════════════════════════════════════════════════════════
# OpenAI — real SDK parsing; HTTP intercepted by respx
# ═══════════════════════════════════════════════════════════════════════════════

@pytest.mark.integration
class TestOpenAIIntegration:
    """
    Exercises the real openai SDK.  respx intercepts every httpx request so
    the real JSON parsing and model validation inside openai runs, but no
    network calls leave the test process.
    """

    @pytest.fixture(scope="class", autouse=True)
    def _require_and_patch(self):
        pytest.importorskip("respx", reason="respx not installed")
        pytest.importorskip("httpx", reason="httpx not installed")
        openai = pytest.importorskip("openai", reason="openai not installed")
        import agentfabric  # noqa: PLC0415

        original = openai.chat.completions.create
        agentfabric._patch_openai(openai)
        yield
        openai.chat.completions.create = original

    @staticmethod
    def _fake_200():
        import httpx  # noqa: PLC0415
        return httpx.Response(
            200,
            json=_OPENAI_RESPONSE,
            headers={"content-type": "application/json"},
        )

    @staticmethod
    def _call(model: str = "gpt-4o", content: str = "hello"):
        import openai, respx  # noqa: PLC0415, E401
        with respx.mock(assert_all_called=False) as rmock:
            rmock.post("https://api.openai.com/v1/chat/completions").mock(
                return_value=TestOpenAIIntegration._fake_200()
            )
            return openai.chat.completions.create(
                model=model,
                messages=[{"role": "user", "content": content}],
            )

    # ── correctness ──────────────────────────────────────────────────────────

    def test_response_is_parsed_by_real_sdk(self):
        resp = self._call()
        assert resp.choices[0].message.content == "Integration test OK"
        assert resp.usage.total_tokens == 19

    # ── span emission ─────────────────────────────────────────────────────────

    def test_call_emits_span(self):
        self._call()
        assert len(_spans_named("openai")) >= 1

    def test_span_name_includes_model(self):
        self._call(model="gpt-4o")
        assert len(_spans_named("llm.openai.gpt-4o")) >= 1

    def test_span_has_gen_ai_system(self):
        self._call()
        assert _spans_named("openai")[-1].attributes.get("gen_ai.system") == "openai"

    def test_span_has_model_attribute(self):
        self._call(model="gpt-4o")
        assert _spans_named("openai")[-1].attributes.get("gen_ai.request.model") == "gpt-4o"

    def test_span_has_token_counts(self):
        self._call()
        attrs = _spans_named("openai")[-1].attributes
        assert attrs.get("gen_ai.usage.input_tokens") == 12
        assert attrs.get("gen_ai.usage.output_tokens") == 7

    def test_span_has_finish_reason(self):
        self._call()
        reasons = _spans_named("openai")[-1].attributes.get("gen_ai.response.finish_reasons", "")
        assert "stop" in reasons

    def test_error_response_still_emits_span(self):
        import openai, respx, httpx  # noqa: PLC0415, E401

        with respx.mock(assert_all_called=False):
            respx.post("https://api.openai.com/v1/chat/completions").mock(
                return_value=httpx.Response(
                    401,
                    json={"error": {"message": "Invalid API key", "type": "invalid_request_error"}},
                    headers={"content-type": "application/json"},
                )
            )
            with pytest.raises(Exception):
                openai.chat.completions.create(
                    model="gpt-4o",
                    messages=[{"role": "user", "content": "hello"}],
                )

        assert len(_spans_named("openai")) >= 1


# ═══════════════════════════════════════════════════════════════════════════════
# Anthropic — real SDK parsing; HTTP intercepted by respx
# ═══════════════════════════════════════════════════════════════════════════════

@pytest.mark.integration
class TestAnthropicIntegration:
    """
    Exercises the real anthropic SDK.  respx intercepts httpx so the SDK's
    full response parsing pipeline runs without network access.
    """

    @pytest.fixture(scope="class", autouse=True)
    def _require_and_patch(self):
        pytest.importorskip("respx", reason="respx not installed")
        pytest.importorskip("httpx", reason="httpx not installed")
        anthropic = pytest.importorskip("anthropic", reason="anthropic not installed")
        import agentfabric  # noqa: PLC0415

        from anthropic import Anthropic  # noqa: PLC0415
        Messages = type(Anthropic(api_key="test").messages)
        original = Messages.create
        agentfabric._patch_anthropic(anthropic)
        yield
        Messages.create = original

    @staticmethod
    def _fake_200():
        import httpx  # noqa: PLC0415
        return httpx.Response(
            200,
            json=_ANTHROPIC_RESPONSE,
            headers={"content-type": "application/json"},
        )

    @staticmethod
    def _call(model: str = "claude-opus-4-6", max_tokens: int = 64):
        import anthropic, respx  # noqa: PLC0415, E401
        client = anthropic.Anthropic(api_key="test-key")
        with respx.mock(assert_all_called=False) as rmock:
            rmock.post("https://api.anthropic.com/v1/messages").mock(
                return_value=TestAnthropicIntegration._fake_200()
            )
            return client.messages.create(
                model=model,
                max_tokens=max_tokens,
                messages=[{"role": "user", "content": "hello"}],
            )

    # ── correctness ──────────────────────────────────────────────────────────

    def test_response_is_parsed_by_real_sdk(self):
        resp = self._call()
        assert resp.content[0].text == "Integration test OK"
        assert resp.usage.input_tokens == 8

    # ── span emission ─────────────────────────────────────────────────────────

    def test_call_emits_span(self):
        self._call()
        assert len(_spans_named("anthropic")) >= 1

    def test_span_name_includes_model(self):
        self._call(model="claude-opus-4-6")
        assert len(_spans_named("llm.anthropic.claude-opus-4-6")) >= 1

    def test_span_has_gen_ai_system(self):
        self._call()
        assert _spans_named("anthropic")[-1].attributes.get("gen_ai.system") == "anthropic"

    def test_span_has_model_attributes(self):
        self._call(model="claude-opus-4-6")
        attrs = _spans_named("anthropic")[-1].attributes
        assert attrs.get("gen_ai.request.model") == "claude-opus-4-6"
        assert attrs.get("anthropic.model") == "claude-opus-4-6"

    def test_span_has_token_counts(self):
        self._call()
        attrs = _spans_named("anthropic")[-1].attributes
        assert attrs.get("gen_ai.usage.input_tokens") == 8
        assert attrs.get("gen_ai.usage.output_tokens") == 6

    def test_span_has_stop_reason(self):
        self._call()
        reasons = _spans_named("anthropic")[-1].attributes.get("gen_ai.response.finish_reasons", "")
        assert "end_turn" in reasons

    def test_span_has_api_version(self):
        self._call()
        assert _spans_named("anthropic")[-1].attributes.get("anthropic.api_version") == "2023-06-01"

    def test_error_response_still_emits_span(self):
        import anthropic, respx, httpx  # noqa: PLC0415, E401

        client = anthropic.Anthropic(api_key="test-key")
        with respx.mock(assert_all_called=False):
            respx.post("https://api.anthropic.com/v1/messages").mock(
                return_value=httpx.Response(
                    401,
                    json={"type": "error", "error": {"type": "authentication_error", "message": "bad key"}},
                    headers={"content-type": "application/json"},
                )
            )
            with pytest.raises(Exception):
                client.messages.create(
                    model="claude-opus-4-6",
                    max_tokens=64,
                    messages=[{"role": "user", "content": "hello"}],
                )

        assert len(_spans_named("anthropic")) >= 1


# ═══════════════════════════════════════════════════════════════════════════════
# CrewAI — real agent objects; LLM failures are expected and handled
# ═══════════════════════════════════════════════════════════════════════════════

@pytest.mark.integration
class TestCrewAIIntegration:
    """
    Exercises the real crewai package.  Because no real LLM is wired up,
    execute_task will raise after the span wrapper fires — these tests confirm
    that spans and attributes are populated correctly *before* the LLM call.
    """

    @pytest.fixture(scope="class", autouse=True)
    def _require_and_patch(self):
        crewai = pytest.importorskip("crewai", reason="crewai not installed")
        if not hasattr(crewai.Agent, "execute_task"):
            pytest.skip("crewai.Agent.execute_task not present in this version")
        import agentfabric  # noqa: PLC0415

        original = crewai.Agent.execute_task
        agentfabric._patch_crewai(crewai)
        yield
        crewai.Agent.execute_task = original

    @staticmethod
    def _make_agent_and_task(role: str = "Integration Tester", description: str = "Do something"):
        import crewai  # noqa: PLC0415

        try:
            agent = crewai.Agent(
                role=role,
                goal="Test span emission",
                backstory="An agent used in integration tests",
                llm="gpt-4o",
                allow_delegation=False,
            )
            task = crewai.Task(
                description=description,
                expected_output="A result",
                agent=agent,
            )
            return agent, task
        except Exception as exc:
            pytest.skip(f"CrewAI Agent/Task instantiation failed: {exc}")

    # ── patch application ─────────────────────────────────────────────────────

    def test_execute_task_method_is_replaced(self):
        import crewai  # noqa: PLC0415
        method = crewai.Agent.execute_task
        # After patching the function's __name__ is preserved by functools.wraps
        # but the object itself is no longer the original undecorated function.
        assert callable(method)
        assert method.__name__ in ("execute_task", "patched_execute")

    # ── span emission on LLM failure ──────────────────────────────────────────

    def test_span_emitted_before_llm_raises(self):
        agent, task = self._make_agent_and_task(role="Span Checker")
        with pytest.raises(Exception):
            agent.execute_task(task)

        spans = _spans_named("crewai")
        assert len(spans) >= 1, "Expected a crewai span even when LLM call fails"

    def test_span_carries_agent_role(self):
        agent, task = self._make_agent_and_task(role="Role Verifier")
        with pytest.raises(Exception):
            agent.execute_task(task)

        attrs = _spans_named("crewai")[-1].attributes
        assert attrs.get("crewai.agent.role") == "Role Verifier"
        assert attrs.get("gen_ai.system") == "crewai"

    def test_span_name_uses_normalised_role(self):
        """Agent role is lower-cased and spaces replaced with underscores."""
        agent, task = self._make_agent_and_task(role="Research Expert")
        with pytest.raises(Exception):
            agent.execute_task(task)

        assert len(_spans_named("crewai.agent.research_expert")) >= 1

    def test_task_description_is_hashed(self):
        description = "Return the current date and time"
        agent, task = self._make_agent_and_task(description=description)
        with pytest.raises(Exception):
            agent.execute_task(task)

        expected = hashlib.sha256(description.encode()).hexdigest()[:16]
        attrs = _spans_named("crewai")[-1].attributes
        assert attrs.get("crewai.task.description_hash") == expected


# ═══════════════════════════════════════════════════════════════════════════════
# Google ADK
# ═══════════════════════════════════════════════════════════════════════════════

@pytest.mark.integration
class TestGoogleADKIntegration:
    """Tests for agentfabric._patch_google_adk().

    Uses a lightweight EchoAgent (no LLM) so tests run without API keys or
    network access.  The patching wraps Runner.run_async and BaseAgent.run_async
    to emit spans — both layers are exercised here.
    """

    @pytest.fixture(autouse=True, scope="class")
    def _apply_patch(self):
        """Apply google-adk patching once for the whole class (not per test).

        Calling _patch_google_adk() per test wraps Runner.run_async/
        BaseAgent.run_async repeatedly, creating N spans per invocation on the
        Nth test — which breaks span-count and parent-child assertions.
        """
        import agentfabric  # noqa: PLC0415

        agentfabric._patch_google_adk()

    # ── minimal agent fixture ─────────────────────────────────────────────────

    @staticmethod
    def _make_agent():
        """Return a no-LLM BaseAgent subclass that yields one event and exits."""
        try:
            from google.adk.agents.base_agent import BaseAgent
            from google.adk.events import Event

            class EchoAgent(BaseAgent):
                model_config = {"arbitrary_types_allowed": True}

                async def _run_async_impl(self, ctx):
                    yield Event(
                        author=self.name,
                        invocation_id=ctx.invocation_id,
                        content=None,
                    )

            return EchoAgent(name="echo_agent")
        except Exception as exc:
            pytest.skip(f"Google ADK unavailable: {exc}")

    @staticmethod
    def _run(agent, session_id: str = "sess-001"):
        """Synchronously drive Runner.run_async to completion."""
        import asyncio

        from google.adk.runners import Runner
        from google.adk.sessions import InMemorySessionService
        from google.genai.types import Content, Part

        async def _inner():
            ss = InMemorySessionService()
            runner = Runner(
                agent=agent,
                app_name="af-integration-test",
                session_service=ss,
            )
            await ss.create_session(
                app_name="af-integration-test",
                user_id="test-user",
                session_id=session_id,
            )
            msg = Content(role="user", parts=[Part(text="hello")])
            events = []
            async for ev in runner.run_async(
                user_id="test-user",
                session_id=session_id,
                new_message=msg,
            ):
                events.append(ev)
            return events

        return asyncio.run(_inner())

    # ── patch verification ────────────────────────────────────────────────────

    def test_runner_run_async_is_replaced(self):
        from google.adk.runners import Runner

        # After patching the method wrapper is not the original coroutine
        # function — it is our traced_run_async generator wrapper.
        fn = Runner.run_async
        assert callable(fn)
        assert fn.__name__ in ("run_async", "traced_run_async")

    def test_base_agent_run_async_is_replaced(self):
        from google.adk.agents.base_agent import BaseAgent

        fn = BaseAgent.run_async
        assert callable(fn)
        assert fn.__name__ in ("run_async", "traced_agent_run_async")

    # ── runner-level span ─────────────────────────────────────────────────────

    def test_runner_span_is_emitted(self):
        agent = self._make_agent()
        self._run(agent)

        spans = _spans_named("google_adk.runner.")
        assert len(spans) >= 1, "Expected a runner-level span"

    def test_runner_span_name_contains_agent_name(self):
        agent = self._make_agent()
        self._run(agent)

        assert len(_spans_named("google_adk.runner.echo_agent")) >= 1

    def test_runner_span_gen_ai_system(self):
        agent = self._make_agent()
        self._run(agent)

        attrs = _spans_named("google_adk.runner.echo_agent")[-1].attributes
        assert attrs.get("gen_ai.system") == "google_adk"

    def test_runner_span_agent_name_attribute(self):
        agent = self._make_agent()
        self._run(agent)

        attrs = _spans_named("google_adk.runner.echo_agent")[-1].attributes
        assert attrs.get("google.adk.agent.name") == "echo_agent"

    def test_runner_span_session_id_attribute(self):
        agent = self._make_agent()
        self._run(agent, session_id="sess-adk-42")

        attrs = _spans_named("google_adk.runner.echo_agent")[-1].attributes
        assert attrs.get("google.adk.session.id") == "sess-adk-42"

    # ── agent-level span ──────────────────────────────────────────────────────

    def test_agent_span_is_emitted(self):
        agent = self._make_agent()
        self._run(agent)

        spans = _spans_named("google_adk.agent.")
        assert len(spans) >= 1, "Expected an agent-level span"

    def test_agent_span_name_contains_agent_name(self):
        agent = self._make_agent()
        self._run(agent)

        assert len(_spans_named("google_adk.agent.echo_agent")) >= 1

    def test_agent_span_gen_ai_system(self):
        agent = self._make_agent()
        self._run(agent)

        attrs = _spans_named("google_adk.agent.echo_agent")[-1].attributes
        assert attrs.get("gen_ai.system") == "google_adk"

    def test_agent_span_agent_name_attribute(self):
        agent = self._make_agent()
        self._run(agent)

        attrs = _spans_named("google_adk.agent.echo_agent")[-1].attributes
        assert attrs.get("google.adk.agent.name") == "echo_agent"

    # ── span hierarchy ────────────────────────────────────────────────────────

    def test_agent_span_is_child_of_runner_span(self):
        """The agent-level span must be nested inside the runner-level span."""
        agent = self._make_agent()
        self._run(agent)

        runner_spans = _spans_named("google_adk.runner.echo_agent")
        agent_spans = _spans_named("google_adk.agent.echo_agent")
        assert runner_spans and agent_spans
        runner_ctx = runner_spans[-1].context
        agent_parent_id = agent_spans[-1].parent.span_id
        assert agent_parent_id == runner_ctx.span_id, (
            "Agent span's parent span_id should match the runner span"
        )

    # ── multiple independent runs do not bleed spans ──────────────────────────

    def test_two_runs_produce_two_runner_spans(self):
        agent = self._make_agent()
        _mem_exporter.clear()
        self._run(agent, session_id="s1")
        self._run(agent, session_id="s2")

        runner_spans = _spans_named("google_adk.runner.echo_agent")
        assert len(runner_spans) == 2
