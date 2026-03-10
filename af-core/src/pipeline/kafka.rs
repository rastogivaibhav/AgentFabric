// af-core/src/pipeline/kafka.rs
// Kafka consumer — durable buffer between collector and af-core (RT-006 fix)
// at-least-once delivery with manual offset commit after successful processing

use anyhow::Result;
use rdkafka::{
    config::ClientConfig,
    consumer::{CommitMode, Consumer, StreamConsumer},
    message::Message,
};
use tracing::{error, info, warn};

use super::Pipeline;
use crate::pipeline::span::EnrichedSpan;

pub async fn run_consumer(
    pipeline: Pipeline,
    brokers: &str,
    topic: &str,
) -> Result<()> {
    let consumer: StreamConsumer = ClientConfig::new()
        .set("bootstrap.servers", brokers)
        .set("group.id", "af-core-processor")
        .set("enable.auto.commit", "false")     // Manual commit — RT-006
        .set("auto.offset.reset", "earliest")
        .set("session.timeout.ms", "30000")
        .set("max.poll.interval.ms", "300000")
        .set("fetch.min.bytes", "1")
        .set("fetch.wait.max.ms", "100")
        .create()?;

    consumer.subscribe(&[topic])?;
    info!(topic, brokers, "Kafka consumer subscribed");

    loop {
        match consumer.recv().await {
            Ok(msg) => {
                let payload = match msg.payload() {
                    Some(p) => p,
                    None => {
                        // Commit tombstone/empty messages
                        consumer.commit_message(&msg, CommitMode::Async)?;
                        continue;
                    }
                };

                // Deserialize span batch
                match serde_json::from_slice::<Vec<EnrichedSpan>>(payload) {
                    Ok(spans) => {
                        match pipeline.process_batch(spans).await {
                            Ok(_) => {
                                // Only commit AFTER successful processing (at-least-once)
                                if let Err(e) = consumer.commit_message(&msg, CommitMode::Async) {
                                    warn!("Kafka commit failed: {}", e);
                                }
                            }
                            Err(e) => {
                                error!("Batch processing failed, NOT committing offset: {}", e);
                                // Message will be redelivered on restart
                            }
                        }
                    }
                    Err(e) => {
                        warn!("Failed to deserialize span batch: {}", e);
                        // Commit bad messages to avoid infinite retry of unparseable data
                        consumer.commit_message(&msg, CommitMode::Async)?;
                    }
                }
            }
            Err(e) => {
                error!("Kafka receive error: {}", e);
                tokio::time::sleep(std::time::Duration::from_secs(1)).await;
            }
        }
    }
}
