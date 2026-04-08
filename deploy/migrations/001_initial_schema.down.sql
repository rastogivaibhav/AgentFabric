-- Govagn — Reverse migration 001
-- Drops all objects created by 001_initial_schema.up.sql in strict reverse
-- dependency order so foreign-key constraints are never violated.

-- ─── Leaf tables (no outgoing FKs to later tables) ────────────────────────

DROP TABLE IF EXISTS environments;

DROP INDEX IF EXISTS idx_users_email;
DROP INDEX IF EXISTS idx_users_tenant;
DROP TABLE IF EXISTS users;

DROP INDEX IF EXISTS idx_api_keys_hash;
DROP TABLE IF EXISTS api_keys;

DROP TABLE IF EXISTS model_pricing;
DROP TABLE IF EXISTS run_feedback;

DROP INDEX IF EXISTS idx_topology_trace;
DROP TABLE IF EXISTS agent_topology;

-- ─── Policy audit log (rules + role + grants must go first) ───────────────

DROP RULE IF EXISTS no_delete_audit ON policy_audit_log;
DROP RULE IF EXISTS no_update_audit ON policy_audit_log;
REVOKE INSERT ON policy_audit_log FROM af_audit_writer;
REVOKE USAGE ON SEQUENCE policy_audit_log_id_seq FROM af_audit_writer;
DROP INDEX IF EXISTS idx_audit_result;
DROP INDEX IF EXISTS idx_audit_trace;
DROP INDEX IF EXISTS idx_audit_tenant_time;
DROP TABLE IF EXISTS policy_audit_log;
DROP ROLE IF EXISTS af_audit_writer;

-- ─── Span metadata ────────────────────────────────────────────────────────

DROP INDEX IF EXISTS idx_spans_time;
DROP INDEX IF EXISTS idx_spans_run;
DROP INDEX IF EXISTS idx_spans_tenant;
DROP INDEX IF EXISTS idx_spans_trace;
DROP TABLE IF EXISTS span_metadata;

-- ─── Agent runs ───────────────────────────────────────────────────────────

DROP INDEX IF EXISTS idx_runs_environment;
DROP INDEX IF EXISTS idx_runs_status;
DROP INDEX IF EXISTS idx_runs_parent;
DROP INDEX IF EXISTS idx_runs_model;
DROP INDEX IF EXISTS idx_runs_framework;
DROP INDEX IF EXISTS idx_runs_trace;
DROP INDEX IF EXISTS idx_runs_tenant_time;
DROP TABLE IF EXISTS agent_runs;

-- ─── Root table ───────────────────────────────────────────────────────────

DROP TABLE IF EXISTS tenants;

-- ─── Extensions ───────────────────────────────────────────────────────────

DROP EXTENSION IF EXISTS "pgcrypto";
DROP EXTENSION IF EXISTS "uuid-ossp";
