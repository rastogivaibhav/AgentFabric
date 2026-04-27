-- Add risk scoring columns to spans table
ALTER TABLE spans
ADD COLUMN risk_score INT DEFAULT 0 NOT NULL,
ADD COLUMN risk_category VARCHAR(64) DEFAULT 'none' NOT NULL;

-- Create index for efficient risk-based queries
CREATE INDEX idx_spans_risk_score ON spans(tenant_id, risk_score DESC, received_at DESC);
CREATE INDEX idx_spans_risk_category ON spans(tenant_id, risk_category, received_at DESC);
