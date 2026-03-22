ALTER TABLE policy_rules
    ADD COLUMN IF NOT EXISTS decision_mode TEXT NOT NULL DEFAULT 'fast'
        CHECK (decision_mode IN ('fast', 'rego')),
    ADD COLUMN IF NOT EXISTS rule_conditions JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS rego_module TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_policy_rules_decision_mode
    ON policy_rules (decision_mode, rule_type, enabled, priority DESC);
