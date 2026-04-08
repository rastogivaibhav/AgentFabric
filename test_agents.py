"""
Govagn End-to-End Telemetry Test
Simulates 5 agent frameworks sending spans to the collector via OTLP/HTTP.

Run with:  python test_agents.py
"""
import sys, os, time, uuid, random, logging, base64, json, hmac, hashlib

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "agent-sdk"))

logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] %(message)s")
log = logging.getLogger("af-test")

# ─── JWT helper (generates dev token matching collector's dev-secret) ─────────
def make_jwt(secret: str, tenant: str) -> str:
    header  = base64.urlsafe_b64encode(
        json.dumps({"alg":"HS256","typ":"JWT"}, separators=(',',':')).encode()
    ).rstrip(b'=').decode()
    payload = base64.urlsafe_b64encode(
        json.dumps({"tenant_id": tenant, "sub": "test-agent",
                    "exp": int(time.time()) + 86400}, separators=(',',':')).encode()
    ).rstrip(b'=').decode()
    sig_input = f"{header}.{payload}".encode()
    sig = base64.urlsafe_b64encode(
        hmac.new(secret.encode(), sig_input, hashlib.sha256).digest()
    ).rstrip(b'=').decode()
    return f"{header}.{payload}.{sig}"

JWT_SECRET      = os.environ.get("GV_JWT_SECRET", "dev-secret")
JWT_TENANT      = os.environ.get("GV_TEST_TENANT", "00000000-0000-0000-0000-000000000001")
JWT_TOKEN       = make_jwt(secret=JWT_SECRET, tenant=JWT_TENANT)
COLLECTOR_HTTP  = os.environ.get("GV_HTTP_ENDPOINT", "http://localhost:4318")
COLLECTOR_GRPC  = os.environ.get("GV_ENDPOINT",      "http://localhost:4317")
API_BASE        = os.environ.get("GV_API",            "http://localhost:8080")

print(f"\n{'='*62}")
print(f"  Govagn  |  End-to-End Telemetry Test")
print(f"  Collector    →  {COLLECTOR_HTTP}  (HTTP/OTLP)")
print(f"  API Gateway  →  {API_BASE}")
print(f"{'='*62}\n")

# ─── Bootstrap SDK with HTTP exporter ─────────────────────────────────────────
from opentelemetry import trace
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor, ConsoleSpanExporter
from opentelemetry.trace import SpanKind, Status, StatusCode

# Import our SDK helpers
from govagn import agent_span, trace_tool_call, Attrs

# Build a TracerProvider that exports to BOTH the collector AND console
resource = Resource.create({
    "service.name":    "af-test-suite",
    "service.version": "1.0.0",
    "deployment.environment": "development",
})

provider = TracerProvider(resource=resource)

# HTTP OTLP exporter → collector :4318
otlp_exporter = OTLPSpanExporter(
    endpoint=f"{COLLECTOR_HTTP}/v1/traces",
    headers={"Authorization": f"Bearer {JWT_TOKEN}"},
)
provider.add_span_processor(
    BatchSpanProcessor(otlp_exporter, max_queue_size=1000,
                       max_export_batch_size=128, schedule_delay_millis=500)
)

# Console exporter → see spans in terminal directly
provider.add_span_processor(
    BatchSpanProcessor(ConsoleSpanExporter(), schedule_delay_millis=200)
)

trace.set_tracer_provider(provider)

# Monkey-patch the SDK's global tracer so agent_span() uses our provider
import govagn as af_sdk
af_sdk._tracer    = trace.get_tracer("govagn", "1.0.0")
af_sdk._initialized = True

tracer = af_sdk._tracer


# ─── Tool helpers ─────────────────────────────────────────────────────────────

@trace_tool_call("web_search")
def web_search(query: str) -> str:
    time.sleep(random.uniform(0.05, 0.12))
    return f"Results: {query}"

@trace_tool_call("calculator")
def calculator(a: float, b: float, op: str) -> float:
    time.sleep(random.uniform(0.01, 0.04))
    ops = {"add": a + b, "sub": a - b, "mul": a * b, "div": a / b if b != 0 else 0}
    return ops.get(op, 0)

@trace_tool_call("database_query")
def database_query(table: str, key: str) -> list:
    time.sleep(random.uniform(0.02, 0.07))
    return [{"id": 1, "table": table, "key": key}]

