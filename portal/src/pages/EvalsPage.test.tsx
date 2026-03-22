import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import EvalsPage from './EvalsPage'

vi.mock('../hooks/auth', () => ({
  useAuth: vi.fn(),
  hasRole: vi.fn(),
}))

vi.mock('../hooks/api', () => ({
  useEvalRuns: vi.fn(),
  useScoreTraceEval: vi.fn(),
}))

import { hasRole, useAuth } from '../hooks/auth'
import { useEvalRuns, useScoreTraceEval } from '../hooks/api'

const mockUseAuth = vi.mocked(useAuth)
const mockHasRole = vi.mocked(hasRole)
const mockUseEvalRuns = vi.mocked(useEvalRuns)
const mockUseScoreTraceEval = vi.mocked(useScoreTraceEval)

describe('EvalsPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseAuth.mockReturnValue({ user: { role: 'admin' } } as any)
    mockHasRole.mockReturnValue(true)
    mockUseEvalRuns.mockReturnValue({ data: { items: [], total: 0, has_more: false }, isLoading: false } as any)
    mockUseScoreTraceEval.mockReturnValue({ mutate: vi.fn(), isPending: false } as any)
  })

  it('shows restricted message for non-admin users', () => {
    mockHasRole.mockReturnValue(false)
    render(<MemoryRouter><EvalsPage /></MemoryRouter>)
    expect(screen.getByText(/restricted to administrators/i)).toBeInTheDocument()
  })

  it('renders eval runs and score summary', () => {
    mockUseEvalRuns.mockReturnValue({
      data: {
        items: [{
          id: 1,
          trace_id: 'trace-1',
          eval_suite: 'core-release',
          release_tag: 'candidate-1',
          overall_score: 88.2,
          risk_level: 'low',
          created_at: '2026-03-22T10:00:00Z',
          policy_effectiveness: { coverage_ratio: 1, blocked_spans: 0, redacted_spans: 0 },
          scores: [{ metric: 'reliability', score: 90, summary: 'healthy' }],
        }],
        total: 1,
        has_more: false,
      },
      isLoading: false,
    } as any)

    render(<MemoryRouter><EvalsPage /></MemoryRouter>)
    expect(screen.getByText('trace-1')).toBeInTheDocument()
    expect(screen.getByText('88.2')).toBeInTheDocument()
    expect(screen.getByText(/Policy coverage: 100%/i)).toBeInTheDocument()
  })

  it('submits a score request', () => {
    const mutate = vi.fn()
    mockUseScoreTraceEval.mockReturnValue({ mutate, isPending: false } as any)

    render(<MemoryRouter><EvalsPage /></MemoryRouter>)
    fireEvent.change(screen.getByPlaceholderText('trace id'), { target: { value: 'trace-99' } })
    fireEvent.change(screen.getByPlaceholderText('candidate-2026-03-22'), { target: { value: 'candidate-2' } })
    fireEvent.click(screen.getByRole('button', { name: 'Score' }))

    expect(mutate).toHaveBeenCalledWith({
      trace_id: 'trace-99',
      release_tag: 'candidate-2',
      eval_suite: 'core-release',
    })
  })
})
