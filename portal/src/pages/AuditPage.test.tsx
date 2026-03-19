import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import React from 'react'

// ─── Mock react-router-dom (Link) ─────────────────────────────────────────────
vi.mock('react-router-dom', () => ({
  Link: ({ to, children }: { to: string; children: React.ReactNode }) => (
    <a href={to}>{children}</a>
  ),
}))

// ─── Mock @tanstack/react-query ───────────────────────────────────────────────
vi.mock('@tanstack/react-query', () => ({
  useQuery: vi.fn(),
}))

// ─── Mock ../hooks/auth ───────────────────────────────────────────────────────
vi.mock('../hooks/auth', () => ({
  useAuth: vi.fn(),
  hasRole: vi.fn(),
}))

import AuditPage from './AuditPage'
import { useQuery } from '@tanstack/react-query'
import { useAuth, hasRole } from '../hooks/auth'

const mockUseQuery = vi.mocked(useQuery)
const mockUseAuth  = vi.mocked(useAuth)
const mockHasRole  = vi.mocked(hasRole)

// ─── Fixtures ─────────────────────────────────────────────────────────────────

const ADMIN_USER = { sub: 'u1', email: 'admin@af.io', name: 'Admin', role: 'admin' }
const VIEWER_USER = { sub: 'u2', email: 'viewer@af.io', name: 'Viewer', role: 'viewer' }

function makeEntry(overrides: Partial<{
  id: number; decision_id: string; trace_id: string; span_id: string;
  policy_name: string; result: string; reason: string; tenant_id: string;
  evaluated_at: string; previous_hash: string; entry_hash: string;
}> = {}) {
  return {
    id:            overrides.id            ?? 1,
    decision_id:   overrides.decision_id   ?? 'dec-0001',
    trace_id:      overrides.trace_id      ?? 'trace-abc123',
    span_id:       overrides.span_id       ?? 'span-def456',
    policy_name:   overrides.policy_name   ?? 'pii-block',
    result:        overrides.result        ?? 'deny',
    reason:        overrides.reason        ?? 'PII detected in prompt',
    tenant_id:     overrides.tenant_id     ?? 'default',
    evaluated_at:  overrides.evaluated_at  ?? '2026-03-19T10:00:00.000Z',
    previous_hash: overrides.previous_hash ?? 'abc123',
    entry_hash:    overrides.entry_hash    ?? 'def456deadbeef00',
  }
}

function adminQuery(entries: ReturnType<typeof makeEntry>[], overrides = {}) {
  return {
    data: entries,
    isLoading:  false,
    isError:    false,
    isFetching: false,
    refetch:    vi.fn(),
    ...overrides,
  } as any
}

// ─── Test setup ───────────────────────────────────────────────────────────────

beforeEach(() => {
  vi.clearAllMocks()
  // Default: admin user, query idle
  mockUseAuth.mockReturnValue({ user: ADMIN_USER } as any)
  mockHasRole.mockImplementation((user, roles) =>
    user?.role != null && (roles as string[]).includes(user.role)
  )
  mockUseQuery.mockReturnValue(adminQuery([]))
})

// ─── Access control ───────────────────────────────────────────────────────────

describe('AuditPage — access control', () => {
  it('shows AccessDenied for non-admin users', () => {
    mockUseAuth.mockReturnValue({ user: VIEWER_USER } as any)
    mockHasRole.mockReturnValue(false)

    render(<AuditPage />)

    expect(screen.getByText('Access denied')).toBeInTheDocument()
    expect(screen.getByText(/restricted to administrators/i)).toBeInTheDocument()
  })

  it('does not render the table for non-admin users', () => {
    mockUseAuth.mockReturnValue({ user: VIEWER_USER } as any)
    mockHasRole.mockReturnValue(false)

    render(<AuditPage />)

    expect(screen.queryByText('Audit Log')).not.toBeInTheDocument()
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
  })

  it('shows the audit log page for admin users', () => {
    render(<AuditPage />)

    expect(screen.getByText('Audit Log')).toBeInTheDocument()
    expect(screen.getByText(/immutable, hash-chained/i)).toBeInTheDocument()
  })

  it('shows AccessDenied when user is null (unauthenticated)', () => {
    mockUseAuth.mockReturnValue({ user: null } as any)
    mockHasRole.mockReturnValue(false)

    render(<AuditPage />)

    expect(screen.getByText('Access denied')).toBeInTheDocument()
  })
})

// ─── Loading state ────────────────────────────────────────────────────────────

describe('AuditPage — loading state', () => {
  it('renders skeleton rows while loading', () => {
    mockUseQuery.mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
      isFetching: false,
      refetch: vi.fn(),
    } as any)

    render(<AuditPage />)

    // 10 skeleton rows × 8 cells = 80 skeleton divs
    const skeletons = document.querySelectorAll('.animate-pulse')
    expect(skeletons.length).toBeGreaterThanOrEqual(1)
  })

  it('disables Refresh button while fetching', () => {
    mockUseQuery.mockReturnValue({
      ...adminQuery([]),
      isFetching: true,
    } as any)

    render(<AuditPage />)

    const refreshBtn = screen.getByRole('button', { name: /refresh/i })
    expect(refreshBtn).toBeDisabled()
  })

  it('Refresh button is enabled when not fetching', () => {
    render(<AuditPage />)

    const refreshBtn = screen.getByRole('button', { name: /refresh/i })
    expect(refreshBtn).not.toBeDisabled()
  })
})

