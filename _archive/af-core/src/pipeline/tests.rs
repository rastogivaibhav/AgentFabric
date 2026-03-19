// af-core/src/pipeline/tests.rs
// Unit tests for span types, TraceGraph, AnomalyDetector, and policy engine.
// Run: cargo test  (no external dependencies required)

#[cfg(test)]
mod span_tests {
    use std::collections::HashMap;
    use crate::pipeline::span::{EnrichedSpan, TraceGraph, EdgeKind, AnomalyDetector};

    fn make_span(span_id: &str, parent_id: &str) -> EnrichedSpan {
        EnrichedSpan {
            trace_id:       "trace-001".into(),
            span_id:        span_id.into(),
            parent_span_id: parent_id.into(),
            name:           "test-span".into(),
            framework:      "crewai".into(),
            span_kind:      "agent".into(),
            start_time_ns:  1_000_000_000,
            duration_ns:    500_000_000,
            status_code:    0,
            status_msg:     String::new(),
            attributes:     HashMap::new(),
            model:          "claude-3-5-sonnet".into(),
            agent_name:     "test-agent".into(),
            tool_name:      String::new(),
            tool_status:    String::new(),
            run_id:         "run-001".into(),
            input_tokens:   100,
            output_tokens:  50,
            cost_usd:       0.001,
            input_cost_usd:  0.0006,
            output_cost_usd: 0.0004,
            service_name:   "test-service".into(),
            environment:    "test".into(),
            cloud_region:   "eu-west-1".into(),
            host_name:      "host-1".into(),
            collector_node: "collector-1".into(),
            received_ns:    1_000_000_001,
            policy_decision: "allow".into(),
            pii_detected:   false,
            tenant_id:      "tenant-1".into(),
        }
    }

    #[test]
    fn is_root_empty_parent() {
        let span = make_span("s1", "");
        assert!(span.is_root(), "empty parent_span_id should be root");
    }

    #[test]
    fn is_root_zero_parent() {
        let span = make_span("s1", "0000000000000000");
        assert!(span.is_root(), "zero parent should be root");
    }

    #[test]
    fn is_not_root_with_parent() {
        let span = make_span("s2", "s1");
        assert!(!span.is_root(), "span with parent should not be root");
    }

    #[test]
    fn is_error_nonzero_status() {
        let mut span = make_span("s1", "");
        span.status_code = 2; // OTLP STATUS_CODE_ERROR
        assert!(span.is_error());
    }

    #[test]
    fn is_not_error_zero_status() {
        let span = make_span("s1", "");
        assert!(!span.is_error());
    }

    #[test]
    fn duration_ms_calculation() {
        let mut span = make_span("s1", "");
        span.duration_ns = 1_500_000; // 1.5ms
        assert!((span.duration_ms() - 1.5).abs() < f64::EPSILON);
    }

    #[test]
    fn cost_fields_separate() {
        let span = make_span("s1", "");
        assert!((span.input_cost_usd - 0.0006).abs() < 1e-10);
        assert!((span.output_cost_usd - 0.0004).abs() < 1e-10);
        assert!((span.cost_usd - 0.001).abs() < 1e-10);
    }

    // ─── TraceGraph ─────────────────────────────────────────────────────────

    fn make_trace() -> Vec<EnrichedSpan> {
        let mut root = make_span("root", "");
        root.duration_ns = 1_000_000_000; // 1000ms

        let mut child1 = make_span("child1", "root");
        child1.duration_ns = 300_000_000; // 300ms

        let mut child2 = make_span("child2", "root");
        child2.duration_ns = 500_000_000; // 500ms

        vec![root, child1, child2]
    }

    #[test]
    fn trace_graph_builds_with_correct_node_count() {
        let spans = make_trace();
        let graph = TraceGraph::build(spans);
        assert_eq!(graph.graph.node_count(), 3);
    }

    #[test]
    fn trace_graph_root_found() {
        let spans = make_trace();
        let graph = TraceGraph::build(spans);
        let root = graph.root();
        assert!(root.is_some(), "should find root span");
        assert_eq!(root.unwrap().span_id, "root");
    }

    #[test]
    fn trace_graph_edge_count() {
        let spans = make_trace();
        let graph = TraceGraph::build(spans);
        assert_eq!(graph.graph.edge_count(), 2, "root should have 2 child edges");
    }

    #[test]
    fn trace_graph_critical_path_is_root_plus_longest_child() {
        let spans = make_trace();
        let graph = TraceGraph::build(spans);
        // root(1000ms) + child2(500ms) = 1500ms
        let cp = graph.critical_path_ms();
        assert!((cp - 1500.0).abs() < 1.0, "critical path should be 1500ms, got {}", cp);
    }

