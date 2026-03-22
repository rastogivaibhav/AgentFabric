import { useState, type CSSProperties } from 'react'
import { useCompareEvalRegressions } from '../hooks/api'

export default function RegressionPage() {
  const compare = useCompareEvalRegressions()
  const [form, setForm] = useState({
    baseline_tag: '',
    candidate_tag: '',
    eval_suite: 'core-release',
  })

  return (
    <div style={{ padding: 24, display: 'grid', gap: 16 }}>
      <div>
        <h1 style={{ margin: 0, color: '#F0F9FF', fontSize: 20 }}>Regression Report</h1>
        <div style={{ marginTop: 8, color: '#64748B', fontSize: 12 }}>
          Compare release tags using stored trace eval evidence.
        </div>
      </div>

      <div style={panelStyle}>
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 1fr auto', gap: 12 }}>
          <input value={form.baseline_tag} onChange={e => setForm(current => ({ ...current, baseline_tag: e.target.value }))} style={inputStyle} placeholder="baseline tag" />
          <input value={form.candidate_tag} onChange={e => setForm(current => ({ ...current, candidate_tag: e.target.value }))} style={inputStyle} placeholder="candidate tag" />
          <input value={form.eval_suite} onChange={e => setForm(current => ({ ...current, eval_suite: e.target.value }))} style={inputStyle} placeholder="core-release" />
          <button style={primaryBtn} disabled={compare.isPending || !form.baseline_tag || !form.candidate_tag} onClick={() => compare.mutate(form)}>
            {compare.isPending ? 'Comparing...' : 'Compare'}
          </button>
        </div>
      </div>

      {compare.data && (
        <div style={panelStyle}>
          <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap' }}>
            <div>
              <div style={{ color: '#F8FAFC', fontSize: 15 }}>
                {compare.data.baseline_tag} vs {compare.data.candidate_tag}
              </div>
              <div style={{ marginTop: 6, color: '#64748B', fontSize: 12 }}>
                {compare.data.compared_runs} compared runs • suite {compare.data.eval_suite}
              </div>
            </div>
            <div style={{ textAlign: 'right' }}>
              <div style={{ color: compare.data.overall_delta >= 0 ? '#10B981' : '#EF4444', fontSize: 18, fontWeight: 700 }}>
                {compare.data.overall_delta >= 0 ? '+' : ''}{compare.data.overall_delta.toFixed(2)}
              </div>
              <div style={{ color: '#64748B', fontSize: 11 }}>{compare.data.risk_level} risk</div>
            </div>
          </div>

          {(compare.data.highlights ?? []).length > 0 && (
            <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginTop: 12 }}>
              {compare.data.highlights?.map(item => (
                <span key={item} style={chipStyle}>{item}</span>
              ))}
            </div>
          )}

          <div style={{ marginTop: 14, display: 'grid', gap: 8 }}>
            {(compare.data.metrics ?? []).map(metric => (
              <div key={metric.metric} style={rowStyle}>
                <div style={{ color: '#93C5FD', fontSize: 12 }}>{metric.metric}</div>
                <div style={{ color: '#CBD5E1', fontSize: 12 }}>{metric.baseline_score.toFixed(1)}</div>
                <div style={{ color: '#CBD5E1', fontSize: 12 }}>{metric.candidate_score.toFixed(1)}</div>
                <div style={{ color: metric.delta >= 0 ? '#10B981' : '#EF4444', fontSize: 12 }}>
                  {metric.delta >= 0 ? '+' : ''}{metric.delta.toFixed(2)}
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {compare.isError && <div style={{ color: '#FCA5A5', fontSize: 12 }}>Regression comparison failed.</div>}
    </div>
  )
}

const panelStyle = {
  background: '#0D1B2A',
  border: '1px solid #0F1F35',
  borderRadius: 8,
  padding: 16,
} satisfies CSSProperties

const rowStyle = {
  display: 'grid',
  gridTemplateColumns: '2fr 1fr 1fr 1fr',
  gap: 12,
  borderBottom: '1px solid #0A1020',
  paddingBottom: 8,
} satisfies CSSProperties

const chipStyle = {
  padding: '4px 10px',
  borderRadius: 999,
  background: '#102742',
  color: '#93C5FD',
  fontSize: 10,
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
