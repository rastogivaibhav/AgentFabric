CREATE INDEX IF NOT EXISTS spans_cost_window_idx
    ON spans (tenant_id, start_time_ns DESC)
    WHERE cost_usd > 0;

CREATE INDEX IF NOT EXISTS spans_cost_prompt_release_idx
    ON spans (
        tenant_id,
        (COALESCE(NULLIF(attributes->>'af.prompt.id', ''), 'unknown')),
        (COALESCE(NULLIF(attributes->>'af.prompt.release_tag', ''), 'unreleased')),
        start_time_ns DESC
    )
    WHERE cost_usd > 0;
