import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import PricingRulesPage from './PricingRulesPage'

vi.mock('../hooks/auth', () => ({
  useAuth: vi.fn(),
  hasRole: vi.fn(),
}))

vi.mock('../hooks/api', () => ({
  usePricingRules: vi.fn(),
  useUpsertPricingRule: vi.fn(),
  useDeletePricingRule: vi.fn(),
  usePreviewPricingRule: vi.fn(),
  usePricingRuleAudit: vi.fn(),
}))

import { hasRole, useAuth } from '../hooks/auth'
import {
  useDeletePricingRule,
  usePreviewPricingRule,
  usePricingRuleAudit,
  usePricingRules,
  useUpsertPricingRule,
} from '../hooks/api'

const mockUseAuth = vi.mocked(useAuth)
const mockHasRole = vi.mocked(hasRole)
const mockUsePricingRules = vi.mocked(usePricingRules)
const mockUseUpsertPricingRule = vi.mocked(useUpsertPricingRule)
const mockUseDeletePricingRule = vi.mocked(useDeletePricingRule)
const mockUsePreviewPricingRule = vi.mocked(usePreviewPricingRule)
const mockUsePricingRuleAudit = vi.mocked(usePricingRuleAudit)

describe('PricingRulesPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseAuth.mockReturnValue({ user: { role: 'admin' } } as any)
    mockHasRole.mockReturnValue(true)
    mockUsePricingRules.mockReturnValue({ data: { items: [], count: 0 }, isLoading: false, error: null } as any)
    mockUseUpsertPricingRule.mockReturnValue({ mutate: vi.fn(), isPending: false, isError: false } as any)
    mockUseDeletePricingRule.mockReturnValue({ mutate: vi.fn(), isPending: false } as any)
    mockUsePreviewPricingRule.mockReturnValue({ mutate: vi.fn(), isPending: false, isError: false } as any)
    mockUsePricingRuleAudit.mockReturnValue({ data: { items: [] } } as any)
  })

  it('shows restricted message for non-admin users', () => {
    mockHasRole.mockReturnValue(false)
    render(<PricingRulesPage />)
    expect(screen.getByText(/restricted to administrators/i)).toBeInTheDocument()
  })

  it('saves a rule and resets the form', () => {
    const mutate = vi.fn((payload, opts) => opts?.onSuccess?.())
    mockUseUpsertPricingRule.mockReturnValue({ mutate, isPending: false, isError: false } as any)

    render(<PricingRulesPage />)
    fireEvent.change(screen.getByRole('textbox', { name: 'Model Pattern' }), { target: { value: 'gpt-4o-mini' } })
    fireEvent.click(screen.getByRole('button', { name: /create rule/i }))

    expect(mutate).toHaveBeenCalledWith(expect.objectContaining({
      model_pattern: 'gpt-4o-mini',
      tenant_id: null,
    }), expect.any(Object))
    expect(screen.getByRole('textbox', { name: 'Model Pattern' })).toHaveValue('')
  })

  it('renders rules, preview stats, and audit entries', () => {
    const previewMutate = vi.fn()
    mockUsePricingRules.mockReturnValue({
      data: {
        items: [{
          id: 4,
          provider: 'openai',
          model_pattern: 'gpt-4o',
          input_per_million: 5,
          output_per_million: 15,
          priority: 100,
          active: true,
          description: 'default rule',
        }],
        count: 1,
      },
      isLoading: false,
      error: null,
    } as any)
    mockUsePreviewPricingRule.mockReturnValue({
      mutate: previewMutate,
      isPending: false,
      isError: false,
      data: {
        matched: true,
        rule_id: 4,
        pricing_scope: 'global',
        model_pattern: 'gpt-4o',
        input_cost_usd: 0.005,
        output_cost_usd: 0.0075,
        total_cost_usd: 0.0125,
      },
    } as any)
    mockUsePricingRuleAudit.mockReturnValue({
      data: {
        items: [{
          id: 1,
          action: 'upsert',
          rule_id: 4,
          actor: 'architect',
          tenant_id: 'tenant-a',
          created_at: '2026-01-01T00:00:00Z',
        }],
      },
    } as any)

    render(<PricingRulesPage />)
    expect(screen.getByText('default rule')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /run preview/i }))
    expect(previewMutate).toHaveBeenCalledWith(expect.objectContaining({ model: 'gpt-4o' }))
    expect(screen.getByText('$0.012500')).toBeInTheDocument()
    expect(screen.getByText(/architect/i)).toBeInTheDocument()
  })
})
