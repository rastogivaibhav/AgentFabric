import { useState, useMemo } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTraces } from '../hooks/api'

const FRAMEWORKS = ['', 'crewai', 'langgraph', 'google_adk', 'openai_agents', 'claude_agents']
const FW_COLORS: Record<string, string> = {
  crewai: '#FF6B35', langgraph: '#4ECDC4', google_adk: '#4285F4',
  openai_agents: '#10A37F', claude_agents: '#D97706', unknown: '#475569'
}
const PAGE_SIZE = 100

export default function TracesPage() {
  const nav = useNavigate()
  const [fw, setFw] = useState('')
  const [status, setStatus] = useState('')
  const [pageIndex, setPageIndex] = useState(0)
  const [search, setSearch] = useState('')

  const { data, isLoading, refetch } = useTraces({
    framework: fw,
    status,
    limit: String(PAGE_SIZE),
    offset: String(pageIndex * PAGE_SIZE),
  })

  const allItems = data?.items ?? []
  const total = data?.total ?? 0

  // Client-side text search on trace_id prefix and root_span_name
  const filtered = useMemo(() => {
    if (!search.trim()) return allItems
    const q = search.trim().toLowerCase()
    return allItems.filter(t =>
      t.id.toLowerCase().startsWith(q) ||
      t.root_span_name.toLowerCase().includes(q)
    )
  }, [allItems, search])

  const rangeStart = pageIndex * PAGE_SIZE + 1
  const rangeEnd = pageIndex * PAGE_SIZE + allItems.length
  const hasMore = data?.has_more ?? false
  const hasPrev = pageIndex > 0
  const hasNext = hasMore

  // Reset page when filters change
  function handleFwChange(v: string) { setFw(v); setPageIndex(0) }
  function handleStatusChange(v: string) { setStatus(v); setPageIndex(0) }
  function handleSearchChange(v: string) { setSearch(v) }

  return (
    <div style={{ padding: 32 }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 24 }}>
        <h1 style={{ fontSize: 20, fontWeight: 700, color: '#F0F9FF', margin: 0 }}>Traces</h1>
        <button
          onClick={() => refetch()}
          style={{ padding: '6px 14px', background: '#1E3A5F', border: '1px solid #3B82F6', borderRadius: 6, color: '#60A5FA', fontSize: 11, cursor: 'pointer' }}
        >
          Refresh
        </button>
      </div>

      {/* Filters */}
      <div style={{ display: 'flex', gap: 12, marginBottom: 20, flexWrap: 'wrap' }}>
        <select value={fw} onChange={e => handleFwChange(e.target.value)} style={selectStyle}>
          <option value="">All Frameworks</option>
          {FRAMEWORKS.slice(1).map(f => <option key={f} value={f}>{f}</option>)}
        </select>
        <select value={status} onChange={e => handleStatusChange(e.target.value)} style={selectStyle}>
          <option value="">All Status</option>
          <option value="ok">OK</option>
          <option value="error">Error</option>
        </select>
        <input
          type="text"
          value={search}
          onChange={e => handleSearchChange(e.target.value)}
          placeholder="Search trace ID or span name…"
          style={{
            ...selectStyle,
            flex: 1,
            minWidth: 220,
            background: '#0D1B2A',
            border: '1px solid #1E3A5F',
            color: '#CBD5E1',
          }}
        />
      </div>

      {/* Table */}
      <div style={{ background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 10, overflow: 'hidden' }}>
        <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
          <thead>
            <tr style={{ borderBottom: '1px solid #0F1F35' }}>
              {['Trace ID', 'Framework', 'Root Span', 'Started', 'Duration', 'Spans', 'Tokens', 'Cost', 'Status'].map(h => (
                <th key={h} style={{ padding: '10px 14px', textAlign: 'left', fontSize: 10, color: '#334155', letterSpacing: '0.1em', fontWeight: 700 }}>{h}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {isLoading && (
              <tr><td colSpan={9} style={{ padding: 32, textAlign: 'center', color: '#475569' }}>Loading…</td></tr>
            )}
            {!isLoading && filtered.length === 0 && (
              <tr><td colSpan={9} style={{ padding: 32, textAlign: 'center', color: '#334155', fontSize: 12 }}>
                {search ? 'No traces match your search.' : 'No traces found.'}
              </td></tr>
            )}
            {filtered.map((t, i) => (
              <tr
                key={t.id}
                onClick={() => nav(`/traces/${t.id}`)}
                style={{
                  borderBottom: '1px solid #0A1020', cursor: 'pointer',
                  background: i % 2 === 0 ? 'transparent' : '#060A1430',
                  transition: 'background 0.1s'
                }}
                onMouseEnter={e => (e.currentTarget.style.background = '#1E3A5F20')}
                onMouseLeave={e => (e.currentTarget.style.background = i % 2 === 0 ? 'transparent' : '#060A1430')}
              >
                <td style={{ padding: '9px 14px', color: '#3B82F6', fontFamily: 'monospace', fontSize: 11 }}>{t.id.substring(0, 16)}…</td>
                <td style={{ padding: '9px 14px' }}>
                  <span style={{ padding: '2px 7px', borderRadius: 3, fontSize: 10, background: (FW_COLORS[t.framework] ?? '#475569') + '20', color: FW_COLORS[t.framework] ?? '#475569' }}>
                    {t.framework}
                  </span>
                </td>
                <td style={{ padding: '9px 14px', color: '#94A3B8', maxWidth: 200, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{t.root_span_name}</td>
                <td style={{ padding: '9px 14px', color: '#64748B', fontSize: 11 }}>{new Date(t.start_time).toLocaleString()}</td>
                <td style={{ padding: '9px 14px', color: '#94A3B8' }}>{(t.duration_ns / 1_000_000).toFixed(0)}ms</td>
                <td style={{ padding: '9px 14px', color: '#94A3B8' }}>{t.span_count}</td>
                <td style={{ padding: '9px 14px', color: '#94A3B8' }}>{t.total_tokens.toLocaleString()}</td>
                <td style={{ padding: '9px 14px', color: '#F59E0B' }}>${t.total_cost_usd.toFixed(6)}</td>
                <td style={{ padding: '9px 14px' }}>
                  <span style={{ padding: '2px 7px', borderRadius: 3, fontSize: 10, background: t.status === 'error' ? '#EF444420' : '#10B98120', color: t.status === 'error' ? '#EF4444' : '#10B981' }}>
                    {t.status}
                  </span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {/* Pagination footer */}
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginTop: 16, paddingTop: 12, borderTop: '1px solid #0F1F35' }}>
        {/* Result count */}
        <div style={{ fontSize: 11, color: '#475569' }}>
          {!isLoading && allItems.length > 0
            ? total > 0
              ? `Showing ${rangeStart}–${rangeEnd} of ${total.toLocaleString()} results`
              : `Showing ${rangeStart}–${rangeEnd} results`
            : null
          }
          {!isLoading && search && filtered.length !== allItems.length && (
            <span style={{ color: '#3B82F6', marginLeft: 8 }}>
              ({filtered.length} matching search)
            </span>
          )}
        </div>

        {/* Page navigation */}
        <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
          <button
            onClick={() => setPageIndex(p => Math.max(0, p - 1))}
            disabled={!hasPrev}
            style={{
              ...paginationBtn,
              opacity: hasPrev ? 1 : 0.35,
              cursor: hasPrev ? 'pointer' : 'not-allowed',
            }}
          >
            ← Prev
          </button>
          <span style={{ fontSize: 11, color: '#475569' }}>
            Page {pageIndex + 1}{total > 0 ? ` of ${Math.ceil(total / PAGE_SIZE)}` : ''}
          </span>
          <button
            onClick={() => setPageIndex(p => p + 1)}
            disabled={!hasNext}
            style={{
              ...paginationBtn,
              opacity: hasNext ? 1 : 0.35,
              cursor: hasNext ? 'pointer' : 'not-allowed',
            }}
          >
            Next →
          </button>
        </div>
      </div>
    </div>
  )
}

const selectStyle: React.CSSProperties = {
  padding: '6px 12px', borderRadius: 6, fontSize: 11,
  background: '#0D1B2A', border: '1px solid #1E3A5F',
  color: '#CBD5E1', outline: 'none', cursor: 'pointer',
  fontFamily: "'JetBrains Mono',monospace",
}

const paginationBtn: React.CSSProperties = {
  padding: '6px 14px',
  background: '#0D1B2A',
  border: '1px solid #1E3A5F',
  borderRadius: 6,
  color: '#60A5FA',
  fontSize: 11,
  fontFamily: "'JetBrains Mono',monospace",
}
