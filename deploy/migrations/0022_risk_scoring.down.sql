-- Remove risk scoring columns and indexes
DROP INDEX IF EXISTS idx_spans_risk_category;
DROP INDEX IF EXISTS idx_spans_risk_score;

ALTER TABLE spans
DROP COLUMN IF EXISTS risk_category,
DROP COLUMN IF EXISTS risk_score;
