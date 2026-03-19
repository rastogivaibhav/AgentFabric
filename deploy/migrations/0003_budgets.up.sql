-- Layer 1b: Budget hard-limit enforcement
-- Creates tables for tenant budgets and usage alerts

CREATE TABLE IF NOT EXISTS tenant_budgets (
    tenant_id        TEXT          PRIMARY KEY,
    monthly_tokens   BIGINT        NOT NULL DEFAULT 0,
    monthly_cost_usd NUMERIC(10,4) NOT NULL DEFAULT 0,
    alert_threshold  NUMERIC(5,2)  NOT NULL DEFAULT 0.80,
    hard_limit       BOOLEAN       NOT NULL DEFAULT true,
    reset_day        INT           NOT NULL DEFAULT 1,
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS usage_alerts (
    id              BIGSERIAL     PRIMARY KEY,
    tenant_id       TEXT          NOT NULL,
    alert_type      TEXT          NOT NULL,
    tokens_used     BIGINT,
    cost_used_usd   NUMERIC(10,4),
    budget_tokens   BIGINT,
    budget_cost_usd NUMERIC(10,4),
    triggered_at    TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_usage_alerts_tenant_time
    ON usage_alerts(tenant_id, triggered_at DESC);
