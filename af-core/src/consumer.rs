use anyhow::Result;
use rdkafka::config::ClientConfig;
use rdkafka::consumer::{Consumer, StreamConsumer};
use rdkafka::message::Message;
use std::sync::atomic::Ordering;
use std::sync::Arc;
use std::time::Duration;
use tokio::time::timeout;
use tracing::{error, info, warn};

use crate::{
    clickhouse::ClickHouseWriter,
    config::Config,
    policy,
    server::Counters,
    span::Span,
};

/// Run the main Kafka consumer loop.
///
/// Design:
/// - Single consumer, single goroutine (can be scaled by increasing partition count)
/// - Batches up to `cfg.batch_size` spans or waits `cfg.batch_timeout_ms` before flush
/// - Policy evaluation happens synchronously per-span (CPU-bound, ~1µs per span)
/// - ClickHouse writes are async, fail-open (errors are logged but not fatal)
/// - Kafka auto-commit is enabled — at-least-once delivery (acceptable for analytics)
pub async fn run_consumer(cfg: &Config, counters: Arc<Counters>) -> Result<()> {
    let consumer: StreamConsumer = ClientConfig::new()
        .set("bootstrap.servers", &cfg.kafka_brokers)
        .set("group.id", &cfg.kafka_group_id)
        .set("auto.offset.reset", "earliest")
        .set("enable.auto.commit", "true")
        .set("auto.commit.interval.ms", "5000")
        .set("session.timeout.ms", "30000")
        .set("heartbeat.interval.ms", "3000")
        .create()
        .map_err(|e| anyhow::anyhow!("kafka consumer create: {}", e))?;

    consumer
        .subscribe(&[cfg.kafka_topic.as_str()])
        .map_err(|e| anyhow::anyhow!("kafka subscribe: {}", e))?;

    info!(
        topic = %cfg.kafka_topic,
        brokers = %cfg.kafka_brokers,
        group = %cfg.kafka_group_id,
        "Kafka consumer subscribed"
    );

    let ch = ClickHouseWriter::new(&cfg.clickhouse_url, &cfg.clickhouse_db);
    let batch_size = cfg.batch_size;
    let batch_timeout = Duration::from_millis(cfg.batch_timeout_ms);

    let mut batch: Vec<(Span, policy::PolicyResult)> = Vec::with_capacity(batch_size);

    loop {
        // Try to receive a message within the batch timeout window.
        // When the timeout fires we flush whatever is in the buffer.
        let recv_result = timeout(batch_timeout, consumer.recv()).await;

        match recv_result {
            // Timeout — flush current batch even if not full
            Err(_elapsed) => {
                if !batch.is_empty() {
                    flush_batch(&ch, &mut batch, &counters).await;
                }
            }

            // Kafka error (transient: broker unavailable, partition rebalance, etc.)
            Ok(Err(e)) => {
                warn!("kafka recv error: {}", e);
                // Brief back-off to avoid tight error loops
                tokio::time::sleep(Duration::from_millis(500)).await;
            }

            // Message received
            Ok(Ok(msg)) => {
                counters.spans_consumed.fetch_add(1, Ordering::Relaxed);

                let payload = match msg.payload_view::<str>() {
                    Some(Ok(s)) => s,
                    Some(Err(e)) => {
                        warn!("non-utf8 kafka payload: {}", e);
                        continue;
                    }
                    None => {
                        warn!("empty kafka message payload");
                        continue;
                    }
                };

                match serde_json::from_str::<Span>(payload) {
                    Ok(span) => {
                        let policy = policy::evaluate(&span);

                        match policy.decision {
                            "allow" => {
                                counters.spans_allowed.fetch_add(1, Ordering::Relaxed)
                            }
                            _ => counters.spans_review.fetch_add(1, Ordering::Relaxed),
                        };

                        batch.push((span, policy));

                        if batch.len() >= batch_size {
                            flush_batch(&ch, &mut batch, &counters).await;
                        }
                    }
                    Err(e) => {
                        warn!(
                            offset = msg.offset(),
                            "span deserialization failed: {}",
                            e
                        );
                    }
                }
            }
        }
    }
}

/// Write the current batch to ClickHouse and clear it.
/// Errors are logged but not propagated — analytics writes are best-effort.
async fn flush_batch(
    ch: &ClickHouseWriter,
    batch: &mut Vec<(Span, policy::PolicyResult)>,
    counters: &Arc<Counters>,
) {
    if batch.is_empty() {
        return;
    }

    let n = batch.len();
    counters.ch_batches.fetch_add(1, Ordering::Relaxed);

    match ch.insert_batch(batch).await {
        Ok(()) => {
            info!(count = n, "flushed batch to ClickHouse");
        }
        Err(e) => {
            counters.ch_errors.fetch_add(1, Ordering::Relaxed);
            error!(count = n, "ClickHouse flush failed: {}", e);
            // On error we still clear the batch to avoid indefinite growth.
            // With at-least-once Kafka semantics the spans will be re-delivered
            // on restart if auto-commit has not yet fired.
        }
    }

    batch.clear();
}
