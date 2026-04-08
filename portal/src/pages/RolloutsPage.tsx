import { useState, type CSSProperties } from 'react'
import {
  useRollouts,
  useUpsertRolloutRule,
  useUpdateRolloutStatus,
  usePreviewRollout,
  type RolloutRule,
  type RolloutPreviewRequest,
} from '../hooks/api'

// ─── Helpers ──────────────────────────────────────────────────────────────────

function statusBadge(status?: string) {
  const active = status !== 'paused'
  return (
    <span style={{
      display: 'inline-flex', alignItems: 'center', gap: 5,
      padding: '3px 10px', borderRadius: 999, fontSize: 10, fontWeight: 700,
      letterSpacing: '0.08em',
      background: active ? '#10B98120' : '#47556920',
      color: active ? '#10B981' : '#64748B',
      border: `1px solid ${active ? '#10B98140' : '#47556940'}`,
    }}>
      <span style={{ width: 6, height: 6, borderRadius: '50%', background: active ? '#10B981' : '#64748B', display: 'inline-block' }} />
      {active ? 'ACTIVE' : 'PAUSED'}
    </span>
  )
}

function targetTypeBadge(type: string) {
  const colors: Record<string, string> = {
    model: '#3B82F6',
    prompt_release: '#8B5CF6',
    policy_rule: '#F59E0B',
  }
  const c = colors[type] ?? '#64748B'
  return (
    <span style={{ padding: '2px 8px', borderRadius: 4, fontSize: 10, background: `${c}20`, color: c, border: `1px solid ${c}40` }}>
      {type.replace('_', ' ')}
    </span>
  )
}

function pct(n: number) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
      <div style={{ flex: 1, height: 6, background: '#0F1F35', borderRadius: 999, overflow: 'hidden' }}>
        <div style={{ width: `${n}%`, height: '100%', background: n < 20 ? '#F59E0B' : n < 80 ? '#3B82F6' : '#10B981', borderRadius: 999 }} />
      </div>
      <span style={{ color: '#94A3B8', fontSize: 11, flexShrink: 0 }}>{n}%</span>
    </div>
  )
}

