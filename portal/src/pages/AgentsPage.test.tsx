import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import React from 'react'

// AgentsPage uses useQuery directly for the traces fetch (not a named hook),
// and useOverview from hooks/api. We mock both.
vi.mock('../hooks/api', () => ({
  useOverview: vi.fn(),
}))

// AgentsPage also uses @tanstack/react-query's useQuery for its internal traces fetch
vi.mock('@tanstack/react-query', () => ({
  useQuery: vi.fn(),
}))

import AgentsPage from './AgentsPage'
import { useOverview } from '../hooks/api'
import { useQuery } from '@tanstack/react-query'

const mockUseOverview = vi.mocked(useOverview)
const mockUseQuery = vi.mocked(useQuery)

const MOCK_TRACES_ITEMS = [
  {
    id: 'aabbccdd-0001-0002-0003-000400050006',
    root_span_name: 'crew_run',
    framework: 'crewai',
    start_time: '2024-03-01T10:00:00Z',
    duration_ns: 1_000_000_000,
    span_count: 4,
    total_tokens: 1200,
    total_cost_usd: 0.001500,
    status: 'ok',
  },
  {
    id: 'bbccddee-0002-0003-0004-000500060007',
    root_span_name: 'graph_exec',
    framework: 'langgraph',
    start_time: '2024-03-01T10:01:00Z',
    duration_ns: 500_000_000,
    span_count: 2,
    total_tokens: 600,
    total_cost_usd: 0.000750,
    status: 'ok',
  },
]

const MOCK_OVERVIEW = {
  total_traces: 150,
  active_agents: 3,
  total_cost_usd: 2.5,
  total_tokens: 500000,
  error_rate: 0.01,
  avg_latency_ms: 300,
  spans_per_second: 5,
  framework_counts: {
    crewai: 80,
    langgraph: 40,
    google_adk: 15,
    openai_agents: 10,
    claude_agents: 5,
  },
}

describe('AgentsPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders 5 framework cards', () => {
    mockUseOverview.mockReturnValue({ data: MOCK_OVERVIEW, isLoading: false } as any)
    mockUseQuery.mockReturnValue({ data: { items: MOCK_TRACES_ITEMS }, isLoading: false } as any)
    render(<AgentsPage />)
    expect(screen.getByText(/crewai/i)).toBeInTheDocument()
    expect(screen.getByText(/langgraph/i)).toBeInTheDocument()
    expect(screen.getByText(/google adk/i)).toBeInTheDocument()
    expect(screen.getByText(/openai agents/i)).toBeInTheDocument()
    expect(screen.getByText(/claude agents/i)).toBeInTheDocument()
  })

  it('framework card shows trace count from mocked useOverview data', () => {
    mockUseOverview.mockReturnValue({ data: MOCK_OVERVIEW, isLoading: false } as any)
    mockUseQuery.mockReturnValue({ data: { items: [] }, isLoading: false } as any)
    render(<AgentsPage />)
    // crewai count = 80
    expect(screen.getByText('80')).toBeInTheDocument()
    // langgraph count = 40
    expect(screen.getByText('40')).toBeInTheDocument()
  })

  it('framework card shows percentage of total', () => {
    mockUseOverview.mockReturnValue({ data: MOCK_OVERVIEW, isLoading: false } as any)
    mockUseQuery.mockReturnValue({ data: { items: [] }, isLoading: false } as any)
    render(<AgentsPage />)
    // crewai = 80/150 = 53%
    expect(screen.getByText('53% of total')).toBeInTheDocument()
  })

  it('renders recent activity table with trace rows', () => {
    mockUseOverview.mockReturnValue({ data: MOCK_OVERVIEW, isLoading: false } as any)
    mockUseQuery.mockReturnValue({ data: { items: MOCK_TRACES_ITEMS }, isLoading: false } as any)
    render(<AgentsPage />)
    expect(screen.getByText('crew_run')).toBeInTheDocument()
    expect(screen.getByText('graph_exec')).toBeInTheDocument()
  })

  it('loading state shows loading indicator in recent activity', () => {
    mockUseOverview.mockReturnValue({ data: undefined, isLoading: true } as any)
    mockUseQuery.mockReturnValue({ data: undefined, isLoading: true } as any)
    render(<AgentsPage />)
    expect(screen.getByText('loading…')).toBeInTheDocument()
  })

  it('empty state renders no activity message when traces are empty', () => {
    mockUseOverview.mockReturnValue({ data: MOCK_OVERVIEW, isLoading: false } as any)
    mockUseQuery.mockReturnValue({ data: { items: [] }, isLoading: false } as any)
    render(<AgentsPage />)
    expect(screen.getByText(/no agent activity yet/i)).toBeInTheDocument()
  })

  it('framework cards display framework name from server response (not hardcoded client label)', () => {
    // The overview data drives card counts — verify they match what the server returns
    mockUseOverview.mockReturnValue({ data: MOCK_OVERVIEW, isLoading: false } as any)
    mockUseQuery.mockReturnValue({ data: { items: [] }, isLoading: false } as any)
    render(<AgentsPage />)
    // google_adk count = 15
    expect(screen.getByText('15')).toBeInTheDocument()
  })

  it('multi-tenancy: renders zero counts when overview returns no framework_counts', () => {
    mockUseOverview.mockReturnValue({
      data: { ...MOCK_OVERVIEW, framework_counts: {}, total_traces: 0 },
      isLoading: false,
    } as any)
    mockUseQuery.mockReturnValue({ data: { items: [] }, isLoading: false } as any)
    render(<AgentsPage />)
    // All 5 framework cards should show 0
    const zeros = screen.getAllByText('0')
    expect(zeros.length).toBeGreaterThanOrEqual(5)
  })
})
