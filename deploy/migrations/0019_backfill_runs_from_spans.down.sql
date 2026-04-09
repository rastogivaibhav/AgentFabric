DELETE FROM runs
WHERE COALESCE(metadata->>'source', '') = 'spans_backfill';
