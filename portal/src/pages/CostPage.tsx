import { useOverview, useFrameworkStats, useTraces, useBudget, useBudgetUsage, useUpsertBudget, useDeleteBudget, useCostReport } from '../hooks/api'
import { BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, Cell } from 'recharts'
import { useMemo, useState } from 'react'

const FRAMEWORK_COLORS: Record<string, string> = {
  crewai: '#FF6B35',
  langgraph: '#4ECDC4',
  google_adk: '#4285F4',
  openai_agents: '#10A37F',
  claude_agents: '#D97706',
  unknown: '#475569',
}

// Framework stats may or may not carry cost data — treat as partial
interface FrameworkStat {
  framework: string
  trace_count: number
  total_cost_usd?: number
  total_tokens?: number
  input_tokens?: number
  output_tokens?: number
  error_rate?: number
}

// ─── Budget Panel ─────────────────────────────────────────────────────────────

function UsageBar({ label, used, limit, pct, color }: {
  label: string; used: string; limit: string; pct: number; color: string
}) {
  const capped = Math.min(pct, 1)
  const barColor = pct >= 1 ? '#EF4444' : pct >= 0.8 ? '#F59E0B' : color
  return (
    <div style={{ marginBottom: 16 }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 6 }}>
        <span style={{ fontSize: 11, color: '#94A3B8' }}>{label}</span>
        <span style={{ fontSize: 11, color: barColor, fontWeight: 600 }}>
          {used} / {limit} &nbsp;({Math.round(pct * 100)}%)
        </span>
      </div>
      <div style={{ height: 8, background: '#0F1F35', borderRadius: 4 }}>
        <div style={{ width: `${capped * 100}%`, height: '100%', background: barColor, borderRadius: 4, transition: 'width 0.3s' }} />
      </div>
    </div>
  )
}

