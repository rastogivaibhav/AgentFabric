import { BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, Cell } from 'recharts'
import { useMemo, useState } from 'react'

import CostBreakdownTable from '../components/cost/CostBreakdownTable'
import CostRuleMatchPanel from '../components/cost/CostRuleMatchPanel'
import {
  useOverview,
  useFrameworkStats,
  useTraces,
  useBudget,
  useBudgetUsage,
  useUpsertBudget,
  useDeleteBudget,
  useCostReport,
  useCostSpikes,
  usePreviewPricingRule,
  type CostContributorGroup,
  type CostReportFilters,
} from '../hooks/api'

const FRAMEWORK_COLORS: Record<string, string> = {
  crewai: '#FF6B35',
  langgraph: '#4ECDC4',
  google_adk: '#4285F4',
  openai_agents: '#10A37F',
  claude_agents: '#D97706',
  unknown: '#475569',
}

const dimensionLabels: Record<string, string> = {
  app_name: 'App',
  environment: 'Environment',
  provider: 'Provider',
  model: 'Model',
  prompt_id: 'Prompt',
  release_tag: 'Release',
}

interface FrameworkStat {
  framework: string
  trace_count: number
  total_cost_usd?: number
  total_tokens?: number
  input_tokens?: number
  output_tokens?: number
  error_rate?: number
}

function formatMoney(value?: number) {
  return `$${(value ?? 0).toFixed(6)}`
}

function formatPct(value?: number) {
  return `${Math.round(value ?? 0)}%`
}

