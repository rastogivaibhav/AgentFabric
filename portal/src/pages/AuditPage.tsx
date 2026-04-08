import { useState, type CSSProperties } from 'react'
import { Archive, ChevronLeft, ChevronRight, Download, History, Lock, RefreshCw } from 'lucide-react'
import { hasRole, useAuth } from '../hooks/auth'
import { useControlAudit, useControlHistory, useCreateEvidenceBundle, useEvidenceBundles, type AdminAuditEntry } from '../hooks/api'

const PAGE_SIZE = 100

function truncate(value: string, len = 8): string {
  if (!value) return '-'
  return value.length <= len ? value : `${value.slice(0, len)}...`
}

function formatDate(iso: string): string {
  try {
    return new Date(iso).toLocaleString(undefined, { year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit' })
  } catch { return iso }
}

function OutcomeBadge({ outcome }: { outcome: AdminAuditEntry['outcome'] }) {
  const cfg: Record<string, { bg: string; color: string }> = {
    success: { bg: 'rgba(48,209,88,0.12)', color: 'var(--spend)' },
    error:   { bg: 'rgba(255,69,58,0.12)', color: 'var(--protect)' },
    failed:  { bg: 'rgba(255,69,58,0.12)', color: 'var(--protect)' },
    blocked: { bg: 'rgba(255,159,10,0.12)', color: 'var(--prove)' },
  }
  const c = cfg[outcome] ?? { bg: 'rgba(255,255,255,0.06)', color: 'var(--text-secondary)' }
  return (
    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4, padding: '2px 8px', borderRadius: 999, fontSize: 10, fontWeight: 600, background: c.bg, color: c.color }}>
      {outcome || 'unknown'}
    </span>
  )
}

function HashCell({ hash }: { hash: string }) {
  if (!hash) return <span style={{ color: 'var(--text-tertiary)', fontSize: 11 }}>-</span>
  return (
    <span title={hash} style={{ fontFamily: 'monospace', fontSize: 10, color: 'var(--text-tertiary)', cursor: 'help' }}>
      {hash.slice(0, 12)}…
    </span>
  )
}

function AccessDenied() {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: 'calc(100vh - 100px)', gap: 16, color: 'var(--text-tertiary)' }}>
      <Lock size={40} style={{ color: 'var(--text-tertiary)' }} />
      <p style={{ fontSize: 18, fontWeight: 600, color: 'var(--text-secondary)', margin: 0 }}>Access denied</p>
      <p style={{ fontSize: 13, margin: 0 }}>The audit log is restricted to administrators.</p>
    </div>
  )
}

