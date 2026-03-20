-- Layer 2: Virtual key vault
-- Real LLM API keys are encrypted with AES-256-GCM and never returned after registration.
-- Developers use af-vk-* virtual keys; the proxy resolves them to real keys at request time.

CREATE TABLE virtual_keys (
    id           BIGSERIAL    PRIMARY KEY,
    tenant_id    TEXT         NOT NULL,
    virtual_key  TEXT         NOT NULL UNIQUE,  -- af-vk-<32 hex chars>
    display_name TEXT         NOT NULL,
    provider     TEXT         NOT NULL,          -- openai | anthropic | google
    real_key_enc BYTEA        NOT NULL,          -- AES-256-GCM: nonce || ciphertext
    key_id       TEXT         NOT NULL,          -- first 8 chars of real key (display only)
    team_id      TEXT,
    created_by   TEXT,
    expires_at   TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    revoked      BOOLEAN      NOT NULL DEFAULT false,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX ON virtual_keys (virtual_key) WHERE NOT revoked;
CREATE INDEX ON virtual_keys (tenant_id);
