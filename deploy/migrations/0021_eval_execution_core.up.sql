CREATE TABLE IF NOT EXISTS eval_datasets (
  id BIGSERIAL PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  dataset_id TEXT NOT NULL,
  version TEXT NOT NULL,
  name TEXT NOT NULL DEFAULT '',
  dataset_type TEXT NOT NULL DEFAULT 'golden',
  description TEXT NOT NULL DEFAULT '',
  owner_name TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'draft',
  source TEXT NOT NULL DEFAULT 'custom',
  provenance TEXT NOT NULL DEFAULT '',
  redaction_status TEXT NOT NULL DEFAULT '',
  approval_status TEXT NOT NULL DEFAULT '',
  freshness_date TIMESTAMPTZ,
  tags TEXT[] NOT NULL DEFAULT '{}',
  metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (tenant_id, dataset_id, version)
);

CREATE INDEX IF NOT EXISTS idx_eval_datasets_tenant_ref ON eval_datasets (tenant_id, dataset_id, version);

CREATE TABLE IF NOT EXISTS eval_dataset_items (
  id BIGSERIAL PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  dataset_ref TEXT NOT NULL,
  item_key TEXT NOT NULL,
  input_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  expected_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  labels TEXT[] NOT NULL DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (tenant_id, dataset_ref, item_key)
);

CREATE INDEX IF NOT EXISTS idx_eval_dataset_items_dataset_ref ON eval_dataset_items (tenant_id, dataset_ref);

CREATE TABLE IF NOT EXISTS eval_executions (
  id BIGSERIAL PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  pack_id TEXT NOT NULL,
  mode TEXT NOT NULL DEFAULT 'offline',
  status TEXT NOT NULL DEFAULT 'queued',
  release_tag TEXT NOT NULL DEFAULT '',
  trace_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
  dataset_refs JSONB NOT NULL DEFAULT '[]'::jsonb,
  attributes JSONB NOT NULL DEFAULT '{}'::jsonb,
  sample_limit INTEGER NOT NULL DEFAULT 0,
  overall_score DOUBLE PRECISION NOT NULL DEFAULT 0,
  risk_level TEXT NOT NULL DEFAULT 'unknown',
  summary TEXT NOT NULL DEFAULT '',
  policy_effectiveness JSONB NOT NULL DEFAULT '{}'::jsonb,
  eval_run_id BIGINT REFERENCES eval_runs(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_eval_executions_tenant_created_at ON eval_executions (tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_eval_executions_pack_status ON eval_executions (tenant_id, pack_id, status, created_at DESC);

CREATE TABLE IF NOT EXISTS eval_execution_items (
  id BIGSERIAL PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  execution_id BIGINT NOT NULL REFERENCES eval_executions(id) ON DELETE CASCADE,
  item_ref TEXT NOT NULL,
  item_type TEXT NOT NULL DEFAULT 'trace',
  trace_id TEXT NOT NULL DEFAULT '',
  dataset_ref TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'pending',
  overall_score DOUBLE PRECISION NOT NULL DEFAULT 0,
  risk_level TEXT NOT NULL DEFAULT 'unknown',
  summary TEXT NOT NULL DEFAULT '',
  evidence_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_eval_execution_items_execution ON eval_execution_items (tenant_id, execution_id, created_at ASC);

CREATE TABLE IF NOT EXISTS eval_evaluator_results (
  id BIGSERIAL PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  execution_id BIGINT NOT NULL REFERENCES eval_executions(id) ON DELETE CASCADE,
  execution_item_id BIGINT NOT NULL REFERENCES eval_execution_items(id) ON DELETE CASCADE,
  evaluator_id TEXT NOT NULL,
  dimension_id TEXT NOT NULL DEFAULT '',
  evaluator_type TEXT NOT NULL DEFAULT '',
  method TEXT NOT NULL DEFAULT '',
  score DOUBLE PRECISION NOT NULL DEFAULT 0,
  severity TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'completed',
  summary TEXT NOT NULL DEFAULT '',
  input_fields JSONB NOT NULL DEFAULT '[]'::jsonb,
  details_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_eval_evaluator_results_item ON eval_evaluator_results (tenant_id, execution_item_id, evaluator_id);

CREATE TABLE IF NOT EXISTS eval_evidence_links (
  id BIGSERIAL PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  execution_id BIGINT NOT NULL REFERENCES eval_executions(id) ON DELETE CASCADE,
  execution_item_id BIGINT NOT NULL REFERENCES eval_execution_items(id) ON DELETE CASCADE,
  link_type TEXT NOT NULL,
  ref_id TEXT NOT NULL,
  label TEXT NOT NULL DEFAULT '',
  metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_eval_evidence_links_item ON eval_evidence_links (tenant_id, execution_item_id, link_type);
