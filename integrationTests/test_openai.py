"""
Integration tests for AgentFabric OpenAI instrumentation.

All HTTP calls are intercepted by respx at the httpx transport layer.
No real API key or network access is required.
"""

from __future__ import annotations

import os
import sys

# Ensure agent-sdk is importable
sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "agent-sdk"))

# Provide a placeholder API key so the OpenAI client initialises without error
os.environ.setdefault("OPENAI_API_KEY", "sk-integration-test-placeholder")

import pytest
import httpx


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _fake_response(
    model="gpt-4o",
    content="Response.",
    prompt_tokens=12,
    completion_tokens=7,
    finish_reason="stop",
):
    return {
        "id": f"chatcmpl-{model}-test",
        "object": "chat.completion",
        "created": 1700000000,
        "model": model,
        "choices": [
            {
                "index": 0,
                "message": {"role": "assistant", "content": content},
                "finish_reason": finish_reason,
                "logprobs": None,
            }
        ],
        "usage": {
            "prompt_tokens": prompt_tokens,
            "completion_tokens": completion_tokens,
            "total_tokens": prompt_tokens + completion_tokens,
        },
        "system_fingerprint": None,
    }


# ---------------------------------------------------------------------------
# Module-level tracer setup — done once per test session
# ---------------------------------------------------------------------------

from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import SimpleSpanProcessor
from opentelemetry.sdk.trace.export.in_memory_span_exporter import InMemorySpanExporter

_exporter = InMemorySpanExporter()
_provider = TracerProvider()
_provider.add_span_processor(SimpleSpanProcessor(_exporter))
# Gateway processor is added inside _setup_tracer once the session fixture is available.


# ---------------------------------------------------------------------------
# Test class
# ---------------------------------------------------------------------------

