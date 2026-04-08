-- deploy/migrations/0016_decision_records.up.sql
-- Missing table required for the PROTECT / Decisions stack.

CREATE TABLE IF NOT EXISTS decision_records (
    id                  BIGSERIAL   PRIMARY KEY,
    decision_id         UUID        NOT NULL DEFAULT gen_random_uuid(),
    tenant_id           TEXT        NOT NULL,
    trace_id            TEXT,
    span_id             TEXT,
    decision_type       TEXT        NOT NULL, -- 'policy', 'budget', 'routing', etc.
    result              TEXT        NOT NULL, -- 'allow', 'deny', 'warn', 'redact'
    reason              TEXT,
    explanation         TEXT,
    trigger             TEXT,
    inputs              JSONB       NOT NULL DEFAULT '{}',
    evidence            JSONB       NOT NULL DEFAULT '{}',
    action_taken        TEXT,
    source              TEXT,
    framework           TEXT,
    app_name            TEXT,
    environment         TEXT,
    provider            TEXT,
    model               TEXT,
    prompt_id           TEXT,
    prompt_release_tag  TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_decision_records_tenant ON decision_records(tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_decision_records_trace  ON decision_records(trace_id, tenant_id);
CREATE INDEX IF NOT EXISTS idx_decision_records_type   ON decision_records(decision_type, tenant_id);
