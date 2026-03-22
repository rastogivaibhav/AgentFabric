import { expect, test } from '@playwright/test'

async function mockApi(page: import('@playwright/test').Page) {
  await page.route('**/auth/me', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        sub: 'admin-1',
        email: 'admin@example.com',
        name: 'Admin User',
        role: 'admin',
        exp: Math.floor(Date.now() / 1000) + 3600,
      }),
    })
  })

  await page.route('**/api/v1/analytics/overview*', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        total_traces: 42,
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
      }),
    })
  })

  await page.route('**/api/v1/analytics/frameworks*', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ crewai: 12, langgraph: 30 }),
    })
  })

  await page.route('**/api/v1/traces/saved-views*', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify([]),
    })
  })

  await page.route('**/api/v1/traces/compare**', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        left: {
          trace_id: 'trace-left',
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
          trace_id: 'trace-right',
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
      }),
    })
  })

  await page.route('**/api/v1/traces**', async (route) => {
    if (route.request().url().includes('/api/v1/traces/compare')) {
      await route.fallback()
      return
    }
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        items: [
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
        ],
        total: 2,
        has_more: false,
      }),
    })
  })

  await page.route('**/api/v1/evals**', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
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
      }),
    })
  })

  await page.route('**/api/v1/prompts**', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        items: [
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
            current_release: {
              id: 91,
              prompt_id: 'support-bot.system',
              environment: 'production',
              version: 3,
              release_tag: '2026.03',
            },
          },
        ],
        releases: [
          {
            id: 91,
            prompt_id: 'support-bot.system',
            environment: 'production',
            version: 3,
            release_tag: '2026.03',
          },
        ],
        count: 1,
      }),
    })
  })
}

test.beforeEach(async ({ page }) => {
  await mockApi(page)
})

test('dashboard and navigation render with mocked admin data', async ({ page }) => {
  await page.goto('/dashboard')
  await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible()
  await expect(page.getByText('1.2345')).toBeVisible()

  await page.goto('/traces')
  await expect(page).toHaveURL(/\/traces$/)

  await page.goto('/evals')
  await expect(page.getByRole('heading', { name: 'Evals' })).toBeVisible()

  await page.goto('/prompts')
  await expect(page.getByText('Prompt Registry')).toBeVisible()
})

test('trace comparison page renders compare output', async ({ page }) => {
  await page.goto('/traces/compare?left=trace-left&right=trace-right')
  await expect(page.getByRole('heading', { name: 'Trace Comparison' })).toBeVisible()
  await expect(page.getByText('candidate trace is slower')).toBeVisible()
  await expect(page.getByText('STATUS', { exact: true })).toBeVisible()
})
