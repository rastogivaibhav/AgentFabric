// af-core/src/pipeline/mod.rs
// Main pipeline: receive spans → build trace DAG → run policy → store

pub mod kafka;
pub mod span;
#[cfg(test)]
mod tests;

use anyhow::Result;
use std::sync::Arc;
use tracing::{error, info};

use crate::config::Config;
use crate::policy::engine::PolicyEngine;
use crate::storage::{
    clickhouse::ClickHouseStore,
    postgres::PostgresStore,
    redis::RedisStore,
};
use span::{AnomalyDetector, EnrichedSpan, TraceGraph};

#[derive(Clone)]
pub struct Pipeline {
    cfg:           Config,
    pg:            PostgresStore,
    ch:            ClickHouseStore,
    redis:         RedisStore,
    policy:        Arc<PolicyEngine>,
    anomaly:       Arc<AnomalyDetector>,
}

impl Pipeline {
    pub fn new(
        cfg: Config,
        pg: PostgresStore,
        ch: ClickHouseStore,
        redis: RedisStore,
        policy: PolicyEngine,
    ) -> Self {
        Self {
            cfg,
            pg,
            ch,
            redis,
            policy:  Arc::new(policy),
            anomaly: Arc::new(AnomalyDetector::new()),
        }
    }

    /// Process a batch of enriched spans from Kafka or gRPC
    pub async fn process_batch(&self, spans: Vec<EnrichedSpan>) -> Result<()> {
        if spans.is_empty() { return Ok(()); }

        // Group by trace for DAG construction
        let mut by_trace: std::collections::HashMap<String, Vec<EnrichedSpan>> =
            std::collections::HashMap::new();

        for span in spans {
            by_trace.entry(span.trace_id.clone()).or_default().push(span);
        }

        let mut processed: Vec<EnrichedSpan> = Vec::new();

        for (_trace_id, trace_spans) in by_trace {
            let result = self.process_trace(trace_spans).await;
            match result {
                Ok(mut spans) => processed.append(&mut spans),
                Err(e) => {
                    error!("Trace processing failed: {}", e);
                    // Continue with other traces
                }
            }
        }

        // Batch write to ClickHouse
        if let Err(e) = self.ch.insert_spans(&processed).await {
            error!("ClickHouse write failed: {}", e);
            // Non-fatal — PostgreSQL has the relational record
        }

        // Publish to Redis pub/sub for live portal stream
        if let Err(e) = self.redis.publish_spans(&processed).await {
            error!("Redis publish failed: {}", e);
        }

        Ok(())
    }

    /// Process all spans in a single trace
    async fn process_trace(&self, spans: Vec<EnrichedSpan>) -> Result<Vec<EnrichedSpan>> {
        // Build DAG for topology analysis
        let dag = TraceGraph::build(spans.clone());
        let _critical_path = dag.critical_path_ms();

        let mut result = Vec::with_capacity(spans.len());

        for mut span in spans {
            // Ensure tenant_id is set
            if span.tenant_id.is_empty() {
                span.tenant_id = self.cfg.default_tenant_id.clone();
            }

            // Run policy engine (server-side always — RT-001, RT-004 fix)
            let decisions = self.policy
                .evaluate_all(&span, &span.tenant_id)
                .await?;

            // Set authoritative policy decision on span
            let aggregate = PolicyEngine::aggregate_result(&decisions);
            span.policy_decision = aggregate.to_string();

            // Anomaly detection
            if self.anomaly.is_latency_anomaly(&span) {
                span.attributes.insert(
                    "af.anomaly.latency".to_string(),
                    "true".to_string(),
                );
            }
            if self.anomaly.is_cost_anomaly(&span) {
                span.attributes.insert(
                    "af.anomaly.cost".to_string(),
                    "true".to_string(),
                );
            }

            // Write to PostgreSQL (relational, hot path)
            self.pg.upsert_span(&span).await?;
            self.pg.upsert_agent_run(&span).await?;

            result.push(span);
        }

        Ok(result)
    }

    pub async fn shutdown(&self) {
        info!("Pipeline shutting down");
        // Drain any pending work — ClickHouse inserter flushes on drop
    }
}
