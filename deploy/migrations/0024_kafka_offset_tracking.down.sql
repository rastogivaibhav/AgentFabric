-- Rollback: Drop Kafka offset tracking

DROP VIEW IF EXISTS kafka_consumer_status;

DROP INDEX IF EXISTS idx_kafka_offsets_committed;
DROP INDEX IF EXISTS idx_kafka_offsets_topic;
DROP INDEX IF EXISTS idx_kafka_offsets_group;

DROP TABLE IF EXISTS kafka_offsets;
