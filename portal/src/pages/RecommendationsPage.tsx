import { useState, type CSSProperties } from 'react'
import {
  useRecommendations,
  useUpdateRecommendationStatus,
  type Recommendation,
} from '../hooks/api'

const TYPE_COLORS: Record<string, string> = {
  rollout: '#3B82F6',
  routing: '#8B5CF6',
  policy: '#F59E0B',
  cost: '#10B981',
}

const STATUS_CFG: Record<string, { color: string; bg: string }> = {
  open:      { color: '#EF4444', bg: '#EF444420' },
  reviewing: { color: '#F59E0B', bg: '#F59E0B20' },
  applied:   { color: '#10B981', bg: '#10B98120' },
  dismissed: { color: '#64748B', bg: '#47556920' },
  resolved:  { color: '#10B981', bg: '#10B98115' },
}

type StatusKey = 'open' | 'reviewing' | 'applied' | 'dismissed' | 'resolved'

function typeBadge(type: string) {
  const c = TYPE_COLORS[type] ?? '#64748B'
  return (
    <span style={{ padding: '2px 8px', borderRadius: 4, fontSize: 10, background: `${c}20`, color: c, border: `1px solid ${c}40` }}>
      {type}
    </span>
  )
}

function statusBadge(status: string) {
  const cfg = STATUS_CFG[status] ?? STATUS_CFG.open
  return (
    <span style={{ padding: '3px 10px', borderRadius: 999, fontSize: 10, fontWeight: 700, background: cfg.bg, color: cfg.color, letterSpacing: '0.07em' }}>
      {status.toUpperCase()}
    </span>
  )
}

function confidenceBar(confidence: number) {
  const pct = Math.round(confidence * 100)
  const color = pct >= 80 ? '#10B981' : pct >= 60 ? '#F59E0B' : '#EF4444'
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
      <div style={{ flex: 1, height: 5, background: '#0F1F35', borderRadius: 999 }}>
        <div style={{ width: `${pct}%`, height: '100%', background: color, borderRadius: 999 }} />
      </div>
      <span style={{ fontSize: 10, color, flexShrink: 0 }}>{pct}%</span>
    </div>
  )
}

const STATUS_SEQUENCE: StatusKey[] = ['open', 'reviewing', 'applied', 'dismissed', 'resolved']

