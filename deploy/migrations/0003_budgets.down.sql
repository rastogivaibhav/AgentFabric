-- Rollback Layer 1b: Budget hard-limit enforcement

DROP INDEX IF EXISTS idx_usage_alerts_tenant_time;
DROP TABLE IF EXISTS usage_alerts;
DROP TABLE IF EXISTS tenant_budgets;
