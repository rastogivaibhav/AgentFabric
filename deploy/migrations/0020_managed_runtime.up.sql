CREATE TABLE IF NOT EXISTS managed_agents (
  tenant_id TEXT NOT NULL,
  agent_id TEXT NOT NULL,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  model TEXT NOT NULL DEFAULT '',
  system_prompt TEXT NOT NULL DEFAULT '',
  runtime_provider TEXT NOT NULL DEFAULT 'anthropic',
  runtime_product TEXT NOT NULL DEFAULT 'claude_managed_agents',
  status TEXT NOT NULL DEFAULT 'active',
  tool_types JSONB NOT NULL DEFAULT '[]'::jsonb,
  mcp_servers JSONB NOT NULL DEFAULT '[]'::jsonb,
  skills JSONB NOT NULL DEFAULT '[]'::jsonb,
  environment_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_session_at TIMESTAMPTZ NULL,
  PRIMARY KEY (tenant_id, agent_id)
);

CREATE INDEX IF NOT EXISTS idx_managed_agents_updated ON managed_agents (tenant_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_managed_agents_status ON managed_agents (tenant_id, status, updated_at DESC);

CREATE TABLE IF NOT EXISTS managed_environments (
  tenant_id TEXT NOT NULL,
  environment_id TEXT NOT NULL,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  runtime TEXT NOT NULL DEFAULT '',
  container_template TEXT NOT NULL DEFAULT '',
  network_access TEXT NOT NULL DEFAULT '',
  mounted_files JSONB NOT NULL DEFAULT '[]'::jsonb,
  packages JSONB NOT NULL DEFAULT '[]'::jsonb,
  status TEXT NOT NULL DEFAULT 'active',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (tenant_id, environment_id)
);

CREATE INDEX IF NOT EXISTS idx_managed_environments_updated ON managed_environments (tenant_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS managed_sessions (
  tenant_id TEXT NOT NULL,
  session_id TEXT NOT NULL,
  agent_id TEXT NOT NULL,
  environment_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'running',
  current_task_id TEXT NOT NULL DEFAULT '',
  runtime_provider TEXT NOT NULL DEFAULT 'anthropic',
  runtime_product TEXT NOT NULL DEFAULT 'claude_managed_agents',
  persistent_filesystem BOOLEAN NOT NULL DEFAULT TRUE,
  conversation_turns INTEGER NOT NULL DEFAULT 0,
  event_count INTEGER NOT NULL DEFAULT 0,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_event_at TIMESTAMPTZ NULL,
  PRIMARY KEY (tenant_id, session_id),
  CONSTRAINT fk_managed_sessions_agent FOREIGN KEY (tenant_id, agent_id)
    REFERENCES managed_agents (tenant_id, agent_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_managed_sessions_agent ON managed_sessions (tenant_id, agent_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_managed_sessions_status ON managed_sessions (tenant_id, status, updated_at DESC);

CREATE TABLE IF NOT EXISTS managed_session_events (
  tenant_id TEXT NOT NULL,
  event_id TEXT NOT NULL,
  session_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  role TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT '',
  sequence_num BIGINT NOT NULL,
  summary TEXT NOT NULL DEFAULT '',
  data JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (tenant_id, event_id),
  CONSTRAINT fk_managed_events_session FOREIGN KEY (tenant_id, session_id)
    REFERENCES managed_sessions (tenant_id, session_id) ON DELETE CASCADE,
  CONSTRAINT uq_managed_events_sequence UNIQUE (tenant_id, session_id, sequence_num)
);

CREATE INDEX IF NOT EXISTS idx_managed_events_session_created ON managed_session_events (tenant_id, session_id, created_at DESC);

CREATE TABLE IF NOT EXISTS managed_tasks (
  tenant_id TEXT NOT NULL,
  task_id TEXT NOT NULL,
  session_id TEXT NOT NULL,
  parent_task_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'running',
  input_summary TEXT NOT NULL DEFAULT '',
  output_summary TEXT NOT NULL DEFAULT '',
  interruption_reason TEXT NOT NULL DEFAULT '',
  server_tool_count INTEGER NOT NULL DEFAULT 0,
  client_tool_count INTEGER NOT NULL DEFAULT 0,
  total_cost_usd DOUBLE PRECISION NOT NULL DEFAULT 0,
  total_tokens BIGINT NOT NULL DEFAULT 0,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  completed_at TIMESTAMPTZ NULL,
  PRIMARY KEY (tenant_id, task_id),
  CONSTRAINT fk_managed_tasks_session FOREIGN KEY (tenant_id, session_id)
    REFERENCES managed_sessions (tenant_id, session_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_managed_tasks_session ON managed_tasks (tenant_id, session_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_managed_tasks_status ON managed_tasks (tenant_id, status, updated_at DESC);

CREATE TABLE IF NOT EXISTS managed_artifacts (
  tenant_id TEXT NOT NULL,
  artifact_id TEXT NOT NULL,
  task_id TEXT NOT NULL,
  session_id TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL,
  kind TEXT NOT NULL,
  uri TEXT NOT NULL DEFAULT '',
  content_type TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'ready',
  size_bytes BIGINT NOT NULL DEFAULT 0,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (tenant_id, artifact_id),
  CONSTRAINT fk_managed_artifacts_task FOREIGN KEY (tenant_id, task_id)
    REFERENCES managed_tasks (tenant_id, task_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_managed_artifacts_task ON managed_artifacts (tenant_id, task_id, created_at DESC);

CREATE TABLE IF NOT EXISTS managed_task_decisions (
  id BIGSERIAL PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  task_id TEXT NOT NULL,
  action TEXT NOT NULL,
  actor TEXT NOT NULL DEFAULT '',
  reason TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT fk_managed_decisions_task FOREIGN KEY (tenant_id, task_id)
    REFERENCES managed_tasks (tenant_id, task_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_managed_task_decisions_task ON managed_task_decisions (tenant_id, task_id, created_at DESC);
