import { useMemo, useState } from 'react'
import { useParams } from 'react-router-dom'
import { Span, useTrace } from '../hooks/api'
import TopologyGraph from '../components/TopologyGraph'

const FW_COLORS: Record<string, string> = {
  crewai: '#FF6B35',
  langgraph: '#4ECDC4',
  google_adk: '#4285F4',
  openai_agents: '#10A37F',
  claude_agents: '#D97706',
  unknown: '#475569',
}

function fmtDuration(ns: number) {
  if (ns < 1_000_000) return `${(ns / 1_000).toFixed(0)}us`
  if (ns < 1_000_000_000) return `${(ns / 1_000_000).toFixed(1)}ms`
  return `${(ns / 1_000_000_000).toFixed(2)}s`
}

function WaterfallRow({
  span,
  minTime,
  totalDuration,
  depth,
}: {
  span: Span
  minTime: number
  totalDuration: number
  depth: number
}) {
  const [hovered, setHovered] = useState(false)
  const startPct = totalDuration > 0 ? ((span.start_time_ns - minTime) / totalDuration) * 100 : 0
  const widthPct = totalDuration > 0 ? Math.max(0.3, (span.duration_ns / totalDuration) * 100) : 0
  const color = FW_COLORS[span.framework] ?? '#3B82F6'
  const isError = span.status_code === 2

  return (
    <div
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      style={{
        display: 'flex',
        alignItems: 'center',
        height: 28,
        borderBottom: '1px solid #0A1020',
        position: 'relative',
        background: hovered ? '#1E3A5F15' : 'transparent',
        cursor: 'pointer',
      }}
    >
      <div
        style={{
          width: 280,
          flexShrink: 0,
          padding: '0 12px',
          paddingLeft: 12 + depth * 16,
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
          fontSize: 11,
          color: isError ? '#EF4444' : '#CBD5E1',
          display: 'flex',
          alignItems: 'center',
          gap: 6,
        }}
      >
        {depth > 0 && <span style={{ color: '#1E3A5F' }}>-</span>}
        <span style={{ fontSize: 9, color, marginRight: 2 }}>o</span>
        {span.name}
      </div>
      <div style={{ width: 70, flexShrink: 0, fontSize: 10, color: '#475569', textAlign: 'right', paddingRight: 12 }}>
        {fmtDuration(span.duration_ns)}
      </div>
      <div style={{ flex: 1, position: 'relative', height: 16 }}>
        <div
          style={{
            position: 'absolute',
            top: 2,
            height: 12,
            borderRadius: 2,
            left: `${startPct}%`,
            width: `${widthPct}%`,
            background: isError ? '#EF4444' : color,
            opacity: 0.8,
            minWidth: 2,
          }}
        />
      </div>
      <div style={{ width: 80, flexShrink: 0, fontSize: 10, color: '#F59E0B', textAlign: 'right', paddingRight: 12 }}>
        {span.cost_usd ? `$${span.cost_usd.toFixed(6)}` : ''}
      </div>
    </div>
  )
}

function buildSpanTree(spans: Span[]) {
  const byId: Record<string, Span & { children?: Span[] }> = {}
  for (const span of spans) byId[span.id] = { ...span, children: [] }
  const roots: (Span & { children?: Span[] })[] = []
  for (const span of spans) {
    const parentId = span.parent_id ?? span.attributes?.parent_span_id
    if (parentId && byId[parentId]) {
      byId[parentId].children!.push(byId[span.id])
    } else {
      roots.push(byId[span.id])
    }
  }
  return roots
}

