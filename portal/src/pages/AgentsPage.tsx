import { useQuery } from '@tanstack/react-query'
import { useOverview } from '../hooks/api'

const BASE = import.meta.env.VITE_API_URL ?? 'http://localhost:8080'

const FRAMEWORK_COLORS: Record<string, string> = {
  crewai: '#FF6B35',
  langgraph: '#4ECDC4',
  google_adk: '#4285F4',
  openai_agents: '#10A37F',
  claude_agents: '#D97706',
  unknown: '#475569',
}

export default function AgentsPage() {
  const { data: overview } = useOverview('24h')

  const { data: traces, isLoading } = useQuery({
    queryKey: ['traces-agents'],
    queryFn: async () => {
      const res = await fetch(`${BASE}/api/v1/traces?limit=100`)
      if (!res.ok) throw new Error('fetch failed')
      return res.json()
    },
    refetchInterval: 30_000,
  })

  const frameworkCounts = overview?.framework_counts ?? {}
  const totalTraces = overview?.total_traces ?? 0

  return (
    <div style={{ padding: 32 }}>
      <div style={{ marginBottom: 28 }}>
        <h1 style={{ fontSize: 22, fontWeight: 700, color: '#F0F9FF', margin: 0 }}>Agents</h1>
        <p style={{ fontSize: 12, color: '#475569', marginTop: 4 }}>Framework activity · Last 24 hours</p>
      </div>

      {/* Framework cards */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(5,1fr)', gap: 16, marginBottom: 32 }}>
        {Object.entries(FRAMEWORK_COLORS).filter(([k]) => k !== 'unknown').map(([fw, color]) => {
          const count = frameworkCounts[fw] ?? 0
          const pct = totalTraces > 0 ? Math.round((count / Number(totalTraces)) * 100) : 0
          return (
            <div key={fw} style={{
              background: '#0D1B2A', border: `1px solid ${color}30`,
              borderTop: `2px solid ${color}`, borderRadius: 10, padding: '20px 18px'
            }}>
              <div style={{ fontSize: 10, color: '#475569', letterSpacing: '0.1em', marginBottom: 8 }}>
                {fw.replace(/_/g, ' ').toUpperCase()}
              </div>
              <div style={{ fontSize: 32, fontWeight: 700, color, lineHeight: 1, marginBottom: 4 }}>
                {count.toLocaleString()}
              </div>
              <div style={{ fontSize: 11, color: '#334155' }}>traces</div>
              <div style={{ marginTop: 12, height: 4, background: '#0F1F35', borderRadius: 2 }}>
                <div style={{ width: `${pct}%`, height: '100%', background: color, borderRadius: 2 }} />
              </div>
              <div style={{ fontSize: 10, color: '#475569', marginTop: 4 }}>{pct}% of total</div>
            </div>
          )
        })}
      </div>

      {/* Recent traces as agent activity */}
      <div style={{ background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 10 }}>
        <div style={{ padding: '16px 24px', borderBottom: '1px solid #0F1F35', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <span style={{ fontSize: 12, color: '#475569', letterSpacing: '0.1em' }}>RECENT ACTIVITY</span>
          {isLoading && <span style={{ fontSize: 11, color: '#334155' }}>loading…</span>}
        </div>
        <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
          <thead>
            <tr>
              {['Trace ID', 'Framework', 'Root Span', 'Duration', 'Spans', 'Tokens', 'Cost', 'Status'].map(h => (
                <th key={h} style={{
                  padding: '10px 16px', textAlign: 'left', color: '#334155',
                  borderBottom: '1px solid #0F1F35', fontSize: 11, fontWeight: 600, letterSpacing: '0.08em'
                }}>{h}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {((traces?.items ?? []) as any[]).map((t: any, i: number) => (
              <tr key={t.id} style={{ background: i % 2 === 0 ? 'transparent' : '#060A1430' }}>
                <td style={{ padding: '8px 16px', color: '#3B82F6', fontFamily: 'monospace', fontSize: 11 }}>
                  <a href={`/traces/${t.id}`} style={{ color: '#3B82F6', textDecoration: 'none' }}>
                    {t.id.substring(0, 16)}…
                  </a>
                </td>
                <td style={{ padding: '8px 16px' }}>
                  <span style={{
                    padding: '2px 8px', borderRadius: 4, fontSize: 10,
                    background: (FRAMEWORK_COLORS[t.framework] ?? '#475569') + '20',
                    color: FRAMEWORK_COLORS[t.framework] ?? '#475569',
                  }}>{t.framework}</span>
                </td>
                <td style={{ padding: '8px 16px', color: '#94A3B8', maxWidth: 200, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                  {t.root_span_name}
                </td>
                <td style={{ padding: '8px 16px', color: '#94A3B8' }}>{(t.duration_ns / 1_000_000).toFixed(0)}ms</td>
                <td style={{ padding: '8px 16px', color: '#94A3B8' }}>{t.span_count}</td>
                <td style={{ padding: '8px 16px', color: '#94A3B8' }}>{(t.total_tokens ?? 0).toLocaleString()}</td>
                <td style={{ padding: '8px 16px', color: '#F59E0B' }}>${(t.total_cost_usd ?? 0).toFixed(6)}</td>
                <td style={{ padding: '8px 16px' }}>
                  <span style={{
                    padding: '2px 8px', borderRadius: 4, fontSize: 10,
                    background: t.status === 'error' ? '#EF444420' : '#10B98120',
                    color: t.status === 'error' ? '#EF4444' : '#10B981',
                  }}>{t.status}</span>
                </td>
              </tr>
            ))}
            {(traces?.items ?? []).length === 0 && !isLoading && (
              <tr>
                <td colSpan={8} style={{ padding: '32px 16px', textAlign: 'center', color: '#334155', fontSize: 12 }}>
                  No agent activity yet — send spans via OTLP to get started.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  )
}
