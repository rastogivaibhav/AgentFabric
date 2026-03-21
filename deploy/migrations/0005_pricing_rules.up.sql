CREATE TABLE IF NOT EXISTS pricing_rules (
    id                  BIGSERIAL PRIMARY KEY,
    provider            TEXT NOT NULL DEFAULT '',
    model_pattern       TEXT NOT NULL,
    input_per_million   DOUBLE PRECISION NOT NULL DEFAULT 0,
    output_per_million  DOUBLE PRECISION NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_pricing_rules_provider_model
    ON pricing_rules (provider, model_pattern);

INSERT INTO pricing_rules (provider, model_pattern, input_per_million, output_per_million)
VALUES
    ('anthropic', 'claude-3-5-sonnet', 3.0, 15.0),
    ('anthropic', 'claude-3-5-haiku', 0.80, 4.00),
    ('anthropic', 'claude-3-opus', 15.0, 75.0),
    ('anthropic', 'claude-3-haiku', 0.25, 1.25),
    ('openai', 'gpt-4o', 5.0, 15.0),
    ('openai', 'gpt-4o-mini', 0.15, 0.60),
    ('openai', 'gpt-4-turbo', 10.0, 30.0),
    ('openai', 'gpt-3.5-turbo', 0.50, 1.50),
    ('google', 'gemini-1.5-pro', 3.5, 10.5),
    ('google', 'gemini-1.5-flash', 0.35, 1.05),
    ('meta', 'llama-3.1-405b', 5.0, 15.0)
ON CONFLICT (provider, model_pattern) DO NOTHING;
