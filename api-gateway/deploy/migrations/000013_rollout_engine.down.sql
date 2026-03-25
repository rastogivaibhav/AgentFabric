DROP INDEX IF EXISTS rollout_events_tenant_target_idx;
DROP INDEX IF EXISTS rollout_events_rule_created_idx;
DROP TABLE IF EXISTS rollout_events;

DROP INDEX IF EXISTS rollout_rules_tenant_status_idx;
DROP TABLE IF EXISTS rollout_rules;
