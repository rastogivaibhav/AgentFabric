-- Rollback secret rotation log table

DROP VIEW IF EXISTS secret_rotation_recent;
DROP INDEX IF EXISTS idx_secret_rotation_log_key_id;
DROP INDEX IF EXISTS idx_secret_rotation_log_status;
DROP INDEX IF EXISTS idx_secret_rotation_log_rotated_at;
DROP TABLE IF EXISTS secret_rotation_log;
