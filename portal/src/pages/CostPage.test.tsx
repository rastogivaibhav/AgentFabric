import React from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'

vi.mock('../hooks/api', () => ({
  useOverview: vi.fn(),
  useFrameworkStats: vi.fn(),
  useCostReport: vi.fn(),
  useCostSpikes: vi.fn(),
  usePreviewPricingRule: vi.fn(),
  useTraces: vi.fn(),
  useBudget: vi.fn(),
  useBudgetUsage: vi.fn(),
  useUpsertBudget: vi.fn(),
  useDeleteBudget: vi.fn(),
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
import {
  useOverview,
  useFrameworkStats,
  useCostReport,
  useCostSpikes,
  usePreviewPricingRule,
  useTraces,
  useBudget,
  useBudgetUsage,
  useUpsertBudget,
  useDeleteBudget,
} from '../hooks/api'

const mockUseOverview = vi.mocked(useOverview)
const mockUseFrameworkStats = vi.mocked(useFrameworkStats)
const mockUseCostReport = vi.mocked(useCostReport)
const mockUseCostSpikes = vi.mocked(useCostSpikes)
const mockUsePreviewPricingRule = vi.mocked(usePreviewPricingRule)
const mockUseTraces = vi.mocked(useTraces)
const mockUseBudget = vi.mocked(useBudget)
const mockUseBudgetUsage = vi.mocked(useBudgetUsage)
const mockUseUpsertBudget = vi.mocked(useUpsertBudget)
const mockUseDeleteBudget = vi.mocked(useDeleteBudget)

const MOCK_OVERVIEW = {
  total_traces: 200,
  active_agents: 5,
  total_cost_usd: 4.567890,
  total_tokens: 3_500_000,
  error_rate: 0.02,
  avg_latency_ms: 400,
  spans_per_second: 8,
  blocked_requests: 3,
  llm_calls: 12,
  tool_calls: 4,
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

const MOCK_SPIKE_REPORT = {
  current_window_start: '2026-03-24T00:00:00Z',
  current_window_end: '2026-03-25T00:00:00Z',
  previous_window_start: '2026-03-23T00:00:00Z',
  previous_window_end: '2026-03-24T00:00:00Z',
  filters_applied: { since: '24h' },
  spikes: [
    {
      app_name: 'support-app',
      environment: 'staging',
      provider: 'openai',
      model: 'gpt-4o',
      prompt_id: 'support.system',
      release_tag: 'candidate-7',
      current_cost_usd: 2.7,
      previous_cost_usd: 0.7,
      delta_cost_usd: 2.0,
      delta_pct: 285.7,
      current_trace_count: 28,
      previous_trace_count: 8,
      current_total_tokens: 150000,
      previous_total_tokens: 44000,
      explanation: 'Spend increased because the staging candidate release moved more traffic onto gpt-4o.',
    },
  ],
  contributor_groups: [
    {
      dimension: 'release_tag',
      items: [
        {
          key: 'candidate-7',
          current_cost_usd: 2.7,
          previous_cost_usd: 0.7,
          delta_cost_usd: 2.0,
          delta_pct: 285.7,
          share_of_delta: 0.75,
        },
      ],
    },
  ],
}

describe('CostPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseBudget.mockReturnValue({ data: undefined, isLoading: false } as any)
    mockUseBudgetUsage.mockReturnValue({ data: undefined, isLoading: false } as any)
    mockUseUpsertBudget.mockReturnValue({ mutate: vi.fn(), isPending: false, isError: false } as any)
    mockUseDeleteBudget.mockReturnValue({ mutate: vi.fn() } as any)
    mockUseOverview.mockReturnValue({ data: MOCK_OVERVIEW, isLoading: false } as any)
    mockUseFrameworkStats.mockReturnValue({ data: MOCK_FW_STATS_WITH_COST, isLoading: false } as any)
    mockUseTraces.mockReturnValue({ data: { items: [], total: 0, has_more: false }, isLoading: false } as any)
    mockUseCostReport.mockReturnValue({
      data: [{
        app_name: 'support-app',
        environment: 'staging',
        provider: 'openai',
        model: 'gpt-4o',
        prompt_id: 'support.system',
        release_tag: 'candidate-7',
        total_tokens: 2400,
        total_cost_usd: 1.2,
        input_tokens: 1000,
        output_tokens: 1200,
        cache_read_tokens: 100,
        cache_write_tokens: 50,
        reasoning_tokens: 50,
        input_cost_usd: 0.4,
        output_cost_usd: 0.6,
        cache_read_cost_usd: 0.05,
        cache_write_cost_usd: 0.05,
        reasoning_cost_usd: 0.1,
        trace_count: 12,
        blocked_count: 1,
      }],
      isLoading: false,
    } as any)
    mockUseCostSpikes.mockReturnValue({ data: MOCK_SPIKE_REPORT, isLoading: false, isError: false } as any)
    mockUsePreviewPricingRule.mockReturnValue({ mutate: vi.fn(), isPending: false, data: undefined } as any)
  })

  it('renders total cost stat card', () => {
    render(<CostPage />)
    expect(screen.getByText('TOTAL COST (24H)')).toBeInTheDocument()
    expect(screen.getByText('$4.567890')).toBeInTheDocument()
  })

  it('renders total tokens stat card', () => {
    render(<CostPage />)
    expect(screen.getByText('TOTAL TOKENS')).toBeInTheDocument()
    expect(screen.getByText('3.500M')).toBeInTheDocument()
  })

  it('renders average cost per trace stat card', () => {
    render(<CostPage />)
    expect(screen.getByText('AVG COST / TRACE')).toBeInTheDocument()
    expect(screen.getByText('$0.022839')).toBeInTheDocument()
  })

  it('renders framework charts and share sections', () => {
    render(<CostPage />)
    expect(screen.getByText('COST BY FRAMEWORK')).toBeInTheDocument()
    expect(screen.getByText(/FRAMEWORK SHARE - BY TRACE COUNT/i)).toBeInTheDocument()
    expect(screen.getByText(/FRAMEWORK SHARE - BY COST/i)).toBeInTheDocument()
    expect(screen.getAllByTestId('bar-chart').length).toBeGreaterThanOrEqual(1)
  })

  it('renders cost spike diagnosis cards and contributor table', () => {
    render(<CostPage />)
    expect(screen.getByText('COST SPIKE DIAGNOSIS')).toBeInTheDocument()
    expect(screen.getByText('Spend increased because the staging candidate release moved more traffic onto gpt-4o.')).toBeInTheDocument()
    expect(screen.getByText(/TOP CONTRIBUTORS BY RELEASE/i)).toBeInTheDocument()
    expect(screen.getAllByText('candidate-7').length).toBeGreaterThan(0)
  })

  it('passes updated filters to cost hooks', () => {
    render(<CostPage />)
    fireEvent.change(screen.getByLabelText('Release'), { target: { value: 'candidate-8' } })
    const lastReportCall = mockUseCostReport.mock.calls.at(-1)?.[0]
    const lastSpikeCall = mockUseCostSpikes.mock.calls.at(-1)?.[0]
    expect(lastReportCall).toMatchObject({ release_tag: 'candidate-8' })
    expect(lastSpikeCall).toMatchObject({ release_tag: 'candidate-8' })
  })

  it('renders spike error state', () => {
    mockUseCostSpikes.mockReturnValue({ data: undefined, isLoading: false, isError: true } as any)
    render(<CostPage />)
    expect(screen.getByText(/Spike analysis failed/i)).toBeInTheDocument()
  })

  it('renders prompt and release columns in governed cost breakdown', () => {
    render(<CostPage />)
    expect(screen.getByText('GOVERNED COST BREAKDOWN')).toBeInTheDocument()
    expect(screen.getByText('support.system')).toBeInTheDocument()
    expect(screen.getAllByText('candidate-7').length).toBeGreaterThan(0)
  })

  it('renders loading placeholders when overview is loading', () => {
    mockUseOverview.mockReturnValue({ data: undefined, isLoading: true } as any)
    mockUseFrameworkStats.mockReturnValue({ data: undefined, isLoading: true } as any)
    render(<CostPage />)
    const placeholders = screen.getAllByText('-')
    expect(placeholders.length).toBeGreaterThanOrEqual(3)
  })

  it('renders no spike state when no spikes are present', () => {
    mockUseCostSpikes.mockReturnValue({
      data: { ...MOCK_SPIKE_REPORT, spikes: [], contributor_groups: [] },
      isLoading: false,
      isError: false,
    } as any)
    render(<CostPage />)
    expect(screen.getByText(/No cost spike detected/i)).toBeInTheDocument()
  })
})
