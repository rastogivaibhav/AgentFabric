import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import React from 'react'

vi.mock('../hooks/api', () => ({
  useTraces: vi.fn(),
}))

vi.mock('react-router-dom', () => ({
  useNavigate: vi.fn(),
}))

import TracesPage from './TracesPage'
import { useTraces } from '../hooks/api'
import { useNavigate } from 'react-router-dom'

const mockUseTraces = vi.mocked(useTraces)
const mockUseNavigate = vi.mocked(useNavigate)

const MOCK_TRACES = [
  {
    id: 'aabbccdd-1111-2222-3333-44445566aabb',
    root_span_name: 'run_crew',
    framework: 'crewai',
    start_time: '2024-03-01T10:00:00Z',
    duration_ns: 1_200_000_000,
    span_count: 5,
    total_tokens: 1500,
    total_cost_usd: 0.002341,
    status: 'ok' as const,
  },
  {
    id: 'bbccddee-2222-3333-4444-55556677bbcc',
    root_span_name: 'execute_graph',
    framework: 'langgraph',
    start_time: '2024-03-01T10:05:00Z',
    duration_ns: 800_000_000,
    span_count: 3,
    total_tokens: 900,
    total_cost_usd: 0.001200,
    status: 'error' as const,
  },
]

const makeRefetch = () => vi.fn()

