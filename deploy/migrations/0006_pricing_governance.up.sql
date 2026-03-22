DROP INDEX IF EXISTS idx_pricing_rules_provider_model;

ALTER TABLE pricing_rules
    ADD COLUMN IF NOT EXISTS tenant_id TEXT NULL,
    ADD COLUMN IF NOT EXISTS active BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS priority INTEGER NOT NULL DEFAULT 100,
    ADD COLUMN IF NOT EXISTS effective_from TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS effective_to TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_pricing_rules_lookup
    ON pricing_rules (tenant_id, provider, model_pattern, active, priority DESC);

CREATE TABLE IF NOT EXISTS pricing_rule_audit (
    id           BIGSERIAL PRIMARY KEY,
    rule_id       BIGINT NOT NULL,
    action        TEXT NOT NULL,
    actor         TEXT NOT NULL DEFAULT '',
    tenant_id     TEXT NOT NULL DEFAULT '',
    before_state  JSONB NOT NULL DEFAULT '{}'::jsonb,
    after_state   JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_pricing_rule_audit_rule_id_created_at
    ON pricing_rule_audit (rule_id, created_at DESC);
