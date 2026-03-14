import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import React from 'react'

vi.mock('../hooks/api', () => ({
  useTrace: vi.fn(),
}))

vi.mock('react-router-dom', () => ({
  useParams: vi.fn(),
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
}

describe('TraceDetail', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseParams.mockReturnValue({ traceId: TRACE_ID })
  })

  it('renders loading state while useTrace fetches', () => {
    mockUseTrace.mockReturnValue({ data: undefined, isLoading: true } as any)
    render(<TraceDetail />)
    expect(screen.getByText('Loading trace…')).toBeInTheDocument()
  })

  it('renders not found state when useTrace returns null', () => {
    mockUseTrace.mockReturnValue({ data: null, isLoading: false } as any)
    render(<TraceDetail />)
    expect(screen.getByText('Trace not found')).toBeInTheDocument()
  })

  it('renders trace header with trace ID', () => {
    mockUseTrace.mockReturnValue({ data: MOCK_TRACE, isLoading: false } as any)
    render(<TraceDetail />)
    expect(screen.getByText(TRACE_ID)).toBeInTheDocument()
  })

  it('renders trace header stats: framework, duration, spans', () => {
    mockUseTrace.mockReturnValue({ data: MOCK_TRACE, isLoading: false } as any)
    render(<TraceDetail />)
    expect(screen.getByText('crewai')).toBeInTheDocument()
    // span count shown as string
    expect(screen.getByText('2')).toBeInTheDocument()
    // duration in seconds (1.2s)
    expect(screen.getByText('1.20s')).toBeInTheDocument()
  })

  it('waterfall tab is active by default and renders span rows', () => {
    mockUseTrace.mockReturnValue({ data: MOCK_TRACE, isLoading: false } as any)
    render(<TraceDetail />)
    // span names appear in waterfall
    expect(screen.getByText('root_operation')).toBeInTheDocument()
    expect(screen.getByText('child_llm_call')).toBeInTheDocument()
  })

  it('spans table tab renders span columns when clicked', () => {
    mockUseTrace.mockReturnValue({ data: MOCK_TRACE, isLoading: false } as any)
    render(<TraceDetail />)
    const spansTab = screen.getByRole('button', { name: /spans/i })
    fireEvent.click(spansTab)
    expect(screen.getByText('Span ID')).toBeInTheDocument()
    expect(screen.getByText('Name')).toBeInTheDocument()
    expect(screen.getByText('Framework')).toBeInTheDocument()
    expect(screen.getByText('Duration')).toBeInTheDocument()
  })

  it('graph tab renders TopologyGraph component with spans prop', () => {
    mockUseTrace.mockReturnValue({ data: MOCK_TRACE, isLoading: false } as any)
    render(<TraceDetail />)
    const graphTab = screen.getByRole('button', { name: /graph/i })
    fireEvent.click(graphTab)
    const graph = screen.getByTestId('topology-graph')
    expect(graph).toBeInTheDocument()
    expect(graph).toHaveAttribute('data-span-count', '2')
  })

  it('clicking a span in the waterfall selects it and shows detail panel', () => {
    mockUseTrace.mockReturnValue({ data: MOCK_TRACE, isLoading: false } as any)
    render(<TraceDetail />)
    // Click on the root span row wrapper
    const spanName = screen.getByText('root_operation')
    // The clickable div wraps the WaterfallRow — go up to the div with onClick
    const clickable = spanName.closest('div[style]')!
    fireEvent.click(clickable)
    // Detail panel heading should appear
    expect(screen.getByText('SPAN DETAIL')).toBeInTheDocument()
  })

  it('clicking a span in the spans table selects it and shows detail panel', () => {
    mockUseTrace.mockReturnValue({ data: MOCK_TRACE, isLoading: false } as any)
    render(<TraceDetail />)
    const spansTab = screen.getByRole('button', { name: /spans/i })
    fireEvent.click(spansTab)
    // Find the span name in the table body
    const spanCell = screen.getAllByText('root_operation')[0]
    const row = spanCell.closest('tr')!
    fireEvent.click(row)
    expect(screen.getByText('SPAN DETAIL')).toBeInTheDocument()
  })

  it('switching between tabs works correctly', () => {
    mockUseTrace.mockReturnValue({ data: MOCK_TRACE, isLoading: false } as any)
    render(<TraceDetail />)
    // Start on waterfall
    expect(screen.getByText('root_operation')).toBeInTheDocument()
    // Switch to spans
    fireEvent.click(screen.getByRole('button', { name: /^spans$/i }))
    expect(screen.getByText('Span ID')).toBeInTheDocument()
    // Switch to graph
    fireEvent.click(screen.getByRole('button', { name: /^graph$/i }))
    expect(screen.getByTestId('topology-graph')).toBeInTheDocument()
    // Switch back to waterfall
    fireEvent.click(screen.getByRole('button', { name: /^waterfall$/i }))
    expect(screen.getByText('SPAN NAME')).toBeInTheDocument()
  })

  it('handles empty spans array gracefully', () => {
    const traceNoSpans = { ...MOCK_TRACE, spans: [], span_count: 0 }
    mockUseTrace.mockReturnValue({ data: traceNoSpans, isLoading: false } as any)
    render(<TraceDetail />)
    // Should still render the header
    expect(screen.getByText(TRACE_ID)).toBeInTheDocument()
    // Waterfall is empty — no span names
    expect(screen.queryByText('root_operation')).not.toBeInTheDocument()
  })
})
