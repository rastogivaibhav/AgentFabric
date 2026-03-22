CREATE TABLE IF NOT EXISTS eval_runs (
  id BIGSERIAL PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  trace_id TEXT NOT NULL,
  release_tag TEXT NOT NULL DEFAULT '',
  eval_suite TEXT NOT NULL DEFAULT 'core-release',
  overall_score DOUBLE PRECISION NOT NULL DEFAULT 0,
  risk_level TEXT NOT NULL DEFAULT 'unknown',
  summary TEXT NOT NULL DEFAULT '',
  policy_effectiveness JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_eval_runs_tenant_created_at ON eval_runs (tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_eval_runs_release_suite ON eval_runs (tenant_id, release_tag, eval_suite, created_at DESC);

CREATE TABLE IF NOT EXISTS eval_scores (
  id BIGSERIAL PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  eval_run_id BIGINT NOT NULL REFERENCES eval_runs(id) ON DELETE CASCADE,
  metric_name TEXT NOT NULL,
  score DOUBLE PRECISION NOT NULL DEFAULT 0,
  weight DOUBLE PRECISION NOT NULL DEFAULT 0,
  severity TEXT NOT NULL DEFAULT '',
  summary TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_eval_scores_run_metric ON eval_scores (tenant_id, eval_run_id, metric_name);
