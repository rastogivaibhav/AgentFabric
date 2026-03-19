"""
AgentFabric real agent runner.
Runs LangGraph, CrewAI, OpenAI, and Anthropic agents (LLM calls mocked via respx)
and exports every span directly to the mock gateway at http://localhost:8080.

    python run_agents.py
"""
from __future__ import annotations
import json, os, sys, time, urllib.request

# ensure local SDK package takes precedence over any installed version
_SDK = os.path.join(os.path.dirname(__file__), "agent-sdk")
if _SDK not in sys.path:
    sys.path.insert(0, _SDK)

os.environ.setdefault("OPENAI_API_KEY",    "sk-run-agents-placeholder")
os.environ.setdefault("ANTHROPIC_API_KEY", "sk-ant-run-agents-placeholder")

GATEWAY = "http://localhost:8080"

# ─── Custom exporter: OTel spans -> mock gateway /internal/ingest ─────────────

from opentelemetry.sdk.trace.export import SpanExporter, SpanExportResult

class MockGatewayExporter(SpanExporter):
    def export(self, spans):
        payload = []
        for s in spans:
            attrs = dict(s.attributes or {})
            dur   = s.end_time - s.start_time if s.end_time and s.start_time else 0
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
            req  = urllib.request.Request(
                f"{GATEWAY}/internal/ingest", data=data,
                headers={"Content-Type": "application/json"}, method="POST",
            )
            urllib.request.urlopen(req, timeout=5)
        except Exception as e:
            print(f"  [exporter warn] {e}")
        return SpanExportResult.SUCCESS

    def shutdown(self): pass


# ─── Bootstrap: provider + patch all frameworks ───────────────────────────────

from opentelemetry import trace
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import SimpleSpanProcessor
import agentfabric

# Set provider once before any OTel usage; suppress the "not allowed" warning
# by only setting when the current provider is a no-op proxy.
_current = trace.get_tracer_provider()
if type(_current).__name__ in ("ProxyTracerProvider", "NoOpTracerProvider", "_DefaultTracerProvider"):
    _provider = TracerProvider()
    _provider.add_span_processor(SimpleSpanProcessor(MockGatewayExporter()))
    trace.set_tracer_provider(_provider)
else:
    _provider = _current
    _provider.add_span_processor(SimpleSpanProcessor(MockGatewayExporter()))
agentfabric._tracer      = trace.get_tracer("agentfabric", "1.0.0")
agentfabric._initialized = True

# Patch all available frameworks up front
try:
    import langgraph as _lg; agentfabric._patch_langgraph(_lg); print("[setup] langgraph patched")
except ImportError: pass

try:
    import crewai as _ca; agentfabric._patch_crewai(_ca); print("[setup] crewai patched")
except ImportError: pass

try:
    import openai as _oa; agentfabric._patch_openai(_oa); print("[setup] openai patched")
except ImportError: pass

try:
    import anthropic as _an; agentfabric._patch_anthropic(_an); print("[setup] anthropic patched")
except ImportError: pass

print()


# ─── LangGraph runs ──────────────────────────────────────────────────────────

def run_langgraph():
    from langgraph.graph import StateGraph, START, END

    def make_graph(nodes: dict):
        b = StateGraph(dict)
        for n, fn in nodes.items():
            b.add_node(n, fn)
        names = list(nodes)
        b.add_edge(START, names[0])
        for a, b_ in zip(names, names[1:]):
            b.add_edge(a, b_)
        b.add_edge(names[-1], END)
        return b.compile()

    research = make_graph({
        "retrieve":  lambda s: {**s, "docs": ["doc1", "doc2", "doc3"]},
        "summarise": lambda s: {**s, "summary": "Summarised: " + str(s.get("docs"))},
        "write":     lambda s: {**s, "output":  "Report ready"},
    })
    r = research.invoke({"query": "AgentFabric observability"})
    print(f"  research-pipeline  output={r['output']}")

    qa = make_graph({
        "load":   lambda s: {**s, "chunks": ["chunk-a", "chunk-b"]},
        "embed":  lambda s: {**s, "vectors": [0.1, 0.2]},
        "rank":   lambda s: {**s, "ranked": s["chunks"][:1]},
        "answer": lambda s: {**s, "answer":  "chunk-a"},
    })
    r = qa.invoke({"question": "What is tracing?"})
    print(f"  qa-pipeline        answer={r['answer']}")

    import asyncio
    async def async_step(state):
        await asyncio.sleep(0)
        return {**state, "processed": True}

    pipeline = make_graph({"process": async_step, "finalize": lambda s: s})
    r = asyncio.run(pipeline.ainvoke({"data": "input"}))
    print(f"  async-pipeline     processed={r.get('processed')}")


# ─── CrewAI runs ─────────────────────────────────────────────────────────────

