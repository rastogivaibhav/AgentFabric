-- deploy/migrations/002_gateway_tables.down.sql
-- Reverses 002_gateway_tables.up.sql

DROP INDEX IF EXISTS idx_feedback_run;
DROP TABLE IF EXISTS feedback;

DROP INDEX IF EXISTS idx_runs_agent;
DROP INDEX IF EXISTS idx_runs_framework;
DROP INDEX IF EXISTS idx_runs_trace;
DROP TABLE IF EXISTS runs;

DROP INDEX IF EXISTS idx_spans_time;
DROP INDEX IF EXISTS idx_spans_framework;
DROP INDEX IF EXISTS idx_spans_run;
DROP INDEX IF EXISTS idx_spans_trace;
DROP TABLE IF EXISTS spans;