function BudgetPanel({ tenantId }: { tenantId: string }) {
  const { data: budget, isLoading: budgetLoading } = useBudget(tenantId)
  const { data: usage, isLoading: usageLoading } = useBudgetUsage(tenantId)
  const upsert = useUpsertBudget(tenantId)
  const remove = useDeleteBudget(tenantId)

  const [editing, setEditing] = useState(false)
  const [form, setForm] = useState({ monthly_tokens: 0, monthly_cost_usd: 0, hard_limit: true, alert_threshold: 0.8, reset_day: 1 })

  const openEdit = () => {
    setForm({
      monthly_tokens: budget?.monthly_tokens ?? 0,
      monthly_cost_usd: budget?.monthly_cost_usd ?? 0,
      hard_limit: budget?.hard_limit ?? true,
      alert_threshold: budget?.alert_threshold ?? 0.8,
      reset_day: budget?.reset_day ?? 1,
    })
    setEditing(true)
  }

  const save = () => {
    upsert.mutate(form, { onSuccess: () => setEditing(false) })
  }

  const noBudget = !budget || (budget.monthly_tokens === 0 && budget.monthly_cost_usd === 0)
  const loading = budgetLoading || usageLoading

  const cardStyle: React.CSSProperties = {
    background: '#0D1B2A', border: '1px solid #0F1F35',
    borderTop: '2px solid #3B82F6', borderRadius: 10, padding: 24, marginBottom: 16,
  }
  const inputStyle: React.CSSProperties = {
    background: '#071525', border: '1px solid #1E3A5F', borderRadius: 6,
    color: '#F0F9FF', padding: '6px 10px', fontSize: 12, width: '100%', boxSizing: 'border-box',
  }
  const btnStyle = (color: string): React.CSSProperties => ({
    background: color, border: 'none', borderRadius: 6, color: '#fff',
    padding: '7px 16px', fontSize: 12, cursor: 'pointer', fontWeight: 600,
  })

  return (
    <div style={cardStyle}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
        <div>
          <div style={{ fontSize: 12, color: '#475569', letterSpacing: '0.1em' }}>MONTHLY BUDGET</div>
          <div style={{ fontSize: 10, color: '#334155', marginTop: 2 }}>
            {noBudget ? 'No limit set — unlimited usage' : `Resets on day ${budget.reset_day} · ${budget.hard_limit ? 'Hard limit (blocks at 100%)' : 'Soft limit (alerts only)'}`}
          </div>
        </div>
        <div style={{ display: 'flex', gap: 8 }}>
          {!editing && (
            <>
              <button style={btnStyle('#1E3A5F')} onClick={openEdit}>
                {noBudget ? 'Set Budget' : 'Edit'}
              </button>
              {!noBudget && (
                <button style={btnStyle('#7F1D1D')} onClick={() => remove.mutate()}>
                  Remove
                </button>
              )}
            </>
          )}
        </div>
      </div>

      {/* Usage bars */}
      {!loading && !noBudget && usage && (
        <>
          {budget.monthly_tokens > 0 && (
            <UsageBar
              label="Token Usage"
              used={`${(usage.tokens_used / 1_000).toFixed(1)}K`}
              limit={`${(budget.monthly_tokens / 1_000).toFixed(0)}K`}
              pct={usage.tokens_pct ?? usage.tokens_used / budget.monthly_tokens}
              color="#3B82F6"
            />
          )}
          {budget.monthly_cost_usd > 0 && (
            <UsageBar
              label="Cost Usage"
              used={`$${usage.cost_used_usd.toFixed(4)}`}
              limit={`$${budget.monthly_cost_usd.toFixed(2)}`}
              pct={usage.cost_pct ?? usage.cost_used_usd / budget.monthly_cost_usd}
              color="#10B981"
            />
          )}
          {budget.alert_threshold > 0 && (
            <div style={{ fontSize: 10, color: '#334155', marginTop: -8 }}>
              Alert threshold: {Math.round(budget.alert_threshold * 100)}% · Period: {usage.period_start ? new Date(usage.period_start).toLocaleDateString() : '—'} — {usage.period_end ? new Date(usage.period_end).toLocaleDateString() : '—'}
            </div>
          )}
        </>
      )}

      {!loading && noBudget && !editing && (
        <div style={{ color: '#334155', fontSize: 12, textAlign: 'center', padding: '12px 0' }}>
          No budget set. Click "Set Budget" to define monthly token or cost limits.
        </div>
      )}

      {/* Edit form */}
      {editing && (
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12, marginTop: 8 }}>
          <label style={{ fontSize: 11, color: '#94A3B8' }}>
            Monthly Tokens (0 = unlimited)
            <input style={{ ...inputStyle, marginTop: 4 }} type="number" min={0}
              value={form.monthly_tokens}
              onChange={e => setForm(f => ({ ...f, monthly_tokens: Number(e.target.value) }))} />
          </label>
          <label style={{ fontSize: 11, color: '#94A3B8' }}>
            Monthly Cost USD (0 = unlimited)
            <input style={{ ...inputStyle, marginTop: 4 }} type="number" min={0} step={0.01}
              value={form.monthly_cost_usd}
              onChange={e => setForm(f => ({ ...f, monthly_cost_usd: Number(e.target.value) }))} />
          </label>
          <label style={{ fontSize: 11, color: '#94A3B8' }}>
            Alert Threshold (0–1, default 0.8)
            <input style={{ ...inputStyle, marginTop: 4 }} type="number" min={0} max={1} step={0.05}
              value={form.alert_threshold}
              onChange={e => setForm(f => ({ ...f, alert_threshold: Number(e.target.value) }))} />
          </label>
          <label style={{ fontSize: 11, color: '#94A3B8' }}>
            Reset Day of Month (1–28)
            <input style={{ ...inputStyle, marginTop: 4 }} type="number" min={1} max={28}
              value={form.reset_day}
              onChange={e => setForm(f => ({ ...f, reset_day: Number(e.target.value) }))} />
          </label>
          <label style={{ fontSize: 11, color: '#94A3B8', display: 'flex', alignItems: 'center', gap: 8 }}>
            <input type="checkbox" checked={form.hard_limit}
              onChange={e => setForm(f => ({ ...f, hard_limit: e.target.checked }))} />
            Hard Limit (block ingest at 100% — uncheck for alerts only)
          </label>
          <div style={{ display: 'flex', gap: 8, alignItems: 'flex-end' }}>
            <button style={btnStyle('#1D4ED8')} onClick={save} disabled={upsert.isPending}>
              {upsert.isPending ? 'Saving…' : 'Save'}
            </button>
            <button style={{ ...btnStyle('#1E3A5F') }} onClick={() => setEditing(false)}>Cancel</button>
          </div>
          {upsert.isError && (
            <div style={{ fontSize: 11, color: '#EF4444', gridColumn: '1/-1' }}>
              Save failed — check console.
            </div>
          )}
        </div>
      )}
    </div>
  )
}

