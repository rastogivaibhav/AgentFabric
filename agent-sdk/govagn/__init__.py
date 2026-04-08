"""
Govagn Python SDK
Automatic instrumentation for CrewAI, LangGraph, Google ADK, OpenAI Agents SDK, Claude

Usage:
    from govagn import instrument

    instrument(
        endpoint="http://localhost:4317",
        service_name="my-agent-service",
        environment="production",
    )

    # Your existing agent code works unchanged
    from crewai import Crew, Agent, Task
    crew = Crew(...)  # automatically traced
"""

from __future__ import annotations

import functools
import inspect
import logging
import os
import time
import uuid
from contextlib import contextmanager
from dataclasses import dataclass, field
from typing import Any, Callable, Optional

from opentelemetry import trace
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.trace import SpanKind, Status, StatusCode
from opentelemetry.trace.propagation.tracecontext import TraceContextTextMapPropagator

logger = logging.getLogger("govagn")

# ─── Semantic Attribute Constants ────────────────────────────────────────────

class Attrs:
    # Standard GenAI OTEL
    GEN_AI_SYSTEM         = "gen_ai.system"
    GEN_AI_REQUEST_MODEL  = "gen_ai.request.model"
    GEN_AI_INPUT_TOKENS   = "gen_ai.usage.input_tokens"
    GEN_AI_OUTPUT_TOKENS  = "gen_ai.usage.output_tokens"
    GEN_AI_TEMPERATURE    = "gen_ai.request.temperature"
    GEN_AI_MAX_TOKENS     = "gen_ai.request.max_tokens"
    GEN_AI_FINISH_REASONS = "gen_ai.response.finish_reasons"

    # Govagn SDK attributes
    AGENT_NAME     = "af.agent.name"
    AGENT_ROLE     = "af.agent.role"
    AGENT_RUN_ID   = "af.agent.run_id"
    TOOL_NAME      = "af.agent.tool.name"
    TOOL_CALL_ID   = "af.agent.tool.call_id"
    TOOL_STATUS    = "af.agent.tool.status"

    # CrewAI
    CREWAI_AGENT_ROLE = "crewai.agent.role"
    CREWAI_CREW_ID    = "crewai.crew.id"
    CREWAI_TASK_DESC  = "crewai.task.description_hash"

    # LangGraph
    LANGGRAPH_NODE  = "langgraph.node.name"
    LANGGRAPH_GRAPH = "langgraph.graph.id"
    LANGGRAPH_EDGE  = "langgraph.edge.type"

    # Google ADK
    GOOGLE_ADK_AGENT   = "google.adk.agent.name"
    GOOGLE_ADK_SESSION = "google.adk.session.id"

    # OpenAI Agents
    OPENAI_RUN_ID    = "openai.run.id"
    OPENAI_THREAD_ID = "openai.thread.id"

    # Anthropic
    ANTHROPIC_MODEL          = "anthropic.model"
    ANTHROPIC_API_VERSION    = "anthropic.api_version"
    ANTHROPIC_CACHE_CREATION = "anthropic.cache_creation_tokens"
    ANTHROPIC_CACHE_READ     = "anthropic.cache_read_tokens"

    # Prompt lifecycle
    PROMPT_ID          = "af.prompt.id"
    PROMPT_VERSION     = "af.prompt.version"
    PROMPT_RELEASE_TAG = "af.prompt.release_tag"
    PROMPT_ENVIRONMENT = "af.prompt.environment"


# ─── SDK Initialisation ──────────────────────────────────────────────────────

_tracer: Optional[trace.Tracer] = None
_initialized = False


