import { useOverview } from '../hooks/api'
import { BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, Cell } from 'recharts'

const FRAMEWORK_COLORS: Record<string, string> = {
  crewai: '#FF6B35',
  langgraph: '#4ECDC4',
  google_adk: '#4285F4',
  openai_agents: '#10A37F',
  claude_agents: '#D97706',
  unknown: '#475569',
}

export default function CostPage() {
  const { data: overview, isLoading } = useOverview('24h')

  const totalCost = overview?.total_cost_usd ?? 0
  const totalTokens = overview?.total_tokens ?? 0
  const frameworkCounts = overview?.framework_counts ?? {}

  const chartData = Object.entries(frameworkCounts).map(([name, count]) => ({
    name: name.replace(/_/g, ' '),
    traces: count,
    fw: name,
  }))

  return (
    <div style={{ padding: 32 }}>
      <div style={{ marginBottom: 28 }}>
        <h1 style={{ fontSize: 22, fontWeight: 700, color: '#F0F9FF', margin: 0 }}>Cost Report</h1>
        <p style={{ fontSize: 12, color: '#475569', marginTop: 4 }}>Token and cost breakdown · Last 24 hours</p>
      </div>

      {/* Top stats */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3,1fr)', gap: 16, marginBottom: 28 }}>
        {[
          { label: 'TOTAL COST (24H)', value: isLoading ? '—' : `$${totalCost.toFixed(6)}`, color: '#F59E0B' },
          { label: 'TOTAL TOKENS', value: isLoading ? '—' : `${(totalTokens / 1_000_000).toFixed(3)}M`, color: '#3B82F6' },
          { label: 'AVG COST / TRACE', value: isLoading || !overview?.total_traces ? '—' : `$${(totalCost / Number(overview.total_traces)).toFixed(6)}`, color: '#10B981' },
        ].map(({ label, value, color }) => (
          <div key={label} style={{
            background: '#0D1B2A', border: '1px solid #0F1F35',
            borderTop: `2px solid ${color}`, borderRadius: 10, padding: '20px 24px'
          }}>
            <div style={{ fontSize: 11, color: '#475569', letterSpacing: '0.1em', marginBottom: 8 }}>{label}</div>
            <div style={{ fontSize: 28, fontWeight: 700, color: '#F0F9FF' }}>{value}</div>
          </div>
        ))}
      </div>

      {/* Framework distribution */}
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
        <div style={{ background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 10, padding: 24 }}>
          <div style={{ fontSize: 12, color: '#475569', marginBottom: 16, letterSpacing: '0.1em' }}>TRACES BY FRAMEWORK</div>
          <ResponsiveContainer width="100%" height={220}>
            <BarChart data={chartData} layout="vertical">
              <XAxis type="number" tick={{ fontSize: 10, fill: '#475569' }} />
              <YAxis type="category" dataKey="name" tick={{ fontSize: 10, fill: '#94A3B8' }} width={90} />
              <Tooltip contentStyle={{ background: '#0D1B2A', border: '1px solid #1E3A5F', fontSize: 11, borderRadius: 6 }} />
              <Bar dataKey="traces" radius={[0, 4, 4, 0]}>
                {chartData.map((entry) => (
                  <Cell key={entry.name} fill={FRAMEWORK_COLORS[entry.fw] ?? '#475569'} />
                ))}
              </Bar>
            </BarChart>
          </ResponsiveContainer>
        </div>

        <div style={{ background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 10, padding: 24 }}>
          <div style={{ fontSize: 12, color: '#475569', marginBottom: 16, letterSpacing: '0.1em' }}>FRAMEWORK SHARE</div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
            {Object.entries(frameworkCounts).map(([fw, count]) => {
              const total = Object.values(frameworkCounts).reduce((a, b) => a + b, 0)
              const pct = total > 0 ? Math.round((count / total) * 100) : 0
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
    </div>
  )
}
