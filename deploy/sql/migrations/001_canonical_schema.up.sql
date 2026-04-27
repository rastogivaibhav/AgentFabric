-- Canonical vendor-neutral event schema for ai_agent_events
CREATE TABLE ai_agent_events (
    id              UUID            PRIMARY KEY DEFAULT gen_random_uuid(),
    event_time      TIMESTAMPTZ     NOT NULL,
    source_tool     VARCHAR(32)     NOT NULL CHECK (source_tool IN ('codex', 'claude_code', 'crewai', 'langgraph', 'google_adk', 'openai_agents', 'claude_agents')),
    event_type      VARCHAR(64)     NOT NULL,
    severity        VARCHAR(16)     DEFAULT 'info',
    user_id         TEXT,
    user_email      TEXT,
    org_id          UUID,
    team_id         UUID,
    session_id      VARCHAR(64),
    trace_id        VARCHAR(32),
    span_id         VARCHAR(16),
    repo_url        TEXT,
    repo_name       TEXT,
    git_branch      TEXT,
    git_commit      VARCHAR(40),
    working_directory TEXT,
    model_name      TEXT,
    provider        VARCHAR(32),
    tool_name       VARCHAR(64),
    command         TEXT,
    command_hash    VARCHAR(64),
    file_path       TEXT,
    risk_score      INTEGER DEFAULT 0,
    requires_review BOOLEAN DEFAULT FALSE,
    prompt_redacted BOOLEAN DEFAULT TRUE,
    raw_event       JSONB NOT NULL,
    created_at      TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_ai_events_time        ON ai_agent_events(event_time DESC);
CREATE INDEX idx_ai_events_source      ON ai_agent_events(source_tool, event_time DESC);
CREATE INDEX idx_ai_events_user        ON ai_agent_events(user_email, event_time DESC);
CREATE INDEX idx_ai_events_session     ON ai_agent_events(session_id);
CREATE INDEX idx_ai_events_risk        ON ai_agent_events(risk_score DESC, event_time DESC) WHERE risk_score > 50;

-- Canonical usage table
CREATE TABLE ai_agent_usage (
    id              UUID            PRIMARY KEY DEFAULT gen_random_uuid(),
    event_time      TIMESTAMPTZ     NOT NULL,
    source_tool     VARCHAR(32)     NOT NULL,
    user_id         TEXT,
    user_email      TEXT,
    org_id          UUID,
    team_id         UUID,
    session_id      VARCHAR(64),
    model_name      TEXT,
    input_tokens    BIGINT DEFAULT 0,
    output_tokens   BIGINT DEFAULT 0,
    cache_read_tokens   BIGINT DEFAULT 0,
    cache_write_tokens  BIGINT DEFAULT 0,
    total_tokens    BIGINT GENERATED ALWAYS AS (input_tokens + output_tokens + cache_read_tokens + cache_write_tokens) STORED,
    estimated_cost_usd NUMERIC(12, 6),
    created_at      TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_usage_time    ON ai_agent_usage(event_time DESC);
CREATE INDEX idx_usage_source  ON ai_agent_usage(source_tool, event_time DESC);
CREATE INDEX idx_usage_user    ON ai_agent_usage(user_email, event_time DESC);

-- Canonical tool calls table
CREATE TABLE ai_agent_tool_calls (
    id              UUID            PRIMARY KEY DEFAULT gen_random_uuid(),
    event_time      TIMESTAMPTZ     NOT NULL,
    source_tool     VARCHAR(32)     NOT NULL,
    session_id      VARCHAR(64),
    user_email      TEXT,
    repo_name       TEXT,
    tool_name       VARCHAR(64) NOT NULL,
    action_type     VARCHAR(32),
    target_resource TEXT,
    command         TEXT,
    status          VARCHAR(16),
    duration_ms     BIGINT,
    exit_code       INTEGER,
    approval_required BOOLEAN DEFAULT FALSE,
    approved_by     TEXT,
    risk_score      INTEGER DEFAULT 0,
    raw_event       JSONB NOT NULL,
    created_at      TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_tool_calls_time     ON ai_agent_tool_calls(event_time DESC);
CREATE INDEX idx_tool_calls_tool     ON ai_agent_tool_calls(tool_name, event_time DESC);
CREATE INDEX idx_tool_calls_risk     ON ai_agent_tool_calls(risk_score DESC, event_time DESC) WHERE risk_score > 50;
CREATE INDEX idx_tool_calls_user     ON ai_agent_tool_calls(user_email, event_time DESC);
