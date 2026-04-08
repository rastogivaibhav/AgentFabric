import type { Page, Request, Route } from '@playwright/test'

type Role = 'admin' | 'viewer'

interface MockPortalOptions {
  authenticated?: boolean
  role?: Role
}

interface VirtualKeyRecord {
  virtual_key: string
  display_name: string
  provider: string
  key_id: string
  last_used_at?: string
  revoked: boolean
  created_at: string
}

interface PolicyRuleRecord {
  id: number
  name: string
  rule_type: 'traffic' | 'dlp'
  action: 'allow' | 'warn' | 'deny' | 'redact'
  provider: string
  model_pattern: string
  environment: string
  enabled: boolean
  priority: number
  decision_mode: 'fast' | 'rego'
  guardrails: string[]
  rollout_percent: number
  version: number
}

interface RolloutRecord {
  id: number
  name: string
  target_type: 'policy_rule' | 'prompt_release' | 'model'
  target_id: string
  policy_rule_id?: number
  environment: string
  percentage: number
  status: 'active' | 'paused'
  control_release_tag?: string
  candidate_release_tag?: string
  recent_requests?: number
  recent_error_rate?: number
}

interface RecommendationRecord {
  id: number
  type: 'policy' | 'cost' | 'rollout' | 'routing'
  status: 'open' | 'reviewing' | 'applied' | 'dismissed' | 'resolved'
  title: string
  summary: string
  target: string
  suggested_action: string
  confidence: number
  created_at: string
  updated_at: string
  last_seen_at: string
}

interface PromptVersionRecord {
  id: number
  prompt_id: string
  version: number
  environment: string
  release_tag: string
  content: string
  description: string
  promoted: boolean
  is_latest: boolean
  current_release?: PromptReleaseRecord
}

interface PromptReleaseRecord {
  id: number
  prompt_id: string
  environment: string
  version: number
  release_tag: string
  status: string
  promoted_by: string
  created_at: string
  notes?: string
  promotion_reason?: string
  eval_summary: {
    eval_count: number
    average_score: number
    latest_score: number
    risk_level: string
  }
  regression_summary?: {
    baseline_tag: string
    candidate_tag: string
    compared_runs: number
    overall_delta: number
    risk_level: string
    summary: string
  }
}

interface TraceRecord {
  id: string
  root_span_name: string
  framework: string
  start_time: string
  duration_ns: number
  span_count: number
  error_count: number
  total_cost_usd: number
  total_tokens: number
  status: 'ok' | 'partial' | 'error'
}

function json(status: number, body: unknown) {
  return {
    status,
    contentType: 'application/json',
    body: JSON.stringify(body),
  }
}

async function fulfill(route: Route, status: number, body: unknown) {
  await route.fulfill(json(status, body))
}

function parseBody(request: Request) {
  const raw = request.postData()
  if (!raw) return undefined
  try {
    return JSON.parse(raw) as Record<string, unknown>
  } catch {
    return undefined
  }
}

function toLower(value: unknown): string {
  return String(value ?? '').trim().toLowerCase()
}

