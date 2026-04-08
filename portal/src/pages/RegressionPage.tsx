import { useState, type CSSProperties } from 'react'
import { useCompareEvalRegressions } from '../hooks/api'

export default function RegressionPage() {
  const compare = useCompareEvalRegressions()
  const [form, setForm] = useState({ baseline_tag: '', candidate_tag: '', eval_suite: 'core-release' })

  return (
    <div style={{ padding: '40px 48px', maxWidth: 1200, margin: '0 auto', display: 'flex', flexDirection: 'column', gap: 20 }}>
      <div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
          <span style={{ width: 8, height: 8, borderRadius: '50%', background: 'var(--ship)', display: 'inline-block' }} />
          <span style={{ fontSize: 10, fontWeight: 700, color: 'var(--ship)', letterSpacing: '0.1em' }}>SHIP</span>
        </div>
        <h1 style={{ margin: 0, color: 'var(--text-primary)', fontSize: 28, fontWeight: 700, letterSpacing: '-0.02em' }}>Regression Report</h1>
        <div style={{ marginTop: 6, color: 'var(--text-tertiary)', fontSize: 12 }}>Compare release tags using stored trace eval evidence.</div>
      </div>

      <div style={panelStyle}>
        <div style={{ fontSize: 10, color: 'var(--text-tertiary)', letterSpacing: '0.12em', marginBottom: 14, fontWeight: 700 }}>COMPARISON PARAMETERS</div>
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 1fr auto', gap: 12 }}>
          <input id="reg-baseline" value={form.baseline_tag} onChange={e => setForm(c => ({ ...c, baseline_tag: e.target.value }))} style={inputStyle} placeholder="baseline tag e.g. v1.0.0" />
          <input id="reg-candidate" value={form.candidate_tag} onChange={e => setForm(c => ({ ...c, candidate_tag: e.target.value }))} style={inputStyle} placeholder="candidate tag e.g. v1.1.0" />
          <input id="reg-suite" value={form.eval_suite} onChange={e => setForm(c => ({ ...c, eval_suite: e.target.value }))} style={inputStyle} placeholder="eval suite" />
          <button id="reg-compare-btn" style={primaryBtn} disabled={compare.isPending || !form.baseline_tag || !form.candidate_tag} onClick={() => compare.mutate(form)}>
            {compare.isPending ? 'Comparing…' : '▶ Compare'}
          </button>
        </div>
      </div>

      {compare.isError && <div style={{ color: 'var(--protect)', fontSize: 13 }}>Regression comparison failed.</div>}

      {compare.data && (
        <div style={panelStyle}>
          <div style={{ display: 'flex', justifyContent: 'space-between', gap: 16, flexWrap: 'wrap', marginBottom: 16 }}>
            <div>
              <div style={{ color: 'var(--text-primary)', fontSize: 18, fontWeight: 700 }}>
                <span style={{ color: 'var(--text-secondary)' }}>{compare.data.baseline_tag}</span>
                {' '}<span style={{ color: 'var(--text-tertiary)' }}>vs</span>{' '}
                <span style={{ color: 'var(--ship)' }}>{compare.data.candidate_tag}</span>
              </div>
              <div style={{ marginTop: 6, color: 'var(--text-tertiary)', fontSize: 12 }}>
                {compare.data.compared_runs} compared runs · suite {compare.data.eval_suite}
              </div>
            </div>
            <div style={{ textAlign: 'right' }}>
              <div style={{ color: compare.data.overall_delta >= 0 ? 'var(--spend)' : 'var(--protect)', fontSize: 28, fontWeight: 700, letterSpacing: '-0.02em' }}>
                {compare.data.overall_delta >= 0 ? '+' : ''}{compare.data.overall_delta.toFixed(2)}
              </div>
              <div style={{ color: 'var(--text-tertiary)', fontSize: 11 }}>{compare.data.risk_level} risk</div>
            </div>
          </div>

          {(compare.data.highlights ?? []).length > 0 && (
            <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginBottom: 14 }}>
              {compare.data.highlights?.map(item => (
                <span key={item} style={{ padding: '3px 10px', borderRadius: 999, background: 'rgba(10,132,255,0.10)', color: 'var(--control)', fontSize: 11 }}>{item}</span>
              ))}
            </div>
          )}

          <div style={{ display: 'grid', gap: 8 }}>
            <div style={{ display: 'grid', gridTemplateColumns: '2fr 1fr 1fr 1fr', gap: 12, padding: '8px 0', fontSize: 9, fontWeight: 700, letterSpacing: '0.1em', color: 'var(--text-tertiary)', borderBottom: '1px solid var(--layer-border)' }}>
              <span>METRIC</span><span>BASELINE</span><span>CANDIDATE</span><span>DELTA</span>
            </div>
            {(compare.data.metrics ?? []).map(metric => (
              <div key={metric.metric} style={rowStyle}>
                <div style={{ color: 'var(--ship)', fontSize: 13, fontWeight: 600 }}>{metric.metric}</div>
                <div style={{ color: 'var(--text-secondary)', fontSize: 13 }}>{metric.baseline_score.toFixed(1)}</div>
                <div style={{ color: 'var(--text-secondary)', fontSize: 13 }}>{metric.candidate_score.toFixed(1)}</div>
                <div style={{ color: metric.delta >= 0 ? 'var(--spend)' : 'var(--protect)', fontSize: 13, fontWeight: 700 }}>
                  {metric.delta >= 0 ? '+' : ''}{metric.delta.toFixed(2)}
                </div>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}

const panelStyle = { background: 'var(--layer-2)', border: '1px solid var(--layer-border)', borderRadius: 12, padding: 24 } satisfies CSSProperties
const rowStyle = { display: 'grid', gridTemplateColumns: '2fr 1fr 1fr 1fr', gap: 12, paddingBottom: 10, borderBottom: '1px solid var(--layer-border)' } satisfies CSSProperties
const inputStyle = { width: '100%', borderRadius: 8, border: '1px solid var(--layer-border)', background: 'var(--layer-1)', color: 'var(--text-primary)', padding: '10px 12px', fontSize: 12, outline: 'none' } satisfies CSSProperties
const primaryBtn = { border: 'none', borderRadius: 8, background: 'var(--ship)', color: '#fff', padding: '10px 18px', cursor: 'pointer', fontSize: 12, fontWeight: 700, whiteSpace: 'nowrap' } satisfies CSSProperties
