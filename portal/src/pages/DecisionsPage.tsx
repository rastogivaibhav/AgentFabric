import { useState, type CSSProperties } from 'react'
import { Link } from 'react-router-dom'
import { hasRole, useAuth } from '../hooks/auth'
import { useDecisions } from '../hooks/api'

const PAGE_SIZE = 50

const RESULT_COLORS: Record<string, string> = {
  deny:     'var(--protect)',
  block:    'var(--protect)',
  warn:     'var(--prove)',
  sanitize: 'var(--prove)',
  redact:   'var(--prove)',
  allow:    'var(--spend)',
  ok:       'var(--spend)',
  retry:    'var(--control)',
  error:    'var(--protect)',
}

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
        <div style={{ background: 'var(--layer-2)', border: '1px solid var(--layer-border)', borderRadius: 10, padding: 24, color: 'var(--text-secondary)' }}>
          Decision evidence is restricted to administrators.
        </div>
      </div>
    )
  }

  return (
    <div style={{ padding: '40px 48px', maxWidth: 1440, margin: '0 auto' }}>
      <div style={{ marginBottom: 28 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
          <span style={{ width: 8, height: 8, borderRadius: '50%', background: 'var(--protect)', display: 'inline-block' }} />
          <span style={{ fontSize: 10, fontWeight: 700, color: 'var(--protect)', letterSpacing: '0.1em' }}>PROTECT</span>
        </div>
        <h1 style={{ fontSize: 28, fontWeight: 700, color: 'var(--text-primary)', margin: 0, letterSpacing: '-0.02em' }}>Decision Log</h1>
        <p style={{ fontSize: 12, color: 'var(--text-tertiary)', marginTop: 8 }}>
          Unified evidence for policy, budget, fallback, routing, and retry decisions made by the AI Gateway.
        </p>
      </div>

      <div style={{ display: 'flex', gap: 12, marginBottom: 20 }}>
        <select id="decisions-type-filter" value={type} onChange={e => setType(e.target.value)} style={selectStyle}>
          <option value="">All types</option>
          <option value="policy">policy</option>
          <option value="budget">budget</option>
          <option value="fallback">fallback</option>
          <option value="routing">routing</option>
          <option value="retry">retry</option>
        </select>
        <select id="decisions-result-filter" value={result} onChange={e => setResult(e.target.value)} style={selectStyle}>
          <option value="">All results</option>
          <option value="allow">allow</option>
          <option value="deny">deny</option>
          <option value="warn">warn</option>
          <option value="sanitize">sanitize</option>
          <option value="retry">retry</option>
          <option value="error">error</option>
        </select>
        <div style={{ marginLeft: 'auto', fontSize: 12, color: 'var(--text-tertiary)', alignSelf: 'center' }}>
          {data?.items?.length ?? 0} decisions
        </div>
      </div>

      <div style={{ background: 'var(--layer-2)', border: '1px solid var(--layer-border)', borderRadius: 12, overflow: 'hidden' }}>
        {isLoading ? (
          <div style={{ padding: 32, color: 'var(--text-tertiary)', fontSize: 13 }}>Loading decision evidence...</div>
        ) : isError ? (
          <div style={{ padding: 32, color: 'var(--protect)', fontSize: 13 }}>Failed to load decision evidence.</div>
        ) : (data?.items?.length ?? 0) === 0 ? (
          <div style={{ padding: '48px 32px', color: 'var(--text-tertiary)', fontSize: 13, textAlign: 'center' }}>
            <div style={{ fontSize: 32, marginBottom: 12 }}>🛡️</div>
            No decisions matched the current filters.<br />
            <span style={{ fontSize: 11 }}>Try clearing the filters above, or route some traffic through the proxy.</span>
          </div>
        ) : (
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
            <thead>
              <tr style={{ background: 'var(--layer-1)', borderBottom: '1px solid var(--layer-border)' }}>
                {['Time', 'Type', 'Result', 'Trace', 'Explanation', 'Action Taken'].map(h => (
                  <th key={h} style={{ padding: '10px 14px', textAlign: 'left', fontSize: 9, fontWeight: 700, letterSpacing: '0.1em', color: 'var(--text-tertiary)', whiteSpace: 'nowrap' }}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {(data?.items ?? []).map(record => (
                <tr key={`${record.decision_id}-${record.id}`} style={{ borderBottom: '1px solid var(--layer-border)' }}>
                  <td style={{ padding: '10px 14px', color: 'var(--text-tertiary)', fontSize: 11, whiteSpace: 'nowrap' }}>{new Date(record.created_at).toLocaleString()}</td>
                  <td style={{ padding: '10px 14px' }}>
                    <span style={{ fontSize: 10, padding: '2px 8px', borderRadius: 4, background: 'var(--layer-1)', color: 'var(--text-secondary)', fontWeight: 600 }}>{record.type}</span>
                  </td>
                  <td style={{ padding: '10px 14px' }}>
                    <span style={{ fontSize: 10, padding: '2px 8px', borderRadius: 4, fontWeight: 700, color: RESULT_COLORS[record.result] ?? 'var(--text-secondary)', background: `color-mix(in srgb, ${RESULT_COLORS[record.result] ?? 'currentColor'} 12%, transparent)` }}>
                      {record.result?.toUpperCase()}
                    </span>
                  </td>
                  <td style={{ padding: '10px 14px' }}>
                    {record.trace_id ? (
                      <Link to={`/traces/${record.trace_id}`} style={{ color: 'var(--control)', textDecoration: 'none', fontFamily: 'monospace', fontSize: 11 }}>
                        {record.trace_id.slice(0, 12)}…
                      </Link>
                    ) : (
                      <span style={{ color: 'var(--text-tertiary)' }}>-</span>
                    )}
                  </td>
                  <td style={{ padding: '10px 14px', color: 'var(--text-secondary)', maxWidth: 300, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{record.explanation || record.reason || '-'}</td>
                  <td style={{ padding: '10px 14px', color: 'var(--text-tertiary)', fontSize: 11 }}>{record.action_taken || '-'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}

const selectStyle: CSSProperties = {
  background: 'var(--layer-2)',
  border: '1px solid var(--layer-border)',
  borderRadius: 8,
  color: 'var(--text-primary)',
  padding: '8px 12px',
  fontSize: 12,
  outline: 'none',
}
