import { useState, type CSSProperties } from 'react'
import { Link } from 'react-router-dom'
import { hasRole, useAuth } from '../hooks/auth'
import { useEvalRuns, useScoreTraceEval } from '../hooks/api'

export default function EvalsPage() {
  const { user } = useAuth()
  const isAdmin = hasRole(user, ['admin'])
  const { data, isLoading } = useEvalRuns(20)
  const scoreTrace = useScoreTraceEval()
  const [form, setForm] = useState({
    trace_id: '',
    release_tag: '',
    eval_suite: 'core-release',
  })

  if (!isAdmin) {
    return <div style={{ padding: 32, color: '#94A3B8' }}>This page is restricted to administrators.</div>
  }

  return (
    <div style={{ padding: 24, display: 'grid', gap: 16 }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 12 }}>
        <div>
          <h1 style={{ margin: 0, color: '#F0F9FF', fontSize: 20 }}>Evals</h1>
          <div style={{ marginTop: 8, color: '#64748B', fontSize: 12 }}>
            Score enriched traces and persist release evidence for governance reviews.
          </div>
        </div>
        <Link to="/evals/regressions" style={{ color: '#60A5FA', fontSize: 12, textDecoration: 'none' }}>
          Open regressions
        </Link>
      </div>

      <div style={panelStyle}>
        <div style={sectionLabel}>SCORE TRACE</div>
        <div style={{ display: 'grid', gridTemplateColumns: '2fr 1fr 1fr auto', gap: 12 }}>
          <input value={form.trace_id} onChange={e => setForm(current => ({ ...current, trace_id: e.target.value }))} style={inputStyle} placeholder="trace id" />
          <input value={form.release_tag} onChange={e => setForm(current => ({ ...current, release_tag: e.target.value }))} style={inputStyle} placeholder="candidate-2026-03-22" />
          <input value={form.eval_suite} onChange={e => setForm(current => ({ ...current, eval_suite: e.target.value }))} style={inputStyle} placeholder="core-release" />
          <button
            style={primaryBtn}
            disabled={scoreTrace.isPending || !form.trace_id}
            onClick={() => scoreTrace.mutate(form)}
          >
            {scoreTrace.isPending ? 'Scoring...' : 'Score'}
          </button>
        </div>
        {scoreTrace.data && (
          <div style={{ marginTop: 12, color: '#94A3B8', fontSize: 12 }}>
            Stored eval run #{scoreTrace.data.id} with overall score {scoreTrace.data.overall_score.toFixed(2)}.
          </div>
        )}
        {scoreTrace.isError && (
          <div style={{ marginTop: 12, color: '#FCA5A5', fontSize: 12 }}>Scoring failed. Check trace ID and backend logs.</div>
        )}
      </div>

      <div style={{ display: 'grid', gap: 12 }}>
        {(data?.items ?? []).map(run => (
          <div key={run.id} style={panelStyle}>
            <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap' }}>
              <div>
                <div style={{ color: '#F8FAFC', fontSize: 14 }}>{run.trace_id}</div>
                <div style={{ color: '#64748B', fontSize: 11, marginTop: 4 }}>
                  suite {run.eval_suite} {run.release_tag ? `• ${run.release_tag}` : ''} • {new Date(run.created_at).toLocaleString()}
                </div>
              </div>
              <div style={{ textAlign: 'right' }}>
                <div style={{ color: scoreColor(run.overall_score), fontSize: 18, fontWeight: 700 }}>
                  {run.overall_score.toFixed(1)}
                </div>
                <div style={{ color: '#64748B', fontSize: 11 }}>{run.risk_level} risk</div>
              </div>
            </div>
            <div style={{ marginTop: 12, display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))', gap: 10 }}>
              {(run.scores ?? []).map(score => (
                <div key={score.metric} style={metricCard}>
                  <div style={{ color: '#60A5FA', fontSize: 10, textTransform: 'uppercase' }}>{score.metric}</div>
                  <div style={{ marginTop: 6, color: '#E2E8F0', fontSize: 18 }}>{score.score.toFixed(1)}</div>
                  <div style={{ marginTop: 4, color: '#64748B', fontSize: 11 }}>{score.summary}</div>
                </div>
              ))}
            </div>
            <div style={{ marginTop: 12, color: '#94A3B8', fontSize: 12 }}>
              Policy coverage: {Math.round((run.policy_effectiveness.coverage_ratio ?? 0) * 100)}% •
              blocked spans {run.policy_effectiveness.blocked_spans} •
              redacted spans {run.policy_effectiveness.redacted_spans}
            </div>
          </div>
        ))}
      </div>

      {isLoading && <div style={{ color: '#94A3B8', fontSize: 12 }}>Loading eval runs...</div>}
    </div>
  )
}

function scoreColor(score: number) {
  if (score >= 85) return '#10B981'
  if (score >= 65) return '#F59E0B'
  return '#EF4444'
}

const panelStyle = {
  background: '#0D1B2A',
  border: '1px solid #0F1F35',
  borderRadius: 8,
  padding: 16,
} satisfies CSSProperties

const metricCard = {
  background: '#081221',
  border: '1px solid #0F1F35',
  borderRadius: 8,
  padding: 12,
} satisfies CSSProperties

const sectionLabel = {
  color: '#3B82F6',
  fontSize: 10,
  letterSpacing: '0.1em',
  marginBottom: 12,
} satisfies CSSProperties

const inputStyle = {
  width: '100%',
  borderRadius: 8,
  border: '1px solid #1E293B',
  background: '#081221',
  color: '#E2E8F0',
  padding: '10px 12px',
} satisfies CSSProperties

const primaryBtn = {
  border: 'none',
  borderRadius: 8,
  background: '#2563EB',
  color: '#EFF6FF',
  padding: '10px 16px',
  cursor: 'pointer',
} satisfies CSSProperties
