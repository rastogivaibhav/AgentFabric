// AgentFabric Core — Production Processing Engine
// Rust 1.78 | Tokio async | gRPC ingestion | Policy engine | Hash-chain audit

mod config;
mod pipeline;
mod policy;
mod storage;
mod server;
mod metrics;

use anyhow::Result;
use tracing::{info, error};
use tracing_subscriber::{EnvFilter, fmt, prelude::*};

#[tokio::main]
async fn main() -> Result<()> {
    // Structured JSON logging for production
    tracing_subscriber::registry()
        .with(EnvFilter::from_env("RUST_LOG").add_directive("af_core=info".parse()?))
        .with(fmt::layer().json())
        .init();

    info!(version = "1.0.0", "AgentFabric Core starting");

    let cfg = config::Config::from_env()?;

    // Storage layer
    let pg = storage::postgres::PostgresStore::connect(&cfg.database_url).await?;
    pg.run_migrations().await?;

    let ch = storage::clickhouse::ClickHouseStore::connect(&cfg.clickhouse_url).await?;
    ch.ensure_schema().await?;

    let redis = storage::redis::RedisStore::connect(&cfg.redis_url).await?;

    // Policy engine with audit writer
    let audit_writer = policy::audit::AuditWriter::new(pg.clone());
    let policy_engine = policy::engine::PolicyEngine::new(cfg.clone(), audit_writer);

    // Pipeline
    let pipeline = pipeline::Pipeline::new(
        cfg.clone(),
        pg.clone(),
        ch.clone(),
        redis.clone(),
        policy_engine,
    );

    // Start Kafka consumer (RT-006 durable buffer)
    let kafka_handle = if cfg.kafka_enabled {
        let p = pipeline.clone();
        let brokers = cfg.kafka_brokers.clone();
        let topic = cfg.kafka_topic.clone();
        Some(tokio::spawn(async move {
            if let Err(e) = pipeline::kafka::run_consumer(p, &brokers, &topic).await {
                error!("Kafka consumer error: {}", e);
            }
        }))
    } else {
        None
    };

    // gRPC server for direct ingestion (when Kafka disabled)
    let grpc_handle = {
        let p = pipeline.clone();
        let addr = cfg.grpc_addr.parse()?;
        tokio::spawn(async move {
            if let Err(e) = server::grpc::serve(p, addr).await {
                error!("gRPC server error: {}", e);
            }
        })
    };

    // HTTP server: health + metrics
    let http_handle = {
        let addr = cfg.http_addr.parse()?;
        tokio::spawn(async move {
            if let Err(e) = server::http::serve(addr).await {
                error!("HTTP server error: {}", e);
            }
        })
    };

    info!(
        grpc = cfg.grpc_addr,
        http = cfg.http_addr,
        kafka = cfg.kafka_enabled,
        "af-core ready"
    );

    // Wait for shutdown signal
    tokio::signal::ctrl_c().await?;
    info!("Shutdown signal received, draining pipeline...");

    // Graceful shutdown
    pipeline.shutdown().await;
    grpc_handle.abort();
    http_handle.abort();
    if let Some(h) = kafka_handle { h.abort(); }

    info!("af-core stopped cleanly");
    Ok(())
}
