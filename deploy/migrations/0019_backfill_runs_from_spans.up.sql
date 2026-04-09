INSERT INTO runs (
    run_id,
    trace_id,
    parent_run_id,
    framework,
    agent_name,
    model,
    start_time,
    end_time,
    status,
    total_tokens,
    total_cost_usd,
    metadata,
    tenant_id
)
SELECT
    COALESCE(NULLIF(run_id, ''), trace_id) AS run_id,
    trace_id,
    NULL::TEXT AS parent_run_id,
    COALESCE(NULLIF(MAX(framework), ''), 'unknown') AS framework,
    COALESCE(
        NULLIF(
            MAX(
                COALESCE(
                    NULLIF(attributes->>'af.agent.name', ''),
                    NULLIF(attributes->>'agent.name', ''),
                    NULLIF(attributes->>'service.name', '')
                )
            ),
            ''
        ),
        'unknown'
    ) AS agent_name,
    COALESCE(
        NULLIF(
            MAX(
                COALESCE(
                    NULLIF(attributes->>'gen_ai.request.model', ''),
                    NULLIF(attributes->>'llm.model', '')
                )
            ),
            ''
        ),
        ''
    ) AS model,
    to_timestamp(MIN(start_time_ns) / 1000000000.0) AS start_time,
    to_timestamp(MAX(start_time_ns + duration_ns) / 1000000000.0) AS end_time,
    CASE
        WHEN BOOL_OR(
            status_code = 2
            OR status_code >= 500
            OR LOWER(COALESCE(attributes->>'af.outcome_status', '')) IN ('error', 'blocked', 'degraded', 'partial')
            OR LOWER(COALESCE(attributes->>'af.policy.decision', '')) IN ('deny', 'block')
            OR COALESCE(attributes->>'af.policy.blocked', 'false') = 'true'
        ) THEN 'error'
        ELSE 'success'
    END AS status,
    COALESCE(SUM(input_tokens + output_tokens + cache_read_tokens + cache_write_tokens + reasoning_tokens), 0) AS total_tokens,
    COALESCE(SUM(cost_usd), 0) AS total_cost_usd,
    '{"source":"spans_backfill"}'::jsonb AS metadata,
    tenant_id
FROM spans
WHERE COALESCE(NULLIF(trace_id, ''), '') <> ''
GROUP BY tenant_id, COALESCE(NULLIF(run_id, ''), trace_id), trace_id
ON CONFLICT (run_id, tenant_id) DO NOTHING;
