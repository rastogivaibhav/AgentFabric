import { useState, type CSSProperties } from 'react'
import { useErrorReport } from '../hooks/api'

export default function ErrorAnalyticsPage() {
  const [since, setSince] = useState('24h')
  const { data: rows, isLoading, error } = useErrorReport(since)

  const sortedRows = [...(rows ?? [])].sort((a, b) => b.count - a.count)
  const totalErrors = sortedRows.reduce((sum, r) => sum + r.count, 0)
  const affectedTraces = sortedRows.reduce((sum, r) => sum + r.affected_traces, 0)

  return (
    <div style={{ padding: '40px 48px', maxWidth: 1440, margin: '0 auto' }}>
      <div style={{ marginBottom: 28, display: 'flex', justifyContent: 'space-between', alignItems: 'flex-end' }}>
        <div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
            <span style={{ width: 8, height: 8, borderRadius: '50%', background: 'var(--protect)', display: 'inline-block' }} />
            <span style={{ fontSize: 10, fontWeight: 700, color: 'var(--protect)', letterSpacing: '0.1em' }}>OBSERVE</span>
          </div>
          <h1 style={titleStyle}>Error Analytics</h1>
          <p style={subtleText}>Grouped error classes and frequencies across agents and models.</p>
        </div>
        <select id="error-window" style={selectStyle} value={since} onChange={e => setSince(e.target.value)}>
          <option value="1h">Last 1h</option>
          <option value="24h">Last 24h</option>
          <option value="7d">Last 7d</option>
        </select>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 20, marginBottom: 28 }}>
        <StatCard label="Total Errors" value={isLoading ? '…' : totalErrors.toLocaleString()} color="var(--protect)" />
        <StatCard label="Affected Traces" value={isLoading ? '…' : affectedTraces.toLocaleString()} color="var(--prove)" />
        <StatCard label="Unique Error Classes" value={isLoading ? '…' : sortedRows.length.toString()} color="var(--observe)" />
      </div>

      <div style={{ background: 'var(--layer-2)', border: '1px solid var(--layer-border)', borderRadius: 12, overflow: 'hidden' }}>
        <div style={{ padding: '14px 20px', borderBottom: '1px solid var(--layer-border)', fontSize: 10, fontWeight: 700, letterSpacing: '0.1em', color: 'var(--text-tertiary)' }}>
          ERROR FREQUENCY ({sortedRows.length})
        </div>
        {isLoading && <div style={{ padding: 32, color: 'var(--text-tertiary)', fontSize: 13 }}>Loading error report...</div>}
        {error && <div style={{ padding: 32, color: 'var(--protect)', fontSize: 13 }}>Failed to load error report.</div>}
        {!isLoading && !error && sortedRows.length === 0 && (
          <div style={{ textAlign: 'center', padding: '48px 16px', color: 'var(--text-tertiary)', fontSize: 13 }}>
            <div style={{ fontSize: 32, marginBottom: 12 }}>✓</div>
            No errors recorded in this time window.
          </div>
        )}
        {!isLoading && !error && sortedRows.length > 0 && (
          <table style={{ width: '100%', borderCollapse: 'collapse', textAlign: 'left', fontSize: 12 }}>
            <thead>
              <tr style={{ background: 'var(--layer-1)' }}>
                {['Error Class', 'Agent', 'Model', 'Count', 'Affected Traces', 'Last Seen'].map(h => (
                  <th key={h} style={thStyle}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {sortedRows.map((row, idx) => (
                <tr key={idx} style={{ borderBottom: '1px solid var(--layer-border)' }}>
                  <td style={{ ...tdStyle, color: 'var(--protect)', fontWeight: 600 }}>{row.error_class}</td>
                  <td style={tdStyle}>{row.agent_name || '—'}</td>
                  <td style={tdStyle}>{row.model || '—'}</td>
                  <td style={{ ...tdStyle, fontWeight: 700, color: 'var(--text-primary)' }}>{row.count.toLocaleString()}</td>
                  <td style={tdStyle}>{row.affected_traces.toLocaleString()}</td>
                  <td style={{ ...tdStyle, fontSize: 10, color: 'var(--text-tertiary)' }}>{new Date(row.last_seen).toLocaleString()}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}

function StatCard({ label, value, color }: { label: string; value: string; color: string }) {
  return (
    <div style={{ background: 'var(--layer-2)', border: '1px solid var(--layer-border)', borderRadius: 12, padding: 20 }}>
      <div style={{ fontSize: 10, color: 'var(--text-tertiary)', letterSpacing: '0.08em', marginBottom: 10, fontWeight: 700 }}>{label.toUpperCase()}</div>
      <div style={{ fontSize: 32, fontWeight: 700, color, letterSpacing: '-0.02em' }}>{value}</div>
    </div>
  )
}

const titleStyle: CSSProperties = { fontSize: 28, fontWeight: 700, color: 'var(--text-primary)', margin: 0, letterSpacing: '-0.02em' }
const subtleText: CSSProperties = { fontSize: 12, color: 'var(--text-tertiary)', marginTop: 4 }
const thStyle: CSSProperties = { padding: '10px 16px', fontSize: 9, color: 'var(--text-tertiary)', fontWeight: 700, letterSpacing: '0.08em', borderBottom: '1px solid var(--layer-border)' }
const tdStyle: CSSProperties = { padding: '11px 16px', fontSize: 12, color: 'var(--text-secondary)' }
const selectStyle: CSSProperties = { background: 'var(--layer-2)', border: '1px solid var(--layer-border)', borderRadius: 8, color: 'var(--text-primary)', padding: '8px 12px', fontSize: 12, outline: 'none' }
