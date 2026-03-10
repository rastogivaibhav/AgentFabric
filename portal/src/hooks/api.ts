import { useQuery } from '@tanstack/react-query'

const BASE = import.meta.env.VITE_API_URL ?? 'http://localhost:8080'
const TOKEN = localStorage.getItem('af_token') ?? ''

async function apiFetch<T>(path: string, params?: Record<string, string>): Promise<T> {
  const url = new URL(BASE + '/api/v1' + path)
  if (params) Object.entries(params).forEach(([k, v]) => v && url.searchParams.set(k, v))
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  if (TOKEN) headers['Authorization'] = `Bearer ${TOKEN}`
  const res = await fetch(url.toString(), { headers })
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

// ─── WebSocket hook ───────────────────────────────────────────────────────────

import { useEffect, useRef, useCallback, useState } from 'react'

export function useLiveStream(maxEvents = 500) {
  const [events, setEvents] = useState<LiveEvent[]>([])
  const [connected, setConnected] = useState(false)
  const wsRef = useRef<WebSocket | null>(null)
  const pausedRef = useRef(false)

  const connect = useCallback(() => {
    const wsBase = BASE.replace(/^http/, 'ws')
    const ws = new WebSocket(`${wsBase}/api/v1/stream/live`)
    wsRef.current = ws

    ws.onopen = () => setConnected(true)
    ws.onclose = () => {
      setConnected(false)
      setTimeout(connect, 3000) // auto-reconnect
    }
    ws.onerror = () => ws.close()
    ws.onmessage = (e) => {
      if (pausedRef.current) return
      try {
        const event: LiveEvent = JSON.parse(e.data)
        setEvents(prev => [event, ...prev].slice(0, maxEvents))
      } catch {}
    }
  }, [maxEvents])

  useEffect(() => {
    connect()
    return () => wsRef.current?.close()
  }, [connect])

  const pause = () => { pausedRef.current = true }
  const resume = () => { pausedRef.current = false }
  const clear = () => setEvents([])

  return { events, connected, pause, resume, clear }
}
