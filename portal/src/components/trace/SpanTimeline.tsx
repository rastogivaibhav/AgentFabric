import { TraceTimeline, TraceTimelineItem } from '../../hooks/api'

function fmtDuration(ns: number) {
  if (ns < 1_000_000) return `${(ns / 1_000).toFixed(0)}us`
  if (ns < 1_000_000_000) return `${(ns / 1_000_000).toFixed(1)}ms`
  return `${(ns / 1_000_000_000).toFixed(2)}s`
}

function itemColor(item: TraceTimelineItem) {
  if (item.status === 'blocked') return '#F59E0B'
  if (item.status === 'error') return '#EF4444'
  if (item.step_type === 'llm') return '#3B82F6'
  if (item.step_type === 'tool') return '#10B981'
  if (item.step_type === 'policy') return '#A855F7'
  return '#64748B'
}

export default function SpanTimeline({
  timeline,
  selectedSpanId,
  onSelectSpan,
}: {
  timeline: TraceTimeline
  selectedSpanId?: string
  onSelectSpan?: (spanId: string) => void
}) {
  const totalDuration = timeline.duration_ns || 1

  return (
    <div style={{ background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 8, overflow: 'hidden' }}>
      <div style={{ display: 'flex', alignItems: 'center', height: 32, background: '#080C18', borderBottom: '1px solid #0F1F35', fontSize: 9, color: '#334155', letterSpacing: '0.1em' }}>
        <div style={{ width: 320, paddingLeft: 12 }}>SPAN NAME</div>
        <div style={{ width: 80, textAlign: 'right', paddingRight: 12 }}>DURATION</div>
        <div style={{ flex: 1, paddingLeft: 8 }}>TIMELINE</div>
        <div style={{ width: 90, textAlign: 'right', paddingRight: 12 }}>TOKENS</div>
        <div style={{ width: 90, textAlign: 'right', paddingRight: 12 }}>COST</div>
      </div>

      {(timeline.highlights ?? []).length > 0 && (
        <div style={{ padding: '10px 12px', borderBottom: '1px solid #0F1F35', background: '#071525', display: 'flex', flexWrap: 'wrap', gap: 8 }}>
          {(timeline.highlights ?? []).map(item => (
            <span key={item} style={{ padding: '3px 8px', borderRadius: 999, background: '#0F1F35', color: '#93C5FD', fontSize: 10 }}>
              {item}
            </span>
          ))}
        </div>
      )}

      {timeline.items.map(item => {
        const startPct = (item.start_offset_ns / totalDuration) * 100
        const widthPct = Math.max(0.4, (item.duration_ns / totalDuration) * 100)
        const selected = selectedSpanId === item.span_id
        const color = itemColor(item)
        return (
          <div
            key={item.span_id}
            onClick={() => onSelectSpan?.(item.span_id)}
            style={{
              display: 'flex',
              alignItems: 'center',
              minHeight: 30,
              borderBottom: '1px solid #0A1020',
              cursor: 'pointer',
              background: selected ? '#1E3A5F30' : 'transparent',
            }}
          >
            <div
              style={{
                width: 320,
                flexShrink: 0,
                padding: '6px 12px',
                paddingLeft: 12 + item.depth * 16,
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap',
                fontSize: 11,
                color: '#CBD5E1',
              }}
            >
              <span style={{ color, marginRight: 6 }}>o</span>
              {item.name}
              {item.blocked ? <span style={{ color: '#F59E0B' }}> [blocked]</span> : null}
              {item.redaction_count ? <span style={{ color: '#F59E0B' }}> [{item.redaction_count} redactions]</span> : null}
            </div>
            <div style={{ width: 80, textAlign: 'right', paddingRight: 12, fontSize: 10, color: '#64748B' }}>
              {fmtDuration(item.duration_ns)}
            </div>
            <div style={{ flex: 1, position: 'relative', height: 18 }}>
              <div
                style={{
                  position: 'absolute',
                  top: 3,
                  left: `${startPct}%`,
                  width: `${widthPct}%`,
                  minWidth: 3,
                  height: 12,
                  borderRadius: 3,
                  background: color,
                  opacity: 0.9,
                }}
              />
            </div>
            <div style={{ width: 90, textAlign: 'right', paddingRight: 12, fontSize: 10, color: '#94A3B8' }}>
              {item.total_tokens ? item.total_tokens.toLocaleString() : '-'}
            </div>
            <div style={{ width: 90, textAlign: 'right', paddingRight: 12, fontSize: 10, color: '#F59E0B' }}>
              {item.cost_usd ? `$${item.cost_usd.toFixed(6)}` : '-'}
            </div>
          </div>
        )
      })}
    </div>
  )
}
