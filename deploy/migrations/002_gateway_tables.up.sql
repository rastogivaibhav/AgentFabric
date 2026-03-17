-- deploy/migrations/002_gateway_tables.up.sql
-- Gateway hot-path operational tables (spans, runs, feedback).
--
-- These are the tables queried by api-gateway/internal/store/postgres.go.
-- They use TEXT tenant_id keys (vs UUID in migration 001) to match the
-- OTLP string tenant identifiers forwarded by the collector.
--
-- Migration 001 owns: tenants, agent_runs (UUID-keyed), span_metadata,
--                     policy_audit_log, users, environments, etc.
-- Migration 002 owns: spans, runs, feedback (TEXT-keyed, gateway hot path).

-- ─── spans ────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS spans (
    span_id         TEXT        NOT NULL,
    trace_id        TEXT        NOT NULL,
    parent_span_id  TEXT,
    run_id          TEXT        NOT NULL DEFAULT '',
    name            TEXT        NOT NULL,
    framework       TEXT        NOT NULL DEFAULT 'unknown',
    start_time_ns   BIGINT      NOT NULL,
    duration_ns     BIGINT      NOT NULL DEFAULT 0,
    status_code     SMALLINT    NOT NULL DEFAULT 0,
    status_msg      TEXT,
    attributes      JSONB       NOT NULL DEFAULT '{}',
    events          JSONB       NOT NULL DEFAULT '[]',
    input_tokens    BIGINT      NOT NULL DEFAULT 0,
    output_tokens   BIGINT      NOT NULL DEFAULT 0,
    cost_usd        NUMERIC(12,8) NOT NULL DEFAULT 0,
    tenant_id       TEXT        NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001',
    received_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (span_id, tenant_id)
);

CREATE INDEX IF NOT EXISTS idx_spans_trace     ON spans(trace_id, tenant_id);
CREATE INDEX IF NOT EXISTS idx_spans_run       ON spans(run_id, tenant_id);
CREATE INDEX IF NOT EXISTS idx_spans_framework ON spans(framework, tenant_id, received_at DESC);
CREATE INDEX IF NOT EXISTS idx_spans_time      ON spans(received_at DESC, tenant_id);

-- ─── runs ─────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS runs (
    run_id          TEXT        NOT NULL,
    trace_id        TEXT        NOT NULL,
    parent_run_id   TEXT,
    framework       TEXT        NOT NULL DEFAULT 'unknown',
    agent_name      TEXT        NOT NULL DEFAULT 'unknown',
    model           TEXT        NOT NULL DEFAULT '',
    start_time      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    end_time        TIMESTAMPTZ,
    status          TEXT        NOT NULL DEFAULT 'running',
    total_tokens    BIGINT      NOT NULL DEFAULT 0,
    total_cost_usd  NUMERIC(12,8) NOT NULL DEFAULT 0,
    metadata        JSONB       NOT NULL DEFAULT '{}',
    tenant_id       TEXT        NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001',
    PRIMARY KEY (run_id, tenant_id)
);

CREATE INDEX IF NOT EXISTS idx_runs_trace     ON runs(trace_id, tenant_id);
CREATE INDEX IF NOT EXISTS idx_runs_framework ON runs(framework, tenant_id, start_time DESC);
CREATE INDEX IF NOT EXISTS idx_runs_agent     ON runs(agent_name, tenant_id, start_time DESC);

-- ─── feedback ─────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS feedback (
    id          BIGSERIAL   PRIMARY KEY,
    run_id      TEXT        NOT NULL,
    score       SMALLINT,
    comment     TEXT,
    tenant_id   TEXT        NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_feedback_run ON feedback(run_id, tenant_id);
