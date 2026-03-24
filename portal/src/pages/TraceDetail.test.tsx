import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import React from 'react'

vi.mock('../hooks/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../hooks/api')>()
  return {
    ...actual,
    useTrace: vi.fn(),
  }
})

vi.mock('react-router-dom', () => ({
  useParams: vi.fn(),
  Link: ({ children }: { children: React.ReactNode }) => <a>{children}</a>,
}))

vi.mock('../components/TopologyGraph', () => ({
  default: ({ spans }: { spans: unknown[] }) => (
    <div data-testid="topology-graph" data-span-count={spans.length}>Topology Graph</div>
  ),
}))

import TraceDetail from './TraceDetail'
import { useTrace } from '../hooks/api'
import { useParams } from 'react-router-dom'

const mockUseTrace = vi.mocked(useTrace)
const mockUseParams = vi.mocked(useParams)

const TRACE_ID = 'aabbccdd-1111-2222-3333-44445566aabb'

const MOCK_SPANS = [
  {
    id: 'span-0001-aaaa-bbbb-cccc-ddddeeee0001',
    trace_id: TRACE_ID,
    parent_id: undefined,
    run_id: 'run-0001',
    name: 'root_operation',
    framework: 'crewai',
    start_time_ns: 1_700_000_000_000_000_000,
    duration_ns: 1_200_000_000,
    status_code: 0,
    attributes: { 'agent.name': 'ResearchAgent' },
    received_at: '2024-03-01T10:00:01Z',
  },
  {
    id: 'span-0002-aaaa-bbbb-cccc-ddddeeee0002',
    trace_id: TRACE_ID,
    parent_id: 'span-0001-aaaa-bbbb-cccc-ddddeeee0001',
    run_id: 'run-0001',
    name: 'child_llm_call',
    framework: 'crewai',
    start_time_ns: 1_700_000_000_100_000_000,
    duration_ns: 800_000_000,
    status_code: 0,
    attributes: {},
    cost_usd: 0.000123,
    input_tokens: 200,
    output_tokens: 150,
    received_at: '2024-03-01T10:00:02Z',
  },
]

const MOCK_TRACE = {
  id: TRACE_ID,
  root_span_name: 'root_operation',
  framework: 'crewai',
  start_time: '2024-03-01T10:00:00Z',
  duration_ns: 1_200_000_000,
  span_count: 2,
  error_count: 0,
  total_cost_usd: 0.000123,
  total_tokens: 350,
  status: 'ok' as const,
  spans: MOCK_SPANS,
  policy_events: [
    {
      decision_id: 'decision-1',
      trace_id: TRACE_ID,
      span_id: 'span-0002-aaaa-bbbb-cccc-ddddeeee0002',
      policy_name: 'deny-gpt4o',
      result: 'deny',
      reason: 'provider/model policy matched',
      tenant_id: 'tenant-1',
      provider: 'openai',
      model: 'gpt-4o',
      scope: 'request',
    },
  ],
}

describe('TraceDetail', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseParams.mockReturnValue({ traceId: TRACE_ID })
  })

  it('renders loading state while trace is loading', () => {
    mockUseTrace.mockReturnValue({ data: undefined, isLoading: true } as never)
    render(<TraceDetail />)
    expect(screen.getByText('Loading trace...')).toBeInTheDocument()
  })

  it('renders not found state when trace is missing', () => {
    mockUseTrace.mockReturnValue({ data: null, isLoading: false } as never)
    render(<TraceDetail />)
    expect(screen.getByText('Trace not found')).toBeInTheDocument()
  })

  it('renders trace header with trace ID and stats', () => {
    mockUseTrace.mockReturnValue({ data: MOCK_TRACE, isLoading: false } as never)
    render(<TraceDetail />)
    expect(screen.getByText(TRACE_ID)).toBeInTheDocument()
    expect(screen.getAllByText('crewai').length).toBeGreaterThanOrEqual(1)
    expect(screen.getByText('Policy Events')).toBeInTheDocument()
  })

  it('renders waterfall rows by default', () => {
    mockUseTrace.mockReturnValue({ data: MOCK_TRACE, isLoading: false } as never)
    render(<TraceDetail />)
    expect(screen.getByText('root_operation')).toBeInTheDocument()
    expect(screen.getByText('child_llm_call')).toBeInTheDocument()
  })

  it('renders spans table when spans tab is clicked', () => {
    mockUseTrace.mockReturnValue({ data: MOCK_TRACE, isLoading: false } as never)
    render(<TraceDetail />)
    fireEvent.click(screen.getByRole('button', { name: /spans/i }))
    expect(screen.getByText('Span ID')).toBeInTheDocument()
    expect(screen.getAllByText('Duration').length).toBeGreaterThanOrEqual(1)
  })

  it('renders blocked span outcomes from normalized status', () => {
    const blockedTrace = {
      ...MOCK_TRACE,
      status: 'partial' as const,
      spans: [
        MOCK_SPANS[0],
        {
          ...MOCK_SPANS[1],
          status_code: 429,
          outcome_status: 'blocked' as const,
          blocked: true,
        },
      ],
    }
    mockUseTrace.mockReturnValue({ data: blockedTrace, isLoading: false } as never)
    render(<TraceDetail />)
    fireEvent.click(screen.getByRole('button', { name: /spans/i }))
    expect(screen.getByText('BLOCKED')).toBeInTheDocument()
  })

  it('renders graph tab content', () => {
    mockUseTrace.mockReturnValue({ data: MOCK_TRACE, isLoading: false } as never)
    render(<TraceDetail />)
    fireEvent.click(screen.getByRole('button', { name: /graph/i }))
    expect(screen.getByTestId('topology-graph')).toBeInTheDocument()
  })

  it('shows policy decision cards when trace has policy events', () => {
    mockUseTrace.mockReturnValue({ data: MOCK_TRACE, isLoading: false } as never)
    render(<TraceDetail />)
    expect(screen.getByText('POLICY DECISIONS')).toBeInTheDocument()
    expect(screen.getByText('deny-gpt4o')).toBeInTheDocument()
  })

  it('shows span detail and correlated policy decisions after selecting a span', () => {
    mockUseTrace.mockReturnValue({ data: MOCK_TRACE, isLoading: false } as never)
    render(<TraceDetail />)
    fireEvent.click(screen.getAllByText('child_llm_call')[0])
    expect(screen.getByText('SPAN DETAIL')).toBeInTheDocument()
    expect(screen.getAllByText('Policy Decisions').length).toBeGreaterThanOrEqual(1)
    expect(screen.getAllByText('provider/model policy matched').length).toBeGreaterThanOrEqual(1)
  })

  it('handles empty spans gracefully', () => {
    const traceNoSpans = { ...MOCK_TRACE, spans: [], span_count: 0, policy_events: [] }
    mockUseTrace.mockReturnValue({ data: traceNoSpans, isLoading: false } as never)
    render(<TraceDetail />)
    expect(screen.getByText(TRACE_ID)).toBeInTheDocument()
    expect(screen.queryByText('root_operation')).not.toBeInTheDocument()
  })
})
