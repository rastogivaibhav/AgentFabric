-- Govagn PostgreSQL Schema
-- Production-grade with Row Level Security, audit trail, and indexes

-- Enable required extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ─── Multi-tenancy ────────────────────────────────────────────────────────

CREATE TABLE tenants (
    tenant_id   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL UNIQUE,
    plan        VARCHAR(32) NOT NULL DEFAULT 'starter',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    settings    JSONB NOT NULL DEFAULT '{}'
);

INSERT INTO tenants (tenant_id, name, plan) VALUES
    ('00000000-0000-0000-0000-000000000001', 'default', 'enterprise');

-- ─── Agent Runs (RT-010 fix: tenant_id on all tables) ─────────────────────

CREATE TABLE agent_runs (
    run_id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID        NOT NULL REFERENCES tenants(tenant_id),
    trace_id        VARCHAR(32) NOT NULL,
    framework       VARCHAR(32) NOT NULL CHECK (framework IN (
                        'crewai', 'langgraph', 'google_adk',
                        'openai_agents', 'claude_agents', 'codex', 'claude_code', 'unknown')),
    agent_name      TEXT        NOT NULL DEFAULT '',
    agent_role      TEXT        NOT NULL DEFAULT '',
    model           TEXT        NOT NULL DEFAULT '',
    environment     TEXT        NOT NULL DEFAULT 'unknown',
    service_name    TEXT        NOT NULL DEFAULT '',
    host_name       TEXT        NOT NULL DEFAULT '',
    cloud_region    TEXT        NOT NULL DEFAULT '',
    start_time      TIMESTAMPTZ NOT NULL,
    end_time        TIMESTAMPTZ,
    duration_ms     DOUBLE PRECISION GENERATED ALWAYS AS (
                        EXTRACT(EPOCH FROM (end_time - start_time)) * 1000
                    ) STORED,
    status          VARCHAR(16) NOT NULL DEFAULT 'running'
                        CHECK (status IN ('running', 'success', 'error', 'timeout')),
    input_tokens    BIGINT      NOT NULL DEFAULT 0,
    output_tokens   BIGINT      NOT NULL DEFAULT 0,
    total_tokens    BIGINT      GENERATED ALWAYS AS (input_tokens + output_tokens) STORED,
    cost_input_usd  NUMERIC(14, 8) NOT NULL DEFAULT 0,
    cost_output_usd NUMERIC(14, 8) NOT NULL DEFAULT 0,
    cost_total_usd  NUMERIC(14, 8) GENERATED ALWAYS AS (cost_input_usd + cost_output_usd) STORED,
    parent_run_id   UUID        REFERENCES agent_runs(run_id),
    error_message   TEXT,
    metadata        JSONB       NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_runs_tenant_time    ON agent_runs(tenant_id, start_time DESC);
CREATE INDEX idx_runs_trace          ON agent_runs(trace_id);
CREATE INDEX idx_runs_framework      ON agent_runs(tenant_id, framework, start_time DESC);
CREATE INDEX idx_runs_model          ON agent_runs(tenant_id, model, start_time DESC);
CREATE INDEX idx_runs_parent         ON agent_runs(parent_run_id) WHERE parent_run_id IS NOT NULL;
CREATE INDEX idx_runs_status         ON agent_runs(tenant_id, status, start_time DESC);
CREATE INDEX idx_runs_environment    ON agent_runs(tenant_id, environment, start_time DESC);

-- Row Level Security (RT-010)
ALTER TABLE agent_runs ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON agent_runs
    USING (tenant_id = current_setting('app.tenant_id')::UUID);

-- ─── Span Metadata (relational, hot path) ─────────────────────────────────

CREATE TABLE span_metadata (
    span_id         VARCHAR(16)  PRIMARY KEY,
    trace_id        VARCHAR(32)  NOT NULL,
    tenant_id       UUID         NOT NULL REFERENCES tenants(tenant_id),
    run_id          UUID         REFERENCES agent_runs(run_id),
    parent_span_id  VARCHAR(16),
    name            TEXT         NOT NULL,
    framework       VARCHAR(32)  NOT NULL,
    span_kind       VARCHAR(32)  NOT NULL,
    model           TEXT         NOT NULL DEFAULT '',
    tool_name       TEXT         NOT NULL DEFAULT '',
    tool_status     VARCHAR(16)  NOT NULL DEFAULT '',
    start_ns        BIGINT       NOT NULL,
    end_ns          BIGINT       NOT NULL,
    duration_ms     DOUBLE PRECISION NOT NULL DEFAULT 0,
    status_code     SMALLINT     NOT NULL DEFAULT 0,
    status_message  TEXT         NOT NULL DEFAULT '',
    input_tokens    INT          NOT NULL DEFAULT 0,
    output_tokens   INT          NOT NULL DEFAULT 0,
    cost_total_usd  NUMERIC(12, 8) NOT NULL DEFAULT 0,
    service_name    TEXT         NOT NULL DEFAULT '',
    environment     TEXT         NOT NULL DEFAULT '',
    cloud_region    TEXT         NOT NULL DEFAULT '',
    -- Computed policy result — set by server, never by client
    policy_decision VARCHAR(16)  NOT NULL DEFAULT 'pending',
    pii_detected    BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_spans_trace    ON span_metadata(trace_id);
CREATE INDEX idx_spans_tenant   ON span_metadata(tenant_id, start_ns DESC);
CREATE INDEX idx_spans_run      ON span_metadata(run_id) WHERE run_id IS NOT NULL;
CREATE INDEX idx_spans_time     ON span_metadata(tenant_id, start_ns DESC);

ALTER TABLE span_metadata ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON span_metadata
    USING (tenant_id = current_setting('app.tenant_id')::UUID);

-- ─── Policy Audit Log (RT-009 fix: hash-chained, immutable) ───────────────

CREATE TABLE policy_audit_log (
    id              BIGSERIAL    PRIMARY KEY,
    decision_id     UUID         NOT NULL UNIQUE DEFAULT gen_random_uuid(),
    tenant_id       UUID         NOT NULL,
    trace_id        VARCHAR(32)  NOT NULL,
    span_id         VARCHAR(16)  NOT NULL,
    policy_name     TEXT         NOT NULL,
    result          VARCHAR(16)  NOT NULL CHECK (result IN ('allow', 'deny', 'warn', 'sanitize')),
    reason          TEXT         NOT NULL,
    evaluated_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    evaluated_ns    BIGINT       NOT NULL DEFAULT 0,
    -- Hash chain fields (RT-009)
    previous_hash   TEXT         NOT NULL DEFAULT 'genesis',
    entry_hash      TEXT         NOT NULL,
    -- Filled from span context
    framework       VARCHAR(32)  NOT NULL DEFAULT '',
    model           TEXT         NOT NULL DEFAULT '',
    cloud_region    TEXT         NOT NULL DEFAULT '',
    environment     TEXT         NOT NULL DEFAULT ''
) WITH (fillfactor=100);

-- No UPDATE or DELETE — ever
CREATE RULE no_update_audit AS ON UPDATE TO policy_audit_log DO INSTEAD NOTHING;
CREATE RULE no_delete_audit AS ON DELETE TO policy_audit_log DO INSTEAD NOTHING;

-- Separate role with INSERT-only access (immutable audit principle).
-- NOLOGIN by default — the role exists for GRANT enforcement.
-- Production setup: run the following after deployment, replacing the placeholder:
--   ALTER ROLE af_audit_writer WITH LOGIN PASSWORD '$GV_AUDIT_WRITER_PASSWORD';
-- Then configure the current audit writer to use GV_AUDIT_DSN for audit writes.
DO $$
BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'af_audit_writer') THEN
    CREATE ROLE af_audit_writer NOLOGIN;
  END IF;
END$$;
GRANT INSERT ON policy_audit_log TO af_audit_writer;
GRANT USAGE ON SEQUENCE policy_audit_log_id_seq TO af_audit_writer;
-- No SELECT, UPDATE, DELETE granted

CREATE INDEX idx_audit_tenant_time  ON policy_audit_log(tenant_id, evaluated_at DESC);
CREATE INDEX idx_audit_trace        ON policy_audit_log(trace_id);
CREATE INDEX idx_audit_result       ON policy_audit_log(tenant_id, result, evaluated_at DESC);

-- ─── Agent Topology (graph edges) ────────────────────────────────────────

CREATE TABLE agent_topology (
    id              BIGSERIAL    PRIMARY KEY,
    tenant_id       UUID         NOT NULL,
    trace_id        VARCHAR(32)  NOT NULL,
    source_span_id  VARCHAR(16)  NOT NULL,
    target_span_id  VARCHAR(16)  NOT NULL,
    edge_type       VARCHAR(32)  NOT NULL DEFAULT 'parent_child',
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_topology_trace ON agent_topology(trace_id);

-- ─── Human Feedback / Evals (LangSmith-style) ────────────────────────────

CREATE TABLE run_feedback (
    feedback_id     UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID         NOT NULL,
    run_id          UUID         NOT NULL REFERENCES agent_runs(run_id),
    user_id         TEXT         NOT NULL DEFAULT 'anonymous',
    score           NUMERIC(4, 2) CHECK (score BETWEEN 0 AND 1),
    value           TEXT,
    comment         TEXT,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- ─── Pricing Cache (RT-012 fix) ───────────────────────────────────────────

CREATE TABLE model_pricing (
    model           TEXT         PRIMARY KEY,
    provider        TEXT         NOT NULL,
    input_per_1m    NUMERIC(10, 6) NOT NULL,
    output_per_1m   NUMERIC(10, 6) NOT NULL,
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- Seed with known pricing (updated by pricing feed job)
INSERT INTO model_pricing VALUES
    ('claude-3-5-sonnet-20241022', 'anthropic', 3.00, 15.00, NOW()),
    ('claude-3-5-haiku-20241022',  'anthropic', 0.80,  4.00, NOW()),
    ('claude-opus-4-5',           'anthropic', 15.00, 75.00, NOW()),
    ('gpt-4o',                    'openai',    2.50, 10.00,  NOW()),
    ('gpt-4o-mini',               'openai',    0.15,  0.60,  NOW()),
    ('o1-preview',                'openai',   15.00, 60.00,  NOW()),
    ('gemini-1.5-pro',            'google',    3.50, 10.50,  NOW()),
    ('gemini-1.5-flash',          'google',    0.075, 0.30,  NOW());

-- ─── API Keys / Tenant Auth ───────────────────────────────────────────────

CREATE TABLE api_keys (
    key_id      UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID        NOT NULL REFERENCES tenants(tenant_id),
    key_hash    TEXT        NOT NULL UNIQUE,  -- sha256 of actual key
    name        TEXT        NOT NULL,
    scopes      TEXT[]      NOT NULL DEFAULT '{"read"}',
    last_used   TIMESTAMPTZ,
    expires_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at  TIMESTAMPTZ
);

CREATE INDEX idx_api_keys_hash ON api_keys(key_hash) WHERE revoked_at IS NULL;

-- ─── Users (S4: local user management) ───────────────────────────────────
-- For environments without an OIDC provider.
-- Passwords are stored as bcrypt hashes (via pgcrypto crypt/gen_salt).
-- The api-gateway checks: password_hash = crypt($input, password_hash)

CREATE TABLE users (
    user_id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID        NOT NULL REFERENCES tenants(tenant_id),
    username        TEXT        NOT NULL,
    password_hash   TEXT        NOT NULL,  -- bcrypt via pgcrypto crypt()
    email           TEXT        NOT NULL,
    display_name    TEXT        NOT NULL DEFAULT '',
    role            VARCHAR(32) NOT NULL DEFAULT 'viewer'
                        CHECK (role IN ('admin', 'editor', 'viewer')),
    is_active       BOOLEAN     NOT NULL DEFAULT TRUE,
    last_login_at   TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, username),
    UNIQUE (tenant_id, email)
);

-- Seed the default admin user (password: admin — change via GV_ADMIN_PASSWORD)
INSERT INTO users (tenant_id, username, password_hash, email, display_name, role)
VALUES (
    '00000000-0000-0000-0000-000000000001',
    'admin',
    crypt('admin', gen_salt('bf', 10)),
    'admin@govagn.local',
    'Admin',
    'admin'
);

CREATE INDEX idx_users_tenant   ON users(tenant_id, username);
CREATE INDEX idx_users_email    ON users(tenant_id, email);

-- ─── Environments ─────────────────────────────────────────────────────────

CREATE TABLE environments (
    env_id      UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID        NOT NULL REFERENCES tenants(tenant_id),
    name        TEXT        NOT NULL,
    cluster     TEXT        NOT NULL DEFAULT '',
    cloud       TEXT        NOT NULL DEFAULT '',
    region      TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, name)
);

INSERT INTO environments (tenant_id, name) VALUES
    ('00000000-0000-0000-0000-000000000001', 'development'),
    ('00000000-0000-0000-0000-000000000001', 'staging'),
    ('00000000-0000-0000-0000-000000000001', 'production');
