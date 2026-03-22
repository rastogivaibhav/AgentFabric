import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import React from 'react'

vi.mock('react-router-dom', () => ({
  Link: ({ to, children }: { to: string; children: React.ReactNode }) => <a href={to}>{children}</a>,
}))

vi.mock('@tanstack/react-query', () => ({
  useQuery: vi.fn(),
}))

vi.mock('../hooks/auth', () => ({
  useAuth: vi.fn(),
  hasRole: vi.fn(),
}))

import AuditPage from './AuditPage'
import { useQuery } from '@tanstack/react-query'
import { hasRole, useAuth } from '../hooks/auth'

const mockUseQuery = vi.mocked(useQuery)
const mockUseAuth = vi.mocked(useAuth)
const mockHasRole = vi.mocked(hasRole)

const ADMIN_USER = { sub: 'u1', email: 'admin@af.io', name: 'Admin', role: 'admin' }
const VIEWER_USER = { sub: 'u2', email: 'viewer@af.io', name: 'Viewer', role: 'viewer' }

function makeEntry(overrides: Partial<{
  id: number
  decision_id: string
  trace_id: string
  span_id: string
  policy_name: string
  result: string
  reason: string
  tenant_id: string
  evaluated_at: string
  previous_hash: string
  entry_hash: string
}> = {}) {
  return {
    id: overrides.id ?? 1,
    decision_id: overrides.decision_id ?? 'dec-0001',
    trace_id: overrides.trace_id ?? 'trace-abc123',
    span_id: overrides.span_id ?? 'span-def456',
    policy_name: overrides.policy_name ?? 'pii-block',
    result: overrides.result ?? 'deny',
    reason: overrides.reason ?? 'PII detected in prompt',
    tenant_id: overrides.tenant_id ?? 'default',
    evaluated_at: overrides.evaluated_at ?? '2026-03-19T10:00:00.000Z',
    previous_hash: overrides.previous_hash ?? 'abc123',
    entry_hash: overrides.entry_hash ?? 'def456deadbeef00',
  }
}

function adminQuery(entries: ReturnType<typeof makeEntry>[], overrides = {}) {
  return {
    data: {
      items: entries,
      count: entries.length,
      limit: 100,
      offset: 0,
    },
    isLoading: false,
    isError: false,
    isFetching: false,
    refetch: vi.fn(),
    ...overrides,
  } as never
}

beforeEach(() => {
  vi.clearAllMocks()
  mockUseAuth.mockReturnValue({ user: ADMIN_USER } as never)
  mockHasRole.mockImplementation((user, roles) => !!user?.role && (roles as string[]).includes(user.role))
  mockUseQuery.mockReturnValue(adminQuery([]))
})

describe('AuditPage access control', () => {
  it('shows access denied for non-admin users', () => {
    mockUseAuth.mockReturnValue({ user: VIEWER_USER } as never)
    mockHasRole.mockReturnValue(false)
    render(<AuditPage />)
    expect(screen.getByText('Access denied')).toBeInTheDocument()
    expect(screen.getByText(/restricted to administrators/i)).toBeInTheDocument()
  })

  it('shows the audit page for admin users', () => {
    render(<AuditPage />)
    expect(screen.getByText('Audit Log')).toBeInTheDocument()
    expect(screen.getByText(/immutable, hash-chained/i)).toBeInTheDocument()
  })
})

describe('AuditPage loading and error states', () => {
  it('renders loading skeleton rows', () => {
    mockUseQuery.mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
      isFetching: false,
      refetch: vi.fn(),
    } as never)
    render(<AuditPage />)
    expect(document.querySelectorAll('.animate-pulse').length).toBeGreaterThanOrEqual(1)
  })

  it('shows an error message when the query fails', () => {
    mockUseQuery.mockReturnValue({
      data: { items: [], count: 0, limit: 100, offset: 0 },
      isLoading: false,
      isError: true,
      isFetching: false,
      refetch: vi.fn(),
    } as never)
    render(<AuditPage />)
    expect(screen.getByText(/failed to load audit log/i)).toBeInTheDocument()
  })
})

describe('AuditPage data rendering', () => {
  it('shows the empty state when no entries exist', () => {
    render(<AuditPage />)
    expect(screen.getByText(/no audit entries found/i)).toBeInTheDocument()
  })

  it('renders policy name, reason, and truncated hash', () => {
    mockUseQuery.mockReturnValue(adminQuery([makeEntry({
      policy_name: 'pii-block',
      reason: 'PII detected: email address in output',
      entry_hash: 'deadbeef01234567',
    })]))
    render(<AuditPage />)
    expect(screen.getByText('pii-block')).toBeInTheDocument()
    expect(screen.getByText('PII detected: email address in output')).toBeInTheDocument()
    expect(screen.getByText('deadbeef0123')).toBeInTheDocument()
  })

  it('renders trace links and result badges', () => {
    mockUseQuery.mockReturnValue(adminQuery([
      makeEntry({ id: 1, result: 'allow', trace_id: 'trace-abc123456789' }),
      makeEntry({ id: 2, result: 'sanitize' }),
    ]))
    render(<AuditPage />)
    expect(screen.getByText('allow')).toBeInTheDocument()
    expect(screen.getByText('sanitize')).toBeInTheDocument()
    const link = screen.getAllByRole('link').find(item => item.getAttribute('href') === '/traces/trace-abc123456789')
    expect(link).toHaveAttribute('href', '/traces/trace-abc123456789')
  })
})

describe('AuditPage pagination and refresh', () => {
  it('disables previous button on the first page', () => {
    render(<AuditPage />)
    expect(screen.getByRole('button', { name: /previous/i })).toBeDisabled()
  })

  it('disables next button when fewer than 100 entries are returned', () => {
    mockUseQuery.mockReturnValue(adminQuery([makeEntry()]))
    render(<AuditPage />)
    expect(screen.getByRole('button', { name: /next/i })).toBeDisabled()
  })

  it('shows row range text', () => {
    mockUseQuery.mockReturnValue(adminQuery([makeEntry()]))
    render(<AuditPage />)
    expect(screen.getByText(/showing rows 1-1/i)).toBeInTheDocument()
  })

  it('calls refetch when refresh is clicked', () => {
    const refetch = vi.fn()
    const queryState = adminQuery([]) as any
    queryState.refetch = refetch
    mockUseQuery.mockReturnValue(queryState)
    render(<AuditPage />)
    fireEvent.click(screen.getByRole('button', { name: /refresh/i }))
    expect(refetch).toHaveBeenCalledOnce()
  })
})

describe('AuditPage chain integrity note', () => {
  it('renders the audit verify API note', () => {
    render(<AuditPage />)
    expect(screen.getByText(/GET \/api\/v1\/audit\/verify/i)).toBeInTheDocument()
  })
})
