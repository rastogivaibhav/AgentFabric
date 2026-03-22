ALTER TABLE policy_rules
    DROP COLUMN IF EXISTS version,
    DROP COLUMN IF EXISTS rollout_percent,
    DROP COLUMN IF EXISTS unsafe_categories,
    DROP COLUMN IF EXISTS schema_json,
    DROP COLUMN IF EXISTS guardrails;
