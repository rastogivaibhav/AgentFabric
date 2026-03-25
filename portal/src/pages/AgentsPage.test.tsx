import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import AgentsPage from './AgentsPage'

vi.mock('../hooks/api', () => ({
  useAgentScorecards: vi.fn(),
  useAgentScorecard: vi.fn(),
}))

import { useAgentScorecard, useAgentScorecards } from '../hooks/api'

const mockUseAgentScorecards = vi.mocked(useAgentScorecards)
const mockUseAgentScorecard = vi.mocked(useAgentScorecard)

const scorecards = [
  {
    agent_id: 'support-bot',
    agent_name: 'support-bot',
    framework: 'openai_agents',
    app_name: 'ops-ui',
    environment: 'production',
    release_tag: '2026.04',
    run_count: 42,
    total_cost_usd: 4.25,
    total_tokens: 52000,
    avg_latency_ms: 410.5,
    eval_count: 6,
    overall_score: 88.4,
    risk_level: 'low',
    trend: { previous_overall_score: 81.2, current_overall_score: 88.4, delta: 7.2, direction: 'improving' },
    components: [
      { key: 'reliability', label: 'Reliability', score: 92, weight: 0.3, severity: 'low', summary: '4.0% run errors with 2 fallback events across 42 runs' },
      { key: 'policy_risk', label: 'Policy Risk', score: 84, weight: 0.2, severity: 'low', summary: '2 policy blocks, 1 redactions, 0 budget denials' },
      { key: 'cost_efficiency', label: 'Cost Efficiency', score: 80, weight: 0.15, severity: 'medium', summary: '$0.0817 per 1K tokens with 410.5ms average latency' },
      { key: 'regression_risk', label: 'Regression Risk', score: 90, weight: 0.2, severity: 'low', summary: '6 evals, latest 91.0, recent delta 3.20' },
      { key: 'release_health', label: 'Release Health', score: 88, weight: 0.15, severity: 'low', summary: '10 rollout events, 8.0% error rate, 0 auto-pauses' },
    ],
    recommended_actions: ['Maintain the current release posture and monitor for regressions.'],
    generated_at: '2026-03-25T12:00:00Z',
  },
  {
    agent_id: 'billing-agent',
    agent_name: 'billing-agent',
    framework: 'langgraph',
    app_name: 'billing-ui',
    environment: 'staging',
    release_tag: 'candidate-7',
    run_count: 16,
    total_cost_usd: 6.75,
    total_tokens: 14000,
    avg_latency_ms: 930.2,
    eval_count: 1,
    overall_score: 58.1,
    risk_level: 'high',
    trend: { previous_overall_score: 64.4, current_overall_score: 58.1, delta: -6.3, direction: 'declining' },
    components: [
      { key: 'reliability', label: 'Reliability', score: 61, weight: 0.3, severity: 'high', summary: '22.0% run errors with 4 fallback events across 16 runs' },
      { key: 'policy_risk', label: 'Policy Risk', score: 55, weight: 0.2, severity: 'high', summary: 'No policy or budget decision evidence recorded in the scoring window.' },
      { key: 'cost_efficiency', label: 'Cost Efficiency', score: 44, weight: 0.15, severity: 'high', summary: '$0.4821 per 1K tokens with 930.2ms average latency' },
      { key: 'regression_risk', label: 'Regression Risk', score: 52, weight: 0.2, severity: 'high', summary: '1 evals, latest 52.0, recent delta -6.00' },
      { key: 'release_health', label: 'Release Health', score: 49, weight: 0.15, severity: 'high', summary: '6 rollout events, 35.0% error rate, 1 auto-pauses' },
    ],
    recommended_actions: ['Pause or narrow canaries until rollout error rates stabilize.'],
    generated_at: '2026-03-25T12:00:00Z',
  },
]

describe('AgentsPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseAgentScorecards.mockReturnValue({
      data: { items: scorecards, total: 2, has_more: false },
      isLoading: false,
      error: null,
    } as any)
    mockUseAgentScorecard.mockImplementation((agentId: string) => ({
      data: scorecards.find(item => item.agent_id === agentId) ?? scorecards[0],
      isLoading: false,
      error: null,
    } as any))
  })

  it('renders scorecard summary cards and selected agent detail', () => {
    render(<AgentsPage />)
    expect(screen.getByText('Agent Scorecards')).toBeInTheDocument()
    expect(screen.getAllByText('support-bot')[0]).toBeInTheDocument()
    expect(screen.getByText('billing-agent')).toBeInTheDocument()
    expect(screen.getByText('AVERAGE SCORE')).toBeInTheDocument()
    expect(screen.getAllByText('88.4')[0]).toBeInTheDocument()
    expect(screen.getByText(/maintain the current release posture/i)).toBeInTheDocument()
  })

  it('switches drill-down when a different agent card is selected', () => {
    render(<AgentsPage />)
    fireEvent.click(screen.getByRole('button', { name: /billing-agent/i }))
    expect(screen.getByText(/candidate-7/i)).toBeInTheDocument()
    expect(screen.getByText(/pause or narrow canaries until rollout error rates stabilize/i)).toBeInTheDocument()
    expect(screen.getByText('49.0')).toBeInTheDocument()
  })

  it('shows empty state when no scorecards are returned', () => {
    mockUseAgentScorecards.mockReturnValue({
      data: { items: [], total: 0, has_more: false },
      isLoading: false,
      error: null,
    } as any)
    mockUseAgentScorecard.mockReturnValue({ data: undefined, isLoading: false, error: null } as any)
    render(<AgentsPage />)
    expect(screen.getByText(/No scored agents yet/i)).toBeInTheDocument()
  })
})
