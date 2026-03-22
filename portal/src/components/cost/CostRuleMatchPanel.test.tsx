import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import CostRuleMatchPanel from './CostRuleMatchPanel'

describe('CostRuleMatchPanel', () => {
  it('renders empty state when no preview exists', () => {
    render(<CostRuleMatchPanel preview={null} />)
    expect(screen.getByText(/run a pricing preview/i)).toBeInTheDocument()
  })

  it('renders rule summary, category rows, and explanations', () => {
    render(
      <CostRuleMatchPanel
        preview={{
          rule_id: 12,
          model_pattern: 'gpt-4o',
          pricing_scope: 'tenant',
          total_cost_usd: 0.0085,
          input_tokens: 1000,
          input_per_million: 5,
          input_cost_usd: 0.005,
          output_tokens: 500,
          output_per_million: 7,
          output_cost_usd: 0.0035,
          cache_read_tokens: 100,
          cache_read_per_million: 0.5,
          cache_read_cost_usd: 0.00005,
          cache_write_tokens: 0,
          cache_write_per_million: 0,
          cache_write_cost_usd: 0,
          reasoning_tokens: 25,
          reasoning_per_million: 2,
          reasoning_cost_usd: 0.00005,
          explain: ['tenant override matched', 'provider/model matched'],
        } as any}
      />,
    )

    expect(screen.getByText('12')).toBeInTheDocument()
    expect(screen.getByText('tenant')).toBeInTheDocument()
    expect(screen.getByText('$0.008500')).toBeInTheDocument()
    expect(screen.getByText(/1,000 tokens/i)).toBeInTheDocument()
    expect(screen.getByText(/tenant override matched/i)).toBeInTheDocument()
    expect(screen.getByText(/provider\/model matched/i)).toBeInTheDocument()
  })
})