const BLANK_RULE: RolloutRule = {
  name: '',
  target_type: 'model',
  target_id: '',
  percentage: 10,
  environment: '',
  control_model: '',
  candidate_model: '',
  status: 'active',
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function RolloutsPage() {
  const { data, isLoading, error } = useRollouts()
  const upsert = useUpsertRolloutRule()
  const toggleStatus = useUpdateRolloutStatus()
  const previewMut = usePreviewRollout()

  const [showForm, setShowForm] = useState(false)
  const [editRule, setEditRule] = useState<RolloutRule>(BLANK_RULE)
  const [previewReq, setPreviewReq] = useState<RolloutPreviewRequest>({})
  const [formError, setFormError] = useState('')

  const rules = data?.items ?? []

  function openCreate() {
    setEditRule(BLANK_RULE)
    setFormError('')
    setShowForm(true)
  }

  function openEdit(rule: RolloutRule) {
    setEditRule({ ...rule })
    setFormError('')
    setShowForm(true)
  }

  async function handleSave() {
    const name = editRule.name.trim()
    const target = editRule.target_id.trim()
    if (!name) { setFormError('Name is required.'); return }
    if (!target) { setFormError('Target ID is required.'); return }
    if (editRule.percentage < 0 || editRule.percentage > 100) {
      setFormError('Percentage must be between 0 and 100.'); return
    }
    try {
      await upsert.mutateAsync(editRule)
      setShowForm(false)
    } catch (e: unknown) {
      setFormError(e instanceof Error ? e.message : 'Save failed')
    }
  }

  async function handleToggle(rule: RolloutRule) {
    if (!rule.id) return
    const next = rule.status === 'active' ? 'paused' : 'active'
    await toggleStatus.mutateAsync({ id: rule.id, status: next })
  }

  async function handlePreview() {
    await previewMut.mutateAsync(previewReq)
  }

  const preview = previewMut.data

  return (
    <div style={{ padding: 32 }}>
      {/* Header */}
      <div style={{ marginBottom: 24, display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 16 }}>
        <div>
          <h1 style={titleStyle}>Rollouts</h1>
          <p style={subtleText}>Canary and shadow-mode traffic splitting — route a percentage of requests to a candidate model, prompt release, or policy rule.</p>
        </div>
        <button id="create-rollout-btn" style={primaryBtn} onClick={openCreate}>+ New Rule</button>
      </div>

      {/* Metric strip */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, minmax(0, 1fr))', gap: 16, marginBottom: 24 }}>
        <MetricCard label="Total Rules" value={String(rules.length)} />
        <MetricCard label="Active" value={String(rules.filter(r => r.status !== 'paused').length)} color="#10B981" />
        <MetricCard label="Paused" value={String(rules.filter(r => r.status === 'paused').length)} color="#64748B" />
      </div>

      {/* Rule table */}
      <div style={panelStyle}>
        <div style={{ fontSize: 11, color: '#334155', letterSpacing: '0.12em', marginBottom: 16 }}>ROLLOUT RULES</div>

        {isLoading && <div style={subtleText}>Loading rollout rules…</div>}
        {error && <div style={{ color: '#FCA5A5', fontSize: 12 }}>Failed to load rollout rules.</div>}

        {!isLoading && !error && rules.length === 0 && (
          <div style={{ ...subtleText, textAlign: 'center', padding: 40 }}>
            No rollout rules yet. Create one to start splitting traffic.
          </div>
        )}

        {rules.length > 0 && (
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
            <thead>
              <tr>
                {['Name', 'Target Type', 'Target ID', 'Traffic Split', 'Status', 'Requests', ''].map(h => (
                  <th key={h} style={thStyle}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rules.map((rule, i) => (
                <tr key={rule.id ?? rule.name} style={{ background: i % 2 === 0 ? 'transparent' : '#06091430' }}>
                  <td style={tdStyle}>
                    <div style={{ color: '#E2E8F0', fontWeight: 600 }}>{rule.name}</div>
                    {rule.environment && <div style={{ color: '#475569', fontSize: 10, marginTop: 2 }}>{rule.environment}</div>}
                  </td>
                  <td style={tdStyle}>{targetTypeBadge(rule.target_type)}</td>
                  <td style={{ ...tdStyle, fontFamily: 'monospace', color: '#94A3B8', maxWidth: 200, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                    {rule.target_id}
                  </td>
                  <td style={{ ...tdStyle, minWidth: 160 }}>{pct(rule.percentage)}</td>
                  <td style={tdStyle}>{statusBadge(rule.status)}</td>
                  <td style={tdStyle}>
                    {rule.recent_requests != null ? (
                      <div>
                        <div style={{ color: '#94A3B8' }}>{rule.recent_requests.toLocaleString()} req</div>
                        {(rule.recent_error_rate ?? 0) > 0 && (
                          <div style={{ color: '#FCA5A5', fontSize: 10 }}>{((rule.recent_error_rate ?? 0) * 100).toFixed(1)}% err</div>
                        )}
                      </div>
                    ) : <span style={{ color: '#334155' }}>—</span>}
                  </td>
                  <td style={{ ...tdStyle, textAlign: 'right', whiteSpace: 'nowrap' }}>
                    <button id={`edit-rollout-${rule.id}`} style={ghostBtn} onClick={() => openEdit(rule)}>Edit</button>
                    <button
                      id={`toggle-rollout-${rule.id}`}
                      style={{ ...ghostBtn, color: rule.status === 'active' ? '#F59E0B' : '#10B981', marginLeft: 6 }}
                      onClick={() => handleToggle(rule)}
                      disabled={toggleStatus.isPending}
                    >
                      {rule.status === 'active' ? 'Pause' : 'Activate'}
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {/* Preview panel */}
      <div style={{ ...panelStyle, marginTop: 24 }}>
        <div style={{ fontSize: 11, color: '#334155', letterSpacing: '0.12em', marginBottom: 16 }}>ASSIGNMENT PREVIEW</div>
        <p style={{ ...subtleText, marginBottom: 16 }}>Simulate which rollout rule would be assigned to a given request context.</p>

        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 12, marginBottom: 16 }}>
          {(['provider', 'model', 'environment'] as (keyof RolloutPreviewRequest)[]).map(field => (
            <label key={field} style={labelStyle}>
              {field.charAt(0).toUpperCase() + field.slice(1)}
              <input
                id={`preview-${field}`}
                style={inputStyle}
                value={(previewReq[field] as string) ?? ''}
                placeholder={field === 'provider' ? 'openai' : field === 'model' ? 'gpt-4o' : 'production'}
                onChange={e => setPreviewReq(prev => ({ ...prev, [field]: e.target.value }))}
              />
            </label>
          ))}
        </div>

        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(2, 1fr)', gap: 12, marginBottom: 16 }}>
          <label style={labelStyle}>
            Prompt ID
            <input id="preview-prompt_id" style={inputStyle} value={previewReq.prompt_id ?? ''} placeholder="my-prompt"
              onChange={e => setPreviewReq(prev => ({ ...prev, prompt_id: e.target.value }))} />
          </label>
          <label style={labelStyle}>
            Assignment Key (user/session ID)
            <input id="preview-assignment_key" style={inputStyle} value={previewReq.assignment_key ?? ''} placeholder="user-abc123"
              onChange={e => setPreviewReq(prev => ({ ...prev, assignment_key: e.target.value }))} />
          </label>
        </div>

        <button id="run-rollout-preview" style={primaryBtn} onClick={handlePreview} disabled={previewMut.isPending}>
          {previewMut.isPending ? 'Simulating…' : 'Run Preview'}
        </button>

        {preview && (
          <div style={{ marginTop: 20, display: 'grid', gap: 12 }}>
            <PreviewResult assignment={preview.assignment} rules={preview.rules} />
          </div>
        )}
        {previewMut.isError && (
          <div style={{ color: '#FCA5A5', fontSize: 12, marginTop: 12 }}>Preview failed: {(previewMut.error as Error).message}</div>
        )}
      </div>

      {/* Create / Edit modal */}
      {showForm && (
        <div style={overlay}>
          <div style={modal}>
            <div style={{ fontSize: 16, fontWeight: 700, color: '#F0F9FF', marginBottom: 20 }}>
              {editRule.id ? 'Edit Rollout Rule' : 'New Rollout Rule'}
            </div>

            <div style={{ display: 'grid', gap: 14 }}>
              <label style={labelStyle}>
                Rule Name *
                <input id="form-rollout-name" style={inputStyle} value={editRule.name}
                  placeholder="canary-gpt4o-10pct"
                  onChange={e => setEditRule(prev => ({ ...prev, name: e.target.value }))} />
              </label>

              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
                <label style={labelStyle}>
                  Target Type *
                  <select id="form-rollout-target-type" style={inputStyle}
                    value={editRule.target_type}
                    onChange={e => setEditRule(prev => ({ ...prev, target_type: e.target.value as RolloutRule['target_type'] }))}>
                    <option value="model">Model</option>
                    <option value="prompt_release">Prompt Release</option>
                    <option value="policy_rule">Policy Rule</option>
                  </select>
                </label>
                <label style={labelStyle}>
                  Target ID *
                  <input id="form-rollout-target-id" style={inputStyle} value={editRule.target_id}
                    placeholder={editRule.target_type === 'model' ? 'gpt-4o' : editRule.target_type === 'prompt_release' ? 'my-prompt' : 'rule-id'}
                    onChange={e => setEditRule(prev => ({ ...prev, target_id: e.target.value }))} />
                </label>
              </div>

              <label style={labelStyle}>
                Traffic Percentage: {editRule.percentage}%
                <input id="form-rollout-pct" type="range" min={0} max={100} step={1}
                  value={editRule.percentage}
                  style={{ width: '100%', accentColor: '#3B82F6', marginTop: 6 }}
                  onChange={e => setEditRule(prev => ({ ...prev, percentage: Number(e.target.value) }))} />
              </label>

              {editRule.target_type === 'model' && (
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
                  <label style={labelStyle}>
                    Control Model
                    <input id="form-rollout-control-model" style={inputStyle} value={editRule.control_model ?? ''}
                      placeholder="gpt-4-turbo"
                      onChange={e => setEditRule(prev => ({ ...prev, control_model: e.target.value }))} />
                  </label>
                  <label style={labelStyle}>
                    Candidate Model
                    <input id="form-rollout-candidate-model" style={inputStyle} value={editRule.candidate_model ?? ''}
                      placeholder="gpt-4o"
                      onChange={e => setEditRule(prev => ({ ...prev, candidate_model: e.target.value }))} />
                  </label>
                </div>
              )}

              {editRule.target_type === 'prompt_release' && (
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
                  <label style={labelStyle}>
                    Control Release Tag
                    <input id="form-rollout-control-tag" style={inputStyle} value={editRule.control_release_tag ?? ''}
                      placeholder="v1.0.0"
                      onChange={e => setEditRule(prev => ({ ...prev, control_release_tag: e.target.value }))} />
                  </label>
                  <label style={labelStyle}>
                    Candidate Release Tag
                    <input id="form-rollout-candidate-tag" style={inputStyle} value={editRule.candidate_release_tag ?? ''}
                      placeholder="v1.1.0"
                      onChange={e => setEditRule(prev => ({ ...prev, candidate_release_tag: e.target.value }))} />
                  </label>
                </div>
              )}

              <label style={labelStyle}>
                Environment (leave blank for all)
                <input id="form-rollout-env" style={inputStyle} value={editRule.environment ?? ''}
                  placeholder="production"
                  onChange={e => setEditRule(prev => ({ ...prev, environment: e.target.value }))} />
              </label>
            </div>

            {formError && <div style={{ color: '#FCA5A5', fontSize: 12, marginTop: 12 }}>{formError}</div>}

            <div style={{ display: 'flex', gap: 10, justifyContent: 'flex-end', marginTop: 24 }}>
              <button id="rollout-form-cancel" style={ghostBtn} onClick={() => setShowForm(false)}>Cancel</button>
              <button id="rollout-form-save" style={primaryBtn} onClick={handleSave} disabled={upsert.isPending}>
                {upsert.isPending ? 'Saving…' : 'Save Rule'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

// ─── Sub-components ────────────────────────────────────────────────────────────

function MetricCard({ label, value, color = '#60A5FA' }: { label: string; value: string; color?: string }) {
  return (
    <div style={{ background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 10, padding: '18px 20px' }}>
      <div style={{ fontSize: 10, color: '#475569', letterSpacing: '0.1em', marginBottom: 8 }}>{label.toUpperCase()}</div>
      <div style={{ fontSize: 28, fontWeight: 700, color }}>{value}</div>
    </div>
  )
}

function PreviewResult({ assignment, rules }: { assignment: import('../hooks/api').RolloutAssignment; rules: import('../hooks/api').RolloutRule[] }) {
  return (
    <div style={{ border: '1px solid #0F1F35', borderRadius: 10, padding: 20, background: '#071525' }}>
      <div style={{ fontSize: 11, color: '#334155', letterSpacing: '0.12em', marginBottom: 12 }}>ASSIGNMENT RESULT</div>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 12, marginBottom: 16 }}>
        <div>
          <div style={{ fontSize: 10, color: '#475569', marginBottom: 4 }}>SELECTED</div>
          <div style={{ fontSize: 13, color: assignment.selected ? '#10B981' : '#94A3B8', fontWeight: 600 }}>
            {assignment.selected ? 'Yes (candidate)' : 'No (control)'}
          </div>
        </div>
        <div>
          <div style={{ fontSize: 10, color: '#475569', marginBottom: 4 }}>VARIANT</div>
          <div style={{ fontSize: 13, color: '#E2E8F0' }}>{assignment.variant ?? '—'}</div>
        </div>
        <div>
          <div style={{ fontSize: 10, color: '#475569', marginBottom: 4 }}>BUCKET</div>
          <div style={{ fontSize: 13, color: '#E2E8F0' }}>{assignment.bucket ?? '—'}</div>
        </div>
      </div>
      {assignment.rule_name && (
        <div style={{ fontSize: 12, color: '#94A3B8' }}>
          Matched rule: <span style={{ color: '#60A5FA' }}>{assignment.rule_name}</span>
          {assignment.candidate_model && <> · candidate model: <span style={{ color: '#60A5FA' }}>{assignment.candidate_model}</span></>}
          {assignment.release_tag && <> · release: <span style={{ color: '#8B5CF6' }}>{assignment.release_tag}</span></>}
        </div>
      )}
      {rules.length > 0 && (
        <div style={{ marginTop: 12, fontSize: 11, color: '#334155' }}>
          {rules.length} rule{rules.length > 1 ? 's' : ''} evaluated
        </div>
      )}
    </div>
  )
}

// ─── Styles ───────────────────────────────────────────────────────────────────

const titleStyle: CSSProperties = { fontSize: 22, fontWeight: 700, color: '#F0F9FF', margin: 0 }
const subtleText: CSSProperties = { fontSize: 12, color: '#475569', marginTop: 4 }
const panelStyle: CSSProperties = { background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 10, padding: 24 }
const labelStyle: CSSProperties = { display: 'grid', gap: 6, fontSize: 11, color: '#94A3B8' }
const inputStyle: CSSProperties = {
  background: '#060A14', border: '1px solid #0F1F35', borderRadius: 8,
  color: '#E2E8F0', padding: '9px 12px', fontSize: 12, outline: 'none',
}
const primaryBtn: CSSProperties = {
  background: 'linear-gradient(135deg, #2563EB, #3B82F6)', color: '#fff',
  border: 'none', borderRadius: 8, padding: '10px 18px', fontSize: 12,
  fontWeight: 600, cursor: 'pointer', letterSpacing: '0.02em',
}
const ghostBtn: CSSProperties = {
  background: 'none', border: '1px solid #0F1F35', borderRadius: 8,
  color: '#64748B', padding: '7px 14px', fontSize: 12, cursor: 'pointer',
}
const thStyle: CSSProperties = {
  padding: '8px 14px', textAlign: 'left', color: '#334155',
  borderBottom: '1px solid #0F1F35', fontSize: 10, fontWeight: 600, letterSpacing: '0.08em',
}
const tdStyle: CSSProperties = { padding: '10px 14px', color: '#94A3B8', verticalAlign: 'middle' }
const overlay: CSSProperties = {
  position: 'fixed', inset: 0, background: 'rgba(4,8,20,0.85)',
  display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 1000,
  backdropFilter: 'blur(4px)',
}
const modal: CSSProperties = {
  background: '#0D1B2A', border: '1px solid #1E3A5F', borderRadius: 14,
  padding: 28, maxWidth: 600, width: '100%', boxShadow: '0 24px 64px rgba(0,0,0,0.6)',
}
