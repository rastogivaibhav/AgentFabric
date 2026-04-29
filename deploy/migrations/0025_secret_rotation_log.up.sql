-- Secret rotation audit log table
-- Tracks all key rotations with before/after hashes for compliance

CREATE TABLE IF NOT EXISTS secret_rotation_log (
    id BIGSERIAL PRIMARY KEY,
    rotated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    key_id VARCHAR(64) NOT NULL,
    old_key_hash VARCHAR(64) NOT NULL,
    new_key_hash VARCHAR(64) NOT NULL,
    encrypted_by_user VARCHAR(255),
    items_rotated BIGINT NOT NULL DEFAULT 0,
    status VARCHAR(32) NOT NULL DEFAULT 'pending', -- pending, in_progress, success, partial, failed
    error_message TEXT,
    rotation_duration_seconds INT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMP
);

-- Indexes for efficient queries
CREATE INDEX idx_secret_rotation_log_rotated_at ON secret_rotation_log(rotated_at DESC);
CREATE INDEX idx_secret_rotation_log_status ON secret_rotation_log(status);
CREATE INDEX idx_secret_rotation_log_key_id ON secret_rotation_log(key_id);

-- View for monitoring recent rotations
CREATE VIEW secret_rotation_recent AS
SELECT
    id,
    rotated_at,
    key_id,
    items_rotated,
    status,
    rotation_duration_seconds,
    error_message
FROM secret_rotation_log
WHERE rotated_at > NOW() - INTERVAL '30 days'
ORDER BY rotated_at DESC;

GRANT SELECT ON secret_rotation_recent TO fabric;
