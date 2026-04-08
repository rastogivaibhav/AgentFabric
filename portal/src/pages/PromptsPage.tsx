import { useMemo, useState, type CSSProperties } from 'react'
import { Link } from 'react-router-dom'
import { hasRole, useAuth } from '../hooks/auth'
import { type PromptVersion, usePrompts, useUpsertPromptVersion } from '../hooks/api'

const emptyPrompt: PromptVersion = {
  prompt_id: '',
  environment: 'development',
  release_tag: '',
  content: '',
  config: {},
  description: '',
}

export default function PromptsPage() {
  const { user } = useAuth()
  const isAdmin = hasRole(user, ['admin'])
  const { data, isLoading, error } = usePrompts()
  const upsert = useUpsertPromptVersion()
  const [form, setForm] = useState<PromptVersion>(emptyPrompt)

  const prompts = useMemo(() => data?.items ?? [], [data])

  if (!isAdmin) {
    return (
      <div style={{ padding: 32 }}>
        <div style={panelStyle}>
          <h1 style={titleStyle}>Prompts</h1>
          <p style={subtleText}>This page is restricted to administrators.</p>
        </div>
      </div>
    )
  }

  const save = () => {
    upsert.mutate(
      {
        ...form,
        prompt_id: form.prompt_id.trim(),
        environment: form.environment.trim().toLowerCase() || 'development',
        release_tag: form.release_tag?.trim(),
        content: form.content,
        description: form.description?.trim(),
        config: form.config ?? {},
      },
      { onSuccess: () => setForm(emptyPrompt) },
    )
  }

  return (
    <div style={{ padding: 32 }}>
      <div style={{ marginBottom: 24 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 8, color: 'var(--ship)', fontWeight: 600, fontSize: 10, letterSpacing: '0.1em' }}>
          <span style={{ width: 8, height: 8, borderRadius: '50%', background: 'var(--ship)' }} />
          SHIP
        </div>
        <h1 style={titleStyle}>Prompt Registry</h1>
        <p style={subtleText}>
          Version prompt content, annotate releases, and connect trace spans back to the exact prompt revision that was active.
        </p>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'minmax(360px, 420px) 1fr', gap: 16 }}>
        <div style={panelStyle}>
          <div style={sectionLabel}>{form.id ? 'EDIT VERSION' : 'NEW VERSION'}</div>
          <div style={{ display: 'grid', gap: 12 }}>
            <label style={labelStyle}>
              Prompt ID
              <input value={form.prompt_id} onChange={e => setForm(f => ({ ...f, prompt_id: e.target.value }))} style={inputStyle} placeholder="support-bot.system" />
            </label>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
              <label style={labelStyle}>
                Environment
                <input value={form.environment} onChange={e => setForm(f => ({ ...f, environment: e.target.value }))} style={inputStyle} placeholder="development" />
              </label>
              <label style={labelStyle}>
                Release Tag
                <input value={form.release_tag ?? ''} onChange={e => setForm(f => ({ ...f, release_tag: e.target.value }))} style={inputStyle} placeholder="2026.03-preview" />
              </label>
            </div>
            <label style={labelStyle}>
              Description
              <input value={form.description ?? ''} onChange={e => setForm(f => ({ ...f, description: e.target.value }))} style={inputStyle} placeholder="assistant system prompt for support escalation" />
            </label>
            <label style={labelStyle}>
              Content
              <textarea value={form.content} onChange={e => setForm(f => ({ ...f, content: e.target.value }))} style={textareaStyle} rows={10} />
            </label>
            <label style={labelStyle}>
              Config JSON
              <textarea
                value={JSON.stringify(form.config ?? {}, null, 2)}
                onChange={e => {
                  try {
                    const parsed = JSON.parse(e.target.value || '{}') as Record<string, string>
                    setForm(f => ({ ...f, config: parsed }))
                  } catch {}
                }}
                style={textareaStyle}
                rows={5}
                placeholder='{"temperature":"0.2","channel":"support"}'
              />
            </label>
          </div>
          <div style={{ display: 'flex', gap: 8, marginTop: 16 }}>
            <button style={primaryBtn} onClick={save} disabled={upsert.isPending || !form.prompt_id.trim() || !form.content.trim()}>
              {upsert.isPending ? 'Saving...' : form.version ? 'Save Version' : 'Create Version'}
            </button>
            <button style={secondaryBtn} onClick={() => setForm(emptyPrompt)}>Clear</button>
          </div>
          {upsert.isError && <div style={errorStyle}>Failed to save prompt version.</div>}
        </div>

        <div style={panelStyle}>
          <div style={sectionLabel}>REGISTERED PROMPTS</div>
          {isLoading ? (
            <div style={{ color: '#475569', fontSize: 12 }}>Loading...</div>
          ) : error ? (
            <div style={errorStyle}>Failed to load prompts.</div>
          ) : prompts.length === 0 ? (
            <div style={{ color: '#475569', fontSize: 12 }}>No prompts recorded yet.</div>
          ) : (
            <div style={{ display: 'grid', gap: 10 }}>
              {prompts.map(prompt => (
                <div key={`${prompt.prompt_id}-${prompt.version}`} style={cardStyle}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12 }}>
                    <div>
                      <div style={{ color: '#F0F9FF', fontWeight: 700, fontSize: 13 }}>{prompt.prompt_id}</div>
                      <div style={{ color: '#64748B', fontSize: 11, marginTop: 4 }}>
                        v{prompt.version} · {prompt.environment} · {prompt.release_tag || 'unreleased'}
                      </div>
                    </div>
                    <div style={{ display: 'flex', gap: 8, alignItems: 'flex-start' }}>
                      {prompt.promoted && <span style={badgeStyle('#10B98120', '#10B981')}>LIVE</span>}
                      {prompt.is_latest && <span style={badgeStyle('#3B82F620', '#60A5FA')}>LATEST</span>}
                      {prompt.current_release?.status && <span style={badgeStyle('#F59E0B20', '#FBBF24')}>{prompt.current_release.status.toUpperCase()}</span>}
                    </div>
                  </div>
                  {prompt.description && <div style={{ color: '#94A3B8', fontSize: 11, marginTop: 8 }}>{prompt.description}</div>}
                  {prompt.current_release && (
                    <div style={releaseMetaStyle}>
                      <div style={{ color: '#E2E8F0', fontSize: 11 }}>
                        active release {prompt.current_release.release_tag} · {prompt.current_release.eval_summary?.eval_count ?? 0} evals
                      </div>
                      <div style={{ color: evalColor(prompt.current_release.eval_summary?.average_score ?? 0), fontSize: 11, marginTop: 4 }}>
                        avg {formatScore(prompt.current_release.eval_summary?.average_score)} · {prompt.current_release.eval_summary?.risk_level ?? 'unknown'} risk
                      </div>
                      {prompt.current_release.regression_summary && (
                        <div style={{ color: prompt.current_release.regression_summary.overall_delta >= 0 ? '#10B981' : '#FCA5A5', fontSize: 10, marginTop: 4 }}>
                          regression {prompt.current_release.regression_summary.overall_delta >= 0 ? '+' : ''}{prompt.current_release.regression_summary.overall_delta.toFixed(2)} vs {prompt.current_release.regression_summary.baseline_tag}
                        </div>
                      )}
                    </div>
                  )}
                  <div style={{ color: '#CBD5E1', fontSize: 11, marginTop: 8, whiteSpace: 'pre-wrap' }}>
                    {prompt.content.length > 220 ? `${prompt.content.slice(0, 220)}...` : prompt.content}
                  </div>
                  <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, marginTop: 12 }}>
                    <button style={secondaryBtnSmall} onClick={() => setForm(prompt)}>Edit</button>
                    <Link style={linkBtnStyle} to={`/prompts/${encodeURIComponent(prompt.prompt_id)}`}>
                      Open Release View
                    </Link>
                  </div>
                </div>
              ))}
            </div>
          )}
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
const textareaStyle: CSSProperties = { ...inputStyle, minHeight: 100, fontFamily: 'inherit', resize: 'vertical' }
const primaryBtn: CSSProperties = { background: '#2563EB', color: '#F8FAFC', border: 'none', borderRadius: 8, padding: '10px 14px', fontSize: 12, cursor: 'pointer' }
const secondaryBtn: CSSProperties = { background: '#0B1627', color: '#CBD5E1', border: '1px solid #1E293B', borderRadius: 8, padding: '10px 14px', fontSize: 12, cursor: 'pointer' }
const secondaryBtnSmall: CSSProperties = { ...secondaryBtn, padding: '6px 10px', fontSize: 11 }
const linkBtnStyle: CSSProperties = { color: '#60A5FA', fontSize: 11, textDecoration: 'none', alignSelf: 'center' }
const cardStyle: CSSProperties = { border: '1px solid #0F1F35', borderRadius: 8, background: '#071525', padding: 14 }
const errorStyle: CSSProperties = { color: '#FCA5A5', fontSize: 12, marginTop: 12 }
const releaseMetaStyle: CSSProperties = { marginTop: 8, padding: '8px 10px', borderRadius: 8, border: '1px solid #10243D', background: '#081221' }

function badgeStyle(background: string, color: string): CSSProperties {
  return {
    display: 'inline-block',
    padding: '2px 8px',
    borderRadius: 999,
    background,
    color,
    fontSize: 10,
    fontWeight: 700,
    letterSpacing: '0.08em',
  }
}

function evalColor(score: number): string {
  if (score >= 85) return '#10B981'
  if (score >= 65) return '#F59E0B'
  return '#FCA5A5'
}

function formatScore(score?: number): string {
  if (score === undefined) return 'n/a'
  return score.toFixed(1)
}
