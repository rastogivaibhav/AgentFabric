-- Rollback: Drop constraint and index, reset entry_hash for recent entries
-- This should only be used if the backfill process needs to be reversed

-- Drop the constraint
ALTER TABLE policy_audit_log
DROP CONSTRAINT IF EXISTS check_entry_hash_not_empty;

-- Drop the index
DROP INDEX IF EXISTS idx_policy_audit_log_entry_hash;

-- Optionally reset recent hashes (entries from last 7 days)
-- Comment this out if you want to keep backfilled hashes
-- UPDATE policy_audit_log
-- SET entry_hash = NULL
-- WHERE created_at > NOW() - INTERVAL '7 days';
