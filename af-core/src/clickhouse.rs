use anyhow::Result;
use reqwest::Client;
use serde::Serialize;
use std::collections::HashMap;
use tracing::{error, warn};

use crate::{policy::PolicyResult, span::Span};

/// A single row for ClickHouse `agentfabric.spans` table.
/// Field names and types match clickhouse_init.sql exactly.
#[derive(Debug, Serialize)]
struct ChRow<'a> {
    trace_id: &'a str,
    span_id: &'a str,
    parent_span_id: &'a str,
    tenant_id: &'a str,
    start_ns: u64,
    end_ns: u64,
    name: &'a str,
    framework: &'a str,
    span_kind: &'a str,
    status_code: u8,
    status_message: &'a str,
    model: &'a str,
    agent_name: &'a str,
    agent_role: &'a str,
    tool_name: &'a str,
    tool_status: &'a str,
    run_id: &'a str,
    input_tokens: u32,
    output_tokens: u32,
    cost_input_usd: f64,
    cost_output_usd: f64,
    service_name: &'a str,
    environment: &'a str,
    cloud_region: &'a str,
    host_name: &'a str,
    policy_decision: &'a str,
    pii_detected: u8,
    attributes: &'a HashMap<String, String>,
}

/// Writes spans to ClickHouse via the HTTP interface using JSONEachRow format.
///
/// ClickHouse HTTP API accepts:
///   POST /
///   Body: <INSERT statement>\n<json row>\n<json row>...
///
/// No third-party ClickHouse client needed — reqwest + plain HTTP is sufficient.
pub struct ClickHouseWriter {
    client: Client,
    /// e.g. "http://clickhouse:8123"
    base_url: String,
    /// e.g. "agentfabric"
    db: String,
}

impl ClickHouseWriter {
    pub fn new(base_url: &str, db: &str) -> Self {
        Self {
            client: Client::builder()
                .timeout(std::time::Duration::from_secs(10))
                .build()
                .expect("reqwest client"),
            base_url: base_url.to_string(),
            db: db.to_string(),
        }
    }

    /// Insert a batch of spans with their policy results.
    /// Returns Ok(()) even on partial failures — individual rows are logged.
    pub async fn insert_batch(&self, batch: &[(Span, PolicyResult)]) -> Result<()> {
        if batch.is_empty() {
            return Ok(());
        }

        // Build NDJSON body: INSERT header + one JSON object per line
        let mut body =
            format!("INSERT INTO {}.spans FORMAT JSONEachRow\n", self.db);

        for (span, policy) in batch {
            let row = ChRow {
                trace_id: trim_fixed(&span.trace_id, 32),
                span_id: trim_fixed(&span.span_id, 16),
                parent_span_id: trim_fixed(&span.parent_span_id, 16),
                tenant_id: trim_fixed(&span.tenant_id, 36),
                start_ns: span.start_time_ns.max(0) as u64,
                end_ns: span.end_time_ns().max(0) as u64,
                name: &span.name,
                framework: &span.framework,
                span_kind: span.attr("af.span.kind"),
                status_code: span.status_code.clamp(0, 255) as u8,
                status_message: &span.status_msg,
                model: span.attr("gen_ai.request.model"),
                agent_name: span.attr("crewai.agent.role"),
                agent_role: span.attr("crewai.agent.role"),
                tool_name: span.attr("crewai.tool.name"),
                tool_status: span.attr("crewai.tool.status"),
                run_id: trim_fixed(&span.run_id, 36),
                input_tokens: span.input_tokens.max(0).min(u32::MAX as i64) as u32,
                output_tokens: span.output_tokens.max(0).min(u32::MAX as i64) as u32,
                cost_input_usd: span
                    .attr("af.cost.input_usd")
                    .parse::<f64>()
                    .unwrap_or(0.0),
                cost_output_usd: span
                    .attr("af.cost.output_usd")
                    .parse::<f64>()
                    .unwrap_or(0.0),
                service_name: span.attr("service.name"),
                environment: span.attr("deployment.environment"),
                cloud_region: span.attr("cloud.region"),
                host_name: span.attr("host.name"),
                policy_decision: policy.decision,
                pii_detected: policy.pii_detected as u8,
                attributes: &span.attributes,
            };

            match serde_json::to_string(&row) {
                Ok(line) => {
                    body.push_str(&line);
                    body.push('\n');
                }
                Err(e) => {
                    warn!(
                        span_id = %span.span_id,
                        "failed to serialize span for ClickHouse: {}",
                        e
                    );
                }
            }
        }

        let resp = self
            .client
            .post(&self.base_url)
            .header("Content-Type", "text/plain; charset=utf-8")
            .body(body)
            .send()
            .await?;

        if !resp.status().is_success() {
            let status = resp.status();
            let text = resp.text().await.unwrap_or_default();
            error!("ClickHouse insert failed ({}) — {}", status, text);
            anyhow::bail!("clickhouse: {} — {}", status, text);
        }

        Ok(())
    }
}

/// Truncate a string slice to at most `max_bytes` bytes.
/// ClickHouse FixedString columns silently pad shorter values with null bytes,
/// but reject values longer than their declared size.
fn trim_fixed(s: &str, max_bytes: usize) -> &str {
    if s.len() <= max_bytes {
        s
    } else {
        &s[..max_bytes]
    }
}
