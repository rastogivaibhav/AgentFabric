DROP INDEX IF EXISTS eval_runs_prompt_release_idx;

ALTER TABLE IF EXISTS eval_runs
    DROP COLUMN IF EXISTS prompt_environment,
    DROP COLUMN IF EXISTS prompt_version,
    DROP COLUMN IF EXISTS prompt_id;

DROP INDEX IF EXISTS prompt_releases_prompt_env_tag_idx;
DROP INDEX IF EXISTS prompt_releases_active_release_idx;

ALTER TABLE IF EXISTS prompt_releases
    DROP COLUMN IF EXISTS promotion_reason,
    DROP COLUMN IF EXISTS status;
