import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import React from 'react'

vi.mock('react-router-dom', () => ({
  Link: ({ to, children }: { to: string; children: React.ReactNode }) => <a href={to}>{children}</a>,
}))

vi.mock('../hooks/auth', () => ({
  useAuth: vi.fn(),
  hasRole: vi.fn(),
}))

vi.mock('../hooks/api', () => ({
  useControlAudit: vi.fn(),
  useControlHistory: vi.fn(),
  useEvidenceBundles: vi.fn(),
  useCreateEvidenceBundle: vi.fn(),
}))

import AuditPage from './AuditPage'
import { hasRole, useAuth } from '../hooks/auth'
import { useControlAudit, useControlHistory, useCreateEvidenceBundle, useEvidenceBundles } from '../hooks/api'

const mockUseAuth = vi.mocked(useAuth)
const mockHasRole = vi.mocked(hasRole)
const mockUseControlAudit = vi.mocked(useControlAudit)
const mockUseControlHistory = vi.mocked(useControlHistory)
const mockUseEvidenceBundles = vi.mocked(useEvidenceBundles)
const mockUseCreateEvidenceBundle = vi.mocked(useCreateEvidenceBundle)

const ADMIN_USER = { sub: 'u1', email: 'admin@af.io', name: 'Admin', role: 'admin' }
const VIEWER_USER = { sub: 'u2', email: 'viewer@af.io', name: 'Viewer', role: 'viewer' }

function makeAuditEntry(overrides: Partial<{
  id: number
  trace_id: string
  result: string
  policy_name: string
  reason: string
  entry_hash: string
}> = {}) {
  return {
    id: overrides.id ?? 1,
    decision_id: 'dec-1',
    trace_id: overrides.trace_id ?? 'trace-abc123',
    span_id: 'span-1',
    policy_name: overrides.policy_name ?? 'pii-block',
    result: overrides.result ?? 'deny',
    reason: overrides.reason ?? 'PII detected in prompt',
    tenant_id: 'default',
    evaluated_at: '2026-03-25T10:00:00.000Z',
    previous_hash: 'abc123',
    entry_hash: overrides.entry_hash ?? 'def456deadbeef00',
  }
}

function makeHistoryEntry(overrides: Partial<{
  id: number
  category: string
  action: string
  target_type: string
  target_id: string
  reason: string
}> = {}) {
  return {
    id: overrides.id ?? 9,
    tenant_id: 'default',
    category: overrides.category ?? 'rollouts',
    action: overrides.action ?? 'status_update',
    target_type: overrides.target_type ?? 'rollout_rule',
    target_id: overrides.target_id ?? '17',
    actor: 'Admin',
    reason: overrides.reason ?? 'paused after error-rate breach',
    outcome: 'success',
    evidence_refs: [],
    previous_hash: 'prev',
    entry_hash: 'hash',
    created_at: '2026-03-25T11:00:00.000Z',
  }
}

function makeBundle(overrides: Partial<{
  id: number
  name: string
  scope: string
  item_count: number
  summary: string[]
}> = {}) {
  return {
    id: overrides.id ?? 4,
    tenant_id: 'default',
    name: overrides.name ?? 'Rollout incident bundle',
    scope: overrides.scope ?? 'incident',
    status: 'ready',
    created_by: 'Admin',
    created_at: '2026-03-25T12:00:00.000Z',
    item_count: overrides.item_count ?? 5,
    summary: overrides.summary ?? ['2 rollout events show assignment and auto-pause behavior.'],
  }
}

function queryResult<T>(data: T, overrides: Partial<{
  isLoading: boolean
  isError: boolean
  isFetching: boolean
}> = {}) {
  return {
    data,
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
  mockUseControlAudit.mockReturnValue(queryResult({ items: [], count: 0, limit: 100 }))
  mockUseControlHistory.mockReturnValue(queryResult({ items: [], total: 0, has_more: false }))
  mockUseEvidenceBundles.mockReturnValue(queryResult({ items: [], count: 0, limit: 12 }))
  mockUseCreateEvidenceBundle.mockReturnValue({
    mutateAsync: vi.fn(),
    isPending: false,
    isError: false,
    isSuccess: false,
    data: undefined,
  } as never)
})