def instrument(
    endpoint: str = None,
    service_name: str = "govagn-service",
    service_version: str = "1.0.0",
    environment: str = "development",
    api_key: str = None,
    insecure: bool = True,
    auto_instrument_crewai: bool = True,
    auto_instrument_langgraph: bool = True,
    auto_instrument_openai: bool = True,
    auto_instrument_anthropic: bool = True,
    auto_instrument_google_adk: bool = True,
) -> trace.Tracer:
    """
    Initialise Govagn instrumentation.
    Call once at application startup before importing agent frameworks.
    """
    global _tracer, _initialized

    endpoint = endpoint or os.environ.get("GV_ENDPOINT", "http://localhost:4317")
    api_key  = api_key  or os.environ.get("GV_API_KEY", "")

    resource = Resource.create({
        "service.name":    service_name,
        "service.version": service_version,
        "deployment.environment": environment,
        Attrs.PROMPT_ID: os.environ.get("GV_PROMPT_ID", "").strip(),
        Attrs.PROMPT_VERSION: os.environ.get("GV_PROMPT_VERSION", "").strip(),
        Attrs.PROMPT_RELEASE_TAG: os.environ.get("GV_PROMPT_RELEASE_TAG", "").strip(),
        Attrs.PROMPT_ENVIRONMENT: os.environ.get("GV_PROMPT_ENVIRONMENT", environment).strip(),
    })

    headers = {}
    if api_key:
        headers["x-af-api-key"] = api_key

    exporter = OTLPSpanExporter(
        endpoint=endpoint,
        insecure=insecure,
        headers=headers,
    )

    provider = TracerProvider(resource=resource)
    provider.add_span_processor(
        BatchSpanProcessor(
            exporter,
            max_queue_size=10000,
            max_export_batch_size=512,
            schedule_delay_millis=2000,
        )
    )
    trace.set_tracer_provider(provider)
    _tracer = trace.get_tracer("govagn", "1.0.0")
    _initialized = True

    # Auto-instrument frameworks
    if auto_instrument_crewai:
        _try_instrument_crewai()
    if auto_instrument_langgraph:
        _try_instrument_langgraph()
    if auto_instrument_openai:
        _try_instrument_openai()
    if auto_instrument_anthropic:
        _try_instrument_anthropic()
    if auto_instrument_google_adk:
        _try_instrument_google_adk()

    logger.info(f"Govagn instrumentation active → {endpoint}")
    return _tracer


def get_tracer() -> trace.Tracer:
    if not _initialized:
        raise RuntimeError("Call govagn.instrument() before using the SDK")
    return _tracer


# ─── Context Manager for Manual Instrumentation ──────────────────────────────

@contextmanager
def agent_span(
    name: str,
    framework: str = "custom",
    agent_name: str = "",
    agent_role: str = "",
    run_id: str = None,
    attributes: dict = None,
):
    """Context manager for manual span creation."""
    tracer = get_tracer()
    run_id = run_id or str(uuid.uuid4())

    with tracer.start_as_current_span(
        name,
        kind=SpanKind.INTERNAL,
        attributes={
            Attrs.AGENT_NAME:  agent_name,
            Attrs.AGENT_ROLE:  agent_role,
            Attrs.AGENT_RUN_ID: run_id,
            **(attributes or {}),
        },
    ) as span:
        try:
            yield span
        except Exception as e:
            span.set_status(Status(StatusCode.ERROR, str(e)))
            span.record_exception(e)
            raise


def trace_tool_call(tool_name: str, call_id: str = None):
    """Decorator for tool functions."""
    def decorator(func: Callable) -> Callable:
        @functools.wraps(func)
        def wrapper(*args, **kwargs):
            tracer = get_tracer()
            cid = call_id or str(uuid.uuid4())
            with tracer.start_as_current_span(
                f"tool.{tool_name}",
                kind=SpanKind.INTERNAL,
                attributes={
                    Attrs.TOOL_NAME:    tool_name,
                    Attrs.TOOL_CALL_ID: cid,
                },
            ) as span:
                try:
                    result = func(*args, **kwargs)
                    span.set_attribute(Attrs.TOOL_STATUS, "success")
                    return result
                except Exception as e:
                    span.set_attribute(Attrs.TOOL_STATUS, "error")
                    span.set_status(Status(StatusCode.ERROR, str(e)))
                    span.record_exception(e)
                    raise
        return wrapper
    return decorator


# ─── CrewAI Auto-instrumentation ─────────────────────────────────────────────

def _try_instrument_crewai():
    try:
        import crewai
        _patch_crewai(crewai)
        logger.info("Govagn: CrewAI instrumented")
    except ImportError:
        pass


