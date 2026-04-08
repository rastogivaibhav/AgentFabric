import { useState, useRef, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useLiveStream, Span, LiveEvent, PolicyLiveEventData, getSpanOutcomeStatus } from '../hooks/api'
import { Pause, Play, Trash2, Download } from 'lucide-react'
import SpanDetailPanel from '../components/trace/SpanDetailPanel'

const FRAMEWORK_COLORS: Record<string, string> = {
  crewai: '#FF6B35',
  langgraph: '#4ECDC4',
  google_adk: '#4285F4',
  openai_agents: '#10A37F',
  claude_agents: '#D97706',
  policy: 'var(--prove)',
  unknown: 'var(--text-tertiary)',
}

const EVENT_TYPE_COLOR: Record<string, string> = {
  span: 'var(--control)',
  run_start: 'var(--spend)',
  run_end: 'var(--ship)',
  error: 'var(--protect)',
  policy: 'var(--prove)',
}

function fmtDuration(ns: number) {
  if (ns < 1_000) return `${ns}ns`
  if (ns < 1_000_000) return `${(ns / 1_000).toFixed(1)}us`
  if (ns < 1_000_000_000) return `${(ns / 1_000_000).toFixed(1)}ms`
  return `${(ns / 1_000_000_000).toFixed(2)}s`
}

function fmtTime(ts: number) {
  return new Date(ts).toISOString().split('T')[1].replace('Z', '')
}

