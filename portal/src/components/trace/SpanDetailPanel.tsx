import { Link } from 'react-router-dom'
import { PolicyEvent, Span } from '../../hooks/api'

function fmtDuration(ns: number) {
  if (ns < 1_000_000) return `${(ns / 1_000).toFixed(0)}us`
  if (ns < 1_000_000_000) return `${(ns / 1_000_000).toFixed(1)}ms`
  return `${(ns / 1_000_000_000).toFixed(2)}s`
}

export default function SpanDetailPanel({
  span,
  policyEvents = [],
  title = 'SPAN DETAIL',
  showRaw = true,
}: {
  span: Span
  policyEvents?: PolicyEvent[]
  title?: string
  showRaw?: boolean
}) {
  return (
    <div style={{ padding: 16 }}>
      <div style={{ fontSize: 10, color: '#334155', letterSpacing: '0.1em', marginBottom: 12 }}>{title}</div>
      <div style={{ fontSize: 12, fontWeight: 600, color: '#F0F9FF', marginBottom: 12, wordBreak: 'break-word' }}>{span.name}</div>
      <div style={{ display: 'grid', gap: 8, marginBottom: 14 }}>
        {[
          ['Trace ID', span.trace_id],
          ['Span ID', span.id],
          ['Parent', span.parent_name ?? span.parent_id],
          ['Lineage', (span.lineage ?? []).join(' -> ')],
          ['Step', span.step_type],
          ['Model', span.model],
          ['Provider', span.provider],
          ['App', span.app_name],
          ['Environment', span.environment],
          ['User', span.user_id],
          ['Session', span.session_id],
          ['Prompt ID', span.prompt_id],
          ['Prompt Version', span.prompt_version != null && span.prompt_version > 0 ? `v${span.prompt_version}` : undefined],
          ['Prompt Release', span.prompt_release_tag],
          ['Prompt Environment', span.prompt_environment],
          ['Status', span.status_code === 2 ? 'ERROR' : 'OK'],
          ['Failure', span.failure_summary],
          ['Duration', span.duration_ns ? fmtDuration(span.duration_ns) : undefined],
          ['Tokens', span.input_tokens != null || span.output_tokens != null ? `${span.input_tokens ?? 0} in / ${span.output_tokens ?? 0} out` : undefined],
          ['Cache Read Tokens', span.cache_read_tokens ? String(span.cache_read_tokens) : undefined],
          ['Cache Write Tokens', span.cache_write_tokens ? String(span.cache_write_tokens) : undefined],
          ['Reasoning Tokens', span.reasoning_tokens ? String(span.reasoning_tokens) : undefined],
          ['Cost', span.cost_usd ? `$${span.cost_usd.toFixed(6)}` : undefined],
          ['Input Cost', span.input_cost_usd ? `$${span.input_cost_usd.toFixed(6)}` : undefined],
          ['Output Cost', span.output_cost_usd ? `$${span.output_cost_usd.toFixed(6)}` : undefined],
          ['Cache Read Cost', span.cache_read_cost_usd ? `$${span.cache_read_cost_usd.toFixed(6)}` : undefined],
          ['Cache Write Cost', span.cache_write_cost_usd ? `$${span.cache_write_cost_usd.toFixed(6)}` : undefined],
          ['Reasoning Cost', span.reasoning_cost_usd ? `$${span.reasoning_cost_usd.toFixed(6)}` : undefined],
          ['Retry Count', span.retry_count != null ? String(span.retry_count) : undefined],
          ['Blocked', span.blocked ? `yes${span.blocked_reason ? ` | ${span.blocked_reason}` : ''}` : 'no'],
          ['Redactions', span.redaction_count ? String(span.redaction_count) : undefined],
          ['Policy Decisions', span.policy_decision_count ? String(span.policy_decision_count) : undefined],
          ['Pricing Rule', span.pricing_rule_id ? `${span.pricing_rule_id} (${span.pricing_scope ?? 'global'})` : undefined],
          ['Pricing Pattern', span.pricing_model_pattern],
        ]
          .filter(([, value]) => value !== undefined && value !== '')
          .map(([label, value]) => (
            <div key={label}>
              <div style={{ fontSize: 9, color: '#3B82F6' }}>{label}</div>
              <div style={{ fontSize: 10, color: '#CBD5E1', wordBreak: 'break-word' }}>{value || '-'}</div>
            </div>
          ))}
      </div>

      {span.policy_decision_summary && span.policy_decision_summary.length > 0 && (
        <div style={{ marginBottom: 12 }}>
          <div style={{ fontSize: 9, color: '#3B82F6', marginBottom: 6 }}>Decision Summary</div>
          <div style={{ display: 'grid', gap: 6 }}>
            {span.policy_decision_summary.map(item => (
              <div key={item} style={{ fontSize: 10, color: '#94A3B8' }}>{item}</div>
            ))}
          </div>
        </div>
      )}

      {span.prompt_preview && (
        <div style={{ marginBottom: 12 }}>
          <div style={{ fontSize: 9, color: '#3B82F6' }}>Prompt Preview</div>
          <div style={{ fontSize: 10, color: '#94A3B8', whiteSpace: 'pre-wrap' }}>{span.prompt_preview}</div>
        </div>
      )}

      {span.prompt_id && (
        <div style={{ marginBottom: 12 }}>
          <div style={{ fontSize: 9, color: '#3B82F6' }}>Prompt Lifecycle</div>
          <div style={{ fontSize: 10, color: '#94A3B8', marginTop: 4 }}>
            <Link to={`/prompts/${encodeURIComponent(span.prompt_id)}`} style={{ color: '#60A5FA', textDecoration: 'none' }}>
              Open {span.prompt_id}
            </Link>
            {span.prompt_version ? ` · v${span.prompt_version}` : ''}
            {span.prompt_release_tag ? ` · ${span.prompt_release_tag}` : ''}
          </div>
        </div>
      )}

      {span.response_preview && (
        <div style={{ marginBottom: 12 }}>
          <div style={{ fontSize: 9, color: '#3B82F6' }}>Response Preview</div>
          <div style={{ fontSize: 10, color: '#94A3B8', whiteSpace: 'pre-wrap' }}>{span.response_preview}</div>
        </div>
      )}

      {policyEvents.length > 0 && (
        <div style={{ marginBottom: 12 }}>
          <div style={{ fontSize: 9, color: '#3B82F6', marginBottom: 6 }}>Policy Decisions</div>
          <div style={{ display: 'grid', gap: 8 }}>
            {policyEvents.map(event => (
              <div key={event.decision_id} style={{ border: '1px solid #0F1F35', borderRadius: 6, padding: 10, background: '#071525' }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8 }}>
                  <div style={{ fontSize: 10, color: '#E2E8F0', fontWeight: 600 }}>{event.policy_name}</div>
                  <div style={{ fontSize: 9, color: event.result === 'deny' ? '#EF4444' : event.result === 'warn' ? '#F59E0B' : '#10B981' }}>
                    {event.result}
                  </div>
                </div>
                <div style={{ fontSize: 10, color: '#94A3B8', marginTop: 4 }}>{event.reason || 'No reason recorded'}</div>
              </div>
            ))}
          </div>
        </div>
      )}

      {showRaw && Object.entries(span.attributes ?? {}).slice(0, 30).map(([key, value]) => (
        <div key={key} style={{ marginBottom: 6 }}>
          <div style={{ fontSize: 9, color: '#3B82F6' }}>{key}</div>
          <div style={{ fontSize: 10, color: '#64748B', wordBreak: 'break-all', paddingLeft: 8 }}>{value}</div>
        </div>
      ))}
    </div>
  )
}
