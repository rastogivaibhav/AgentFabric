DROP TABLE IF EXISTS pricing_rule_audit;

DROP INDEX IF EXISTS idx_pricing_rules_lookup;

ALTER TABLE pricing_rules
    DROP COLUMN IF EXISTS description,
    DROP COLUMN IF EXISTS effective_to,
    DROP COLUMN IF EXISTS effective_from,
    DROP COLUMN IF EXISTS priority,
    DROP COLUMN IF EXISTS active,
    DROP COLUMN IF EXISTS tenant_id;

CREATE UNIQUE INDEX IF NOT EXISTS idx_pricing_rules_provider_model
    ON pricing_rules (provider, model_pattern);
