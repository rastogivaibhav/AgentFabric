import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import PoliciesPage from './PoliciesPage'

vi.mock('../hooks/auth', () => ({
  useAuth: vi.fn(),
  hasRole: vi.fn(),
}))

vi.mock('../hooks/api', () => ({
  usePolicyRules: vi.fn(),
  useControlAudit: vi.fn(),
  useUpsertPolicyRule: vi.fn(),
  useDeletePolicyRule: vi.fn(),
  usePreviewPolicyRule: vi.fn(),
  useRollouts: vi.fn(),
  useUpsertRolloutRule: vi.fn(),
  usePreviewRollout: vi.fn(),
  useUpdateRolloutStatus: vi.fn(),
}))

vi.mock('./PolicyDecisionExplorer', () => ({
  default: ({ label, decision }: any) => <div>{label}:{decision.action ?? 'none'}</div>,
}))

vi.mock('./PolicySimulationPage', () => ({
  default: () => <div>Policy Simulation Stub</div>,
}))

import { hasRole, useAuth } from '../hooks/auth'
import {
  useControlAudit,
  useDeletePolicyRule,
  usePolicyRules,
  usePreviewRollout,
  usePreviewPolicyRule,
  useRollouts,
  useUpdateRolloutStatus,
  useUpsertRolloutRule,
  useUpsertPolicyRule,
} from '../hooks/api'

const mockUseAuth = vi.mocked(useAuth)
const mockHasRole = vi.mocked(hasRole)
const mockUsePolicyRules = vi.mocked(usePolicyRules)
const mockUseControlAudit = vi.mocked(useControlAudit)
const mockUseUpsertPolicyRule = vi.mocked(useUpsertPolicyRule)
const mockUseDeletePolicyRule = vi.mocked(useDeletePolicyRule)
const mockUsePreviewPolicyRule = vi.mocked(usePreviewPolicyRule)
const mockUseRollouts = vi.mocked(useRollouts)
const mockUseUpsertRolloutRule = vi.mocked(useUpsertRolloutRule)
const mockUsePreviewRollout = vi.mocked(usePreviewRollout)
const mockUseUpdateRolloutStatus = vi.mocked(useUpdateRolloutStatus)

describe('PoliciesPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseAuth.mockReturnValue({ user: { role: 'admin' } } as any)
    mockHasRole.mockReturnValue(true)
    mockUsePolicyRules.mockReturnValue({ data: { items: [], count: 0 }, isLoading: false, error: null } as any)
    mockUseControlAudit.mockReturnValue({ data: { items: [], count: 0 } } as any)
    mockUseUpsertPolicyRule.mockReturnValue({ mutate: vi.fn(), isPending: false, isError: false } as any)
    mockUseDeletePolicyRule.mockReturnValue({ mutate: vi.fn(), isPending: false } as any)
    mockUsePreviewPolicyRule.mockReturnValue({ mutate: vi.fn(), isPending: false, isError: false } as any)
    mockUseRollouts.mockReturnValue({ data: { items: [], count: 0 }, isLoading: false, error: null } as any)
    mockUseUpsertRolloutRule.mockReturnValue({ mutate: vi.fn(), isPending: false, isError: false } as any)
    mockUsePreviewRollout.mockReturnValue({ mutate: vi.fn(), isPending: false, isError: false, data: null } as any)
    mockUseUpdateRolloutStatus.mockReturnValue({ mutate: vi.fn(), isPending: false } as any)
  })

  it('shows restricted message for non-admin users', () => {
    mockHasRole.mockReturnValue(false)
    render(<PoliciesPage />)
    expect(screen.getByText(/restricted to administrators/i)).toBeInTheDocument()
  })

  it('creates a policy rule and clears form on success', () => {
    const mutate = vi.fn((payload, opts) => opts?.onSuccess?.())
    mockUseUpsertPolicyRule.mockReturnValue({ mutate, isPending: false, isError: false } as any)

    render(<PoliciesPage />)
    fireEvent.change(screen.getByRole('textbox', { name: 'Name' }), { target: { value: 'Prod Guard' } })
    fireEvent.change(screen.getAllByRole('textbox', { name: 'Provider' })[0], { target: { value: ' OPENAI ' } })
    fireEvent.change(screen.getByRole('textbox', { name: 'Model Pattern' }), { target: { value: ' GPT-4O ' } })
    fireEvent.change(screen.getAllByRole('textbox', { name: 'Environment' })[0], { target: { value: ' Production ' } })
    fireEvent.click(screen.getByRole('button', { name: /create rule/i }))

    expect(mutate).toHaveBeenCalledWith(expect.objectContaining({
      name: 'Prod Guard',
      provider: 'openai',
      model_pattern: 'gpt-4o',
      environment: 'production',
      decision_mode: 'fast',
    }), expect.any(Object))

    expect(screen.getByRole('textbox', { name: 'Name' })).toHaveValue('')
  })

  it('renders rules, preview decisions, and audit entries', () => {
    const previewMutate = vi.fn()
    const updateRolloutStatusMutate = vi.fn()
    mockUsePolicyRules.mockReturnValue({
      data: {
        items: [{
          id: 7,
          name: 'Block prod',
          rule_type: 'traffic',
          action: 'deny',
          provider: 'openai',
          model_pattern: 'gpt-4o',
          environment: 'production',
          enabled: true,
          priority: 10,
          decision_mode: 'rego',
          guardrails: ['schema'],
          rollout_percent: 50,
          version: 2,
        }],
        count: 1,
      },
      isLoading: false,
      error: null,
    } as any)
    mockUseControlAudit.mockReturnValue({
      data: {
        items: [{
          id: 1,
          category: 'policy',
          action: 'upsert',
          actor: 'architect',
          target_type: 'policy',
          target_id: '7',
          outcome: 'success',
          created_at: '2026-01-01T00:00:00Z',
        }],
      },
    } as any)
    mockUsePreviewPolicyRule.mockReturnValue({
      mutate: previewMutate,
      isPending: false,
      isError: false,
      data: {
        traffic: { matched: true, action: 'deny' },
        request_dlp: { matched: false },
        response_dlp: { matched: true, action: 'redact' },
      },
    } as any)
    mockUseRollouts.mockReturnValue({
      data: {
        items: [{
          id: 11,
          name: 'Policy canary',
          target_type: 'policy_rule',
          target_id: '7',
          policy_rule_id: 7,
          environment: 'production',
          percentage: 10,
          status: 'active',
          recent_requests: 20,
          recent_error_rate: 0.05,
        }],
        count: 1,
      },
      isLoading: false,
      error: null,
    } as any)
    mockUseUpdateRolloutStatus.mockReturnValue({ mutate: updateRolloutStatusMutate, isPending: false } as any)
    mockUsePreviewRollout.mockReturnValue({
      mutate: vi.fn(),
      isPending: false,
      isError: false,
      data: {
        assignment: {
          rule_id: 11,
          rule_name: 'Policy canary',
          variant: 'control',
          bucket: 42,
        },
      },
    } as any)

    render(<PoliciesPage />)
    expect(screen.getAllByText('Block prod')[0]).toBeInTheDocument()
    expect(screen.getByText(/policy simulation stub/i)).toBeInTheDocument()
    expect(screen.getByText(/architect/i)).toBeInTheDocument()
    expect(screen.getByText(/Preview selected control for rule Policy canary/i)).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /preview policy match/i }))
    expect(previewMutate).toHaveBeenCalled()
    expect(screen.getByText('Traffic:deny')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Pause' }))
    expect(updateRolloutStatusMutate).toHaveBeenCalledWith({ id: 11, status: 'paused' })
  })
})