export default function AuditPage() {
  const { user } = useAuth()
  const isAdmin = hasRole(user, ['admin'])
  const [offset, setOffset] = useState(0)
  const [bundleForm, setBundleForm] = useState({
    name: 'Rollout incident bundle',
    scope: 'incident',
    release_tag: '',
    prompt_id: '',
    environment: '',
    trace_id: '',
    reason: '',
  })

  const auditQuery = useControlAudit(PAGE_SIZE)
  const historyQuery = useControlHistory(25, offset)
  const bundlesQuery = useEvidenceBundles(12)
  const createBundle = useCreateEvidenceBundle()

  if (!isAdmin) return <AccessDenied />

  const auditEntries: AdminAuditEntry[] = auditQuery.data?.items ?? []
  const historyEntries = historyQuery.data?.items ?? []
  const bundles = bundlesQuery.data?.items ?? []
  const hasPrev = offset > 0
  const hasNext = Boolean(historyQuery.data?.has_more)

  const refreshAll = () => {
    void auditQuery.refetch()
    void historyQuery.refetch()
    void bundlesQuery.refetch()
  }

  const handleBundleSubmit = async () => {
    await createBundle.mutateAsync({
      name: bundleForm.name,
      scope: bundleForm.scope,
      release_tag: bundleForm.release_tag || undefined,
      prompt_id: bundleForm.prompt_id || undefined,
      environment: bundleForm.environment || undefined,
      trace_id: bundleForm.trace_id || undefined,
      reason: bundleForm.reason || undefined,
    })
  }

  const loading = auditQuery.isLoading || historyQuery.isLoading || bundlesQuery.isLoading
  const errored = auditQuery.isError || historyQuery.isError || bundlesQuery.isError

  const BASE_URL = import.meta.env.VITE_API_URL ?? 'http://localhost:8080'

  return (
    <div style={{ padding: '40px 48px', maxWidth: 1440, margin: '0 auto', display: 'flex', flexDirection: 'column', gap: 32 }}>
      {/* Header */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-end', gap: 16 }}>
        <div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
            <span style={{ width: 8, height: 8, borderRadius: '50%', background: 'var(--prove)', display: 'inline-block' }} />
            <span style={{ fontSize: 10, fontWeight: 700, color: 'var(--prove)', letterSpacing: '0.1em' }}>PROVE</span>
          </div>
          <h1 style={titleStyle}>Audit Log</h1>
          <p style={subtleText}>Control-plane audit records, append-only control history, and exportable evidence bundles.</p>
        </div>
        <button onClick={refreshAll}
          disabled={auditQuery.isFetching || historyQuery.isFetching || bundlesQuery.isFetching}
          style={ghostBtn}>
          <RefreshCw size={14} style={{ animation: (auditQuery.isFetching || historyQuery.isFetching) ? 'spin 1s linear infinite' : 'none' }} />
          Refresh
        </button>
      </div>

      {errored && (
        <div style={{ borderRadius: 10, background: 'rgba(255,69,58,0.08)', border: '1px solid rgba(255,69,58,0.2)', padding: '12px 16px', fontSize: 13, color: 'var(--protect)' }}>
          Failed to load one or more audit views. Check your connection and ensure your session is still valid.
        </div>
      )}

      {/* Summary cards */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 20 }}>
        {[
          { label: 'Control Audit', value: auditEntries.length, desc: 'Recent control-plane audit records' },
          { label: 'Control History', value: historyQuery.data?.total ?? historyEntries.length, desc: 'Append-only change records' },
          { label: 'Evidence Bundles', value: bundles.length, desc: 'Incident-ready exports' },
        ].map(card => (
          <div key={card.label} style={{ background: 'var(--layer-2)', border: '1px solid var(--layer-border)', borderRadius: 12, padding: '20px 24px' }}>
            <div style={{ fontSize: 10, color: 'var(--text-tertiary)', letterSpacing: '0.1em', fontWeight: 700, marginBottom: 10 }}>{card.label.toUpperCase()}</div>
            <div style={{ fontSize: 36, fontWeight: 700, color: 'var(--prove)', letterSpacing: '-0.02em' }}>{card.value}</div>
            <div style={{ fontSize: 12, color: 'var(--text-tertiary)', marginTop: 6 }}>{card.desc}</div>
          </div>
        ))}
      </div>

      {/* Evidence Bundle Export */}
      <div style={sectionCard}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 20 }}>
          <Archive size={18} style={{ color: 'var(--prove)' }} />
          <div>
            <h2 style={sectionTitle}>Evidence Bundle Export</h2>
            <p style={subtleText}>Capture a rollout or release incident into a tenant-scoped export bundle.</p>
          </div>
        </div>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 12 }}>
          {[
            { label: 'Bundle name', key: 'name', placeholder: 'Rollout incident bundle' },
            { label: 'Release tag', key: 'release_tag', placeholder: 'candidate-v7' },
            { label: 'Prompt ID', key: 'prompt_id', placeholder: 'support.system' },
            { label: 'Environment', key: 'environment', placeholder: 'staging' },
            { label: 'Trace ID', key: 'trace_id', placeholder: 'trace-abc123' },
          ].map(f => (
            <label key={f.key} style={labelStyle}>
              {f.label}
              <input style={inputStyle} value={(bundleForm as Record<string, string>)[f.key]} placeholder={f.placeholder}
                onChange={e => setBundleForm(cur => ({ ...cur, [f.key]: e.target.value }))} />
            </label>
          ))}
          <label style={labelStyle}>
            Scope
            <select style={inputStyle} value={bundleForm.scope} onChange={e => setBundleForm(cur => ({ ...cur, scope: e.target.value }))}>
              <option value="incident">Incident</option>
              <option value="release">Release review</option>
              <option value="audit">Audit package</option>
            </select>
          </label>
        </div>
        <label style={{ ...labelStyle, marginTop: 12, display: 'block' }}>
          Reason
          <textarea style={{ ...inputStyle, resize: 'vertical', minHeight: 80, display: 'block', width: '100%', marginTop: 6 } as CSSProperties}
            value={bundleForm.reason} placeholder="Rollout auto-paused after error rate breach in staging."
            onChange={e => setBundleForm(cur => ({ ...cur, reason: e.target.value }))} />
        </label>
        <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginTop: 16 }}>
          <button onClick={() => void handleBundleSubmit()} disabled={createBundle.isPending} style={primaryBtn}>
            <Archive size={14} />
            {createBundle.isPending ? 'Creating…' : 'Create Evidence Bundle'}
          </button>
          {createBundle.isError && <span style={{ fontSize: 13, color: 'var(--protect)' }}>Bundle creation failed.</span>}
          {createBundle.isSuccess && (
            <a href={`${BASE_URL}/api/v1/audit/evidence-bundles/${createBundle.data.id}/export`}
              style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 13, color: 'var(--control)', textDecoration: 'none' }}>
              <Download size={14} /> Download latest bundle
            </a>
          )}
        </div>
      </div>

      {/* Control History */}
      <div style={sectionCard}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, borderBottom: '1px solid var(--layer-border)', paddingBottom: 16, marginBottom: 0 }}>
          <History size={18} style={{ color: 'var(--prove)' }} />
          <div>
            <h2 style={sectionTitle}>Control History Timeline</h2>
            <p style={subtleText}>Immutable hash-chained provenance for control-plane changes.</p>
          </div>
        </div>
        <div>
          {loading
            ? Array.from({ length: 4 }).map((_, i) => (
                <div key={i} style={{ padding: '16px 0', borderBottom: '1px solid var(--layer-border)' }}>
                  <div style={{ height: 12, width: 200, borderRadius: 6, background: 'var(--layer-3)' }} />
                </div>
              ))
            : historyEntries.length === 0
              ? <div style={{ padding: '32px 0', fontSize: 13, color: 'var(--text-tertiary)' }}>No control history recorded yet.</div>
              : historyEntries.map(entry => (
                  <div key={entry.id} style={{ padding: '16px 0', borderBottom: '1px solid var(--layer-border)', display: 'flex', flexDirection: 'column', gap: 8 }}>
                    <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 8 }}>
                      <span style={{ padding: '2px 8px', borderRadius: 999, fontSize: 10, fontWeight: 600, background: 'rgba(255,159,10,0.12)', color: 'var(--prove)' }}>{entry.category}</span>
                      <span style={{ padding: '2px 8px', borderRadius: 999, fontSize: 10, background: 'var(--layer-3)', color: 'var(--text-secondary)' }}>{entry.action}</span>
                      <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--text-primary)' }}>{entry.target_type}: {entry.target_id || '-'}</span>
                      <span style={{ fontSize: 11, color: 'var(--text-tertiary)' }}>{formatDate(entry.created_at)}</span>
                    </div>
                    <p style={{ margin: 0, fontSize: 13, color: 'var(--text-secondary)' }}>{entry.reason || 'No explicit change reason recorded.'}</p>
                    <div style={{ display: 'flex', gap: 16, fontSize: 11, color: 'var(--text-tertiary)' }}>
                      <span>Actor: {entry.actor || '-'}</span>
                      <span>Outcome: {entry.outcome}</span>
                      <span>Chain: <HashCell hash={entry.entry_hash || ''} /></span>
                    </div>
                  </div>
                ))}
        </div>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', paddingTop: 16, fontSize: 12, color: 'var(--text-tertiary)' }}>
          <span>Rows {offset + 1}–{offset + historyEntries.length}</span>
          <div style={{ display: 'flex', gap: 8 }}>
            <button onClick={() => setOffset(Math.max(0, offset - 25))} disabled={!hasPrev || historyQuery.isFetching} style={ghostBtn}>
              <ChevronLeft size={14} /> Previous
            </button>
            <button onClick={() => setOffset(offset + 25)} disabled={!hasNext || historyQuery.isFetching} style={ghostBtn}>
              Next <ChevronRight size={14} />
            </button>
          </div>
        </div>
      </div>

      {/* Policy Audit Chain */}
      <div style={{ ...sectionCard, overflow: 'hidden', padding: 0 }}>
        <div style={{ padding: '20px 24px', borderBottom: '1px solid var(--layer-border)' }}>
          <h2 style={sectionTitle}>Policy Audit Chain</h2>
          <p style={subtleText}>Legacy control-plane audit records captured alongside enterprise memory.</p>
        </div>
        <div style={{ overflowX: 'auto' }}>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
            <thead>
              <tr style={{ background: 'var(--layer-1)' }}>
                {['#', 'Timestamp', 'Category', 'Action', 'Target', 'Actor', 'Outcome', 'Details'].map(h => (
                  <th key={h} style={{ padding: '10px 16px', textAlign: 'left', fontSize: 10, fontWeight: 700, letterSpacing: '0.08em', color: 'var(--text-tertiary)', whiteSpace: 'nowrap', borderBottom: '1px solid var(--layer-border)' }}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {loading
                ? Array.from({ length: 6 }).map((_, i) => (
                    <tr key={i}>
                      {Array.from({ length: 8 }).map((__, c) => (
                        <td key={c} style={{ padding: '12px 16px' }}>
                          <div style={{ height: 10, borderRadius: 4, background: 'var(--layer-3)', width: 60 }} />
                        </td>
                      ))}
                    </tr>
                  ))
                : auditEntries.length === 0
                  ? <tr><td colSpan={8} style={{ padding: '48px 16px', textAlign: 'center', color: 'var(--text-tertiary)' }}>No control audit entries found.</td></tr>
                  : auditEntries.map(entry => (
                      <tr key={entry.id} style={{ borderBottom: '1px solid var(--layer-border)' }}>
                        <td style={{ padding: '10px 16px', fontFamily: 'monospace', fontSize: 10, color: 'var(--text-tertiary)' }}>{entry.id}</td>
                        <td style={{ padding: '10px 16px', color: 'var(--text-secondary)', whiteSpace: 'nowrap' }}>{formatDate(entry.created_at)}</td>
                        <td style={{ padding: '10px 16px' }}>
                          <span style={{ padding: '2px 8px', borderRadius: 999, fontSize: 10, fontWeight: 600, background: 'rgba(255,159,10,0.12)', color: 'var(--prove)' }}>{entry.category}</span>
                        </td>
                        <td style={{ padding: '10px 16px', color: 'var(--text-primary)', fontWeight: 600 }}>{entry.action}</td>
                        <td style={{ padding: '10px 16px', fontFamily: 'monospace', fontSize: 10, color: 'var(--text-tertiary)' }}>
                          {entry.target_id ? `${entry.target_type}:${truncate(entry.target_id, 16)}` : entry.target_type}
                        </td>
                        <td style={{ padding: '10px 16px', color: 'var(--text-secondary)' }}>{entry.actor || '-'}</td>
                        <td style={{ padding: '10px 16px' }}><OutcomeBadge outcome={entry.outcome} /></td>
                        <td style={{ padding: '10px 16px', color: 'var(--text-secondary)', maxWidth: 280 }}>
                          <span title={entry.details || ''} style={{ display: 'block', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{entry.details || '-'}</span>
                        </td>
                      </tr>
                    ))}
            </tbody>
          </table>
        </div>
      </div>

      {/* Evidence bundles */}
      <div style={sectionCard}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 20 }}>
          <Archive size={18} style={{ color: 'var(--prove)' }} />
          <div>
            <h2 style={sectionTitle}>Recent Evidence Bundles</h2>
            <p style={subtleText}>Tenant-scoped exports for incident review and compliance handoff.</p>
          </div>
        </div>
        {bundles.length === 0 ? (
          <div style={{ border: '1px dashed var(--layer-border)', borderRadius: 10, padding: '32px 16px', textAlign: 'center', fontSize: 13, color: 'var(--text-tertiary)' }}>
            No evidence bundles have been generated yet.
          </div>
        ) : (
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(2, 1fr)', gap: 12 }}>
            {bundles.map(bundle => (
              <div key={bundle.id} style={{ border: '1px solid var(--layer-border)', borderRadius: 10, padding: 16, background: 'var(--layer-0)' }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 12 }}>
                  <div>
                    <div style={{ fontWeight: 600, color: 'var(--text-primary)', fontSize: 14 }}>{bundle.name}</div>
                    <div style={{ fontSize: 12, color: 'var(--text-tertiary)', marginTop: 4 }}>{bundle.scope} · {bundle.item_count} items · {formatDate(bundle.created_at)}</div>
                  </div>
                  <a href={`${BASE_URL}/api/v1/audit/evidence-bundles/${bundle.id}/export`}
                    style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 12, color: 'var(--control)', textDecoration: 'none', flexShrink: 0 }}>
                    <Download size={14} /> Export
                  </a>
                </div>
                {bundle.summary && bundle.summary.length > 0 && (
                  <ul style={{ margin: '12px 0 0', padding: '0 0 0 16px', display: 'flex', flexDirection: 'column', gap: 4 }}>
                    {bundle.summary.slice(0, 3).map(line => (
                      <li key={line} style={{ fontSize: 12, color: 'var(--text-secondary)' }}>{line}</li>
                    ))}
                  </ul>
                )}
              </div>
            ))}
          </div>
        )}
      </div>

      <p style={{ fontSize: 11, color: 'var(--text-tertiary)' }}>
        Control-plane records are tenant-scoped. Verification: <code style={{ fontFamily: 'monospace', background: 'var(--layer-2)', padding: '1px 6px', borderRadius: 4 }}>GET /api/v1/audit/verify</code>
      </p>

      <style>{`@keyframes spin { from { transform: rotate(0deg) } to { transform: rotate(360deg) } }`}</style>
    </div>
  )
}