describe('AuditPage access control', () => {
  it('shows access denied for non-admin users', () => {
    mockUseAuth.mockReturnValue({ user: VIEWER_USER } as never)
    mockHasRole.mockReturnValue(false)
    render(<AuditPage />)
    expect(screen.getByText('Access denied')).toBeInTheDocument()
    expect(screen.getByText(/restricted to administrators/i)).toBeInTheDocument()
  })

  it('shows the enterprise memory audit page for admins', () => {
    render(<AuditPage />)
    expect(screen.getByText('Audit Log')).toBeInTheDocument()
    expect(screen.getByText(/append-only control history/i)).toBeInTheDocument()
    expect(screen.getByText('Evidence Bundle Export')).toBeInTheDocument()
  })
})

describe('AuditPage data states', () => {
  it('renders loading placeholders', () => {
    mockUseControlAudit.mockReturnValue(queryResult(undefined, { isLoading: true }) as never)
    mockUseControlHistory.mockReturnValue(queryResult(undefined, { isLoading: true }) as never)
    mockUseEvidenceBundles.mockReturnValue(queryResult(undefined, { isLoading: true }) as never)
    render(<AuditPage />)
    expect(document.querySelectorAll('.animate-pulse').length).toBeGreaterThanOrEqual(1)
  })

  it('shows an error message when one of the enterprise memory queries fails', () => {
    mockUseControlHistory.mockReturnValue(queryResult({ items: [], total: 0, has_more: false }, { isError: true }))
    render(<AuditPage />)
    expect(screen.getByText(/failed to load one or more enterprise memory views/i)).toBeInTheDocument()
  })

  it('shows empty states for history and bundles', () => {
    render(<AuditPage />)
    expect(screen.getByText(/no control history has been recorded/i)).toBeInTheDocument()
    expect(screen.getByText(/no evidence bundles have been generated yet/i)).toBeInTheDocument()
  })
})

describe('AuditPage renders enterprise memory data', () => {
  it('renders policy audit entries, history timeline, and bundle summaries', () => {
    mockUseControlAudit.mockReturnValue(queryResult({ items: [makeAuditEntry()], count: 1, limit: 100 }))
    mockUseControlHistory.mockReturnValue(queryResult({ items: [makeHistoryEntry()], total: 1, has_more: false }))
    mockUseEvidenceBundles.mockReturnValue(queryResult({ items: [makeBundle()], count: 1, limit: 12 }))
    render(<AuditPage />)

    expect(screen.getByText('pii-block')).toBeInTheDocument()
    expect(screen.getByText(/paused after error-rate breach/i)).toBeInTheDocument()
    expect(screen.getByText('Rollout incident bundle')).toBeInTheDocument()
    expect(screen.getByText(/auto-pause behavior/i)).toBeInTheDocument()
  })

  it('renders trace links and export links', () => {
    mockUseControlAudit.mockReturnValue(queryResult({ items: [makeAuditEntry({ trace_id: 'trace-xyz123456789' })], count: 1, limit: 100 }))
    mockUseEvidenceBundles.mockReturnValue(queryResult({ items: [makeBundle({ id: 12 })], count: 1, limit: 12 }))
    render(<AuditPage />)

    const traceLink = screen.getAllByRole('link').find(item => item.getAttribute('href') === '/traces/trace-xyz123456789')
    expect(traceLink).toHaveAttribute('href', '/traces/trace-xyz123456789')
    const exportLink = screen.getAllByRole('link').find(item => item.getAttribute('href')?.includes('/api/v1/audit/evidence-bundles/12/export'))
    expect(exportLink).toBeDefined()
  })
})

describe('AuditPage bundle actions', () => {
  it('submits a create bundle request from the export form', async () => {
    const mutateAsync = vi.fn().mockResolvedValue({ id: 99 })
    mockUseCreateEvidenceBundle.mockReturnValue({
      mutateAsync,
      isPending: false,
      isError: false,
      isSuccess: false,
      data: undefined,
    } as never)
    render(<AuditPage />)

    fireEvent.change(screen.getByDisplayValue('Rollout incident bundle'), { target: { value: 'Bundle A' } })
    fireEvent.change(screen.getByPlaceholderText('candidate-v7'), { target: { value: 'candidate-v7' } })
    fireEvent.click(screen.getByRole('button', { name: /create evidence bundle/i }))

    expect(mutateAsync).toHaveBeenCalledWith(expect.objectContaining({
      name: 'Bundle A',
      release_tag: 'candidate-v7',
    }))
  })

  it('shows download latest bundle when creation succeeds', () => {
    mockUseCreateEvidenceBundle.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: false,
      isError: false,
      isSuccess: true,
      data: { id: 55 },
    } as never)
    render(<AuditPage />)
    expect(screen.getByText(/download latest bundle/i)).toBeInTheDocument()
  })
})