def run_crewai():
    import crewai, respx, httpx

    FAKE_RESP = {
        "id": "chatcmpl-crew", "object": "chat.completion",
        "created": int(time.time()), "model": "gpt-4o",
        "choices": [{"index": 0, "finish_reason": "stop", "logprobs": None,
                     "message": {"role": "assistant", "content": "Task complete."}}],
        "usage": {"prompt_tokens": 25, "completion_tokens": 10, "total_tokens": 35},
        "system_fingerprint": None,
    }

    for role, goal, desc in [
        ("Researcher",    "Find information",    "Research AI observability trends"),
        ("Analyst",       "Analyse findings",    "Analyse collected research data"),
        ("Report Writer", "Write final report",  "Compile analysis into a report"),
    ]:
        try:
            agent = crewai.Agent(role=role, goal=goal, backstory="A test agent",
                                 llm="gpt-4o", allow_delegation=False)
            task  = crewai.Task(description=desc, expected_output="Done", agent=agent)
            with respx.mock(assert_all_called=False) as rmock:
                rmock.post("https://api.openai.com/v1/chat/completions").mock(
                    return_value=httpx.Response(200, json=FAKE_RESP,
                                                headers={"content-type": "application/json"})
                )
                try:
                    agent.execute_task(task)
                except Exception:
                    pass   # span already exported before LLM call
            print(f"  {role}")
        except Exception as e:
            print(f"  {role} skipped: {e}")


# ─── OpenAI runs ─────────────────────────────────────────────────────────────

def run_openai():
    import openai, respx, httpx

    for model, prompt in [
        ("gpt-4o",      "Classify this support ticket: app crashes on login"),
        ("gpt-4o-mini", "Summarise: AgentFabric tracks AI agent runs end-to-end"),
        ("gpt-4o",      "Extract action items from: deploy by Friday, test Monday"),
    ]:
        fake = {
            "id": "chatcmpl-oa", "object": "chat.completion",
            "created": int(time.time()), "model": model,
            "choices": [{"index": 0, "finish_reason": "stop", "logprobs": None,
                         "message": {"role": "assistant", "content": "Done."}}],
            "usage": {"prompt_tokens": 18, "completion_tokens": 5, "total_tokens": 23},
            "system_fingerprint": None,
        }
        with respx.mock(assert_all_called=False) as rmock:
            rmock.post("https://api.openai.com/v1/chat/completions").mock(
                return_value=httpx.Response(200, json=fake,
                                            headers={"content-type": "application/json"})
            )
            openai.chat.completions.create(model=model,
                                           messages=[{"role": "user", "content": prompt}])
        print(f"  {model}")


# ─── Anthropic runs ──────────────────────────────────────────────────────────

def run_anthropic():
    import anthropic, respx, httpx
    client = anthropic.Anthropic(api_key="test")

    for model, prompt in [
        ("claude-sonnet-4-6", "Describe the purpose of distributed tracing"),
        ("claude-opus-4-6",   "Explain OpenTelemetry in two sentences"),
        ("claude-haiku-4-5",  "What is a span in observability?"),
    ]:
        fake = {
            "id": "msg_run", "type": "message", "role": "assistant",
            "content": [{"type": "text", "text": "Understood."}],
            "model": model, "stop_reason": "end_turn", "stop_sequence": None,
            "usage": {"input_tokens": 14, "output_tokens": 9},
        }
        with respx.mock(assert_all_called=False) as rmock:
            rmock.post("https://api.anthropic.com/v1/messages").mock(
                return_value=httpx.Response(200, json=fake,
                                            headers={"content-type": "application/json"})
            )
            client.messages.create(model=model, max_tokens=64,
                                   messages=[{"role": "user", "content": prompt}])
        print(f"  {model}")


# ─── Main ─────────────────────────────────────────────────────────────────────

if __name__ == "__main__":
    print("== LangGraph " + "=" * 50)
    run_langgraph()

    print("\n== CrewAI " + "=" * 53)
    run_crewai()

    print("\n== OpenAI " + "=" * 53)
    run_openai()

    print("\n== Anthropic " + "=" * 50)
    run_anthropic()

    time.sleep(0.5)  # let SimpleSpanProcessor flush

    r     = urllib.request.urlopen(f"{GATEWAY}/api/v1/analytics/overview", timeout=5)
    stats = json.loads(r.read())
    print("\n== Portal stats after run " + "=" * 37)
    print(f"  Total traces : {stats['total_traces']}")
    print(f"  Active agents: {stats['active_agents']}")
    print(f"  Frameworks   : {stats['framework_counts']}")
    print(f"  Total tokens : {stats['total_tokens']}")
    print(f"  Error rate   : {stats['error_rate']}%")
    print("=" * 62)
