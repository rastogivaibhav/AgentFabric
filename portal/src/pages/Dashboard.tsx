import { outcomeStatusColor, useOverview, useTraces } from '../hooks/api'
import { BarChart, Bar, PieChart, Pie, Cell, XAxis, YAxis, Tooltip, ResponsiveContainer } from 'recharts'

const FRAMEWORK_COLORS: Record<string, string> = {
  crewai: '#FF6B35',
  langgraph: '#4ECDC4',
  google_adk: '#4285F4',
  openai_agents: '#10A37F',
  claude_agents: '#D97706',
  unknown: '#475569',
}

function StatCard({ label, value, sub, color }: { label: string; value: string; sub?: string; color?: string }) {
  return (
    <div style={{
      background:'#0D1B2A', border:'1px solid #0F1F35', borderRadius:10,
      padding:'20px 24px', borderTop:`2px solid ${color ?? '#3B82F6'}`
    }}>
      <div style={{ fontSize:11, color:'#475569', letterSpacing:'0.1em', marginBottom:8 }}>{label}</div>
      <div style={{ fontSize:28, fontWeight:700, color:'#F0F9FF', lineHeight:1 }}>{value}</div>
      {sub && <div style={{ fontSize:11, color:'#334155', marginTop:6 }}>{sub}</div>}
    </div>
  )
}

