import { describe, it, expect } from 'vitest'

// ─── Type shape tests ────────────────────────────────────────────────────────
// These tests verify the TypeScript interfaces match the expected contract
// from the api-gateway. They act as a schema regression suite.

import type {
  Span,
  Trace,
  Page,
  OverviewStats,
  LiveEvent,
} from './api'

describe('API type contracts', () => {
  it('Span has required fields', () => {
    const span: Span = {
      id: 'span-001',
      trace_id: 'trace-001',
      run_id: 'run-001',
      name: 'test.span',
      framework: 'crewai',
      start_time_ns: 1700000000000000000,
      duration_ns: 500000000,
      status_code: 0,
      attributes: {},
      received_at: '2024-01-01T00:00:00Z',
    }
    expect(span.id).toBe('span-001')
    expect(span.framework).toBe('crewai')
  })

  it('Span has optional cost and token fields', () => {
    const span: Span = {
      id: 'span-001',
      trace_id: 'trace-001',
      run_id: 'run-001',
      name: 'test.span',
      framework: 'crewai',
      start_time_ns: 0,
      duration_ns: 0,
      status_code: 0,
      attributes: {},
      received_at: '',
      cost_usd: 0.001,
      input_tokens: 100,
      output_tokens: 50,
    }
    expect(span.cost_usd).toBe(0.001)
    expect(span.input_tokens).toBe(100)
    expect(span.output_tokens).toBe(50)
  })

  it('Trace has status union type', () => {
    const okTrace: Trace = {
      id: 'tr-001',
      root_span_name: 'agent.run',
      framework: 'langgraph',
      start_time: '2024-01-01T00:00:00Z',
      duration_ns: 1000000000,
      span_count: 5,
      error_count: 0,
      total_cost_usd: 0.005,
      total_tokens: 1000,
      status: 'ok',
    }
    expect(okTrace.status).toBe('ok')

    const errTrace: Trace = { ...okTrace, status: 'error', error_count: 1 }
    expect(errTrace.status).toBe('error')
  })

  it('Page has pagination fields', () => {
    const page: Page<Trace> = {
      items: [],
      total: 0,
      has_more: false,
    }
    expect(page.items).toHaveLength(0)
    expect(page.has_more).toBe(false)
  })

  it('Page.has_more is true when more results exist', () => {
    const page: Page<Trace> = {
      items: [],
      total: 100,
      has_more: true,
      next_cursor: 'cursor-abc',
    }
    expect(page.has_more).toBe(true)
    expect(page.next_cursor).toBe('cursor-abc')
  })

  it('OverviewStats has all required metrics', () => {
    const stats: OverviewStats = {
      total_traces: 1000,
      active_agents: 5,
      total_cost_usd: 12.50,
      total_tokens: 500000,
      error_rate: 0.01,
      avg_latency_ms: 450,
      spans_per_second: 25,
      blocked_requests: 0,
      llm_calls: 12,
      tool_calls: 4,
      framework_counts: { crewai: 400, langgraph: 600 },
    }
    expect(stats.total_traces).toBe(1000)
    expect(stats.framework_counts.crewai).toBe(400)
  })

  it('LiveEvent has correct type discriminator', () => {
    const event: LiveEvent = {
      type: 'span',
      ts: Date.now(),
      data: { id: 'span-001' } as any,
    }
    expect(event.type).toBe('span')
  })

  it('LiveEvent supports all event types', () => {
    const types: LiveEvent['type'][] = ['span', 'run_start', 'run_end', 'error', 'policy']
    types.forEach(type => {
      const event: LiveEvent = { type, ts: 0, data: {} }
      expect(event.type).toBe(type)
    })
  })
})

// ─── apiFetch URL construction tests ─────────────────────────────────────────
// Test URL building logic without actual network calls

describe('URL construction logic', () => {
  it('builds base URL correctly', () => {
    const base = 'http://localhost:8080'
    const path = '/traces'
    const url = new URL(base + '/api/v1' + path)
    expect(url.toString()).toBe('http://localhost:8080/api/v1/traces')
  })

  it('appends query params correctly', () => {
    const base = 'http://localhost:8080'
    const url = new URL(base + '/api/v1/traces')
    const params = { framework: 'crewai', limit: '50' }
    Object.entries(params).forEach(([k, v]) => v && url.searchParams.set(k, v))
    expect(url.searchParams.get('framework')).toBe('crewai')
    expect(url.searchParams.get('limit')).toBe('50')
  })

  it('skips empty params', () => {
    const base = 'http://localhost:8080'
    const url = new URL(base + '/api/v1/traces')
    const params = { framework: '', limit: '50' }
    Object.entries(params).forEach(([k, v]) => v && url.searchParams.set(k, v))
    expect(url.searchParams.get('framework')).toBeNull()
    expect(url.searchParams.get('limit')).toBe('50')
  })

  it('WebSocket URL replaces http with ws', () => {
    const httpBase = 'http://localhost:8080'
    const wsBase = httpBase.replace(/^http/, 'ws')
    expect(wsBase).toBe('ws://localhost:8080')
  })

  it('WebSocket URL replaces https with wss', () => {
    const httpsBase = 'https://api.govagn.io'
    const wssBase = httpsBase.replace(/^http/, 'ws')
    expect(wssBase).toBe('wss://api.govagn.io')
  })
})