def _patch_crewai(crewai):
    from crewai import Agent, Task, Crew

    # Patch Agent.execute_task
    original_execute = Agent.execute_task

    def patched_execute(self, task, context=None, tools=None):
        tracer = get_tracer()
        crew_id = getattr(self, "_crew_id", str(uuid.uuid4()))
        with tracer.start_as_current_span(
            f"crewai.agent.{self.role.lower().replace(' ', '_')}",
            attributes={
                Attrs.CREWAI_AGENT_ROLE: self.role,
                Attrs.CREWAI_CREW_ID:    crew_id,
                Attrs.AGENT_NAME:        self.role,
                Attrs.GEN_AI_SYSTEM:     "crewai",
            },
        ) as span:
            if task.description:
                import hashlib
                h = hashlib.sha256(task.description.encode()).hexdigest()[:16]
                span.set_attribute(Attrs.CREWAI_TASK_DESC, h)
            try:
                result = original_execute(self, task, context, tools)
                return result
            except Exception as e:
                span.set_status(Status(StatusCode.ERROR, str(e)))
                span.record_exception(e)
                raise

    Agent.execute_task = patched_execute


# ─── LangGraph Auto-instrumentation ──────────────────────────────────────────

def _try_instrument_langgraph():
    try:
        import langgraph
        _patch_langgraph(langgraph)
        logger.info("Govagn: LangGraph instrumented")
    except ImportError:
        pass


def _patch_langgraph(langgraph):
    try:
        import asyncio
        from langgraph.graph import StateGraph

        # ── Node-level tracing ────────────────────────────────────────────────
        # Wrap each node function at add_node time so every node execution
        # produces a child span under the parent graph.invoke span.

        original_add_node = getattr(StateGraph, "add_node", None)

        def _wrap_node(node_name: str, fn):
            """Return a traced wrapper for a node function (sync or async)."""
            if asyncio.iscoroutinefunction(fn):
                @functools.wraps(fn)
                async def traced_async_node(state, *args, **kw):
                    tracer = get_tracer()
                    with tracer.start_as_current_span(
                        f"langgraph.node.{node_name}",
                        attributes={
                            Attrs.LANGGRAPH_NODE:  node_name,
                            Attrs.GEN_AI_SYSTEM:   "langgraph",
                        },
                    ) as span:
                        try:
                            return await fn(state, *args, **kw)
                        except Exception as e:
                            span.set_status(Status(StatusCode.ERROR, str(e)))
                            span.record_exception(e)
                            raise
                return traced_async_node
            else:
                @functools.wraps(fn)
                def traced_sync_node(state, *args, **kw):
                    tracer = get_tracer()
                    with tracer.start_as_current_span(
                        f"langgraph.node.{node_name}",
                        attributes={
                            Attrs.LANGGRAPH_NODE:  node_name,
                            Attrs.GEN_AI_SYSTEM:   "langgraph",
                        },
                    ) as span:
                        try:
                            return fn(state, *args, **kw)
                        except Exception as e:
                            span.set_status(Status(StatusCode.ERROR, str(e)))
                            span.record_exception(e)
                            raise
                return traced_sync_node

        # Some minimal/mock StateGraph implementations expose compile() only.
        # Keep graph-level tracing active even if node-level hook points are absent.
        if callable(original_add_node):
            def patched_add_node(self, node, action=None, **kwargs):
                # LangGraph accepts add_node(name, func) or add_node(func).
                # Only wrap plain callables — Runnables expose .invoke() and are
                # handled by LangChain's own tracing layer.
                if callable(node) and action is None:
                    node_name = getattr(node, "__name__", "node")
                    node = _wrap_node(node_name, node)
                elif isinstance(node, str) and callable(action):
                    action = _wrap_node(node, action)
                return original_add_node(self, node, action, **kwargs)

            StateGraph.add_node = patched_add_node

        # ── Graph-level tracing ───────────────────────────────────────────────
        # Wrap the compiled graph's invoke() to create the outer span that all
        # node spans nest under.

        original_compile = StateGraph.compile

        def patched_compile(self, *args, **kwargs):
            compiled = original_compile(self, *args, **kwargs)
            graph_id = str(uuid.uuid4())[:8]
            original_invoke = compiled.invoke

            def traced_invoke(input_data, config=None, **kw):
                tracer = get_tracer()
                with tracer.start_as_current_span(
                    "langgraph.graph.invoke",
                    attributes={
                        Attrs.LANGGRAPH_GRAPH: graph_id,
                        Attrs.GEN_AI_SYSTEM:   "langgraph",
                    },
                ):
                    return original_invoke(input_data, config, **kw)

            compiled.invoke = traced_invoke
            return compiled

        StateGraph.compile = patched_compile
    except Exception as e:
        logger.debug(f"LangGraph patch failed: {e}")


