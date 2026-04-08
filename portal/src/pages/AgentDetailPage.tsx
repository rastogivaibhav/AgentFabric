import { useParams, Link } from 'react-router-dom'
import type { CSSProperties } from 'react'
import { useAgent, useAgentRuns, useAgentTopology } from '../hooks/api'
import { ArrowLeft } from 'lucide-react'

export default function AgentDetailPage() {
  const { agentId } = useParams<{ agentId: string }>()
  const { data: agent, isLoading: agentLoading } = useAgent(agentId!)
  const { data: runsPage, isLoading: runsLoading } = useAgentRuns(agentId!, 10)
  const { data: topology, isLoading: topologyLoading } = useAgentTopology(agentId!)

  if (agentLoading) return <div style={{ padding: 32, ...subtleText }}>Loading agent...</div>
  if (!agent) return <div style={{ padding: 32, color: '#EF4444' }}>Agent not found.</div>

  const runs = runsPage?.items ?? []

  return (
    <div style={{ padding: 32 }}>
      <div style={{ marginBottom: 24 }}>
        <Link to="/agents" style={{ display: 'flex', alignItems: 'center', gap: 6, color: '#64748B', textDecoration: 'none', fontSize: 12, marginBottom: 12 }}>
          <ArrowLeft size={14} /> Back to Agents
        </Link>
        <h1 style={titleStyle}>{agent.name}</h1>
        <p style={subtleText}>Framework: {agent.framework}</p>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 16, marginBottom: 24 }}>
        <StatCard label="Total Runs" value={agent.run_count.toLocaleString()} />
        <StatCard label="Total Cost" value={`$${agent.total_cost_usd.toFixed(2)}`} />
        <StatCard label="Error Rate" value={`${(agent.error_rate * 100).toFixed(1)}%`} color={agent.error_rate > 0.05 ? '#EF4444' : '#10B981'} />
        <StatCard label="p95 Latency" value={`${agent.p95_latency_ms.toFixed(0)}ms`} />
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
        <div style={panelStyle}>
          <div style={{ ...sectionLabel, display: 'flex', justifyContent: 'space-between' }}>
            <span>RECENT RUNS</span>
            <Link to={`/runs?agent=${encodeURIComponent(agent.name)}`} style={{ color: '#3B82F6', textDecoration: 'none' }}>View All →</Link>
          </div>
          {runsLoading ? (
            <div style={subtleText}>Loading runs...</div>
          ) : runs.length === 0 ? (
            <div style={subtleText}>No recent runs.</div>
          ) : (
            <div style={{ display: 'grid', gap: 8 }}>
              {runs.map(run => (
                <Link key={run.id} to={`/runs/${run.id}`} style={{ textDecoration: 'none' }}>
                  <div style={{ border: '1px solid #1E3A5F', borderRadius: 8, padding: '12px 16px', background: '#0A1B33', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                    <div>
                      <div style={{ color: '#E2E8F0', fontSize: 12, fontWeight: 600 }}>{run.model}</div>
                      <div style={{ fontSize: 10, color: '#94A3B8', marginTop: 4 }}>{new Date(run.start_time).toLocaleString()}</div>
                    </div>
                    <div style={{ textAlign: 'right' }}>
                      <div style={{ color: run.status === 'error' ? '#EF4444' : '#10B981', fontSize: 12, fontWeight: 600 }}>
                        {run.status.toUpperCase()}
                      </div>
                      <div style={{ fontSize: 10, color: '#64748B', marginTop: 4 }}>
                        ${run.total_cost_usd.toFixed(4)} · {run.total_tokens} tokens
                      </div>
                    </div>
                  </div>
                </Link>
              ))}
            </div>
          )}
        </div>

        <div style={panelStyle}>
          <div style={sectionLabel}>TOPOLOGY SUMMARY</div>
          {topologyLoading ? (
            <div style={subtleText}>Loading topology...</div>
          ) : !topology || topology.nodes.length === 0 ? (
            <div style={subtleText}>No topology data.</div>
          ) : (
            <div>
              <div style={{ fontSize: 12, color: '#E2E8F0', marginBottom: 12 }}>Nodes connected to this agent:</div>
              <div style={{ display: 'grid', gap: 8 }}>
                 {topology.nodes.map(node => (
                   <div key={node.id} style={{ display: 'flex', justifyContent: 'space-between', padding: 8, borderBottom: '1px solid #1E3A5F' }}>
                     <span style={{ color: '#F0F9FF', fontSize: 12 }}>{node.name}</span>
                     <span style={{ fontSize: 10, color: '#64748B', background: '#0F1F35', padding: '2px 6px', borderRadius: 4 }}>{node.type}</span>
                   </div>
                 ))}
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

function StatCard({ label, value, color = '#E2E8F0' }: { label: string; value: string; color?: string }) {
  return (
    <div style={{ background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 10, padding: 20 }}>
      <div style={{ fontSize: 11, color: '#64748B', letterSpacing: '0.05em', marginBottom: 8 }}>{label.toUpperCase()}</div>
      <div style={{ fontSize: 24, fontWeight: 700, color }}>{value}</div>
    </div>
  )
}

const titleStyle: CSSProperties = { fontSize: 22, fontWeight: 700, color: '#F0F9FF', margin: 0 }
const subtleText: CSSProperties = { fontSize: 12, color: '#64748B', marginTop: 4 }
const panelStyle: CSSProperties = { background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 10, padding: 24 }
const sectionLabel: CSSProperties = { fontSize: 10, color: '#334155', letterSpacing: '0.12em', marginBottom: 16 }
