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
}))

import { hasRole, useAuth } from '../hooks/auth'
import { usePromotePromptRelease, usePrompts } from '../hooks/api'

const mockUseAuth = vi.mocked(useAuth)
const mockHasRole = vi.mocked(hasRole)
const mockUsePrompts = vi.mocked(usePrompts)
const mockUsePromotePromptRelease = vi.mocked(usePromotePromptRelease)

describe('PromptReleasePage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseAuth.mockReturnValue({ user: { role: 'admin' } } as any)
    mockHasRole.mockReturnValue(true)
    mockUsePrompts.mockReturnValue({
      data: {
        items: [
          { id: 1, prompt_id: 'support-bot.system', version: 1, environment: 'development', release_tag: 'dev-1', content: 'dev prompt' },
          { id: 2, prompt_id: 'support-bot.system', version: 2, environment: 'production', release_tag: 'prod-2', content: 'prod prompt' },
        ],
        releases: [
          { id: 4, prompt_id: 'support-bot.system', version: 2, environment: 'production', release_tag: '2026.03' },
        ],
      },
      isLoading: false,
      error: null,
    } as any)
    mockUsePromotePromptRelease.mockReturnValue({ mutate: vi.fn(), isPending: false } as any)
  })

  it('shows selected prompt details', () => {
    render(<MemoryRouter><PromptReleasePage /></MemoryRouter>)
    expect(screen.getByText('support-bot.system')).toBeInTheDocument()
    expect(screen.getByText('ACTIVE RELEASE POINTERS')).toBeInTheDocument()
  })

  it('submits a prompt promotion', () => {
    const mutate = vi.fn()
    mockUsePromotePromptRelease.mockReturnValue({ mutate, isPending: false } as any)
    render(<MemoryRouter><PromptReleasePage /></MemoryRouter>)

    fireEvent.change(screen.getByPlaceholderText('2026.03-prod.1'), { target: { value: '2026.04' } })
    fireEvent.click(screen.getByRole('button', { name: 'Promote Release' }))

    expect(mutate).toHaveBeenCalledWith({
      prompt_id: 'support-bot.system',
      environment: 'development',
      version: 1,
      release_tag: '2026.04',
      notes: '',
    })
  })
})
