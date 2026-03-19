use anyhow::{Context, Result};

/// Runtime configuration loaded entirely from environment variables.
/// All fields have sensible defaults so the service starts cleanly in dev.
#[derive(Debug, Clone)]
pub struct Config {
    /// Comma-separated Kafka broker addresses (e.g. "kafka:9092")
    pub kafka_brokers: String,
    /// Kafka topic to consume spans from
    pub kafka_topic: String,
    /// Kafka consumer group ID
    pub kafka_group_id: String,
    /// ClickHouse HTTP endpoint (e.g. "http://clickhouse:8123")
    pub clickhouse_url: String,
    /// ClickHouse database name
    pub clickhouse_db: String,
    /// HTTP management server address (health + metrics)
    pub http_addr: String,
    /// Maximum spans to buffer before flushing to ClickHouse
    pub batch_size: usize,
    /// Maximum milliseconds to wait before flushing a partial batch
    pub batch_timeout_ms: u64,
}

impl Config {
    pub fn from_env() -> Result<Self> {
        Ok(Self {
            kafka_brokers: env_or("KAFKA_BROKERS", "kafka:9092"),
            kafka_topic: env_or("KAFKA_TOPIC", "af.spans"),
            kafka_group_id: env_or("KAFKA_GROUP_ID", "af-core"),
            clickhouse_url: env_or("CLICKHOUSE_URL", "http://clickhouse:8123"),
            clickhouse_db: env_or("CLICKHOUSE_DB", "agentfabric"),
            http_addr: env_or("HTTP_ADDR", "0.0.0.0:8889"),
            batch_size: env_parse("AF_BATCH_SIZE", 256),
            batch_timeout_ms: env_parse("AF_BATCH_TIMEOUT_MS", 500),
        })
    }
}

fn env_or(key: &str, default: &str) -> String {
    std::env::var(key).unwrap_or_else(|_| default.to_string())
}

fn env_parse<T>(key: &str, default: T) -> T
where
    T: std::str::FromStr,
{
    std::env::var(key)
        .ok()
        .and_then(|v| v.parse().ok())
        .unwrap_or(default)
}
