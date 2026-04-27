-- Create unified AI dev events table supporting all tools
CREATE TABLE ai_dev_events (
    id              UUID            PRIMARY KEY DEFAULT gen_random_uuid(),
    ts              TIMESTAMPTZ     NOT NULL,
    source_vendor   VARCHAR(32)     NOT NULL, -- codex, cursor, vscode, anthropic, cowork
    source_product  VARCHAR(64),               -- codex-cli, cursor-editor, github-copilot, claude-api
    source_channel  VARCHAR(32),               -- otlp, extension, api, webhook
    user_id         TEXT,
    user_email      TEXT,
    team_id         UUID,
    repo            TEXT,
    model           TEXT,
    event_type      VARCHAR(64)     NOT NULL,
    event_category  VARCHAR(32),               -- session, model_call, tool_call, approval
    action          VARCHAR(64),
    success         BOOLEAN,
    latency_ms      BIGINT,
    input_tokens    BIGINT,
    output_tokens   BIGINT,
    cache_read_tokens BIGINT,
    cache_write_tokens BIGINT,
    estimated_cost_usd NUMERIC(12, 6),
    risk_score      INTEGER DEFAULT 0,
    risk_category   VARCHAR(32),               -- unsafe_command, secret_exposure, prod_edit
    requires_review BOOLEAN DEFAULT FALSE,
    payload         JSONB           NOT NULL,
    redacted        BOOLEAN         DEFAULT TRUE,
    created_at      TIMESTAMPTZ     DEFAULT now()
);

-- Create indexes for common query patterns
CREATE INDEX idx_ai_dev_events_ts ON ai_dev_events(ts DESC);
CREATE INDEX idx_ai_dev_events_vendor ON ai_dev_events(source_vendor, ts DESC);
CREATE INDEX idx_ai_dev_events_user ON ai_dev_events(user_email, ts DESC);
CREATE INDEX idx_ai_dev_events_risk ON ai_dev_events(risk_score DESC) WHERE risk_score > 50;
CREATE INDEX idx_ai_dev_events_repo ON ai_dev_events(repo, ts DESC) WHERE repo IS NOT NULL;
CREATE INDEX idx_ai_dev_events_requires_review ON ai_dev_events(requires_review, ts DESC) WHERE requires_review = true;
CREATE INDEX idx_ai_dev_events_event_type ON ai_dev_events(event_type, ts DESC);
