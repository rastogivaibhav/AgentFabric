import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const invalidateQueries = vi.fn()
const useQueryMock = vi.fn((options) => options)
const useMutationMock = vi.fn((options) => ({ ...options, mutate: vi.fn() }))
const useQueryClientMock = vi.fn(() => ({ invalidateQueries }))

vi.mock('@tanstack/react-query', () => ({
  useQuery: (options: unknown) => useQueryMock(options),
  useMutation: (options: unknown) => useMutationMock(options),
  useQueryClient: () => useQueryClientMock(),
}))

import {
  apiFetch,
  apiMutate,
  useCollectors,
  useEnvironments,
  useCompareEvalRegressions,
  useControlAudit,
  useDeletePolicyRule,
  useEvalRuns,
  useLiveStream,
  useOverview,
  usePromotePromptRelease,
  useScoreTraceEval,
  useTraceComparison,
  useTraces,
  useUpsertTraceSavedView,
} from './api'

class MockWebSocket {
  static instances: MockWebSocket[] = []
  url: string
  onopen: (() => void) | null = null
  onclose: (() => void) | null = null
  onerror: (() => void) | null = null
  onmessage: ((event: { data: string }) => void) | null = null
  close = vi.fn()

  constructor(url: string) {
    this.url = url
    MockWebSocket.instances.push(this)
  }
}

describe('api runtime coverage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    invalidateQueries.mockReset()
    useQueryMock.mockClear()
    useMutationMock.mockClear()
    ;(globalThis as any).fetch = vi.fn()
    ;(globalThis as any).WebSocket = MockWebSocket
    MockWebSocket.instances = []
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('apiFetch sends credentials and returns JSON', async () => {
    vi.mocked(fetch).mockResolvedValue({
      ok: true,
      json: async () => ({ ok: true }),
    } as any)

    await expect(apiFetch('/traces', { limit: '10' })).resolves.toEqual({ ok: true })
    expect(fetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/v1/traces?limit=10'),
      expect.objectContaining({ credentials: 'include' }),
    )
  })

  it('apiMutate handles 204 responses', async () => {
    vi.mocked(fetch).mockResolvedValue({
      ok: true,
      status: 204,
    } as any)

    await expect(apiMutate('/policies/1', 'DELETE')).resolves.toBeUndefined()
  })

  it('configures overview, traces, and comparison hooks', () => {
    renderHook(() => useOverview('7d'))
    renderHook(() => useTraces({ framework: 'langgraph' }))
    renderHook(() => useTraceComparison('left', 'right'))

    expect(useQueryMock).toHaveBeenCalledWith(expect.objectContaining({
      queryKey: ['overview', '7d'],
      refetchInterval: 30000,
    }))
    expect(useQueryMock).toHaveBeenCalledWith(expect.objectContaining({
      queryKey: ['traces', { framework: 'langgraph' }],
    }))
    expect(useQueryMock).toHaveBeenCalledWith(expect.objectContaining({
      queryKey: ['trace-compare', 'left', 'right'],
      enabled: true,
    }))
  })

  it('collector query falls back to empty list on 404', async () => {
    renderHook(() => useCollectors())
    const query = useQueryMock.mock.calls.at(-1)?.[0] as any

    vi.mocked(fetch).mockResolvedValue({
      ok: false,
      status: 404,
    } as any)

    await expect(query.queryFn()).resolves.toEqual([])
  })

  it('environments query uses the authenticated api fetch path', async () => {
    renderHook(() => useEnvironments())
    const query = useQueryMock.mock.calls.at(-1)?.[0] as any

    vi.mocked(fetch).mockResolvedValue({
      ok: true,
      json: async () => ([{ name: 'production', status: 'active', span_count: 12 }]),
    } as any)

    await expect(query.queryFn()).resolves.toEqual([{ name: 'production', status: 'active', span_count: 12 }])
    expect(fetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/v1/environments'),
      expect.objectContaining({ credentials: 'include' }),
    )
  })

  it('live stream records messages and supports clear', async () => {
    const { result } = renderHook(() => useLiveStream(2))
    const ws = MockWebSocket.instances[0]

    act(() => {
      ws.onopen?.()
      ws.onmessage?.({ data: JSON.stringify({ type: 'span', ts: 1, data: { id: 'a' } }) })
      ws.onmessage?.({ data: JSON.stringify({ type: 'policy', ts: 2, data: { id: 'b' } }) })
    })

    expect(result.current.connected).toBe(true)
    expect(result.current.events).toHaveLength(2)

    act(() => result.current.clear())
    expect(result.current.events).toHaveLength(0)
  })

  it('mutation hooks invalidate their expected caches', async () => {
    renderHook(() => useUpsertTraceSavedView())
    renderHook(() => useDeletePolicyRule())
    renderHook(() => useScoreTraceEval())
    renderHook(() => usePromotePromptRelease())
    renderHook(() => useCompareEvalRegressions())
    renderHook(() => useEvalRuns(15))
    renderHook(() => useControlAudit(25))

    const mutationConfigs = useMutationMock.mock.calls.map(call => call[0] as any)
    for (const config of mutationConfigs) {
      if (typeof config.onSuccess === 'function') {
        config.onSuccess()
      }
    }

    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: ['trace-saved-views'] })
    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: ['policy-rules'] })
    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: ['control-audit'] })
    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: ['eval-runs'] })
    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: ['prompts'] })

    expect(useQueryMock).toHaveBeenCalledWith(expect.objectContaining({
      queryKey: ['eval-runs', 15],
      retry: false,
    }))
    expect(useQueryMock).toHaveBeenCalledWith(expect.objectContaining({
      queryKey: ['control-audit', 25],
      retry: false,
    }))
  })
})
