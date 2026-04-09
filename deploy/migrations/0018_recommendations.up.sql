CREATE TABLE IF NOT EXISTS recommendations (
    id                  BIGSERIAL PRIMARY KEY,
    recommendation_key  TEXT NOT NULL,
    tenant_id           TEXT NOT NULL,
    recommendation_type TEXT NOT NULL,
    status              TEXT NOT NULL DEFAULT 'open',
    title               TEXT NOT NULL DEFAULT '',
    summary             TEXT NOT NULL DEFAULT '',
    target              TEXT NOT NULL DEFAULT '',
    target_type         TEXT NOT NULL DEFAULT '',
    target_id           TEXT NOT NULL DEFAULT '',
    suggested_action    TEXT NOT NULL DEFAULT '',
    estimated_impact    TEXT NOT NULL DEFAULT '',
    blast_radius        TEXT NOT NULL DEFAULT '',
    confidence          DOUBLE PRECISION NOT NULL DEFAULT 0,
    evidence            JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, recommendation_key)
);

CREATE INDEX IF NOT EXISTS idx_recommendations_tenant_status
    ON recommendations (tenant_id, status, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_recommendations_tenant_type
    ON recommendations (tenant_id, recommendation_type, updated_at DESC);
