CREATE TABLE IF NOT EXISTS policy_rules (
    id            BIGSERIAL PRIMARY KEY,
    tenant_id     TEXT NULL,
    name          TEXT NOT NULL,
    rule_type     TEXT NOT NULL CHECK (rule_type IN ('traffic', 'dlp')),
    enabled       BOOLEAN NOT NULL DEFAULT TRUE,
    priority      INTEGER NOT NULL DEFAULT 100,
    action        TEXT NOT NULL CHECK (action IN ('allow', 'warn', 'redact', 'deny')),
    provider      TEXT NOT NULL DEFAULT '',
    model_pattern TEXT NOT NULL DEFAULT '',
    environment   TEXT NOT NULL DEFAULT '',
    max_tokens    BIGINT NOT NULL DEFAULT 0,
    detector      TEXT NOT NULL DEFAULT '',
    scope         TEXT NOT NULL DEFAULT 'both' CHECK (scope IN ('request', 'response', 'both')),
    description   TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_policy_rules_lookup
    ON policy_rules (tenant_id, rule_type, enabled, priority DESC, provider, model_pattern);

CREATE TABLE IF NOT EXISTS control_plane_audit (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   TEXT NOT NULL DEFAULT '',
    actor       TEXT NOT NULL DEFAULT '',
    category    TEXT NOT NULL,
    action      TEXT NOT NULL,
    target_type TEXT NOT NULL,
    target_id   TEXT NOT NULL DEFAULT '',
    outcome     TEXT NOT NULL DEFAULT 'success',
    details     JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_control_plane_audit_tenant_created_at
    ON control_plane_audit (tenant_id, created_at DESC);
