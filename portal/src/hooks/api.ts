import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'

const BASE = import.meta.env.VITE_API_URL ?? 'http://localhost:8080'

// apiFetch uses credentials:'include' so the browser automatically sends the
// af_token HttpOnly cookie on every request. The raw token is never read by JS.
// The api-gateway JWTAuth middleware accepts this cookie as a valid auth source.
// CLI / API callers may still use Authorization: Bearer by calling the gateway directly.
export async function apiFetch<T>(path: string, params?: Record<string, string>): Promise<T> {
  const url = new URL(BASE + '/api/v1' + path)
  if (params) Object.entries(params).forEach(([k, v]) => v && url.searchParams.set(k, v))
  const res = await fetch(url.toString(), {
    headers: { 'Content-Type': 'application/json' },
    credentials: 'include', // sends af_token HttpOnly cookie; JS never reads the token
  })
  if (!res.ok) throw new Error(`API ${res.status}: ${path}`)
  return res.json()
}

// ─── Types ────────────────────────────────────────────────────────────────────

export interface Span {
  id: string
  trace_id: string
  parent_id?: string
  run_id: string
  name: string
  framework: string
  start_time_ns: number
  duration_ns: number
  status_code: number
  status_msg?: string
  attributes: Record<string, string>
  events?: SpanEvent[]
  input_tokens?: number
  output_tokens?: number
  cost_usd?: number
  received_at: string
}

export interface SpanEvent {
  name: string
  time_ns: number
  attributes?: Record<string, string>
}

export interface Trace {
  id: string
  root_span_name: string
  framework: string
  start_time: string
  duration_ns: number
  span_count: number
  error_count: number
  total_cost_usd: number
  total_tokens: number
  status: 'ok' | 'error' | 'partial'
  spans?: Span[]
}

export interface Page<T> {
  items: T[]
  total: number
  next_cursor?: string
  has_more: boolean
}

export interface OverviewStats {
  total_traces: number
  active_agents: number
  total_cost_usd: number
  total_tokens: number
  error_rate: number
  avg_latency_ms: number
  spans_per_second: number
  framework_counts: Record<string, number>
}

export interface LiveEvent {
  type: 'span' | 'run_start' | 'run_end' | 'error' | 'policy'
  ts: number
  data: Span | Record<string, unknown>
}

// ─── Hooks ────────────────────────────────────────────────────────────────────

export function useOverview(since = '24h') {
  return useQuery<OverviewStats>({
    queryKey: ['overview', since],
    queryFn: () => apiFetch('/analytics/overview', { since }),
    refetchInterval: 30_000,
  })
}

export function useTraces(params: Record<string, string> = {}) {
  return useQuery<Page<Trace>>({
    queryKey: ['traces', params],
    queryFn: () => apiFetch('/traces', params),
  })
}

export function useTrace(traceId: string) {
  return useQuery<Trace>({
    queryKey: ['trace', traceId],
    queryFn: () => apiFetch(`/traces/${traceId}`),
    enabled: !!traceId,
  })
}

export function useTraceGraph(traceId: string) {
  return useQuery({
    queryKey: ['trace-graph', traceId],
    queryFn: () => apiFetch(`/traces/${traceId}/graph`),
    enabled: !!traceId,
  })
}

export function useFrameworkStats() {
  return useQuery({
    queryKey: ['framework-stats'],
    queryFn: () => apiFetch('/analytics/frameworks'),
    refetchInterval: 60_000,
  })
}

export interface CollectorInfo {
  id: string
  name: string
  endpoint_grpc: string
  endpoint_http: string
  status: 'healthy' | 'degraded' | 'unreachable' | string
  version: string
  last_checked?: string
}

export function useCollectors() {
  return useQuery<CollectorInfo[]>({
    queryKey: ['collectors'],
    queryFn: async () => {
      try {
        const result = await apiFetch<CollectorInfo[]>('/collectors')
        return result
      } catch (err: unknown) {
        // If the backend endpoint is not yet implemented (404), return empty array
        // so the UI can fall back to static hardcoded cards gracefully
        const msg = err instanceof Error ? err.message : String(err)
        if (msg.includes('404')) return []
        throw err
      }
    },
    refetchInterval: 30_000,
    retry: false,
  })
}

// ─── WebSocket hook ───────────────────────────────────────────────────────────