const titleStyle: CSSProperties = { fontSize: 28, fontWeight: 700, color: 'var(--text-primary)', margin: 0, letterSpacing: '-0.02em' }
const subtleText: CSSProperties = { fontSize: 12, color: 'var(--text-tertiary)', margin: '4px 0 0' }
const sectionCard: CSSProperties = { background: 'var(--layer-2)', border: '1px solid var(--layer-border)', borderRadius: 12, padding: 24 }
const sectionTitle: CSSProperties = { fontSize: 16, fontWeight: 600, color: 'var(--text-primary)', margin: 0 }
const labelStyle: CSSProperties = { fontSize: 12, color: 'var(--text-secondary)', display: 'flex', flexDirection: 'column', gap: 6 }
const inputStyle: CSSProperties = { background: 'var(--layer-1)', border: '1px solid var(--layer-border)', borderRadius: 8, color: 'var(--text-primary)', padding: '9px 12px', fontSize: 12, outline: 'none' }
const primaryBtn: CSSProperties = { display: 'inline-flex', alignItems: 'center', gap: 8, background: 'var(--prove)', border: 'none', borderRadius: 8, padding: '10px 18px', fontSize: 12, fontWeight: 700, color: '#000', cursor: 'pointer' }
const ghostBtn: CSSProperties = { display: 'flex', alignItems: 'center', gap: 6, background: 'none', border: '1px solid var(--layer-border)', borderRadius: 8, color: 'var(--text-secondary)', padding: '7px 14px', fontSize: 12, cursor: 'pointer' }