export default function LiveStream() {
  const nav = useNavigate()
  const { events, connected, pause, resume, clear } = useLiveStream(1000)
  const [paused, setPaused] = useState(false)
  const [filter, setFilter] = useState('')
  const [selectedEvent, setSelectedEvent] = useState<LiveEvent | null>(null)
  const [autoScroll, setAutoScroll] = useState(true)
  const tableRef = useRef<HTMLDivElement>(null)

  const togglePause = () => {
    if (paused) { resume(); setPaused(false) }
    else { pause(); setPaused(true) }
  }

  useEffect(() => {
    if (autoScroll && !paused && tableRef.current) tableRef.current.scrollTop = 0
  }, [events, autoScroll, paused])

  const filtered = filter
    ? events.filter(e => {
        const span = e.data as Span
        const policy = e.data as PolicyLiveEventData
        const name = e.type === 'policy' ? policy.policy_name : span.name
        const framework = e.type === 'policy' ? 'policy' : span.framework
        const traceID = e.type === 'policy' ? policy.trace_id : span.trace_id
        return (
          name?.toLowerCase().includes(filter.toLowerCase()) ||
          framework?.toLowerCase().includes(filter.toLowerCase()) ||
          traceID?.includes(filter) ||
          e.type.includes(filter.toLowerCase())
        )
      })
    : events

  const exportCSV = () => {
    const rows = [
      'timestamp,type,trace_id,span_id,name,framework,duration_ns,status,cost_usd',
      ...filtered.map(e => {
        const span = e.data as Span
        const policy = e.data as PolicyLiveEventData
        const traceID = e.type === 'policy' ? (policy.trace_id ?? '') : (span.trace_id ?? '')
        const spanID = e.type === 'policy' ? (policy.span_id ?? '') : (span.id ?? '')
        const name = e.type === 'policy' ? policy.policy_name : (span.name ?? '')
        const framework = e.type === 'policy' ? 'policy' : (span.framework ?? '')
        const duration = e.type === 'policy' ? 0 : (span.duration_ns ?? 0)
        const status = e.type === 'policy' ? policy.result : getSpanOutcomeStatus(span)
        const cost = e.type === 'policy' ? 0 : (span.cost_usd ?? 0)
        return `${e.ts},${e.type},${traceID},${spanID},${name},${framework},${duration},${status},${cost}`
      }),
    ].join('\n')
    const blob = new Blob([rows], { type: 'text/csv' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url; a.download = `govagn-stream-${Date.now()}.csv`; a.click()
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', padding: 24, gap: 16 }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
          <h1 style={{ fontSize: 20, fontWeight: 700, color: 'var(--text-primary)', margin: 0 }}>Live Stream</h1>
          <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
            <div style={{ width: 8, height: 8, borderRadius: '50%', background: connected ? 'var(--spend)' : 'var(--protect)', boxShadow: connected ? '0 0 8px var(--spend)' : 'none', animation: connected && !paused ? 'pulse 2s infinite' : 'none' }} />
            <span style={{ fontSize: 11, color: connected ? 'var(--spend)' : 'var(--protect)', letterSpacing: '0.1em' }}>
              {connected ? (paused ? 'PAUSED' : 'LIVE') : 'DISCONNECTED'}
            </span>
          </div>
          <span style={{ fontSize: 11, color: 'var(--text-tertiary)' }}>{filtered.length.toLocaleString()} events</span>
        </div>

        <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          <input
            value={filter}
            onChange={e => setFilter(e.target.value)}
            placeholder="Filter: framework, name, trace_id..."
            style={{ padding: '6px 12px', borderRadius: 6, fontSize: 11, background: 'var(--layer-2)', border: '1px solid var(--layer-border)', color: 'var(--text-secondary)', width: 280, outline: 'none' }}
          />
          <button onClick={togglePause} style={btnStyle(paused ? 'var(--spend)' : 'var(--prove)')}>
            {paused ? <><Play size={12} /> Resume</> : <><Pause size={12} /> Pause</>}
          </button>
          <button onClick={clear} style={btnStyle('var(--protect)')}><Trash2 size={12} /> Clear</button>
          <button onClick={exportCSV} style={btnStyle('var(--control)')}><Download size={12} /> Export</button>
          <button onClick={() => setAutoScroll(a => !a)} style={btnStyle(autoScroll ? 'var(--ship)' : 'var(--text-tertiary)')}>
            Auto-scroll {autoScroll ? 'ON' : 'OFF'}
          </button>
        </div>
      </div>

      <div style={{ display: 'flex', gap: 16, flex: 1, overflow: 'hidden' }}>
        <div ref={tableRef} style={{ flex: 1, overflow: 'auto', background: 'var(--layer-0)', border: '1px solid var(--layer-border)', borderRadius: 8 }}>
          <div style={{ display: 'grid', gridTemplateColumns: '90px 70px 140px 130px 1fr 100px 70px 90px', gap: 0, position: 'sticky', top: 0, background: 'var(--layer-1)', borderBottom: '1px solid var(--layer-border)', zIndex: 10 }}>
            {['TIME', 'TYPE', 'TRACE ID', 'SPAN ID', 'NAME', 'FRAMEWORK', 'DURATION', 'COST'].map(h => (
              <div key={h} style={{ padding: '8px 12px', fontSize: 9, color: 'var(--text-tertiary)', letterSpacing: '0.15em', fontWeight: 700 }}>{h}</div>
            ))}
          </div>

          {filtered.map((ev, i) => {
            const span = ev.data as Span
            const policy = ev.data as PolicyLiveEventData
            const isSelected = selectedEvent === ev
            const outcomeStatus = ev.type === 'policy' ? 'ok' : getSpanOutcomeStatus(span)
            const isError = outcomeStatus === 'error'
            const isBlocked = ev.type !== 'policy' && outcomeStatus === 'blocked'
            const isRedacted = ev.type !== 'policy' && (span.redaction_count ?? 0) > 0
            const traceID = ev.type === 'policy' ? (policy.trace_id ?? '') : (span.trace_id ?? '')
            const spanID = ev.type === 'policy' ? (policy.span_id ?? '') : (span.id ?? '')
            const rowName = ev.type === 'policy' ? policy.policy_name : (span.name ?? ev.type)
            const framework = ev.type === 'policy' ? 'policy' : (span.framework ?? '-')
            const duration = ev.type === 'policy' ? '-' : (span.duration_ns ? fmtDuration(span.duration_ns) : '-')
            const cost = ev.type === 'policy' ? '-' : (span.cost_usd ? `$${span.cost_usd.toFixed(6)}` : '-')
            const frameworkColor = ev.type === 'policy' ? FRAMEWORK_COLORS.policy : (FRAMEWORK_COLORS[span.framework ?? 'unknown'] ?? 'var(--text-tertiary)')

            return (
              <div
                key={i}
                onClick={() => setSelectedEvent(isSelected ? null : ev)}
                style={{
                  display: 'grid',
                  gridTemplateColumns: '90px 70px 140px 130px 1fr 100px 70px 90px',
                  cursor: 'pointer',
                  borderBottom: '1px solid var(--layer-border)',
                  background: isSelected ? 'rgba(10,132,255,0.08)' : isBlocked ? 'rgba(255,69,58,0.07)' : isRedacted ? 'rgba(255,159,10,0.06)' : isError ? 'rgba(255,69,58,0.04)' : i % 2 === 0 ? 'transparent' : 'rgba(255,255,255,0.02)',
                  transition: 'background 0.1s',
                }}
              >
                <div style={{ padding: '5px 12px', fontSize: 10, color: 'var(--text-tertiary)', fontFamily: 'monospace' }}>{fmtTime(ev.ts)}</div>
                <div style={{ padding: '5px 12px' }}>
                  <span style={{ fontSize: 9, padding: '1px 5px', borderRadius: 3, background: `${EVENT_TYPE_COLOR[ev.type] ?? 'var(--text-tertiary)'}25`, color: EVENT_TYPE_COLOR[ev.type] ?? 'var(--text-tertiary)' }}>
                    {ev.type}
                  </span>
                </div>
                <div style={{ padding: '5px 12px', fontSize: 10, color: 'var(--control)', fontFamily: 'monospace', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                  <button onClick={e => { e.stopPropagation(); if (traceID) nav(`/traces/${traceID}`) }}
                    style={{ background: 'transparent', border: 'none', color: 'var(--control)', cursor: traceID ? 'pointer' : 'default', fontFamily: 'monospace', padding: 0 }}>
                    {traceID.substring(0, 16)}
                  </button>
                </div>
                <div style={{ padding: '5px 12px', fontSize: 10, color: 'var(--text-secondary)', fontFamily: 'monospace', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{spanID.substring(0, 16)}</div>
                <div style={{ padding: '5px 12px', fontSize: 11, color: isError ? 'var(--protect)' : 'var(--text-primary)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                  {rowName}{isBlocked ? ' [blocked]' : isRedacted ? ' [redacted]' : ''}
                </div>
                <div style={{ padding: '5px 12px' }}><span style={{ fontSize: 9, color: frameworkColor }}>{framework}</span></div>
                <div style={{ padding: '5px 12px', fontSize: 10, color: 'var(--text-secondary)' }}>{duration}</div>
                <div style={{ padding: '5px 12px', fontSize: 10, color: 'var(--prove)' }}>{cost}</div>
              </div>
            )
          })}

          {filtered.length === 0 && (
            <div style={{ padding: 48, textAlign: 'center', color: 'var(--text-tertiary)', fontSize: 13 }}>
              {connected ? 'Waiting for agent activity...' : 'Connecting to stream...'}
            </div>
          )}
        </div>

        {selectedEvent && (
          <div style={{ width: 340, background: 'var(--layer-2)', border: '1px solid var(--layer-border)', borderRadius: 8, overflow: 'auto', flexShrink: 0 }}>
            {selectedEvent.type === 'policy' ? <PolicyDetail event={selectedEvent} /> : <SpanDetail event={selectedEvent} />}
          </div>
        )}
      </div>

      <style>{`@keyframes pulse { 0%,100%{opacity:1} 50%{opacity:0.4} }`}</style>
    </div>
  )
}

function SpanDetail({ event }: { event: LiveEvent }) {
  const span = event.data as Span
  return <SpanDetailPanel span={span} title="SPAN DETAIL" />
}

function PolicyDetail({ event }: { event: LiveEvent }) {
  const data = event.data as PolicyLiveEventData
  return (
    <div style={{ padding: 16 }}>
      <div style={{ fontSize: 11, color: 'var(--text-tertiary)', letterSpacing: '0.1em', marginBottom: 12 }}>POLICY EVENT</div>
      <div style={{ fontSize: 12, fontWeight: 600, color: 'var(--text-primary)', marginBottom: 16, wordBreak: 'break-all' }}>{data.policy_name}</div>

      {[
        ['Result', data.result],
        ['Reason', data.reason],
        ['Provider', data.provider],
        ['Model', data.model],
        ['Scope', data.scope],
        ['Decision ID', data.decision_id],
        ['Trace ID', data.trace_id],
        ['Span ID', data.span_id],
        ['Redactions', data.redactions],
      ].filter(([, v]) => v != null && v !== '').map(([k, v]) => (
        <div key={String(k)} style={{ display: 'flex', justifyContent: 'space-between', padding: '6px 0', borderBottom: '1px solid var(--layer-border)', fontSize: 11, gap: 8 }}>
          <span style={{ color: 'var(--text-tertiary)' }}>{k}</span>
          <span style={{ color: 'var(--text-secondary)', fontFamily: 'monospace', maxWidth: 180, overflow: 'hidden', textOverflow: 'ellipsis', textAlign: 'right' }}>{String(v)}</span>
        </div>
      ))}

      {(data.matched ?? []).length > 0 && (
        <div style={{ marginTop: 16 }}>
          <div style={{ fontSize: 10, color: 'var(--text-tertiary)', letterSpacing: '0.1em', marginBottom: 8 }}>MATCHED DETECTORS</div>
          {(data.matched ?? []).map(item => (
            <div key={item} style={{ fontSize: 10, color: 'var(--text-secondary)', padding: '4px 0' }}>{item}</div>
          ))}
        </div>
      )}
    </div>
  )
}

function btnStyle(color: string) {
  return {
    display: 'flex', alignItems: 'center', gap: 5,
    padding: '6px 12px', borderRadius: 6, fontSize: 11, cursor: 'pointer',
    background: `color-mix(in srgb, ${color} 12%, transparent)`,
    border: `1px solid color-mix(in srgb, ${color} 35%, transparent)`,
    color,
  } as React.CSSProperties
}
