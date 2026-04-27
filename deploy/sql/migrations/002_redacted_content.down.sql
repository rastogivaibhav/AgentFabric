DROP INDEX IF EXISTS idx_audit_event;
DROP INDEX IF EXISTS idx_audit_admin;
DROP TABLE IF EXISTS redacted_content_audit;

DROP TABLE IF EXISTS redacted_content;
DELETE FROM settings WHERE key = 'ai_telemetry.enable_prompt_logging';