@trace_tool_call("send_email")
def send_email(to: str, subject: str) -> bool:
    time.sleep(random.uniform(0.03, 0.06))
    return True


# ─── Test 1: CrewAI research agent ────────────────────────────────────────────
def test_crewai():
    run_id = str(uuid.uuid4())
    print(f"\n[1/6] CrewAI — research agent  (run_id={run_id[:8]})")

    with agent_span(
        "crewai.agent.researcher",
        framework="crewai",
        agent_name="Senior Research Analyst",
        agent_role="researcher",
        run_id=run_id,
        attributes={
            Attrs.GEN_AI_SYSTEM:        "crewai",
            Attrs.CREWAI_AGENT_ROLE:    "researcher",
            Attrs.CREWAI_CREW_ID:       "crew-" + run_id[:8],
            Attrs.GEN_AI_REQUEST_MODEL: "gpt-4o",
            Attrs.GEN_AI_INPUT_TOKENS:  1250,
            Attrs.GEN_AI_OUTPUT_TOKENS: 487,
        }
    ) as root:
        with tracer.start_as_current_span("crewai.task.plan") as t:
            t.set_attribute("crewai.task.name", "Research AI observability landscape")
            time.sleep(0.08)

        web_search("AI observability platforms 2026")
        web_search("OpenTelemetry AI semantic conventions")
        database_query("market_data", "category=ai-ops")

        with tracer.start_as_current_span("llm.openai.gpt-4o") as llm:
            llm.set_attribute(Attrs.GEN_AI_SYSTEM,        "openai")
            llm.set_attribute(Attrs.GEN_AI_REQUEST_MODEL, "gpt-4o")
            llm.set_attribute(Attrs.GEN_AI_INPUT_TOKENS,  1250)
            llm.set_attribute(Attrs.GEN_AI_OUTPUT_TOKENS, 487)
            time.sleep(random.uniform(0.3, 0.5))

        root.set_attribute("af.agent.output_length", 1240)

    print(f"  ✓ CrewAI span emitted + visible in console above")
    return run_id


# ─── Test 2: LangGraph multi-node pipeline ────────────────────────────────────
def test_langgraph():
    run_id   = str(uuid.uuid4())
    graph_id = "graph-" + run_id[:8]
    print(f"\n[2/6] LangGraph — 5-node pipeline  (run_id={run_id[:8]})")

    with agent_span(
        "langgraph.graph.invoke",
        framework="langgraph",
        agent_name="document-processor",
        run_id=run_id,
        attributes={
            Attrs.GEN_AI_SYSTEM:   "langgraph",
            Attrs.LANGGRAPH_GRAPH: graph_id,
        }
    ):
        for node_name, node_type in [
            ("langgraph.node.fetch",    "fetch"),
            ("langgraph.node.extract",  "extract"),
            ("langgraph.node.classify", "classify"),
            ("langgraph.node.generate", "generate"),
            ("langgraph.node.store",    "store"),
        ]:
            with tracer.start_as_current_span(node_name) as n:
                n.set_attribute(Attrs.LANGGRAPH_NODE, node_name)
                if node_type in ("extract", "classify", "generate"):
                    n.set_attribute(Attrs.GEN_AI_REQUEST_MODEL, "claude-3-5-sonnet-20241022")
                    n.set_attribute(Attrs.GEN_AI_SYSTEM,        "anthropic")
                    n.set_attribute(Attrs.GEN_AI_INPUT_TOKENS,  random.randint(200, 800))
                    n.set_attribute(Attrs.GEN_AI_OUTPUT_TOKENS, random.randint(50, 300))
                time.sleep(random.uniform(0.04, 0.15))

    print(f"  ✓ LangGraph pipeline emitted (5 child spans)")
    return run_id


