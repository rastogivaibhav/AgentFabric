import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import PromptsPage from './PromptsPage'

vi.mock('../hooks/auth', () => ({
  useAuth: vi.fn(),
  hasRole: vi.fn(),
}))

vi.mock('../hooks/api', () => ({
  usePrompts: vi.fn(),
  useUpsertPromptVersion: vi.fn(),
}))

import { hasRole, useAuth } from '../hooks/auth'
import { usePrompts, useUpsertPromptVersion } from '../hooks/api'

const mockUseAuth = vi.mocked(useAuth)
const mockHasRole = vi.mocked(hasRole)
const mockUsePrompts = vi.mocked(usePrompts)
const mockUseUpsertPromptVersion = vi.mocked(useUpsertPromptVersion)

describe('PromptsPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseAuth.mockReturnValue({ user: { role: 'admin' } } as any)
    mockHasRole.mockReturnValue(true)
    mockUsePrompts.mockReturnValue({ data: { items: [], releases: [], count: 0 }, isLoading: false, error: null } as any)
    mockUseUpsertPromptVersion.mockReturnValue({ mutate: vi.fn(), isPending: false } as any)
  })

  it('shows restricted message for non-admin users', () => {
    mockHasRole.mockReturnValue(false)
    render(<PromptsPage />)
    expect(screen.getByText(/restricted to administrators/i)).toBeInTheDocument()
  })

  it('renders prompt cards and live badges', () => {
    mockUsePrompts.mockReturnValue({
      data: {
        items: [{
          id: 10,
          prompt_id: 'support-bot.system',
          version: 3,
          environment: 'production',
          release_tag: '2026.03',
          content: 'You are a support assistant.',
          description: 'production prompt',
          promoted: true,
          is_latest: true,
          current_release: {
            id: 11,
            prompt_id: 'support-bot.system',
            version: 3,
            environment: 'production',
            release_tag: '2026.03',
            status: 'active',
            eval_summary: {
              eval_count: 4,
              average_score: 88.2,
              latest_score: 89.1,
              risk_level: 'low',
            },
            regression_summary: {
              baseline_tag: '2026.02',
              candidate_tag: '2026.03',
              compared_runs: 4,
              overall_delta: 3.4,
              risk_level: 'low',
              summary: 'Average eval score moved by 3.40 points versus 2026.02.',
            },
          },
        }],
        releases: [],
        count: 1,
      },
      isLoading: false,
      error: null,
    } as any)

    render(<MemoryRouter><PromptsPage /></MemoryRouter>)
    expect(screen.getByText('support-bot.system')).toBeInTheDocument()
    expect(screen.getByText('LIVE')).toBeInTheDocument()
    expect(screen.getByText('LATEST')).toBeInTheDocument()
    expect(screen.getByText('ACTIVE')).toBeInTheDocument()
    expect(screen.getByText(/active release 2026.03/i)).toBeInTheDocument()
  })

  it('submits a prompt version', () => {
    const mutate = vi.fn()
    mockUseUpsertPromptVersion.mockReturnValue({ mutate, isPending: false } as any)

    render(<MemoryRouter><PromptsPage /></MemoryRouter>)
    fireEvent.change(screen.getByPlaceholderText('support-bot.system'), { target: { value: 'policy-bot.system' } })
    fireEvent.change(screen.getByPlaceholderText('development'), { target: { value: 'production' } })
    fireEvent.change(screen.getByRole('textbox', { name: 'Content' }), { target: { value: 'Hello world' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create Version' }))

    expect(mutate).toHaveBeenCalledWith(expect.objectContaining({
      prompt_id: 'policy-bot.system',
      environment: 'production',
      content: 'Hello world',
    }), expect.any(Object))
  })
})
