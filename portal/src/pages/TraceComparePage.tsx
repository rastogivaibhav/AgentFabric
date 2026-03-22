import { useMemo } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { useTraceComparison } from '../hooks/api'

function fmtDuration(ns: number) {
  if (ns < 1_000_000) return `${(ns / 1_000).toFixed(0)}us`
  if (ns < 1_000_000_000) return `${(ns / 1_000_000).toFixed(1)}ms`
  return `${(ns / 1_000_000_000).toFixed(2)}s`
}

export default function TraceComparePage() {
  const [params] = useSearchParams()
  const left = params.get('left') ?? ''
  const right = params.get('right') ?? ''
  const { data, isLoading } = useTraceComparison(left, right)

  const cards = useMemo(() => {
    if (!data) return []
    return [
      { label: 'Duration', left: fmtDuration(data.left.duration_ns), right: fmtDuration(data.right.duration_ns) },
      { label: 'Spans', left: String(data.left.span_count), right: String(data.right.span_count) },
      { label: 'Tokens', left: data.left.total_tokens.toLocaleString(), right: data.right.total_tokens.toLocaleString() },
      { label: 'Cost', left: `$${data.left.total_cost_usd.toFixed(6)}`, right: `$${data.right.total_cost_usd.toFixed(6)}` },
      { label: 'Status', left: data.left.status, right: data.right.status },
      { label: 'Blocked spans', left: String(data.left.blocked_spans ?? 0), right: String(data.right.blocked_spans ?? 0) },
    ]
  }, [data])

  if (!left || !right) {
    return <div style={{ padding: 32, color: '#94A3B8' }}>Choose two traces from the traces page to compare them.</div>
  }
  if (isLoading) {
    return <div style={{ padding: 32, color: '#94A3B8' }}>Loading comparison...</div>
  }
  if (!data) {
    return <div style={{ padding: 32, color: '#EF4444' }}>Comparison could not be loaded.</div>
  }

  return (
    <div style={{ padding: 24, display: 'grid', gap: 16 }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <div>
          <h1 style={{ margin: 0, color: '#F0F9FF', fontSize: 20 }}>Trace Comparison</h1>
          <div style={{ marginTop: 8, fontSize: 11, color: '#64748B' }}>
            <Link to={`/traces/${left}`} style={{ color: '#60A5FA', textDecoration: 'none' }}>{left}</Link>
            {' '}vs{' '}
            <Link to={`/traces/${right}`} style={{ color: '#60A5FA', textDecoration: 'none' }}>{right}</Link>
          </div>
        </div>
      </div>

      {(data.highlights ?? []).length > 0 && (
        <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
          {data.highlights?.map(item => (
            <span key={item} style={{ padding: '4px 10px', borderRadius: 999, background: '#102742', color: '#93C5FD', fontSize: 10 }}>
              {item}
            </span>
          ))}
        </div>
      )}

      <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'repeat(auto-fit, minmax(240px, 1fr))' }}>
        {cards.map(card => (
          <div key={card.label} style={{ background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 8, padding: 14 }}>
            <div style={{ fontSize: 10, color: '#3B82F6', marginBottom: 8 }}>{card.label.toUpperCase()}</div>
            <div style={{ display: 'grid', gap: 6, fontSize: 12 }}>
              <div style={{ color: '#CBD5E1' }}>Left: {card.left}</div>
              <div style={{ color: '#94A3B8' }}>Right: {card.right}</div>
            </div>
          </div>
        ))}
      </div>

      <div style={{ background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 8, overflow: 'hidden' }}>
        <div style={{ display: 'grid', gridTemplateColumns: '180px 1fr 1fr 100px', background: '#080C18', borderBottom: '1px solid #0F1F35' }}>
          {['Field', 'Left', 'Right', 'Severity'].map(label => (
            <div key={label} style={{ padding: '10px 12px', color: '#334155', fontSize: 10, letterSpacing: '0.1em' }}>{label}</div>
          ))}
        </div>
        {(data.diffs ?? []).map(diff => (
          <div key={diff.field} style={{ display: 'grid', gridTemplateColumns: '180px 1fr 1fr 100px', borderBottom: '1px solid #0A1020' }}>
            <div style={{ padding: '9px 12px', color: '#93C5FD', fontSize: 11 }}>{diff.field}</div>
            <div style={{ padding: '9px 12px', color: '#CBD5E1', fontSize: 11 }}>{diff.left || '-'}</div>
            <div style={{ padding: '9px 12px', color: '#CBD5E1', fontSize: 11 }}>{diff.right || '-'}</div>
            <div style={{ padding: '9px 12px', color: diff.severity === 'high' ? '#EF4444' : diff.severity === 'medium' ? '#F59E0B' : '#10B981', fontSize: 11 }}>
              {diff.severity}
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}
