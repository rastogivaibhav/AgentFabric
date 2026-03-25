import { useState } from 'react'
import { Link } from 'react-router-dom'
import { hasRole, useAuth } from '../hooks/auth'
import { useDecisions } from '../hooks/api'

const PAGE_SIZE = 50

export default function DecisionsPage() {
  const { user } = useAuth()
  const isAdmin = hasRole(user, ['admin'])
  const [type, setType] = useState('')
  const [result, setResult] = useState('')

  const { data, isLoading, isError } = useDecisions({
    limit: String(PAGE_SIZE),
    ...(type ? { type } : {}),
    ...(result ? { result } : {}),
  })

  if (!isAdmin) {
    return (
      <div style={{ padding: 32 }}>
        <div style={{ background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 10, padding: 24, color: '#94A3B8' }}>
          Decision evidence is restricted to administrators.
        </div>
      </div>
    )
  }

  return (
    <div style={{ padding: 32 }}>
      <div style={{ marginBottom: 24 }}>
        <h1 style={{ fontSize: 22, fontWeight: 700, color: '#F0F9FF', margin: 0 }}>Decision Explorer</h1>
        <p style={{ fontSize: 12, color: '#64748B', marginTop: 8 }}>
          Unified evidence for policy, budget, fallback, routing, and retry decisions.
        </p>
      </div>

      <div style={{ display: 'flex', gap: 12, marginBottom: 16 }}>
        <select value={type} onChange={e => setType(e.target.value)} style={inputStyle}>
          <option value="">all types</option>
          <option value="policy">policy</option>
          <option value="budget">budget</option>
          <option value="fallback">fallback</option>
          <option value="routing">routing</option>
          <option value="retry">retry</option>
        </select>
        <select value={result} onChange={e => setResult(e.target.value)} style={inputStyle}>
          <option value="">all results</option>
          <option value="deny">deny</option>
          <option value="warn">warn</option>
          <option value="sanitize">sanitize</option>
          <option value="retry">retry</option>
          <option value="error">error</option>
        </select>
      </div>

      <div style={{ background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 10, overflow: 'hidden' }}>
        {isLoading ? (
          <div style={{ padding: 24, color: '#64748B' }}>Loading decision evidence...</div>
        ) : isError ? (
          <div style={{ padding: 24, color: '#EF4444' }}>Failed to load decision evidence.</div>
        ) : (data?.items?.length ?? 0) === 0 ? (
          <div style={{ padding: 24, color: '#64748B' }}>No decisions matched the current filters.</div>
        ) : (
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
            <thead>
              <tr style={{ borderBottom: '1px solid #0F1F35', color: '#64748B', textAlign: 'left' }}>
                {['Time', 'Type', 'Result', 'Trace', 'Explanation', 'Action'].map(header => (
                  <th key={header} style={{ padding: '10px 12px' }}>{header}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {(data?.items ?? []).map(record => (
                <tr key={`${record.decision_id}-${record.id}`} style={{ borderBottom: '1px solid #0A1020' }}>
                  <td style={{ padding: '10px 12px', color: '#64748B' }}>{new Date(record.created_at).toLocaleString()}</td>
                  <td style={{ padding: '10px 12px', color: '#CBD5E1' }}>{record.type}</td>
                  <td style={{ padding: '10px 12px', color: '#F59E0B' }}>{record.result}</td>
                  <td style={{ padding: '10px 12px' }}>
                    {record.trace_id ? (
                      <Link to={`/traces/${record.trace_id}`} style={{ color: '#60A5FA', textDecoration: 'none' }}>
                        {record.trace_id.slice(0, 12)}...
                      </Link>
                    ) : (
                      <span style={{ color: '#475569' }}>-</span>
                    )}
                  </td>
                  <td style={{ padding: '10px 12px', color: '#94A3B8' }}>{record.explanation || record.reason || '-'}</td>
                  <td style={{ padding: '10px 12px', color: '#475569' }}>{record.action_taken || '-'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}

const inputStyle = {
  background: '#071525',
  border: '1px solid #1E3A5F',
  borderRadius: 6,
  color: '#F0F9FF',
  padding: '8px 10px',
  fontSize: 12,
}
