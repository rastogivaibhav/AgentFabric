CREATE INDEX IF NOT EXISTS idx_spans_trace_start_time
    ON spans (tenant_id, trace_id, start_time_ns ASC);

CREATE INDEX IF NOT EXISTS idx_spans_parent_trace
    ON spans (trace_id, parent_span_id)
    WHERE parent_span_id IS NOT NULL AND parent_span_id <> '';

CREATE INDEX IF NOT EXISTS idx_policy_audit_trace_span
    ON policy_audit_log (tenant_id, trace_id, span_id, evaluated_at DESC);
