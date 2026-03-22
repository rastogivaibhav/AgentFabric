CREATE TABLE IF NOT EXISTS gateway_routes (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID NULL,
    provider TEXT NOT NULL,
    source_model_pattern TEXT NOT NULL,
    target_model TEXT NOT NULL,
    priority INTEGER NOT NULL DEFAULT 100,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_gateway_routes_provider
    ON gateway_routes (provider, priority DESC);

CREATE INDEX IF NOT EXISTS idx_gateway_routes_tenant_provider
    ON gateway_routes (tenant_id, provider, priority DESC);

CREATE TABLE IF NOT EXISTS gateway_cache_policies (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID NULL,
    provider TEXT NOT NULL,
    model_pattern TEXT NOT NULL,
    ttl_seconds INTEGER NOT NULL DEFAULT 90,
    max_entry_bytes INTEGER NOT NULL DEFAULT 524288,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_gateway_cache_policies_provider
    ON gateway_cache_policies (provider);

CREATE INDEX IF NOT EXISTS idx_gateway_cache_policies_tenant_provider
    ON gateway_cache_policies (tenant_id, provider);