export default function RecommendationsPage() {
  const [since, setSince] = useState('72h')
  const [filterStatus, setFilterStatus] = useState('')
  const [filterType, setFilterType] = useState('')
  const [selected, setSelected] = useState<Recommendation | null>(null)

  const { data, isLoading, error } = useRecommendations({
    since,
    limit: 50,
    status: filterStatus,
    type: filterType,
  })

  const updateStatus = useUpdateRecommendationStatus()

  const items = data?.items ?? []

  async function handleStatusChange(rec: Recommendation, next: StatusKey) {
    await updateStatus.mutateAsync({ id: rec.id, status: next })
    if (selected?.id === rec.id) setSelected({ ...rec, status: next })
  }

  return (
    <div style={{ padding: 32 }}>
      {/* Header */}
      <div style={{ marginBottom: 24 }}>
        <h1 style={titleStyle}>Recommendations</h1>
        <p style={subtleText}>Autonomic engine suggestions for cost control, policy tuning, routing optimisation, and rollout decisions.</p>
      </div>

      {/* Filters */}
      <div style={{ display: 'flex', gap: 12, marginBottom: 20, flexWrap: 'wrap' }}>
        <label style={labelStyle}>
          Window
          <select style={selectStyle} value={since} onChange={e => setSince(e.target.value)} id="recs-filter-since">
            <option value="24h">Last 24h</option>
            <option value="72h">Last 72h</option>
            <option value="168h">Last 7d</option>
            <option value="720h">Last 30d</option>
          </select>
        </label>
        <label style={labelStyle}>
          Status
          <select style={selectStyle} value={filterStatus} onChange={e => setFilterStatus(e.target.value)} id="recs-filter-status">
            <option value="">All</option>
            {STATUS_SEQUENCE.map(s => <option key={s} value={s}>{s}</option>)}
          </select>
        </label>
        <label style={labelStyle}>
          Type
          <select style={selectStyle} value={filterType} onChange={e => setFilterType(e.target.value)} id="recs-filter-type">
            <option value="">All</option>
            {['rollout', 'routing', 'policy', 'cost'].map(t => <option key={t} value={t}>{t}</option>)}
          </select>
        </label>
      </div>

      {/* Summary strip */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, minmax(0, 1fr))', gap: 16, marginBottom: 24 }}>
        {(['open', 'reviewing', 'applied', 'dismissed'] as StatusKey[]).map(s => {
          const cfg = STATUS_CFG[s]
          const count = items.filter(r => r.status === s).length
          return (
            <div key={s} style={{ background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 10, padding: '18px 20px' }}>
              <div style={{ fontSize: 10, color: '#475569', letterSpacing: '0.1em', marginBottom: 8 }}>{s.toUpperCase()}</div>
              <div style={{ fontSize: 26, fontWeight: 700, color: cfg.color }}>{count}</div>
            </div>
          )
        })}
      </div>

      {/* Two-pane layout */}
      <div style={{ display: 'grid', gridTemplateColumns: 'minmax(380px, 420px) 1fr', gap: 16 }}>
        {/* List */}
        <div style={panelStyle}>
          <div style={sectionLabel}>ALL RECOMMENDATIONS ({items.length})</div>
          {isLoading && <div style={subtleText}>Loading…</div>}
          {error && <div style={{ color: '#FCA5A5', fontSize: 12 }}>Failed to load recommendations.</div>}
          {!isLoading && !error && items.length === 0 && (
            <div style={{ ...subtleText, textAlign: 'center', padding: 40 }}>No recommendations in this window. The engine will surface signals as activity grows.</div>
          )}
          <div style={{ display: 'grid', gap: 8 }}>
            {items.map(rec => (
              <button
                key={rec.id}
                id={`rec-${rec.id}`}
                style={{
                  ...cardBtn,
                  borderColor: selected?.id === rec.id ? '#2563EB' : '#0F1F35',
                  background: selected?.id === rec.id ? '#0A1B33' : '#071525',
                }}
                onClick={() => setSelected(rec)}
              >
                <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8, alignItems: 'flex-start' }}>
                  <div style={{ color: '#E2E8F0', fontSize: 12, fontWeight: 600, textAlign: 'left', flex: 1 }}>{rec.title}</div>
                  {statusBadge(rec.status)}
                </div>
                <div style={{ marginTop: 8, display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
                  {typeBadge(rec.type)}
                  <span style={{ fontSize: 10, color: '#475569' }}>confidence</span>
                  {confidenceBar(rec.confidence)}
                </div>
                {rec.estimated_impact && (
                  <div style={{ fontSize: 10, color: '#64748B', marginTop: 6, textAlign: 'left' }}>Impact: {rec.estimated_impact}</div>
                )}
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
              <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, alignItems: 'flex-start', marginBottom: 16 }}>
                <div>
                  <div style={{ color: '#F0F9FF', fontSize: 18, fontWeight: 700 }}>{selected.title}</div>
                  <div style={{ display: 'flex', gap: 8, marginTop: 8, flexWrap: 'wrap', alignItems: 'center' }}>
                    {typeBadge(selected.type)}
                    {statusBadge(selected.status)}
                  </div>
                </div>
                <div style={{ textAlign: 'right', flexShrink: 0 }}>
                  <div style={{ fontSize: 10, color: '#475569', marginBottom: 4 }}>CONFIDENCE</div>
                  <div style={{ fontSize: 24, fontWeight: 700, color: selected.confidence >= 0.8 ? '#10B981' : selected.confidence >= 0.6 ? '#F59E0B' : '#EF4444' }}>
                    {Math.round(selected.confidence * 100)}%
                  </div>
                </div>
              </div>

              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12, marginBottom: 16 }}>
                <DetailStat label="Target" value={selected.target} />
                <DetailStat label="Blast Radius" value={selected.blast_radius ?? '—'} />
                <DetailStat label="Estimated Impact" value={selected.estimated_impact ?? '—'} />
                <DetailStat label="Last Seen" value={new Date(selected.last_seen_at).toLocaleString()} />
              </div>

              <div style={{ border: '1px solid #0F1F35', borderRadius: 8, padding: 14, background: '#071525', marginBottom: 16 }}>
                <div style={{ fontSize: 10, color: '#334155', letterSpacing: '0.1em', marginBottom: 8 }}>SUMMARY</div>
                <div style={{ fontSize: 12, color: '#CBD5E1', lineHeight: 1.6 }}>{selected.summary}</div>
              </div>

              <div style={{ border: '1px solid #0F1F35', borderRadius: 8, padding: 14, background: '#071525', marginBottom: 20 }}>
                <div style={{ fontSize: 10, color: '#334155', letterSpacing: '0.1em', marginBottom: 8 }}>SUGGESTED ACTION</div>
                <div style={{ fontSize: 12, color: '#10B981', lineHeight: 1.6 }}>{selected.suggested_action}</div>
              </div>

              {/* Status transitions */}
              <div style={sectionLabel}>UPDATE STATUS</div>
              <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                {STATUS_SEQUENCE.filter(s => s !== selected.status).map(next => (
                  <button
                    key={next}
                    id={`rec-status-${selected.id}-${next}`}
                    style={{
                      ...ghostBtn,
                      color: STATUS_CFG[next].color,
                      borderColor: `${STATUS_CFG[next].color}40`,
                    }}
                    onClick={() => handleStatusChange(selected, next)}
                    disabled={updateStatus.isPending}
                  >
                    → {next}
                  </button>
                ))}
              </div>

              {selected.evidence && Object.keys(selected.evidence).length > 0 && (
                <div style={{ marginTop: 20 }}>
                  <div style={sectionLabel}>EVIDENCE</div>
                  <pre style={codeStyle}>{JSON.stringify(selected.evidence, null, 2)}</pre>
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

function DetailStat({ label, value }: { label: string; value: string }) {
  return (
    <div style={{ border: '1px solid #0F1F35', borderRadius: 8, padding: '10px 14px', background: '#071525' }}>
      <div style={{ fontSize: 9, color: '#475569', letterSpacing: '0.1em', marginBottom: 4 }}>{label.toUpperCase()}</div>
      <div style={{ fontSize: 12, color: '#E2E8F0', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{value}</div>
    </div>
  )
}

const titleStyle: CSSProperties = { fontSize: 22, fontWeight: 700, color: '#F0F9FF', margin: 0 }
const subtleText: CSSProperties = { fontSize: 12, color: '#475569', marginTop: 4 }
const panelStyle: CSSProperties = { background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 10, padding: 24 }
const sectionLabel: CSSProperties = { fontSize: 10, color: '#334155', letterSpacing: '0.12em', marginBottom: 12 }
const labelStyle: CSSProperties = { display: 'grid', gap: 4, fontSize: 11, color: '#94A3B8' }
const selectStyle: CSSProperties = { background: '#060A14', border: '1px solid #0F1F35', borderRadius: 8, color: '#E2E8F0', padding: '8px 12px', fontSize: 12 }
const ghostBtn: CSSProperties = { background: 'none', border: '1px solid #0F1F35', borderRadius: 8, color: '#64748B', padding: '7px 14px', fontSize: 12, cursor: 'pointer' }
const cardBtn: CSSProperties = { border: '1px solid', borderRadius: 10, padding: 14, cursor: 'pointer', width: '100%', background: 'transparent' }
const codeStyle: CSSProperties = { margin: 0, padding: 12, borderRadius: 8, background: '#020817', color: '#CBD5E1', fontSize: 10, whiteSpace: 'pre-wrap', border: '1px solid #0F1F35' }
