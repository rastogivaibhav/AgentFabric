use serde::{Deserialize, Serialize};
use std::collections::HashMap;

/// Span message deserialized from the Kafka topic `af.spans`.
/// Field names match the JSON produced by the api-gateway Kafka producer,
/// which mirrors the `models.Span` Go struct (with `tenant_id` re-added).
#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct Span {
    pub span_id: String,
    pub trace_id: String,
    #[serde(default)]
    pub parent_span_id: String,
    #[serde(default)]
    pub run_id: String,
    pub name: String,
    pub framework: String,
    pub start_time_ns: i64,
    pub duration_ns: i64,
    #[serde(default)]
    pub status_code: i32,
    #[serde(default)]
    pub status_msg: String,
    #[serde(default)]
    pub attributes: HashMap<String, String>,
    #[serde(default)]
    pub input_tokens: i64,
    #[serde(default)]
    pub output_tokens: i64,
    #[serde(default)]
    pub cost_usd: f64,
    /// Tenant ID — re-added by the api-gateway producer (json:"-" in models.Span)
    pub tenant_id: String,
    #[serde(default)]
    pub received_at: String,
}

impl Span {
    /// End timestamp in nanoseconds since epoch.
    pub fn end_time_ns(&self) -> i64 {
        self.start_time_ns + self.duration_ns
    }

    /// Extract a named attribute, returning empty string if absent.
    pub fn attr(&self, key: &str) -> &str {
        self.attributes.get(key).map(|s| s.as_str()).unwrap_or("")
    }
}
