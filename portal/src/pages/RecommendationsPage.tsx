import { useState, type CSSProperties } from 'react'
import { useRecommendations, useUpdateRecommendationStatus, type Recommendation } from '../hooks/api'

const TYPE_COLORS: Record<string, string> = {
  rollout: 'var(--control)', routing: '#BF5AF2', policy: 'var(--prove)', cost: 'var(--spend)',
}
const STATUS_CFG: Record<string, { color: string; bg: string }> = {
  open:      { color: 'var(--protect)', bg: 'rgba(255,69,58,0.1)' },
  reviewing: { color: 'var(--prove)',   bg: 'rgba(255,159,10,0.1)' },
  applied:   { color: 'var(--spend)',   bg: 'rgba(48,209,88,0.1)' },
  dismissed: { color: 'var(--text-tertiary)', bg: 'rgba(255,255,255,0.05)' },
  resolved:  { color: 'var(--spend)',   bg: 'rgba(48,209,88,0.08)' },
}
type StatusKey = 'open' | 'reviewing' | 'applied' | 'dismissed' | 'resolved'
const STATUS_SEQUENCE: StatusKey[] = ['open', 'reviewing', 'applied', 'dismissed', 'resolved']

function typeBadge(type: string) {
  const c = TYPE_COLORS[type] ?? 'var(--text-tertiary)'
  return <span style={{ padding: '2px 8px', borderRadius: 4, fontSize: 10, background: `color-mix(in srgb, ${c} 12%, transparent)`, color: c, border: `1px solid color-mix(in srgb, ${c} 25%, transparent)` }}>{type}</span>
}

function statusBadge(status: string) {
  const cfg = STATUS_CFG[status] ?? STATUS_CFG.open
  return <span style={{ padding: '3px 10px', borderRadius: 999, fontSize: 10, fontWeight: 700, background: cfg.bg, color: cfg.color, letterSpacing: '0.07em' }}>{status.toUpperCase()}</span>
}

function confidenceBar(confidence: number) {
  const pct = Math.round(confidence * 100)
  const color = pct >= 80 ? 'var(--spend)' : pct >= 60 ? 'var(--prove)' : 'var(--protect)'
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
      <div style={{ flex: 1, height: 4, background: 'var(--layer-1)', borderRadius: 999 }}>
        <div style={{ width: `${pct}%`, height: '100%', background: color, borderRadius: 999, transition: 'width 0.3s' }} />
      </div>
      <span style={{ fontSize: 10, color, flexShrink: 0, fontWeight: 700 }}>{pct}%</span>
    </div>
  )
}

