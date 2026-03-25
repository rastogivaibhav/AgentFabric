ALTER TABLE IF EXISTS prompt_releases
    ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active',
    ADD COLUMN IF NOT EXISTS promotion_reason TEXT NOT NULL DEFAULT '';

DO $$
DECLARE
    existing_constraint TEXT;
BEGIN
    SELECT con.conname
    INTO existing_constraint
    FROM pg_constraint con
    JOIN pg_class rel ON rel.oid = con.conrelid
    JOIN pg_namespace nsp ON nsp.oid = rel.relnamespace
    WHERE nsp.nspname = current_schema()
      AND rel.relname = 'prompt_releases'
      AND con.contype = 'u'
      AND pg_get_constraintdef(con.oid) LIKE '%tenant_id, prompt_id, environment%';

    IF existing_constraint IS NOT NULL THEN
        EXECUTE format('ALTER TABLE prompt_releases DROP CONSTRAINT %I', existing_constraint);
    END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS prompt_releases_active_release_idx
    ON prompt_releases (tenant_id, prompt_id, environment)
    WHERE status = 'active';

CREATE INDEX IF NOT EXISTS prompt_releases_prompt_env_tag_idx
    ON prompt_releases (tenant_id, prompt_id, environment, release_tag, created_at DESC);

ALTER TABLE IF EXISTS eval_runs
    ADD COLUMN IF NOT EXISTS prompt_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS prompt_version INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS prompt_environment TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS eval_runs_prompt_release_idx
    ON eval_runs (tenant_id, prompt_id, prompt_environment, release_tag, created_at DESC);
