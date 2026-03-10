// af-core/src/server/http.rs
// Health check and Prometheus metrics endpoint

use anyhow::Result;
use axum::{routing::get, Router};
use prometheus::TextEncoder;
use std::net::SocketAddr;
use tracing::info;

pub async fn serve(addr: SocketAddr) -> Result<()> {
    let app = Router::new()
        .route("/healthz", get(health))
        .route("/readyz",  get(ready))
        .route("/metrics", get(metrics));

    let listener = tokio::net::TcpListener::bind(addr).await?;
    info!(%addr, "af-core HTTP server starting");
    axum::serve(listener, app).await?;
    Ok(())
}

async fn health() -> &'static str { "ok" }
async fn ready()  -> &'static str { "ok" }

async fn metrics() -> String {
    let encoder = TextEncoder::new();
    let metric_families = prometheus::gather();
    encoder.encode_to_string(&metric_families).unwrap_or_default()
}
