import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import React from 'react'

vi.mock('../hooks/api', () => ({
  useOverview: vi.fn(),
  useFrameworkStats: vi.fn(),
  useTraces: vi.fn(),
}))

vi.mock('recharts', () => ({
  BarChart: ({ children }: { children?: React.ReactNode }) => <div data-testid="bar-chart">{children}</div>,
  Bar: () => null,
  Cell: () => null,
  XAxis: () => null,
  YAxis: () => null,
  Tooltip: () => null,
  ResponsiveContainer: ({ children }: { children?: React.ReactNode }) => <div data-testid="responsive-container">{children}</div>,
}))

import CostPage from './CostPage'
import { useOverview, useFrameworkStats, useTraces } from '../hooks/api'

const mockUseOverview = vi.mocked(useOverview)
const mockUseFrameworkStats = vi.mocked(useFrameworkStats)
const mockUseTraces = vi.mocked(useTraces)

const MOCK_OVERVIEW = {
  total_traces: 200,
  active_agents: 5,
  total_cost_usd: 4.567890,
  total_tokens: 3_500_000,
  error_rate: 0.02,
  avg_latency_ms: 400,
  spans_per_second: 8,
  framework_counts: {
    crewai: 100,
    langgraph: 60,
    google_adk: 40,
  },
}

const MOCK_FW_STATS_WITH_COST = [
  { framework: 'crewai', trace_count: 100, total_cost_usd: 2.5, total_tokens: 2000000, input_tokens: 1200000, output_tokens: 800000 },
  { framework: 'langgraph', trace_count: 60, total_cost_usd: 1.5, total_tokens: 1000000, input_tokens: 600000, output_tokens: 400000 },
  { framework: 'google_adk', trace_count: 40, total_cost_usd: 0.567890, total_tokens: 500000, input_tokens: 300000, output_tokens: 200000 },
]

const MOCK_FW_STATS_NO_COST = [
  { framework: 'crewai', trace_count: 100 },
  { framework: 'langgraph', trace_count: 60 },
]

