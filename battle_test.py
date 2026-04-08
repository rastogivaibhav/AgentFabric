import json
import time
import uuid
import urllib.request
import urllib.error

# Config
GATEWAY_URL = "http://localhost:8080"
INTERNAL_INGEST = f"{GATEWAY_URL}/internal/ingest"

def log_step(msg):
    print(f"\n==> {msg}")

def send_spans(spans):
    try:
        data = json.dumps({"spans": spans}).encode()
        req = urllib.request.Request(
            INTERNAL_INGEST,
            data=data,
            headers={"Content-Type": "application/json"},
            method="POST"
        )
        urllib.request.urlopen(req, timeout=5)
        print(f"  [OK] Ingested {len(spans)} spans")
    except Exception as e:
        print(f"  [ERR] {e}")

def run_battle_simulation():
    run_id = str(uuid.uuid4())
    trace_id = str(uuid.uuid4().hex)
    
    # 1. PROTECT Stack: PII Redaction Simulation
    log_step("Scenario 1: PII Redaction (PROTECT)")
    span_id_1 = str(uuid.uuid4().hex)[:16]
    send_spans([{
        "trace_id": trace_id,
        "span_id": span_id_1,
        "name": "pii_scrubbing_agent",
        "framework": "langgraph",
        "agent_name": "SecurityGuard",
        "start_time_ns": int(time.time() * 1e9),
        "duration_ns": 500 * 1000 * 1000,
        "status_code": 1,
        "attributes": {
            "af.agent.run_id": run_id,
            "af.policy.action": "redact",
            "af.policy.decision": "sanitize",
            "af.policy.match_type": "DLP",
            "af.policy.matched_pattern": "SSN",
            "gen_ai.system": "openai",
            "gen_ai.usage.input_tokens": "120",
            "gen_ai.usage.output_tokens": "40"
        }
    }])

    # 2. PROTECT Stack: Malicious Intent Blocking
    log_step("Scenario 2: Prompt Injection Blocking (PROTECT)")
    span_id_2 = str(uuid.uuid4().hex)[:16]
    send_spans([{
        "trace_id": trace_id,
        "span_id": span_id_2,
        "parent_span_id": span_id_1,
        "name": "injection_filter",
        "framework": "crewai",
        "agent_name": "GatewayEnforcer",
        "start_time_ns": int(time.time() * 1e9),
        "duration_ns": 100 * 1000 * 1000,
        "status_code": 2, # Error/Blocked
        "attributes": {
            "af.agent.run_id": run_id,
            "af.policy.blocked": "true",
            "af.policy.decision": "deny",
            "af.policy.reason": "Prompt Injection Attempt Detected",
            "af.outcome_status": "blocked"
        }
    }])

    # 3. CONTROL Stack: Canary Rollout
    log_step("Scenario 3: Canary Rollout Routing (CONTROL)")
    for i in range(5):
        m = "gpt-4o-mini" if i < 4 else "claude-3-5-sonnet"
        send_spans([{
            "trace_id": str(uuid.uuid4().hex),
            "span_id": str(uuid.uuid4().hex)[:16],
            "name": f"routing_test_{i}",
            "framework": "openai",
            "start_time_ns": int(time.time() * 1e9),
            "duration_ns": 800 * 1000 * 1000,
            "status_code": 1,
            "attributes": {
                "af.gateway.route_type": "canary",
                "af.gateway.target_model": m,
                "gen_ai.model": m
            }
        }])

    # 4. SPEND Stack: Budget Hard Limit
    log_step("Scenario 4: Budget Hard Limit (SPEND)")
    send_spans([{
        "trace_id": str(uuid.uuid4().hex),
        "span_id": str(uuid.uuid4().hex)[:16],
        "name": "over_budget_call",
        "framework": "langchain",
        "start_time_ns": int(time.time() * 1e9),
        "duration_ns": 50 * 1000 * 1000,
        "status_code": 2,
        "attributes": {
            "af.policy.blocked": "true",
            "af.policy.decision": "deny",
            "af.policy.reason": "Monthly budget limit ($10.00) reached",
            "af.cost.total_usd": "0.00",
            "af.outcome_status": "blocked"
        }
    }])

    # 5. OBSERVE Stack: Deep Chain Tracing
    log_step("Scenario 5: Deep Agent Tracing (OBSERVE)")
    p_trace_id = str(uuid.uuid4().hex)
    p_span_id = str(uuid.uuid4().hex)[:16]
    # Parent
    send_spans([{
        "trace_id": p_trace_id,
        "span_id": p_span_id,
        "name": "research_workflow",
        "framework": "langgraph",
        "agent_name": "Orchestrator",
        "start_time_ns": int(time.time() * 1e9),
        "duration_ns": 2000 * 1000 * 1000,
        "status_code": 1,
        "attributes": {"af.agent.run_id": run_id}
    }])
    # Child 1
    send_spans([{
        "trace_id": p_trace_id,
        "span_id": str(uuid.uuid4().hex)[:16],
        "parent_span_id": p_span_id,
        "name": "search_step",
        "start_time_ns": int(time.time() * 1e9),
        "duration_ns": 600 * 1000 * 1000,
        "status_code": 1,
        "attributes": {"af.agent.run_id": run_id}
    }])
    # Child 2
    send_spans([{
        "trace_id": p_trace_id,
        "span_id": str(uuid.uuid4().hex)[:16],
        "parent_span_id": p_span_id,
        "name": "summarize_step",
        "start_time_ns": int(time.time() * 1e9),
        "duration_ns": 400 * 1000 * 1000,
        "status_code": 1,
        "attributes": {"af.agent.run_id": run_id, "gen_ai.usage.total_tokens": "850"}
    }])

    log_step("Super-Test Verification Complete.")

if __name__ == "__main__":
    run_battle_simulation()
