import { useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  TraceSavedView,
  useDeleteTraceSavedView,
  useTraceSavedViews,
  useTraces,
  useUpsertTraceSavedView,
} from '../hooks/api'
import TraceFilters, { TraceFilterState } from '../components/trace/TraceFilters'

const FW_COLORS: Record<string, string> = {
  crewai: '#FF6B35',
  langgraph: '#4ECDC4',
  google_adk: 'var(--control)',
  openai_agents: 'var(--spend)',
  claude_agents: 'var(--prove)',
  unknown: 'var(--text-tertiary)',
}

const PAGE_SIZE = 50

const defaultFilters: TraceFilterState = {
  framework: '',
  status: '',
  provider: '',
  model: '',
  app_name: '',
  environment: '',
  blocked: '',
  search: '',
}

export default function TracesPage() {
  const nav = useNavigate()
  const [filters, setFilters] = useState<TraceFilterState>(defaultFilters)
  const [cursorStack, setCursorStack] = useState<string[]>([])
  const [selectedTraceIds, setSelectedTraceIds] = useState<string[]>([])

  const cursor = cursorStack[cursorStack.length - 1] ?? ''
  const query = useMemo(
    () => ({
      framework: filters.framework,
      status: filters.status,
      provider: filters.provider,
      model: filters.model,
      app_name: filters.app_name,
      environment: filters.environment,
      blocked: filters.blocked,
      search: filters.search,
      limit: String(PAGE_SIZE),
      cursor,
    }),
    [filters, cursor],
  )

  const { data, isLoading, refetch } = useTraces(query)
  const { data: savedViews = [] } = useTraceSavedViews()
  const upsertSavedView = useUpsertTraceSavedView()
  const deleteSavedView = useDeleteTraceSavedView()

  const items = data?.items ?? []
  const total = data?.total ?? 0
  const hasPrev = cursorStack.length > 0
  const hasNext = Boolean(data?.next_cursor)

  const resetPagination = () => setCursorStack([])

  const onSaveCurrentView = async () => {
    const name = window.prompt('Saved view name')
    if (!name?.trim()) return
    await upsertSavedView.mutateAsync({
      name: name.trim(),
      filters: Object.fromEntries(Object.entries(filters).filter(([, value]) => value)),
    })
  }

  const onApplySavedView = (view: TraceSavedView) => {
    setFilters({
      ...defaultFilters,
      ...view.filters,
    })
    resetPagination()
  }

  const toggleSelected = (traceId: string) => {
    setSelectedTraceIds(current => {
      if (current.includes(traceId)) return current.filter(item => item !== traceId)
      if (current.length >= 2) return [current[1], traceId]
      return [...current, traceId]
    })
  }

  return (
    <div style={{ padding: 32 }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 24 }}>
        <div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 8, color: 'var(--observe)', fontWeight: 600, fontSize: 10, letterSpacing: '0.1em' }}>
            <span style={{ width: 8, height: 8, borderRadius: '50%', background: 'var(--observe)' }} />
            OBSERVE
          </div>
          <h1 style={{ fontSize: 20, fontWeight: 700, color: 'var(--text-primary)', margin: 0 }}>Traces</h1>
          <div style={{ marginTop: 6, fontSize: 11, color: 'var(--text-tertiary)' }}>
            Search, save filtered views, and compare runtime traces side by side.
          </div>
        </div>
        <div style={{ display: 'flex', gap: 10 }}>
          <button onClick={() => refetch()} style={actionButton('#60A5FA')}>
            Refresh
          </button>
          <button
            disabled={selectedTraceIds.length !== 2}
            onClick={() => nav(`/traces/compare?left=${selectedTraceIds[0]}&right=${selectedTraceIds[1]}`)}
            style={{ ...actionButton('var(--prove)'), opacity: selectedTraceIds.length === 2 ? 1 : 0.45, cursor: selectedTraceIds.length === 2 ? 'pointer' : 'not-allowed' }}
          >
            Compare selected
          </button>
        </div>
      </div>

      <TraceFilters
        value={filters}
        onChange={next => {
          setFilters(next)
          resetPagination()
        }}
        savedViews={savedViews}
        onApplySavedView={onApplySavedView}
        onSaveCurrentView={onSaveCurrentView}
      />

      {savedViews.length > 0 && (
        <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginBottom: 16 }}>
          {savedViews.map(view => (
            <div key={view.id} style={{ display: 'flex', alignItems: 'center', gap: 6, background: 'var(--layer-2)', border: '1px solid var(--layer-border)', borderRadius: 999, padding: '4px 10px' }}>
              <button onClick={() => onApplySavedView(view)} style={{ background: 'transparent', border: 'none', color: '#93C5FD', cursor: 'pointer', fontSize: 10 }}>
                {view.name}
              </button>
              <button onClick={() => deleteSavedView.mutate(view.id)} style={{ background: 'transparent', border: 'none', color: 'var(--protect)', cursor: 'pointer', fontSize: 10 }}>
                x
              </button>
            </div>
          ))}
        </div>
      )}

      <div style={{ background: 'var(--layer-2)', border: '1px solid var(--layer-border)', borderRadius: 10, overflow: 'hidden' }}>
        <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
          <thead>
            <tr style={{ borderBottom: '1px solid var(--layer-border)' }}>
              {['Compare', 'Trace ID', 'Framework', 'Root Span', 'Started', 'Duration', 'Spans', 'Tokens', 'Cost', 'Status'].map(h => (
                <th key={h} style={{ padding: '10px 14px', textAlign: 'left', fontSize: 10, color: 'var(--text-tertiary)', letterSpacing: '0.1em', fontWeight: 700 }}>{h}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {isLoading && (
              <tr><td colSpan={10} style={{ padding: 32, textAlign: 'center', color: 'var(--text-tertiary)' }}>Loading...</td></tr>
            )}
            {!isLoading && items.length === 0 && (
              <tr><td colSpan={10} style={{ padding: 32, textAlign: 'center', color: 'var(--text-tertiary)', fontSize: 12 }}>No traces matched the current filters.</td></tr>
            )}
            {items.map((trace, index) => {
              const selected = selectedTraceIds.includes(trace.id)
              return (
                <tr
                  key={trace.id}
                  onClick={() => nav(`/traces/${trace.id}`)}
                  style={{
                    borderBottom: '1px solid var(--layer-border)',
                    cursor: 'pointer',
                    background: selected ? 'var(--layer-border)25' : index % 2 === 0 ? 'transparent' : 'var(--layer-1)30',
                  }}
                >
                  <td style={{ padding: '9px 14px' }} onClick={e => e.stopPropagation()}>
                    <input type="checkbox" checked={selected} onChange={() => toggleSelected(trace.id)} />
                  </td>
                  <td style={{ padding: '9px 14px', color: 'var(--control)', fontFamily: 'monospace', fontSize: 11 }}>{trace.id.substring(0, 16)}...</td>
                  <td style={{ padding: '9px 14px' }}>
                    <span style={{ padding: '2px 7px', borderRadius: 3, fontSize: 10, background: (FW_COLORS[trace.framework] ?? 'var(--text-tertiary)') + '20', color: FW_COLORS[trace.framework] ?? 'var(--text-tertiary)' }}>
                      {trace.framework}
                    </span>
                  </td>
                  <td style={{ padding: '9px 14px', color: 'var(--text-secondary)', maxWidth: 220, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{trace.root_span_name}</td>
                  <td style={{ padding: '9px 14px', color: 'var(--text-tertiary)', fontSize: 11 }}>{new Date(trace.start_time).toLocaleString()}</td>
                  <td style={{ padding: '9px 14px', color: 'var(--text-secondary)' }}>{(trace.duration_ns / 1_000_000).toFixed(0)}ms</td>
                  <td style={{ padding: '9px 14px', color: 'var(--text-secondary)' }}>{trace.span_count}</td>
                  <td style={{ padding: '9px 14px', color: 'var(--text-secondary)' }}>{trace.total_tokens.toLocaleString()}</td>
                  <td style={{ padding: '9px 14px', color: 'var(--prove)' }}>${trace.total_cost_usd.toFixed(6)}</td>
                  <td style={{ padding: '9px 14px' }}>
                    <span style={{ padding: '2px 7px', borderRadius: 3, fontSize: 10, background: statusColor(trace.status) + '20', color: statusColor(trace.status) }}>
                      {trace.status}
                    </span>
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>

      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginTop: 16, paddingTop: 12, borderTop: '1px solid var(--layer-border)' }}>
        <div style={{ fontSize: 11, color: 'var(--text-tertiary)' }}>
          {items.length > 0 ? `Showing ${items.length} traces${total > 0 ? ` of ${total.toLocaleString()}` : ''}` : null}
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
          <button
            onClick={() => setCursorStack(stack => stack.slice(0, -1))}
            disabled={!hasPrev}
            style={{ ...actionButton('#60A5FA'), opacity: hasPrev ? 1 : 0.35, cursor: hasPrev ? 'pointer' : 'not-allowed' }}
          >
            Prev
          </button>
          <button
            onClick={() => data?.next_cursor && setCursorStack(stack => [...stack, data.next_cursor!])}
            disabled={!hasNext}
            style={{ ...actionButton('#60A5FA'), opacity: hasNext ? 1 : 0.35, cursor: hasNext ? 'pointer' : 'not-allowed' }}
          >
            Next
          </button>
        </div>
      </div>
    </div>
  )
}

function actionButton(color: string): React.CSSProperties {
  return {
    padding: '6px 14px',
    background: 'var(--layer-2)',
    border: `1px solid ${color}`,
    borderRadius: 6,
    color,
    fontSize: 11,
    cursor: 'pointer',
    fontFamily: "'JetBrains Mono',monospace",
  }
}

function statusColor(status: string) {
  if (status === 'error') return 'var(--protect)'
  if (status === 'partial') return 'var(--prove)'
  return 'var(--spend)'
}
