ALTER TABLE spans
    DROP COLUMN IF EXISTS reasoning_cost_usd,
    DROP COLUMN IF EXISTS cache_write_cost_usd,
    DROP COLUMN IF EXISTS cache_read_cost_usd,
    DROP COLUMN IF EXISTS output_cost_usd,
    DROP COLUMN IF EXISTS input_cost_usd,
    DROP COLUMN IF EXISTS reasoning_tokens,
    DROP COLUMN IF EXISTS cache_write_tokens,
    DROP COLUMN IF EXISTS cache_read_tokens;

ALTER TABLE pricing_rules
    DROP COLUMN IF EXISTS reasoning_per_million,
    DROP COLUMN IF EXISTS cache_write_per_million,
    DROP COLUMN IF EXISTS cache_read_per_million;
