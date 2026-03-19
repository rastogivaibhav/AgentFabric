mod clickhouse;
mod config;
mod consumer;
mod policy;
mod server;
mod span;

use anyhow::Result;
use std::sync::Arc;
use tracing::info;

#[tokio::main]
async fn main() -> Result<()> {
    // ── Logging ───────────────────────────────────────────────────────────────
    tracing_subscriber::fmt()
        .with_env_filter(
            std::env::var("RUST_LOG").unwrap_or_else(|_| "info".to_string()),
        )
        .init();

    info!(
        version = env!("CARGO_PKG_VERSION"),
        "af-core starting"
    );

    // ── Config ────────────────────────────────────────────────────────────────
    let cfg = config::Config::from_env()?;

    info!(
        kafka_brokers = %cfg.kafka_brokers,
        kafka_topic   = %cfg.kafka_topic,
        clickhouse_url = %cfg.clickhouse_url,
        http_addr      = %cfg.http_addr,
        "configuration loaded"
    );

    // ── Shared counters (health server + consumer) ────────────────────────────
    let counters = server::Counters::new();

    // ── Management HTTP server (health + metrics) ─────────────────────────────
    // Spawned as a background task; if it crashes the process stays alive but
    // the health check will fail and the container will be restarted.
    let health_addr = cfg.http_addr.clone();
    let counters_for_server = Arc::clone(&counters);
    tokio::spawn(async move {
        server::run_health_server(&health_addr, counters_for_server).await;
    });

    // ── Kafka consumer (runs until SIGTERM) ───────────────────────────────────
    consumer::run_consumer(&cfg, Arc::clone(&counters)).await?;

    Ok(())
}