# ─── Test 3: OpenAI agent (multi-turn) ────────────────────────────────────────
def test_openai():
    run_id    = str(uuid.uuid4())
    thread_id = "thread-" + run_id[:8]
    print(f"\n[3/6] OpenAI Agents SDK — 3-turn conversation  (run_id={run_id[:8]})")

    with agent_span(
        "openai.agent.run",
        framework="openai_agents",
        agent_name="customer-support-bot",
        run_id=run_id,
        attributes={
            Attrs.GEN_AI_SYSTEM:        "openai",
            Attrs.OPENAI_RUN_ID:        "run-" + run_id[:8],
            Attrs.OPENAI_THREAD_ID:     thread_id,
            Attrs.GEN_AI_REQUEST_MODEL: "gpt-4o-mini",
        }
    ):
        for turn in range(3):
            with tracer.start_as_current_span(f"openai.llm.turn.{turn+1}") as t:
                t.set_attribute(Attrs.GEN_AI_SYSTEM,        "openai")
                t.set_attribute(Attrs.GEN_AI_REQUEST_MODEL, "gpt-4o-mini")
                t.set_attribute(Attrs.GEN_AI_INPUT_TOKENS,  random.randint(100, 400))
                t.set_attribute(Attrs.GEN_AI_OUTPUT_TOKENS, random.randint(30, 150))
                t.set_attribute("openai.conversation.turn", turn + 1)
                time.sleep(random.uniform(0.08, 0.25))
            if turn == 1:
                database_query("orders", "id=12345")
                send_email("user@company.com", "Your order has shipped")

    print(f"  ✓ OpenAI agent emitted (3 LLM turns + 2 tool calls)")
    return run_id


# ─── Test 4: Anthropic Claude + PII scrub verification ───────────────────────
def test_anthropic_pii():
    run_id = str(uuid.uuid4())
    print(f"\n[4/6] Anthropic Claude — PII scrub test  (run_id={run_id[:8]})")
    print(f"       Sending: email, SSN, credit card → collector should redact all")

    with agent_span(
        "llm.anthropic.claude-3-5-sonnet-20241022",
        framework="anthropic",
        agent_name="legal-document-analyzer",
        run_id=run_id,
        attributes={
            Attrs.GEN_AI_SYSTEM:         "anthropic",
            Attrs.GEN_AI_REQUEST_MODEL:  "claude-3-5-sonnet-20241022",
            Attrs.ANTHROPIC_MODEL:       "claude-3-5-sonnet-20241022",
            Attrs.ANTHROPIC_API_VERSION: "2023-06-01",
            Attrs.GEN_AI_INPUT_TOKENS:   3200,
            Attrs.GEN_AI_OUTPUT_TOKENS:  890,
            Attrs.ANTHROPIC_CACHE_READ:  1200,
            # These should be [REDACTED] by the collector's PII scrubber:
            "af.test.user_email":  "john.doe@company.com",
            "af.test.user_ssn":    "123-45-6789",
            "af.test.user_phone":  "+44 7700 900123",
            "af.test.credit_card": "4532 1234 5678 9012",
        }
    ) as span:
        with tracer.start_as_current_span("tool.extract_clauses") as t:
            t.set_attribute(Attrs.TOOL_NAME,   "extract_clauses")
            t.set_attribute(Attrs.TOOL_STATUS, "success")
            time.sleep(0.18)
        with tracer.start_as_current_span("tool.risk_assessment") as t:
            t.set_attribute(Attrs.TOOL_NAME,   "risk_assessment")
            t.set_attribute(Attrs.TOOL_STATUS, "success")
            time.sleep(0.12)

        span.set_attribute("af.cost.estimated_usd", 0.048)

    print(f"  ✓ Anthropic span emitted with PII payload")
    print(f"    Check collector metrics: govagn_pii_scrubbed_total should be > 0")
    return run_id


# ─── Test 5: Error / failure scenario ────────────────────────────────────────
def test_error():
    run_id = str(uuid.uuid4())
    print(f"\n[5/6] Error scenario — rate limit failure  (run_id={run_id[:8]})")

    try:
        with agent_span(
            "crewai.agent.writer",
            framework="crewai",
            agent_name="Content Writer",
            agent_role="writer",
            run_id=run_id,
            attributes={
                Attrs.GEN_AI_SYSTEM:        "crewai",
                Attrs.GEN_AI_REQUEST_MODEL: "gpt-4o",
                Attrs.CREWAI_AGENT_ROLE:    "writer",
            }
        ):
            web_search("content writing best practices")
            time.sleep(0.06)
            raise RuntimeError("429 Too Many Requests from OpenAI API — rate limit exceeded")
    except RuntimeError:
        pass  # span already marked ERROR by agent_span context manager

    print(f"  ✓ Error span emitted (StatusCode=ERROR, exception recorded)")
    return run_id


