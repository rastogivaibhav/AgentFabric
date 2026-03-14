import { useOverview, useFrameworkStats, useTraces } from '../hooks/api'
import { BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, Cell } from 'recharts'
import { useMemo } from 'react'

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

export default function CostPage() {
  const { data: overview, isLoading } = useOverview('24h')
  const { data: fwStats, isLoading: fwLoading } = useFrameworkStats()

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
              const totalCount = Object.values(frameworkCounts).reduce((a, b) => a + b, 0)
              const pct = totalCount > 0 ? Math.round((count / totalCount) * 100) : 0
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
    </div>
  )
}
