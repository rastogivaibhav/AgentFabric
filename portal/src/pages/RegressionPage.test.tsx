import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import RegressionPage from './RegressionPage'

vi.mock('../hooks/api', () => ({
  useCompareEvalRegressions: vi.fn(),
}))

import { useCompareEvalRegressions } from '../hooks/api'

const mockUseCompareEvalRegressions = vi.mocked(useCompareEvalRegressions)

describe('RegressionPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseCompareEvalRegressions.mockReturnValue({ mutate: vi.fn(), isPending: false } as any)
  })

  it('submits baseline and candidate tags', () => {
    const mutate = vi.fn()
    mockUseCompareEvalRegressions.mockReturnValue({ mutate, isPending: false } as any)

    render(<RegressionPage />)
    fireEvent.change(screen.getByPlaceholderText(/baseline tag/i), { target: { value: 'baseline-1' } })
    fireEvent.change(screen.getByPlaceholderText(/candidate tag/i), { target: { value: 'candidate-1' } })
    fireEvent.click(screen.getByRole('button', { name: /compare/i }))

    expect(mutate).toHaveBeenCalledWith({
      baseline_tag: 'baseline-1',
      candidate_tag: 'candidate-1',
      eval_suite: 'core-release',
    })
  })

  it('renders regression results and highlights', () => {
    mockUseCompareEvalRegressions.mockReturnValue({
      mutate: vi.fn(),
      isPending: false,
      data: {
        baseline_tag: 'baseline-1',
        candidate_tag: 'candidate-1',
        compared_runs: 4,
        eval_suite: 'core-release',
        overall_delta: -6.5,
        risk_level: 'medium',
        highlights: ['latency regressed materially'],
        metrics: [
          { metric: 'latency', baseline_score: 80, candidate_score: 70, delta: -10 },
        ],
      },
    } as any)

    render(<RegressionPage />)
    expect(screen.getByText('baseline-1')).toBeInTheDocument()
    expect(screen.getByText('candidate-1')).toBeInTheDocument()
    expect(screen.getByText('latency regressed materially')).toBeInTheDocument()
    expect(screen.getByText('-10.00')).toBeInTheDocument()
  })
})