export default function CostPage() {
  const { data: overview, isLoading } = useOverview('24h')
  const { data: fwStats, isLoading: fwLoading } = useFrameworkStats()
  const { data: costReport, isLoading: reportLoading } = useCostReport('24h')

  // Fallback: aggregate cost per framework from traces if server doesn't provide it
  // Use a large page to get a reasonable sample; only fetched if fwStats lack cost
  const fwStatsArr: FrameworkStat[] = Array.isArray(fwStats) ? (fwStats as FrameworkStat[]) : []
  const fwHasCost = fwStatsArr.length > 0 && fwStatsArr.some(f => (f.total_cost_usd ?? 0) > 0)

  const { data: tracesPage, isLoading: tracesLoading } = useTraces(
    !fwHasCost ? { limit: '500' } : { limit: '0' }
  )

  // Build cost-by-framework map — prefer server data, fall back to client aggregation
  const fwCostMap: Record<string, number> = useMemo(() => {
    if (fwHasCost) {
      return Object.fromEntries(fwStatsArr.map(f => [f.framework, f.total_cost_usd ?? 0]))
    }
    // Aggregate from traces as fallback
    const map: Record<string, number> = {}
    for (const t of tracesPage?.items ?? []) {
      map[t.framework] = (map[t.framework] ?? 0) + (t.total_cost_usd ?? 0)
    }
    return map
  }, [fwHasCost, fwStatsArr, tracesPage])

  const totalCost = overview?.total_cost_usd ?? 0
  const totalTokens = overview?.total_tokens ?? 0
  const frameworkCounts = overview?.framework_counts ?? {}

  // Try to derive input/output tokens from framework stats
  const totalInputTokens: number | null = useMemo(() => {
    if (fwStatsArr.length > 0 && fwStatsArr.some(f => f.input_tokens != null)) {
      return fwStatsArr.reduce((sum, f) => sum + (f.input_tokens ?? 0), 0)
    }
    return null
  }, [fwStatsArr])

  const totalOutputTokens: number | null = useMemo(() => {
    if (fwStatsArr.length > 0 && fwStatsArr.some(f => f.output_tokens != null)) {
      return fwStatsArr.reduce((sum, f) => sum + (f.output_tokens ?? 0), 0)
    }
    return null
  }, [fwStatsArr])

  // Chart data for traces-by-framework bar chart
  const traceChartData = Object.entries(frameworkCounts).map(([name, count]) => ({
    name: name.replace(/_/g, ' '),
    traces: count,
    fw: name,
  }))

  // Chart data for cost-by-framework bar chart
  const costChartData = Object.entries(fwCostMap)
    .filter(([, cost]) => cost > 0)
    .map(([fw, cost]) => ({
      name: fw.replace(/_/g, ' '),
      cost_usd: cost,
      fw,
    }))
    .sort((a, b) => b.cost_usd - a.cost_usd)

  const totalAggregatedCost = Object.values(fwCostMap).reduce((a, b) => a + b, 0)

  const costDataLoading = !fwHasCost ? tracesLoading : fwLoading
  const traceCountTotal = Object.values(frameworkCounts).reduce((a, b) => a + b, 0)

  return (
    <div style={{ padding: 32 }}>
      <div style={{ marginBottom: 28 }}>
        <h1 style={{ fontSize: 22, fontWeight: 700, color: '#F0F9FF', margin: 0 }}>Cost Report</h1>
        <p style={{ fontSize: 12, color: '#475569', marginTop: 4 }}>Token and cost breakdown · Last 24 hours</p>
      </div>

      {/* Top stats — 5 cards */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(160px, 1fr))', gap: 16, marginBottom: 28 }}>
        {[
          {
            label: 'TOTAL COST (24H)',
            value: isLoading ? '—' : `$${totalCost.toFixed(6)}`,
            color: '#F59E0B',
          },
          {
            label: 'TOTAL TOKENS',
            value: isLoading ? '—' : `${(totalTokens / 1_000_000).toFixed(3)}M`,
            color: '#3B82F6',
          },
          {
            label: 'AVG COST / TRACE',
            value: isLoading || !overview?.total_traces ? '—' : `$${(totalCost / Number(overview.total_traces)).toFixed(6)}`,
            color: '#10B981',
          },
          {
            label: 'INPUT TOKENS',
            value: fwLoading
              ? '—'
              : totalInputTokens != null
                ? (totalInputTokens / 1_000).toFixed(1) + 'K'
                : 'N/A',
            color: '#8B5CF6',
          },
          {
            label: 'OUTPUT TOKENS',
            value: fwLoading
              ? '—'
              : totalOutputTokens != null
                ? (totalOutputTokens / 1_000).toFixed(1) + 'K'
                : 'N/A',
            color: '#EC4899',
          },
        ].map(({ label, value, color }) => (
          <div
            key={label}
            style={{
              background: '#0D1B2A', border: '1px solid #0F1F35',
              borderTop: `2px solid ${color}`, borderRadius: 10, padding: '20px 24px',
            }}
          >
            <div style={{ fontSize: 11, color: '#475569', letterSpacing: '0.1em', marginBottom: 8 }}>{label}</div>
            <div style={{ fontSize: 24, fontWeight: 700, color: '#F0F9FF' }}>{value}</div>
          </div>
        ))}
      </div>

      {/* Row 1: traces by framework + framework share by trace count */}
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16, marginBottom: 16 }}>
        <div style={{ background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 10, padding: 24 }}>
          <div style={{ fontSize: 12, color: '#475569', marginBottom: 16, letterSpacing: '0.1em' }}>TRACES BY FRAMEWORK</div>
          <ResponsiveContainer width="100%" height={220}>
            <BarChart data={traceChartData} layout="vertical">
              <XAxis type="number" tick={{ fontSize: 10, fill: '#475569' }} />
              <YAxis type="category" dataKey="name" tick={{ fontSize: 10, fill: '#94A3B8' }} width={90} />
              <Tooltip contentStyle={{ background: '#0D1B2A', border: '1px solid #1E3A5F', fontSize: 11, borderRadius: 6 }} />
              <Bar dataKey="traces" radius={[0, 4, 4, 0]}>
                {traceChartData.map((entry) => (
                  <Cell key={entry.name} fill={FRAMEWORK_COLORS[entry.fw] ?? '#475569'} />
                ))}
              </Bar>
            </BarChart>
          </ResponsiveContainer>
        </div>

        <div style={{ background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 10, padding: 24 }}>
          <div style={{ fontSize: 12, color: '#475569', marginBottom: 16, letterSpacing: '0.1em' }}>FRAMEWORK SHARE — BY TRACE COUNT</div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
            {Object.entries(frameworkCounts).map(([fw, count]) => {
              const pct = traceCountTotal > 0 ? Math.round((count / traceCountTotal) * 100) : 0
              const color = FRAMEWORK_COLORS[fw] ?? '#475569'
              return (
                <div key={fw}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}>
                    <span style={{ fontSize: 11, color: '#94A3B8' }}>{fw.replace(/_/g, ' ')}</span>
                    <span style={{ fontSize: 11, color }}>
                      {count.toLocaleString()} &nbsp;·&nbsp; {pct}%
                    </span>
                  </div>
                  <div style={{ height: 6, background: '#0F1F35', borderRadius: 3 }}>
                    <div style={{ width: `${pct}%`, height: '100%', background: color, borderRadius: 3 }} />
                  </div>
                </div>
              )
            })}
            {Object.keys(frameworkCounts).length === 0 && (
              <div style={{ color: '#334155', fontSize: 12, textAlign: 'center', padding: 32 }}>
                No data yet — send spans to see cost breakdown.
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Budget Panel */}
      <BudgetPanel tenantId="default" />

      {/* Row 2: cost by framework (bar) + framework share by cost */}
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
        <div style={{ background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 10, padding: 24 }}>
          <div style={{ fontSize: 12, color: '#475569', marginBottom: 4, letterSpacing: '0.1em' }}>COST BY FRAMEWORK</div>
          <div style={{ fontSize: 10, color: '#334155', marginBottom: 16 }}>
            {!fwHasCost && !costDataLoading ? '(aggregated from recent traces — server cost breakdown not available)' : ''}
          </div>
          {costDataLoading ? (
            <div style={{ color: '#334155', fontSize: 12, padding: 32, textAlign: 'center' }}>Loading…</div>
          ) : costChartData.length === 0 ? (
            <div style={{ color: '#334155', fontSize: 12, padding: 32, textAlign: 'center' }}>No cost data available.</div>
          ) : (
            <ResponsiveContainer width="100%" height={220}>
              <BarChart data={costChartData} layout="vertical">
                <XAxis type="number" tick={{ fontSize: 10, fill: '#475569' }} tickFormatter={v => `$${Number(v).toFixed(4)}`} />
                <YAxis type="category" dataKey="name" tick={{ fontSize: 10, fill: '#94A3B8' }} width={90} />
                <Tooltip
                  formatter={(val: number) => [`$${val.toFixed(6)}`, 'Cost']}
                  contentStyle={{ background: '#0D1B2A', border: '1px solid #1E3A5F', fontSize: 11, borderRadius: 6 }}
                />
                <Bar dataKey="cost_usd" radius={[0, 4, 4, 0]}>
                  {costChartData.map((entry) => (
                    <Cell key={entry.fw} fill={FRAMEWORK_COLORS[entry.fw] ?? '#475569'} />
                  ))}
                </Bar>
              </BarChart>
            </ResponsiveContainer>
          )}
        </div>

        <div style={{ background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 10, padding: 24 }}>
          <div style={{ fontSize: 12, color: '#475569', marginBottom: 16, letterSpacing: '0.1em' }}>FRAMEWORK SHARE — BY COST USD</div>
          {costDataLoading ? (
            <div style={{ color: '#334155', fontSize: 12, padding: 32, textAlign: 'center' }}>Loading…</div>
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
              {Object.entries(fwCostMap)
                .filter(([, c]) => c > 0)
                .sort(([, a], [, b]) => b - a)
                .map(([fw, cost]) => {
                  const pct = totalAggregatedCost > 0 ? Math.round((cost / totalAggregatedCost) * 100) : 0
                  const color = FRAMEWORK_COLORS[fw] ?? '#475569'
                  return (
                    <div key={fw}>
                      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}>
                        <span style={{ fontSize: 11, color: '#94A3B8' }}>{fw.replace(/_/g, ' ')}</span>
                        <span style={{ fontSize: 11, color }}>
                          ${cost.toFixed(6)} &nbsp;·&nbsp; {pct}%
                        </span>
                      </div>
                      <div style={{ height: 6, background: '#0F1F35', borderRadius: 3 }}>
                        <div style={{ width: `${pct}%`, height: '100%', background: color, borderRadius: 3 }} />
                      </div>
                    </div>
                  )
                })}
              {Object.values(fwCostMap).every(c => c === 0) && (
                <div style={{ color: '#334155', fontSize: 12, textAlign: 'center', padding: 32 }}>
                  No cost data yet.
                </div>
              )}
            </div>
          )}
        </div>
      </div>

      <div style={{ background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 10, padding: 24, marginTop: 16 }}>
        <div style={{ fontSize: 12, color: '#475569', marginBottom: 6, letterSpacing: '0.1em' }}>GOVERNED COST BREAKDOWN</div>
        <div style={{ fontSize: 10, color: '#334155', marginBottom: 16 }}>
          See who spent what by app, environment, provider, and model, including blocked events.
        </div>
        {reportLoading ? (
          <div style={{ color: '#334155', fontSize: 12, padding: 24, textAlign: 'center' }}>Loading…</div>
        ) : (costReport?.length ?? 0) === 0 ? (
          <div style={{ color: '#334155', fontSize: 12, padding: 24, textAlign: 'center' }}>No governed cost rows yet.</div>
        ) : (
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 11 }}>
            <thead>
              <tr style={{ borderBottom: '1px solid #0F1F35' }}>
                {['App', 'Environment', 'Provider', 'Model', 'Trace Count', 'Blocked', 'Cost'].map(h => (
                  <th key={h} style={{ padding: '9px 12px', textAlign: 'left', color: '#334155', fontSize: 10, letterSpacing: '0.08em', fontWeight: 700 }}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {(costReport ?? []).slice(0, 12).map((row, i) => (
                <tr key={`${row.app_name}-${row.environment}-${row.provider}-${row.model}-${i}`} style={{ borderBottom: '1px solid #0A1020', background: i % 2 === 0 ? 'transparent' : '#060A1430' }}>
                  <td style={{ padding: '8px 12px', color: '#E2E8F0' }}>{row.app_name}</td>
                  <td style={{ padding: '8px 12px', color: '#94A3B8' }}>{row.environment}</td>
                  <td style={{ padding: '8px 12px', color: '#60A5FA' }}>{row.provider}</td>
                  <td style={{ padding: '8px 12px', color: '#CBD5E1' }}>{row.model}</td>
                  <td style={{ padding: '8px 12px', color: '#94A3B8' }}>{row.trace_count}</td>
                  <td style={{ padding: '8px 12px', color: row.blocked_count > 0 ? '#EF4444' : '#10B981' }}>{row.blocked_count}</td>
                  <td style={{ padding: '8px 12px', color: '#F59E0B' }}>${row.total_cost_usd.toFixed(6)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}
