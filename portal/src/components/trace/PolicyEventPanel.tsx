import { PolicyEvent } from '../../hooks/api'

function policyColor(result: string) {
  switch (result) {
    case 'deny':
      return '#EF4444'
    case 'warn':
      return '#F59E0B'
    case 'sanitize':
    case 'redact':
      return '#C084FC'
    default:
      return '#10B981'
  }
}

export default function PolicyEventPanel({
  title = 'POLICY DECISIONS',
  events,
  emptyLabel,
}: {
  title?: string
  events: PolicyEvent[]
  emptyLabel?: string
}) {
  if (events.length === 0) {
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
        {events.map(event => (
          <div key={event.decision_id} style={{ border: '1px solid #0F1F35', borderRadius: 6, padding: 10, background: '#0D1B2A' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8 }}>
              <div style={{ fontSize: 11, color: '#E2E8F0', fontWeight: 600 }}>{event.policy_name}</div>
              <div style={{ fontSize: 10, color: policyColor(event.result) }}>{event.result.toUpperCase()}</div>
            </div>
            <div style={{ fontSize: 10, color: '#94A3B8', marginTop: 4 }}>{event.reason || 'No reason recorded'}</div>
            <div style={{ fontSize: 10, color: '#64748B', marginTop: 4 }}>
              {(event.provider || 'unknown provider') + (event.model ? ` | ${event.model}` : '') + (event.scope ? ` | ${event.scope}` : '')}
              {event.redactions ? ` | redactions ${event.redactions}` : ''}
            </div>
            {(event.matched ?? []).length > 0 && (
              <div style={{ fontSize: 10, color: '#475569', marginTop: 4 }}>
                matched: {(event.matched ?? []).join(', ')}
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  )
}
