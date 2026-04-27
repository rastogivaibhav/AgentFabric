-- Encrypted storage for redacted prompts/arguments (admin-only access)
-- Only stored if admin enables prompt logging
CREATE TABLE redacted_content (
    id              UUID            PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id        UUID            NOT NULL REFERENCES ai_agent_events(id) ON DELETE CASCADE,
    content_type    VARCHAR(32)     NOT NULL CHECK (content_type IN ('prompt', 'response', 'tool_arguments', 'file_content')),
    encrypted_content BYTEA         NOT NULL, -- encrypted with admin master key
    encryption_key_version INT      NOT NULL DEFAULT 1,
    revealed_by     UUID,                       -- user who revealed it
    revealed_at     TIMESTAMPTZ,
    reason          TEXT,                       -- audit: why was this revealed?
    created_at      TIMESTAMPTZ     DEFAULT now()
);

CREATE INDEX idx_redacted_event ON redacted_content(event_id);
CREATE INDEX idx_redacted_revealed ON redacted_content(revealed_at) WHERE revealed_at IS NOT NULL;

-- Settings table for admin control
CREATE TABLE IF NOT EXISTS settings (
    key             VARCHAR(128)    PRIMARY KEY,
    value           TEXT            NOT NULL,
    description     TEXT,
    updated_at      TIMESTAMPTZ     DEFAULT now()
);

-- Insert default setting: prompt logging disabled
INSERT INTO settings (key, value, description) VALUES
    ('ai_telemetry.enable_prompt_logging', 'false', 'Store user prompts (requires admin enablement)')
    ON CONFLICT (key) DO NOTHING;

-- Audit log for redacted content access
CREATE TABLE redacted_content_audit (
    id              UUID            PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id        UUID            NOT NULL,
    admin_user_id   UUID            NOT NULL,
    admin_email     TEXT            NOT NULL,
    action          VARCHAR(32)     NOT NULL CHECK (action IN ('viewed', 'exported', 'shared')),
    ip_address      INET,
    user_agent      TEXT,
    reason          TEXT,
    accessed_at     TIMESTAMPTZ     DEFAULT now()
);

CREATE INDEX idx_audit_admin ON redacted_content_audit(admin_email, accessed_at DESC);
CREATE INDEX idx_audit_event ON redacted_content_audit(event_id);
