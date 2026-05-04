-- Create table to track Kafka consumer group offsets
-- Used for monitoring and recovery of consumer position

CREATE TABLE IF NOT EXISTS kafka_offsets (
    id BIGSERIAL PRIMARY KEY,
    group_id VARCHAR(255) NOT NULL,
    topic VARCHAR(255) NOT NULL,
    partition INT NOT NULL,
    consumer_offset BIGINT NOT NULL,
    committed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    -- Unique constraint: one entry per consumer group + topic + partition
    UNIQUE(group_id, topic, partition)
);

-- Create indexes for fast lookups
CREATE INDEX IF NOT EXISTS idx_kafka_offsets_group
    ON kafka_offsets(group_id);

CREATE INDEX IF NOT EXISTS idx_kafka_offsets_topic
    ON kafka_offsets(topic);

CREATE INDEX IF NOT EXISTS idx_kafka_offsets_committed
    ON kafka_offsets(committed_at DESC);

-- Create a view for monitoring consumer lag
CREATE OR REPLACE VIEW kafka_consumer_status AS
SELECT
    group_id,
    topic,
    COUNT(DISTINCT partition) as partitions_tracked,
    MIN(consumer_offset) as min_offset,
    MAX(consumer_offset) as max_offset,
    AVG(consumer_offset) as avg_offset,
    MAX(committed_at) as last_commit_time,
    EXTRACT(EPOCH FROM (NOW() - MAX(committed_at)))::INT as seconds_since_commit
FROM kafka_offsets
GROUP BY group_id, topic;
