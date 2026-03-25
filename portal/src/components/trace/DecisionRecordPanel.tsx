import type { DecisionRecord } from '../../hooks/api'

function resultColor(result: string) {
  switch ((result || '').toLowerCase()) {
    case 'deny':
    case 'error':
      return '#EF4444'
    case 'warn':
    case 'retry':
      return '#F59E0B'
    case 'sanitize':
    case 'reroute':
      return '#C084FC'
    default:
      return '#10B981'
  }
}

export default function DecisionRecordPanel({
  title = 'DECISION EVIDENCE',
  records,
  emptyLabel,
}: {
  title?: string
  records: DecisionRecord[]
  emptyLabel?: string
}) {
  if (records.length === 0) {
    return emptyLabel ? (
      <div style={{ background: '#071525', border: '1px solid #0F1F35', borderRadius: 8, padding: 12 }}>
        <div style={{ fontSize: 9, color: '#334155', letterSpacing: '0.1em', marginBottom: 8 }}>{title}</div>
        <div style={{ fontSize: 10, color: '#64748B' }}>{emptyLabel}</div>
      </div>
    ) : null
  }

  return (
    <div style={{ background: '#071525', border: '1px solid #0F1F35', borderRadius: 8, padding: 12 }}>
      <div style={{ fontSize: 9, color: '#334155', letterSpacing: '0.1em', marginBottom: 8 }}>{title}</div>
      <div style={{ display: 'grid', gap: 8 }}>
        {records.map(record => (
          <div key={`${record.decision_id}-${record.id}`} style={{ border: '1px solid #0F1F35', borderRadius: 6, padding: 10, background: '#0D1B2A' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8 }}>
              <div style={{ fontSize: 11, color: '#E2E8F0', fontWeight: 600 }}>
                {(record.type || 'decision').toUpperCase()}
                {record.source ? ` | ${record.source}` : ''}
              </div>
              <div style={{ fontSize: 10, color: resultColor(record.result) }}>{record.result.toUpperCase()}</div>
            </div>
            <div style={{ fontSize: 10, color: '#CBD5E1', marginTop: 6 }}>
              {record.explanation || record.reason || 'No explanation recorded'}
            </div>
            <div style={{ fontSize: 10, color: '#64748B', marginTop: 4 }}>
              {(record.provider || 'unknown provider') + (record.model ? ` | ${record.model}` : '') + (record.environment ? ` | ${record.environment}` : '')}
              {record.action_taken ? ` | action ${record.action_taken}` : ''}
            </div>
            {record.trigger && (
              <div style={{ fontSize: 10, color: '#475569', marginTop: 4 }}>trigger: {record.trigger}</div>
            )}
            {record.evidence && Object.keys(record.evidence).length > 0 && (
              <pre style={{ margin: '6px 0 0', padding: 8, borderRadius: 6, background: '#020817', color: '#94A3B8', fontSize: 10, whiteSpace: 'pre-wrap' }}>
                {JSON.stringify(record.evidence, null, 2)}
              </pre>
            )}
          </div>
        ))}
      </div>
    </div>
  )
}
