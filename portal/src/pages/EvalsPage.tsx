import { useState, type CSSProperties } from 'react'
import { Link } from 'react-router-dom'
import { hasRole, useAuth } from '../hooks/auth'
import { useEvalRuns, useScoreTraceEval } from '../hooks/api'

export default function EvalsPage() {
  const { user } = useAuth()
  const isAdmin = hasRole(user, ['admin'])
  const { data, isLoading } = useEvalRuns(20)
  const scoreTrace = useScoreTraceEval()
  const [form, setForm] = useState({ trace_id: '', release_tag: '', eval_suite: 'core-release' })

  if (!isAdmin) {
    return (
      <div style={{ padding: 32, color: 'var(--text-secondary)' }}>
        This page is restricted to administrators.
      </div>
    )
  }

  return (
    <div style={{ padding: '40px 48px', maxWidth: 1440, margin: '0 auto', display: 'flex', flexDirection: 'column', gap: 24 }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 12 }}>
        <div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
            <span style={{ width: 8, height: 8, borderRadius: '50%', background: 'var(--ship)', display: 'inline-block' }} />
            <span style={{ fontSize: 10, fontWeight: 700, color: 'var(--ship)', letterSpacing: '0.1em' }}>SHIP</span>
          </div>
          <h1 style={{ margin: 0, color: 'var(--text-primary)', fontSize: 28, fontWeight: 700, letterSpacing: '-0.02em' }}>Evaluations</h1>
          <div style={{ marginTop: 6, color: 'var(--text-tertiary)', fontSize: 12 }}>
            Score enriched traces and persist release evidence for governance reviews.
          </div>
        </div>
        <Link to="/evals/regressions" style={{ color: 'var(--control)', fontSize: 12, textDecoration: 'none', background: 'rgba(10,132,255,0.1)', border: '1px solid rgba(10,132,255,0.25)', padding: '8px 14px', borderRadius: 8 }}>
          View Regressions →
        </Link>
      </div>

      {/* Score form */}
      <div style={panelStyle}>
        <div style={sectionLabel}>SCORE TRACE</div>
        <div style={{ display: 'grid', gridTemplateColumns: '2fr 1fr 1fr auto', gap: 12 }}>
          <input id="eval-trace-id" value={form.trace_id} onChange={e => setForm(c => ({ ...c, trace_id: e.target.value }))} style={inputStyle} placeholder="trace id" />
          <input id="eval-release-tag" value={form.release_tag} onChange={e => setForm(c => ({ ...c, release_tag: e.target.value }))} style={inputStyle} placeholder="candidate-2026-03-22" />
          <input id="eval-suite" value={form.eval_suite} onChange={e => setForm(c => ({ ...c, eval_suite: e.target.value }))} style={inputStyle} placeholder="core-release" />
          <button id="eval-score-btn" style={primaryBtn} disabled={scoreTrace.isPending || !form.trace_id} onClick={() => scoreTrace.mutate(form)}>
            {scoreTrace.isPending ? 'Scoring...' : 'Score'}
          </button>
        </div>
        {scoreTrace.data && (
          <div style={{ marginTop: 12, color: 'var(--text-secondary)', fontSize: 12 }}>
            ✓ Eval run #{scoreTrace.data.id} — overall score <strong style={{ color: 'var(--spend)' }}>{scoreTrace.data.overall_score.toFixed(2)}</strong>
          </div>
        )}
        {scoreTrace.isError && (
          <div style={{ marginTop: 12, color: 'var(--protect)', fontSize: 12 }}>Scoring failed. Check trace ID and backend logs.</div>
        )}
      </div>

      {/* Eval runs */}
      {isLoading && <div style={{ color: 'var(--text-tertiary)', fontSize: 12 }}>Loading eval runs...</div>}
      <div style={{ display: 'grid', gap: 12 }}>
        {(data?.items ?? []).map(run => (
          <div key={run.id} style={panelStyle}>
            <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap' }}>
              <div>
                <div style={{ color: 'var(--text-primary)', fontSize: 14, fontWeight: 600, fontFamily: 'monospace' }}>{run.trace_id}</div>
                <div style={{ color: 'var(--text-tertiary)', fontSize: 11, marginTop: 4 }}>
                  suite {run.eval_suite} {run.release_tag ? `· ${run.release_tag}` : ''} · {new Date(run.created_at).toLocaleString()}
                </div>
              </div>
              <div style={{ textAlign: 'right' }}>
                <div style={{ color: scoreColor(run.overall_score), fontSize: 24, fontWeight: 700, letterSpacing: '-0.02em' }}>
                  {run.overall_score.toFixed(1)}
                </div>
                <div style={{ color: 'var(--text-tertiary)', fontSize: 11 }}>{run.risk_level} risk</div>
              </div>
            </div>
            <div style={{ marginTop: 16, display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))', gap: 10 }}>
              {(run.scores ?? []).map(score => (
                <div key={score.metric} style={metricCard}>
                  <div style={{ color: 'var(--ship)', fontSize: 9, textTransform: 'uppercase', letterSpacing: '0.1em', fontWeight: 700 }}>{score.metric}</div>
                  <div style={{ marginTop: 8, color: scoreColor(score.score), fontSize: 22, fontWeight: 700 }}>{score.score.toFixed(1)}</div>
                  <div style={{ marginTop: 4, color: 'var(--text-tertiary)', fontSize: 11 }}>{score.summary}</div>
                </div>
              ))}
            </div>
            <div style={{ marginTop: 14, fontSize: 12, color: 'var(--text-tertiary)', borderTop: '1px solid var(--layer-border)', paddingTop: 12 }}>
              Policy coverage: <strong style={{ color: 'var(--protect)' }}>{Math.round((run.policy_effectiveness.coverage_ratio ?? 0) * 100)}%</strong>
              {' · '}blocked spans: {run.policy_effectiveness.blocked_spans}
              {' · '}redacted spans: {run.policy_effectiveness.redacted_spans}
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}

function scoreColor(score: number) {
  if (score >= 85) return 'var(--spend)'
  if (score >= 65) return 'var(--prove)'
  return 'var(--protect)'
}

const panelStyle = {
  background: 'var(--layer-2)',
  border: '1px solid var(--layer-border)',
  borderRadius: 12,
  padding: 20,
} satisfies CSSProperties

const metricCard = {
  background: 'var(--layer-1)',
  border: '1px solid var(--layer-border)',
  borderRadius: 10,
  padding: 14,
} satisfies CSSProperties

const sectionLabel = {
  color: 'var(--ship)',
  fontSize: 10,
  letterSpacing: '0.1em',
  marginBottom: 12,
  fontWeight: 700,
} satisfies CSSProperties

const inputStyle = {
  width: '100%',
  borderRadius: 8,
  border: '1px solid var(--layer-border)',
  background: 'var(--layer-1)',
  color: 'var(--text-primary)',
  padding: '10px 12px',
  fontSize: 12,
  outline: 'none',
} satisfies CSSProperties

const primaryBtn = {
  border: 'none',
  borderRadius: 8,
  background: 'var(--ship)',
  color: '#fff',
  padding: '10px 18px',
  cursor: 'pointer',
  fontSize: 12,
  fontWeight: 700,
  whiteSpace: 'nowrap' as const,
} satisfies CSSProperties
