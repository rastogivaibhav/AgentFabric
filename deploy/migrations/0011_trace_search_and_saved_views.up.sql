CREATE TABLE IF NOT EXISTS trace_saved_views (
    id BIGSERIAL PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    filters JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by TEXT NOT NULL DEFAULT '',
    is_pinned BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_trace_saved_views_tenant_updated
    ON trace_saved_views (tenant_id, updated_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_spans_trace_search_tenant_start
    ON spans (tenant_id, start_time_ns DESC, trace_id);

CREATE INDEX IF NOT EXISTS idx_spans_trace_search_provider_model
    ON spans (
        tenant_id,
        (COALESCE(NULLIF(attributes->>'gen_ai.system', ''), 'unknown')),
        (COALESCE(NULLIF(attributes->>'gen_ai.request.model', ''), 'unknown'))
    );

CREATE INDEX IF NOT EXISTS idx_spans_trace_search_app_env
    ON spans (
        tenant_id,
        (COALESCE(NULLIF(attributes->>'service.name', ''), NULLIF(attributes->>'af.app.name', ''), 'unknown')),
        (COALESCE(NULLIF(attributes->>'deployment.environment', ''), NULLIF(attributes->>'environment', ''), 'unknown'))
    );
