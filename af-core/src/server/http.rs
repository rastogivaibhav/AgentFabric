// af-core/src/server/http.rs
// Health check, readiness probe, and Prometheus metrics endpoint.
//
// GET /healthz — liveness: returns 200 as long as the process is running
// GET /readyz  — readiness: returns 200 {"status":"ready"} or 503 {"status":"not_ready"}
// GET /metrics — Prometheus text format

use anyhow::Result;
use axum::{http::StatusCode, response::IntoResponse, routing::get, Json, Router};
use prometheus::TextEncoder;
use serde_json::json;
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

/// Liveness probe — returns 200 as long as the process is alive.
/// Kubernetes will restart the container if this fails.
async fn health() -> impl IntoResponse {
    Json(json!({ "status": "ok", "service": "af-core", "version": env!("CARGO_PKG_VERSION") }))
}

/// Readiness probe — returns 200 only when the pipeline is operational.
/// Kubernetes will stop sending traffic until this passes.
/// Currently checks that the Prometheus registry is functional (proxy for pipeline health).
/// Sprint 5: wire actual DB ping + Kafka consumer lag check here.
async fn ready() -> impl IntoResponse {
    // Verify metrics registry responds — lightweight proxy for "am I processing"
    let families = prometheus::gather();
    if families.is_empty() {
        // Metrics not yet initialised — pipeline hasn't started
        return (
            StatusCode::SERVICE_UNAVAILABLE,
            Json(json!({ "status": "not_ready", "reason": "metrics not initialised" })),
        );
    }
    (
        StatusCode::OK,
        Json(json!({ "status": "ready", "service": "af-core" })),
    )
}

async fn metrics() -> String {
    let encoder = TextEncoder::new();
    let metric_families = prometheus::gather();
    encoder.encode_to_string(&metric_families).unwrap_or_default()
}