export default function Dashboard() {
  const { data: overview, isLoading } = useOverview('24h')
  const { data: traces } = useTraces({ limit: '10' })

  const frameworkData = overview
    ? Object.entries(overview.framework_counts ?? {}).map(([name, count]) => ({ name, count }))
    : []

  return (
    <div style={{ padding:32 }}>
      <div style={{ marginBottom:28 }}>
        <h1 style={{ fontSize:22, fontWeight:700, color:'#F0F9FF', margin:0 }}>Dashboard</h1>
        <p style={{ fontSize:12, color:'#475569', marginTop:4 }}>Last 24 hours · Auto-refresh 30s</p>
      </div>

      <div style={{ display:'grid', gridTemplateColumns:'repeat(auto-fit, minmax(180px,1fr))', gap:16, marginBottom:28 }}>
        <StatCard label="TOTAL TRACES" value={isLoading ? '—' : (overview?.total_traces ?? 0).toLocaleString()} color="#3B82F6" />
        <StatCard label="TOTAL COST" value={isLoading ? '—' : `$${(overview?.total_cost_usd ?? 0).toFixed(4)}`} color="#F59E0B" />
        <StatCard label="AVG LATENCY" value={isLoading ? '—' : `${(overview?.avg_latency_ms ?? 0).toFixed(0)}ms`} color="#10B981" />
        <StatCard label="ERROR RATE" value={isLoading ? '—' : `${((overview?.error_rate ?? 0) * 100).toFixed(2)}%`} color={(overview?.error_rate ?? 0) > 0.05 ? '#EF4444' : '#10B981'} />
        <StatCard label="BLOCKED EVENTS" value={isLoading ? '—' : (overview?.blocked_requests ?? 0).toLocaleString()} color={(overview?.blocked_requests ?? 0) > 0 ? '#EF4444' : '#3B82F6'} />
        <StatCard label="LLM CALLS" value={isLoading ? '—' : (overview?.llm_calls ?? 0).toLocaleString()} sub={`tool calls ${(overview?.tool_calls ?? 0).toLocaleString()}`} color="#60A5FA" />
      </div>

      <div style={{ display:'grid', gridTemplateColumns:'1fr 320px', gap:16, marginBottom:28 }}>
        <div style={{ background:'#0D1B2A', border:'1px solid #0F1F35', borderRadius:10, padding:24 }}>
          <div style={{ fontSize:12, color:'#475569', marginBottom:16, letterSpacing:'0.1em' }}>FRAMEWORK DISTRIBUTION</div>
          <div style={{ display:'grid', gridTemplateColumns:'repeat(5,1fr)', gap:12, marginBottom:20 }}>
            {Object.entries(FRAMEWORK_COLORS).filter(([k]) => k !== 'unknown').map(([fw, color]) => (
              <div key={fw} style={{ textAlign:'center' }}>
                <div style={{
                  height:6, borderRadius:3, background:color,
                  opacity: (overview?.framework_counts?.[fw] ?? 0) > 0 ? 1 : 0.2,
                  marginBottom:6
                }} />
                <div style={{ fontSize:9, color:'#475569', letterSpacing:'0.08em' }}>{fw.replace('_',' ').toUpperCase()}</div>
                <div style={{ fontSize:13, color, fontWeight:700 }}>
                  {(overview?.framework_counts?.[fw] ?? 0).toLocaleString()}
                </div>
              </div>
            ))}
          </div>
          <ResponsiveContainer width="100%" height={120}>
            <BarChart data={frameworkData}>
              <XAxis dataKey="name" tick={{ fontSize:10, fill:'#475569' }} />
              <YAxis tick={{ fontSize:10, fill:'#475569' }} />
              <Tooltip contentStyle={{ background:'#0D1B2A', border:'1px solid #1E3A5F', borderRadius:6, fontSize:11 }} />
              <Bar dataKey="count" radius={[4,4,0,0]}>
                {frameworkData.map((entry) => (
                  <Cell key={entry.name} fill={FRAMEWORK_COLORS[entry.name] ?? '#475569'} />
                ))}
              </Bar>
            </BarChart>
          </ResponsiveContainer>
        </div>

        <div style={{ background:'#0D1B2A', border:'1px solid #0F1F35', borderRadius:10, padding:24 }}>
          <div style={{ fontSize:12, color:'#475569', marginBottom:16, letterSpacing:'0.1em' }}>TOKEN USAGE</div>
          <div style={{ textAlign:'center', marginBottom:12 }}>
            <div style={{ fontSize:24, fontWeight:700, color:'#F0F9FF' }}>
              {((overview?.total_tokens ?? 0) / 1_000_000).toFixed(2)}M
            </div>
            <div style={{ fontSize:11, color:'#475569' }}>total tokens</div>
          </div>
          <ResponsiveContainer width="100%" height={160}>
            <PieChart>
              <Pie data={frameworkData} cx="50%" cy="50%" innerRadius={50} outerRadius={75} dataKey="count" paddingAngle={3}>
                {frameworkData.map((entry) => (
                  <Cell key={entry.name} fill={FRAMEWORK_COLORS[entry.name] ?? '#475569'} />
                ))}
              </Pie>
              <Tooltip contentStyle={{ background:'#0D1B2A', border:'1px solid #1E3A5F', fontSize:11 }} />
            </PieChart>
          </ResponsiveContainer>
        </div>
      </div>

      <div style={{ background:'#0D1B2A', border:'1px solid #0F1F35', borderRadius:10 }}>
        <div style={{ padding:'16px 24px', borderBottom:'1px solid #0F1F35', fontSize:12, color:'#475569', letterSpacing:'0.1em' }}>
          RECENT TRACES
        </div>
        <table style={{ width:'100%', borderCollapse:'collapse', fontSize:12 }}>
          <thead>
            <tr>
              {['Trace ID','Framework','Root Span','Duration','Spans','Cost','Status'].map(h => (
                <th key={h} style={{ padding:'10px 16px', textAlign:'left', color:'#334155', borderBottom:'1px solid #0F1F35', fontSize:11, fontWeight:600, letterSpacing:'0.08em' }}>
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {(traces?.items ?? []).map((t, i) => {
              const statusColor = outcomeStatusColor(t.status)
              return (
              <tr key={t.id} style={{ background: i % 2 === 0 ? 'transparent' : '#060A1430' }}>
                <td style={{ padding:'8px 16px', color:'#3B82F6', fontFamily:'monospace', fontSize:11 }}>
                  <a href={`/traces/${t.id}`} style={{ color:'#3B82F6', textDecoration:'none' }}>
                    {t.id.substring(0, 16)}…
                  </a>
                </td>
                <td style={{ padding:'8px 16px' }}>
                  <span style={{
                    padding:'2px 8px', borderRadius:4, fontSize:10,
                    background: (FRAMEWORK_COLORS[t.framework] ?? '#475569') + '20',
                    color: FRAMEWORK_COLORS[t.framework] ?? '#475569',
                  }}>{t.framework}</span>
                </td>
                <td style={{ padding:'8px 16px', color:'#94A3B8', maxWidth:200, overflow:'hidden', textOverflow:'ellipsis', whiteSpace:'nowrap' }}>
                  {t.root_span_name}
                </td>
                <td style={{ padding:'8px 16px', color:'#94A3B8' }}>{(t.duration_ns / 1_000_000).toFixed(0)}ms</td>
                <td style={{ padding:'8px 16px', color:'#94A3B8' }}>{t.span_count}</td>
                <td style={{ padding:'8px 16px', color:'#F59E0B' }}>${t.total_cost_usd.toFixed(6)}</td>
                <td style={{ padding:'8px 16px' }}>
                  <span style={{
                    padding:'2px 8px', borderRadius:4, fontSize:10,
                    background: `${statusColor}20`,
                    color: statusColor,
                  }}>{t.status}</span>
                </td>
              </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </div>
  )
}
