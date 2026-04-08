import { useState, type CSSProperties } from 'react'
import { Link } from 'react-router-dom'
import { useRuns, type Run } from '../hooks/api'

export default function RunsPage() {
  const [framework, setFramework] = useState('')
  const [cursor, setCursor] = useState('')
  const { data, isLoading, error } = useRuns({ framework, limit: 100, cursor })

  const runs = data?.items ?? []

  return (
    <div style={{ padding: 32 }}>
      <div style={{ marginBottom: 24, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 8, color: 'var(--observe)', fontWeight: 600, fontSize: 10, letterSpacing: '0.1em' }}>
            <span style={{ width: 8, height: 8, borderRadius: '50%', background: 'var(--observe)' }} />
            OBSERVE
          </div>
          <h1 style={titleStyle}>Runs</h1>
          <p style={subtleText}>Detailed function execution trees and feedback tracking.</p>
        </div>
        <div style={{ display: 'flex', gap: 12 }}>
          <label style={{ ...subtleText, display: 'flex', alignItems: 'center', gap: 8 }}>
            Framework
            <select
              style={selectStyle}
              value={framework}
              onChange={e => {
                setFramework(e.target.value)
                setCursor('')
              }}
            >
              <option value="">All</option>
              <option value="langchain">LangChain</option>
              <option value="crewai">CrewAI</option>
              <option value="autogen">AutoGen</option>
            </select>
          </label>
        </div>
      </div>

      <div style={panelStyle}>
        {isLoading && <div style={subtleText}>Loading runs...</div>}
        {error && <div style={{ color: 'var(--protect)', fontSize: 12 }}>Failed to load runs.</div>}
        {!isLoading && !error && runs.length === 0 && (
          <div style={{ textAlign: 'center', padding: 40, ...subtleText }}>
            No runs found.
          </div>
        )}
        {!isLoading && !error && runs.length > 0 && (
          <table style={{ width: '100%', borderCollapse: 'collapse', textAlign: 'left' }}>
            <thead>
              <tr style={{ borderBottom: '1px solid var(--layer-border)' }}>
                <th style={thStyle}>Status</th>
                <th style={thStyle}>Agent</th>
                <th style={thStyle}>Framework</th>
                <th style={thStyle}>Model</th>
                <th style={thStyle}>Cost</th>
                <th style={thStyle}>Tokens</th>
                <th style={thStyle}>Date</th>
                <th style={thStyle}>Action</th>
              </tr>
            </thead>
            <tbody>
              {runs.map(run => (
                <tr key={run.id} style={{ borderBottom: '1px solid var(--layer-border)' }}>
                  <td style={tdStyle}>
                    <span style={{
                      padding: '2px 8px', borderRadius: 4, fontSize: 10,
                      background: run.status === 'error' ? 'var(--protect)20' : 'var(--spend)20',
                      color: run.status === 'error' ? 'var(--protect)' : 'var(--spend)'
                    }}>
                      {run.status.toUpperCase()}
                    </span>
                  </td>
                  <td style={{ ...tdStyle, color: 'var(--text-primary)', fontWeight: 600 }}>{run.agent_name}</td>
                  <td style={tdStyle}>{run.framework}</td>
                  <td style={tdStyle}>{run.model}</td>
                  <td style={{ ...tdStyle, fontFamily: 'monospace' }}>${run.total_cost_usd.toFixed(4)}</td>
                  <td style={{ ...tdStyle, fontFamily: 'monospace' }}>{run.total_tokens.toLocaleString()}</td>
                  <td style={{ ...tdStyle, fontSize: 10 }}>{new Date(run.start_time).toLocaleString()}</td>
                  <td style={tdStyle}>
                    <Link to={`/runs/${run.id}`} style={{ color: 'var(--control)', textDecoration: 'none', fontSize: 12 }}>View Run →</Link>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {data?.has_more && (
        <div style={{ display: 'flex', justifyContent: 'center', marginTop: 16 }}>
          <button style={ghostBtn} onClick={() => setCursor(data.next_cursor || '')}>
            Load More
          </button>
        </div>
      )}
    </div>
  )
}

const titleStyle: CSSProperties = { fontSize: 22, fontWeight: 700, color: 'var(--text-primary)', margin: 0 }
const subtleText: CSSProperties = { fontSize: 12, color: 'var(--text-tertiary)', marginTop: 4 }
const panelStyle: CSSProperties = { background: 'var(--layer-2)', border: '1px solid var(--layer-border)', borderRadius: 10, padding: 24 }
const thStyle: CSSProperties = { padding: '12px 16px', fontSize: 10, color: 'var(--text-tertiary)', fontWeight: 600, letterSpacing: '0.05em' }
const tdStyle: CSSProperties = { padding: '12px 16px', fontSize: 12, color: 'var(--text-secondary)' }
const selectStyle: CSSProperties = { background: 'var(--layer-1)', border: '1px solid var(--layer-border)', borderRadius: 8, color: 'var(--text-primary)', padding: '6px 10px', fontSize: 12 }
const ghostBtn: CSSProperties = { background: 'none', border: '1px solid var(--layer-border)', borderRadius: 8, color: 'var(--text-tertiary)', padding: '6px 12px', fontSize: 12, cursor: 'pointer' }
