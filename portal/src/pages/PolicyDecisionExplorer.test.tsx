import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import PolicyDecisionExplorer from './PolicyDecisionExplorer'

describe('PolicyDecisionExplorer', () => {
  it('renders matched decision metadata and traces', () => {
    render(
      <PolicyDecisionExplorer
        label="Traffic"
        decision={{
          matched: true,
          action: 'deny',
          policy_name: 'block-prod',
          reason: 'model denied',
          explain: 'Matched provider and environment',
          engine: 'policy-v2',
          decision_mode: 'rego',
          version: 3,
          rollout_percent: 75,
          final: true,
          evaluation_path: ['fast', 'rego'],
          matched_fields: ['provider', 'environment'],
          matched_names: ['secret'],
          guardrail_matches: ['prompt_injection'],
          rule_conditions: { app: 'ops-ui' },
          rego_query: 'deny if input.environment == "production"',
          redacted_preview: '***',
          condition_trace: [
            { field: 'provider', operator: 'eq', expected: 'openai', actual: 'openai', matched: true, source: 'request' },
            { field: 'environment', operator: 'eq', expected: 'production', actual: 'development', matched: false },
          ],
        }}
      />,
    )

    expect(screen.getByText(/deny via block-prod/i)).toBeInTheDocument()
    expect(screen.getByText(/Matched provider and environment/)).toBeInTheDocument()
    expect(screen.getByText(/engine policy-v2/i)).toBeInTheDocument()
    expect(screen.getByText(/path: fast -> rego/i)).toBeInTheDocument()
    expect(screen.getByText(/matched fields: provider, environment/i)).toBeInTheDocument()
    expect(screen.getByText(/guardrails: prompt_injection/i)).toBeInTheDocument()
    expect(screen.getByText(/rego-style query/i)).toBeInTheDocument()
    expect(screen.getByText(/redacted preview/i)).toBeInTheDocument()
    expect(screen.getByText('MATCH')).toBeInTheDocument()
    expect(screen.getByText('MISS')).toBeInTheDocument()
  })

  it('renders no-match state', () => {
    render(<PolicyDecisionExplorer label="Request DLP" decision={{ matched: false }} />)
    expect(screen.getByText(/no matching rule/i)).toBeInTheDocument()
  })
})
