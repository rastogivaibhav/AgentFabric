import type { CSSProperties } from 'react'
import type { PolicyPreviewDecision } from '../hooks/api'

interface Props {
  label: string
  decision: PolicyPreviewDecision
}

export default function PolicyDecisionExplorer({ label, decision }: Props) {
  return (
    <div style={panelStyle}>
      <div style={{ color: '#E2E8F0', fontSize: 12, fontWeight: 700 }}>{label}</div>
      <div style={{ color: decision.matched ? '#10B981' : '#64748B', fontSize: 11, marginTop: 6 }}>
        {decision.matched ? `${decision.action || 'matched'}${decision.policy_name ? ` via ${decision.policy_name}` : ''}` : 'no matching rule'}
      </div>
      {decision.reason && <div style={textStyle}>{decision.reason}</div>}
      {decision.explain && <div style={textStyle}>{decision.explain}</div>}
      {decision.engine && (
        <div style={metaStyle}>
          engine {decision.engine}
          {decision.decision_mode ? ` | mode ${decision.decision_mode}` : ''}
          {decision.version ? ` | v${decision.version}` : ''}
          {decision.rollout_percent ? ` | rollout ${decision.rollout_percent}%` : ''}
          {decision.final !== undefined ? ` | final ${decision.final ? 'yes' : 'no'}` : ''}
        </div>
      )}
      {decision.evaluation_path && decision.evaluation_path.length > 0 && (
        <div style={metaStyle}>path: {decision.evaluation_path.join(' -> ')}</div>
      )}
      {decision.matched_fields && decision.matched_fields.length > 0 && (
        <div style={metaStyle}>matched fields: {decision.matched_fields.join(', ')}</div>
      )}
      {decision.matched_names && decision.matched_names.length > 0 && (
        <div style={metaStyle}>detectors: {decision.matched_names.join(', ')}</div>
      )}
      {decision.guardrail_matches && decision.guardrail_matches.length > 0 && (
        <div style={metaStyle}>guardrails: {decision.guardrail_matches.join(', ')}</div>
      )}
      {decision.rule_conditions && Object.keys(decision.rule_conditions).length > 0 && (
        <div style={{ marginTop: 8 }}>
          <div style={metaStyle}>rule conditions</div>
          <pre style={codeStyle}>{JSON.stringify(decision.rule_conditions, null, 2)}</pre>
        </div>
      )}
      {decision.rego_query && (
        <div style={{ marginTop: 8 }}>
          <div style={metaStyle}>rego-style query</div>
          <pre style={codeStyle}>{decision.rego_query}</pre>
        </div>
      )}
      {decision.condition_trace && decision.condition_trace.length > 0 && (
        <div style={{ marginTop: 8, display: 'grid', gap: 6 }}>
          {decision.condition_trace.map((trace, index) => (
            <div key={`${trace.field}-${index}`} style={traceRowStyle}>
              <div style={{ color: trace.matched ? '#10B981' : '#F97316', fontSize: 10, fontWeight: 700 }}>
                {trace.matched ? 'MATCH' : 'MISS'}
              </div>
              <div style={{ color: '#CBD5E1', fontSize: 11 }}>
                {trace.field} {trace.operator} {trace.expected || 'n/a'}
              </div>
              <div style={metaStyle}>actual: {trace.actual || 'n/a'}</div>
              {trace.source && <div style={metaStyle}>source: {trace.source}</div>}
            </div>
          ))}
        </div>
      )}
      {decision.redacted_preview && (
        <div style={{ marginTop: 8 }}>
          <div style={metaStyle}>redacted preview</div>
          <pre style={codeStyle}>{decision.redacted_preview}</pre>
        </div>
      )}
    </div>
  )
}

const panelStyle: CSSProperties = {
  border: '1px solid #0F1F35',
  borderRadius: 8,
  padding: 12,
  background: '#071525',
}

const textStyle: CSSProperties = {
  color: '#94A3B8',
  fontSize: 11,
  marginTop: 6,
}

const metaStyle: CSSProperties = {
  color: '#64748B',
  fontSize: 10,
  marginTop: 6,
}

const codeStyle: CSSProperties = {
  margin: '6px 0 0',
  padding: 8,
  borderRadius: 6,
  background: '#020817',
  color: '#CBD5E1',
  fontSize: 10,
  whiteSpace: 'pre-wrap',
}

const traceRowStyle: CSSProperties = {
  padding: 8,
  borderRadius: 6,
  background: '#020817',
}
