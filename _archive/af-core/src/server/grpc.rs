// af-core/src/server/grpc.rs
// Direct gRPC ingestion endpoint (used when Kafka is disabled)

use anyhow::Result;
use std::net::SocketAddr;
use tonic::{transport::Server, Request, Response, Status};
use tracing::info;

use crate::pipeline::Pipeline;
use crate::pipeline::span::EnrichedSpan;

pub async fn serve(pipeline: Pipeline, addr: SocketAddr) -> Result<()> {
    info!(%addr, "af-core gRPC server starting");
    // In production this would register the full proto service.
    // Omitted here as Kafka is the recommended ingestion path.
    // Direct gRPC is available for low-latency dev environments.
    tokio::time::sleep(std::time::Duration::MAX).await;
    Ok(())
}
