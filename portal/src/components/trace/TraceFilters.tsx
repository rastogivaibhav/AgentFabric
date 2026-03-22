import { TraceSavedView } from '../../hooks/api'

export interface TraceFilterState {
  framework: string
  status: string
  provider: string
  model: string
  app_name: string
  environment: string
  blocked: string
  search: string
}

const selectStyle: React.CSSProperties = {
  padding: '6px 12px',
  borderRadius: 6,
  fontSize: 11,
  background: '#0D1B2A',
  border: '1px solid #1E3A5F',
  color: '#CBD5E1',
  outline: 'none',
  cursor: 'pointer',
  fontFamily: "'JetBrains Mono',monospace",
}

const inputStyle: React.CSSProperties = {
  ...selectStyle,
  cursor: 'text',
}

export default function TraceFilters({
  value,
  onChange,
  savedViews = [],
  onApplySavedView,
  onSaveCurrentView,
}: {
  value: TraceFilterState
  onChange: (next: TraceFilterState) => void
  savedViews?: TraceSavedView[]
  onApplySavedView?: (view: TraceSavedView) => void
  onSaveCurrentView?: () => void
}) {
  const setField = (key: keyof TraceFilterState, fieldValue: string) => onChange({ ...value, [key]: fieldValue })

  return (
    <div style={{ display: 'grid', gap: 12, marginBottom: 20 }}>
      <div style={{ display: 'flex', gap: 12, flexWrap: 'wrap' }}>
        <select value={value.framework} onChange={e => setField('framework', e.target.value)} style={selectStyle}>
          <option value="">All Frameworks</option>
          <option value="crewai">crewai</option>
          <option value="langgraph">langgraph</option>
          <option value="google_adk">google_adk</option>
          <option value="openai_agents">openai_agents</option>
          <option value="claude_agents">claude_agents</option>
        </select>
        <select value={value.status} onChange={e => setField('status', e.target.value)} style={selectStyle}>
          <option value="">All Status</option>
          <option value="ok">OK</option>
          <option value="partial">Partial</option>
          <option value="error">Error</option>
        </select>
        <select value={value.provider} onChange={e => setField('provider', e.target.value)} style={selectStyle}>
          <option value="">All Providers</option>
          <option value="openai">OpenAI</option>
          <option value="anthropic">Anthropic</option>
          <option value="google">Google</option>
        </select>
        <select value={value.blocked} onChange={e => setField('blocked', e.target.value)} style={selectStyle}>
          <option value="">All Enforcement</option>
          <option value="true">Blocked only</option>
        </select>
        <input value={value.model} onChange={e => setField('model', e.target.value)} placeholder="Model" style={{ ...inputStyle, minWidth: 180 }} />
        <input value={value.app_name} onChange={e => setField('app_name', e.target.value)} placeholder="App name" style={{ ...inputStyle, minWidth: 180 }} />
        <input value={value.environment} onChange={e => setField('environment', e.target.value)} placeholder="Environment" style={{ ...inputStyle, minWidth: 160 }} />
      </div>

      <div style={{ display: 'flex', gap: 12, flexWrap: 'wrap', alignItems: 'center' }}>
        <input
          value={value.search}
          onChange={e => setField('search', e.target.value)}
          placeholder="Search trace, model, provider, app, user..."
          style={{ ...inputStyle, flex: 1, minWidth: 320 }}
        />
        <select
          defaultValue=""
          onChange={e => {
            const view = savedViews.find(item => String(item.id) === e.target.value)
            if (view) onApplySavedView?.(view)
            e.currentTarget.value = ''
          }}
          style={{ ...selectStyle, minWidth: 220 }}
        >
          <option value="">Saved views</option>
          {savedViews.map(view => (
            <option key={view.id} value={String(view.id)}>
              {view.name}
            </option>
          ))}
        </select>
        <button
          onClick={onSaveCurrentView}
          style={{
            padding: '6px 12px',
            borderRadius: 6,
            fontSize: 11,
            background: '#102742',
            border: '1px solid #3B82F6',
            color: '#93C5FD',
            cursor: 'pointer',
            fontFamily: "'JetBrains Mono',monospace",
          }}
        >
          Save current view
        </button>
      </div>
    </div>
  )
}
