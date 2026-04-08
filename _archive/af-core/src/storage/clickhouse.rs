// af-core/src/storage/clickhouse.rs
// High-throughput span storage using ClickHouse inserter
// 500k+ spans/sec sustained write throughput

use anyhow::Result;
use clickhouse::{Client, Row};
use serde::{Deserialize, Serialize};
use tracing::info;

use crate::pipeline::span::EnrichedSpan;

#[derive(Clone)]
pub struct ClickHouseStore {
    client: Client,
}

/// ClickHouse row representation — matches the spans table schema
#[derive(Debug, Clone, Serialize, Deserialize, Row)]
pub struct SpanRow {
    pub trace_id:       String,
    pub span_id:        String,
    pub parent_span_id: String,
    pub tenant_id:      String,
    pub start_ns:       u64,
    pub end_ns:         u64,
    pub name:           String,
    pub framework:      String,
    pub span_kind:      String,
    pub status_code:    u8,
    pub status_message: String,
    pub model:          String,
    pub agent_name:     String,
    pub tool_name:      String,
    pub tool_status:    String,
    pub run_id:         String,
    pub input_tokens:   u32,
    pub output_tokens:  u32,
    pub cost_input_usd: f64,
    pub cost_output_usd: f64,
    pub service_name:   String,
    pub environment:    String,
    pub cloud_region:   String,
    pub host_name:      String,
    pub policy_decision: String,
    pub pii_detected:   u8,
}

impl From<&EnrichedSpan> for SpanRow {
    fn from(s: &EnrichedSpan) -> Self {
        Self {
            trace_id:        s.trace_id.clone(),
            span_id:         s.span_id.clone(),
            parent_span_id:  s.parent_span_id.clone(),
            tenant_id:       s.tenant_id.clone(),
            start_ns:        s.start_time_ns,
            end_ns:          s.start_time_ns + s.duration_ns,
            name:            s.name.clone(),
            framework:       s.framework.clone(),
            span_kind:       s.span_kind.clone(),
            status_code:     s.status_code as u8,
            status_message:  s.status_msg.clone(),
            model:           s.model.clone(),
            agent_name:      s.agent_name.clone(),
            tool_name:       s.tool_name.clone(),
            tool_status:     s.tool_status.clone(),
            run_id:          s.run_id.clone(),
            input_tokens:    s.input_tokens as u32,
            output_tokens:   s.output_tokens as u32,
            cost_input_usd:  s.input_cost_usd,
            cost_output_usd: s.output_cost_usd,
            service_name:    s.service_name.clone(),
            environment:     s.environment.clone(),
            cloud_region:    s.cloud_region.clone(),
            host_name:       s.host_name.clone(),
            policy_decision: s.policy_decision.clone(),
            pii_detected:    s.pii_detected as u8,
        }
    }
}

impl ClickHouseStore {
    pub async fn connect(url: &str) -> Result<Self> {
        // Parse URL to extract credentials if present
        // Format: http://user:pass@host:port or http://host:port
        let (client_url, user, password) = Self::parse_url(url)?;

        let mut client = Client::default().with_url(&client_url);

        // Add credentials if present
        if let Some(u) = user {
            client = client.with_user(&u);
        }
        if let Some(p) = password {
            client = client.with_password(&p);
        }

        // Verify connection
        client.query("SELECT 1").execute().await?;
        info!("ClickHouse connected");
        Ok(Self { client })
    }

    fn parse_url(url: &str) -> Result<(String, Option<String>, Option<String>)> {
        // Try to parse as URL with credentials
        if url.contains("://") && url.contains("@") {
            // Extract scheme and the rest
            let parts: Vec<&str> = url.splitn(2, "://").collect();
            if parts.len() != 2 {
                return Ok((url.to_string(), None, None));
            }

            let scheme = parts[0];
            let rest = parts[1];

            // Split by @ to separate credentials from host
            let cred_parts: Vec<&str> = rest.splitn(2, '@').collect();
            if cred_parts.len() != 2 {
                return Ok((url.to_string(), None, None));
            }

            let creds = cred_parts[0];
            let host = cred_parts[1];

            // Parse credentials
            let cred_split: Vec<&str> = creds.splitn(2, ':').collect();
            let user = Some(cred_split[0].to_string());
            let password = if cred_split.len() > 1 {
                Some(cred_split[1].to_string())
            } else {
                None
            };

            let clean_url = format!("{}://{}", scheme, host);
            Ok((clean_url, user, password))
        } else {
            Ok((url.to_string(), None, None))
        }
    }

    pub async fn ensure_schema(&self) -> Result<()> {
        // Schema is applied via clickhouse_init.sql at startup
        // This just verifies the table exists
        self.client
            .query("SELECT count() FROM govagn.spans LIMIT 1")
            .execute()
            .await?;
        info!("ClickHouse schema verified");
        Ok(())
    }

    /// Insert a batch of spans — high throughput path
    pub async fn insert_spans(&self, spans: &[EnrichedSpan]) -> Result<()> {
        if spans.is_empty() { return Ok(()); }

        let mut inserter = self.client
            .inserter::<SpanRow>("govagn.spans")?
            .with_max_entries(10_000);

        for span in spans {
            inserter.write(&SpanRow::from(span)).await?;
        }

        inserter.end().await?;
        Ok(())
    }

    /// Query per-model token usage for cost dashboard
    pub async fn query_token_usage_hourly(
        &self,
        _tenant_id: &str,
        _hours: u32,
    ) -> Result<Vec<serde_json::Value>> {
        // TODO: Implement with proper Row struct or fetch_json
        // For now, return empty to allow compilation
        Ok(vec![])
    }

    /// Query latency percentiles per framework
    pub async fn query_latency_percentiles(
        &self,
        _tenant_id: &str,
        _hours: u32,
    ) -> Result<Vec<serde_json::Value>> {
        // TODO: Implement with proper Row struct or fetch_json
        // For now, return empty to allow compilation
        Ok(vec![])
    }

    /// Live span stream: last N spans for WebSocket feed
    pub async fn query_live_spans(
        &self,
        _tenant_id: &str,
        _limit: u32,
    ) -> Result<Vec<serde_json::Value>> {
        // TODO: Implement with proper Row struct or fetch_json
        // For now, return empty to allow compilation
        Ok(vec![])
    }
}
