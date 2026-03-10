import { useState, useRef, useEffect } from 'react'
import { useLiveStream, Span, LiveEvent } from '../hooks/api'
import { Pause, Play, Trash2, Download, Filter } from 'lucide-react'

const FRAMEWORK_COLORS: Record<string, string> = {
  crewai: '#FF6B35',
  langgraph: '#4ECDC4',
  google_adk: '#4285F4',
  openai_agents: '#10A37F',
  claude_agents: '#D97706',
  unknown: '#475569',
}

const EVENT_TYPE_COLOR: Record<string, string> = {
  span: '#3B82F6',
  run_start: '#10B981',
  run_end: '#6366F1',
  error: '#EF4444',
  policy: '#F59E0B',
}

function fmtDuration(ns: number) {
  if (ns < 1_000) return `${ns}ns`
  if (ns < 1_000_000) return `${(ns / 1_000).toFixed(1)}µs`
  if (ns < 1_000_000_000) return `${(ns / 1_000_000).toFixed(1)}ms`
  return `${(ns / 1_000_000_000).toFixed(2)}s`
}

function fmtTime(ts: number) {
  return new Date(ts).toISOString().split('T')[1].replace('Z', '')
}

export default function LiveStream() {
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
    if (autoScroll && !paused && tableRef.current) {
      tableRef.current.scrollTop = 0
    }
  }, [events, autoScroll, paused])

  const filtered = filter
    ? events.filter(e => {
        const s = e.data as Span
        return (
          s.name?.toLowerCase().includes(filter.toLowerCase()) ||
          s.framework?.includes(filter.toLowerCase()) ||
          s.trace_id?.includes(filter) ||
          e.type.includes(filter.toLowerCase())
        )
      })
    : events

  const exportCSV = () => {
    const rows = [
      'timestamp,type,trace_id,span_id,name,framework,duration_ns,status,cost_usd',
      ...filtered.map(e => {
        const s = e.data as Span
        return `${e.ts},${e.type},${s.trace_id ?? ''},${s.id ?? ''},${s.name ?? ''},${s.framework ?? ''},${s.duration_ns ?? 0},${s.status_code ?? 0},${s.cost_usd ?? 0}`
      })
    ].join('\n')
    const blob = new Blob([rows], { type: 'text/csv' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url; a.download = `agentfabric-stream-${Date.now()}.csv`
    a.click()
  }

  return (
    <div style={{ display:'flex', flexDirection:'column', height:'100%', padding:24, gap:16 }}>
      {/* Header */}
      <div style={{ display:'flex', alignItems:'center', justifyContent:'space-between' }}>
        <div style={{ display:'flex', alignItems:'center', gap:12 }}>
          <h1 style={{ fontSize:20, fontWeight:700, color:'#F0F9FF', margin:0 }}>Live Stream</h1>
          <div style={{ display:'flex', alignItems:'center', gap:6 }}>
            <div style={{ width:8, height:8, borderRadius:'50%', background: connected ? '#10B981' : '#EF4444', boxShadow: connected ? '0 0 8px #10B981' : 'none', animation: connected && !paused ? 'pulse 2s infinite' : 'none' }} />
            <span style={{ fontSize:11, color: connected ? '#10B981' : '#EF4444', letterSpacing:'0.1em' }}>
              {connected ? (paused ? 'PAUSED' : 'LIVE') : 'DISCONNECTED'}
            </span>
          </div>
          <span style={{ fontSize:11, color:'#334155' }}>{filtered.length.toLocaleString()} events</span>
        </div>

        <div style={{ display:'flex', gap:8, alignItems:'center' }}>
          {/* Filter */}
          <input
            value={filter}
            onChange={e => setFilter(e.target.value)}
            placeholder="Filter: framework, name, trace_id..."
            style={{
              padding:'6px 12px', borderRadius:6, fontSize:11,
              background:'#0D1B2A', border:'1px solid #1E3A5F',
              color:'#CBD5E1', width:280, outline:'none',
              fontFamily:"'JetBrains Mono',monospace",
            }}
          />
          <button onClick={togglePause} style={btnStyle(paused ? '#10B981' : '#F59E0B')}>
            {paused ? <><Play size={12} /> Resume</> : <><Pause size={12} /> Pause</>}
          </button>
          <button onClick={clear} style={btnStyle('#EF4444')}>
            <Trash2 size={12} /> Clear
          </button>
          <button onClick={exportCSV} style={btnStyle('#3B82F6')}>
            <Download size={12} /> Export
          </button>
          <button
            onClick={() => setAutoScroll(a => !a)}
            style={btnStyle(autoScroll ? '#8B5CF6' : '#475569')}
          >
            Auto-scroll {autoScroll ? 'ON' : 'OFF'}
          </button>
        </div>
      </div>

      <div style={{ display:'flex', gap:16, flex:1, overflow:'hidden' }}>
        {/* Stream table */}
        <div ref={tableRef} style={{
          flex:1, overflow:'auto',
          background:'#060A14', border:'1px solid #0F1F35', borderRadius:8,
        }}>
          {/* Column headers — Wireshark style */}
          <div style={{
            display:'grid',
            gridTemplateColumns:'90px 70px 140px 130px 1fr 100px 70px 90px',
            gap:0, position:'sticky', top:0, background:'#080C18',
            borderBottom:'1px solid #0F1F35', zIndex:10,
          }}>
            {['TIME', 'TYPE', 'TRACE ID', 'SPAN ID', 'NAME', 'FRAMEWORK', 'DURATION', 'COST'].map(h => (
              <div key={h} style={{ padding:'8px 12px', fontSize:9, color:'#334155', letterSpacing:'0.15em', fontWeight:700 }}>{h}</div>
            ))}
          </div>

          {filtered.map((ev, i) => {
            const span = ev.data as Span
            const isSelected = selectedEvent === ev
            const isError = span.status_code === 2
            return (
              <div
                key={i}
                onClick={() => setSelectedEvent(isSelected ? null : ev)}
                style={{
                  display:'grid',
                  gridTemplateColumns:'90px 70px 140px 130px 1fr 100px 70px 90px',
                  cursor:'pointer', borderBottom:'1px solid #0A1020',
                  background: isSelected ? '#1E3A5F30' : isError ? '#EF444408' : i % 2 === 0 ? 'transparent' : '#060A1480',
                  transition:'background 0.1s',
                }}
              >
                <div style={{ padding:'5px 12px', fontSize:10, color:'#334155', fontFamily:'monospace' }}>
                  {fmtTime(ev.ts)}
                </div>
                <div style={{ padding:'5px 12px' }}>
                  <span style={{ fontSize:9, padding:'1px 5px', borderRadius:3, background: (EVENT_TYPE_COLOR[ev.type] ?? '#475569') + '20', color: EVENT_TYPE_COLOR[ev.type] ?? '#475569' }}>
                    {ev.type}
                  </span>
                </div>
                <div style={{ padding:'5px 12px', fontSize:10, color:'#3B82F6', fontFamily:'monospace', overflow:'hidden', textOverflow:'ellipsis', whiteSpace:'nowrap' }}>
                  {(span.trace_id ?? '').substring(0, 16)}
                </div>
                <div style={{ padding:'5px 12px', fontSize:10, color:'#475569', fontFamily:'monospace', overflow:'hidden', textOverflow:'ellipsis', whiteSpace:'nowrap' }}>
                  {(span.id ?? '').substring(0, 16)}
                </div>
                <div style={{ padding:'5px 12px', fontSize:11, color: isError ? '#EF4444' : '#CBD5E1', overflow:'hidden', textOverflow:'ellipsis', whiteSpace:'nowrap' }}>
                  {span.name ?? ev.type}
                </div>
                <div style={{ padding:'5px 12px' }}>
                  <span style={{ fontSize:9, color: FRAMEWORK_COLORS[span.framework ?? 'unknown'] ?? '#475569' }}>
                    {span.framework ?? '—'}
                  </span>
                </div>
                <div style={{ padding:'5px 12px', fontSize:10, color:'#64748B' }}>
                  {span.duration_ns ? fmtDuration(span.duration_ns) : '—'}
                </div>
                <div style={{ padding:'5px 12px', fontSize:10, color:'#F59E0B' }}>
                  {span.cost_usd ? `$${span.cost_usd.toFixed(6)}` : '—'}
                </div>
              </div>
            )
          })}

          {filtered.length === 0 && (
            <div style={{ padding:48, textAlign:'center', color:'#334155', fontSize:13 }}>
              {connected ? 'Waiting for agent activity…' : 'Connecting to stream…'}
            </div>
          )}
        </div>

        {/* Detail panel */}
        {selectedEvent && (
          <div style={{
            width:340, background:'#0D1B2A', border:'1px solid #0F1F35',
            borderRadius:8, overflow:'auto', flexShrink:0,
          }}>
            <SpanDetail event={selectedEvent} />
          </div>
        )}
      </div>

      <style>{`@keyframes pulse { 0%,100%{opacity:1} 50%{opacity:0.4} }`}</style>
    </div>
  )
}

function SpanDetail({ event }: { event: LiveEvent }) {
  const span = event.data as Span
  return (
    <div style={{ padding:16 }}>
      <div style={{ fontSize:11, color:'#475569', letterSpacing:'0.1em', marginBottom:12 }}>SPAN DETAIL</div>
      <div style={{ fontSize:12, fontWeight:600, color:'#F0F9FF', marginBottom:16, wordBreak:'break-all' }}>{span.name}</div>

      {[
        ['Trace ID', span.trace_id],
        ['Span ID', span.id],
        ['Parent ID', span.parent_id],
        ['Framework', span.framework],
        ['Status', span.status_code === 0 ? 'OK' : span.status_code === 1 ? 'UNSET' : 'ERROR'],
        ['Duration', span.duration_ns ? fmtDuration(span.duration_ns) : '—'],
        ['Input Tokens', span.input_tokens],
        ['Output Tokens', span.output_tokens],
        ['Cost USD', span.cost_usd ? `$${span.cost_usd.toFixed(8)}` : '—'],
      ].filter(([, v]) => v != null && v !== '').map(([k, v]) => (
        <div key={String(k)} style={{ display:'flex', justifyContent:'space-between', padding:'6px 0', borderBottom:'1px solid #060A14', fontSize:11 }}>
          <span style={{ color:'#475569' }}>{k}</span>
          <span style={{ color:'#CBD5E1', fontFamily:'monospace', maxWidth:180, overflow:'hidden', textOverflow:'ellipsis', textAlign:'right' }}>{String(v)}</span>
        </div>
      ))}

      {Object.keys(span.attributes ?? {}).length > 0 && (
        <div style={{ marginTop:16 }}>
          <div style={{ fontSize:10, color:'#334155', letterSpacing:'0.1em', marginBottom:8 }}>ATTRIBUTES</div>
          {Object.entries(span.attributes ?? {}).map(([k, v]) => (
            <div key={k} style={{ marginBottom:4 }}>
              <div style={{ fontSize:10, color:'#3B82F6' }}>{k}</div>
              <div style={{ fontSize:10, color:'#64748B', wordBreak:'break-all', paddingLeft:8 }}>{v}</div>
            </div>
          ))}
        </div>
      )}

      {(span.events ?? []).length > 0 && (
        <div style={{ marginTop:16 }}>
          <div style={{ fontSize:10, color:'#334155', letterSpacing:'0.1em', marginBottom:8 }}>EVENTS ({span.events!.length})</div>
          {span.events!.map((ev, i) => (
            <div key={i} style={{ padding:'6px 8px', background:'#060A14', borderRadius:4, marginBottom:4 }}>
              <div style={{ fontSize:10, color:'#94A3B8', fontWeight:600 }}>{ev.name}</div>
              <div style={{ fontSize:9, color:'#334155' }}>{new Date(ev.time_ns / 1_000_000).toISOString()}</div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

function btnStyle(color: string) {
  return {
    display:'flex', alignItems:'center', gap:5,
    padding:'6px 12px', borderRadius:6, fontSize:11, cursor:'pointer',
    background:`${color}15`, border:`1px solid ${color}40`, color,
    fontFamily:"'JetBrains Mono',monospace",
  } as React.CSSProperties
}
