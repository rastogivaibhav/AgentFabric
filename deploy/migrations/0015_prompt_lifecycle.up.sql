CREATE TABLE IF NOT EXISTS prompt_versions (
  id BIGSERIAL PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  prompt_id TEXT NOT NULL,
  version_num INTEGER NOT NULL,
  environment TEXT NOT NULL DEFAULT 'development',
  release_tag TEXT NOT NULL DEFAULT '',
  content TEXT NOT NULL DEFAULT '',
  config_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  description TEXT NOT NULL DEFAULT '',
  created_by TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (tenant_id, prompt_id, version_num)
);

CREATE INDEX IF NOT EXISTS idx_prompt_versions_prompt ON prompt_versions (tenant_id, prompt_id, version_num DESC);
CREATE INDEX IF NOT EXISTS idx_prompt_versions_env ON prompt_versions (tenant_id, environment, updated_at DESC);

CREATE TABLE IF NOT EXISTS prompt_releases (
  id BIGSERIAL PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  prompt_id TEXT NOT NULL,
  environment TEXT NOT NULL,
  version_num INTEGER NOT NULL,
  release_tag TEXT NOT NULL,
  notes TEXT NOT NULL DEFAULT '',
  promoted_by TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (tenant_id, prompt_id, environment),
  FOREIGN KEY (tenant_id, prompt_id, version_num)
    REFERENCES prompt_versions (tenant_id, prompt_id, version_num)
    ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_prompt_releases_lookup ON prompt_releases (tenant_id, prompt_id, environment);
