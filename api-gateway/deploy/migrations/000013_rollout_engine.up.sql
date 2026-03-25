CREATE TABLE IF NOT EXISTS rollout_rules (
    id BIGSERIAL PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    name TEXT NOT NULL,
    target_type TEXT NOT NULL,
    target_id TEXT NOT NULL DEFAULT '',
    environment TEXT NOT NULL DEFAULT '',
    percentage INTEGER NOT NULL DEFAULT 0,
    control_model TEXT NOT NULL DEFAULT '',
    candidate_model TEXT NOT NULL DEFAULT '',
    control_release_tag TEXT NOT NULL DEFAULT '',
    candidate_release_tag TEXT NOT NULL DEFAULT '',
    policy_rule_id BIGINT NOT NULL DEFAULT 0,
    conditions JSONB NOT NULL DEFAULT '{}'::jsonb,
    rollback_criteria JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL DEFAULT 'active',
    created_by TEXT NOT NULL DEFAULT '',
    updated_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS rollout_rules_tenant_status_idx
    ON rollout_rules (tenant_id, status, target_type, environment, updated_at DESC);

CREATE TABLE IF NOT EXISTS rollout_events (
    id BIGSERIAL PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    rollout_rule_id BIGINT NOT NULL REFERENCES rollout_rules(id) ON DELETE CASCADE,
    trace_id TEXT,
    span_id TEXT,
    target_type TEXT NOT NULL,
    assigned_variant TEXT NOT NULL,
    assignment_key TEXT NOT NULL DEFAULT '',
    provider TEXT NOT NULL DEFAULT '',
    model TEXT NOT NULL DEFAULT '',
    environment TEXT NOT NULL DEFAULT '',
    prompt_id TEXT NOT NULL DEFAULT '',
    prompt_release_tag TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT '',
    status_code INTEGER NOT NULL DEFAULT 0,
    cost_usd DOUBLE PRECISION NOT NULL DEFAULT 0,
    latency_ms BIGINT NOT NULL DEFAULT 0,
    error_rate_snapshot DOUBLE PRECISION NOT NULL DEFAULT 0,
    auto_paused BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS rollout_events_rule_created_idx
    ON rollout_events (rollout_rule_id, created_at DESC);

CREATE INDEX IF NOT EXISTS rollout_events_tenant_target_idx
    ON rollout_events (tenant_id, target_type, created_at DESC);