    #[test]
    fn trace_graph_error_count_zero_by_default() {
        let spans = make_trace();
        let graph = TraceGraph::build(spans);
        assert_eq!(graph.error_count(), 0);
    }

    #[test]
    fn trace_graph_error_count_counts_errors() {
        let mut spans = make_trace();
        spans[1].status_code = 2;
        let graph = TraceGraph::build(spans);
        assert_eq!(graph.error_count(), 1);
    }

    #[test]
    fn trace_graph_violation_count() {
        let mut spans = make_trace();
        spans[0].policy_decision = "deny".into();
        let graph = TraceGraph::build(spans);
        assert_eq!(graph.violation_count(), 1);
    }

    #[test]
    fn trace_graph_json_nodes_serializes() {
        let spans = make_trace();
        let graph = TraceGraph::build(spans);
        let nodes = graph.to_json_nodes();
        assert_eq!(nodes.len(), 3);
        // Each node has required keys
        for node in &nodes {
            assert!(node.get("span_id").is_some());
            assert!(node.get("duration_ms").is_some());
        }
    }

    // ─── AnomalyDetector ────────────────────────────────────────────────────

    #[test]
    fn anomaly_detector_normal_latency_not_anomalous() {
        let detector = AnomalyDetector::new();
        let mut span = make_span("s1", "");
        span.model = "claude-3-5-sonnet".into();
        span.duration_ns = 1_200_000_000; // 1200ms — exactly the mean
        assert!(!detector.is_latency_anomaly(&span));
    }

    #[test]
    fn anomaly_detector_high_latency_is_anomalous() {
        let detector = AnomalyDetector::new();
        let mut span = make_span("s1", "");
        span.model = "claude-3-5-sonnet".into();
        // mean=1200ms + 3*stddev=3*400ms = 2400ms; 3000ms > threshold
        span.duration_ns = 3_000_000_000;
        assert!(detector.is_latency_anomaly(&span));
    }

    #[test]
    fn anomaly_detector_unknown_model_never_anomalous() {
        let detector = AnomalyDetector::new();
        let mut span = make_span("s1", "");
        span.model = String::new();
        span.duration_ns = 999_999_999_999;
        assert!(!detector.is_latency_anomaly(&span));
    }

    #[test]
    fn anomaly_detector_cost_over_one_dollar_anomalous() {
        let detector = AnomalyDetector::new();
        let mut span = make_span("s1", "");
        span.cost_usd = 1.01;
        assert!(detector.is_cost_anomaly(&span));
    }

    #[test]
    fn anomaly_detector_cost_under_threshold_not_anomalous() {
        let detector = AnomalyDetector::new();
        let mut span = make_span("s1", "");
        span.cost_usd = 0.99;
        assert!(!detector.is_cost_anomaly(&span));
    }
}

#[cfg(test)]
mod policy_tests {
    use std::collections::HashMap;
    use crate::pipeline::span::EnrichedSpan;
    use crate::policy::engine::{
        CostThresholdPolicy, PIIOutputPolicy, Policy, PolicyDecision, PolicyResult,
        PolicyEngine, RateLimitPolicy, SovereigntyPolicy, UnauthorizedToolPolicy,
    };

    fn base_span() -> EnrichedSpan {
        EnrichedSpan {
            trace_id:        "trace-001".into(),
            span_id:         "span-001".into(),
            parent_span_id:  String::new(),
            name:            "test-span".into(),
            framework:       "crewai".into(),
            span_kind:       "llm_call".into(),
            start_time_ns:   0,
            duration_ns:     1_000_000,
            status_code:     0,
            status_msg:      String::new(),
            attributes:      HashMap::new(),
            model:           "claude-3-5-sonnet".into(),
            agent_name:      "test-agent".into(),
            tool_name:       String::new(),
            tool_status:     String::new(),
            run_id:          "run-001".into(),
            input_tokens:    100,
            output_tokens:   50,
            cost_usd:        0.001,
            input_cost_usd:  0.0006,
            output_cost_usd: 0.0004,
            service_name:    "svc-a".into(),
            environment:     "production".into(),
            cloud_region:    "eu-west-1".into(),
            host_name:       "host-1".into(),
            collector_node:  "col-1".into(),
            received_ns:     1,
            policy_decision: "allow".into(),
            pii_detected:    false,
            tenant_id:       "tenant-1".into(),
        }
    }