# ─── OpenAI Agents SDK Auto-instrumentation ───────────────────────────────────

def _try_instrument_openai():
    try:
        import openai
        _patch_openai(openai)
        logger.info("Govagn: OpenAI instrumented")
    except ImportError:
        pass


def _patch_openai(openai_module):
    try:
        original_create = openai_module.chat.completions.create

        def traced_create(*args, **kwargs):
            tracer = get_tracer()
            model = kwargs.get("model", "unknown")
            with tracer.start_as_current_span(
                f"llm.openai.{model}",
                attributes={
                    Attrs.GEN_AI_SYSTEM:        "openai",
                    Attrs.GEN_AI_REQUEST_MODEL:  model,
                    Attrs.GEN_AI_TEMPERATURE:    str(kwargs.get("temperature", "—")),
                    Attrs.GEN_AI_MAX_TOKENS:     str(kwargs.get("max_tokens", "—")),
                },
            ) as span:
                try:
                    response = original_create(*args, **kwargs)
                    if hasattr(response, "usage") and response.usage:
                        span.set_attribute(Attrs.GEN_AI_INPUT_TOKENS, response.usage.prompt_tokens)
                        span.set_attribute(Attrs.GEN_AI_OUTPUT_TOKENS, response.usage.completion_tokens)
                    if hasattr(response, "choices") and response.choices:
                        reasons = [c.finish_reason for c in response.choices if c.finish_reason]
                        span.set_attribute(Attrs.GEN_AI_FINISH_REASONS, str(reasons))
                    return response
                except Exception as e:
                    span.set_status(Status(StatusCode.ERROR, str(e)))
                    span.record_exception(e)
                    raise

        openai_module.chat.completions.create = traced_create
    except Exception as e:
        logger.debug(f"OpenAI patch failed: {e}")


# ─── Anthropic Auto-instrumentation ──────────────────────────────────────────

def _try_instrument_anthropic():
    try:
        import anthropic
        _patch_anthropic(anthropic)
        logger.info("Govagn: Anthropic instrumented")
    except ImportError:
        pass


def _patch_anthropic(anthropic_module):
    try:
        from anthropic import Anthropic

        # Resolve the concrete Messages class from a live instance.
        # Accessing Anthropic.messages at class level returns a cached_property
        # descriptor (sdk >= 0.28), not the Messages class — so we instantiate.
        Messages = type(Anthropic().messages)
        original_messages_create = Messages.create

        def traced_messages_create(self_messages, *args, **kwargs):
            tracer = get_tracer()
            model = kwargs.get("model", "unknown")
            with tracer.start_as_current_span(
                f"llm.anthropic.{model}",
                attributes={
                    Attrs.GEN_AI_SYSTEM:        "anthropic",
                    Attrs.GEN_AI_REQUEST_MODEL:  model,
                    Attrs.ANTHROPIC_MODEL:       model,
                    Attrs.ANTHROPIC_API_VERSION: "2023-06-01",
                    Attrs.GEN_AI_MAX_TOKENS:     str(kwargs.get("max_tokens", "—")),
                },
            ) as span:
                try:
                    response = original_messages_create(self_messages, *args, **kwargs)
                    if hasattr(response, "usage") and response.usage:
                        span.set_attribute(Attrs.GEN_AI_INPUT_TOKENS, response.usage.input_tokens)
                        span.set_attribute(Attrs.GEN_AI_OUTPUT_TOKENS, response.usage.output_tokens)
                        if hasattr(response.usage, "cache_creation_input_tokens"):
                            span.set_attribute(
                                Attrs.ANTHROPIC_CACHE_CREATION,
                                response.usage.cache_creation_input_tokens or 0,
                            )
                        if hasattr(response.usage, "cache_read_input_tokens"):
                            span.set_attribute(
                                Attrs.ANTHROPIC_CACHE_READ,
                                response.usage.cache_read_input_tokens or 0,
                            )
                    if hasattr(response, "stop_reason"):
                        span.set_attribute(Attrs.GEN_AI_FINISH_REASONS, str([response.stop_reason]))
                    return response
                except Exception as e:
                    span.set_status(Status(StatusCode.ERROR, str(e)))
                    span.record_exception(e)
                    raise

        Messages.create = traced_messages_create
    except Exception as e:
        logger.debug(f"Anthropic patch failed: {e}")