function UsageBar({ label, used, limit, pct, color }: {
  label: string
  used: string
  limit: string
  pct: number
  color: string
}) {
  const capped = Math.min(pct, 1)
  const barColor = pct >= 1 ? '#EF4444' : pct >= 0.8 ? '#F59E0B' : color
  return (
    <div style={{ marginBottom: 16 }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 6 }}>
        <span style={{ fontSize: 11, color: '#94A3B8' }}>{label}</span>
        <span style={{ fontSize: 11, color: barColor, fontWeight: 600 }}>
          {used} / {limit} ({Math.round(pct * 100)}%)
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
    background: '#0D1B2A',
    border: '1px solid #0F1F35',
    borderTop: '2px solid #3B82F6',
    borderRadius: 10,
    padding: 24,
    marginBottom: 16,
  }
  const inputStyle: React.CSSProperties = {
    background: '#071525',
    border: '1px solid #1E3A5F',
    borderRadius: 6,
    color: '#F0F9FF',
    padding: '6px 10px',
    fontSize: 12,
    width: '100%',
    boxSizing: 'border-box',
  }
  const buttonStyle = (color: string): React.CSSProperties => ({
    background: color,
    border: 'none',
    borderRadius: 6,
    color: '#fff',
    padding: '7px 16px',
    fontSize: 12,
    cursor: 'pointer',
    fontWeight: 600,
  })

  return (
    <div style={cardStyle}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
        <div>
          <div style={{ fontSize: 12, color: '#475569', letterSpacing: '0.1em' }}>MONTHLY BUDGET</div>
          <div style={{ fontSize: 10, color: '#334155', marginTop: 2 }}>
            {noBudget ? 'No limit set - unlimited usage' : `Resets on day ${budget.reset_day} | ${budget.hard_limit ? 'Hard limit (blocks at 100%)' : 'Soft limit (alerts only)'}`}
          </div>
        </div>
        {!editing && (
          <div style={{ display: 'flex', gap: 8 }}>
            <button style={buttonStyle('#1E3A5F')} onClick={openEdit}>
              {noBudget ? 'Set Budget' : 'Edit'}
            </button>
            {!noBudget && (
              <button style={buttonStyle('#7F1D1D')} onClick={() => remove.mutate()}>
                Remove
              </button>
            )}
          </div>
        )}
      </div>

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
              Alert threshold: {Math.round(budget.alert_threshold * 100)}% | Period: {usage.period_start ? new Date(usage.period_start).toLocaleDateString() : '-'} to {usage.period_end ? new Date(usage.period_end).toLocaleDateString() : '-'}
            </div>
          )}
        </>
      )}

      {!loading && noBudget && !editing && (
        <div style={{ color: '#334155', fontSize: 12, textAlign: 'center', padding: '12px 0' }}>
          No budget set. Click "Set Budget" to define monthly token or cost limits.
        </div>
      )}

      {editing && (
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12, marginTop: 8 }}>
          <label style={{ fontSize: 11, color: '#94A3B8' }}>
            Monthly Tokens (0 = unlimited)
            <input style={{ ...inputStyle, marginTop: 4 }} type="number" min={0} value={form.monthly_tokens} onChange={e => setForm(f => ({ ...f, monthly_tokens: Number(e.target.value) }))} />
          </label>
          <label style={{ fontSize: 11, color: '#94A3B8' }}>
            Monthly Cost USD (0 = unlimited)
            <input style={{ ...inputStyle, marginTop: 4 }} type="number" min={0} step={0.01} value={form.monthly_cost_usd} onChange={e => setForm(f => ({ ...f, monthly_cost_usd: Number(e.target.value) }))} />
          </label>
          <label style={{ fontSize: 11, color: '#94A3B8' }}>
            Alert Threshold (0-1)
            <input style={{ ...inputStyle, marginTop: 4 }} type="number" min={0} max={1} step={0.05} value={form.alert_threshold} onChange={e => setForm(f => ({ ...f, alert_threshold: Number(e.target.value) }))} />
          </label>
          <label style={{ fontSize: 11, color: '#94A3B8' }}>
            Reset Day of Month (1-28)
            <input style={{ ...inputStyle, marginTop: 4 }} type="number" min={1} max={28} value={form.reset_day} onChange={e => setForm(f => ({ ...f, reset_day: Number(e.target.value) }))} />
          </label>
          <label style={{ fontSize: 11, color: '#94A3B8', display: 'flex', alignItems: 'center', gap: 8 }}>
            <input type="checkbox" checked={form.hard_limit} onChange={e => setForm(f => ({ ...f, hard_limit: e.target.checked }))} />
            Hard Limit (block ingest at 100%)
          </label>
          <div style={{ display: 'flex', gap: 8, alignItems: 'flex-end' }}>
            <button style={buttonStyle('#1D4ED8')} onClick={save} disabled={upsert.isPending}>
              {upsert.isPending ? 'Saving...' : 'Save'}
            </button>
            <button style={buttonStyle('#1E3A5F')} onClick={() => setEditing(false)}>Cancel</button>
          </div>
          {upsert.isError && (
            <div style={{ fontSize: 11, color: '#EF4444', gridColumn: '1 / -1' }}>
              Save failed. Check console.
            </div>
          )}
        </div>
      )}
    </div>
  )
}

function FilterInput({ label, value, onChange }: { label: string; value: string; onChange: (value: string) => void }) {
  return (
    <label style={{ fontSize: 11, color: '#94A3B8' }}>
      {label}
      <input
        value={value}
        onChange={e => onChange(e.target.value)}
        style={{ background: '#071525', border: '1px solid #1E3A5F', borderRadius: 6, color: '#F0F9FF', padding: '6px 10px', fontSize: 12, width: '100%', boxSizing: 'border-box', marginTop: 4 }}
      />
    </label>
  )
}

