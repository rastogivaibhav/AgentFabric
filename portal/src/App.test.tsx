import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import App, { RequireRole } from './App'

vi.mock('./hooks/auth', () => ({
  useAuth: vi.fn(),
  isAuthEnabled: vi.fn(),
  hasRole: vi.fn(),
}))

vi.mock('./components/Layout', async () => {
  const actual = await vi.importActual<typeof import('react-router-dom')>('react-router-dom')
  return {
    default: () => (
      <div>
        layout
        <actual.Outlet />
      </div>
    ),
  }
})

vi.mock('./pages/Dashboard', () => ({ default: () => <div>dashboard</div> }))
vi.mock('./pages/TracesPage', () => ({ default: () => <div>traces</div> }))
vi.mock('./pages/TraceDetail', () => ({ default: () => <div>trace-detail</div> }))
vi.mock('./pages/TraceComparePage', () => ({ default: () => <div>trace-compare</div> }))
vi.mock('./pages/LiveStream', () => ({ default: () => <div>live</div> }))
vi.mock('./pages/AgentsPage', () => ({ default: () => <div>agents</div> }))
vi.mock('./pages/CostPage', () => ({ default: () => <div>cost</div> }))
vi.mock('./pages/EnvironmentsPage', () => ({ default: () => <div>environments</div> }))
vi.mock('./pages/UsersPage', () => ({ default: () => <div>users</div> }))
vi.mock('./pages/AuditPage', () => ({ default: () => <div>audit</div> }))
vi.mock('./pages/ApiKeysPage', () => ({ default: () => <div>keys</div> }))
vi.mock('./pages/PricingRulesPage', () => ({ default: () => <div>pricing</div> }))
vi.mock('./pages/PoliciesPage', () => ({ default: () => <div>policies</div> }))
vi.mock('./pages/EvalsPage', () => ({ default: () => <div>evals</div> }))
vi.mock('./pages/RegressionPage', () => ({ default: () => <div>regressions</div> }))
vi.mock('./pages/PromptsPage', () => ({ default: () => <div>prompts</div> }))
vi.mock('./pages/PromptReleasePage', () => ({ default: () => <div>prompt-release</div> }))
vi.mock('./pages/LoginPage', () => ({ default: () => <div>login</div> }))

import { hasRole, isAuthEnabled, useAuth } from './hooks/auth'

const mockUseAuth = vi.mocked(useAuth)
const mockIsAuthEnabled = vi.mocked(isAuthEnabled)
const mockHasRole = vi.mocked(hasRole)

describe('App', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseAuth.mockReturnValue({
      user: { role: 'admin' },
      isAuthenticated: true,
      isLoading: false,
      login: vi.fn(),
      logout: vi.fn(),
      refetch: vi.fn(),
    } as any)
    mockIsAuthEnabled.mockReturnValue(true)
    mockHasRole.mockImplementation((user, roles) => Boolean(user && roles.includes(user.role)))
    window.history.replaceState({}, '', '/')
  })

  it('redirects index to dashboard when authenticated', async () => {
    render(<App />)
    await waitFor(() => {
      expect(screen.getByText('layout')).toBeInTheDocument()
      expect(screen.getByText('dashboard')).toBeInTheDocument()
    })
  })

  it('shows login route when auth is disabled', async () => {
    mockIsAuthEnabled.mockReturnValue(false)
    window.history.replaceState({}, '', '/login')
    render(<App />)
    expect(await screen.findByText('login')).toBeInTheDocument()
  })

  it('redirects unauthenticated users to login', async () => {
    mockUseAuth.mockReturnValue({
      user: null,
      isAuthenticated: false,
      isLoading: false,
      login: vi.fn(),
      logout: vi.fn(),
      refetch: vi.fn(),
    } as any)
    window.history.replaceState({}, '', '/dashboard')

    render(<App />)
    expect(await screen.findByText('login')).toBeInTheDocument()
  })

  it('guards content with RequireRole', () => {
    mockHasRole.mockReturnValue(false)
    render(<RequireRole roles={['admin']} fallback={<div>fallback</div>}><div>secret</div></RequireRole>)
    expect(screen.getByText('fallback')).toBeInTheDocument()
  })
})
