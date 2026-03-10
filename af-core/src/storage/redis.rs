// af-core/src/storage/redis.rs
use anyhow::Result;
use redis::{aio::ConnectionManager, AsyncCommands, Client};
use serde_json;
use tracing::info;
use crate::pipeline::span::EnrichedSpan;

#[derive(Clone)]
pub struct RedisStore {
    conn: ConnectionManager,
}

impl RedisStore {
    pub async fn connect(url: &str) -> Result<Self> {
        let client = Client::open(url)?;
        let conn = ConnectionManager::new(client).await?;
        info!("Redis connected");
        Ok(Self { conn })
    }

    /// Publish spans to Redis channel for live portal WebSocket stream
    pub async fn publish_spans(&self, spans: &[EnrichedSpan]) -> Result<()> {
        if spans.is_empty() { return Ok(()); }
        let mut conn = self.conn.clone();
        let payload = serde_json::to_string(spans)?;
        conn.publish::<_, _, ()>("af:live:spans", payload).await?;
        Ok(())
    }

    /// Cache a computed value with TTL
    pub async fn set_ex(&self, key: &str, value: &str, ttl_secs: u64) -> Result<()> {
        let mut conn = self.conn.clone();
        conn.set_ex::<_, _, ()>(key, value, ttl_secs).await?;
        Ok(())
    }

    pub async fn get(&self, key: &str) -> Result<Option<String>> {
        let mut conn = self.conn.clone();
        Ok(conn.get(key).await?)
    }
}