function ContributorTable({ group }: { group: CostContributorGroup }) {
  return (
    <div style={{ background: '#071525', border: '1px solid #10243B', borderRadius: 8, padding: 16 }}>
      <div style={{ fontSize: 11, color: '#475569', letterSpacing: '0.08em', marginBottom: 10 }}>
        TOP CONTRIBUTORS BY {dimensionLabels[group.dimension]?.toUpperCase() ?? group.dimension.toUpperCase()}
      </div>
      {group.items.length === 0 ? (
        <div style={{ color: '#334155', fontSize: 11 }}>No positive deltas for this dimension.</div>
      ) : (
        <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 11 }}>
          <thead>
            <tr style={{ borderBottom: '1px solid #0F1F35' }}>
              <th style={{ padding: '8px 0', textAlign: 'left', color: '#334155' }}>Value</th>
              <th style={{ padding: '8px 0', textAlign: 'right', color: '#334155' }}>Current</th>
              <th style={{ padding: '8px 0', textAlign: 'right', color: '#334155' }}>Previous</th>
              <th style={{ padding: '8px 0', textAlign: 'right', color: '#334155' }}>Delta</th>
              <th style={{ padding: '8px 0', textAlign: 'right', color: '#334155' }}>Share</th>
            </tr>
          </thead>
          <tbody>
            {group.items.map(item => (
              <tr key={`${group.dimension}-${item.key}`} style={{ borderBottom: '1px solid #0A1020' }}>
                <td style={{ padding: '8px 0', color: '#E2E8F0' }}>{item.key}</td>
                <td style={{ padding: '8px 0', textAlign: 'right', color: '#94A3B8' }}>{formatMoney(item.current_cost_usd)}</td>
                <td style={{ padding: '8px 0', textAlign: 'right', color: '#64748B' }}>{formatMoney(item.previous_cost_usd)}</td>
                <td style={{ padding: '8px 0', textAlign: 'right', color: '#F59E0B' }}>{formatMoney(item.delta_cost_usd)}</td>
                <td style={{ padding: '8px 0', textAlign: 'right', color: '#60A5FA' }}>{formatPct(item.share_of_delta * 100)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}

export default function CostPage() {
  const { data: overview, isLoading } = useOverview('24h')
  const { data: fwStats, isLoading: fwLoading } = useFrameworkStats()
  const [filters, setFilters] = useState<CostReportFilters>({
    since: '24h',
    app_name: '',
    environment: '',
    provider: '',
    model: '',
    prompt_id: '',
    release_tag: '',
    limit: '12',
  })
  const { data: costReport, isLoading: reportLoading } = useCostReport(filters)
  const { data: spikeReport, isLoading: spikeLoading, isError: spikeError } = useCostSpikes(filters)
  const previewPricing = usePreviewPricingRule()
  const [previewForm, setPreviewForm] = useState({
    provider: 'openai',
    model: 'gpt-4o',
    input_tokens: 1000,
    output_tokens: 500,
    cache_read_tokens: 0,
    cache_write_tokens: 0,
    reasoning_tokens: 0,
  })

  const fwStatsArr: FrameworkStat[] = Array.isArray(fwStats) ? (fwStats as FrameworkStat[]) : []
  const fwHasCost = fwStatsArr.length > 0 && fwStatsArr.some(f => (f.total_cost_usd ?? 0) > 0)
  const { data: tracesPage, isLoading: tracesLoading } = useTraces(!fwHasCost ? { limit: '500' } : { limit: '0' })

  const fwCostMap: Record<string, number> = useMemo(() => {
    if (fwHasCost) {
      return Object.fromEntries(fwStatsArr.map(f => [f.framework, f.total_cost_usd ?? 0]))
    }
    const map: Record<string, number> = {}
    for (const trace of tracesPage?.items ?? []) {
      map[trace.framework] = (map[trace.framework] ?? 0) + (trace.total_cost_usd ?? 0)
    }
    return map
  }, [fwHasCost, fwStatsArr, tracesPage])

  const totalCost = overview?.total_cost_usd ?? 0
  const totalTokens = overview?.total_tokens ?? 0
  const frameworkCounts = overview?.framework_counts ?? {}

  const totalInputTokens = useMemo(() => {
    if (fwStatsArr.length > 0 && fwStatsArr.some(f => f.input_tokens != null)) {
      return fwStatsArr.reduce((sum, stat) => sum + (stat.input_tokens ?? 0), 0)
    }
    return null
  }, [fwStatsArr])

  const totalOutputTokens = useMemo(() => {
    if (fwStatsArr.length > 0 && fwStatsArr.some(f => f.output_tokens != null)) {
      return fwStatsArr.reduce((sum, stat) => sum + (stat.output_tokens ?? 0), 0)
    }
    return null
  }, [fwStatsArr])

  const traceChartData = Object.entries(frameworkCounts).map(([name, count]) => ({
    name: name.replace(/_/g, ' '),
    traces: count,
    fw: name,
  }))

  const costChartData = Object.entries(fwCostMap)
    .filter(([, cost]) => cost > 0)
    .map(([fw, cost]) => ({
      name: fw.replace(/_/g, ' '),
      cost_usd: cost,
      fw,
    }))
    .sort((a, b) => b.cost_usd - a.cost_usd)

  const totalAggregatedCost = Object.values(fwCostMap).reduce((sum, value) => sum + value, 0)
  const costDataLoading = !fwHasCost ? tracesLoading : fwLoading
  const traceCountTotal = Object.values(frameworkCounts).reduce((sum, value) => sum + value, 0)

  return (
    <div style={{ padding: 32 }}>
      <div style={{ marginBottom: 28 }}>
        <h1 style={{ fontSize: 22, fontWeight: 700, color: '#F0F9FF', margin: 0 }}>Cost Report</h1>
        <p style={{ fontSize: 12, color: '#475569', marginTop: 4 }}>Token and cost breakdown | Last 24 hours</p>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(160px, 1fr))', gap: 16, marginBottom: 28 }}>
        {[
          { label: 'TOTAL COST (24H)', value: isLoading ? '-' : `$${totalCost.toFixed(6)}`, color: '#F59E0B' },
          { label: 'TOTAL TOKENS', value: isLoading ? '-' : `${(totalTokens / 1_000_000).toFixed(3)}M`, color: '#3B82F6' },
          { label: 'AVG COST / TRACE', value: isLoading || !overview?.total_traces ? '-' : `$${(totalCost / Number(overview.total_traces)).toFixed(6)}`, color: '#10B981' },
          { label: 'INPUT TOKENS', value: fwLoading ? '-' : totalInputTokens != null ? `${(totalInputTokens / 1_000).toFixed(1)}K` : 'N/A', color: '#8B5CF6' },
          { label: 'OUTPUT TOKENS', value: fwLoading ? '-' : totalOutputTokens != null ? `${(totalOutputTokens / 1_000).toFixed(1)}K` : 'N/A', color: '#EC4899' },
        ].map(card => (
          <div key={card.label} style={{ background: '#0D1B2A', border: '1px solid #0F1F35', borderTop: `2px solid ${card.color}`, borderRadius: 10, padding: '20px 24px' }}>
            <div style={{ fontSize: 11, color: '#475569', letterSpacing: '0.1em', marginBottom: 8 }}>{card.label}</div>
            <div style={{ fontSize: 24, fontWeight: 700, color: '#F0F9FF' }}>{card.value}</div>
          </div>
        ))}
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16, marginBottom: 16 }}>
        <div style={{ background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 10, padding: 24 }}>
          <div style={{ fontSize: 12, color: '#475569', marginBottom: 16, letterSpacing: '0.1em' }}>TRACES BY FRAMEWORK</div>
          <ResponsiveContainer width="100%" height={220}>
            <BarChart data={traceChartData} layout="vertical">
              <XAxis type="number" tick={{ fontSize: 10, fill: '#475569' }} />
              <YAxis type="category" dataKey="name" tick={{ fontSize: 10, fill: '#94A3B8' }} width={90} />
              <Tooltip contentStyle={{ background: '#0D1B2A', border: '1px solid #1E3A5F', fontSize: 11, borderRadius: 6 }} />
              <Bar dataKey="traces" radius={[0, 4, 4, 0]}>
                {traceChartData.map(entry => (
                  <Cell key={entry.name} fill={FRAMEWORK_COLORS[entry.fw] ?? '#475569'} />
                ))}
              </Bar>
            </BarChart>
          </ResponsiveContainer>
        </div>

        <div style={{ background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 10, padding: 24 }}>
          <div style={{ fontSize: 12, color: '#475569', marginBottom: 16, letterSpacing: '0.1em' }}>FRAMEWORK SHARE - BY TRACE COUNT</div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
            {Object.entries(frameworkCounts).map(([fw, count]) => {
              const pct = traceCountTotal > 0 ? Math.round((count / traceCountTotal) * 100) : 0
              const color = FRAMEWORK_COLORS[fw] ?? '#475569'
              return (
                <div key={fw}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}>
                    <span style={{ fontSize: 11, color: '#94A3B8' }}>{fw.replace(/_/g, ' ')}</span>
                    <span style={{ fontSize: 11, color }}>{count.toLocaleString()} | {pct}%</span>
                  </div>
                  <div style={{ height: 6, background: '#0F1F35', borderRadius: 3 }}>
                    <div style={{ width: `${pct}%`, height: '100%', background: color, borderRadius: 3 }} />
                  </div>
                </div>
              )
            })}
            {Object.keys(frameworkCounts).length === 0 && (
              <div style={{ color: '#334155', fontSize: 12, textAlign: 'center', padding: 32 }}>
                No data yet - send spans to see cost breakdown.
              </div>
            )}
          </div>
        </div>
      </div>

      <BudgetPanel tenantId="default" />

      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
        <div style={{ background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 10, padding: 24 }}>
          <div style={{ fontSize: 12, color: '#475569', marginBottom: 4, letterSpacing: '0.1em' }}>COST BY FRAMEWORK</div>
          <div style={{ fontSize: 10, color: '#334155', marginBottom: 16 }}>
            {!fwHasCost && !costDataLoading ? '(aggregated from recent traces - server cost breakdown not available)' : ''}
          </div>
          {costDataLoading ? (
            <div style={{ color: '#334155', fontSize: 12, padding: 32, textAlign: 'center' }}>Loading...</div>
          ) : costChartData.length === 0 ? (
            <div style={{ color: '#334155', fontSize: 12, padding: 32, textAlign: 'center' }}>No cost data available.</div>
          ) : (
            <ResponsiveContainer width="100%" height={220}>
              <BarChart data={costChartData} layout="vertical">
                <XAxis type="number" tick={{ fontSize: 10, fill: '#475569' }} tickFormatter={value => `$${Number(value).toFixed(4)}`} />
                <YAxis type="category" dataKey="name" tick={{ fontSize: 10, fill: '#94A3B8' }} width={90} />
                <Tooltip formatter={(value: number) => [`$${value.toFixed(6)}`, 'Cost']} contentStyle={{ background: '#0D1B2A', border: '1px solid #1E3A5F', fontSize: 11, borderRadius: 6 }} />
                <Bar dataKey="cost_usd" radius={[0, 4, 4, 0]}>
                  {costChartData.map(entry => (
                    <Cell key={entry.fw} fill={FRAMEWORK_COLORS[entry.fw] ?? '#475569'} />
                  ))}
                </Bar>
              </BarChart>
            </ResponsiveContainer>
          )}
        </div>

        <div style={{ background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 10, padding: 24 }}>
          <div style={{ fontSize: 12, color: '#475569', marginBottom: 16, letterSpacing: '0.1em' }}>FRAMEWORK SHARE - BY COST</div>
          {costDataLoading ? (
            <div style={{ color: '#334155', fontSize: 12, padding: 32, textAlign: 'center' }}>Loading...</div>
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
              {Object.entries(fwCostMap)
                .filter(([, cost]) => cost > 0)
                .sort(([, left], [, right]) => right - left)
                .map(([fw, cost]) => {
                  const pct = totalAggregatedCost > 0 ? Math.round((cost / totalAggregatedCost) * 100) : 0
                  const color = FRAMEWORK_COLORS[fw] ?? '#475569'
                  return (
                    <div key={fw}>
                      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}>
                        <span style={{ fontSize: 11, color: '#94A3B8' }}>{fw.replace(/_/g, ' ')}</span>
                        <span style={{ fontSize: 11, color }}>{formatMoney(cost)} | {pct}%</span>
                      </div>
                      <div style={{ height: 6, background: '#0F1F35', borderRadius: 3 }}>
                        <div style={{ width: `${pct}%`, height: '100%', background: color, borderRadius: 3 }} />
                      </div>
                    </div>
                  )
                })}
              {Object.values(fwCostMap).every(value => value === 0) && (
                <div style={{ color: '#334155', fontSize: 12, textAlign: 'center', padding: 32 }}>
                  No cost data yet.
                </div>
              )}
            </div>
          )}
        </div>
      </div>

      <div style={{ background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 10, padding: 24, marginTop: 16 }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', gap: 16, alignItems: 'flex-start', marginBottom: 16 }}>
          <div>
            <div style={{ fontSize: 12, color: '#475569', marginBottom: 6, letterSpacing: '0.1em' }}>COST SPIKE DIAGNOSIS</div>
            <div style={{ fontSize: 10, color: '#334155' }}>
              Explain why spend changed across tenant, app, environment, provider, model, prompt, and release.
            </div>
          </div>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, minmax(120px, 1fr))', gap: 12, flex: 1 }}>
            <label style={{ fontSize: 11, color: '#94A3B8' }}>
              Window
              <select
                aria-label="Window"
                value={filters.since ?? '24h'}
                onChange={e => setFilters(current => ({ ...current, since: e.target.value }))}
                style={{ background: '#071525', border: '1px solid #1E3A5F', borderRadius: 6, color: '#F0F9FF', padding: '6px 10px', fontSize: 12, width: '100%', boxSizing: 'border-box', marginTop: 4 }}
              >
                <option value="6h">Last 6h</option>
                <option value="24h">Last 24h</option>
                <option value="168h">Last 7d</option>
              </select>
            </label>
            <FilterInput label="App" value={filters.app_name ?? ''} onChange={value => setFilters(current => ({ ...current, app_name: value }))} />
            <FilterInput label="Environment" value={filters.environment ?? ''} onChange={value => setFilters(current => ({ ...current, environment: value }))} />
            <FilterInput label="Provider" value={filters.provider ?? ''} onChange={value => setFilters(current => ({ ...current, provider: value }))} />
            <FilterInput label="Model" value={filters.model ?? ''} onChange={value => setFilters(current => ({ ...current, model: value }))} />
            <FilterInput label="Prompt" value={filters.prompt_id ?? ''} onChange={value => setFilters(current => ({ ...current, prompt_id: value }))} />
            <FilterInput label="Release" value={filters.release_tag ?? ''} onChange={value => setFilters(current => ({ ...current, release_tag: value }))} />
          </div>
        </div>

        {spikeLoading ? (
          <div style={{ color: '#334155', fontSize: 12, padding: 24, textAlign: 'center' }}>Loading spike analysis...</div>
        ) : spikeError ? (
          <div style={{ color: '#FCA5A5', fontSize: 12, padding: 24, textAlign: 'center' }}>Spike analysis failed. Check the backend cost analytics endpoint.</div>
        ) : spikeReport && spikeReport.spikes.length > 0 ? (
          <>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(260px, 1fr))', gap: 16, marginBottom: 16 }}>
              {spikeReport.spikes.slice(0, 3).map(spike => (
                <div key={`${spike.app_name}-${spike.environment}-${spike.model}-${spike.release_tag}`} style={{ background: '#071525', border: '1px solid #10243B', borderRadius: 8, padding: 16 }}>
                  <div style={{ fontSize: 11, color: '#475569', letterSpacing: '0.08em', marginBottom: 8 }}>TOP SPIKE</div>
                  <div style={{ fontSize: 16, fontWeight: 700, color: '#F8FAFC', marginBottom: 6 }}>{formatMoney(spike.delta_cost_usd)}</div>
                  <div style={{ fontSize: 11, color: '#F59E0B', marginBottom: 10 }}>{spike.model} | {spike.release_tag}</div>
                  <div style={{ fontSize: 11, color: '#94A3B8', marginBottom: 8 }}>{spike.explanation}</div>
                  <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 8, fontSize: 11 }}>
                    <div style={{ color: '#64748B' }}>Current: <span style={{ color: '#E2E8F0' }}>{formatMoney(spike.current_cost_usd)}</span></div>
                    <div style={{ color: '#64748B' }}>Previous: <span style={{ color: '#E2E8F0' }}>{formatMoney(spike.previous_cost_usd)}</span></div>
                    <div style={{ color: '#64748B' }}>Traces: <span style={{ color: '#E2E8F0' }}>{spike.current_trace_count}</span></div>
                    <div style={{ color: '#64748B' }}>Growth: <span style={{ color: '#E2E8F0' }}>{spike.delta_pct.toFixed(1)}%</span></div>
                  </div>
                </div>
              ))}
            </div>

            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(320px, 1fr))', gap: 16 }}>
              {spikeReport.contributor_groups.map(group => (
                <ContributorTable key={group.dimension} group={group} />
              ))}
            </div>
          </>
        ) : (
          <div style={{ color: '#334155', fontSize: 12, padding: 24, textAlign: 'center' }}>
            No cost spike detected for the selected filters.
          </div>
        )}
      </div>

      <div style={{ background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 10, padding: 24, marginTop: 16 }}>
        <div style={{ fontSize: 12, color: '#475569', marginBottom: 6, letterSpacing: '0.1em' }}>GOVERNED COST BREAKDOWN</div>
        <div style={{ fontSize: 10, color: '#334155', marginBottom: 16 }}>
          See who spent what by app, environment, provider, model, prompt, and release, including blocked events.
        </div>
        {reportLoading ? (
          <div style={{ color: '#334155', fontSize: 12, padding: 24, textAlign: 'center' }}>Loading...</div>
        ) : (costReport?.length ?? 0) === 0 ? (
          <div style={{ color: '#334155', fontSize: 12, padding: 24, textAlign: 'center' }}>No governed cost rows yet.</div>
        ) : (
          <CostBreakdownTable rows={costReport ?? []} />
        )}
      </div>

      <div style={{ background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 10, padding: 24, marginTop: 16 }}>
        <div style={{ fontSize: 12, color: '#475569', marginBottom: 6, letterSpacing: '0.1em' }}>PRICING RULE MATCH</div>
        <div style={{ fontSize: 10, color: '#334155', marginBottom: 16 }}>
          Preview the exact rule and category breakdown that will be applied to a request.
        </div>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(140px, 1fr))', gap: 12, marginBottom: 16 }}>
          {[
            ['provider', 'Provider'],
            ['model', 'Model'],
            ['input_tokens', 'Input Tokens'],
            ['output_tokens', 'Output Tokens'],
            ['cache_read_tokens', 'Cache Read'],
            ['cache_write_tokens', 'Cache Write'],
            ['reasoning_tokens', 'Reasoning'],
          ].map(([key, label]) => (
            <label key={key} style={{ fontSize: 11, color: '#94A3B8' }}>
              {label}
              <input
                style={{ background: '#071525', border: '1px solid #1E3A5F', borderRadius: 6, color: '#F0F9FF', padding: '6px 10px', fontSize: 12, width: '100%', boxSizing: 'border-box', marginTop: 4 }}
                value={String(previewForm[key as keyof typeof previewForm])}
                onChange={e => setPreviewForm(current => ({
                  ...current,
                  [key]: key === 'provider' || key === 'model' ? e.target.value : Number(e.target.value),
                }))}
              />
            </label>
          ))}
        </div>
        <div style={{ display: 'flex', gap: 10, marginBottom: 16 }}>
          <button
            style={{ background: '#1D4ED8', border: 'none', borderRadius: 6, color: '#fff', padding: '8px 16px', fontSize: 12, cursor: 'pointer', fontWeight: 600 }}
            onClick={() => previewPricing.mutate(previewForm)}
            disabled={previewPricing.isPending}
          >
            {previewPricing.isPending ? 'Previewing...' : 'Preview Pricing'}
          </button>
        </div>
        <CostRuleMatchPanel preview={previewPricing.data} />
      </div>
    </div>
  )
}