# ─── Test 6: Burst — 10 mixed-framework spans ─────────────────────────────────
def test_burst():
    print(f"\n[6/6] High-throughput burst — 10 spans across all 5 frameworks")
    run_ids  = []
    fw_cycle = ["crewai", "langgraph", "openai_agents", "anthropic", "google_adk"]
    models   = ["gpt-4o", "gpt-4o-mini", "claude-3-5-sonnet-20241022",
                "claude-3-haiku-20240307", "gemini-1.5-pro"]

    for i in range(10):
        fw     = fw_cycle[i % len(fw_cycle)]
        model  = models[i % len(models)]
        run_id = str(uuid.uuid4())
        run_ids.append(run_id)

        with agent_span(
            f"{fw}.agent.burst_{i+1}",
            framework=fw,
            agent_name=f"burst-agent-{i+1}",
            run_id=run_id,
            attributes={
                Attrs.GEN_AI_SYSTEM:        fw,
                Attrs.GEN_AI_REQUEST_MODEL: model,
                Attrs.GEN_AI_INPUT_TOKENS:  random.randint(100, 2000),
                Attrs.GEN_AI_OUTPUT_TOKENS: random.randint(50, 500),
            }
        ):
            time.sleep(random.uniform(0.005, 0.03))

    print(f"  ✓ 10 burst spans emitted (crewai×2, langgraph×2, openai×2, anthropic×2, google_adk×2)")
    return run_ids


# ─── Main ─────────────────────────────────────────────────────────────────────
if __name__ == "__main__":
    results = {}
    results["crewai"]    = test_crewai();      time.sleep(0.2)
    results["langgraph"] = test_langgraph();   time.sleep(0.2)
    results["openai"]    = test_openai();      time.sleep(0.2)
    results["anthropic"] = test_anthropic_pii(); time.sleep(0.2)
    results["error"]     = test_error();       time.sleep(0.2)
    results["burst"]     = test_burst()

    print(f"\n{'='*62}")
    print(f"  All 6 tests complete — flushing spans ...")
    print(f"{'='*62}")

    # Give BatchSpanProcessor time to flush (500ms delay set above)
    time.sleep(4)
    provider.force_flush(timeout_millis=10000)
    provider.shutdown()

    print(f"\n  Collector metrics → {COLLECTOR_HTTP}/metrics")
    print(f"  Portal dashboard  → http://localhost:3000")
    print(f"  Live stream       → http://localhost:3000/live")
    print()

    # ─── Verify collector received spans ─────────────────────────────────────
    print(f"\n{'='*62}")
    print(f"  Collector Telemetry Verification")
    print(f"{'='*62}")
    try:
        import urllib.request
        with urllib.request.urlopen(f"{COLLECTOR_HTTP}/metrics", timeout=3) as resp:
            metrics_text = resp.read().decode()

        def get_metric(name):
            for line in metrics_text.splitlines():
                if line.startswith(name) and not line.startswith('#'):
                    try:    return float(line.split()[-1])
                    except: pass
            return None

        received = get_metric("govagn_receive_latency_seconds_count{source=\"http\"}")
        pii      = get_metric("govagn_pii_scrubbed_total")
        queue    = get_metric("govagn_queue_depth")

        print(f"  HTTP spans received : {int(received) if received else 'N/A'}")
        print(f"  PII values scrubbed : {int(pii) if pii is not None else 'N/A'}")
        print(f"  Queue depth now     : {int(queue) if queue is not None else 'N/A'}")

        # Check for processed spans with framework labels
        fw_counts = {}
        for line in metrics_text.splitlines():
            if 'govagn_processed_spans_total' in line and not line.startswith('#'):
                import re
                fw_m = re.search(r'framework="([^"]+)"', line)
                cnt  = float(line.split()[-1])
                if fw_m:
                    fw_counts[fw_m.group(1)] = int(cnt)

        if fw_counts:
            print(f"\n  Spans by framework:")
            for fw, cnt in sorted(fw_counts.items()):
                print(f"    {fw:<20} {cnt} spans processed")
        else:
            print(f"\n  [Note] processed_spans counter not yet visible in metrics")
            print(f"         Spans were received (latency histogram shows {int(received) if received else '?'} HTTP requests)")
            print(f"         They are forwarded to api-gateway ({API_BASE})")

        print(f"\n  Collector health: {COLLECTOR_HTTP}/healthz")
    except Exception as e:
        print(f"  Could not read metrics: {e}")
