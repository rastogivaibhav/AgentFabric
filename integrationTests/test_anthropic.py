"""
Govagn — Anthropic Integration Test Suite
===============================================

Tests the Govagn SDK's Anthropic patching layer using the real Anthropic
Python SDK with all HTTP traffic intercepted at the httpx transport layer via
`respx`. No real API key or outbound network access is required.

Scenarios covered
-----------------
Group 1  Basic agentic messages (6 tests)
Group 2  Different Claude model variants (4 tests)
Group 3  Span attribute verification (7 tests)
Group 4  Request feature coverage (5 tests)
Group 5  Real-world agentic use cases (6 tests)
Group 6  Error handling & edge cases (5 tests)

Install
-------
    pip install anthropic respx httpx opentelemetry-sdk opentelemetry-exporter-otlp

Run
---
    pytest integrationTests/test_anthropic.py -v -m integration
"""

from __future__ import annotations

import os
import sys

# ---------------------------------------------------------------------------
# Path bootstrap — must happen before any govagn / anthropic import
# ---------------------------------------------------------------------------
sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "agent-sdk"))
os.environ.setdefault("ANTHROPIC_API_KEY", "sk-ant-integration-test-placeholder")

import pytest
import httpx
import respx

# ---------------------------------------------------------------------------
# Optional-dependency guard — skip the whole module if libs are absent
# ---------------------------------------------------------------------------
anthropic = pytest.importorskip("anthropic")
respx = pytest.importorskip("respx")  # noqa: F811 (shadow import fine here)

import govagn  # noqa: E402

from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import SimpleSpanProcessor
from opentelemetry.sdk.trace.export.in_memory_span_exporter import InMemorySpanExporter

# ---------------------------------------------------------------------------
# Module-level helpers
# ---------------------------------------------------------------------------

_MESSAGES_URL = "https://api.anthropic.com/v1/messages"


def _fake_response(
    model: str = "claude-opus-4-6",
    content: str = "Response.",
    input_tokens: int = 8,
    output_tokens: int = 6,
    stop_reason: str = "end_turn",
) -> dict:
    """Return a minimal Anthropic Messages API response payload."""
    return {
        "id": "msg_integration_test",
        "type": "message",
        "role": "assistant",
        "content": [{"type": "text", "text": content}],
        "model": model,
        "stop_reason": stop_reason,
        "stop_sequence": None,
        "usage": {"input_tokens": input_tokens, "output_tokens": output_tokens},
    }


def _error_response(error_type: str = "authentication_error", message: str = "bad key") -> dict:
    return {"type": "error", "error": {"type": error_type, "message": message}}


# ---------------------------------------------------------------------------
# Test class
# ---------------------------------------------------------------------------