export default function RecommendationsPage() {
  const [since, setSince] = useState('72h')
  const [filterStatus, setFilterStatus] = useState('')
  const [filterType, setFilterType] = useState('')
  const [selected, setSelected] = useState<Recommendation | null>(null)

  const { data, isLoading, error } = useRecommendations({ since, limit: 50, status: filterStatus, type: filterType })
  const updateStatus = useUpdateRecommendationStatus()
  const items = data?.items ?? []

  async function handleStatusChange(rec: Recommendation, next: StatusKey) {
    await updateStatus.mutateAsync({ id: rec.id, status: next })
    if (selected?.id === rec.id) setSelected({ ...rec, status: next })
  }

  return (
    <div style={{ padding: '40px 48px', maxWidth: 1440, margin: '0 auto' }}>
      <div style={{ marginBottom: 28 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
          <span style={{ width: 8, height: 8, borderRadius: '50%', background: 'var(--control)', display: 'inline-block' }} />
          <span style={{ fontSize: 10, fontWeight: 700, color: 'var(--control)', letterSpacing: '0.1em' }}>CONTROL</span>
        </div>
        <h1 style={titleStyle}>Recommendations</h1>
        <p style={subtleText}>Autonomic engine suggestions for cost, policy tuning, routing optimization, and rollout decisions.</p>
      </div>

      {/* Filters */}
      <div style={{ display: 'flex', gap: 12, marginBottom: 24, flexWrap: 'wrap' }}>
        {[
          { id: 'recs-filter-since', label: 'Window', value: since, setter: setSince, options: [['24h','Last 24h'],['72h','Last 72h'],['168h','Last 7d'],['720h','Last 30d']] },
          { id: 'recs-filter-status', label: 'Status', value: filterStatus, setter: setFilterStatus, options: [['','All'], ...STATUS_SEQUENCE.map(s => [s,s])] },
          { id: 'recs-filter-type', label: 'Type', value: filterType, setter: setFilterType, options: [['','All'], ...['rollout','routing','policy','cost'].map(t => [t,t])] },
        ].map(f => (
          <label key={f.id} style={{ display: 'grid', gap: 5, fontSize: 11, color: 'var(--text-secondary)' }}>
            {f.label}
            <select id={f.id} style={selectStyle} value={f.value} onChange={e => f.setter(e.target.value)}>
              {f.options.map(([v, l]) => <option key={v} value={v}>{l}</option>)}
            </select>
          </label>
        ))}
      </div>

      {/* Summary strip */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, minmax(0, 1fr))', gap: 16, marginBottom: 28 }}>
        {(['open', 'reviewing', 'applied', 'dismissed'] as StatusKey[]).map(s => {
          const cfg = STATUS_CFG[s]
          const count = items.filter(r => r.status === s).length
          return (
            <div key={s} style={{ background: 'var(--layer-2)', border: '1px solid var(--layer-border)', borderRadius: 12, padding: '18px 20px' }}>
              <div style={{ fontSize: 10, color: 'var(--text-tertiary)', letterSpacing: '0.1em', marginBottom: 10, fontWeight: 700 }}>{s.toUpperCase()}</div>
              <div style={{ fontSize: 32, fontWeight: 700, color: cfg.color, letterSpacing: '-0.02em' }}>{count}</div>
            </div>
          )
        })}
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'minmax(380px, 420px) 1fr', gap: 16 }}>
        {/* List */}
        <div style={panelStyle}>
          <div style={sectionLabel}>ALL RECOMMENDATIONS ({items.length})</div>
          {isLoading && <div style={subtleText}>Loading…</div>}
          {error && <div style={{ color: 'var(--protect)', fontSize: 12 }}>Failed to load recommendations.</div>}
          {!isLoading && !error && items.length === 0 && (
            <div style={{ ...subtleText, textAlign: 'center', padding: 40 }}>
              No recommendations in this window. The engine surfaces signals as activity grows.
            </div>
          )}
          <div style={{ display: 'grid', gap: 8 }}>
            {items.map(rec => (
              <button key={rec.id} id={`rec-${rec.id}`}
                style={{ ...cardBtn, borderColor: selected?.id === rec.id ? 'var(--control)' : 'var(--layer-border)', background: selected?.id === rec.id ? 'rgba(10,132,255,0.06)' : 'transparent' }}
                onClick={() => setSelected(rec)}>
                <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8, alignItems: 'flex-start' }}>
                  <div style={{ color: 'var(--text-primary)', fontSize: 12, fontWeight: 600, textAlign: 'left', flex: 1 }}>{rec.title}</div>
                  {statusBadge(rec.status)}
                </div>
                <div style={{ marginTop: 10, display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
                  {typeBadge(rec.type)}
                  <div style={{ flex: 1 }}>{confidenceBar(rec.confidence)}</div>
                </div>
                {rec.estimated_impact && <div style={{ fontSize: 10, color: 'var(--text-tertiary)', marginTop: 6, textAlign: 'left' }}>Impact: {rec.estimated_impact}</div>}
              </button>
            ))}
          </div>
        </div>

        {/* Detail */}
        <div style={panelStyle}>
          <div style={sectionLabel}>DETAIL</div>
          {!selected ? (
            <div style={subtleText}>Select a recommendation to view details and take action.</div>
          ) : (
            <div>
              <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, alignItems: 'flex-start', marginBottom: 20 }}>
                <div>
                  <div style={{ color: 'var(--text-primary)', fontSize: 20, fontWeight: 700 }}>{selected.title}</div>
                  <div style={{ display: 'flex', gap: 8, marginTop: 8, flexWrap: 'wrap', alignItems: 'center' }}>
                    {typeBadge(selected.type)}{statusBadge(selected.status)}
                  </div>
                </div>
                <div style={{ textAlign: 'right', flexShrink: 0 }}>
                  <div style={{ fontSize: 10, color: 'var(--text-tertiary)', marginBottom: 4 }}>CONFIDENCE</div>
                  <div style={{ fontSize: 30, fontWeight: 700, letterSpacing: '-0.02em', color: selected.confidence >= 0.8 ? 'var(--spend)' : selected.confidence >= 0.6 ? 'var(--prove)' : 'var(--protect)' }}>
                    {Math.round(selected.confidence * 100)}%
                  </div>
                </div>
              </div>

              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10, marginBottom: 16 }}>
                {[['Target', selected.target], ['Blast Radius', selected.blast_radius ?? '—'], ['Estimated Impact', selected.estimated_impact ?? '—'], ['Last Seen', new Date(selected.last_seen_at).toLocaleString()]].map(([l, v]) => (
                  <div key={l} style={{ border: '1px solid var(--layer-border)', borderRadius: 8, padding: '10px 14px', background: 'var(--layer-0)' }}>
                    <div style={{ fontSize: 9, color: 'var(--text-tertiary)', letterSpacing: '0.1em', marginBottom: 4, fontWeight: 700 }}>{l.toUpperCase()}</div>
                    <div style={{ fontSize: 12, color: 'var(--text-primary)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{v}</div>
                  </div>
                ))}
              </div>

              <div style={{ border: '1px solid var(--layer-border)', borderRadius: 10, padding: 14, background: 'var(--layer-0)', marginBottom: 12 }}>
                <div style={{ fontSize: 9, color: 'var(--text-tertiary)', letterSpacing: '0.1em', marginBottom: 8, fontWeight: 700 }}>SUMMARY</div>
                <div style={{ fontSize: 12, color: 'var(--text-secondary)', lineHeight: 1.7 }}>{selected.summary}</div>
              </div>

              <div style={{ border: '1px solid rgba(48,209,88,0.2)', borderRadius: 10, padding: 14, background: 'rgba(48,209,88,0.04)', marginBottom: 20 }}>
                <div style={{ fontSize: 9, color: 'var(--spend)', letterSpacing: '0.1em', marginBottom: 8, fontWeight: 700 }}>SUGGESTED ACTION</div>
                <div style={{ fontSize: 12, color: 'var(--spend)', lineHeight: 1.7 }}>{selected.suggested_action}</div>
              </div>

              <div style={sectionLabel}>UPDATE STATUS</div>
              <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                {STATUS_SEQUENCE.filter(s => s !== selected.status).map(next => (
                  <button key={next} id={`rec-status-${selected.id}-${next}`}
                    style={{ background: STATUS_CFG[next].bg, border: `1px solid ${STATUS_CFG[next].color}40`, borderRadius: 8, color: STATUS_CFG[next].color, padding: '7px 14px', fontSize: 12, cursor: 'pointer' }}
                    onClick={() => handleStatusChange(selected, next)} disabled={updateStatus.isPending}>
                    → {next}
                  </button>
                ))}
              </div>

              {selected.evidence && Object.keys(selected.evidence).length > 0 && (
                <div style={{ marginTop: 20 }}>
                  <div style={sectionLabel}>EVIDENCE</div>
                  <pre style={{ margin: 0, padding: 12, borderRadius: 8, background: 'var(--layer-0)', color: 'var(--text-secondary)', fontSize: 10, whiteSpace: 'pre-wrap', border: '1px solid var(--layer-border)' }}>
                    {JSON.stringify(selected.evidence, null, 2)}
                  </pre>
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

const titleStyle: CSSProperties = { fontSize: 28, fontWeight: 700, color: 'var(--text-primary)', margin: 0, letterSpacing: '-0.02em' }
const subtleText: CSSProperties = { fontSize: 12, color: 'var(--text-tertiary)', marginTop: 4 }
const panelStyle: CSSProperties = { background: 'var(--layer-2)', border: '1px solid var(--layer-border)', borderRadius: 12, padding: 24 }
const sectionLabel: CSSProperties = { fontSize: 10, color: 'var(--text-tertiary)', letterSpacing: '0.12em', marginBottom: 12, fontWeight: 700 }
const selectStyle: CSSProperties = { background: 'var(--layer-2)', border: '1px solid var(--layer-border)', borderRadius: 8, color: 'var(--text-primary)', padding: '8px 12px', fontSize: 12, outline: 'none' }
const cardBtn: CSSProperties = { border: '1px solid', borderRadius: 10, padding: 14, cursor: 'pointer', width: '100%', background: 'transparent', transition: 'all 0.15s' }