function flattenTree(nodes: (Span & { children?: Span[] })[], depth = 0): { span: Span; depth: number }[] {
  const result: { span: Span; depth: number }[] = []
  for (const node of nodes) {
    result.push({ span: node, depth })
    if (node.children?.length) result.push(...flattenTree(node.children, depth + 1))
  }
  return result
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

export default function TraceDetail() {
  const { traceId } = useParams<{ traceId: string }>()
  const { data: trace, isLoading } = useTrace(traceId!)
  const [tab, setTab] = useState<'waterfall' | 'spans' | 'graph'>('waterfall')
  const [selected, setSelected] = useState<Span | null>(null)

  if (isLoading) return <div style={{ padding: 32, color: '#475569' }}>Loading trace...</div>
  if (!trace) return <div style={{ padding: 32, color: '#EF4444' }}>Trace not found</div>

  const spans = trace.spans ?? []
  const policyEvents = trace.policy_events ?? []
  const minTime = spans.length > 0 ? Math.min(...spans.map(span => span.start_time_ns)) : 0
  const maxTime = spans.length > 0 ? Math.max(...spans.map(span => span.start_time_ns + span.duration_ns)) : 0
  const totalDuration = maxTime - minTime
  const tree = buildSpanTree(spans)
  const flatSpans = flattenTree(tree)
  const insights = trace.insights ?? {}

  const selectedPolicyEvents = useMemo(
    () => (selected ? policyEvents.filter(event => event.span_id === selected.id) : []),
    [policyEvents, selected],
  )

  const stepMix = Object.entries(insights.step_types ?? {})
    .map(([key, value]) => `${key}:${value}`)
    .join(' | ')

  return (
    <div style={{ padding: 24, display: 'flex', gap: 16, height: '100%', overflow: 'hidden' }}>
      <div style={{ flex: 1, overflow: 'auto' }}>
        <div style={{ marginBottom: 20 }}>
          <div style={{ fontSize: 11, color: '#475569', marginBottom: 4 }}>TRACE</div>
          <div style={{ fontSize: 13, color: '#3B82F6', fontFamily: 'monospace', marginBottom: 12, wordBreak: 'break-all' }}>{trace.id}</div>
          <div style={{ display: 'flex', gap: 12, flexWrap: 'wrap' }}>
            {[
              statCard('Framework', trace.framework, FW_COLORS[trace.framework] ?? '#475569'),
              statCard('Duration', fmtDuration(trace.duration_ns), '#94A3B8'),
              statCard('Spans', String(trace.span_count), '#94A3B8'),
              statCard('Errors', String(trace.error_count), trace.error_count > 0 ? '#EF4444' : '#10B981'),
              statCard('Tokens', trace.total_tokens.toLocaleString(), '#94A3B8'),
              statCard('Cost', `$${trace.total_cost_usd.toFixed(6)}`, '#F59E0B'),
              statCard('LLM Calls', String(insights.llm_calls ?? 0), '#60A5FA'),
              statCard('Blocked', String(insights.blocked_spans ?? 0), (insights.blocked_spans ?? 0) > 0 ? '#EF4444' : '#10B981'),
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
          </div>

          {policyEvents.length > 0 && (
            <div style={{ background: '#071525', border: '1px solid #0F1F35', borderRadius: 8, padding: 12, marginTop: 12 }}>
              <div style={{ fontSize: 9, color: '#334155', letterSpacing: '0.1em', marginBottom: 8 }}>POLICY DECISIONS</div>
              <div style={{ display: 'grid', gap: 8 }}>
                {policyEvents.slice(0, 6).map(event => (
                  <div key={event.decision_id} style={{ border: '1px solid #0F1F35', borderRadius: 6, padding: 10, background: '#0D1B2A' }}>
                    <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8 }}>
                      <div style={{ fontSize: 11, color: '#E2E8F0', fontWeight: 600 }}>{event.policy_name}</div>
                      <div
                        style={{
                          fontSize: 10,
                          color:
                            event.result === 'deny'
                              ? '#EF4444'
                              : event.result === 'warn'
                                ? '#F59E0B'
                                : '#10B981',
                        }}
                      >
                        {event.result.toUpperCase()}
                      </div>
                    </div>
                    <div style={{ fontSize: 10, color: '#94A3B8', marginTop: 4 }}>{event.reason || 'No reason recorded'}</div>
                    <div style={{ fontSize: 10, color: '#64748B', marginTop: 4 }}>
                      {(event.provider || 'unknown provider') + (event.model ? ` | ${event.model}` : '') + (event.scope ? ` | ${event.scope}` : '')}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>

        <div style={{ display: 'flex', gap: 4, marginBottom: 16 }}>
          {(['waterfall', 'spans', 'graph'] as const).map(currentTab => (
            <button
              key={currentTab}
              onClick={() => setTab(currentTab)}
              style={{
                padding: '6px 14px',
                borderRadius: 5,
                fontSize: 11,
                cursor: 'pointer',
                background: tab === currentTab ? '#1E3A5F' : 'transparent',
                border: `1px solid ${tab === currentTab ? '#3B82F6' : '#1E3A5F'}`,
                color: tab === currentTab ? '#60A5FA' : '#475569',
              }}
            >
              {currentTab.charAt(0).toUpperCase() + currentTab.slice(1)}
            </button>
          ))}
        </div>

        {tab === 'waterfall' && (
          <div style={{ background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 8, overflow: 'hidden' }}>
            <div style={{ display: 'flex', alignItems: 'center', height: 32, background: '#080C18', borderBottom: '1px solid #0F1F35', fontSize: 9, color: '#334155', letterSpacing: '0.1em' }}>
              <div style={{ width: 280, paddingLeft: 12 }}>SPAN NAME</div>
              <div style={{ width: 70, textAlign: 'right', paddingRight: 12 }}>DURATION</div>
              <div style={{ flex: 1, paddingLeft: 8 }}>TIMELINE</div>
              <div style={{ width: 80, textAlign: 'right', paddingRight: 12 }}>COST</div>
            </div>
            {flatSpans.map(({ span, depth }) => (
              <div key={span.id} onClick={() => setSelected(selected?.id === span.id ? null : span)}>
                <WaterfallRow span={span} minTime={minTime} totalDuration={totalDuration} depth={depth} />
              </div>
            ))}
          </div>
        )}

        {tab === 'spans' && (
          <div style={{ background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 8, overflow: 'hidden' }}>
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 11 }}>
              <thead>
                <tr style={{ borderBottom: '1px solid #0F1F35' }}>
                  {['Span ID', 'Name', 'Step', 'Framework', 'Duration', 'Status', 'Tokens', 'Cost'].map(header => (
                    <th key={header} style={{ padding: '9px 12px', textAlign: 'left', color: '#334155', fontSize: 10, letterSpacing: '0.08em', fontWeight: 700 }}>
                      {header}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {spans.map((span, index) => (
                  <tr
                    key={span.id}
                    onClick={() => setSelected(selected?.id === span.id ? null : span)}
                    style={{ borderBottom: '1px solid #0A1020', cursor: 'pointer', background: index % 2 === 0 ? 'transparent' : '#060A1430' }}
                  >
                    <td style={{ padding: '7px 12px', color: '#3B82F6', fontFamily: 'monospace' }}>{span.id.substring(0, 12)}...</td>
                    <td style={{ padding: '7px 12px', color: '#CBD5E1' }}>{span.name}</td>
                    <td style={{ padding: '7px 12px', color: '#60A5FA' }}>{span.step_type ?? '-'}</td>
                    <td style={{ padding: '7px 12px' }}>
                      <span style={{ fontSize: 9, color: FW_COLORS[span.framework] ?? '#475569' }}>{span.framework}</span>
                    </td>
                    <td style={{ padding: '7px 12px', color: '#64748B' }}>{fmtDuration(span.duration_ns)}</td>
                    <td style={{ padding: '7px 12px' }}>
                      <span
                        style={{
                          fontSize: 9,
                          padding: '1px 5px',
                          borderRadius: 3,
                          background: span.status_code === 2 ? '#EF444420' : '#10B98120',
                          color: span.status_code === 2 ? '#EF4444' : '#10B981',
                        }}
                      >
                        {span.status_code === 2 ? 'ERROR' : 'OK'}
                      </span>
                    </td>
                    <td style={{ padding: '7px 12px', color: '#64748B' }}>{(span.input_tokens || 0) + (span.output_tokens || 0)}</td>
                    <td style={{ padding: '7px 12px', color: '#F59E0B' }}>{span.cost_usd ? `$${span.cost_usd.toFixed(6)}` : '-'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        {tab === 'graph' && (
          <TopologyGraph
            spans={spans}
            selectedSpanId={selected?.id}
            onSelectSpan={spanId => {
              const span = spans.find(item => item.id === spanId) ?? null
              setSelected(prev => (prev?.id === spanId ? null : span))
            }}
          />
        )}
      </div>

      {selected && (
        <div style={{ width: 300, background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 8, overflow: 'auto', padding: 16, flexShrink: 0 }}>
          <div style={{ fontSize: 10, color: '#334155', letterSpacing: '0.1em', marginBottom: 12 }}>SPAN DETAIL</div>
          <div style={{ fontSize: 12, fontWeight: 600, color: '#F0F9FF', marginBottom: 12 }}>{selected.name}</div>
          <div style={{ display: 'grid', gap: 8, marginBottom: 14 }}>
            {[
              ['Step', selected.step_type],
              ['Model', selected.model],
              ['Provider', selected.provider],
              ['App', selected.app_name],
              ['Environment', selected.environment],
              ['User', selected.user_id],
              ['Session', selected.session_id],
              ['Retry Count', selected.retry_count != null ? String(selected.retry_count) : undefined],
              ['Blocked', selected.blocked ? `yes${selected.blocked_reason ? ` | ${selected.blocked_reason}` : ''}` : 'no'],
              ['Pricing Rule', selected.pricing_rule_id ? `${selected.pricing_rule_id} (${selected.pricing_scope ?? 'global'})` : undefined],
            ]
              .filter(([, value]) => value !== undefined)
              .map(([label, value]) => (
                <div key={label}>
                  <div style={{ fontSize: 9, color: '#3B82F6' }}>{label}</div>
                  <div style={{ fontSize: 10, color: '#CBD5E1', wordBreak: 'break-word' }}>{value || '-'}</div>
                </div>
              ))}
          </div>

          {selected.prompt_preview && (
            <div style={{ marginBottom: 12 }}>
              <div style={{ fontSize: 9, color: '#3B82F6' }}>Prompt Preview</div>
              <div style={{ fontSize: 10, color: '#94A3B8', whiteSpace: 'pre-wrap' }}>{selected.prompt_preview}</div>
            </div>
          )}

          {selected.response_preview && (
            <div style={{ marginBottom: 12 }}>
              <div style={{ fontSize: 9, color: '#3B82F6' }}>Response Preview</div>
              <div style={{ fontSize: 10, color: '#94A3B8', whiteSpace: 'pre-wrap' }}>{selected.response_preview}</div>
            </div>
          )}

          {selectedPolicyEvents.length > 0 && (
            <div style={{ marginBottom: 12 }}>
              <div style={{ fontSize: 9, color: '#3B82F6', marginBottom: 6 }}>Policy Decisions</div>
              <div style={{ display: 'grid', gap: 8 }}>
                {selectedPolicyEvents.map(event => (
                  <div key={event.decision_id} style={{ border: '1px solid #0F1F35', borderRadius: 6, padding: 10, background: '#071525' }}>
                    <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8 }}>
                      <div style={{ fontSize: 10, color: '#E2E8F0', fontWeight: 600 }}>{event.policy_name}</div>
                      <div style={{ fontSize: 9, color: event.result === 'deny' ? '#EF4444' : event.result === 'warn' ? '#F59E0B' : '#10B981' }}>
                        {event.result}
                      </div>
                    </div>
                    <div style={{ fontSize: 10, color: '#94A3B8', marginTop: 4 }}>{event.reason || 'No reason recorded'}</div>
                    <div style={{ fontSize: 9, color: '#64748B', marginTop: 4 }}>
                      {event.scope || 'request'}
                      {event.redactions ? ` | redactions ${event.redactions}` : ''}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}

          {Object.entries(selected.attributes ?? {}).slice(0, 30).map(([key, value]) => (
            <div key={key} style={{ marginBottom: 6 }}>
              <div style={{ fontSize: 9, color: '#3B82F6' }}>{key}</div>
              <div style={{ fontSize: 10, color: '#64748B', wordBreak: 'break-all', paddingLeft: 8 }}>{value}</div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
