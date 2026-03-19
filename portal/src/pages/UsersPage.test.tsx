import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import React from 'react'

// ─── Mock @tanstack/react-query ───────────────────────────────────────────────
vi.mock('@tanstack/react-query', () => ({
  useQuery:       vi.fn(),
  useMutation:    vi.fn(),
  useQueryClient: vi.fn(),
}))

// ─── Mock ../hooks/auth ───────────────────────────────────────────────────────
vi.mock('../hooks/auth', () => ({
  useAuth:     vi.fn(),
  hasRole:     vi.fn(),
  isSelfOrRole: vi.fn(),
}))

import UsersPage from './UsersPage'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useAuth, hasRole, isSelfOrRole } from '../hooks/auth'

const mockUseQuery      = vi.mocked(useQuery)
const mockUseMutation   = vi.mocked(useMutation)
const mockUseQueryClient = vi.mocked(useQueryClient)
const mockUseAuth       = vi.mocked(useAuth)
const mockHasRole       = vi.mocked(hasRole)
const mockIsSelfOrRole  = vi.mocked(isSelfOrRole)

// ─── Fixtures ─────────────────────────────────────────────────────────────────

const ADMIN_USER  = { sub: 'u1', email: 'admin@af.io',  name: 'Admin',  role: 'admin'  }
const EDITOR_USER = { sub: 'u2', email: 'editor@af.io', name: 'Editor', role: 'editor' }
const VIEWER_USER = { sub: 'u3', email: 'viewer@af.io', name: 'Viewer', role: 'viewer' }

function makeUser(overrides: Partial<{
  user_id: string; username: string; email: string; display_name: string;
  role: string; is_active: boolean; last_login_at: string | null;
  created_at: string; tenant_id: string;
}> = {}) {
  return {
    user_id:       overrides.user_id      ?? 'user-001',
    tenant_id:     overrides.tenant_id    ?? 'default',
    username:      overrides.username     ?? 'testuser',
    email:         overrides.email        ?? 'test@af.io',
    display_name:  overrides.display_name ?? 'Test User',
    role:          overrides.role         ?? 'viewer',
    is_active:     overrides.is_active    ?? true,
    last_login_at: overrides.last_login_at ?? null,
    created_at:    overrides.created_at   ?? '2026-01-01T00:00:00Z',
  }
}

function idleMutation() {
  return { mutate: vi.fn(), isPending: false } as any
}

// ─── Test setup ───────────────────────────────────────────────────────────────

beforeEach(() => {
  vi.clearAllMocks()
  mockUseAuth.mockReturnValue({ user: ADMIN_USER } as any)
  mockHasRole.mockImplementation((user, roles) =>
    user?.role != null && (roles as string[]).includes(user.role)
  )
  mockIsSelfOrRole.mockReturnValue(false)
  mockUseQuery.mockReturnValue({ data: [], isLoading: false, error: null } as any)
  mockUseMutation.mockReturnValue(idleMutation())
  mockUseQueryClient.mockReturnValue({ invalidateQueries: vi.fn() } as any)
})

// ─── Loading state ────────────────────────────────────────────────────────────

describe('UsersPage — loading state', () => {
  it('shows loading indicator while fetching', () => {
    mockUseQuery.mockReturnValue({ data: undefined, isLoading: true, error: null } as any)

    render(<UsersPage />)

    expect(screen.getByText('Loading users…')).toBeInTheDocument()
  })

  it('shows the user count as … while loading', () => {
    mockUseQuery.mockReturnValue({ data: undefined, isLoading: true, error: null } as any)

    render(<UsersPage />)

    expect(screen.getByText(/USERS — …/i)).toBeInTheDocument()
  })
})

// ─── Error state ──────────────────────────────────────────────────────────────

describe('UsersPage — error state', () => {
  it('shows error message when query fails', () => {
    mockUseQuery.mockReturnValue({
      data: undefined, isLoading: false, error: new Error('HTTP 500'),
    } as any)

    render(<UsersPage />)

    expect(screen.getByText(/failed to load users/i)).toBeInTheDocument()
  })
})

// ─── Empty state ──────────────────────────────────────────────────────────────

describe('UsersPage — empty state', () => {
  it('shows no users message when list is empty', () => {
    mockUseQuery.mockReturnValue({ data: [], isLoading: false, error: null } as any)

    render(<UsersPage />)

    expect(screen.getByText('No users found.')).toBeInTheDocument()
  })
})

