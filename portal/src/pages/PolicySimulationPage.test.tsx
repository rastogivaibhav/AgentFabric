import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import PolicySimulationPage from './PolicySimulationPage'

vi.mock('../hooks/api', () => ({
  useSimulatePolicyRule: vi.fn(),
}))

vi.mock('./PolicyDecisionExplorer', () => ({
  default: ({ label, decision }: any) => <div>{label}:{decision.action ?? 'none'}</div>,
}))

import { useSimulatePolicyRule } from '../hooks/api'

const mockUseSimulatePolicyRule = vi.mocked(useSimulatePolicyRule)

describe('PolicySimulationPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseSimulatePolicyRule.mockReturnValue({ mutate: vi.fn(), isPending: false } as any)
  })

  it('submits normalized simulation input', () => {
    const mutate = vi.fn()
    mockUseSimulatePolicyRule.mockReturnValue({ mutate, isPending: false } as any)

    render(<PolicySimulationPage />)
    fireEvent.change(screen.getByDisplayValue('openai'), { target: { value: ' Google ' } })
    fireEvent.change(screen.getByDisplayValue('gpt-4o'), { target: { value: ' Gemini-2.5 ' } })
    fireEvent.change(screen.getByDisplayValue('production'), { target: { value: ' Staging ' } })
    fireEvent.click(screen.getByRole('button', { name: /run simulation/i }))

    expect(mutate).toHaveBeenCalledWith([expect.objectContaining({
      provider: 'google',
      model: 'gemini-2.5',
      environment: 'staging',
    })])
  })

  it('renders simulation results and error state', () => {
    mockUseSimulatePolicyRule.mockReturnValue({
      mutate: vi.fn(),
      isPending: false,
      isError: true,
      data: {
        results: [{
          label: 'candidate scenario',
          traffic: { matched: true, action: 'deny' },
          request_dlp: { matched: false },
          response_dlp: { matched: true, action: 'redact' },
        }],
      },
    } as any)

    render(<PolicySimulationPage />)
    expect(screen.getByText(/failed to simulate policy set/i)).toBeInTheDocument()
    expect(screen.getByText('Traffic:deny')).toBeInTheDocument()
    expect(screen.getByText('Response DLP:redact')).toBeInTheDocument()
  })
})
