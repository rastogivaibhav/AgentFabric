import { useMemo, useState, type CSSProperties } from 'react'
import { Link, useParams } from 'react-router-dom'
import { hasRole, useAuth } from '../hooks/auth'
import { usePromotePromptRelease, usePrompts } from '../hooks/api'

export default function PromptReleasePage() {
  const { promptId = '' } = useParams()
  const decodedPromptID = decodeURIComponent(promptId)
  const { user } = useAuth()
  const isAdmin = hasRole(user, ['admin'])
  const { data, isLoading, error } = usePrompts()
  const promote = usePromotePromptRelease()
  const [form, setForm] = useState({
    environment: 'development',
    version: 1,
    release_tag: '',
    status: 'active',
    notes: '',
    promotion_reason: '',
  })

  const promptVersions = useMemo(
    () => (data?.items ?? []).filter(item => item.prompt_id === decodedPromptID),
    [data, decodedPromptID],
  )
  const promptReleases = useMemo(
    () => (data?.releases ?? []).filter(item => item.prompt_id === decodedPromptID),
    [data, decodedPromptID],
  )

  if (!isAdmin) {
    return (
      <div style={{ padding: 32 }}>
        <div style={panelStyle}>
          <h1 style={titleStyle}>Prompt Releases</h1>
          <p style={subtleText}>This page is restricted to administrators.</p>
        </div>
      </div>
    )
  }

  const selectedVersion = promptVersions.find(item => item.version === form.version)

  const submit = () => {
    promote.mutate({
      prompt_id: decodedPromptID,
      environment: form.environment.trim().toLowerCase() || 'development',
      version: form.version,
      release_tag: form.release_tag.trim(),
      status: form.status.trim().toLowerCase() || 'active',
      notes: form.notes.trim(),
      promotion_reason: form.promotion_reason.trim(),
    })
  }

  return (
    <div style={{ padding: 32 }}>
      <div style={{ marginBottom: 24, display: 'flex', justifyContent: 'space-between', gap: 12 }}>
        <div>
          <h1 style={titleStyle}>Prompt Release View</h1>
          <p style={subtleText}>
            Promote a concrete prompt version into an environment release and use the trace-linked metadata for rollback and audit.
          </p>
        </div>
        <Link to="/prompts" style={backLinkStyle}>Back to Prompt Registry</Link>
      </div>

      <div style={panelStyle}>
        <div style={sectionLabel}>PROMPT</div>
        <div style={{ color: '#F0F9FF', fontSize: 18, fontWeight: 700 }}>{decodedPromptID || 'unknown prompt'}</div>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'minmax(360px, 420px) 1fr', gap: 16, marginTop: 16 }}>
        <div style={panelStyle}>
          <div style={sectionLabel}>PROMOTE RELEASE</div>
          <div style={{ display: 'grid', gap: 12 }}>
            <label style={labelStyle}>
              Environment
              <input value={form.environment} onChange={e => setForm(f => ({ ...f, environment: e.target.value }))} style={inputStyle} />
            </label>
            <label style={labelStyle}>
              Version
              <select value={form.version} onChange={e => setForm(f => ({ ...f, version: Number(e.target.value) }))} style={inputStyle}>
                {promptVersions.map(version => (
                  <option key={version.id ?? version.version} value={version.version}>
                    v{version.version} | {version.environment}
                  </option>
                ))}
              </select>
            </label>
            <label style={labelStyle}>
              Release Tag
              <input value={form.release_tag} onChange={e => setForm(f => ({ ...f, release_tag: e.target.value }))} style={inputStyle} placeholder="2026.03-prod.1" />
            </label>
            <label style={labelStyle}>
              Status
              <select value={form.status} onChange={e => setForm(f => ({ ...f, status: e.target.value }))} style={inputStyle}>
                <option value="active">active</option>
                <option value="candidate">candidate</option>
                <option value="archived">archived</option>
              </select>
            </label>
            <label style={labelStyle}>
              Notes
              <textarea value={form.notes} onChange={e => setForm(f => ({ ...f, notes: e.target.value }))} style={textareaStyle} rows={4} placeholder="promote validated prompt after release-candidate run" />
            </label>
            <label style={labelStyle}>
              Promotion Reason
              <textarea value={form.promotion_reason} onChange={e => setForm(f => ({ ...f, promotion_reason: e.target.value }))} style={textareaStyle} rows={3} placeholder="improve escalation quality and reduce hallucinations" />
            </label>
          </div>
          <div style={{ marginTop: 16, display: 'flex', gap: 8 }}>
            <button
              style={primaryBtn}
              onClick={submit}
              disabled={promote.isPending || !decodedPromptID || !form.release_tag.trim() || !selectedVersion}
            >
              {promote.isPending ? 'Promoting...' : 'Promote Release'}
            </button>
          </div>
          {promote.isError && <div style={errorStyle}>Failed to promote prompt release.</div>}
          {promote.data && (
            <div style={{ marginTop: 12, color: '#94A3B8', fontSize: 11 }}>
              Promoted {promote.data.prompt_id} v{promote.data.version} to {promote.data.environment} as {promote.data.release_tag}.
            </div>
          )}
        </div>

        <div style={panelStyle}>
          <div style={sectionLabel}>VERSION DETAIL</div>
          {isLoading ? (
            <div style={{ color: '#475569', fontSize: 12 }}>Loading...</div>
          ) : error ? (
            <div style={errorStyle}>Failed to load prompt data.</div>
          ) : !selectedVersion ? (
            <div style={{ color: '#475569', fontSize: 12 }}>No version selected for this prompt.</div>
          ) : (
            <>
              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, minmax(0, 1fr))', gap: 10 }}>
                {[
                  ['Version', `v${selectedVersion.version}`],
                  ['Environment', selectedVersion.environment],
                  ['Current Release', selectedVersion.current_release?.release_tag ?? 'none'],
                ].map(([label, value]) => (
                  <div key={label} style={statCardStyle}>
                    <div style={{ fontSize: 9, color: '#334155', letterSpacing: '0.08em' }}>{label}</div>
                    <div style={{ fontSize: 12, color: '#E2E8F0', marginTop: 4 }}>{value}</div>
                  </div>
                ))}
              </div>
              {selectedVersion.description && (
                <div style={{ marginTop: 12, color: '#94A3B8', fontSize: 11 }}>{selectedVersion.description}</div>
              )}
              {selectedVersion.current_release && (
                <div style={healthPanelStyle}>
                  <div style={{ color: '#F8FAFC', fontSize: 12, fontWeight: 600 }}>
                    {(selectedVersion.current_release.status ?? 'active')} release health
                  </div>
                  <div style={{ color: scoreColor(selectedVersion.current_release.eval_summary?.average_score ?? 0), fontSize: 12, marginTop: 6 }}>
                    avg {formatScore(selectedVersion.current_release.eval_summary?.average_score)} | latest {formatScore(selectedVersion.current_release.eval_summary?.latest_score)}
                  </div>
                  <div style={{ color: '#94A3B8', fontSize: 11, marginTop: 4 }}>
                    {selectedVersion.current_release.eval_summary?.eval_count ?? 0} evals | {selectedVersion.current_release.eval_summary?.risk_level ?? 'unknown'} risk
                  </div>
                  {selectedVersion.current_release.regression_summary && (
                    <div style={{ color: selectedVersion.current_release.regression_summary.overall_delta >= 0 ? '#10B981' : '#FCA5A5', fontSize: 11, marginTop: 6 }}>
                      {selectedVersion.current_release.regression_summary.summary}
                    </div>
                  )}
                </div>
              )}
              <div style={{ marginTop: 12, color: '#CBD5E1', fontSize: 11, whiteSpace: 'pre-wrap', maxHeight: 260, overflow: 'auto' }}>
                {selectedVersion.content}
              </div>
            </>
          )}
        </div>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16, marginTop: 16 }}>
        <div style={panelStyle}>
          <div style={sectionLabel}>VERSIONS</div>
          <div style={{ display: 'grid', gap: 10 }}>
            {promptVersions.map(version => (
              <button
                key={version.id ?? version.version}
                style={{
                  ...versionBtnStyle,
                  borderColor: version.version === form.version ? '#2563EB' : '#0F1F35',
                }}
                onClick={() => setForm(f => ({ ...f, version: version.version ?? 1, environment: version.environment }))}
              >
                <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12 }}>
                  <span>v{version.version}</span>
                  <span style={{ color: '#64748B' }}>{version.environment}</span>
                </div>
                <div style={{ marginTop: 4, fontSize: 10, color: '#94A3B8' }}>
                  {version.current_release?.release_tag ?? version.release_tag ?? 'unreleased'}
                </div>
              </button>
            ))}
          </div>
        </div>

        <div style={panelStyle}>
          <div style={sectionLabel}>RELEASE HISTORY</div>
          <div style={{ display: 'grid', gap: 10 }}>
            {promptReleases.length === 0 ? (
              <div style={{ color: '#475569', fontSize: 12 }}>No releases yet.</div>
            ) : promptReleases.map(release => (
              <div key={`${release.environment}-${release.release_tag}-${release.created_at}`} style={releaseCardStyle}>
                <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12 }}>
                  <div style={{ color: '#E2E8F0', fontSize: 12, fontWeight: 600 }}>{release.environment}</div>
                  <div style={{ color: '#60A5FA', fontSize: 11 }}>{release.release_tag}</div>
                </div>
                <div style={{ color: '#94A3B8', fontSize: 11, marginTop: 4 }}>
                  v{release.version} | {release.promoted_by || 'system'} | {release.status || 'active'}
                </div>
                <div style={{ color: scoreColor(release.eval_summary?.average_score ?? 0), fontSize: 11, marginTop: 6 }}>
                  avg {formatScore(release.eval_summary?.average_score)} | {release.eval_summary?.eval_count ?? 0} evals
                </div>
                {release.promotion_reason && <div style={{ color: '#CBD5E1', fontSize: 10, marginTop: 6 }}>{release.promotion_reason}</div>}
                {release.notes && <div style={{ color: '#64748B', fontSize: 10, marginTop: 6 }}>{release.notes}</div>}
                {release.regression_summary && <div style={{ color: release.regression_summary.overall_delta >= 0 ? '#10B981' : '#FCA5A5', fontSize: 10, marginTop: 6 }}>{release.regression_summary.summary}</div>}
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}

