import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import ApiKeysPage from './ApiKeysPage'

vi.mock('../hooks/api', () => ({
  apiFetch: vi.fn(),
  apiMutate: vi.fn(),
  SUPPORTED_KEY_PROVIDERS: [
    { value: 'openai', label: 'OpenAI', route_hint: '/proxy/openai/v1/chat/completions', key_placeholder: 'sk-...' },
    { value: 'google', label: 'Google Gemini', route_hint: '/proxy/google/v1beta/models', key_placeholder: 'AIza...' },
  ],
}))

import { apiFetch, apiMutate } from '../hooks/api'

const mockApiFetch = vi.mocked(apiFetch)
const mockApiMutate = vi.mocked(apiMutate)

describe('ApiKeysPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.useRealTimers()
    Object.assign(navigator, {
      clipboard: { writeText: vi.fn().mockResolvedValue(undefined) },
    })
    mockApiFetch.mockResolvedValue({ items: [], count: 0 } as any)
    mockApiMutate.mockResolvedValue({ virtual_key: 'af-vk-1234567890', provider: 'google' } as any)
  })

  function renderPage() {
    const client = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })
    return render(
      <QueryClientProvider client={client}>
        <ApiKeysPage />
      </QueryClientProvider>,
    )
  }

  it('renders empty state when no keys exist', async () => {
    renderPage()
    expect(await screen.findByText(/no virtual keys yet/i)).toBeInTheDocument()
  })

  it('registers a key and reveals the generated virtual key', async () => {
    renderPage()

    fireEvent.click(screen.getByRole('button', { name: /add key/i }))
    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'google' } })
    fireEvent.change(screen.getByPlaceholderText('e.g. Prod OpenAI'), { target: { value: 'Gemini prod' } })
    fireEvent.change(screen.getByPlaceholderText('AIza...'), { target: { value: 'AIza-real' } })
    fireEvent.click(screen.getByRole('button', { name: 'Register' }))

    await waitFor(() => {
      expect(mockApiMutate).toHaveBeenCalledWith('/keys', 'POST', expect.objectContaining({
        provider: 'google',
        real_key: 'AIza-real',
        display_name: 'Gemini prod',
      }))
    })

    expect(await screen.findByText(/key registered/i)).toBeInTheDocument()
    expect(screen.getAllByText('af-vk-1234567890')).toHaveLength(2)
    expect(screen.getByText(/proxy\/google/i)).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /copy/i }))
    await waitFor(() => {
      expect(navigator.clipboard.writeText).toHaveBeenCalledWith('af-vk-1234567890')
    })
    expect(screen.getByText('Copied')).toBeInTheDocument()
  })

  it('renders keys and handles revoke confirmation', async () => {
    mockApiFetch.mockResolvedValue({
      items: [{
        virtual_key: 'af-vk-abcdef123456',
        display_name: 'Prod OpenAI',
        provider: 'openai',
        key_id: 'key-prefix',
        revoked: false,
        created_at: '2026-01-01T10:00:00Z',
        last_used_at: '2026-01-02T11:00:00Z',
      }],
      count: 1,
    } as any)
    mockApiMutate.mockResolvedValue(undefined as any)

    renderPage()

    expect(await screen.findByText('Prod OpenAI')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /revoke/i }))
    expect(screen.getByRole('button', { name: 'Confirm' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Confirm' }))

    await waitFor(() => {
      expect(mockApiMutate).toHaveBeenCalledWith('/keys/af-vk-abcdef123456', 'DELETE')
    })
  })
})
