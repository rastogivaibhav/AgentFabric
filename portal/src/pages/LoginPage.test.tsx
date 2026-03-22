import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import LoginPage from './LoginPage'

const navigate = vi.fn()

vi.mock('react-router-dom', () => ({
  useNavigate: () => navigate,
}))

vi.mock('../hooks/auth', () => ({
  useAuth: vi.fn(),
  isAuthEnabled: vi.fn(),
}))

import { isAuthEnabled, useAuth } from '../hooks/auth'

const mockUseAuth = vi.mocked(useAuth)
const mockIsAuthEnabled = vi.mocked(isAuthEnabled)

describe('LoginPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseAuth.mockReturnValue({
      isAuthenticated: false,
      isLoading: false,
      login: vi.fn(),
    } as any)
    mockIsAuthEnabled.mockReturnValue(true)
    vi.stubGlobal('fetch', vi.fn())
  })

  it('redirects to dashboard when auth is disabled', () => {
    mockIsAuthEnabled.mockReturnValue(false)
    render(<LoginPage />)
    expect(navigate).toHaveBeenCalledWith('/dashboard', { replace: true })
  })

  it('shows validation error when credentials are empty', async () => {
    render(<LoginPage />)
    fireEvent.click(screen.getByRole('button', { name: /sign in/i }))
    expect(await screen.findByText('Username and password are required.')).toBeInTheDocument()
  })

  it('submits credentials and navigates on success', async () => {
    vi.mocked(fetch).mockResolvedValue({
      ok: true,
      json: async () => ({ token: 'ignored' }),
    } as any)

    render(<LoginPage />)
    fireEvent.change(screen.getByLabelText('Username'), { target: { value: 'admin' } })
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'secret' } })
    fireEvent.click(screen.getByRole('button', { name: /sign in/i }))

    await waitFor(() => {
      expect(fetch).toHaveBeenCalled()
      expect(navigate).toHaveBeenCalledWith('/dashboard', { replace: true })
    })
  })

  it('shows server error message on failed login', async () => {
    vi.mocked(fetch).mockResolvedValue({
      ok: false,
      json: async () => ({ error: 'Invalid credentials.' }),
    } as any)

    render(<LoginPage />)
    fireEvent.change(screen.getByLabelText('Username'), { target: { value: 'admin' } })
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'bad' } })
    fireEvent.click(screen.getByRole('button', { name: /sign in/i }))

    expect(await screen.findByText('Invalid credentials.')).toBeInTheDocument()
  })
})
