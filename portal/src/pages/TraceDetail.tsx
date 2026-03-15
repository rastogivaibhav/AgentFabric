import { useParams } from 'react-router-dom'
import { useTrace, Span } from '../hooks/api'
import { useState } from 'react'
import TopologyGraph from '../components/TopologyGraph'

const FW_COLORS: Record<string, string> = {
  crewai:'#FF6B35', langgraph:'#4ECDC4', google_adk:'#4285F4',
  openai_agents:'#10A37F', claude_agents:'#D97706', unknown:'#475569'
}

function fmtDuration(ns: number) {
  if (ns < 1_000_000) return `${(ns/1_000).toFixed(0)}µs`
  if (ns < 1_000_000_000) return `${(ns/1_000_000).toFixed(1)}ms`
  return `${(ns/1_000_000_000).toFixed(2)}s`
}

// Waterfall chart
function WaterfallRow({ span, minTime, totalDuration, depth }: {
  span: Span; minTime: number; totalDuration: number; depth: number
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
      style={{ display:'flex', alignItems:'center', height:28, borderBottom:'1px solid #0A1020', position:'relative', background: hovered ? '#1E3A5F15' : 'transparent', cursor:'pointer' }}
    >
      {/* Name column */}
      <div style={{ width:280, flexShrink:0, padding:'0 12px', paddingLeft: 12 + depth * 16, overflow:'hidden', textOverflow:'ellipsis', whiteSpace:'nowrap', fontSize:11, color: isError ? '#EF4444' : '#CBD5E1', display:'flex', alignItems:'center', gap:6 }}>
        {depth > 0 && <span style={{ color:'#1E3A5F' }}>{'─'.repeat(1)}</span>}
        <span style={{ fontSize:9, color, marginRight:2 }}>●</span>
        {span.name}
      </div>
      {/* Duration label */}
      <div style={{ width:70, flexShrink:0, fontSize:10, color:'#475569', textAlign:'right', paddingRight:12 }}>
        {fmtDuration(span.duration_ns)}
      </div>
      {/* Bar */}
      <div style={{ flex:1, position:'relative', height:16 }}>
        <div style={{
          position:'absolute', top:2, height:12, borderRadius:2,
          left:`${startPct}%`, width:`${widthPct}%`,
          background: isError ? '#EF4444' : color,
          opacity: 0.8,
          minWidth:2,
        }} />
      </div>
      {/* Cost */}
      <div style={{ width:80, flexShrink:0, fontSize:10, color:'#F59E0B', textAlign:'right', paddingRight:12 }}>
        {span.cost_usd ? `$${span.cost_usd.toFixed(6)}` : ''}
      </div>
    </div>
  )
}

function buildSpanTree(spans: Span[]) {
  const byId: Record<string, Span & { children?: Span[] }> = {}
  for (const s of spans) byId[s.id] = { ...s, children: [] }
  const roots: (Span & { children?: Span[] })[] = []
  for (const s of spans) {
    if (s.parent_id && byId[s.parent_id]) {
      byId[s.parent_id].children!.push(byId[s.id])
    } else {
      roots.push(byId[s.id])
    }
  }
  return roots
}

function flattenTree(nodes: (Span & { children?: Span[] })[], depth = 0): { span: Span, depth: number }[] {
  const result: { span: Span, depth: number }[] = []
  for (const n of nodes) {
    result.push({ span: n, depth })
    if (n.children?.length) result.push(...flattenTree(n.children, depth + 1))
  }
  return result
}