const titleStyle: CSSProperties = { fontSize: 22, fontWeight: 700, color: '#F0F9FF', margin: 0 }
const subtleText: CSSProperties = { color: '#475569', fontSize: 12, marginTop: 6 }
const panelStyle: CSSProperties = { background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 10, padding: 20 }
const sectionLabel: CSSProperties = { fontSize: 10, color: '#334155', letterSpacing: '0.12em', marginBottom: 12 }
const labelStyle: CSSProperties = { display: 'grid', gap: 6, color: '#94A3B8', fontSize: 11 }
const inputStyle: CSSProperties = { background: '#071525', border: '1px solid #0F1F35', borderRadius: 8, color: '#E2E8F0', padding: '10px 12px', fontSize: 12 }
const textareaStyle: CSSProperties = { ...inputStyle, minHeight: 90, fontFamily: 'inherit', resize: 'vertical' }
const primaryBtn: CSSProperties = { background: '#2563EB', color: '#F8FAFC', border: 'none', borderRadius: 8, padding: '10px 14px', fontSize: 12, cursor: 'pointer' }
const errorStyle: CSSProperties = { color: '#FCA5A5', fontSize: 12, marginTop: 12 }
const backLinkStyle: CSSProperties = { color: '#60A5FA', fontSize: 12, textDecoration: 'none', alignSelf: 'flex-start' }
const statCardStyle: CSSProperties = { border: '1px solid #0F1F35', borderRadius: 8, background: '#071525', padding: 10 }
const versionBtnStyle: CSSProperties = { background: '#071525', border: '1px solid #0F1F35', borderRadius: 8, color: '#E2E8F0', padding: 12, textAlign: 'left', cursor: 'pointer' }
const releaseCardStyle: CSSProperties = { border: '1px solid #0F1F35', borderRadius: 8, background: '#071525', padding: 12 }
const healthPanelStyle: CSSProperties = { marginTop: 12, padding: 12, borderRadius: 8, border: '1px solid #10243D', background: '#081221' }

function scoreColor(score: number): string {
  if (score >= 85) return '#10B981'
  if (score >= 65) return '#F59E0B'
  return '#FCA5A5'
}

function formatScore(score?: number): string {
  if (score === undefined) return 'n/a'
  return score.toFixed(1)
}
