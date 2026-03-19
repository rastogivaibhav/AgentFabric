use axum::{routing::get, Json, Router};
use serde_json::{json, Value};
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;
use tracing::info;

/// Shared counters exposed on the /metrics endpoint.
pub struct Counters {
    pub spans_consumed: AtomicU64,
    pub spans_allowed: AtomicU64,
    pub spans_review: AtomicU64,
    pub ch_batches: AtomicU64,
    pub ch_errors: AtomicU64,
}

impl Counters {
    pub fn new() -> Arc<Self> {
        Arc::new(Self {
            spans_consumed: AtomicU64::new(0),
            spans_allowed: AtomicU64::new(0),
            spans_review: AtomicU64::new(0),
            ch_batches: AtomicU64::new(0),
            ch_errors: AtomicU64::new(0),
        })
    }
}

/// Start the HTTP management server on `addr` (e.g. "0.0.0.0:8889").
/// Exposes:
///   GET /health  — liveness probe (always 200 {"status":"ok"})
///   GET /metrics — Prometheus-style text metrics
///   GET /ready   — readiness probe (same as health for now)
pub async fn run_health_server(addr: &str, counters: Arc<Counters>) {
    let counters_clone = counters.clone();

    let app = Router::new()
        .route("/health", get(health))
        .route("/ready", get(health))
        .route(
            "/metrics",
            get(move || {
                let c = counters_clone.clone();
                async move { metrics_handler(c).await }
            }),
        );

    info!("af-core management server listening on {}", addr);
    let listener = tokio::net::TcpListener::bind(addr)
        .await
        .expect("bind management server");
    axum::serve(listener, app).await.expect("management server");
}

async fn health() -> Json<Value> {
    Json(json!({"status": "ok", "service": "af-core"}))
}

async fn metrics_handler(c: Arc<Counters>) -> String {
    format!(
        "# HELP afcore_spans_consumed_total Total spans consumed from Kafka\n\
         # TYPE afcore_spans_consumed_total counter\n\
         afcore_spans_consumed_total {}\n\
         # HELP afcore_spans_allowed_total Spans that passed policy evaluation\n\
         # TYPE afcore_spans_allowed_total counter\n\
         afcore_spans_allowed_total {}\n\
         # HELP afcore_spans_review_total Spans flagged for review\n\
         # TYPE afcore_spans_review_total counter\n\
         afcore_spans_review_total {}\n\
         # HELP afcore_ch_batches_total ClickHouse batch inserts attempted\n\
         # TYPE afcore_ch_batches_total counter\n\
         afcore_ch_batches_total {}\n\
         # HELP afcore_ch_errors_total ClickHouse insert failures\n\
         # TYPE afcore_ch_errors_total counter\n\
         afcore_ch_errors_total {}\n",
        c.spans_consumed.load(Ordering::Relaxed),
        c.spans_allowed.load(Ordering::Relaxed),
        c.spans_review.load(Ordering::Relaxed),
        c.ch_batches.load(Ordering::Relaxed),
        c.ch_errors.load(Ordering::Relaxed),
    )
}