// ─── Error state ──────────────────────────────────────────────────────────────

describe('AuditPage — error state', () => {
  it('shows error message when query fails', () => {
    mockUseQuery.mockReturnValue({
      data: [],
      isLoading: false,
      isError: true,
      isFetching: false,
      refetch: vi.fn(),
    } as any)

    render(<AuditPage />)

    expect(screen.getByText(/failed to load audit log/i)).toBeInTheDocument()
  })
})

// ─── Empty state ──────────────────────────────────────────────────────────────

describe('AuditPage — empty state', () => {
  it('shows empty message when no entries exist', () => {
    render(<AuditPage />)

    expect(screen.getByText(/no audit entries found/i)).toBeInTheDocument()
  })
})

// ─── Data rendering ───────────────────────────────────────────────────────────

describe('AuditPage — data rendering', () => {
  it('renders policy name', () => {
    mockUseQuery.mockReturnValue(adminQuery([makeEntry({ policy_name: 'pii-block' })]))

    render(<AuditPage />)

    expect(screen.getByText('pii-block')).toBeInTheDocument()
  })

  it('renders result badges for all result types', () => {
    const entries = [
      makeEntry({ id: 1, result: 'allow' }),
      makeEntry({ id: 2, result: 'deny' }),
      makeEntry({ id: 3, result: 'warn' }),
      makeEntry({ id: 4, result: 'sanitize' }),
    ]
    mockUseQuery.mockReturnValue(adminQuery(entries))

    render(<AuditPage />)

    expect(screen.getByText('allow')).toBeInTheDocument()
    expect(screen.getByText('deny')).toBeInTheDocument()
    expect(screen.getByText('warn')).toBeInTheDocument()
    expect(screen.getByText('sanitize')).toBeInTheDocument()
  })

  it('renders reason text', () => {
    mockUseQuery.mockReturnValue(adminQuery([
      makeEntry({ reason: 'PII detected: email address in output' }),
    ]))

    render(<AuditPage />)

    expect(screen.getByText('PII detected: email address in output')).toBeInTheDocument()
  })

  it('renders truncated entry hash in HashCell', () => {
    mockUseQuery.mockReturnValue(adminQuery([
      makeEntry({ entry_hash: 'deadbeef01234567' }),
    ]))

    render(<AuditPage />)

    // HashCell shows first 12 characters
    expect(screen.getByText('deadbeef0123')).toBeInTheDocument()
  })

  it('renders trace_id as a link to TraceDetail', () => {
    mockUseQuery.mockReturnValue(adminQuery([
      makeEntry({ trace_id: 'trace-abc123456789' }),
    ]))

    render(<AuditPage />)

    // truncate(trace_id, 12) → first 12 chars + '…'  =  'trace-abc123…'
    const link = screen.getByRole('link', { name: /trace-abc123/i })
    expect(link).toHaveAttribute('href', '/traces/trace-abc123456789')
  })

  it('renders row ID', () => {
    mockUseQuery.mockReturnValue(adminQuery([makeEntry({ id: 42 })]))

    render(<AuditPage />)

    expect(screen.getByText('42')).toBeInTheDocument()
  })

  it('renders multiple entries', () => {
    const entries = Array.from({ length: 5 }, (_, i) =>
      makeEntry({ id: i + 1, policy_name: `policy-${i + 1}` })
    )
    mockUseQuery.mockReturnValue(adminQuery(entries))

    render(<AuditPage />)

    for (let i = 1; i <= 5; i++) {
      expect(screen.getByText(`policy-${i}`)).toBeInTheDocument()
    }
  })
})

// ─── Pagination ───────────────────────────────────────────────────────────────

describe('AuditPage — pagination', () => {
  it('Previous button is disabled on the first page (offset=0)', () => {
    render(<AuditPage />)

    const prevBtn = screen.getByRole('button', { name: /previous/i })
    expect(prevBtn).toBeDisabled()
  })

  it('Next button is disabled when fewer than 100 entries returned', () => {
    mockUseQuery.mockReturnValue(adminQuery([makeEntry()]))

    render(<AuditPage />)

    const nextBtn = screen.getByRole('button', { name: /next/i })
    expect(nextBtn).toBeDisabled()
  })

  it('shows row range display', () => {
    mockUseQuery.mockReturnValue(adminQuery([makeEntry()]))

    render(<AuditPage />)

    expect(screen.getByText(/showing rows 1–1/i)).toBeInTheDocument()
  })

  it('calls refetch when Refresh is clicked', () => {
    const refetch = vi.fn()
    mockUseQuery.mockReturnValue({ ...adminQuery([]), refetch } as any)

    render(<AuditPage />)

    fireEvent.click(screen.getByRole('button', { name: /refresh/i }))
    expect(refetch).toHaveBeenCalledOnce()
  })
})

// ─── Chain integrity footer ───────────────────────────────────────────────────

describe('AuditPage — chain integrity', () => {
  it('renders the audit verify API note', () => {
    render(<AuditPage />)

    expect(screen.getByText(/GET \/api\/v1\/audit\/verify/i)).toBeInTheDocument()
  })
})
