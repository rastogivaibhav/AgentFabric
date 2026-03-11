-- AgentFabric ClickHouse Schema
-- High-performance time-series storage for spans and metrics

-- Create fabric user with password
CREATE USER IF NOT EXISTS fabric IDENTIFIED BY 'fabric_dev_only' HOST ANY;
GRANT ALL ON *.* TO fabric;

CREATE DATABASE IF NOT EXISTS agentfabric;

USE agentfabric;

-- ─── Main spans table (ReplacingMergeTree for upserts) ────────────────────
CREATE TABLE IF NOT EXISTS spans (
    -- Identity
    trace_id        FixedString(32),
    span_id         FixedString(16),
    parent_span_id  FixedString(16) DEFAULT '',
    tenant_id       FixedString(36),

    -- Timing
    start_ns        UInt64,
    end_ns          UInt64,
    duration_ms     Float64 MATERIALIZED (end_ns - start_ns) / 1000000.0,
    start_date      Date    MATERIALIZED toDate(fromUnixTimestamp64Nano(start_ns)),

    -- Classification
    name            LowCardinality(String),
    framework       LowCardinality(String),
    span_kind       LowCardinality(String),
    status_code     UInt8   DEFAULT 0,
    status_message  String  DEFAULT '',

    -- Agent context
    model           LowCardinality(String) DEFAULT '',
    agent_name      String  DEFAULT '',
    agent_role      LowCardinality(String) DEFAULT '',
    tool_name       LowCardinality(String) DEFAULT '',
    tool_status     LowCardinality(String) DEFAULT '',
    run_id          FixedString(36) DEFAULT '',

    -- Tokens & cost
    input_tokens    UInt32  DEFAULT 0,
    output_tokens   UInt32  DEFAULT 0,
    total_tokens    UInt32  MATERIALIZED input_tokens + output_tokens,
    cost_input_usd  Float64 DEFAULT 0,
    cost_output_usd Float64 DEFAULT 0,
    cost_total_usd  Float64 MATERIALIZED cost_input_usd + cost_output_usd,

    -- Infrastructure
    service_name    LowCardinality(String) DEFAULT '',
    environment     LowCardinality(String) DEFAULT '',
    cloud_region    LowCardinality(String) DEFAULT '',
    host_name       String  DEFAULT '',

    -- Policy
    policy_decision LowCardinality(String) DEFAULT 'pending',
    pii_detected    UInt8   DEFAULT 0,

    -- Raw attributes (sanitised, max 4KB per value)
    attributes      Map(String, String),

    -- Metadata
    ingested_at     DateTime64(3) DEFAULT now64()

) ENGINE = ReplacingMergeTree(ingested_at)
PARTITION BY (tenant_id, toYYYYMM(start_date))
ORDER BY (tenant_id, trace_id, span_id)
SETTINGS index_granularity = 8192, ttl_only_drop_parts = 1;

-- TTL: hot=90 days then delete (cold tier handled by S3 backup job)
ALTER TABLE spans MODIFY TTL start_date + INTERVAL 90 DAY DELETE;

-- ─── Materialized views for fast aggregations ─────────────────────────────

-- Per-model token usage by hour
CREATE MATERIALIZED VIEW IF NOT EXISTS mv_token_usage_hourly
ENGINE = SummingMergeTree()
PARTITION BY toYYYYMM(hour)
ORDER BY (tenant_id, hour, model, framework)
AS SELECT
    tenant_id,
    toStartOfHour(fromUnixTimestamp64Nano(start_ns)) AS hour,
    model,
    framework,
    count()             AS span_count,
    sum(input_tokens)   AS total_input_tokens,
    sum(output_tokens)  AS total_output_tokens,
    sum(cost_total_usd) AS total_cost_usd
FROM spans
GROUP BY tenant_id, hour, model, framework;

-- Per-framework latency distribution by day
CREATE MATERIALIZED VIEW IF NOT EXISTS mv_latency_daily
ENGINE = AggregatingMergeTree()
PARTITION BY toYYYYMM(day)
ORDER BY (tenant_id, day, framework, span_kind)
AS SELECT
    tenant_id,
    toDate(fromUnixTimestamp64Nano(start_ns)) AS day,
    framework,
    span_kind,
    count()                                          AS span_count,
    quantileState(0.50)(duration_ms)                AS p50_state,
    quantileState(0.95)(duration_ms)                AS p95_state,
    quantileState(0.99)(duration_ms)                AS p99_state,
    avg(duration_ms)                                AS avg_duration_ms,
    countIf(status_code > 0)                        AS error_count
FROM spans
GROUP BY tenant_id, day, framework, span_kind;

-- Error rate by service by hour
CREATE MATERIALIZED VIEW IF NOT EXISTS mv_error_rate_hourly
ENGINE = SummingMergeTree()
PARTITION BY toYYYYMM(hour)
ORDER BY (tenant_id, hour, service_name, framework)
AS SELECT
    tenant_id,
    toStartOfHour(fromUnixTimestamp64Nano(start_ns)) AS hour,
    service_name,
    framework,
    count()                     AS total,
    countIf(status_code > 0)    AS errors
FROM spans
GROUP BY tenant_id, hour, service_name, framework;

-- Tool usage summary
CREATE MATERIALIZED VIEW IF NOT EXISTS mv_tool_usage
ENGINE = SummingMergeTree()
PARTITION BY toYYYYMM(day)
ORDER BY (tenant_id, day, tool_name, framework)
AS SELECT
    tenant_id,
    toDate(fromUnixTimestamp64Nano(start_ns)) AS day,
    tool_name,
    framework,
    count()                              AS invocations,
    countIf(tool_status = 'success')     AS successes,
    countIf(tool_status = 'error')       AS errors,
    avg(duration_ms)                     AS avg_duration_ms
FROM spans
WHERE tool_name != ''
GROUP BY tenant_id, day, tool_name, framework;

-- Policy violation summary
CREATE MATERIALIZED VIEW IF NOT EXISTS mv_policy_violations
ENGINE = SummingMergeTree()
PARTITION BY toYYYYMM(day)
ORDER BY (tenant_id, day, policy_decision)
AS SELECT
    tenant_id,
    toDate(fromUnixTimestamp64Nano(start_ns)) AS day,
    policy_decision,
    framework,
    count() AS count
FROM spans
WHERE policy_decision != 'allow' AND policy_decision != 'pending'
GROUP BY tenant_id, day, policy_decision, framework;
