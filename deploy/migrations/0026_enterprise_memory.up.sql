CREATE TABLE IF NOT EXISTS control_history (
    id BIGSERIAL PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    category TEXT NOT NULL,
    action TEXT NOT NULL,
    target_type TEXT NOT NULL,
    target_id TEXT NOT NULL DEFAULT '',
    actor TEXT NOT NULL DEFAULT '',
    reason TEXT NOT NULL DEFAULT '',
    outcome TEXT NOT NULL DEFAULT 'success',
    before_state JSONB,
    after_state JSONB,
    evidence_refs JSONB NOT NULL DEFAULT '[]'::jsonb,
    previous_hash TEXT NOT NULL DEFAULT 'genesis',
    entry_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_control_history_tenant_created
    ON control_history (tenant_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_control_history_tenant_category_created
    ON control_history (tenant_id, category, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_control_history_tenant_target_created
    ON control_history (tenant_id, target_type, target_id, created_at DESC);

CREATE TABLE IF NOT EXISTS evidence_bundles (
    id BIGSERIAL PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    name TEXT NOT NULL,
    scope TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'ready',
    filters JSONB NOT NULL DEFAULT '{}'::jsonb,
    summary JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_evidence_bundles_tenant_created
    ON evidence_bundles (tenant_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_evidence_bundles_tenant_scope_created
    ON evidence_bundles (tenant_id, scope, created_at DESC);

CREATE TABLE IF NOT EXISTS evidence_bundle_items (
    id BIGSERIAL PRIMARY KEY,
    evidence_bundle_id BIGINT NOT NULL REFERENCES evidence_bundles(id) ON DELETE CASCADE,
    tenant_id TEXT NOT NULL,
    item_type TEXT NOT NULL,
    item_title TEXT NOT NULL,
    trace_id TEXT NOT NULL DEFAULT '',
    target_type TEXT NOT NULL DEFAULT '',
    target_id TEXT NOT NULL DEFAULT '',
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_evidence_bundle_items_bundle_created
    ON evidence_bundle_items (evidence_bundle_id, created_at ASC, id ASC);

CREATE INDEX IF NOT EXISTS idx_evidence_bundle_items_tenant_type_created
    ON evidence_bundle_items (tenant_id, item_type, created_at DESC);