describe('CostPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders Total Cost stat card', () => {
    mockUseOverview.mockReturnValue({ data: MOCK_OVERVIEW, isLoading: false } as any)
    mockUseFrameworkStats.mockReturnValue({ data: MOCK_FW_STATS_WITH_COST, isLoading: false } as any)
    mockUseTraces.mockReturnValue({ data: { items: [], total: 0, has_more: false }, isLoading: false } as any)
    render(<CostPage />)
    expect(screen.getByText('TOTAL COST (24H)')).toBeInTheDocument()
    // $4.567890
    expect(screen.getByText('$4.567890')).toBeInTheDocument()
  })

  it('renders Total Tokens stat card', () => {
    mockUseOverview.mockReturnValue({ data: MOCK_OVERVIEW, isLoading: false } as any)
    mockUseFrameworkStats.mockReturnValue({ data: MOCK_FW_STATS_WITH_COST, isLoading: false } as any)
    mockUseTraces.mockReturnValue({ data: { items: [], total: 0, has_more: false }, isLoading: false } as any)
    render(<CostPage />)
    expect(screen.getByText('TOTAL TOKENS')).toBeInTheDocument()
    // 3_500_000 / 1_000_000 = 3.500M
    expect(screen.getByText('3.500M')).toBeInTheDocument()
  })

  it('renders Avg Cost / Trace stat card', () => {
    mockUseOverview.mockReturnValue({ data: MOCK_OVERVIEW, isLoading: false } as any)
    mockUseFrameworkStats.mockReturnValue({ data: MOCK_FW_STATS_WITH_COST, isLoading: false } as any)
    mockUseTraces.mockReturnValue({ data: { items: [], total: 0, has_more: false }, isLoading: false } as any)
    render(<CostPage />)
    expect(screen.getByText('AVG COST / TRACE')).toBeInTheDocument()
    // 4.567890 / 200 = 0.022839...
    expect(screen.getByText('$0.022839')).toBeInTheDocument()
  })

  it('renders Input Tokens card with value when fwStats provide input_tokens', () => {
    mockUseOverview.mockReturnValue({ data: MOCK_OVERVIEW, isLoading: false } as any)
    mockUseFrameworkStats.mockReturnValue({ data: MOCK_FW_STATS_WITH_COST, isLoading: false } as any)
    mockUseTraces.mockReturnValue({ data: { items: [], total: 0, has_more: false }, isLoading: false } as any)
    render(<CostPage />)
    expect(screen.getByText('INPUT TOKENS')).toBeInTheDocument()
    // total input = 1200000 + 600000 + 300000 = 2100000 / 1000 = 2100.0K
    expect(screen.getByText('2100.0K')).toBeInTheDocument()
  })

  it('renders Output Tokens card with value when fwStats provide output_tokens', () => {
    mockUseOverview.mockReturnValue({ data: MOCK_OVERVIEW, isLoading: false } as any)
    mockUseFrameworkStats.mockReturnValue({ data: MOCK_FW_STATS_WITH_COST, isLoading: false } as any)
    mockUseTraces.mockReturnValue({ data: { items: [], total: 0, has_more: false }, isLoading: false } as any)
    render(<CostPage />)
    expect(screen.getByText('OUTPUT TOKENS')).toBeInTheDocument()
    // total output = 800000 + 400000 + 200000 = 1400000 / 1000 = 1400.0K
    expect(screen.getByText('1400.0K')).toBeInTheDocument()
  })

  it('renders Input Tokens as N/A when fwStats have no input_tokens', () => {
    mockUseOverview.mockReturnValue({ data: MOCK_OVERVIEW, isLoading: false } as any)
    mockUseFrameworkStats.mockReturnValue({ data: MOCK_FW_STATS_NO_COST, isLoading: false } as any)
    mockUseTraces.mockReturnValue({ data: { items: [], total: 0, has_more: false }, isLoading: false } as any)
    render(<CostPage />)
    // Both Input and Output Tokens show N/A when no cost data is available
    expect(screen.getAllByText('N/A').length).toBeGreaterThanOrEqual(1)
  })

  it('renders framework cost bar chart container', () => {
    mockUseOverview.mockReturnValue({ data: MOCK_OVERVIEW, isLoading: false } as any)
    mockUseFrameworkStats.mockReturnValue({ data: MOCK_FW_STATS_WITH_COST, isLoading: false } as any)
    mockUseTraces.mockReturnValue({ data: { items: [], total: 0, has_more: false }, isLoading: false } as any)
    render(<CostPage />)
    expect(screen.getByText('COST BY FRAMEWORK')).toBeInTheDocument()
    expect(screen.getAllByTestId('bar-chart').length).toBeGreaterThanOrEqual(1)
  })

  it('renders framework share by trace count section with progress bars', () => {
    mockUseOverview.mockReturnValue({ data: MOCK_OVERVIEW, isLoading: false } as any)
    mockUseFrameworkStats.mockReturnValue({ data: MOCK_FW_STATS_WITH_COST, isLoading: false } as any)
    mockUseTraces.mockReturnValue({ data: { items: [], total: 0, has_more: false }, isLoading: false } as any)
    render(<CostPage />)
    expect(screen.getByText(/framework share.*by trace count/i)).toBeInTheDocument()
  })

  it('renders framework share by cost section with progress bars', () => {
    mockUseOverview.mockReturnValue({ data: MOCK_OVERVIEW, isLoading: false } as any)
    mockUseFrameworkStats.mockReturnValue({ data: MOCK_FW_STATS_WITH_COST, isLoading: false } as any)
    mockUseTraces.mockReturnValue({ data: { items: [], total: 0, has_more: false }, isLoading: false } as any)
    render(<CostPage />)
    expect(screen.getByText(/framework share.*by cost/i)).toBeInTheDocument()
  })

  it('loading state renders em-dash placeholders for stat values', () => {
    mockUseOverview.mockReturnValue({ data: undefined, isLoading: true } as any)
    mockUseFrameworkStats.mockReturnValue({ data: undefined, isLoading: true } as any)
    mockUseTraces.mockReturnValue({ data: undefined, isLoading: true } as any)
    render(<CostPage />)
    const dashes = screen.getAllByText('—')
    expect(dashes.length).toBeGreaterThanOrEqual(3)
  })

  it('principle 6: cost value from server is not modified (displayed as-is)', () => {
    mockUseOverview.mockReturnValue({ data: { ...MOCK_OVERVIEW, total_cost_usd: 9.123456 }, isLoading: false } as any)
    mockUseFrameworkStats.mockReturnValue({ data: MOCK_FW_STATS_WITH_COST, isLoading: false } as any)
    mockUseTraces.mockReturnValue({ data: { items: [], total: 0, has_more: false }, isLoading: false } as any)
    render(<CostPage />)
    expect(screen.getByText('$9.123456')).toBeInTheDocument()
  })

  it('zero safety: all cards render without crashing when overview stats are 0', () => {
    const zeroOverview = {
      total_traces: 0,
      active_agents: 0,
      total_cost_usd: 0,
      total_tokens: 0,
      error_rate: 0,
      avg_latency_ms: 0,
      spans_per_second: 0,
      framework_counts: {},
    }
    mockUseOverview.mockReturnValue({ data: zeroOverview, isLoading: false } as any)
    mockUseFrameworkStats.mockReturnValue({ data: [], isLoading: false } as any)
    mockUseTraces.mockReturnValue({ data: { items: [], total: 0, has_more: false }, isLoading: false } as any)
    // Should not throw
    expect(() => render(<CostPage />)).not.toThrow()
    expect(screen.getByText('$0.000000')).toBeInTheDocument()
  })
})
