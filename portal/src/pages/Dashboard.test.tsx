import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'

// Mock the API hooks — we test rendering logic, not network calls
vi.mock('../hooks/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../hooks/api')>()
  return {
    ...actual,
    useOverview: vi.fn(),
    useRecommendations: vi.fn(),
    useTraces: vi.fn(),
  }
})

// Mock recharts — these cause jsdom rendering issues with SVG
vi.mock('recharts', () => ({
  AreaChart: ({ children }: { children?: React.ReactNode }) => <div data-testid="area-chart">{children}</div>,
  Area: () => null,
  BarChart: ({ children }: { children?: React.ReactNode }) => <div data-testid="bar-chart">{children}</div>,
  Bar: () => null,
  PieChart: ({ children }: { children?: React.ReactNode }) => <div data-testid="pie-chart">{children}</div>,
  Pie: () => null,
  Cell: () => null,
  XAxis: () => null,
  YAxis: () => null,
  Tooltip: () => null,
  ResponsiveContainer: ({ children }: { children?: React.ReactNode }) => <div>{children}</div>,
}))

import Dashboard from './Dashboard'
import { useOverview, useRecommendations, useTraces } from '../hooks/api'
import React from 'react'

const mockUseOverview = vi.mocked(useOverview)
const mockUseRecommendations = vi.mocked(useRecommendations)
const mockUseTraces = vi.mocked(useTraces)

describe('Dashboard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseRecommendations.mockReturnValue({ data: { items: [], total: 0, has_more: false }, isLoading: false, error: null } as any)
  })

  it('renders loading state when data is loading', () => {
    mockUseOverview.mockReturnValue({ data: undefined, isLoading: true } as any)
    mockUseTraces.mockReturnValue({ data: undefined, isLoading: true } as any)

    render(<Dashboard />)

    // Stat cards show em-dash while loading
    const dashes = screen.getAllByText('—')
    expect(dashes.length).toBeGreaterThanOrEqual(4)
  })

  it('renders dashboard heading', () => {
    mockUseOverview.mockReturnValue({ data: undefined, isLoading: false } as any)
    mockUseTraces.mockReturnValue({ data: undefined, isLoading: false } as any)

    render(<Dashboard />)

    expect(screen.getByText('Dashboard')).toBeInTheDocument()
  })

  it('renders total traces from overview data', () => {
    mockUseOverview.mockReturnValue({
      data: {
        total_traces: 1234,
        total_cost_usd: 5.6789,
        avg_latency_ms: 450,
        error_rate: 0.02,
        framework_counts: { crewai: 500, langgraph: 734 },
        active_agents: 8,
      },
      isLoading: false,
    } as any)
    mockUseTraces.mockReturnValue({ data: { items: [], total: 0 }, isLoading: false } as any)

    render(<Dashboard />)

    expect(screen.getByText('1,234')).toBeInTheDocument()
  })

  it('renders total cost formatted to 4 decimal places', () => {
    mockUseOverview.mockReturnValue({
      data: {
        total_traces: 100,
        total_cost_usd: 12.3456,
        avg_latency_ms: 300,
        error_rate: 0,
        framework_counts: {},
        active_agents: 2,
      },
      isLoading: false,
    } as any)
    mockUseTraces.mockReturnValue({ data: { items: [], total: 0 }, isLoading: false } as any)

    render(<Dashboard />)

    expect(screen.getByText('$12.3456')).toBeInTheDocument()
  })

  it('renders avg latency in ms', () => {
    mockUseOverview.mockReturnValue({
      data: {
        total_traces: 50,
        total_cost_usd: 1.0,
        avg_latency_ms: 750,
        error_rate: 0,
        framework_counts: {},
        active_agents: 1,
      },
      isLoading: false,
    } as any)
    mockUseTraces.mockReturnValue({ data: undefined, isLoading: false } as any)

    render(<Dashboard />)

    expect(screen.getByText('750ms')).toBeInTheDocument()
  })

  it('renders zero values safely when overview data has zeros', () => {
    mockUseOverview.mockReturnValue({
      data: {
        total_traces: 0,
        total_cost_usd: 0,
        avg_latency_ms: 0,
        error_rate: 0,
        framework_counts: {},
        active_agents: 0,
      },
      isLoading: false,
    } as any)
    mockUseTraces.mockReturnValue({ data: { items: [], total: 0 }, isLoading: false } as any)

    render(<Dashboard />)

    // Should not crash and should show 0 values (multiple zeros expected)
    expect(screen.getAllByText('0').length).toBeGreaterThanOrEqual(1)
  })

  it('renders stat card labels', () => {
    mockUseOverview.mockReturnValue({ data: undefined, isLoading: false } as any)
    mockUseTraces.mockReturnValue({ data: undefined, isLoading: false } as any)

    render(<Dashboard />)

    expect(screen.getByText('TOTAL TRACES')).toBeInTheDocument()
    expect(screen.getByText('TOTAL COST')).toBeInTheDocument()
    expect(screen.getByText('AVG LATENCY')).toBeInTheDocument()
  })

  it('renders partial recent trace statuses as non-green labels', () => {
    mockUseOverview.mockReturnValue({
      data: {
        total_traces: 1,
        total_cost_usd: 1.25,
        avg_latency_ms: 120,
        error_rate: 0.1,
        framework_counts: { crewai: 1 },
        active_agents: 1,
        total_tokens: 1000,
        spans_per_second: 1,
        blocked_requests: 1,
        llm_calls: 1,
        tool_calls: 0,
      },
      isLoading: false,
    } as any)
    mockUseTraces.mockReturnValue({
      data: {
        items: [{
          id: 'trace-1',
          framework: 'crewai',
          root_span_name: 'root',
          duration_ns: 1_000_000,
          span_count: 2,
          total_cost_usd: 0.2,
          status: 'partial',
        }],
        total: 1,
      },
      isLoading: false,
    } as any)

    render(<Dashboard />)

    expect(screen.getByText('partial')).toBeInTheDocument()
  })

  it('renders autonomic recommendations when present', () => {
    mockUseOverview.mockReturnValue({ data: undefined, isLoading: false } as any)
    mockUseTraces.mockReturnValue({ data: { items: [], total: 0 }, isLoading: false } as any)
    mockUseRecommendations.mockReturnValue({
      data: {
        items: [{
          id: 7,
          type: 'rollout',
          status: 'open',
          title: 'Pause rollout Prompt canary',
          summary: 'Prompt canary is above its error threshold.',
          target: 'rollout Prompt canary',
          suggested_action: 'Pause Prompt canary and inspect the canary path.',
          confidence: 0.82,
          created_at: '2026-03-25T12:00:00Z',
          updated_at: '2026-03-25T12:00:00Z',
          last_seen_at: '2026-03-25T12:00:00Z',
        }],
        total: 1,
        has_more: false,
      },
      isLoading: false,
      error: null,
    } as any)

    render(<Dashboard />)

    expect(screen.getByText('AUTONOMIC RECOMMENDATIONS')).toBeInTheDocument()
    expect(screen.getByText(/Pause rollout Prompt canary/i)).toBeInTheDocument()
  })
})
