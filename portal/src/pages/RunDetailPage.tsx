import { useParams, Link } from 'react-router-dom'
import type { CSSProperties } from 'react'
import { useRun, useRunChildren, usePostFeedback } from '../hooks/api'
import { ArrowLeft } from 'lucide-react'

export default function RunDetailPage() {
  const { runId } = useParams<{ runId: string }>()
  const { data: run, isLoading, error } = useRun(runId!)
  const { data: children } = useRunChildren(runId!)
  const feedback = usePostFeedback()

  if (isLoading) return <div style={subtleText}>Loading run details...</div>
  if (error || !run) return <div style={{ color: '#EF4444' }}>Failed to load run details.</div>

  const handleFeedback = (score: number) => {
    feedback.mutate({ runId: runId!, score })
  }

  return (
    <div style={{ padding: 32 }}>
      <div style={{ marginBottom: 24 }}>
        <Link to="/runs" style={{ display: 'flex', alignItems: 'center', gap: 6, color: '#64748B', textDecoration: 'none', fontSize: 12, marginBottom: 12 }}>
          <ArrowLeft size={14} /> Back to Runs
        </Link>
        <h1 style={titleStyle}>Run Details</h1>
        <p style={{ ...subtleText, fontFamily: 'monospace' }}>{run.id}</p>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'minmax(350px, 1fr) 300px', gap: 16 }}>
        <div style={panelStyle}>
          <div style={sectionLabel}>RUN INFORMATION</div>
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
            <DetailStat label="Agent" value={run.agent_name} />
            <DetailStat label="Framework" value={run.framework} />
            <DetailStat label="Model" value={run.model} />
            <DetailStat label="Status" value={run.status} color={run.status === 'error' ? '#EF4444' : '#10B981'} />
            <DetailStat label="Tokens" value={run.total_tokens.toLocaleString()} />
            <DetailStat label="Cost" value={`$${run.total_cost_usd.toFixed(4)}`} />
            <DetailStat label="Start Time" value={new Date(run.start_time).toLocaleString()} />
            {run.end_time && <DetailStat label="End Time" value={new Date(run.end_time).toLocaleString()} />}
          </div>

          <div style={{ ...sectionLabel, marginTop: 24 }}>FEEDBACK</div>
          <div style={{ display: 'flex', gap: 8 }}>
            <button style={{ ...ghostBtn, borderColor: '#10B981', color: '#10B981' }} onClick={() => handleFeedback(1)} disabled={feedback.isPending}>
              👍 Positive
            </button>
            <button style={{ ...ghostBtn, borderColor: '#EF4444', color: '#EF4444' }} onClick={() => handleFeedback(-1)} disabled={feedback.isPending}>
              👎 Negative
            </button>
          </div>
        </div>

        <div style={panelStyle}>
          <div style={sectionLabel}>CHILD RUNS</div>
          {!children?.length ? (
            <div style={subtleText}>No child runs.</div>
          ) : (
            <div style={{ display: 'grid', gap: 8 }}>
              {children.map(child => (
                <Link key={child.id} to={`/runs/${child.id}`} style={{ display: 'block', textDecoration: 'none' }}>
                  <div style={{ border: '1px solid #1E3A5F', borderRadius: 8, padding: 12, background: '#0A1B33', color: '#E2E8F0', fontSize: 12 }}>
                    <div style={{ fontWeight: 600 }}>{child.agent_name}</div>
                    <div style={{ fontSize: 10, color: '#94A3B8', marginTop: 4 }}>
                      {child.framework} · {child.total_tokens} tokens
                    </div>
                  </div>
                </Link>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

function DetailStat({ label, value, color = '#E2E8F0' }: { label: string; value: string; color?: string }) {
  return (
    <div style={{ border: '1px solid #1E3A5F', borderRadius: 8, padding: '10px 14px', background: '#0A1B33' }}>
      <div style={{ fontSize: 9, color: '#475569', letterSpacing: '0.1em', marginBottom: 4 }}>{label.toUpperCase()}</div>
      <div style={{ fontSize: 12, color, fontWeight: 600 }}>{value}</div>
    </div>
  )
}

const titleStyle: CSSProperties = { fontSize: 22, fontWeight: 700, color: '#F0F9FF', margin: 0 }
const subtleText: CSSProperties = { fontSize: 12, color: '#64748B', marginTop: 4 }
const panelStyle: CSSProperties = { background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 10, padding: 24 }
const sectionLabel: CSSProperties = { fontSize: 10, color: '#334155', letterSpacing: '0.12em', marginBottom: 16 }
const ghostBtn: CSSProperties = { background: 'none', border: '1px solid #0F1F35', borderRadius: 8, padding: '6px 12px', fontSize: 12, cursor: 'pointer' }