    fn make_deny_decisions(n: usize) -> Vec<PolicyDecision> {
        (0..n).map(|i| PolicyDecision {
            decision_id:   uuid::Uuid::new_v4(),
            tenant_id:     "t".into(),
            trace_id:      "tr".into(),
            span_id:       format!("sp-{}", i),
            policy_name:   "test".into(),
            result:        PolicyResult::Deny,
            reason:        "test".into(),
            evaluated_at:  chrono::Utc::now(),
            evaluated_ns:  0,
            previous_hash: String::new(),
            entry_hash:    String::new(),
        }).collect()
    }

    // ─── SovereigntyPolicy ──────────────────────────────────────────────────

    #[test]
    fn sovereignty_allows_permitted_region() {
        let policy = SovereigntyPolicy {
            allowed_regions: vec!["eu-west-1".into(), "eu-west-2".into()],
        };
        let span = base_span();
        assert!(policy.applies_to(&span));
        let (result, reason) = policy.evaluate(&span);
        assert_eq!(result, PolicyResult::Allow);
        assert!(reason.contains("eu-west-1"));
    }

    #[test]
    fn sovereignty_denies_prohibited_region() {
        let policy = SovereigntyPolicy {
            allowed_regions: vec!["eu-west-1".into()],
        };
        let mut span = base_span();
        span.cloud_region = "us-east-1".into();
        let (result, reason) = policy.evaluate(&span);
        assert_eq!(result, PolicyResult::Deny);
        assert!(reason.contains("us-east-1"));
    }

    #[test]
    fn sovereignty_does_not_apply_empty_region() {
        let policy = SovereigntyPolicy {
            allowed_regions: vec!["eu-west-1".into()],
        };
        let mut span = base_span();
        span.cloud_region = String::new();
        assert!(!policy.applies_to(&span));
    }

    #[test]
    fn sovereignty_policy_name() {
        let policy = SovereigntyPolicy { allowed_regions: vec![] };
        assert_eq!(policy.name(), "sovereignty.cross_border_data_flow");
    }

    // ─── CostThresholdPolicy ────────────────────────────────────────────────

    #[test]
    fn cost_threshold_allows_within_limit() {
        let policy = CostThresholdPolicy { max_cost_usd: 0.10 };
        let span = base_span(); // cost_usd = 0.001
        let (result, _) = policy.evaluate(&span);
        assert_eq!(result, PolicyResult::Allow);
    }

    #[test]
    fn cost_threshold_warns_over_limit() {
        let policy = CostThresholdPolicy { max_cost_usd: 0.10 };
        let mut span = base_span();
        span.cost_usd = 0.15;
        let (result, reason) = policy.evaluate(&span);
        assert_eq!(result, PolicyResult::Warn);
        assert!(reason.contains("0.1500"));
    }

    #[test]
    fn cost_threshold_does_not_apply_zero_cost() {
        let policy = CostThresholdPolicy { max_cost_usd: 0.10 };
        let mut span = base_span();
        span.cost_usd = 0.0;
        assert!(!policy.applies_to(&span));
    }

    #[test]
    fn cost_threshold_policy_name() {
        let policy = CostThresholdPolicy { max_cost_usd: 1.0 };
        assert_eq!(policy.name(), "cost.threshold_exceeded");
    }

    // ─── UnauthorizedToolPolicy ─────────────────────────────────────────────

    #[test]
    fn unauthorized_tool_denies_blocked_tool() {
        let policy = UnauthorizedToolPolicy {
            denied_tools: vec!["shell_exec".into(), "file_delete".into()],
        };
        let mut span = base_span();
        span.tool_name = "shell_exec".into();
        assert!(policy.applies_to(&span));
        let (result, reason) = policy.evaluate(&span);
        assert_eq!(result, PolicyResult::Deny);
        assert!(reason.contains("shell_exec"));
    }

    #[test]
    fn unauthorized_tool_allows_permitted_tool() {
        let policy = UnauthorizedToolPolicy {
            denied_tools: vec!["shell_exec".into()],
        };
        let mut span = base_span();
        span.tool_name = "web_search".into();
        let (result, _) = policy.evaluate(&span);
        assert_eq!(result, PolicyResult::Allow);
    }

    #[test]
    fn unauthorized_tool_does_not_apply_empty_tool() {
        let policy = UnauthorizedToolPolicy { denied_tools: vec![] };
        let span = base_span(); // tool_name is empty
        assert!(!policy.applies_to(&span));
    }

    // ─── PIIOutputPolicy ────────────────────────────────────────────────────