// ─── Admin view ───────────────────────────────────────────────────────────────

describe('UsersPage — admin view', () => {
  it('shows Create User button for admins', () => {
    render(<UsersPage />)

    expect(screen.getByRole('button', { name: /create user/i })).toBeInTheDocument()
  })

  it('does not show the read-only warning for admins', () => {
    render(<UsersPage />)

    expect(screen.queryByText(/read-only access/i)).not.toBeInTheDocument()
  })

  it('renders total user count', () => {
    const users = [makeUser({ user_id: 'u1' }), makeUser({ user_id: 'u2' })]
    mockUseQuery.mockReturnValue({ data: users, isLoading: false, error: null } as any)

    render(<UsersPage />)

    expect(screen.getByText(/USERS — 2 total/i)).toBeInTheDocument()
  })
})

// ─── Non-admin view ───────────────────────────────────────────────────────────

describe('UsersPage — non-admin view', () => {
  beforeEach(() => {
    mockUseAuth.mockReturnValue({ user: VIEWER_USER } as any)
    mockHasRole.mockReturnValue(false)
  })

  it('hides Create User button for non-admins', () => {
    render(<UsersPage />)

    expect(screen.queryByRole('button', { name: /create user/i })).not.toBeInTheDocument()
  })

  it('shows read-only access notice for non-admins', () => {
    render(<UsersPage />)

    expect(screen.getByText(/read-only access/i)).toBeInTheDocument()
  })
})

// ─── User table rendering ─────────────────────────────────────────────────────

describe('UsersPage — table rendering', () => {
  it('renders username in table', () => {
    mockUseQuery.mockReturnValue({
      data: [makeUser({ username: 'alice' })], isLoading: false, error: null,
    } as any)

    render(<UsersPage />)

    expect(screen.getByText('alice')).toBeInTheDocument()
  })

  it('renders email in table', () => {
    mockUseQuery.mockReturnValue({
      data: [makeUser({ email: 'alice@example.com' })], isLoading: false, error: null,
    } as any)

    render(<UsersPage />)

    expect(screen.getByText('alice@example.com')).toBeInTheDocument()
  })

  it('renders role badge with ADMIN text for admin users', () => {
    mockUseQuery.mockReturnValue({
      data: [makeUser({ role: 'admin' })], isLoading: false, error: null,
    } as any)

    render(<UsersPage />)

    expect(screen.getByText('ADMIN')).toBeInTheDocument()
  })

  it('renders role badge with EDITOR text for editor users', () => {
    mockUseQuery.mockReturnValue({
      data: [makeUser({ role: 'editor' })], isLoading: false, error: null,
    } as any)

    render(<UsersPage />)

    expect(screen.getByText('EDITOR')).toBeInTheDocument()
  })

  it('renders role badge with VIEWER text for viewer users', () => {
    mockUseQuery.mockReturnValue({
      data: [makeUser({ role: 'viewer' })], isLoading: false, error: null,
    } as any)

    render(<UsersPage />)

    expect(screen.getByText('VIEWER')).toBeInTheDocument()
  })

  it('shows active status indicator for active users', () => {
    mockUseQuery.mockReturnValue({
      data: [makeUser({ is_active: true })], isLoading: false, error: null,
    } as any)

    render(<UsersPage />)

    expect(screen.getByText('● active')).toBeInTheDocument()
  })

  it('shows inactive status indicator for inactive users', () => {
    mockUseQuery.mockReturnValue({
      data: [makeUser({ is_active: false })], isLoading: false, error: null,
    } as any)

    render(<UsersPage />)

    expect(screen.getByText('○ inactive')).toBeInTheDocument()
  })

  it('renders multiple users', () => {
    const users = [
      makeUser({ user_id: 'u1', username: 'alice', role: 'admin' }),
      makeUser({ user_id: 'u2', username: 'bob',   role: 'editor' }),
      makeUser({ user_id: 'u3', username: 'carol', role: 'viewer' }),
    ]
    mockUseQuery.mockReturnValue({ data: users, isLoading: false, error: null } as any)

    render(<UsersPage />)

    expect(screen.getByText('alice')).toBeInTheDocument()
    expect(screen.getByText('bob')).toBeInTheDocument()
    expect(screen.getByText('carol')).toBeInTheDocument()
  })

  it('renders — for missing display_name', () => {
    mockUseQuery.mockReturnValue({
      data: [makeUser({ display_name: '' })], isLoading: false, error: null,
    } as any)

    render(<UsersPage />)

    expect(screen.getByText('—')).toBeInTheDocument()
  })
})

