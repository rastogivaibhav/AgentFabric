import { useParams, Link } from 'react-router-dom'
import type { CSSProperties } from 'react'
import { useAgent, useAgentRuns, useAgentTopology } from '../hooks/api'
import { ArrowLeft } from 'lucide-react'

export default function AgentDetailPage() {
  const { agentId } = useParams<{ agentId: string }>()
  const { data: agent, isLoading: agentLoading } = useAgent(agentId!)
  const { data: runsPage, isLoading: runsLoading } = useAgentRuns(agentId!, 10)
  const { data: topology, isLoading: topologyLoading } = useAgentTopology(agentId!)

  if (agentLoading) return <div style={{ padding: 32, color: 'var(--text-tertiary)' }}>Loading agent...</div>
  if (!agent) return <div style={{ padding: 32, color: 'var(--protect)' }}>Agent not found.</div>

  const runs = runsPage?.items ?? []

  return (
    <div style={{ padding: 32 }}>
      <div style={{ marginBottom: 24 }}>
        <Link to="/agents" style={{ display: 'flex', alignItems: 'center', gap: 6, color: 'var(--text-tertiary)', textDecoration: 'none', fontSize: 12, marginBottom: 12 }}>
          <ArrowLeft size={14} /> Back to Agents
        </Link>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
          <span style={{ width: 8, height: 8, borderRadius: '50%', background: 'var(--observe)', display: 'inline-block' }} />
          <span style={{ fontSize: 10, fontWeight: 700, color: 'var(--observe)', letterSpacing: '0.1em' }}>OBSERVE</span>
        </div>
        <h1 style={titleStyle}>{agent.name}</h1>
        <p style={subtleText}>Framework: {agent.framework}</p>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 16, marginBottom: 24 }}>
        <StatCard label="Total Runs" value={agent.run_count.toLocaleString()} />
        <StatCard label="Total Cost" value={`$${agent.total_cost_usd.toFixed(2)}`} />
        <StatCard label="Error Rate" value={`${(agent.error_rate * 100).toFixed(1)}%`} color={agent.error_rate > 0.05 ? 'var(--protect)' : 'var(--spend)'} />
        <StatCard label="p95 Latency" value={`${agent.p95_latency_ms.toFixed(0)}ms`} />
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
        <div style={panelStyle}>
          <div style={{ ...sectionLabel, display: 'flex', justifyContent: 'space-between' }}>
            <span>RECENT RUNS</span>
            <Link to={`/runs?agent=${encodeURIComponent(agent.name)}`} style={{ color: 'var(--control)', textDecoration: 'none', fontSize: 10 }}>View All →</Link>
          </div>
          {runsLoading ? (
            <div style={subtleText}>Loading runs...</div>
          ) : runs.length === 0 ? (
            <div style={subtleText}>No recent runs.</div>
          ) : (
            <div style={{ display: 'grid', gap: 8 }}>
              {runs.map(run => (
                <Link key={run.id} to={`/runs/${run.id}`} style={{ textDecoration: 'none' }}>
                  <div style={{ border: '1px solid var(--layer-border)', borderRadius: 8, padding: '12px 16px', background: 'var(--layer-0)', display: 'flex', justifyContent: 'space-between', alignItems: 'center', transition: 'border-color 0.15s' }}>
                    <div>
                      <div style={{ color: 'var(--text-primary)', fontSize: 12, fontWeight: 600 }}>{run.model}</div>
                      <div style={{ fontSize: 10, color: 'var(--text-tertiary)', marginTop: 4 }}>{new Date(run.start_time).toLocaleString()}</div>
                    </div>
                    <div style={{ textAlign: 'right' }}>
                      <div style={{ color: run.status === 'error' ? 'var(--protect)' : 'var(--spend)', fontSize: 12, fontWeight: 600 }}>{run.status.toUpperCase()}</div>
                      <div style={{ fontSize: 10, color: 'var(--text-tertiary)', marginTop: 4 }}>${run.total_cost_usd.toFixed(4)} · {run.total_tokens} tokens</div>
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
              <div style={{ fontSize: 12, color: 'var(--text-secondary)', marginBottom: 12 }}>Nodes connected to this agent:</div>
              <div style={{ display: 'grid', gap: 6 }}>
                {topology.nodes.map(node => (
                  <div key={node.id} style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '8px 0', borderBottom: '1px solid var(--layer-border)' }}>
                    <span style={{ color: 'var(--text-primary)', fontSize: 12 }}>{node.name}</span>
                    <span style={{ fontSize: 10, color: 'var(--text-secondary)', background: 'var(--layer-1)', padding: '2px 8px', borderRadius: 4 }}>{node.type}</span>
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

function StatCard({ label, value, color = 'var(--text-primary)' }: { label: string; value: string; color?: string }) {
  return (
    <div style={{ background: 'var(--layer-2)', border: '1px solid var(--layer-border)', borderRadius: 10, padding: 20 }}>
      <div style={{ fontSize: 10, color: 'var(--text-tertiary)', letterSpacing: '0.08em', marginBottom: 10, fontWeight: 700 }}>{label.toUpperCase()}</div>
      <div style={{ fontSize: 28, fontWeight: 700, color, letterSpacing: '-0.02em' }}>{value}</div>
    </div>
  )
}

const titleStyle: CSSProperties = { fontSize: 28, fontWeight: 700, color: 'var(--text-primary)', margin: 0, letterSpacing: '-0.02em' }
const subtleText: CSSProperties = { fontSize: 12, color: 'var(--text-tertiary)', marginTop: 4 }
const panelStyle: CSSProperties = { background: 'var(--layer-2)', border: '1px solid var(--layer-border)', borderRadius: 12, padding: 24 }
const sectionLabel: CSSProperties = { fontSize: 10, color: 'var(--text-tertiary)', letterSpacing: '0.12em', marginBottom: 16, fontWeight: 700 }
