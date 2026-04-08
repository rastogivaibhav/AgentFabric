import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'

vi.mock('../hooks/auth', () => ({
  useAuth: vi.fn(),
  hasRole: vi.fn(),
}))

vi.mock('../hooks/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../hooks/api')>()
  return {
    ...actual,
    useDecisions: vi.fn(),
  }
})

import DecisionsPage from './DecisionsPage'
import { hasRole, useAuth } from '../hooks/auth'
import { useDecisions } from '../hooks/api'

const mockUseAuth = vi.mocked(useAuth)
const mockHasRole = vi.mocked(hasRole)
const mockUseDecisions = vi.mocked(useDecisions)

describe('DecisionsPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseAuth.mockReturnValue({ user: { role: 'admin' } } as any)
    mockHasRole.mockReturnValue(true)
  })

  function renderPage() {
    render(
      <MemoryRouter>
        <DecisionsPage />
      </MemoryRouter>,
    )
  }

  it('renders decision records', () => {
    mockUseDecisions.mockReturnValue({
      data: {
        items: [{
          id: 1,
          decision_id: 'decision-1',
          type: 'policy',
          result: 'deny',
          trace_id: 'trace-1',
          explanation: 'Policy blocked gpt-4o because provider/model policy matched.',
          action_taken: 'block_request',
          created_at: '2026-03-24T18:00:00Z',
        }],
        total: 1,
        has_more: false,
      },
      isLoading: false,
      isError: false,
    } as any)

    renderPage()

    expect(screen.getByText('Decision Log')).toBeInTheDocument()
    expect(screen.getByText('Policy blocked gpt-4o because provider/model policy matched.')).toBeInTheDocument()
  })

  it('applies filters through the decisions hook', () => {
    mockUseDecisions.mockReturnValue({ data: { items: [], total: 0, has_more: false }, isLoading: false, isError: false } as any)
    renderPage()

    const [typeFilter, resultFilter] = screen.getAllByRole('combobox')
    fireEvent.change(typeFilter, { target: { value: 'fallback' } })
    fireEvent.change(resultFilter, { target: { value: 'retry' } })

    expect(mockUseDecisions).toHaveBeenLastCalledWith(expect.objectContaining({
      type: 'fallback',
      result: 'retry',
    }))
  })

  it('shows empty state when there are no decisions', () => {
    mockUseDecisions.mockReturnValue({ data: { items: [], total: 0, has_more: false }, isLoading: false, isError: false } as any)
    renderPage()
    expect(screen.getByText('No decisions matched the current filters.')).toBeInTheDocument()
  })

  it('shows error state when the query fails', () => {
    mockUseDecisions.mockReturnValue({ data: undefined, isLoading: false, isError: true } as any)
    renderPage()
    expect(screen.getByText('Failed to load decision evidence.')).toBeInTheDocument()
  })
})
