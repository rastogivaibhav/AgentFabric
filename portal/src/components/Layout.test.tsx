import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import Layout from './Layout'

vi.mock('../hooks/auth', () => ({
  useAuth: vi.fn(),
}))

import { useAuth } from '../hooks/auth'

const mockUseAuth = vi.mocked(useAuth)

describe('Layout', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  function renderLayout() {
    render(
      <MemoryRouter initialEntries={['/dashboard']}>
        <Routes>
          <Route element={<Layout />}>
            <Route path="/dashboard" element={<div>dashboard body</div>} />
          </Route>
        </Routes>
      </MemoryRouter>,
    )
  }

  it('shows admin navigation items for admins', () => {
    mockUseAuth.mockReturnValue({
      user: { sub: '1', email: 'admin@example.com', name: 'Admin', role: 'admin' },
      logout: vi.fn(),
    } as any)

    renderLayout()

    expect(screen.getByText('Dashboard')).toBeInTheDocument()
    expect(screen.getByText('API Keys')).toBeInTheDocument()
    expect(screen.getByText('Policies')).toBeInTheDocument()
    expect(screen.getByText('Prompts')).toBeInTheDocument()
    expect(screen.getByText('Evals')).toBeInTheDocument()
    expect(screen.getByText('Decisions')).toBeInTheDocument()
  })

  it('hides admin-only navigation items for viewers', () => {
    mockUseAuth.mockReturnValue({
      user: { sub: '2', email: 'viewer@example.com', name: 'Viewer', role: 'viewer' },
      logout: vi.fn(),
    } as any)

    renderLayout()

    expect(screen.getByText('Dashboard')).toBeInTheDocument()
    expect(screen.queryByText('API Keys')).not.toBeInTheDocument()
    expect(screen.queryByText('Policies')).not.toBeInTheDocument()
    expect(screen.queryByText('Prompts')).not.toBeInTheDocument()
    expect(screen.queryByText('Decisions')).not.toBeInTheDocument()
  })

  it('calls logout when the sign-out button is pressed', () => {
    const logout = vi.fn()
    mockUseAuth.mockReturnValue({
      user: { sub: '1', email: 'admin@example.com', name: 'Admin', role: 'admin' },
      logout,
    } as any)

    renderLayout()
    fireEvent.click(screen.getByTitle('Sign out'))
    expect(logout).toHaveBeenCalled()
  })
})
