import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import PromptReleasePage from './PromptReleasePage'

vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual<typeof import('react-router-dom')>('react-router-dom')
  return {
    ...actual,
    useParams: () => ({ promptId: 'support-bot.system' }),
  }
})

vi.mock('../hooks/auth', () => ({
  useAuth: vi.fn(),
  hasRole: vi.fn(),
}))

vi.mock('../hooks/api', () => ({
  usePrompts: vi.fn(),
  usePromotePromptRelease: vi.fn(),
  useRollouts: vi.fn(),
  useUpsertRolloutRule: vi.fn(),
  usePreviewRollout: vi.fn(),
  useUpdateRolloutStatus: vi.fn(),
}))

import { hasRole, useAuth } from '../hooks/auth'
import {
  usePreviewRollout,
  usePromotePromptRelease,
  usePrompts,
  useRollouts,
  useUpdateRolloutStatus,
  useUpsertRolloutRule,
} from '../hooks/api'

const mockUseAuth = vi.mocked(useAuth)
const mockHasRole = vi.mocked(hasRole)
const mockUsePrompts = vi.mocked(usePrompts)
const mockUsePromotePromptRelease = vi.mocked(usePromotePromptRelease)
const mockUseRollouts = vi.mocked(useRollouts)
const mockUseUpsertRolloutRule = vi.mocked(useUpsertRolloutRule)
const mockUsePreviewRollout = vi.mocked(usePreviewRollout)
const mockUseUpdateRolloutStatus = vi.mocked(useUpdateRolloutStatus)

describe('PromptReleasePage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseAuth.mockReturnValue({ user: { role: 'admin' } } as any)
    mockHasRole.mockReturnValue(true)
    mockUsePrompts.mockReturnValue({
      data: {
        items: [
          { id: 1, prompt_id: 'support-bot.system', version: 1, environment: 'development', release_tag: 'dev-1', content: 'dev prompt' },
          { id: 2, prompt_id: 'support-bot.system', version: 2, environment: 'production', release_tag: 'prod-2', content: 'prod prompt', current_release: { id: 4, prompt_id: 'support-bot.system', version: 2, environment: 'production', release_tag: '2026.03', status: 'active', eval_summary: { eval_count: 3, average_score: 84.5, latest_score: 86.1, risk_level: 'low' }, regression_summary: { baseline_tag: '2026.02', candidate_tag: '2026.03', compared_runs: 3, overall_delta: -2.4, risk_level: 'low', summary: 'Average eval score moved by -2.40 points versus 2026.02.' } } },
        ],
        releases: [
          { id: 4, prompt_id: 'support-bot.system', version: 2, environment: 'production', release_tag: '2026.03', status: 'active', eval_summary: { eval_count: 3, average_score: 84.5, latest_score: 86.1, risk_level: 'low' }, regression_summary: { baseline_tag: '2026.02', candidate_tag: '2026.03', compared_runs: 3, overall_delta: -2.4, risk_level: 'low', summary: 'Average eval score moved by -2.40 points versus 2026.02.' } },
        ],
      },
      isLoading: false,
      error: null,
    } as any)
    mockUsePromotePromptRelease.mockReturnValue({ mutate: vi.fn(), isPending: false } as any)
    mockUseRollouts.mockReturnValue({
      data: {
        items: [
          {
            id: 8,
            name: 'Prompt canary',
            target_type: 'prompt_release',
            target_id: 'support-bot.system',
            environment: 'development',
            percentage: 10,
            control_release_tag: 'stable-2026.03',
            candidate_release_tag: 'candidate-2026.04',
            status: 'active',
            recent_requests: 12,
            recent_error_rate: 0.08,
          },
        ],
        count: 1,
      },
      isLoading: false,
      error: null,
    } as any)
    mockUseUpsertRolloutRule.mockReturnValue({ mutate: vi.fn(), isPending: false, isError: false } as any)
    mockUsePreviewRollout.mockReturnValue({ mutate: vi.fn(), isPending: false, isError: false, data: null } as any)
    mockUseUpdateRolloutStatus.mockReturnValue({ mutate: vi.fn(), isPending: false } as any)
  })

  it('shows selected prompt details', () => {
    render(<MemoryRouter><PromptReleasePage /></MemoryRouter>)
    expect(screen.getByText('support-bot.system')).toBeInTheDocument()
    expect(screen.getByText('RELEASE HISTORY')).toBeInTheDocument()
    expect(screen.getByText(/Average eval score moved by -2.40 points/i)).toBeInTheDocument()
  })

  it('submits a prompt promotion', () => {
    const mutate = vi.fn()
    mockUsePromotePromptRelease.mockReturnValue({ mutate, isPending: false } as any)
    render(<MemoryRouter><PromptReleasePage /></MemoryRouter>)

    fireEvent.change(screen.getByPlaceholderText('2026.03-prod.1'), { target: { value: '2026.04' } })
    fireEvent.change(screen.getByRole('combobox', { name: 'Status' }), { target: { value: 'candidate' } })
    fireEvent.change(screen.getByPlaceholderText('improve escalation quality and reduce hallucinations'), { target: { value: 'reduce escalations' } })
    fireEvent.click(screen.getByRole('button', { name: 'Promote Release' }))

    expect(mutate).toHaveBeenCalledWith({
      prompt_id: 'support-bot.system',
      environment: 'development',
      version: 1,
      release_tag: '2026.04',
      status: 'candidate',
      notes: '',
      promotion_reason: 'reduce escalations',
    })
  })

  it('previews and pauses a prompt rollout', () => {
    const previewMutate = vi.fn()
    const updateStatusMutate = vi.fn()
    mockUsePreviewRollout.mockReturnValue({
      mutate: previewMutate,
      isPending: false,
      isError: false,
      data: {
        assignment: {
          rule_id: 8,
          rule_name: 'Prompt canary',
          variant: 'canary',
          bucket: 7,
          assignment_key: 'session-42',
        },
      },
    } as any)
    mockUseUpdateRolloutStatus.mockReturnValue({ mutate: updateStatusMutate, isPending: false } as any)

    render(<MemoryRouter><PromptReleasePage /></MemoryRouter>)

    fireEvent.click(screen.getByRole('button', { name: 'Preview Rollout' }))
    expect(previewMutate).toHaveBeenCalledWith({
      prompt_id: 'support-bot.system',
      prompt_environment: 'development',
      environment: 'development',
      app: 'ops-ui',
      session: 'session-42',
      assignment_key: '',
    })
    expect(screen.getByText(/Canary selected/i)).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Pause' }))
    expect(updateStatusMutate).toHaveBeenCalledWith({ id: 8, status: 'paused' })
  })
})
