import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import TraceComparePage from './TraceComparePage'

vi.mock('../hooks/api', () => ({
  useTraceComparison: vi.fn(),
}))

import { useTraceComparison } from '../hooks/api'

const mockUseTraceComparison = vi.mocked(useTraceComparison)

describe('TraceComparePage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseTraceComparison.mockReturnValue({ data: undefined, isLoading: false } as any)
  })

  function renderPage(path = '/traces/compare?left=trace-a&right=trace-b') {
    return render(
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/traces/compare" element={<TraceComparePage />} />
        </Routes>
      </MemoryRouter>,
    )
  }

  it('asks for two traces when query params are missing', () => {
    renderPage('/traces/compare')
    expect(screen.getByText(/choose two traces/i)).toBeInTheDocument()
  })

  it('renders loading and error states', () => {
    mockUseTraceComparison.mockReturnValue({ data: undefined, isLoading: true } as any)
    renderPage()
    expect(screen.getByText(/loading comparison/i)).toBeInTheDocument()

    mockUseTraceComparison.mockReturnValue({ data: undefined, isLoading: false } as any)
    renderPage()
    expect(screen.getByText(/comparison could not be loaded/i)).toBeInTheDocument()
  })

  it('renders cards, highlights, and diffs', () => {
    mockUseTraceComparison.mockReturnValue({
      isLoading: false,
      data: {
        left: { duration_ns: 20_000_000, span_count: 4, total_tokens: 1234, total_cost_usd: 0.0123, status: 'ok', blocked_spans: 1 },
        right: { duration_ns: 2_500_000_000, span_count: 6, total_tokens: 2345, total_cost_usd: 0.0456, status: 'error', blocked_spans: 2 },
        highlights: ['cost increased', 'latency regressed'],
        diffs: [{ field: 'status', left: 'ok', right: 'error', severity: 'high' }],
      },
    } as any)

    renderPage()
    expect(screen.getByText(/Trace Comparison/)).toBeInTheDocument()
    expect(screen.getByText(/cost increased/i)).toBeInTheDocument()
    expect(screen.getByText('DURATION')).toBeInTheDocument()
    expect(screen.getByText('Left: 20.0ms')).toBeInTheDocument()
    expect(screen.getByText('Right: 2.50s')).toBeInTheDocument()
    expect(screen.getByText('status')).toBeInTheDocument()
  })
})