describe('TracesPage', () => {
  let mockNavigate: ReturnType<typeof vi.fn>

  beforeEach(() => {
    vi.clearAllMocks()
    mockNavigate = vi.fn()
    mockUseNavigate.mockReturnValue(mockNavigate)
  })

  it('renders loading state when data is loading', () => {
    mockUseTraces.mockReturnValue({ data: undefined, isLoading: true, refetch: makeRefetch() } as any)
    render(<TracesPage />)
    expect(screen.getByText('Loading…')).toBeInTheDocument()
  })

  it('renders table headers', () => {
    mockUseTraces.mockReturnValue({ data: { items: [], total: 0, has_more: false }, isLoading: false, refetch: makeRefetch() } as any)
    render(<TracesPage />)
    expect(screen.getByText('Trace ID')).toBeInTheDocument()
    expect(screen.getByText('Framework')).toBeInTheDocument()
    expect(screen.getByText('Root Span')).toBeInTheDocument()
    expect(screen.getByText('Duration')).toBeInTheDocument()
    expect(screen.getByText('Status')).toBeInTheDocument()
  })

  it('renders trace rows from mocked useTraces data', () => {
    mockUseTraces.mockReturnValue({ data: { items: MOCK_TRACES, total: 2, has_more: false }, isLoading: false, refetch: makeRefetch() } as any)
    render(<TracesPage />)
    expect(screen.getByText('run_crew')).toBeInTheDocument()
    expect(screen.getByText('execute_graph')).toBeInTheDocument()
    expect(screen.getByText('crewai')).toBeInTheDocument()
    expect(screen.getByText('langgraph')).toBeInTheDocument()
  })

  it('clicking a row navigates to /traces/:id', () => {
    mockUseTraces.mockReturnValue({ data: { items: MOCK_TRACES, total: 2, has_more: false }, isLoading: false, refetch: makeRefetch() } as any)
    render(<TracesPage />)
    const row = screen.getByText('run_crew').closest('tr')!
    fireEvent.click(row)
    expect(mockNavigate).toHaveBeenCalledWith(`/traces/${MOCK_TRACES[0].id}`)
  })

  it('framework dropdown filter calls useTraces with correct params', () => {
    mockUseTraces.mockReturnValue({ data: { items: [], total: 0, has_more: false }, isLoading: false, refetch: makeRefetch() } as any)
    render(<TracesPage />)
    const select = screen.getAllByRole('combobox')[0]
    fireEvent.change(select, { target: { value: 'crewai' } })
    expect(mockUseTraces).toHaveBeenCalledWith(expect.objectContaining({ framework: 'crewai' }))
  })

  it('status dropdown filter calls useTraces with correct params', () => {
    mockUseTraces.mockReturnValue({ data: { items: [], total: 0, has_more: false }, isLoading: false, refetch: makeRefetch() } as any)
    render(<TracesPage />)
    const select = screen.getAllByRole('combobox')[1]
    fireEvent.change(select, { target: { value: 'error' } })
    expect(mockUseTraces).toHaveBeenCalledWith(expect.objectContaining({ status: 'error' }))
  })

  it('text search filters rows matching on root_span_name', () => {
    mockUseTraces.mockReturnValue({ data: { items: MOCK_TRACES, total: 2, has_more: false }, isLoading: false, refetch: makeRefetch() } as any)
    render(<TracesPage />)
    const input = screen.getByPlaceholderText(/search trace id or span name/i)
    fireEvent.change(input, { target: { value: 'run_crew' } })
    expect(screen.getByText('run_crew')).toBeInTheDocument()
    expect(screen.queryByText('execute_graph')).not.toBeInTheDocument()
  })

  it('text search filters rows matching on trace_id prefix', () => {
    mockUseTraces.mockReturnValue({ data: { items: MOCK_TRACES, total: 2, has_more: false }, isLoading: false, refetch: makeRefetch() } as any)
    render(<TracesPage />)
    const input = screen.getByPlaceholderText(/search trace id or span name/i)
    fireEvent.change(input, { target: { value: 'bbccddee' } })
    expect(screen.getByText('execute_graph')).toBeInTheDocument()
    expect(screen.queryByText('run_crew')).not.toBeInTheDocument()
  })

  it('text search shows no results message for unmatched query', () => {
    mockUseTraces.mockReturnValue({ data: { items: MOCK_TRACES, total: 2, has_more: false }, isLoading: false, refetch: makeRefetch() } as any)
    render(<TracesPage />)
    const input = screen.getByPlaceholderText(/search trace id or span name/i)
    fireEvent.change(input, { target: { value: 'zzz_no_match_zzz' } })
    expect(screen.getByText('No traces match your search.')).toBeInTheDocument()
  })

  it('pagination: Next button calls setPageIndex +1', () => {
    mockUseTraces.mockReturnValue({ data: { items: MOCK_TRACES, total: 200, has_more: true }, isLoading: false, refetch: makeRefetch() } as any)
    render(<TracesPage />)
    const nextBtn = screen.getByText(/next/i)
    expect(nextBtn).not.toBeDisabled()
    fireEvent.click(nextBtn)
    // After clicking next, useTraces should be called with offset 100
    expect(mockUseTraces).toHaveBeenCalledWith(expect.objectContaining({ offset: '100' }))
  })

  it('pagination: Prev button is disabled on page 0', () => {
    mockUseTraces.mockReturnValue({ data: { items: MOCK_TRACES, total: 200, has_more: true }, isLoading: false, refetch: makeRefetch() } as any)
    render(<TracesPage />)
    const prevBtn = screen.getByText(/← prev/i)
    expect(prevBtn).toBeDisabled()
  })

  it('pagination: changing framework filter resets page to 0', () => {
    mockUseTraces.mockReturnValue({ data: { items: MOCK_TRACES, total: 200, has_more: true }, isLoading: false, refetch: makeRefetch() } as any)
    render(<TracesPage />)

    // Advance to page 2
    const nextBtn = screen.getByText(/next/i)
    fireEvent.click(nextBtn)

    // Now change the framework filter
    const select = screen.getAllByRole('combobox')[0]
    fireEvent.change(select, { target: { value: 'crewai' } })

    // offset should be reset to 0
    expect(mockUseTraces).toHaveBeenCalledWith(expect.objectContaining({ offset: '0' }))
  })

  it('multi-tenancy: renders empty state when server returns no items', () => {
    mockUseTraces.mockReturnValue({ data: { items: [], total: 0, has_more: false }, isLoading: false, refetch: makeRefetch() } as any)
    render(<TracesPage />)
    expect(screen.getByText('No traces found.')).toBeInTheDocument()
    expect(screen.queryByText('run_crew')).not.toBeInTheDocument()
  })
})