@pytest.mark.integration
class TestAnthropicIntegration:
    """Full integration suite for the Govagn Anthropic patch."""

    # ------------------------------------------------------------------
    # Fixtures
    # ------------------------------------------------------------------

    @pytest.fixture(autouse=True)
    def _require_deps(self):
        """Guard: skip if anthropic or respx are unavailable."""
        pytest.importorskip("anthropic")
        pytest.importorskip("respx")

    @pytest.fixture(scope="class", autouse=True)
    def tracer_setup(self, gateway_exporter):
        """
        Module-scoped fixture that:
        1. Creates an InMemorySpanExporter + TracerProvider.
        2. Injects the tracer into govagn globals.
        3. Applies the Anthropic class-level monkey-patch.
        4. Restores the original Messages.create on teardown.
        Spans are also forwarded to the Govagn dashboard in real-time.
        """
        exporter = InMemorySpanExporter()
        provider = TracerProvider()
        provider.add_span_processor(SimpleSpanProcessor(exporter))
        provider.add_span_processor(SimpleSpanProcessor(gateway_exporter))

        tracer = provider.get_tracer("govagn-test", "1.0.0")
        govagn._tracer = tracer
        govagn._initialized = True

        # Identify the Messages class BEFORE patching so we can restore it
        _dummy_client = anthropic.Anthropic(api_key="test")
        Messages = type(_dummy_client.messages)
        original_create = Messages.create

        govagn._patch_anthropic(anthropic)

        # Expose exporter to instance methods via class attribute
        self.__class__._exporter = exporter
        self.__class__._Messages = Messages
        self.__class__._original_create = original_create

        yield

        # Teardown — restore original method
        Messages.create = original_create
        govagn._initialized = False
        govagn._tracer = None

    @pytest.fixture(autouse=True)
    def _clear_spans(self):
        """Clear the in-memory span buffer before every test."""
        self._exporter.clear()
        yield

    # ------------------------------------------------------------------
    # Internal helpers
    # ------------------------------------------------------------------

    def _spans(self, fragment: str):
        """Return all finished spans whose name contains *fragment*."""
        return [
            s for s in self._exporter.get_finished_spans()
            if fragment in s.name
        ]

    @staticmethod
    def _call(
        model: str,
        messages: list,
        max_tokens: int = 64,
        **kwargs,
    ):
        """
        Create a fresh Anthropic client, mock the API endpoint with respx,
        and invoke messages.create().  Returns the SDK response object.
        """
        client = anthropic.Anthropic(api_key="test-key")
        fake = _fake_response(model=model)
        with respx.mock(assert_all_called=False) as mock:
            mock.post(_MESSAGES_URL).mock(
                return_value=httpx.Response(200, json=fake)
            )
            return client.messages.create(
                model=model,
                max_tokens=max_tokens,
                messages=messages,
                **kwargs,
            )

    # ------------------------------------------------------------------
    # Group 1 — Basic Agentic Messages
    # ------------------------------------------------------------------

    def test_basic_message_response(self):
        """User asks a simple question; verify the SDK response is accessible."""
        resp = self._call(
            model="claude-opus-4-6",
            messages=[{"role": "user", "content": "What is 2 + 2?"}],
        )
        assert resp is not None
        assert resp.role == "assistant"

    def test_code_analysis_request(self):
        """User asks Claude to analyse a function for bugs; verify response text."""
        content = "There is an off-by-one error on line 4."
        client = anthropic.Anthropic(api_key="test-key")
        fake = _fake_response(model="claude-opus-4-6", content=content)
        with respx.mock(assert_all_called=False) as mock:
            mock.post(_MESSAGES_URL).mock(return_value=httpx.Response(200, json=fake))
            resp = client.messages.create(
                model="claude-opus-4-6",
                max_tokens=128,
                messages=[
                    {
                        "role": "user",
                        "content": (
                            "Analyse the following Python function for bugs:\n"
                            "def add(a, b):\n    total = 0\n    for i in range(a + 1):\n"
                            "        total += b\n    return total"
                        ),
                    }
                ],
            )
        assert resp.content[0].text == content

    def test_document_summarisation(self):
        """User provides a paragraph and asks for a one-sentence summary."""
        summary = "Photosynthesis converts light energy into chemical energy stored in glucose."
        client = anthropic.Anthropic(api_key="test-key")
        fake = _fake_response(model="claude-opus-4-6", content=summary)
        with respx.mock(assert_all_called=False) as mock:
            mock.post(_MESSAGES_URL).mock(return_value=httpx.Response(200, json=fake))
            resp = client.messages.create(
                model="claude-opus-4-6",
                max_tokens=64,
                messages=[
                    {
                        "role": "user",
                        "content": (
                            "Summarise in one sentence: Plants absorb sunlight through "
                            "chlorophyll and convert it into glucose via photosynthesis, "
                            "releasing oxygen as a byproduct."
                        ),
                    }
                ],
            )
        assert resp.content[0].text == summary

    def test_creative_writing_prompt(self):
        """User asks for a haiku about distributed systems."""
        haiku = "Nodes fall one by one\nConsensus lost in the fog\nQuorum never reached"
        client = anthropic.Anthropic(api_key="test-key")
        fake = _fake_response(model="claude-opus-4-6", content=haiku)
        with respx.mock(assert_all_called=False) as mock:
            mock.post(_MESSAGES_URL).mock(return_value=httpx.Response(200, json=fake))
            resp = client.messages.create(
                model="claude-opus-4-6",
                max_tokens=64,
                messages=[{"role": "user", "content": "Write a haiku about distributed systems."}],
            )
        assert "Nodes" in resp.content[0].text or resp.content[0].text == haiku

    def test_technical_explanation(self):
        """User asks Claude to explain what a content delivery network is."""
        resp = self._call(
            model="claude-opus-4-6",
            messages=[{"role": "user", "content": "What is a content delivery network?"}],
        )
        # Span must have been emitted
        spans = self._spans("anthropic")
        assert len(spans) >= 1

    def test_step_by_step_reasoning(self):
        """User asks Claude to walk through solving a problem step by step."""
        resp = self._call(
            model="claude-opus-4-6",
            messages=[
                {
                    "role": "user",
                    "content": (
                        "Walk me through step by step how to reverse a linked list in Python."
                    ),
                }
            ],
        )
        assert resp.stop_reason == "end_turn"

    # ------------------------------------------------------------------
    # Group 2 — Different Claude Models
    # ------------------------------------------------------------------

    def test_claude_opus_span(self):
        """Span name contains the opus model identifier."""
        model = "claude-opus-4-6"
        self._call(model=model, messages=[{"role": "user", "content": "Hello."}])
        spans = self._spans(model)
        assert len(spans) == 1
        assert spans[0].name == f"llm.anthropic.{model}"

    def test_claude_sonnet_span(self):
        """Span name contains the sonnet model identifier."""
        model = "claude-sonnet-4-6"
        client = anthropic.Anthropic(api_key="test-key")
        fake = _fake_response(model=model)
        with respx.mock(assert_all_called=False) as mock:
            mock.post(_MESSAGES_URL).mock(return_value=httpx.Response(200, json=fake))
            client.messages.create(
                model=model,
                max_tokens=64,
                messages=[{"role": "user", "content": "Hello."}],
            )
        spans = self._spans(model)
        assert len(spans) == 1
        assert spans[0].name == f"llm.anthropic.{model}"

    def test_claude_haiku_span(self):
        """Span name contains the haiku model identifier."""
        model = "claude-haiku-4-5-20251001"
        client = anthropic.Anthropic(api_key="test-key")
        fake = _fake_response(model=model)
        with respx.mock(assert_all_called=False) as mock:
            mock.post(_MESSAGES_URL).mock(return_value=httpx.Response(200, json=fake))
            client.messages.create(
                model=model,
                max_tokens=64,
                messages=[{"role": "user", "content": "Hi."}],
            )
        spans = self._spans(model)
        assert len(spans) == 1
        assert spans[0].name == f"llm.anthropic.{model}"

    def test_claude_3_5_sonnet_span(self):
        """Span name is correctly formed for claude-3-5-sonnet-20241022."""
        model = "claude-3-5-sonnet-20241022"
        client = anthropic.Anthropic(api_key="test-key")
        fake = _fake_response(model=model)
        with respx.mock(assert_all_called=False) as mock:
            mock.post(_MESSAGES_URL).mock(return_value=httpx.Response(200, json=fake))
            client.messages.create(
                model=model,
                max_tokens=64,
                messages=[{"role": "user", "content": "Hello."}],
            )
        spans = self._spans(model)
        assert len(spans) == 1
        assert spans[0].name == f"llm.anthropic.{model}"

    # ------------------------------------------------------------------
    # Group 3 — Span Attribute Verification
    # ------------------------------------------------------------------

    def test_gen_ai_system_is_anthropic(self):
        """gen_ai.system attribute equals 'anthropic'."""
        self._call(
            model="claude-opus-4-6",
            messages=[{"role": "user", "content": "Ping."}],
        )
        spans = self._spans("anthropic")
        assert spans, "No span found"
        assert spans[0].attributes.get("gen_ai.system") == "anthropic"

    def test_gen_ai_request_model_matches(self):
        """gen_ai.request.model attribute matches the model passed to create()."""
        model = "claude-opus-4-6"
        self._call(model=model, messages=[{"role": "user", "content": "Ping."}])
        spans = self._spans(model)
        assert spans[0].attributes.get("gen_ai.request.model") == model

    def test_anthropic_model_attribute_matches(self):
        """anthropic.model attribute matches the model passed to create()."""
        model = "claude-sonnet-4-6"
        client = anthropic.Anthropic(api_key="test-key")
        fake = _fake_response(model=model)
        with respx.mock(assert_all_called=False) as mock:
            mock.post(_MESSAGES_URL).mock(return_value=httpx.Response(200, json=fake))
            client.messages.create(
                model=model,
                max_tokens=64,
                messages=[{"role": "user", "content": "Verify model attribute."}],
            )
        spans = self._spans(model)
        assert spans[0].attributes.get("anthropic.model") == model

    def test_input_tokens_captured(self):
        """gen_ai.usage.input_tokens equals the value from the fake response usage."""
        model = "claude-opus-4-6"
        client = anthropic.Anthropic(api_key="test-key")
        fake = _fake_response(model=model, input_tokens=42, output_tokens=10)
        with respx.mock(assert_all_called=False) as mock:
            mock.post(_MESSAGES_URL).mock(return_value=httpx.Response(200, json=fake))
            client.messages.create(
                model=model,
                max_tokens=128,
                messages=[{"role": "user", "content": "Count my tokens."}],
            )
        spans = self._spans("anthropic")
        assert spans[0].attributes.get("gen_ai.usage.input_tokens") == 42

    def test_output_tokens_captured(self):
        """gen_ai.usage.output_tokens equals the value from the fake response usage."""
        model = "claude-opus-4-6"
        client = anthropic.Anthropic(api_key="test-key")
        fake = _fake_response(model=model, input_tokens=5, output_tokens=99)
        with respx.mock(assert_all_called=False) as mock:
            mock.post(_MESSAGES_URL).mock(return_value=httpx.Response(200, json=fake))
            client.messages.create(
                model=model,
                max_tokens=128,
                messages=[{"role": "user", "content": "Output token count check."}],
            )
        spans = self._spans("anthropic")
        assert spans[0].attributes.get("gen_ai.usage.output_tokens") == 99

    def test_stop_reason_end_turn_in_span(self):
        """gen_ai.response.finish_reasons encodes stop_reason as a stringified list."""
        model = "claude-opus-4-6"
        client = anthropic.Anthropic(api_key="test-key")
        fake = _fake_response(model=model, stop_reason="end_turn")
        with respx.mock(assert_all_called=False) as mock:
            mock.post(_MESSAGES_URL).mock(return_value=httpx.Response(200, json=fake))
            client.messages.create(
                model=model,
                max_tokens=64,
                messages=[{"role": "user", "content": "Are you done?"}],
            )
        spans = self._spans("anthropic")
        finish = spans[0].attributes.get("gen_ai.response.finish_reasons")
        assert finish == str(["end_turn"])

    def test_api_version_attribute_is_2023_06_01(self):
        """anthropic.api_version attribute equals '2023-06-01'."""
        self._call(
            model="claude-opus-4-6",
            messages=[{"role": "user", "content": "API version check."}],
        )
        spans = self._spans("anthropic")
        assert spans[0].attributes.get("anthropic.api_version") == "2023-06-01"

    # ------------------------------------------------------------------
    # Group 4 — Request Features
    # ------------------------------------------------------------------

    def test_system_prompt_message(self):
        """A system param included in create() still produces a valid span."""
        model = "claude-opus-4-6"
        client = anthropic.Anthropic(api_key="test-key")
        fake = _fake_response(model=model)
        with respx.mock(assert_all_called=False) as mock:
            mock.post(_MESSAGES_URL).mock(return_value=httpx.Response(200, json=fake))
            resp = client.messages.create(
                model=model,
                max_tokens=128,
                system="You are a concise technical assistant.",
                messages=[{"role": "user", "content": "Explain idempotency."}],
            )
        spans = self._spans("anthropic")
        assert len(spans) == 1
        assert resp.role == "assistant"

    def test_multi_turn_conversation(self):
        """A multi-turn messages list (user/assistant/user) produces exactly one span."""
        model = "claude-opus-4-6"
        client = anthropic.Anthropic(api_key="test-key")
        fake = _fake_response(model=model)
        with respx.mock(assert_all_called=False) as mock:
            mock.post(_MESSAGES_URL).mock(return_value=httpx.Response(200, json=fake))
            client.messages.create(
                model=model,
                max_tokens=128,
                messages=[
                    {"role": "user", "content": "What is observability?"},
                    {"role": "assistant", "content": "Observability is the ability to infer internal state from outputs."},
                    {"role": "user", "content": "Can you give a practical example?"},
                ],
            )
        spans = self._spans("anthropic")
        assert len(spans) == 1

    def test_max_tokens_param_in_span(self):
        """gen_ai.request.max_tokens in the span is the string form of the value passed."""
        model = "claude-opus-4-6"
        client = anthropic.Anthropic(api_key="test-key")
        fake = _fake_response(model=model)
        with respx.mock(assert_all_called=False) as mock:
            mock.post(_MESSAGES_URL).mock(return_value=httpx.Response(200, json=fake))
            client.messages.create(
                model=model,
                max_tokens=256,
                messages=[{"role": "user", "content": "Token limit test."}],
            )
        spans = self._spans("anthropic")
        assert spans[0].attributes.get("gen_ai.request.max_tokens") == "256"

    def test_temperature_zero(self):
        """Passing temperature=0.0 does not break span creation."""
        model = "claude-opus-4-6"
        client = anthropic.Anthropic(api_key="test-key")
        fake = _fake_response(model=model)
        with respx.mock(assert_all_called=False) as mock:
            mock.post(_MESSAGES_URL).mock(return_value=httpx.Response(200, json=fake))
            client.messages.create(
                model=model,
                max_tokens=64,
                temperature=0.0,
                messages=[{"role": "user", "content": "Zero temperature test."}],
            )
        spans = self._spans("anthropic")
        assert len(spans) == 1

    def test_stop_sequences_param(self):
        """Passing stop_sequences does not break span creation."""
        model = "claude-opus-4-6"
        client = anthropic.Anthropic(api_key="test-key")
        fake = _fake_response(model=model, stop_reason="stop_sequence")
        with respx.mock(assert_all_called=False) as mock:
            mock.post(_MESSAGES_URL).mock(return_value=httpx.Response(200, json=fake))
            client.messages.create(
                model=model,
                max_tokens=64,
                stop_sequences=["END"],
                messages=[{"role": "user", "content": "Stop sequence test."}],
            )
        spans = self._spans("anthropic")
        assert len(spans) == 1

    # ------------------------------------------------------------------
    # Group 5 — Real-World Agentic Use Cases
    # ------------------------------------------------------------------

    def test_legal_clause_analysis(self):
        """Agent analyses an indemnification clause for legal risks."""
        content = (
            "The clause creates unlimited liability exposure for the licensee. "
            "Key risk: no cap on indemnification obligations."
        )
        model = "claude-opus-4-6"
        client = anthropic.Anthropic(api_key="test-key")
        fake = _fake_response(model=model, content=content, input_tokens=120, output_tokens=45)
        with respx.mock(assert_all_called=False) as mock:
            mock.post(_MESSAGES_URL).mock(return_value=httpx.Response(200, json=fake))
            resp = client.messages.create(
                model=model,
                max_tokens=512,
                system="You are a legal analysis assistant specialising in contract review.",
                messages=[
                    {
                        "role": "user",
                        "content": (
                            "Analyse the following indemnification clause for risks: "
                            "'Licensee shall indemnify, defend, and hold harmless Licensor "
                            "from any and all claims, damages, losses, costs, and expenses "
                            "arising from Licensee's use of the Software.'"
                        ),
                    }
                ],
            )
        assert resp.content[0].text == content
        spans = self._spans("anthropic")
        assert spans[0].attributes.get("gen_ai.usage.input_tokens") == 120

    def test_code_generation_task(self):
        """Agent generates a Python function to parse CSV files."""
        content = "def parse_csv(path):\n    import csv\n    with open(path) as f:\n        return list(csv.DictReader(f))"
        model = "claude-sonnet-4-6"
        client = anthropic.Anthropic(api_key="test-key")
        fake = _fake_response(model=model, content=content, input_tokens=30, output_tokens=50)
        with respx.mock(assert_all_called=False) as mock:
            mock.post(_MESSAGES_URL).mock(return_value=httpx.Response(200, json=fake))
            resp = client.messages.create(
                model=model,
                max_tokens=256,
                messages=[
                    {
                        "role": "user",
                        "content": "Write a Python function that parses a CSV file and returns a list of dicts.",
                    }
                ],
            )
        assert "def parse_csv" in resp.content[0].text
        spans = self._spans("anthropic")
        assert spans[0].attributes.get("gen_ai.usage.output_tokens") == 50

    def test_research_summarisation(self):
        """Agent summarises a research abstract on AI safety."""
        summary = (
            "The paper proposes a scalable oversight technique that uses AI assistance "
            "to evaluate complex tasks beyond human ability to verify directly."
        )
        model = "claude-opus-4-6"
        client = anthropic.Anthropic(api_key="test-key")
        fake = _fake_response(model=model, content=summary, input_tokens=200, output_tokens=55)
        with respx.mock(assert_all_called=False) as mock:
            mock.post(_MESSAGES_URL).mock(return_value=httpx.Response(200, json=fake))
            resp = client.messages.create(
                model=model,
                max_tokens=256,
                system="You are a research summarisation assistant.",
                messages=[
                    {
                        "role": "user",
                        "content": (
                            "Summarise this abstract: 'We present a method for scalable "
                            "oversight of AI systems using AI-assisted evaluation. Our approach "
                            "allows human supervisors to leverage AI to help evaluate tasks that "
                            "are too complex for unaided human verification, addressing a key "
                            "challenge in AI safety research.'"
                        ),
                    }
                ],
            )
        assert resp.content[0].text == summary
        spans = self._spans("anthropic")
        assert len(spans) == 1

    def test_security_vulnerability_review(self):
        """Agent reviews authentication code for vulnerabilities."""
        finding = (
            "The code stores passwords in plain text and uses a predictable session token. "
            "Critical: replace with bcrypt hashing and cryptographically secure token generation."
        )
        model = "claude-opus-4-6"
        client = anthropic.Anthropic(api_key="test-key")
        fake = _fake_response(model=model, content=finding, input_tokens=90, output_tokens=60)
        with respx.mock(assert_all_called=False) as mock:
            mock.post(_MESSAGES_URL).mock(return_value=httpx.Response(200, json=fake))
            resp = client.messages.create(
                model=model,
                max_tokens=512,
                system="You are a security code review assistant.",
                messages=[
                    {
                        "role": "user",
                        "content": (
                            "Review this authentication code for vulnerabilities:\n"
                            "def login(user, password):\n"
                            "    stored = db.get(user)\n"
                            "    if stored['password'] == password:\n"
                            "        return 'token_' + str(user)\n"
                        ),
                    }
                ],
            )
        assert resp.content[0].text == finding

    def test_incident_postmortem_writing(self):
        """Agent writes a postmortem for a database outage."""
        postmortem = (
            "## Incident Postmortem\n"
            "**Impact**: 45 minutes of read unavailability.\n"
            "**Root Cause**: Unindexed query under high load caused full table scan.\n"
            "**Action Items**: Add composite index; introduce slow-query alerting."
        )
        model = "claude-opus-4-6"
        client = anthropic.Anthropic(api_key="test-key")
        fake = _fake_response(model=model, content=postmortem, input_tokens=60, output_tokens=80)
        with respx.mock(assert_all_called=False) as mock:
            mock.post(_MESSAGES_URL).mock(return_value=httpx.Response(200, json=fake))
            resp = client.messages.create(
                model=model,
                max_tokens=512,
                system="You are a site reliability engineering assistant.",
                messages=[
                    {
                        "role": "user",
                        "content": (
                            "Write a postmortem for the following incident: "
                            "The primary database became unresponsive for 45 minutes on 2026-03-10 "
                            "due to an unindexed query introduced in a recent deployment."
                        ),
                    }
                ],
            )
        assert "Postmortem" in resp.content[0].text or resp.content[0].text == postmortem
        spans = self._spans("anthropic")
        assert spans[0].attributes.get("gen_ai.request.model") == model

    def test_product_requirements_writing(self):
        """Agent drafts a user story for a new feature."""
        user_story = (
            "As a platform administrator, I want to export audit logs as CSV "
            "so that I can provide compliance evidence to auditors without granting "
            "them direct database access."
        )
        model = "claude-sonnet-4-6"
        client = anthropic.Anthropic(api_key="test-key")
        fake = _fake_response(model=model, content=user_story, input_tokens=50, output_tokens=55)
        with respx.mock(assert_all_called=False) as mock:
            mock.post(_MESSAGES_URL).mock(return_value=httpx.Response(200, json=fake))
            resp = client.messages.create(
                model=model,
                max_tokens=256,
                system="You are a product management assistant.",
                messages=[
                    {
                        "role": "user",
                        "content": (
                            "Draft a user story for a feature that allows administrators "
                            "to export audit logs as CSV files for compliance reporting."
                        ),
                    }
                ],
            )
        assert resp.content[0].text == user_story
        spans = self._spans("anthropic")
        assert spans[0].attributes.get("anthropic.model") == model

    # ------------------------------------------------------------------
    # Group 6 — Error Handling
    # ------------------------------------------------------------------

    def test_401_auth_error_still_emits_span(self):
        """A 401 auth error causes the call to raise but a span is still exported."""
        model = "claude-opus-4-6"
        client = anthropic.Anthropic(api_key="bad-key")
        with respx.mock(assert_all_called=False) as mock:
            mock.post(_MESSAGES_URL).mock(
                return_value=httpx.Response(
                    401,
                    json=_error_response("authentication_error", "Invalid API key"),
                )
            )
            with pytest.raises(Exception):
                client.messages.create(
                    model=model,
                    max_tokens=64,
                    messages=[{"role": "user", "content": "This should fail."}],
                )
        all_spans = self._exporter.get_finished_spans()
        assert len(all_spans) >= 1, "Span must be emitted even on auth error"

    def test_529_overloaded_still_emits_span(self):
        """A 529 overloaded response causes the call to raise but a span is still exported."""
        model = "claude-opus-4-6"
        client = anthropic.Anthropic(api_key="test-key")
        with respx.mock(assert_all_called=False) as mock:
            mock.post(_MESSAGES_URL).mock(
                return_value=httpx.Response(
                    529,
                    json=_error_response("overloaded_error", "Service temporarily overloaded"),
                )
            )
            with pytest.raises(Exception):
                client.messages.create(
                    model=model,
                    max_tokens=64,
                    messages=[{"role": "user", "content": "Try during overload."}],
                )
        all_spans = self._exporter.get_finished_spans()
        assert len(all_spans) >= 1, "Span must be emitted even on 529"

    def test_500_server_error_still_emits_span(self):
        """A 500 internal server error causes the call to raise but a span is still exported."""
        model = "claude-opus-4-6"
        client = anthropic.Anthropic(api_key="test-key")
        with respx.mock(assert_all_called=False) as mock:
            mock.post(_MESSAGES_URL).mock(
                return_value=httpx.Response(
                    500,
                    json=_error_response("api_error", "Internal server error"),
                )
            )
            with pytest.raises(Exception):
                client.messages.create(
                    model=model,
                    max_tokens=64,
                    messages=[{"role": "user", "content": "Server error test."}],
                )
        all_spans = self._exporter.get_finished_spans()
        assert len(all_spans) >= 1, "Span must be emitted even on 500"

    def test_response_content_not_corrupted(self):
        """The patched Messages.create returns content identical to the mocked payload."""
        expected_text = "The mitochondria is the powerhouse of the cell."
        model = "claude-opus-4-6"
        client = anthropic.Anthropic(api_key="test-key")
        fake = _fake_response(model=model, content=expected_text)
        with respx.mock(assert_all_called=False) as mock:
            mock.post(_MESSAGES_URL).mock(return_value=httpx.Response(200, json=fake))
            resp = client.messages.create(
                model=model,
                max_tokens=64,
                messages=[{"role": "user", "content": "What is the powerhouse of the cell?"}],
            )
        assert resp.content[0].text == expected_text
        assert resp.role == "assistant"
        assert resp.stop_reason == "end_turn"

    def test_consecutive_calls_produce_isolated_spans(self):
        """Two sequential calls with a span-clear between them each produce exactly one span."""
        model = "claude-opus-4-6"

        # First call
        client = anthropic.Anthropic(api_key="test-key")
        fake = _fake_response(model=model)
        with respx.mock(assert_all_called=False) as mock:
            mock.post(_MESSAGES_URL).mock(return_value=httpx.Response(200, json=fake))
            client.messages.create(
                model=model,
                max_tokens=64,
                messages=[{"role": "user", "content": "First call."}],
            )
        first_spans = self._exporter.get_finished_spans()
        assert len(first_spans) == 1

        # Clear and second call
        self._exporter.clear()
        client2 = anthropic.Anthropic(api_key="test-key")
        with respx.mock(assert_all_called=False) as mock:
            mock.post(_MESSAGES_URL).mock(return_value=httpx.Response(200, json=fake))
            client2.messages.create(
                model=model,
                max_tokens=64,
                messages=[{"role": "user", "content": "Second call."}],
            )
        second_spans = self._exporter.get_finished_spans()
        assert len(second_spans) == 1
        assert second_spans[0].name == f"llm.anthropic.{model}"
