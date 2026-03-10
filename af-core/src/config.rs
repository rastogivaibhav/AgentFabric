// af-core/src/config.rs
use anyhow::Result;
use serde::Deserialize;

#[derive(Debug, Clone, Deserialize)]
pub struct Config {
    #[serde(default = "default_database_url")]
    pub database_url: String,

    #[serde(default = "default_clickhouse_url")]
    pub clickhouse_url: String,

    #[serde(default = "default_redis_url")]
    pub redis_url: String,

    #[serde(default = "default_grpc_addr")]
    pub grpc_addr: String,

    #[serde(default = "default_http_addr")]
    pub http_addr: String,

    #[serde(default)]
    pub kafka_enabled: bool,

    #[serde(default = "default_kafka_brokers")]
    pub kafka_brokers: String,

    #[serde(default = "default_kafka_topic")]
    pub kafka_topic: String,

    #[serde(default = "default_tenant_id")]
    pub default_tenant_id: String,

    // Policy
    #[serde(default = "default_allowed_regions")]
    pub allowed_regions: Vec<String>,

    #[serde(default = "default_cost_threshold")]
    pub cost_threshold_usd: f64,

    #[serde(default)]
    pub denied_tools: Vec<String>,

    #[serde(default = "default_rate_limit")]
    pub rate_limit_spans_per_min: u64,
}

impl Config {
    pub fn from_env() -> Result<Self> {
        let cfg = config::Config::builder()
            .add_source(config::Environment::default().separator("__"))
            .build()?
            .try_deserialize()?;
        Ok(cfg)
    }
}

fn default_database_url()    -> String { "postgres://fabric:fabric@localhost:5432/agentfabric".into() }
fn default_clickhouse_url()  -> String { "http://localhost:8123".into() }
fn default_redis_url()       -> String { "redis://localhost:6379/0".into() }
fn default_grpc_addr()       -> String { "0.0.0.0:50051".into() }
fn default_http_addr()       -> String { "0.0.0.0:8889".into() }
fn default_kafka_brokers()   -> String { "localhost:9092".into() }
fn default_kafka_topic()     -> String { "af.spans".into() }
fn default_tenant_id()       -> String { "default".into() }
fn default_cost_threshold()  -> f64    { 10.0 }
fn default_rate_limit()      -> u64    { 10_000 }
fn default_allowed_regions() -> Vec<String> {
    vec!["eu-west-1".into(), "eu-west-2".into(), "eu-central-1".into()]
}
