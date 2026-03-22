import { PolicyEvent, Trace } from '../../hooks/api'

function fmtDuration(ns: number) {
  if (ns < 1_000_000) return `${(ns / 1_000).toFixed(0)}us`
  if (ns < 1_000_000_000) return `${(ns / 1_000_000).toFixed(1)}ms`
  return `${(ns / 1_000_000_000).toFixed(2)}s`
}

function statCard(label: string, value: string, color: string) {
  return (
    <div key={label} style={{ background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 6, padding: '8px 14px' }}>
      <div style={{ fontSize: 9, color: '#334155', letterSpacing: '0.1em' }}>{label}</div>
      <div style={{ fontSize: 13, color, fontWeight: 600 }}>{value}</div>
    </div>
  )
}

function infoPanel(title: string, value: string) {
  return (
    <div key={title} style={{ background: '#071525', border: '1px solid #0F1F35', borderRadius: 8, padding: 12 }}>
      <div style={{ fontSize: 9, color: '#334155', letterSpacing: '0.1em', marginBottom: 6 }}>{title}</div>
      <div style={{ fontSize: 11, color: '#CBD5E1' }}>{value || '-'}</div>
    </div>
  )
}

export default function TraceHeader({
  trace,
  policyEvents,
  frameworkColor,
}: {
  trace: Trace
  policyEvents: PolicyEvent[]
  frameworkColor: string
}) {
  const insights = trace.insights ?? {}
  const stepMix = Object.entries(insights.step_types ?? {})
    .map(([key, value]) => `${key}:${value}`)
    .join(' | ')
  const policyMix = Object.entries(insights.policy_results ?? {})
    .map(([key, value]) => `${key}:${value}`)
    .join(' | ')

  return (
    <div style={{ marginBottom: 20 }}>
      <div style={{ fontSize: 11, color: '#475569', marginBottom: 4 }}>TRACE</div>
      <div style={{ fontSize: 13, color: '#3B82F6', fontFamily: 'monospace', marginBottom: 12, wordBreak: 'break-all' }}>{trace.id}</div>
      <div style={{ display: 'flex', gap: 12, flexWrap: 'wrap' }}>
        {[
          statCard('Framework', trace.framework, frameworkColor),
          statCard('Duration', fmtDuration(trace.duration_ns), '#94A3B8'),
          statCard('Spans', String(trace.span_count), '#94A3B8'),
          statCard('Errors', String(trace.error_count), trace.error_count > 0 ? '#EF4444' : '#10B981'),
          statCard('Tokens', trace.total_tokens.toLocaleString(), '#94A3B8'),
          statCard('Cost', `$${trace.total_cost_usd.toFixed(6)}`, '#F59E0B'),
          statCard('LLM Calls', String(insights.llm_calls ?? 0), '#60A5FA'),
          statCard('Blocked', String(insights.blocked_spans ?? 0), (insights.blocked_spans ?? 0) > 0 ? '#EF4444' : '#10B981'),
          statCard('Redacted', String(insights.redacted_spans ?? 0), (insights.redacted_spans ?? 0) > 0 ? '#F59E0B' : '#10B981'),
          statCard('Policy Events', String(policyEvents.length), policyEvents.length > 0 ? '#C084FC' : '#94A3B8'),
        ]}
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))', gap: 12, marginTop: 12 }}>
        {infoPanel('MODELS', (insights.models ?? []).join(', '))}
        {infoPanel(
          'PROVIDERS / ENV',
          `${(insights.providers ?? []).join(', ') || '-'}${(insights.environments?.length ?? 0) > 0 ? ` | ${(insights.environments ?? []).join(', ')}` : ''}`,
        )}
        {infoPanel('STEP MIX', stepMix)}
        {infoPanel('POLICY RESULTS', policyMix)}
        {infoPanel('WORKFLOW', (insights.workflow_summary ?? []).join(' -> '))}
      </div>
    </div>
  )
}
