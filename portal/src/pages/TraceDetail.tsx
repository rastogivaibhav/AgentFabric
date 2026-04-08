import { useMemo, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { Span, getSpanOutcomeStatus, outcomeStatusColor, useTrace, useTraceDecisions } from '../hooks/api'
import TopologyGraph from '../components/TopologyGraph'
import DecisionRecordPanel from '../components/trace/DecisionRecordPanel'
import TraceHeader from '../components/trace/TraceHeader'
import PolicyEventPanel from '../components/trace/PolicyEventPanel'
import SpanDetailPanel from '../components/trace/SpanDetailPanel'
import SpanTimeline from '../components/trace/SpanTimeline'

const FW_COLORS: Record<string, string> = {
  crewai: '#FF6B35', langgraph: '#4ECDC4', google_adk: 'var(--control)',
  openai_agents: 'var(--spend)', claude_agents: 'var(--prove)', unknown: 'var(--text-tertiary)',
}

function fmtDuration(ns: number) {
  if (ns < 1_000_000) return `${(ns / 1_000).toFixed(0)}us`
  if (ns < 1_000_000_000) return `${(ns / 1_000_000).toFixed(1)}ms`
  return `${(ns / 1_000_000_000).toFixed(2)}s`
}

function buildSpanTree(spans: Span[]) {
  const byId: Record<string, Span & { children?: Span[] }> = {}
  for (const span of spans) byId[span.id] = { ...span, children: [] }
  const roots: (Span & { children?: Span[] })[] = []
  for (const span of spans) {
    const parentId = span.parent_id ?? span.attributes?.parent_span_id
    if (parentId && byId[parentId]) { byId[parentId].children!.push(byId[span.id]) }
    else { roots.push(byId[span.id]) }
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

export default function TraceDetail() {
  const { traceId } = useParams<{ traceId: string }>()
  const { data: trace, isLoading } = useTrace(traceId!)
  const { data: decisionsData } = useTraceDecisions(traceId!)
  const [tab, setTab] = useState<'waterfall' | 'spans' | 'graph'>('waterfall')
  const [selected, setSelected] = useState<Span | null>(null)

  if (isLoading) return <div style={{ padding: 32, color: 'var(--text-tertiary)' }}>Loading trace...</div>
  if (!trace) return <div style={{ padding: 32, color: 'var(--protect)' }}>Trace not found</div>

  const spans = trace.spans ?? []
  const policyEvents = trace.policy_events ?? []
  const decisionRecords = decisionsData?.items ?? []
  const tree = buildSpanTree(spans)
  const flatSpans = flattenTree(tree)
  const timeline = trace.timeline ?? {
    trace_id: trace.id, start_time: trace.start_time, duration_ns: trace.duration_ns,
    items: flatSpans.map(({ span, depth }) => ({
      span_id: span.id, parent_span_id: span.parent_id, name: span.name, step_type: span.step_type,
      provider: span.provider, model: span.model, app_name: span.app_name, environment: span.environment,
      status: getSpanOutcomeStatus(span), failure_summary: span.failure_summary, blocked: span.blocked,
      blocked_reason: span.blocked_reason, redaction_count: span.redaction_count, depth, lineage: span.lineage,
      start_offset_ns: span.start_time_ns - (spans[0]?.start_time_ns ?? 0),
      end_offset_ns: span.start_time_ns - (spans[0]?.start_time_ns ?? 0) + span.duration_ns,
      duration_ns: span.duration_ns,
      total_tokens: (span.input_tokens ?? 0) + (span.output_tokens ?? 0) + (span.cache_read_tokens ?? 0) + (span.cache_write_tokens ?? 0) + (span.reasoning_tokens ?? 0),
      cost_usd: span.cost_usd, policy_event_count: policyEvents.filter(event => event.span_id === span.id).length,
    })),
    highlights: trace.insights?.workflow_summary ?? [],
  }

  // eslint-disable-next-line react-hooks/rules-of-hooks
  const selectedPolicyEvents = useMemo(() => (selected ? policyEvents.filter(event => event.span_id === selected.id) : []), [policyEvents, selected])

  return (
    <div style={{ padding: 24, display: 'flex', gap: 16, height: '100%', overflow: 'hidden' }}>
      <div style={{ flex: 1, overflow: 'auto' }}>
        <TraceHeader trace={trace} policyEvents={policyEvents} frameworkColor={FW_COLORS[trace.framework] ?? 'var(--text-tertiary)'} />

        <div style={{ marginBottom: 12 }}>
          <DecisionRecordPanel title="DECISION EVIDENCE" records={decisionRecords.slice(0, 6)} emptyLabel="No decision evidence recorded for this trace." />
        </div>
        <div style={{ marginBottom: 12 }}>
          <PolicyEventPanel title="POLICY DECISIONS" events={policyEvents.slice(0, 6)} emptyLabel="No policy evidence recorded for this trace." />
        </div>

        <div style={{ display: 'flex', justifyContent: 'flex-end', marginBottom: 12 }}>
          <Link to="/traces" style={{ color: 'var(--control)', fontSize: 11, textDecoration: 'none' }}>← Compare from traces page</Link>
        </div>

        <div style={{ display: 'flex', gap: 4, marginBottom: 16 }}>
          {(['waterfall', 'spans', 'graph'] as const).map(currentTab => (
            <button key={currentTab} id={`trace-tab-${currentTab}`} onClick={() => setTab(currentTab)}
              style={{ padding: '6px 14px', borderRadius: 6, fontSize: 11, cursor: 'pointer',
                background: tab === currentTab ? 'rgba(10,132,255,0.12)' : 'transparent',
                border: `1px solid ${tab === currentTab ? 'rgba(10,132,255,0.4)' : 'var(--layer-border)'}`,
                color: tab === currentTab ? 'var(--control)' : 'var(--text-tertiary)' }}>
              {currentTab.charAt(0).toUpperCase() + currentTab.slice(1)}
            </button>
          ))}
        </div>

        {tab === 'waterfall' && (
          <SpanTimeline timeline={timeline} selectedSpanId={selected?.id}
            onSelectSpan={spanId => { const span = spans.find(item => item.id === spanId) ?? null; setSelected(prev => (prev?.id === spanId ? null : span)) }} />
        )}

        {tab === 'spans' && (
          <div style={{ background: 'var(--layer-2)', border: '1px solid var(--layer-border)', borderRadius: 8, overflow: 'hidden' }}>
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 11 }}>
              <thead>
                <tr style={{ background: 'var(--layer-1)', borderBottom: '1px solid var(--layer-border)' }}>
                  {['Span ID', 'Name', 'Step', 'Framework', 'Duration', 'Status', 'Tokens', 'Cost'].map(header => (
                    <th key={header} style={{ padding: '9px 12px', textAlign: 'left', color: 'var(--text-tertiary)', fontSize: 9, letterSpacing: '0.08em', fontWeight: 700 }}>{header}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {spans.map((span, index) => {
                  const outcomeStatus = getSpanOutcomeStatus(span)
                  const outcomeColor = outcomeStatusColor(outcomeStatus)
                  return (
                    <tr key={span.id} onClick={() => setSelected(selected?.id === span.id ? null : span)}
                      style={{ borderBottom: '1px solid var(--layer-border)', cursor: 'pointer', background: index % 2 === 0 ? 'transparent' : 'rgba(255,255,255,0.02)' }}>
                      <td style={{ padding: '7px 12px', color: 'var(--control)', fontFamily: 'monospace' }}>{span.id.substring(0, 12)}…</td>
                      <td style={{ padding: '7px 12px', color: 'var(--text-primary)' }}>{span.name}</td>
                      <td style={{ padding: '7px 12px', color: 'var(--control)' }}>{span.step_type ?? '-'}</td>
                      <td style={{ padding: '7px 12px' }}><span style={{ fontSize: 9, color: FW_COLORS[span.framework ?? 'unknown'] ?? 'var(--text-tertiary)' }}>{span.framework}</span></td>
                      <td style={{ padding: '7px 12px', color: 'var(--text-secondary)' }}>{fmtDuration(span.duration_ns)}</td>
                      <td style={{ padding: '7px 12px' }}>
                        <span style={{ fontSize: 9, padding: '1px 5px', borderRadius: 3, background: `${outcomeColor}20`, color: outcomeColor }}>{outcomeStatus.toUpperCase()}</span>
                      </td>
                      <td style={{ padding: '7px 12px', color: 'var(--text-secondary)' }}>{(span.input_tokens || 0) + (span.output_tokens || 0)}</td>
                      <td style={{ padding: '7px 12px', color: 'var(--prove)' }}>{span.cost_usd ? `$${span.cost_usd.toFixed(6)}` : '-'}</td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}

        {tab === 'graph' && (
          <TopologyGraph spans={spans} selectedSpanId={selected?.id}
            onSelectSpan={spanId => { const span = spans.find(item => item.id === spanId) ?? null; setSelected(prev => (prev?.id === spanId ? null : span)) }} />
        )}
      </div>

      {selected && (
        <div style={{ width: 320, background: 'var(--layer-2)', border: '1px solid var(--layer-border)', borderRadius: 8, overflow: 'auto', flexShrink: 0 }}>
          <SpanDetailPanel span={selected} policyEvents={selectedPolicyEvents} />
        </div>
      )}
    </div>
  )
}