export default function TraceDetail() {
  const { traceId } = useParams<{ traceId: string }>()
  const { data: trace, isLoading } = useTrace(traceId!)
  const [tab, setTab] = useState<'waterfall' | 'spans' | 'graph'>('waterfall')
  const [selected, setSelected] = useState<Span | null>(null)

  if (isLoading) return <div style={{ padding:32, color:'#475569' }}>Loading trace…</div>
  if (!trace) return <div style={{ padding:32, color:'#EF4444' }}>Trace not found</div>

  const spans = trace.spans ?? []
  const minTime = Math.min(...spans.map(s => s.start_time_ns))
  const maxTime = Math.max(...spans.map(s => s.start_time_ns + s.duration_ns))
  const totalDuration = maxTime - minTime

  const tree = buildSpanTree(spans)
  const flatSpans = flattenTree(tree)

  return (
    <div style={{ padding:24, display:'flex', gap:16, height:'100%', overflow:'hidden' }}>
      <div style={{ flex:1, overflow:'auto' }}>
        {/* Header */}
        <div style={{ marginBottom:20 }}>
          <div style={{ fontSize:11, color:'#475569', marginBottom:4 }}>TRACE</div>
          <div style={{ fontSize:13, color:'#3B82F6', fontFamily:'monospace', marginBottom:12, wordBreak:'break-all' }}>{trace.id}</div>
          <div style={{ display:'flex', gap:12, flexWrap:'wrap' }}>
            {[
              ['Framework', trace.framework, FW_COLORS[trace.framework] ?? '#475569'],
              ['Duration', fmtDuration(trace.duration_ns), '#94A3B8'],
              ['Spans', String(trace.span_count), '#94A3B8'],
              ['Errors', String(trace.error_count), trace.error_count > 0 ? '#EF4444' : '#10B981'],
              ['Tokens', trace.total_tokens.toLocaleString(), '#94A3B8'],
              ['Cost', `$${trace.total_cost_usd.toFixed(6)}`, '#F59E0B'],
            ].map(([k, v, c]) => (
              <div key={k} style={{ background:'#0D1B2A', border:'1px solid #0F1F35', borderRadius:6, padding:'8px 14px' }}>
                <div style={{ fontSize:9, color:'#334155', letterSpacing:'0.1em' }}>{k}</div>
                <div style={{ fontSize:13, color:c, fontWeight:600 }}>{v}</div>
              </div>
            ))}
          </div>
        </div>

        {/* Tabs */}
        <div style={{ display:'flex', gap:4, marginBottom:16 }}>
          {(['waterfall','spans','graph'] as const).map(t => (
            <button key={t} onClick={() => setTab(t)} style={{
              padding:'6px 14px', borderRadius:5, fontSize:11, cursor:'pointer',
              background: tab === t ? '#1E3A5F' : 'transparent',
              border: `1px solid ${tab === t ? '#3B82F6' : '#1E3A5F'}`,
              color: tab === t ? '#60A5FA' : '#475569',
            }}>{t.charAt(0).toUpperCase() + t.slice(1)}</button>
          ))}
        </div>

        {/* Waterfall */}
        {tab === 'waterfall' && (
          <div style={{ background:'#0D1B2A', border:'1px solid #0F1F35', borderRadius:8, overflow:'hidden' }}>
            <div style={{ display:'flex', alignItems:'center', height:32, background:'#080C18', borderBottom:'1px solid #0F1F35', fontSize:9, color:'#334155', letterSpacing:'0.1em' }}>
              <div style={{ width:280, paddingLeft:12 }}>SPAN NAME</div>
              <div style={{ width:70, textAlign:'right', paddingRight:12 }}>DURATION</div>
              <div style={{ flex:1, paddingLeft:8 }}>TIMELINE</div>
              <div style={{ width:80, textAlign:'right', paddingRight:12 }}>COST</div>
            </div>
            {flatSpans.map(({ span, depth }) => (
              <div key={span.id} onClick={() => setSelected(selected?.id === span.id ? null : span)}>
                <WaterfallRow span={span} minTime={minTime} totalDuration={totalDuration} depth={depth} />
              </div>
            ))}
          </div>
        )}

        {/* Spans table */}
        {tab === 'spans' && (
          <div style={{ background:'#0D1B2A', border:'1px solid #0F1F35', borderRadius:8, overflow:'hidden' }}>
            <table style={{ width:'100%', borderCollapse:'collapse', fontSize:11 }}>
              <thead>
                <tr style={{ borderBottom:'1px solid #0F1F35' }}>
                  {['Span ID','Name','Framework','Duration','Status','Tokens','Cost'].map(h => (
                    <th key={h} style={{ padding:'9px 12px', textAlign:'left', color:'#334155', fontSize:10, letterSpacing:'0.08em', fontWeight:700 }}>{h}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {spans.map((sp, i) => (
                  <tr key={sp.id} onClick={() => setSelected(selected?.id === sp.id ? null : sp)} style={{ borderBottom:'1px solid #0A1020', cursor:'pointer', background: i%2===0?'transparent':'#060A1430' }}>
                    <td style={{ padding:'7px 12px', color:'#3B82F6', fontFamily:'monospace' }}>{sp.id.substring(0,12)}…</td>
                    <td style={{ padding:'7px 12px', color:'#CBD5E1' }}>{sp.name}</td>
                    <td style={{ padding:'7px 12px' }}>
                      <span style={{ fontSize:9, color: FW_COLORS[sp.framework]??'#475569' }}>{sp.framework}</span>
                    </td>
                    <td style={{ padding:'7px 12px', color:'#64748B' }}>{fmtDuration(sp.duration_ns)}</td>
                    <td style={{ padding:'7px 12px' }}>
                      <span style={{ fontSize:9, padding:'1px 5px', borderRadius:3, background: sp.status_code===2?'#EF444420':'#10B98120', color: sp.status_code===2?'#EF4444':'#10B981' }}>
                        {sp.status_code===2?'ERROR':'OK'}
                      </span>
                    </td>
                    <td style={{ padding:'7px 12px', color:'#64748B' }}>{(sp.input_tokens||0)+(sp.output_tokens||0)}</td>
                    <td style={{ padding:'7px 12px', color:'#F59E0B' }}>{sp.cost_usd?`$${sp.cost_usd.toFixed(6)}`:'—'}</td>
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
            onSelectSpan={(spanId) => {
              const span = spans.find(s => s.id === spanId) ?? null
              setSelected(prev => prev?.id === spanId ? null : span)
            }}
          />
        )}
      </div>

      {/* Detail panel */}
      {selected && (
        <div style={{ width:300, background:'#0D1B2A', border:'1px solid #0F1F35', borderRadius:8, overflow:'auto', padding:16, flexShrink:0 }}>
          <div style={{ fontSize:10, color:'#334155', letterSpacing:'0.1em', marginBottom:12 }}>SPAN DETAIL</div>
          <div style={{ fontSize:12, fontWeight:600, color:'#F0F9FF', marginBottom:12 }}>{selected.name}</div>
          {Object.entries(selected.attributes ?? {}).slice(0, 30).map(([k,v]) => (
            <div key={k} style={{ marginBottom:6 }}>
              <div style={{ fontSize:9, color:'#3B82F6' }}>{k}</div>
              <div style={{ fontSize:10, color:'#64748B', wordBreak:'break-all', paddingLeft:8 }}>{v}</div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