// ─── Delete confirmation flow ─────────────────────────────────────────────────

describe('UsersPage — delete confirmation', () => {
  beforeEach(() => {
    // canDelete = isAdmin && user_id !== currentUser.sub
    // ADMIN_USER.sub = 'u1', so user 'u2' can be deleted by admin
    mockIsSelfOrRole.mockReturnValue(false)
    mockUseQuery.mockReturnValue({
      data: [makeUser({ user_id: 'u2', username: 'deletable' })],
      isLoading: false,
      error: null,
    } as any)
  })

  it('shows delete icon button for deletable users', () => {
    render(<UsersPage />)

    expect(screen.getByTitle('Delete user')).toBeInTheDocument()
  })

  it('clicking delete reveals confirm / cancel buttons', () => {
    render(<UsersPage />)

    fireEvent.click(screen.getByTitle('Delete user'))

    expect(screen.getByTitle('Confirm delete')).toBeInTheDocument()
    expect(screen.getByTitle('Cancel')).toBeInTheDocument()
  })

  it('clicking Cancel hides confirm buttons', () => {
    render(<UsersPage />)

    fireEvent.click(screen.getByTitle('Delete user'))
    fireEvent.click(screen.getByTitle('Cancel'))

    expect(screen.queryByTitle('Confirm delete')).not.toBeInTheDocument()
    expect(screen.getByTitle('Delete user')).toBeInTheDocument()
  })

  it('clicking Confirm calls deleteMutation.mutate with user_id', () => {
    const mutateFn = vi.fn()
    mockUseMutation.mockReturnValue({ mutate: mutateFn, isPending: false } as any)

    render(<UsersPage />)

    fireEvent.click(screen.getByTitle('Delete user'))
    fireEvent.click(screen.getByTitle('Confirm delete'))

    expect(mutateFn).toHaveBeenCalledWith('u2')
  })

  it('does not show delete button for own account (prevent self-delete)', () => {
    // user_id matches currentUser.sub (ADMIN_USER.sub = 'u1')
    mockUseQuery.mockReturnValue({
      data: [makeUser({ user_id: 'u1', username: 'self' })],
      isLoading: false, error: null,
    } as any)

    render(<UsersPage />)

    expect(screen.queryByTitle('Delete user')).not.toBeInTheDocument()
  })
})

// ─── Create user modal ────────────────────────────────────────────────────────

describe('UsersPage — create user modal', () => {
  it('opens modal on clicking Create User', () => {
    render(<UsersPage />)

    fireEvent.click(screen.getByRole('button', { name: /create user/i }))

    // After the modal opens there are multiple "Create User" occurrences
    // (header button + modal heading + form submit). Check the heading specifically.
    expect(screen.getByRole('heading', { name: 'Create User' })).toBeInTheDocument()
    // Labels use uppercase text without htmlFor — verify inputs exist by role
    expect(screen.getAllByRole('textbox').length).toBeGreaterThanOrEqual(1)
  })

  it('modal closes on clicking Cancel', () => {
    render(<UsersPage />)

    fireEvent.click(screen.getByRole('button', { name: /create user/i }))

    // Click the Cancel button inside the modal
    const cancelBtn = screen.getByRole('button', { name: 'Cancel' })
    fireEvent.click(cancelBtn)

    // Modal heading disappears when closed
    expect(screen.queryByRole('heading', { name: 'Create User' })).not.toBeInTheDocument()
  })

  it('shows validation error when required fields are empty', () => {
    render(<UsersPage />)

    fireEvent.click(screen.getByRole('button', { name: /create user/i }))

    // After modal opens there are multiple "Create User" buttons — click the last (form submit)
    const submitBtns = screen.getAllByRole('button', { name: /create user/i })
    fireEvent.click(submitBtns[submitBtns.length - 1])

    expect(screen.getByText(/username, email, and password are required/i)).toBeInTheDocument()
  })
})