# ─── Google ADK Auto-instrumentation ─────────────────────────────────────────

def _try_instrument_google_adk():
    try:
        import google.adk  # noqa: F401
        _patch_google_adk()
        logger.info("Govagn: Google ADK instrumented")
    except ImportError:
        pass


def _patch_google_adk():
    try:
        from google.adk.runners import Runner
        from google.adk.agents.base_agent import BaseAgent

        # ── Runner-level span (outer: one per session turn) ───────────────────
        # Created by Runner.run_async — covers the entire turn and all agents
        # invoked within it.  Agent-level spans nest inside this span.

        original_run_async = Runner.run_async

        async def traced_run_async(
            self,
            *,
            user_id: str,
            session_id: str,
            invocation_id=None,
            new_message=None,
            state_delta=None,
            run_config=None,
        ):
            tracer = get_tracer()
            agent_name = getattr(getattr(self, "agent", None), "name", "unknown")
            with tracer.start_as_current_span(
                f"google_adk.runner.{agent_name}",
                kind=SpanKind.INTERNAL,
                attributes={
                    Attrs.GOOGLE_ADK_AGENT:   agent_name,
                    Attrs.GOOGLE_ADK_SESSION:  session_id,
                    Attrs.AGENT_NAME:          agent_name,
                    Attrs.GEN_AI_SYSTEM:       "google_adk",
                },
            ) as span:
                try:
                    async for event in original_run_async(
                        self,
                        user_id=user_id,
                        session_id=session_id,
                        invocation_id=invocation_id,
                        new_message=new_message,
                        state_delta=state_delta,
                        run_config=run_config,
                    ):
                        yield event
                except Exception as e:
                    span.set_status(Status(StatusCode.ERROR, str(e)))
                    span.record_exception(e)
                    raise

        Runner.run_async = traced_run_async

        # ── Agent-level spans (one per agent invocation, incl. sub-agents) ───
        # BaseAgent.run_async is called for every agent in the call tree
        # (root agent AND any sub-agents it delegates to).  Patching it here
        # produces a child span per agent, giving full node-level visibility.

        original_agent_run_async = BaseAgent.run_async

        async def traced_agent_run_async(self, invocation_context):
            tracer = get_tracer()
            agent_name = getattr(self, "name", "unknown")
            with tracer.start_as_current_span(
                f"google_adk.agent.{agent_name}",
                kind=SpanKind.INTERNAL,
                attributes={
                    Attrs.GOOGLE_ADK_AGENT: agent_name,
                    Attrs.AGENT_NAME:        agent_name,
                    Attrs.GEN_AI_SYSTEM:     "google_adk",
                },
            ) as span:
                try:
                    async for event in original_agent_run_async(self, invocation_context):
                        yield event
                except Exception as e:
                    span.set_status(Status(StatusCode.ERROR, str(e)))
                    span.record_exception(e)
                    raise

        BaseAgent.run_async = traced_agent_run_async

    except Exception as e:
        logger.debug(f"Google ADK patch failed: {e}")


# ─── Setup.py compatible __all__ ─────────────────────────────────────────────

__all__ = [
    "instrument",
    "get_tracer",
    "agent_span",
    "trace_tool_call",
    "Attrs",
]

__version__ = "1.0.0"