export async function mockPortalApi(page: Page, options: MockPortalOptions = {}) {
  const session = {
    authenticated: options.authenticated ?? true,
    role: options.role ?? 'admin',
  }

  let keys: VirtualKeyRecord[] = [
    {
      virtual_key: 'af-vk-openai-initial',
      display_name: 'Prod OpenAI',
      provider: 'openai',
      key_id: 'key-openai-01',
      last_used_at: '2026-03-25T10:00:00Z',
      revoked: false,
      created_at: '2026-03-24T10:00:00Z',
    },
  ]

  let policyRules: PolicyRuleRecord[] = [
    {
      id: 7,
      name: 'Block prod secrets',
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
    },
  ]

  let rollouts: RolloutRecord[] = [
    {
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
    },
    {
      id: 90,
      name: 'Prompt support rollout',
      target_type: 'prompt_release',
      target_id: 'support-bot.system',
      environment: 'production',
      percentage: 20,
      status: 'active',
      control_release_tag: '2026.02',
      candidate_release_tag: '2026.03',
      recent_requests: 42,
      recent_error_rate: 0.01,
    },
  ]

  let recommendations: RecommendationRecord[] = [
    {
      id: 21,
      type: 'policy',
      status: 'open',
      title: 'Tighten policy guardrails for openai / gpt-4o / production',
      summary: '5 policy interventions landed on openai / gpt-4o / production.',
      target: 'openai / gpt-4o / production',
      suggested_action: 'Promote the matching guardrail from warn to redact or deny.',
      confidence: 0.78,
      created_at: '2026-03-25T12:00:00Z',
      updated_at: '2026-03-25T12:00:00Z',
      last_seen_at: '2026-03-25T12:00:00Z',
    },
    {
      id: 61,
      type: 'rollout',
      status: 'open',
      title: 'Increase prompt canary from 20% to 40%',
      summary: 'Candidate release is outperforming baseline for support prompts.',
      target: 'support-bot.system',
      suggested_action: 'Ramp rollout percentage to 40% during low traffic window.',
      confidence: 0.83,
      created_at: '2026-03-25T12:05:00Z',
      updated_at: '2026-03-25T12:05:00Z',
      last_seen_at: '2026-03-25T12:05:00Z',
    },
  ]

  const promptRelease: PromptReleaseRecord = {
    id: 91,
    prompt_id: 'support-bot.system',
    environment: 'production',
    version: 3,
    release_tag: '2026.03',
    status: 'active',
    promoted_by: 'admin@example.com',
    created_at: '2026-03-22T10:00:00Z',
    eval_summary: {
      eval_count: 4,
      average_score: 88.2,
      latest_score: 89.1,
      risk_level: 'low',
    },
    regression_summary: {
      baseline_tag: '2026.02',
      candidate_tag: '2026.03',
      compared_runs: 4,
      overall_delta: 3.4,
      risk_level: 'low',
      summary: 'Average eval score moved by 3.40 points versus 2026.02.',
    },
  }

  let promptVersions: PromptVersionRecord[] = [
    {
      id: 10,
      prompt_id: 'support-bot.system',
      version: 3,
      environment: 'production',
      release_tag: '2026.03',
      content: 'You are a support assistant.',
      description: 'production prompt',
      promoted: true,
      is_latest: true,
      current_release: promptRelease,
    },
    {
      id: 9,
      prompt_id: 'support-bot.system',
      version: 2,
      environment: 'production',
      release_tag: '2026.02',
      content: 'You are a concise support assistant.',
      description: 'baseline prompt',
      promoted: false,
      is_latest: false,
    },
  ]

  let promptReleases: PromptReleaseRecord[] = [promptRelease]

  const traces: TraceRecord[] = [
    {
      id: 'trace-left',
      root_span_name: 'assistant.run',
      framework: 'langgraph',
      start_time: '2026-03-22T10:00:00Z',
      duration_ns: 800000000,
      span_count: 4,
      error_count: 0,
      total_cost_usd: 0.0042,
      total_tokens: 1200,
      status: 'ok',
    },
    {
      id: 'trace-right',
      root_span_name: 'assistant.run',
      framework: 'langgraph',
      start_time: '2026-03-22T10:05:00Z',
      duration_ns: 1200000000,
      span_count: 5,
      error_count: 1,
      total_cost_usd: 0.0061,
      total_tokens: 1500,
      status: 'error',
    },
    {
      id: 'trace-third',
      root_span_name: 'retriever.run',
      framework: 'crewai',
      start_time: '2026-03-22T10:10:00Z',
      duration_ns: 640000000,
      span_count: 6,
      error_count: 0,
      total_cost_usd: 0.0038,
      total_tokens: 980,
      status: 'ok',
    },
  ]

  let savedViews = [
    {
      id: 1,
      name: 'Errors only',
      filters: { status: 'error' },
      created_at: '2026-03-20T09:00:00Z',
      updated_at: '2026-03-20T09:00:00Z',
    },
  ]

  await page.route('**/auth/me', async (route) => {
    if (!session.authenticated) {
      await fulfill(route, 401, { error: 'unauthorized' })
      return
    }
    await fulfill(route, 200, {
      sub: `${session.role}-1`,
      email: `${session.role}@example.com`,
      name: `${session.role} user`,
      role: session.role,
      exp: Math.floor(Date.now() / 1000) + 3600,
    })
  })

  await page.route('**/auth/login', async (route) => {
    const body = parseBody(route.request())
    const username = String(body?.username ?? '')
    const password = String(body?.password ?? '')

    if ((username === 'admin' && password === 'admin') || (username === 'viewer' && password === 'viewer')) {
      session.authenticated = true
      session.role = username === 'viewer' ? 'viewer' : 'admin'
      await fulfill(route, 200, { ok: true })
      return
    }

    await fulfill(route, 401, { error: 'Invalid credentials.' })
  })

  await page.route('**/auth/logout', async (route) => {
    session.authenticated = false
    await fulfill(route, 200, { ok: true })
  })

  await page.route('**/auth/refresh', async (route) => {
    if (!session.authenticated) {
      await fulfill(route, 401, { error: 'session_expired' })
      return
    }
    await fulfill(route, 200, { exp: Math.floor(Date.now() / 1000) + 3600 })
  })

  await page.route('**/api/v1/**', async (route) => {
    const request = route.request()
    const method = request.method()
    const url = new URL(request.url())
    const path = url.pathname.replace('/api/v1', '')
    const body = parseBody(request)

    if (!session.authenticated) {
      await fulfill(route, 401, { error: 'unauthorized' })
      return
    }

    if (method === 'GET' && path === '/analytics/overview') {
      await fulfill(route, 200, {
        total_traces: traces.length,
        active_agents: 3,
        total_cost_usd: 1.2345,
        total_tokens: 12000,
        error_rate: 0.02,
        avg_latency_ms: 345,
        spans_per_second: 8,
        blocked_requests: 1,
        llm_calls: 10,
        tool_calls: 4,
        framework_counts: { crewai: 12, langgraph: 30 },
      })
      return
    }

    if (method === 'GET' && path === '/analytics/frameworks') {
      await fulfill(route, 200, { crewai: 12, langgraph: 30 })
      return
    }

    if (method === 'GET' && path === '/audit/control') {
      const limit = Number(url.searchParams.get('limit') ?? '50')
      const items = [
        {
          id: 1,
          category: 'policy',
          action: 'upsert',
          actor: 'architect',
          target_type: 'policy',
          target_id: '7',
          outcome: 'success',
          created_at: '2026-01-01T00:00:00Z',
        },
      ].slice(0, limit)
      await fulfill(route, 200, { items, count: items.length })
      return
    }

    if (method === 'GET' && path === '/audit/evidence-bundles') {
      await fulfill(route, 200, {
        items: [
          {
            id: 1,
            name: 'release-evidence',
            scope: 'prompt_release',
            status: 'ready',
            created_at: '2026-03-22T10:00:00Z',
            item_count: 3,
          },
        ],
        count: 1,
        limit: 10,
      })
      return
    }

    if (method === 'GET' && path === '/budgets/default/usage') {
      await fulfill(route, 200, {
        tokens_used: 450000,
        cost_used_usd: 320.55,
        period_start: '2026-03-01T00:00:00Z',
        period_end: '2026-03-31T23:59:59Z',
        tokens_pct: 0.45,
        cost_pct: 0.64,
        budget: {
          tenant_id: 'default',
          monthly_tokens: 1000000,
          monthly_cost_usd: 500,
          alert_threshold: 0.8,
          hard_limit: true,
          reset_day: 1,
        },
      })
      return
    }

    if (method === 'GET' && path === '/traces/saved-views') {
      await fulfill(route, 200, savedViews)
      return
    }

    if (method === 'PUT' && path === '/traces/saved-views') {
      const next = {
        id: savedViews.length + 1,
        name: String(body?.name ?? `View ${savedViews.length + 1}`),
        filters: (body?.filters as Record<string, string>) ?? {},
        created_at: '2026-03-22T10:00:00Z',
        updated_at: '2026-03-22T10:00:00Z',
      }
      savedViews = [...savedViews, next]
      await fulfill(route, 200, next)
      return
    }

    if (method === 'DELETE' && path.startsWith('/traces/saved-views/')) {
      const id = Number(path.split('/').pop())
      savedViews = savedViews.filter((item) => item.id !== id)
      await fulfill(route, 204, null)
      return
    }

    if (method === 'GET' && path === '/traces/compare') {
      const left = url.searchParams.get('left') ?? 'trace-left'
      const right = url.searchParams.get('right') ?? 'trace-right'
      await fulfill(route, 200, {
        left: {
          trace_id: left,
          root_span_name: 'assistant.run',
          framework: 'langgraph',
          start_time: '2026-03-22T10:00:00Z',
          duration_ns: 800000000,
          status: 'ok',
          span_count: 4,
          error_count: 0,
          total_cost_usd: 0.0042,
          total_tokens: 1200,
          blocked_spans: 0,
        },
        right: {
          trace_id: right,
          root_span_name: 'assistant.run',
          framework: 'langgraph',
          start_time: '2026-03-22T10:05:00Z',
          duration_ns: 1200000000,
          status: 'partial',
          span_count: 5,
          error_count: 1,
          total_cost_usd: 0.0061,
          total_tokens: 1500,
          blocked_spans: 1,
        },
        highlights: ['candidate trace is slower'],
        diffs: [
          { field: 'status', left: 'ok', right: 'partial', severity: 'high' },
        ],
      })
      return
    }

    if (method === 'GET' && path === '/traces') {
      const search = toLower(url.searchParams.get('search'))
      const statusFilter = toLower(url.searchParams.get('status'))
      let filtered = traces

      if (search) {
        filtered = filtered.filter((item) => {
          return (
            item.id.toLowerCase().includes(search) ||
            item.root_span_name.toLowerCase().includes(search) ||
            item.framework.toLowerCase().includes(search)
          )
        })
      }
      if (statusFilter) {
        filtered = filtered.filter((item) => item.status.toLowerCase() === statusFilter)
      }

      await fulfill(route, 200, {
        items: filtered,
        total: filtered.length,
        has_more: false,
      })
      return
    }

    if (method === 'GET' && path.startsWith('/traces/')) {
      const traceId = path.split('/')[2]
      const trace = traces.find((item) => item.id === traceId)
      if (!trace) {
        await fulfill(route, 404, { error: 'not_found' })
        return
      }
      await fulfill(route, 200, {
        ...trace,
        spans: [],
        timeline: { trace_id: trace.id, start_time: trace.start_time, duration_ns: trace.duration_ns, items: [] },
      })
      return
    }

    if (method === 'GET' && path === '/evals') {
      await fulfill(route, 200, {
        items: [
          {
            id: 1,
            trace_id: 'trace-left',
            release_tag: 'candidate-1',
            eval_suite: 'core-release',
            overall_score: 87.5,
            risk_level: 'low',
            summary: 'good trace',
            created_at: '2026-03-22T10:00:00Z',
            policy_effectiveness: {
              coverage_ratio: 1,
              blocked_spans: 0,
              redacted_spans: 0,
            },
            scores: [
              { metric: 'reliability', score: 90, summary: 'healthy' },
            ],
          },
        ],
        total: 1,
        has_more: false,
      })
      return
    }

    if (method === 'GET' && path === '/keys') {
      await fulfill(route, 200, { items: keys, count: keys.length })
      return
    }

    if (method === 'POST' && path === '/keys') {
      const provider = toLower(body?.provider) || 'openai'
      const displayName = String(body?.display_name ?? 'New key')
      const newKey = `af-vk-${provider}-${keys.length + 1000}`
      const record: VirtualKeyRecord = {
        virtual_key: newKey,
        display_name: displayName,
        provider,
        key_id: `key-${provider}-${keys.length + 1}`,
        revoked: false,
        created_at: '2026-03-22T10:00:00Z',
      }
      keys = [record, ...keys]
      await fulfill(route, 200, { virtual_key: record.virtual_key, provider: record.provider })
      return
    }

    if (method === 'DELETE' && path.startsWith('/keys/')) {
      const virtualKey = decodeURIComponent(path.split('/').pop() ?? '')
      keys = keys.map((item) => (item.virtual_key === virtualKey ? { ...item, revoked: true } : item))
      await fulfill(route, 204, null)
      return
    }

    if (method === 'GET' && path === '/policies') {
      await fulfill(route, 200, { items: policyRules, count: policyRules.length })
      return
    }

    if (method === 'PUT' && path === '/policies') {
      const id = Number(body?.id ?? 0)
      const normalized: PolicyRuleRecord = {
        id: id > 0 ? id : policyRules.length + 100,
        name: String(body?.name ?? 'unnamed'),
        rule_type: toLower(body?.rule_type) === 'dlp' ? 'dlp' : 'traffic',
        action: (toLower(body?.action) as PolicyRuleRecord['action']) || 'deny',
        provider: toLower(body?.provider),
        model_pattern: toLower(body?.model_pattern),
        environment: toLower(body?.environment),
        enabled: body?.enabled !== false,
        priority: Number(body?.priority ?? 100),
        decision_mode: toLower(body?.decision_mode) === 'rego' ? 'rego' : 'fast',
        guardrails: Array.isArray(body?.guardrails) ? body?.guardrails.map((value) => String(value)) : [],
        rollout_percent: Number(body?.rollout_percent ?? 100),
        version: Number(body?.version ?? 1),
      }
      const existing = policyRules.find((item) => item.id === normalized.id)
      if (existing) {
        policyRules = policyRules.map((item) => (item.id === normalized.id ? normalized : item))
      } else {
        policyRules = [normalized, ...policyRules]
      }
      await fulfill(route, 200, normalized)
      return
    }

    if (method === 'DELETE' && path.startsWith('/policies/')) {
      const id = Number(path.split('/').pop())
      policyRules = policyRules.filter((item) => item.id !== id)
      await fulfill(route, 204, null)
      return
    }

    if (method === 'POST' && path === '/policies/preview') {
      await fulfill(route, 200, {
        traffic: {
          matched: true,
          action: 'deny',
          policy_name: 'Block prod secrets',
          reason: 'Matched provider/model/environment and token threshold.',
          engine: 'policy-engine',
          decision_mode: 'rego',
          version: 2,
          rollout_percent: 50,
          final: true,
        },
        request_dlp: {
          matched: false,
          action: 'allow',
        },
        response_dlp: {
          matched: true,
          action: 'redact',
          policy_name: 'Response DLP',
        },
      })
      return
    }

    if (method === 'GET' && path === '/rollouts') {
      await fulfill(route, 200, { items: rollouts, count: rollouts.length })
      return
    }

    if (method === 'PUT' && path === '/rollouts') {
      const requestedId = Number(body?.id ?? 0)
      const record: RolloutRecord = {
        id: requestedId > 0 ? requestedId : rollouts.length + 100,
        name: String(body?.name ?? 'new rollout'),
        target_type: (toLower(body?.target_type) as RolloutRecord['target_type']) || 'policy_rule',
        target_id: String(body?.target_id ?? ''),
        policy_rule_id: Number(body?.policy_rule_id ?? 0) || undefined,
        environment: toLower(body?.environment) || 'production',
        percentage: Number(body?.percentage ?? 10),
        status: toLower(body?.status) === 'paused' ? 'paused' : 'active',
        control_release_tag: String(body?.control_release_tag ?? ''),
        candidate_release_tag: String(body?.candidate_release_tag ?? ''),
        recent_requests: 0,
        recent_error_rate: 0,
      }
      const existing = rollouts.find((item) => item.id === record.id)
      if (existing) {
        rollouts = rollouts.map((item) => (item.id === record.id ? record : item))
      } else {
        rollouts = [record, ...rollouts]
      }
      await fulfill(route, 200, record)
      return
    }

    if (method === 'POST' && path === '/rollouts/preview') {
      await fulfill(route, 200, {
        assignment: {
          selected: true,
          rule_id: 11,
          rule_name: 'Policy canary',
          variant: 'control',
          assignment_key: 'preview-key',
          bucket: 42,
        },
        rules: rollouts,
      })
      return
    }

    if (method === 'POST' && /^\/rollouts\/\d+\/status$/.test(path)) {
      const parts = path.split('/')
      const id = Number(parts[2])
      const nextStatus = toLower(body?.status) === 'paused' ? 'paused' : 'active'
      rollouts = rollouts.map((item) => (item.id === id ? { ...item, status: nextStatus } : item))
      const updated = rollouts.find((item) => item.id === id)
      await fulfill(route, 200, updated ?? {})
      return
    }

    if (method === 'GET' && path === '/recommendations') {
      const type = toLower(url.searchParams.get('type'))
      const status = toLower(url.searchParams.get('status'))
      let filtered = recommendations
      if (type) {
        filtered = filtered.filter((item) => toLower(item.type) === type)
      }
      if (status) {
        filtered = filtered.filter((item) => toLower(item.status) === status)
      }
      await fulfill(route, 200, { items: filtered, total: filtered.length, has_more: false })
      return
    }

    if (method === 'POST' && /^\/recommendations\/\d+\/status$/.test(path)) {
      const parts = path.split('/')
      const id = Number(parts[2])
      const nextStatus = toLower(body?.status) as RecommendationRecord['status']
      recommendations = recommendations.map((item) => (item.id === id ? { ...item, status: nextStatus } : item))
      const updated = recommendations.find((item) => item.id === id)
      await fulfill(route, 200, updated ?? {})
      return
    }

    if (method === 'GET' && path === '/prompts') {
      await fulfill(route, 200, {
        items: promptVersions,
        releases: promptReleases,
        count: promptVersions.length,
      })
      return
    }

    if (method === 'PUT' && path === '/prompts') {
      const nextVersion = Number(body?.version ?? 0) || Math.max(...promptVersions.map((item) => item.version), 0) + 1
      const record: PromptVersionRecord = {
        id: promptVersions.length + 1000,
        prompt_id: String(body?.prompt_id ?? '').trim(),
        version: nextVersion,
        environment: toLower(body?.environment) || 'development',
        release_tag: String(body?.release_tag ?? ''),
        content: String(body?.content ?? ''),
        description: String(body?.description ?? ''),
        promoted: false,
        is_latest: true,
      }

      promptVersions = promptVersions.map((item) => {
        if (item.prompt_id === record.prompt_id && item.environment === record.environment) {
          return { ...item, is_latest: false }
        }
        return item
      })
      promptVersions = [record, ...promptVersions]
      await fulfill(route, 200, record)
      return
    }

    if (method === 'POST' && path === '/prompts/promote') {
      const promptId = String(body?.prompt_id ?? '')
      const version = Number(body?.version ?? 0)
      const environment = toLower(body?.environment) || 'development'
      const releaseTag = String(body?.release_tag ?? '')

      const release: PromptReleaseRecord = {
        id: promptReleases.length + 100,
        prompt_id: promptId,
        environment,
        version,
        release_tag: releaseTag,
        status: toLower(body?.status) || 'active',
        promoted_by: 'admin@example.com',
        created_at: '2026-03-22T10:20:00Z',
        notes: String(body?.notes ?? ''),
        promotion_reason: String(body?.promotion_reason ?? ''),
        eval_summary: {
          eval_count: 4,
          average_score: 89.5,
          latest_score: 90.2,
          risk_level: 'low',
        },
      }

      promptReleases = [release, ...promptReleases]
      promptVersions = promptVersions.map((item) => {
        const match = item.prompt_id === promptId && item.version === version && item.environment === environment
        if (!match) return item
        return {
          ...item,
          promoted: true,
          current_release: release,
        }
      })

      await fulfill(route, 200, release)
      return
    }

    await fulfill(route, 404, {
      error: 'mock_not_found',
      method,
      path,
    })
  })
}
