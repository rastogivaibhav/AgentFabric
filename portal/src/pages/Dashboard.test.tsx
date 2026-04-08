import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'

vi.mock('../hooks/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../hooks/api')>()
  return {
    ...actual,
    useOverview: vi.fn(),
    useRollouts: vi.fn(),
    useTraces: vi.fn(),
    useBudgetUsage: vi.fn(),
    useControlAudit: vi.fn(),
    useEvidenceBundles: vi.fn(),
  }
})

import Dashboard from './Dashboard'
import {
  useOverview,
  useRollouts,
  useTraces,
  useBudgetUsage,
  useControlAudit,
  useEvidenceBundles,
} from '../hooks/api'

const mockUseOverview = vi.mocked(useOverview)
const mockUseRollouts = vi.mocked(useRollouts)
const mockUseTraces = vi.mocked(useTraces)
const mockUseBudgetUsage = vi.mocked(useBudgetUsage)
const mockUseControlAudit = vi.mocked(useControlAudit)
const mockUseEvidenceBundles = vi.mocked(useEvidenceBundles)

function renderPage() {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })

  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <Dashboard />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('Dashboard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseOverview.mockReturnValue({ data: undefined, isLoading: false } as any)
    mockUseRollouts.mockReturnValue({ data: { items: [] } } as any)
    mockUseTraces.mockReturnValue({ data: { items: [] } } as any)
    mockUseBudgetUsage.mockReturnValue({ data: undefined } as any)
    mockUseControlAudit.mockReturnValue({ data: { items: [] } } as any)
    mockUseEvidenceBundles.mockReturnValue({ data: { items: [] } } as any)
  })

  it('renders loading placeholders when overview is loading', () => {
    mockUseOverview.mockReturnValue({ data: undefined, isLoading: true } as any)

    renderPage()

    const dashes = screen.getAllByText((_, node) => node?.textContent === '—')
    expect(dashes.length).toBeGreaterThanOrEqual(2)
  })

  it('renders dashboard heading', () => {
    renderPage()
    expect(screen.getByRole('heading', { name: 'Your AI. Under control.' })).toBeInTheDocument()
  })

  it('renders total traces from overview data', () => {
    mockUseOverview.mockReturnValue({
      data: {
        total_traces: 1234,
        total_cost_usd: 24,
        blocked_requests: 0,
        error_rate: 0,
      },
      isLoading: false,
    } as any)

    renderPage()
    expect(screen.getByText('1,234')).toBeInTheDocument()
  })

  it('renders spend per hour derived from 24h total cost', () => {
    mockUseOverview.mockReturnValue({
      data: {
        total_traces: 50,
        total_cost_usd: 12.3456,
        blocked_requests: 0,
        error_rate: 0,
      },
      isLoading: false,
    } as any)

    renderPage()
    expect(screen.getByText('$0.5144')).toBeInTheDocument()
  })

  it('renders active rollouts and progress', () => {
    mockUseRollouts.mockReturnValue({
      data: {
        items: [
          { id: 1, name: 'Prompt canary', status: 'active', percentage: 25, target_type: 'model' },
          { id: 2, name: 'Budget guardrail', status: 'active', percentage: 60, target_type: 'policy' },
          { id: 3, name: 'Dormant rollout', status: 'paused', percentage: 5, target_type: 'model' },
        ],
      },
    } as any)

    renderPage()

    expect(screen.getByText('Prompt canary')).toBeInTheDocument()
    expect(screen.getByText('25%')).toBeInTheDocument()
    expect(screen.getByText('Budget guardrail')).toBeInTheDocument()
    expect(screen.getByText('60%')).toBeInTheDocument()
  })

  it('renders recent trace status labels in uppercase', () => {
    mockUseTraces.mockReturnValue({
      data: {
        items: [
          {
            id: 'trace-1',
            status: 'partial',
            error_count: 0,
            framework: 'crewai',
            root_span_name: 'planner.run',
            total_cost_usd: 0.12345,
            start_time: '2026-03-25T12:00:00Z',
          },
        ],
      },
    } as any)

    renderPage()
    expect(screen.getByText('PARTIAL')).toBeInTheDocument()
  })

  it('renders core widget labels', () => {
    renderPage()
    expect(screen.getByText('LIVE ACTIVITY FEED')).toBeInTheDocument()
    expect(screen.getByText('ACTIVE ROLLOUTS')).toBeInTheDocument()
    expect(screen.getByText('BUDGET STATUS')).toBeInTheDocument()
  })

  it('renders budget usage summary when configured', () => {
    mockUseBudgetUsage.mockReturnValue({
      data: {
        cost_used_usd: 50,
        cost_pct: 0.5,
        tokens_used: 30000,
        tokens_pct: 0.3,
        budget: {
          monthly_cost_usd: 100,
          monthly_tokens: 100000,
        },
      },
    } as any)

    renderPage()
    expect(screen.getByText('$50.00 / $100')).toBeInTheDocument()
    expect(screen.getByText('30k / 100k')).toBeInTheDocument()
  })
})
