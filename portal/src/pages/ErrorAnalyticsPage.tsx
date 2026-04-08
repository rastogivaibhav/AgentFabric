import { useState, type CSSProperties } from 'react'
import { useErrorReport } from '../hooks/api'

export default function ErrorAnalyticsPage() {
  const [since, setSince] = useState('24h')
  const { data: rows, isLoading, error } = useErrorReport(since)

  const sortedRows = [...(rows ?? [])].sort((a, b) => b.count - a.count)

  const totalErrors = sortedRows.reduce((sum, r) => sum + r.count, 0)
  const affectedTraces = sortedRows.reduce((sum, r) => sum + r.affected_traces, 0)

  return (
    <div style={{ padding: 32 }}>
      <div style={{ marginBottom: 24, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <div>
          <h1 style={titleStyle}>Error Analytics</h1>
          <p style={subtleText}>Grouped error classes and frequencies across agents and models.</p>
        </div>
        <div>
          <select style={selectStyle} value={since} onChange={e => setSince(e.target.value)}>
            <option value="1h">Last 1h</option>
            <option value="24h">Last 24h</option>
            <option value="7d">Last 7d</option>
          </select>
        </div>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 16, marginBottom: 24 }}>
        <StatCard label="Total Errors" value={isLoading ? '...' : totalErrors.toLocaleString()} />
        <StatCard label="Affected Traces" value={isLoading ? '...' : affectedTraces.toLocaleString()} />
        <StatCard label="Unique Error Classes" value={isLoading ? '...' : sortedRows.length.toString()} />
      </div>

      <div style={panelStyle}>
        <div style={sectionLabel}>ERROR FREQUENCY ({sortedRows.length})</div>
        {isLoading && <div style={subtleText}>Loading error report...</div>}
        {error && <div style={{ color: '#EF4444' }}>Failed to load error report.</div>}
        {!isLoading && !error && sortedRows.length === 0 && (
          <div style={{ textAlign: 'center', padding: 40, ...subtleText }}>
            No errors recorded in this time window.
          </div>
        )}
        {!isLoading && !error && sortedRows.length > 0 && (
          <table style={{ width: '100%', borderCollapse: 'collapse', textAlign: 'left' }}>
            <thead>
              <tr style={{ borderBottom: '1px solid #1E3A5F' }}>
                <th style={thStyle}>Error Class</th>
                <th style={thStyle}>Agent</th>
                <th style={thStyle}>Model</th>
                <th style={thStyle}>Count</th>
                <th style={thStyle}>Affected Traces</th>
                <th style={thStyle}>Last Seen</th>
              </tr>
            </thead>
            <tbody>
              {sortedRows.map((row, idx) => (
                <tr key={idx} style={{ borderBottom: '1px solid #0F1F35' }}>
                  <td style={{ ...tdStyle, color: '#EF4444', fontWeight: 600 }}>{row.error_class}</td>
                  <td style={tdStyle}>{row.agent_name || '—'}</td>
                  <td style={tdStyle}>{row.model || '—'}</td>
                  <td style={{ ...tdStyle, fontWeight: 700 }}>{row.count.toLocaleString()}</td>
                  <td style={tdStyle}>{row.affected_traces.toLocaleString()}</td>
                  <td style={{ ...tdStyle, fontSize: 10 }}>{new Date(row.last_seen).toLocaleString()}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}

function StatCard({ label, value }: { label: string; value: string }) {
  return (
    <div style={{ background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 10, padding: 20 }}>
      <div style={{ fontSize: 11, color: '#64748B', letterSpacing: '0.05em', marginBottom: 8 }}>{label.toUpperCase()}</div>
      <div style={{ fontSize: 24, fontWeight: 700, color: '#F0F9FF' }}>{value}</div>
    </div>
  )
}

const titleStyle: CSSProperties = { fontSize: 22, fontWeight: 700, color: '#F0F9FF', margin: 0 }
const subtleText: CSSProperties = { fontSize: 12, color: '#64748B', marginTop: 4 }
const panelStyle: CSSProperties = { background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 10, padding: 24 }
const sectionLabel: CSSProperties = { fontSize: 10, color: '#334155', letterSpacing: '0.12em', marginBottom: 16 }
const thStyle: CSSProperties = { padding: '12px 16px', fontSize: 10, color: '#64748B', fontWeight: 600, letterSpacing: '0.05em' }
const tdStyle: CSSProperties = { padding: '12px 16px', fontSize: 12, color: '#E2E8F0' }
const selectStyle: CSSProperties = { background: '#060A14', border: '1px solid #0F1F35', borderRadius: 8, color: '#E2E8F0', padding: '6px 10px', fontSize: 12 }
