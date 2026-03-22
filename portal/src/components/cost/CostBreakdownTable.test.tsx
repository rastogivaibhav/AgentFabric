import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import CostBreakdownTable from './CostBreakdownTable'

describe('CostBreakdownTable', () => {
  it('renders token totals and cost categories', () => {
    render(
      <CostBreakdownTable
        rows={[
          {
            app_name: 'ops-ui',
            environment: 'production',
            provider: 'openai',
            model: 'gpt-4o',
            input_tokens: 100,
            output_tokens: 50,
            cache_read_tokens: 10,
            cache_write_tokens: 5,
            reasoning_tokens: 2,
            input_cost_usd: 0.001,
            output_cost_usd: 0.002,
            cache_read_cost_usd: 0,
            cache_write_cost_usd: 0.0004,
            reasoning_cost_usd: 0.0008,
            total_cost_usd: 0.0042,
          },
        ] as any}
      />,
    )

    expect(screen.getByText('ops-ui')).toBeInTheDocument()
    expect(screen.getByText('167')).toBeInTheDocument()
    expect(screen.getByText('$0.001000')).toBeInTheDocument()
    expect(screen.getByText('$0.004200')).toBeInTheDocument()
  })
})