import { useEffect, useRef, useCallback, useState } from 'react'

export function useLiveStream(maxEvents = 500) {
  const [events, setEvents] = useState<LiveEvent[]>([])
  const [connected, setConnected] = useState(false)
  const wsRef = useRef<WebSocket | null>(null)
  const pausedRef = useRef(false)
  // Lifecycle guard — prevents reconnect attempts after the component unmounts.
  const mountedRef = useRef(true)
  // Exponential backoff state: retry 0 → 1s, 1 → 2s, 2 → 4s … capped at 30s.
  const retryCountRef = useRef(0)
  const retryTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const connect = useCallback(() => {
    if (!mountedRef.current) return

    const wsBase = BASE.replace(/^http/, 'ws')
    const ws = new WebSocket(`${wsBase}/api/v1/stream/live`)
    wsRef.current = ws

    ws.onopen = () => {
      if (!mountedRef.current) { ws.close(); return }
      setConnected(true)
      retryCountRef.current = 0 // reset backoff on successful connection
    }

    ws.onclose = () => {
      if (!mountedRef.current) return
      setConnected(false)
      // Exponential backoff: 1s, 2s, 4s, 8s, 16s, 30s (max)
      const delay = Math.min(30_000, 1_000 * Math.pow(2, retryCountRef.current))
      retryCountRef.current = Math.min(retryCountRef.current + 1, 5)
      retryTimerRef.current = setTimeout(connect, delay)
    }

    ws.onerror = () => ws.close()

    ws.onmessage = (e) => {
      if (pausedRef.current || !mountedRef.current) return
      try {
        const event: LiveEvent = JSON.parse(e.data)
        setEvents(prev => [event, ...prev].slice(0, maxEvents))
      } catch {}
    }
  }, [maxEvents])

  useEffect(() => {
    mountedRef.current = true
    connect()
    return () => {
      mountedRef.current = false
      if (retryTimerRef.current) clearTimeout(retryTimerRef.current)
      wsRef.current?.close()
    }
  }, [connect])

  const pause = () => { pausedRef.current = true }
  const resume = () => { pausedRef.current = false }
  const clear = () => setEvents([])

  return { events, connected, pause, resume, clear }
}

// ─── Budget hooks ─────────────────────────────────────────────────────────────

export interface Budget {
  tenant_id: string
  monthly_tokens: number    // 0 = unlimited
  monthly_cost_usd: number  // 0 = unlimited
  alert_threshold: number   // 0.80 = 80%
  hard_limit: boolean
  reset_day: number
  created_at?: string
  updated_at?: string
}

export interface BudgetUsage {
  tokens_used: number
  cost_used_usd: number
  period_start: string
  period_end: string
  tokens_pct?: number
  cost_pct?: number
  budget?: Budget
}

export async function apiMutate<T>(path: string, method: string, body?: unknown): Promise<T> {
  const res = await fetch(BASE + '/api/v1' + path, {
    method,
    headers: { 'Content-Type': 'application/json' },
    credentials: 'include',
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })
  if (!res.ok) throw new Error(`API ${res.status}: ${path}`)
  if (res.status === 204) return undefined as T
  return res.json()
}

export function useBudget(tenantId: string) {
  return useQuery<Budget>({
    queryKey: ['budget', tenantId],
    queryFn: () => apiFetch(`/budgets/${tenantId}`),
    retry: false,
    enabled: !!tenantId,
  })
}

export function useBudgetUsage(tenantId: string) {
  return useQuery<BudgetUsage>({
    queryKey: ['budget-usage', tenantId],
    queryFn: () => apiFetch(`/budgets/${tenantId}/usage`),
    retry: false,
    enabled: !!tenantId,
    refetchInterval: 30_000,
  })
}

export function useUpsertBudget(tenantId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (b: Partial<Budget>) => apiMutate<Budget>(`/budgets/${tenantId}`, 'PUT', b),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['budget', tenantId] })
      qc.invalidateQueries({ queryKey: ['budget-usage', tenantId] })
    },
  })
}

export function useDeleteBudget(tenantId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => apiMutate<void>(`/budgets/${tenantId}`, 'DELETE'),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['budget', tenantId] })
      qc.invalidateQueries({ queryKey: ['budget-usage', tenantId] })
    },
  })
}