@pytest.mark.integration
class TestOpenAIIntegration:
    """30+ integration tests for AgentFabric OpenAI patch using respx HTTP mocks."""

    # ------------------------------------------------------------------
    # Fixtures
    # ------------------------------------------------------------------

    @pytest.fixture(autouse=True)
    def _require_deps(self):
        """Skip the whole class if openai or respx are not installed."""
        pytest.importorskip("openai")
        pytest.importorskip("respx")

    @pytest.fixture(autouse=True, scope="class")
    def _setup_tracer(self, gateway_exporter):
        """
        Class-scoped fixture: wire up InMemorySpanExporter and apply the
        agentfabric OpenAI patch exactly once for the entire test class.
        Restores the original create on teardown.
        Spans are also forwarded to the AgentFabric dashboard in real-time.
        """
        import openai
        import agentfabric

        # Add gateway exporter to the shared provider (idempotent — processors
        # accumulate but the gateway exporter silently drops dupes on its end).
        _provider.add_span_processor(SimpleSpanProcessor(gateway_exporter))

        # Point agentfabric at our in-memory provider / tracer
        agentfabric._tracer = _provider.get_tracer("agentfabric-test", "1.0.0")
        agentfabric._initialized = True

        # Save original and patch
        original_create = openai.chat.completions.create
        agentfabric._patch_openai(openai)

        yield

        # Restore
        openai.chat.completions.create = original_create

    @pytest.fixture(autouse=True)
    def _clear_spans(self):
        """Wipe the span exporter before every individual test."""
        _exporter.clear()
        yield

    # ------------------------------------------------------------------
    # Internal helpers
    # ------------------------------------------------------------------

    def _spans(self, fragment: str):
        """Return all finished spans whose name contains *fragment*."""
        return [
            s for s in _exporter.get_finished_spans()
            if fragment in s.name
        ]

    @staticmethod
    def _call(model, messages, **kwargs):
        """
        Make an instrumented openai.chat.completions.create call with the
        HTTP layer mocked via respx so no real network traffic is produced.
        """
        import openai
        import respx

        fake = _fake_response(model=model)

        with respx.mock(assert_all_called=False) as mock_router:
            mock_router.post("https://api.openai.com/v1/chat/completions").mock(
                return_value=httpx.Response(
                    200,
                    json=fake,
                    headers={"content-type": "application/json"},
                )
            )
            response = openai.chat.completions.create(
                model=model,
                messages=messages,
                **kwargs,
            )
        return response

    @staticmethod
    def _call_with_fake(model, messages, fake, **kwargs):
        """Like _call but accepts a fully custom fake response dict."""
        import openai
        import respx

        with respx.mock(assert_all_called=False) as mock_router:
            mock_router.post("https://api.openai.com/v1/chat/completions").mock(
                return_value=httpx.Response(
                    200,
                    json=fake,
                    headers={"content-type": "application/json"},
                )
            )
            response = openai.chat.completions.create(
                model=model,
                messages=messages,
                **kwargs,
            )
        return response

    # ==================================================================
    # Group 1 — Basic Agentic Completions
    # ==================================================================

    def test_customer_greeting_response(self):
        """Agent responds to a simple greeting — verify call completes."""
        messages = [{"role": "user", "content": "Hello, how can you help me?"}]
        response = self._call("gpt-4o", messages)
        assert response is not None
        assert response.choices[0].message.content == "Response."

    def test_code_explanation_request(self):
        """Agent explains a Python decorator — verify response is accessible."""
        messages = [
            {
                "role": "user",
                "content": (
                    "Can you explain what this Python decorator does?\n"
                    "@functools.wraps(func)\ndef wrapper(*args, **kwargs): ..."
                ),
            }
        ]
        response = self._call("gpt-4o", messages)
        assert response.choices is not None
        assert len(response.choices) == 1

    def test_text_summarisation(self):
        """Agent summarises a paragraph — verify a span is emitted."""
        messages = [
            {
                "role": "user",
                "content": (
                    "Please summarise the following text in one sentence:\n"
                    "AgentFabric is an enterprise observability platform designed "
                    "to monitor and govern AI agent behaviour across multiple frameworks."
                ),
            }
        ]
        self._call("gpt-4o", messages)
        spans = self._spans("llm.openai")
        assert len(spans) >= 1

    def test_language_translation(self):
        """Agent translates to French — verify span name contains model."""
        messages = [
            {"role": "user", "content": "Translate 'Good morning' to French."}
        ]
        self._call("gpt-4o", messages)
        spans = self._spans("llm.openai.gpt-4o")
        assert len(spans) >= 1

    def test_sentiment_analysis(self):
        """Agent classifies sentiment — verify span exists."""
        messages = [
            {
                "role": "user",
                "content": (
                    "Classify the sentiment of this review as Positive, Negative, or Neutral:\n"
                    "'The product arrived quickly but the packaging was damaged.'"
                ),
            }
        ]
        self._call("gpt-4o", messages)
        assert len(self._spans("llm.openai")) >= 1

    def test_named_entity_extraction(self):
        """Agent extracts named entities — verify response object integrity."""
        messages = [
            {
                "role": "user",
                "content": (
                    "Extract all named entities (persons, organisations, locations) "
                    "from: 'Elon Musk founded SpaceX in Hawthorne, California.'"
                ),
            }
        ]
        response = self._call("gpt-4o", messages)
        assert response.model == "gpt-4o"

    # ==================================================================
    # Group 2 — Different Models
    # ==================================================================

    def test_gpt4o_produces_span(self):
        """Span name matches llm.openai.gpt-4o for the gpt-4o model."""
        self._call("gpt-4o", [{"role": "user", "content": "Hi"}])
        spans = self._spans("llm.openai.gpt-4o")
        assert len(spans) >= 1
        assert spans[0].name == "llm.openai.gpt-4o"

    def test_gpt4o_mini_produces_span(self):
        """Span name matches llm.openai.gpt-4o-mini for the gpt-4o-mini model."""
        self._call("gpt-4o-mini", [{"role": "user", "content": "Hi"}])
        spans = self._spans("llm.openai.gpt-4o-mini")
        assert len(spans) >= 1
        assert spans[0].name == "llm.openai.gpt-4o-mini"

    def test_gpt35_turbo_produces_span(self):
        """Span name matches llm.openai.gpt-3.5-turbo for the gpt-3.5-turbo model."""
        self._call("gpt-3.5-turbo", [{"role": "user", "content": "Hi"}])
        spans = self._spans("llm.openai.gpt-3.5-turbo")
        assert len(spans) >= 1
        assert spans[0].name == "llm.openai.gpt-3.5-turbo"

    def test_o1_mini_produces_span(self):
        """Span name matches llm.openai.o1-mini for the o1-mini model."""
        self._call("o1-mini", [{"role": "user", "content": "Hi"}])
        spans = self._spans("llm.openai.o1-mini")
        assert len(spans) >= 1
        assert spans[0].name == "llm.openai.o1-mini"

    # ==================================================================
    # Group 3 — Span Attribute Verification
    # ==================================================================

    def test_gen_ai_system_is_openai(self):
        """gen_ai.system attribute must equal 'openai'."""
        self._call("gpt-4o", [{"role": "user", "content": "Ping"}])
        spans = self._spans("llm.openai.gpt-4o")
        assert len(spans) >= 1
        attrs = dict(spans[0].attributes)
        assert attrs.get("gen_ai.system") == "openai"

    def test_gen_ai_request_model_matches(self):
        """gen_ai.request.model attribute must match the model kwarg."""
        self._call("gpt-4o-mini", [{"role": "user", "content": "Ping"}])
        spans = self._spans("llm.openai.gpt-4o-mini")
        assert len(spans) >= 1
        attrs = dict(spans[0].attributes)
        assert attrs.get("gen_ai.request.model") == "gpt-4o-mini"

    def test_input_tokens_captured_from_usage(self):
        """gen_ai.usage.input_tokens must equal the fake response prompt_tokens."""
        fake = _fake_response(model="gpt-4o", prompt_tokens=25, completion_tokens=10)

        import openai
        import respx

        with respx.mock(assert_all_called=False) as mock_router:
            mock_router.post("https://api.openai.com/v1/chat/completions").mock(
                return_value=httpx.Response(
                    200,
                    json=fake,
                    headers={"content-type": "application/json"},
                )
            )
            openai.chat.completions.create(
                model="gpt-4o",
                messages=[{"role": "user", "content": "Count tokens"}],
            )

        spans = self._spans("llm.openai.gpt-4o")
        assert len(spans) >= 1
        attrs = dict(spans[0].attributes)
        assert attrs.get("gen_ai.usage.input_tokens") == 25

    def test_output_tokens_captured_from_usage(self):
        """gen_ai.usage.output_tokens must equal the fake response completion_tokens."""
        fake = _fake_response(model="gpt-4o", prompt_tokens=8, completion_tokens=42)

        import openai
        import respx

        with respx.mock(assert_all_called=False) as mock_router:
            mock_router.post("https://api.openai.com/v1/chat/completions").mock(
                return_value=httpx.Response(
                    200,
                    json=fake,
                    headers={"content-type": "application/json"},
                )
            )
            openai.chat.completions.create(
                model="gpt-4o",
                messages=[{"role": "user", "content": "Count output tokens"}],
            )

        spans = self._spans("llm.openai.gpt-4o")
        assert len(spans) >= 1
        attrs = dict(spans[0].attributes)
        assert attrs.get("gen_ai.usage.output_tokens") == 42

    def test_finish_reason_stop_in_span(self):
        """gen_ai.response.finish_reasons must contain 'stop'."""
        self._call("gpt-4o", [{"role": "user", "content": "Finish check"}])
        spans = self._spans("llm.openai.gpt-4o")
        assert len(spans) >= 1
        attrs = dict(spans[0].attributes)
        finish_reasons = attrs.get("gen_ai.response.finish_reasons", "")
        assert "stop" in finish_reasons

    def test_span_name_format_llm_openai_model(self):
        """Span name must follow the pattern 'llm.openai.<model>'."""
        model = "gpt-4o"
        self._call(model, [{"role": "user", "content": "Name format check"}])
        spans = self._spans(f"llm.openai.{model}")
        assert len(spans) >= 1
        assert spans[0].name == f"llm.openai.{model}"

    # ==================================================================
    # Group 4 — Request Parameters
    # ==================================================================

    def test_zero_temperature_request(self):
        """temperature=0 should still produce a span."""
        self._call(
            "gpt-4o",
            [{"role": "user", "content": "Deterministic answer please."}],
            temperature=0,
        )
        spans = self._spans("llm.openai.gpt-4o")
        assert len(spans) >= 1

    def test_max_tokens_limited(self):
        """max_tokens=50 is forwarded to the model call without error."""
        response = self._call(
            "gpt-4o",
            [{"role": "user", "content": "Short answer only."}],
            max_tokens=50,
        )
        assert response is not None
        assert response.choices[0].finish_reason == "stop"

    def test_system_prompt_plus_user(self):
        """System + user messages together produce a single span."""
        messages = [
            {"role": "system", "content": "You are a helpful assistant."},
            {"role": "user", "content": "What is 2+2?"},
        ]
        self._call("gpt-4o", messages)
        spans = self._spans("llm.openai.gpt-4o")
        assert len(spans) >= 1

    def test_multi_turn_conversation(self):
        """A 3-message (user/assistant/user) conversation produces a span."""
        messages = [
            {"role": "user", "content": "What is the capital of France?"},
            {"role": "assistant", "content": "Paris."},
            {"role": "user", "content": "And Germany?"},
        ]
        self._call("gpt-4o", messages)
        spans = self._spans("llm.openai.gpt-4o")
        assert len(spans) >= 1

    def test_json_mode_response_format(self):
        """response_format with json_object type is accepted without error."""
        messages = [
            {
                "role": "user",
                "content": "Return a JSON object with key 'result' and value 4.",
            }
        ]
        response = self._call(
            "gpt-4o",
            messages,
            response_format={"type": "json_object"},
        )
        assert response is not None

    # ==================================================================
    # Group 5 — Real-World Agentic Use Cases
    # ==================================================================

    def test_customer_support_ticket_response(self):
        """Agent responds to a customer billing complaint — verify span and content."""
        messages = [
            {
                "role": "system",
                "content": "You are a customer support agent for a SaaS company.",
            },
            {
                "role": "user",
                "content": (
                    "I was charged twice for my subscription this month. "
                    "Please help me resolve this immediately."
                ),
            },
        ]
        response = self._call("gpt-4o", messages)
        assert response.choices[0].message.role == "assistant"
        spans = self._spans("llm.openai.gpt-4o")
        assert len(spans) >= 1

    def test_code_review_feedback(self):
        """Agent reviews a Python function and provides feedback — verify span."""
        messages = [
            {
                "role": "system",
                "content": "You are a senior software engineer performing code review.",
            },
            {
                "role": "user",
                "content": (
                    "Review this Python function and give line-level comments:\n"
                    "def divide(a, b):\n    return a / b\n"
                ),
            },
        ]
        self._call("gpt-4o", messages)
        spans = self._spans("llm.openai.gpt-4o")
        assert len(spans) >= 1

    def test_bug_analysis_from_stack_trace(self):
        """Agent diagnoses a NullPointerException from a stack trace — verify response."""
        messages = [
            {
                "role": "system",
                "content": "You are a debugging assistant.",
            },
            {
                "role": "user",
                "content": (
                    "Diagnose this Java error:\n"
                    "java.lang.NullPointerException\n"
                    "    at com.example.UserService.getUser(UserService.java:42)\n"
                    "    at com.example.UserController.handle(UserController.java:17)\n"
                ),
            },
        ]
        response = self._call("gpt-4o", messages)
        assert response is not None
        assert len(self._spans("llm.openai.gpt-4o")) >= 1

    def test_sql_query_generation(self):
        """Agent generates a SQL query from a natural language question."""
        messages = [
            {
                "role": "system",
                "content": "You convert natural language questions into SQL queries.",
            },
            {
                "role": "user",
                "content": (
                    "Show me the top 5 customers by total revenue in the last 30 days "
                    "from the orders table."
                ),
            },
        ]
        response = self._call("gpt-4o", messages)
        assert response.choices[0].message is not None
        assert len(self._spans("llm.openai.gpt-4o")) >= 1

    def test_architecture_recommendation(self):
        """Agent recommends architecture for a high-traffic API — span is captured."""
        messages = [
            {
                "role": "system",
                "content": "You are a principal cloud architect.",
            },
            {
                "role": "user",
                "content": (
                    "Recommend an architecture for a REST API that needs to handle "
                    "100,000 requests per second with 99.99% uptime."
                ),
            },
        ]
        self._call("gpt-4o", messages)
        assert len(self._spans("llm.openai.gpt-4o")) >= 1

    def test_email_drafting_agent(self):
        """Agent drafts a follow-up email after a sales call — verify content accessible."""
        messages = [
            {
                "role": "system",
                "content": "You are a sales assistant that drafts professional emails.",
            },
            {
                "role": "user",
                "content": (
                    "Draft a follow-up email to Acme Corp after our discovery call. "
                    "We discussed their data pipeline needs and our analytics platform."
                ),
            },
        ]
        response = self._call("gpt-4o", messages)
        assert response.choices[0].message.content is not None

    def test_security_vulnerability_analysis(self):
        """Agent analyses a code snippet for a path-traversal vulnerability — span emitted."""
        messages = [
            {
                "role": "system",
                "content": "You are a security engineer specialising in web vulnerabilities.",
            },
            {
                "role": "user",
                "content": (
                    "Analyse this Python snippet for path-traversal risk:\n"
                    "filename = request.args.get('file')\n"
                    "with open('/var/data/' + filename) as f:\n"
                    "    return f.read()\n"
                ),
            },
        ]
        self._call("gpt-4o", messages)
        spans = self._spans("llm.openai.gpt-4o")
        assert len(spans) >= 1
        attrs = dict(spans[0].attributes)
        assert attrs.get("gen_ai.system") == "openai"

    # ==================================================================
    # Group 6 — Error Handling
    # ==================================================================

    def test_401_unauthorized_still_emits_span(self):
        """A 401 response causes an exception but a span is still exported."""
        import openai
        import respx

        with respx.mock(assert_all_called=False) as mock_router:
            mock_router.post("https://api.openai.com/v1/chat/completions").mock(
                return_value=httpx.Response(
                    401,
                    json={
                        "error": {
                            "message": "Incorrect API key",
                            "type": "invalid_request_error",
                        }
                    },
                    headers={"content-type": "application/json"},
                )
            )
            with pytest.raises(Exception):
                openai.chat.completions.create(
                    model="gpt-4o",
                    messages=[{"role": "user", "content": "Auth check"}],
                )

        all_spans = _exporter.get_finished_spans()
        assert len(all_spans) >= 1

    def test_429_rate_limit_still_emits_span(self):
        """A 429 rate-limit response causes an exception but a span is still exported."""
        import openai
        import respx

        with respx.mock(assert_all_called=False) as mock_router:
            mock_router.post("https://api.openai.com/v1/chat/completions").mock(
                return_value=httpx.Response(
                    429,
                    json={
                        "error": {
                            "message": "Rate limit exceeded",
                            "type": "rate_limit_error",
                        }
                    },
                    headers={
                        "content-type": "application/json",
                        "x-ratelimit-limit-requests": "60",
                        "x-ratelimit-remaining-requests": "0",
                        "retry-after": "1",
                    },
                )
            )
            with pytest.raises(Exception):
                openai.chat.completions.create(
                    model="gpt-4o",
                    messages=[{"role": "user", "content": "Rate limit check"}],
                )

        all_spans = _exporter.get_finished_spans()
        assert len(all_spans) >= 1

    def test_500_server_error_still_emits_span(self):
        """A 500 server error causes an exception but a span is still exported."""
        import openai
        import respx

        with respx.mock(assert_all_called=False) as mock_router:
            mock_router.post("https://api.openai.com/v1/chat/completions").mock(
                return_value=httpx.Response(
                    500,
                    json={
                        "error": {
                            "message": "Internal server error",
                            "type": "server_error",
                        }
                    },
                    headers={"content-type": "application/json"},
                )
            )
            with pytest.raises(Exception):
                openai.chat.completions.create(
                    model="gpt-4o",
                    messages=[{"role": "user", "content": "Server error check"}],
                )

        all_spans = _exporter.get_finished_spans()
        assert len(all_spans) >= 1

    def test_response_object_not_corrupted(self):
        """
        The patched create must return the original response object without
        altering choices[0].message.content.
        """
        fake_content = "Unique content that must survive the patch unchanged."
        fake = _fake_response(model="gpt-4o", content=fake_content)

        import openai
        import respx

        with respx.mock(assert_all_called=False) as mock_router:
            mock_router.post("https://api.openai.com/v1/chat/completions").mock(
                return_value=httpx.Response(
                    200,
                    json=fake,
                    headers={"content-type": "application/json"},
                )
            )
            response = openai.chat.completions.create(
                model="gpt-4o",
                messages=[{"role": "user", "content": "Integrity check"}],
            )

        assert response.choices[0].message.content == fake_content

    def test_consecutive_calls_produce_isolated_spans(self):
        """
        Two consecutive calls — with a span clear between them — each produce
        exactly one span so spans are not accumulated or shared between calls.
        """
        # First call
        self._call("gpt-4o", [{"role": "user", "content": "First call"}])
        first_spans = self._spans("llm.openai.gpt-4o")
        assert len(first_spans) == 1, (
            f"Expected 1 span after first call, got {len(first_spans)}"
        )

        # Clear and make second call
        _exporter.clear()
        self._call("gpt-4o", [{"role": "user", "content": "Second call"}])
        second_spans = self._spans("llm.openai.gpt-4o")
        assert len(second_spans) == 1, (
            f"Expected 1 span after second call, got {len(second_spans)}"
        )
