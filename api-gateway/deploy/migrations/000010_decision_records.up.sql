CREATE TABLE IF NOT EXISTS decision_records (
    id BIGSERIAL PRIMARY KEY,
    decision_id TEXT NOT NULL,
    tenant_id UUID NOT NULL,
    trace_id TEXT,
    span_id TEXT,
    decision_type TEXT NOT NULL,
    result TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    explanation TEXT NOT NULL DEFAULT '',
    trigger TEXT NOT NULL DEFAULT '',
    inputs JSONB NOT NULL DEFAULT '{}'::jsonb,
    evidence JSONB NOT NULL DEFAULT '{}'::jsonb,
    action_taken TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT '',
    framework TEXT NOT NULL DEFAULT '',
    app_name TEXT NOT NULL DEFAULT '',
    environment TEXT NOT NULL DEFAULT '',
    provider TEXT NOT NULL DEFAULT '',
    model TEXT NOT NULL DEFAULT '',
    prompt_id TEXT NOT NULL DEFAULT '',
    prompt_release_tag TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_decision_records_tenant_created_at
    ON decision_records (tenant_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_decision_records_trace
    ON decision_records (tenant_id, trace_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_decision_records_span
    ON decision_records (tenant_id, span_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_decision_records_type
    ON decision_records (tenant_id, decision_type, created_at DESC);