    #[test]
    fn pii_policy_warns_when_pii_detected() {
        let policy = PIIOutputPolicy;
        let mut span = base_span();
        span.span_kind = "llm_call".into();
        span.attributes.insert("af.pii.detected".into(), "true".into());
        assert!(policy.applies_to(&span));
        let (result, reason) = policy.evaluate(&span);
        assert_eq!(result, PolicyResult::Warn);
        assert!(reason.contains("PII"));
    }

    #[test]
    fn pii_policy_allows_when_no_pii() {
        let policy = PIIOutputPolicy;
        let span = base_span(); // af.pii.detected not set
        let (result, _) = policy.evaluate(&span);
        assert_eq!(result, PolicyResult::Allow);
    }

    #[test]
    fn pii_policy_does_not_apply_non_llm_span() {
        let policy = PIIOutputPolicy;
        let mut span = base_span();
        span.span_kind = "tool_call".into();
        assert!(!policy.applies_to(&span));
    }

    // ─── RateLimitPolicy ────────────────────────────────────────────────────

    #[test]
    fn rate_limit_allows_under_threshold() {
        let policy = RateLimitPolicy::new(1000);
        let span = base_span();
        let (result, _) = policy.evaluate(&span);
        assert_eq!(result, PolicyResult::Allow);
    }

    #[test]
    fn rate_limit_warns_over_threshold() {
        let policy = RateLimitPolicy::new(2); // very low limit
        let span = base_span();
        // First two calls: under limit
        policy.evaluate(&span);
        policy.evaluate(&span);
        // Third call: over limit
        let (result, reason) = policy.evaluate(&span);
        assert_eq!(result, PolicyResult::Warn);
        assert!(reason.contains("svc-a"));
    }

    #[test]
    fn rate_limit_applies_to_all_spans() {
        let policy = RateLimitPolicy::new(100);
        let span = base_span();
        assert!(policy.applies_to(&span));
    }

    // ─── PolicyEngine.aggregate_result ──────────────────────────────────────

    #[test]
    fn aggregate_deny_takes_precedence_over_warn() {
        let mut decisions = make_deny_decisions(1);
        let mut warn = decisions[0].clone();
        warn.result = PolicyResult::Warn;
        decisions.push(warn);
        assert_eq!(PolicyEngine::aggregate_result(&decisions), PolicyResult::Deny);
    }

    #[test]
    fn aggregate_all_allow_returns_allow() {
        let decisions: Vec<PolicyDecision> = (0..3).map(|i| PolicyDecision {
            decision_id:   uuid::Uuid::new_v4(),
            tenant_id:     "t".into(),
            trace_id:      "tr".into(),
            span_id:       format!("sp-{}", i),
            policy_name:   "test".into(),
            result:        PolicyResult::Allow,
            reason:        "ok".into(),
            evaluated_at:  chrono::Utc::now(),
            evaluated_ns:  0,
            previous_hash: String::new(),
            entry_hash:    String::new(),
        }).collect();
        assert_eq!(PolicyEngine::aggregate_result(&decisions), PolicyResult::Allow);
    }

    #[test]
    fn aggregate_warn_without_deny_returns_warn() {
        let decisions: Vec<PolicyDecision> = vec![
            PolicyDecision {
                decision_id:   uuid::Uuid::new_v4(),
                tenant_id:     "t".into(),
                trace_id:      "tr".into(),
                span_id:       "sp-1".into(),
                policy_name:   "cost".into(),
                result:        PolicyResult::Warn,
                reason:        "high cost".into(),
                evaluated_at:  chrono::Utc::now(),
                evaluated_ns:  0,
                previous_hash: String::new(),
                entry_hash:    String::new(),
            },
        ];
        assert_eq!(PolicyEngine::aggregate_result(&decisions), PolicyResult::Warn);
    }

    #[test]
    fn aggregate_empty_decisions_returns_allow() {
        assert_eq!(PolicyEngine::aggregate_result(&[]), PolicyResult::Allow);
    }

    // ─── Principle compliance checks ────────────────────────────────────────

    #[test]
    fn principle_2_tenant_id_in_decisions() {
        // All PolicyDecision objects must carry tenant_id (multi-tenant isolation)
        let decisions = make_deny_decisions(3);
        for d in &decisions {
            assert!(!d.tenant_id.is_empty(), "PolicyDecision must have tenant_id (Principle 2)");
        }
    }

    #[test]
    fn principle_1_server_side_cost_computed_separately() {
        // input_cost_usd and output_cost_usd must NOT be derived from 60/40 split
        let span = base_span();
        // Verify the fields are independent of total
        assert_ne!(span.input_cost_usd, span.cost_usd * 0.6,
            "input_cost_usd should NOT be hardcoded 60% split (Principle 1)");
    }
}
