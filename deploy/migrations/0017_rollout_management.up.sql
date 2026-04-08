-- deploy/migrations/0017_rollout_management.up.sql
-- Missing tables required for the CONTROL / Rollouts stack.

CREATE TABLE IF NOT EXISTS rollout_rules (
    id                      BIGSERIAL       PRIMARY KEY,
    tenant_id               TEXT            NOT NULL,
    name                    TEXT            NOT NULL,
    target_type             TEXT            NOT NULL, -- 'model', 'prompt', 'agent'
    target_id               TEXT,
    environment             TEXT,
    percentage              INTEGER         NOT NULL DEFAULT 0,
    control_model           TEXT,
    candidate_model         TEXT,
    control_release_tag     TEXT,
    candidate_release_tag   TEXT,
    policy_rule_id          BIGINT,
    conditions              JSONB           NOT NULL DEFAULT '{}',
    rollback_criteria       JSONB           NOT NULL DEFAULT '{}',
    status                  TEXT            NOT NULL DEFAULT 'active', -- 'active', 'paused', 'completed', 'rolled_back'
    created_by              TEXT,
    updated_by              TEXT,
    created_at              TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ     NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS rollout_events (
    id                      BIGSERIAL       PRIMARY KEY,
    tenant_id               TEXT            NOT NULL,
    rollout_rule_id         BIGINT          NOT NULL REFERENCES rollout_rules(id) ON DELETE CASCADE,
    trace_id                TEXT,
    span_id                 TEXT,
    target_type             TEXT,
    assigned_variant        TEXT            NOT NULL, -- 'control', 'canary'
    assignment_key          TEXT,
    provider                TEXT,
    model                   TEXT,
    environment             TEXT,
    prompt_id               TEXT,
    prompt_release_tag      TEXT,
    status                  TEXT,
    status_code             INTEGER,
    cost_usd                NUMERIC(12,8),
    latency_ms              BIGINT,
    error_rate_snapshot     DOUBLE PRECISION,
    auto_paused             BOOLEAN         NOT NULL DEFAULT FALSE,
    created_at              TIMESTAMPTZ     NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_rollout_rules_tenant    ON rollout_rules(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_rollout_events_rule      ON rollout_events(rollout_rule_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_rollout_events_tenant    ON rollout_events(tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_rollout_events_trace     ON rollout_events(trace_id);
