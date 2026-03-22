import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import React from 'react'

vi.mock('../hooks/api', () => ({
  useTraces: vi.fn(),
  useTraceSavedViews: vi.fn(),
  useUpsertTraceSavedView: vi.fn(),
  useDeleteTraceSavedView: vi.fn(),
}))

vi.mock('react-router-dom', () => ({
  useNavigate: vi.fn(),
}))

import TracesPage from './TracesPage'
import { useDeleteTraceSavedView, useTraceSavedViews, useTraces, useUpsertTraceSavedView } from '../hooks/api'
import { useNavigate } from 'react-router-dom'

const mockUseTraces = vi.mocked(useTraces)
const mockUseTraceSavedViews = vi.mocked(useTraceSavedViews)
const mockUseUpsertTraceSavedView = vi.mocked(useUpsertTraceSavedView)
const mockUseDeleteTraceSavedView = vi.mocked(useDeleteTraceSavedView)
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
    total_cost_usd: 0.0012,
    status: 'error' as const,
  },
]

describe('TracesPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseNavigate.mockReturnValue(vi.fn() as any)
    mockUseTraceSavedViews.mockReturnValue({ data: [] } as never)
    mockUseUpsertTraceSavedView.mockReturnValue({ mutateAsync: vi.fn() } as never)
    mockUseDeleteTraceSavedView.mockReturnValue({ mutate: vi.fn() } as never)
  })

  it('renders loading state when data is loading', () => {
    mockUseTraces.mockReturnValue({ data: undefined, isLoading: true, refetch: vi.fn() } as never)
    render(<TracesPage />)
    expect(screen.getByText('Loading...')).toBeInTheDocument()
  })

  it('renders table headers', () => {
    mockUseTraces.mockReturnValue({ data: { items: [], total: 0, has_more: false }, isLoading: false, refetch: vi.fn() } as never)
    render(<TracesPage />)
    expect(screen.getByText('Compare')).toBeInTheDocument()
    expect(screen.getByText('Trace ID')).toBeInTheDocument()
    expect(screen.getByText('Framework')).toBeInTheDocument()
    expect(screen.getByText('Root Span')).toBeInTheDocument()
    expect(screen.getByText('Duration')).toBeInTheDocument()
    expect(screen.getByText('Status')).toBeInTheDocument()
  })

  it('renders trace rows from mocked useTraces data', () => {
    mockUseTraces.mockReturnValue({ data: { items: MOCK_TRACES, total: 2, has_more: false }, isLoading: false, refetch: vi.fn() } as never)
    render(<TracesPage />)
    expect(screen.getByText('run_crew')).toBeInTheDocument()
    expect(screen.getByText('execute_graph')).toBeInTheDocument()
  })

  it('clicking a row navigates to /traces/:id', () => {
    const navigate = vi.fn()
    mockUseNavigate.mockReturnValue(navigate as any)
    mockUseTraces.mockReturnValue({ data: { items: MOCK_TRACES, total: 2, has_more: false }, isLoading: false, refetch: vi.fn() } as never)
    render(<TracesPage />)
    fireEvent.click(screen.getByText('run_crew').closest('tr')!)
    expect(navigate).toHaveBeenCalledWith(`/traces/${MOCK_TRACES[0].id}`)
  })

  it('framework dropdown filter calls useTraces with correct params', () => {
    mockUseTraces.mockReturnValue({ data: { items: [], total: 0, has_more: false }, isLoading: false, refetch: vi.fn() } as never)
    render(<TracesPage />)
    fireEvent.change(screen.getAllByRole('combobox')[0], { target: { value: 'crewai' } })
    expect(mockUseTraces).toHaveBeenCalledWith(expect.objectContaining({ framework: 'crewai' }))
  })

  it('status dropdown filter calls useTraces with correct params', () => {
    mockUseTraces.mockReturnValue({ data: { items: [], total: 0, has_more: false }, isLoading: false, refetch: vi.fn() } as never)
    render(<TracesPage />)
    fireEvent.change(screen.getAllByRole('combobox')[1], { target: { value: 'error' } })
    expect(mockUseTraces).toHaveBeenCalledWith(expect.objectContaining({ status: 'error' }))
  })

  it('text search updates the API query search term', () => {
    mockUseTraces.mockReturnValue({ data: { items: MOCK_TRACES, total: 2, has_more: false }, isLoading: false, refetch: vi.fn() } as never)
    render(<TracesPage />)
    fireEvent.change(screen.getByPlaceholderText(/search trace, model, provider, app, user/i), { target: { value: 'run_crew' } })
    expect(mockUseTraces).toHaveBeenCalledWith(expect.objectContaining({ search: 'run_crew' }))
  })

  it('shows empty-state message when server returns no matches', () => {
    mockUseTraces.mockReturnValue({ data: { items: [], total: 0, has_more: false }, isLoading: false, refetch: vi.fn() } as never)
    render(<TracesPage />)
    expect(screen.getByText('No traces matched the current filters.')).toBeInTheDocument()
  })

  it('pagination: Next button uses the next cursor', () => {
    mockUseTraces.mockReturnValue({ data: { items: MOCK_TRACES, total: 200, has_more: true, next_cursor: 'cursor-2' }, isLoading: false, refetch: vi.fn() } as never)
    render(<TracesPage />)
    fireEvent.click(screen.getByText(/^Next$/i))
    expect(mockUseTraces).toHaveBeenCalledWith(expect.objectContaining({ cursor: 'cursor-2' }))
  })

  it('pagination: Prev button is disabled on the first page', () => {
    mockUseTraces.mockReturnValue({ data: { items: MOCK_TRACES, total: 200, has_more: true }, isLoading: false, refetch: vi.fn() } as never)
    render(<TracesPage />)
    expect(screen.getByText(/^Prev$/i)).toBeDisabled()
  })
})
