// af-core/src/pipeline/span.rs
// Core span types and trace DAG construction using petgraph

use petgraph::graph::{DiGraph, NodeIndex};
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use uuid::Uuid;

// ─── Enriched Span (received from collector via Kafka) ────────────────────────

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct EnrichedSpan {
    pub trace_id:      String,
    pub span_id:       String,
    pub parent_span_id: String,
    pub name:          String,
    pub framework:     String,
    pub span_kind:     String,
    pub start_time_ns: u64,
    pub duration_ns:   u64,
    pub status_code:   i32,
    pub status_msg:    String,
    pub attributes:    HashMap<String, String>,

    // Computed by collector
    pub model:         String,
    pub agent_name:    String,
    pub tool_name:     String,
    pub tool_status:   String,
    pub run_id:        String,
    pub input_tokens:  i64,
    pub output_tokens: i64,
    pub cost_usd:      f64,

    // Infrastructure
    pub service_name:  String,
    pub environment:   String,
    pub cloud_region:  String,
    pub host_name:     String,
    pub collector_node: String,
    pub received_ns:   i64,

    // Set by af-core after policy evaluation
    pub policy_decision: String,
    pub pii_detected:    bool,
    pub tenant_id:       String,
}

impl EnrichedSpan {
    pub fn duration_ms(&self) -> f64 {
        self.duration_ns as f64 / 1_000_000.0
    }

    pub fn is_root(&self) -> bool {
        self.parent_span_id.is_empty() || self.parent_span_id == "0000000000000000"
    }

    pub fn is_error(&self) -> bool {
        self.status_code > 0
    }
}

// ─── Trace DAG ────────────────────────────────────────────────────────────────

/// A directed acyclic graph of spans within a single trace.
/// Root → child spans, child → grandchild, etc.
/// Used to compute critical path, detect anomalies, and render waterfall.
pub struct TraceGraph {
    pub trace_id: String,
    pub graph:    DiGraph<EnrichedSpan, EdgeKind>,
    pub nodes:    HashMap<String, NodeIndex>,
}

#[derive(Debug, Clone, Serialize)]
pub enum EdgeKind {
    ParentChild,
    Async,
    Dependency,
}

impl TraceGraph {
    pub fn build(spans: Vec<EnrichedSpan>) -> Self {
        let trace_id = spans.first().map(|s| s.trace_id.clone()).unwrap_or_default();
        let mut graph: DiGraph<EnrichedSpan, EdgeKind> = DiGraph::new();
        let mut nodes: HashMap<String, NodeIndex> = HashMap::new();

        // Add all spans as nodes
        for span in spans {
            let span_id = span.span_id.clone();
            let idx = graph.add_node(span);
            nodes.insert(span_id, idx);
        }

        // Add parent→child edges
        let edges: Vec<(NodeIndex, NodeIndex)> = nodes.values().filter_map(|&idx| {
            let parent_id = &graph[idx].parent_span_id;
            if parent_id.is_empty() || parent_id == "0000000000000000" {
                return None;
            }
            nodes.get(parent_id).map(|&parent_idx| (parent_idx, idx))
        }).collect();

        for (parent, child) in edges {
            graph.add_edge(parent, child, EdgeKind::ParentChild);
        }

        Self { trace_id, graph, nodes }
    }

    /// Find the root span (no parent)
    pub fn root(&self) -> Option<&EnrichedSpan> {
        self.graph.node_indices().find(|&i| {
            self.graph.edges_directed(i, petgraph::Direction::Incoming).count() == 0
        }).map(|i| &self.graph[i])
    }

    /// Total trace duration from earliest start to latest end
    pub fn total_duration_ms(&self) -> f64 {
        let min_start = self.graph.node_indices()
            .map(|i| self.graph[i].start_time_ns)
            .min()
            .unwrap_or(0);
        let max_end = self.graph.node_indices()
            .map(|i| self.graph[i].start_time_ns + self.graph[i].duration_ns)
            .max()
            .unwrap_or(0);
        (max_end - min_start) as f64 / 1_000_000.0
    }

    /// Critical path: longest chain of spans by duration
    pub fn critical_path_ms(&self) -> f64 {
        // DFS from each root, accumulate max duration
        fn dfs(graph: &DiGraph<EnrichedSpan, EdgeKind>, node: NodeIndex) -> f64 {
            let span_dur = graph[node].duration_ms();
            let max_child = graph
                .neighbors_directed(node, petgraph::Direction::Outgoing)
                .map(|child| dfs(graph, child))
                .fold(0.0_f64, f64::max);
            span_dur + max_child
        }

        self.graph.node_indices()
            .filter(|&i| {
                self.graph.edges_directed(i, petgraph::Direction::Incoming).count() == 0
            })
            .map(|root| dfs(&self.graph, root))
            .fold(0.0_f64, f64::max)
    }

    /// Count spans with errors
    pub fn error_count(&self) -> usize {
        self.graph.node_indices()
            .filter(|&i| self.graph[i].is_error())
            .count()
    }

    /// Count policy violations
    pub fn violation_count(&self) -> usize {
        self.graph.node_indices()
            .filter(|&i| self.graph[i].policy_decision == "deny")
            .count()
    }

    /// Serialize to JSON for API response
    pub fn to_json_nodes(&self) -> Vec<serde_json::Value> {
        self.graph.node_indices().map(|i| {
            let span = &self.graph[i];
            serde_json::json!({
                "span_id":       span.span_id,
                "parent_span_id": span.parent_span_id,
                "name":          span.name,
                "framework":     span.framework,
                "span_kind":     span.span_kind,
                "duration_ms":   span.duration_ms(),
                "status_code":   span.status_code,
                "model":         span.model,
                "tool_name":     span.tool_name,
                "cost_usd":      span.cost_usd,
                "policy_decision": span.policy_decision,
                "depth":         self.graph.edges_directed(i, petgraph::Direction::Incoming).count(),
            })
        }).collect()
    }
}

// ─── Anomaly Detection ────────────────────────────────────────────────────────

pub struct AnomalyDetector {
    /// Rolling baseline: model → (mean_ms, stddev_ms)
    baselines: HashMap<String, (f64, f64)>,
}

impl AnomalyDetector {
    pub fn new() -> Self {
        // Seed with reasonable defaults per model type
        let mut baselines = HashMap::new();
        baselines.insert("claude-3-5-sonnet".to_string(),  (1200.0, 400.0));
        baselines.insert("claude-3-5-haiku".to_string(),    (600.0, 200.0));
        baselines.insert("gpt-4o".to_string(),             (1500.0, 500.0));
        baselines.insert("gpt-4o-mini".to_string(),         (800.0, 250.0));
        baselines.insert("gemini-1.5-pro".to_string(),     (1000.0, 350.0));
        Self { baselines }
    }

    /// Returns true if span duration is anomalous (> 3σ above baseline)
    pub fn is_latency_anomaly(&self, span: &EnrichedSpan) -> bool {
        if span.model.is_empty() { return false; }

        for (prefix, (mean, stddev)) in &self.baselines {
            if span.model.starts_with(prefix.as_str()) {
                let threshold = mean + 3.0 * stddev;
                return span.duration_ms() > threshold;
            }
        }
        false
    }

    /// Returns true if cost is anomalously high for the model
    pub fn is_cost_anomaly(&self, span: &EnrichedSpan) -> bool {
        // Flag spans costing more than $1 — almost certainly a prompt injection or runaway loop
        span.cost_usd > 1.0
    }
}
