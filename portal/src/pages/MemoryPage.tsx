import { useState, type CSSProperties } from 'react'
import {
  useControlHistory,
  useEvidenceBundles,
  useCreateEvidenceBundle,
  type ControlHistoryEntry,
  type EvidenceBundle,
  type EvidenceBundleRequest,
} from '../hooks/api'

const CATEGORY_COLORS: Record<string, string> = {
  policies:        '#F59E0B',
  rollouts:        '#3B82F6',
  pricing:         '#10B981',
  prompts:         '#8B5CF6',
  evals:           '#4ECDC4',
  recommendations: '#60A5FA',
}

function categoryTag(cat: string) {
  const c = CATEGORY_COLORS[cat] ?? '#64748B'
  return (
    <span style={{ padding: '2px 8px', borderRadius: 4, fontSize: 10, background: `${c}20`, color: c, border: `1px solid ${c}40` }}>
      {cat}
    </span>
  )
}

function actionTag(action: string, outcome: string) {
  const success = outcome === 'success'
  return (
    <span style={{ fontSize: 10, color: success ? '#10B981' : '#EF4444' }}>
      {action.toUpperCase()}
    </span>
  )
}

const BLANK_BUNDLE: EvidenceBundleRequest = { name: '', scope: 'incident' }

export default function MemoryPage() {
  const [tab, setTab] = useState<'history' | 'bundles'>('history')
  const [filterCategory, setFilterCategory] = useState('')
  const [historyPage, setHistoryPage] = useState(0)
  const LIMIT = 30

  const { data: historyData, isLoading: historyLoading } = useControlHistory(
    LIMIT, historyPage * LIMIT, filterCategory
  )
  const { data: bundlesData, isLoading: bundlesLoading } = useEvidenceBundles(25)
  const createBundle = useCreateEvidenceBundle()

  const [showNewBundle, setShowNewBundle] = useState(false)
  const [bundleForm, setBundleForm] = useState<EvidenceBundleRequest>(BLANK_BUNDLE)
  const [bundleError, setBundleError] = useState('')
  const [selectedBundle, setSelectedBundle] = useState<EvidenceBundle | null>(null)
  const [selectedEntry, setSelectedEntry] = useState<ControlHistoryEntry | null>(null)

  const historyItems = historyData?.items ?? []
  const bundles = bundlesData?.items ?? []

  async function handleCreateBundle() {
    if (!bundleForm.name.trim()) { setBundleError('Bundle name is required.'); return }
    try {
      await createBundle.mutateAsync(bundleForm)
      setShowNewBundle(false)
      setBundleForm(BLANK_BUNDLE)
      setBundleError('')
    } catch (e: unknown) {
      setBundleError(e instanceof Error ? e.message : 'Failed to create bundle')
    }
  }

  return (
    <div style={{ padding: 32 }}>
      {/* Header */}
      <div style={{ marginBottom: 24 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 8, color: 'var(--prove)', fontWeight: 600, fontSize: 10, letterSpacing: '0.1em' }}>
          <span style={{ width: 8, height: 8, borderRadius: '50%', background: 'var(--prove)' }} />
          PROVE
        </div>
        <h1 style={titleStyle}>Enterprise Memory</h1>
        <p style={subtleText}>Immutable control-plane change history and compliance evidence bundles — the complete audit trail of who changed what, when, and why.</p>
      </div>

      {/* Tab toggle */}
      <div style={{ display: 'flex', gap: 0, marginBottom: 24, border: '1px solid #0F1F35', borderRadius: 10, overflow: 'hidden', width: 'fit-content' }}>
        {(['history', 'bundles'] as const).map(t => (
          <button
            key={t}
            id={`memory-tab-${t}`}
            style={{
              padding: '10px 20px', fontSize: 12, fontWeight: 600, cursor: 'pointer', border: 'none',
              background: tab === t ? '#1E3A5F' : 'transparent',
              color: tab === t ? '#60A5FA' : '#475569',
              letterSpacing: '0.05em',
            }}
            onClick={() => setTab(t)}
          >
            {t === 'history' ? 'CONTROL HISTORY' : 'EVIDENCE BUNDLES'}
          </button>
        ))}
      </div>

      {/* ── HISTORY TAB ─────────────────────────────────────────── */}
      {tab === 'history' && (
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 400px', gap: 16 }}>
          <div style={panelStyle}>
            <div style={{ display: 'flex', gap: 12, marginBottom: 16, alignItems: 'center' }}>
              <div style={{ ...sectionLabel, marginBottom: 0, flex: 1 }}>CHANGE TIMELINE</div>
              <label style={labelStyle}>
                Category
                <select id="history-filter-category" style={selectStyle} value={filterCategory} onChange={e => { setFilterCategory(e.target.value); setHistoryPage(0) }}>
                  <option value="">All</option>
                  {Object.keys(CATEGORY_COLORS).map(c => <option key={c} value={c}>{c}</option>)}
                </select>
              </label>
            </div>

            {historyLoading && <div style={subtleText}>Loading history…</div>}
            {!historyLoading && historyItems.length === 0 && (
              <div style={{ ...subtleText, textAlign: 'center', padding: 40 }}>
                No control history yet. Changes to policies, rollouts, pricing, and prompts are recorded here automatically.
              </div>
            )}

            <div style={{ display: 'grid', gap: 6 }}>
              {historyItems.map(entry => (
                <button
                  key={entry.id}
                  id={`history-entry-${entry.id}`}
                  style={{
                    ...rowBtn,
                    borderColor: selectedEntry?.id === entry.id ? '#2563EB' : '#0F1F35',
                    background: selectedEntry?.id === entry.id ? '#0A1B33' : 'transparent',
                  }}
                  onClick={() => setSelectedEntry(entry)}
                >
                  <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                    <div style={{ width: 6, height: 6, borderRadius: '50%', background: entry.outcome === 'success' ? '#10B981' : '#EF4444', flexShrink: 0 }} />
                    <div style={{ flex: 1, textAlign: 'left' }}>
                      <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
                        {categoryTag(entry.category)}
                        {actionTag(entry.action, entry.outcome)}
                        {entry.target_id && <span style={{ fontSize: 10, color: '#64748B', fontFamily: 'monospace' }}>{entry.target_id}</span>}
                      </div>
                      <div style={{ fontSize: 10, color: '#334155', marginTop: 4 }}>
                        {entry.actor ?? 'system'} · {new Date(entry.created_at).toLocaleString()}
                        {entry.reason ? ` · "${entry.reason}"` : ''}
                      </div>
                    </div>
                  </div>
                </button>
              ))}
            </div>

            {/* Pagination */}
            {(historyData?.has_more || historyPage > 0) && (
              <div style={{ display: 'flex', gap: 10, marginTop: 16, justifyContent: 'center' }}>
                <button style={ghostBtn} onClick={() => setHistoryPage(p => Math.max(0, p - 1))} disabled={historyPage === 0}>← Prev</button>
                <span style={{ fontSize: 12, color: '#475569', padding: '7px 0' }}>Page {historyPage + 1}</span>
                <button style={ghostBtn} onClick={() => setHistoryPage(p => p + 1)} disabled={!historyData?.has_more}>Next →</button>
              </div>
            )}
          </div>

          {/* Entry detail */}
          <div style={panelStyle}>
            <div style={sectionLabel}>ENTRY DETAIL</div>
            {!selectedEntry ? (
              <div style={subtleText}>Select an entry to view before/after state and verifiable hash.</div>
            ) : (
              <div style={{ display: 'grid', gap: 12 }}>
                <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                  {categoryTag(selectedEntry.category)}
                  {actionTag(selectedEntry.action, selectedEntry.outcome)}
                </div>
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 8 }}>
                  <DetailStat label="Actor" value={selectedEntry.actor ?? 'system'} />
                  <DetailStat label="Outcome" value={selectedEntry.outcome} />
                  <DetailStat label="Target" value={`${selectedEntry.target_type} / ${selectedEntry.target_id ?? '—'}`} />
                  <DetailStat label="When" value={new Date(selectedEntry.created_at).toLocaleString()} />
                </div>
                {selectedEntry.reason && (
                  <div style={{ border: '1px solid #0F1F35', borderRadius: 8, padding: 12, background: '#071525' }}>
                    <div style={{ fontSize: 9, color: '#475569', marginBottom: 4 }}>REASON</div>
                    <div style={{ fontSize: 12, color: '#CBD5E1' }}>{selectedEntry.reason}</div>
                  </div>
                )}
                {selectedEntry.before_state && (
                  <div>
                    <div style={{ fontSize: 9, color: '#475569', marginBottom: 4 }}>BEFORE</div>
                    <pre style={codeStyle}>{formatJSON(selectedEntry.before_state)}</pre>
                  </div>
                )}
                {selectedEntry.after_state && (
                  <div>
                    <div style={{ fontSize: 9, color: '#475569', marginBottom: 4 }}>AFTER</div>
                    <pre style={codeStyle}>{formatJSON(selectedEntry.after_state)}</pre>
                  </div>
                )}
                {selectedEntry.entry_hash && (
                  <div style={{ fontSize: 9, color: '#334155', fontFamily: 'monospace', wordBreak: 'break-all' }}>
                    Hash: {selectedEntry.entry_hash}
                  </div>
                )}
              </div>
            )}
          </div>
        </div>
      )}

      {/* ── BUNDLES TAB ─────────────────────────────────────────── */}
      {tab === 'bundles' && (
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 420px', gap: 16 }}>
          <div style={panelStyle}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
              <div style={sectionLabel}>EVIDENCE BUNDLES ({bundles.length})</div>
              <button id="create-bundle-btn" style={primaryBtn} onClick={() => { setShowNewBundle(true); setBundleError('') }}>+ New Bundle</button>
            </div>

            {bundlesLoading && <div style={subtleText}>Loading bundles…</div>}
            {!bundlesLoading && bundles.length === 0 && (
              <div style={{ ...subtleText, textAlign: 'center', padding: 40 }}>
                No evidence bundles yet. Create one to collect control history, decisions, and eval results for compliance or incident review.
              </div>
            )}

            <div style={{ display: 'grid', gap: 8 }}>
              {bundles.map(bundle => (
                <button
                  key={bundle.id}
                  id={`bundle-${bundle.id}`}
                  style={{
                    ...rowBtn,
                    borderColor: selectedBundle?.id === bundle.id ? '#2563EB' : '#0F1F35',
                    background: selectedBundle?.id === bundle.id ? '#0A1B33' : 'transparent',
                  }}
                  onClick={() => setSelectedBundle(bundle)}
                >
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
                    <div style={{ textAlign: 'left' }}>
                      <div style={{ color: '#E2E8F0', fontSize: 12, fontWeight: 600 }}>{bundle.name}</div>
                      <div style={{ fontSize: 10, color: '#475569', marginTop: 3 }}>
                        {bundle.scope} · {bundle.item_count} items · {new Date(bundle.created_at).toLocaleDateString()}
                      </div>
                    </div>
                    <span style={{ fontSize: 10, color: '#10B981', background: '#10B98115', border: '1px solid #10B98130', borderRadius: 4, padding: '2px 8px' }}>
                      {bundle.status}
                    </span>
                  </div>
                  {bundle.summary && bundle.summary.length > 0 && (
                    <div style={{ fontSize: 10, color: '#64748B', marginTop: 6, textAlign: 'left' }}>
                      {bundle.summary[0]}
                    </div>
                  )}
                </button>
              ))}
            </div>
          </div>

          {/* Bundle detail */}
          <div style={panelStyle}>
            <div style={sectionLabel}>BUNDLE DETAIL</div>
            {!selectedBundle ? (
              <div style={subtleText}>Select a bundle to inspect its items and summary.</div>
            ) : (
              <div style={{ display: 'grid', gap: 14 }}>
                <div>
                  <div style={{ color: '#F0F9FF', fontSize: 16, fontWeight: 700 }}>{selectedBundle.name}</div>
                  <div style={{ fontSize: 11, color: '#475569', marginTop: 4 }}>
                    Scope: {selectedBundle.scope} · {selectedBundle.item_count} items · by {selectedBundle.created_by ?? 'system'}
                  </div>
                </div>
                {selectedBundle.summary && selectedBundle.summary.length > 0 && (
                  <div>
                    <div style={sectionLabel}>SUMMARY</div>
                    <div style={{ display: 'grid', gap: 6 }}>
                      {selectedBundle.summary.map((s, i) => (
                        <div key={i} style={{ fontSize: 12, color: '#CBD5E1', padding: '8px 12px', background: '#071525', borderRadius: 8, border: '1px solid #0F1F35' }}>{s}</div>
                      ))}
                    </div>
                  </div>
                )}
                {selectedBundle.items && selectedBundle.items.length > 0 && (
                  <div>
                    <div style={sectionLabel}>ITEMS ({selectedBundle.items.length})</div>
                    <div style={{ display: 'grid', gap: 6, maxHeight: 320, overflowY: 'auto' }}>
                      {selectedBundle.items.map(item => (
                        <div key={item.id} style={{ border: '1px solid #0F1F35', borderRadius: 8, padding: '10px 12px', background: '#071525' }}>
                          <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8 }}>
                            <div style={{ fontSize: 11, color: '#E2E8F0', fontWeight: 600 }}>{item.item_title}</div>
                            <span style={{ fontSize: 9, color: '#64748B', background: '#0F1F35', padding: '2px 6px', borderRadius: 4 }}>{item.item_type}</span>
                          </div>
                          {item.trace_id && <div style={{ fontSize: 10, color: '#475569', marginTop: 3, fontFamily: 'monospace' }}>{item.trace_id.substring(0, 16)}…</div>}
                        </div>
                      ))}
                    </div>
                  </div>
                )}
              </div>
            )}
          </div>
        </div>
      )}

      {/* New Bundle modal */}
      {showNewBundle && (
        <div style={overlay}>
          <div style={modal}>
            <div style={{ fontSize: 16, fontWeight: 700, color: '#F0F9FF', marginBottom: 20 }}>Create Evidence Bundle</div>
            <div style={{ display: 'grid', gap: 14 }}>
              <label style={labelStyle}>
                Bundle Name *
                <input id="bundle-name" style={inputStyle} value={bundleForm.name}
                  placeholder="Q1 Policy Rollout Review"
                  onChange={e => setBundleForm(p => ({ ...p, name: e.target.value }))} />
              </label>
              <label style={labelStyle}>
                Scope
                <select id="bundle-scope" style={inputStyle} value={bundleForm.scope}
                  onChange={e => setBundleForm(p => ({ ...p, scope: e.target.value }))}>
                  <option value="incident">Incident</option>
                  <option value="release">Release</option>
                  <option value="compliance">Compliance</option>
                  <option value="rollout">Rollout</option>
                </select>
              </label>
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
                <label style={labelStyle}>
                  Trace ID (optional)
                  <input id="bundle-trace" style={inputStyle} value={bundleForm.trace_id ?? ''}
                    placeholder="abc123…"
                    onChange={e => setBundleForm(p => ({ ...p, trace_id: e.target.value }))} />
                </label>
                <label style={labelStyle}>
                  Prompt ID (optional)
                  <input id="bundle-prompt" style={inputStyle} value={bundleForm.prompt_id ?? ''}
                    placeholder="my-prompt"
                    onChange={e => setBundleForm(p => ({ ...p, prompt_id: e.target.value }))} />
                </label>
                <label style={labelStyle}>
                  Release Tag (optional)
                  <input id="bundle-release" style={inputStyle} value={bundleForm.release_tag ?? ''}
                    placeholder="v1.1.0"
                    onChange={e => setBundleForm(p => ({ ...p, release_tag: e.target.value }))} />
                </label>
                <label style={labelStyle}>
                  Environment (optional)
                  <input id="bundle-env" style={inputStyle} value={bundleForm.environment ?? ''}
                    placeholder="production"
                    onChange={e => setBundleForm(p => ({ ...p, environment: e.target.value }))} />
                </label>
              </div>
              <label style={labelStyle}>
                Reason / Notes
                <input id="bundle-reason" style={inputStyle} value={bundleForm.reason ?? ''}
                  placeholder="Post-incident review for…"
                  onChange={e => setBundleForm(p => ({ ...p, reason: e.target.value }))} />
              </label>
            </div>
            {bundleError && <div style={{ color: '#FCA5A5', fontSize: 12, marginTop: 12 }}>{bundleError}</div>}
            <div style={{ display: 'flex', gap: 10, justifyContent: 'flex-end', marginTop: 24 }}>
              <button id="bundle-cancel" style={ghostBtn} onClick={() => setShowNewBundle(false)}>Cancel</button>
              <button id="bundle-save" style={primaryBtn} onClick={handleCreateBundle} disabled={createBundle.isPending}>
                {createBundle.isPending ? 'Creating…' : 'Create Bundle'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

function DetailStat({ label, value }: { label: string; value: string }) {
  return (
    <div style={{ border: '1px solid #0F1F35', borderRadius: 8, padding: '10px 14px', background: '#071525' }}>
      <div style={{ fontSize: 9, color: '#475569', letterSpacing: '0.1em', marginBottom: 4 }}>{label.toUpperCase()}</div>
      <div style={{ fontSize: 11, color: '#E2E8F0', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{value}</div>
    </div>
  )
}

function formatJSON(raw: string): string {
  try { return JSON.stringify(JSON.parse(raw), null, 2) } catch { return raw }
}

const titleStyle: CSSProperties = { fontSize: 22, fontWeight: 700, color: '#F0F9FF', margin: 0 }
const subtleText: CSSProperties = { fontSize: 12, color: '#475569', marginTop: 4 }
const panelStyle: CSSProperties = { background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 10, padding: 24 }
const sectionLabel: CSSProperties = { fontSize: 10, color: '#334155', letterSpacing: '0.12em', marginBottom: 12 }
const labelStyle: CSSProperties = { display: 'grid', gap: 6, fontSize: 11, color: '#94A3B8' }
const selectStyle: CSSProperties = { background: '#060A14', border: '1px solid #0F1F35', borderRadius: 8, color: '#E2E8F0', padding: '8px 12px', fontSize: 12 }
const inputStyle: CSSProperties = { background: '#060A14', border: '1px solid #0F1F35', borderRadius: 8, color: '#E2E8F0', padding: '9px 12px', fontSize: 12, outline: 'none' }
const primaryBtn: CSSProperties = { background: 'linear-gradient(135deg,#2563EB,#3B82F6)', color: '#fff', border: 'none', borderRadius: 8, padding: '10px 18px', fontSize: 12, fontWeight: 600, cursor: 'pointer' }
const ghostBtn: CSSProperties = { background: 'none', border: '1px solid #0F1F35', borderRadius: 8, color: '#64748B', padding: '7px 14px', fontSize: 12, cursor: 'pointer' }
const rowBtn: CSSProperties = { border: '1px solid', borderRadius: 10, padding: 12, cursor: 'pointer', width: '100%', background: 'transparent' }
const codeStyle: CSSProperties = { margin: 0, padding: 10, borderRadius: 8, background: '#020817', color: '#CBD5E1', fontSize: 10, whiteSpace: 'pre-wrap', border: '1px solid #0F1F35', maxHeight: 200, overflowY: 'auto' }
const overlay: CSSProperties = { position: 'fixed', inset: 0, background: 'rgba(4,8,20,0.85)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 1000, backdropFilter: 'blur(4px)' }
const modal: CSSProperties = { background: '#0D1B2A', border: '1px solid #1E3A5F', borderRadius: 14, padding: 28, maxWidth: 580, width: '100%', boxShadow: '0 24px 64px rgba(0,0,0,0.6)' }
