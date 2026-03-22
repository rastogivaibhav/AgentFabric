DROP INDEX IF EXISTS idx_policy_rules_decision_mode;

ALTER TABLE policy_rules
    DROP COLUMN IF EXISTS rego_module,
    DROP COLUMN IF EXISTS rule_conditions,
    DROP COLUMN IF EXISTS decision_mode;
