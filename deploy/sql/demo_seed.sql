INSERT INTO pricing_rules (
    tenant_id,
    provider,
    model_pattern,
    input_per_million,
    output_per_million,
    active,
    priority,
    description
)
VALUES
    (
        '00000000-0000-0000-0000-000000000001',
        'openai',
        'gpt-4o',
        4.50,
        13.50,
        TRUE,
        250,
        'Demo tenant override for launch walkthroughs'
    ),
    (
        '00000000-0000-0000-0000-000000000001',
        'openai',
        'gpt-4o-mini',
        0.12,
        0.48,
        TRUE,
        240,
        'Lower-cost demo profile for local experiments'
    )
ON CONFLICT (provider, model_pattern) DO NOTHING;

INSERT INTO policy_rules (
    tenant_id,
    name,
    rule_type,
    enabled,
    priority,
    action,
    provider,
    model_pattern,
    environment,
    max_tokens,
    detector,
    scope,
    description
)
VALUES
    (
        '00000000-0000-0000-0000-000000000001',
        'deny-large-openai-requests',
        'traffic',
        TRUE,
        200,
        'deny',
        'openai',
        'gpt-4o',
        '',
        12000,
        '',
        'both',
        'Blocks unusually large production-like requests in the local demo tenant'
    ),
    (
        '00000000-0000-0000-0000-000000000001',
        'redact-secrets',
        'dlp',
        TRUE,
        220,
        'redact',
        '',
        '',
        '',
        0,
        'secret',
        'both',
        'Redacts obvious secrets before traffic leaves the proxy'
    ),
    (
        '00000000-0000-0000-0000-000000000001',
        'warn-on-pii-response',
        'dlp',
        TRUE,
        180,
        'warn',
        '',
        '',
        '',
        0,
        'pii',
        'response',
        'Warns on likely PII in model responses for the demo tenant'
    )
ON CONFLICT DO NOTHING;
