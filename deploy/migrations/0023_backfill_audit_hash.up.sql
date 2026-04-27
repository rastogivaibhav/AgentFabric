-- Backfill entry_hash for all rows with NULL or empty entry_hash
-- This ensures hash chain integrity for all historical audit entries

-- Ensure pgcrypto extension is available for sha256
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- First, update all rows with empty or NULL entry_hash by computing
-- the hash using the same deterministic algorithm as CreatePolicyAuditEntry
UPDATE policy_audit_log SET entry_hash = (
    -- Compute hash for each row using its data and previous entry's hash
    encode(
        digest(
            concat_ws(
                ':',
                decision_id,
                trace_id,
                policy_name,
                LOWER(result),
                (EXTRACT(EPOCH FROM evaluated_at)::BIGINT * 1000000000)::text,
                COALESCE((
                    SELECT entry_hash FROM policy_audit_log p2
                    WHERE p2.tenant_id = policy_audit_log.tenant_id
                    AND p2.id < policy_audit_log.id
                    ORDER BY p2.id DESC
                    LIMIT 1
                ), 'genesis')
            ),
            'sha256'
        ),
        'hex'
    )
)
WHERE entry_hash IS NULL OR entry_hash = '';

-- Add constraint to prevent future empty hashes
ALTER TABLE policy_audit_log
ADD CONSTRAINT check_entry_hash_not_empty
CHECK (entry_hash IS NOT NULL AND entry_hash != '');

-- Create index for efficient chain verification
CREATE INDEX idx_policy_audit_log_entry_hash ON policy_audit_log(tenant_id, id, entry_hash)
WHERE entry_hash IS NOT NULL;
