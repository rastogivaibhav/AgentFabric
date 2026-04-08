import { useMemo, useState, type CSSProperties } from 'react'
import { hasRole, useAuth } from '../hooks/auth'
import { PricingRule, useDeletePricingRule, usePricingRules, useUpsertPricingRule, usePreviewPricingRule, usePricingRuleAudit } from '../hooks/api'

const emptyRule: PricingRule = {
  provider: 'openai', model_pattern: '', input_per_million: 0, output_per_million: 0,
  active: true, priority: 100, tenant_id: null, effective_from: null, effective_to: null, description: '',
}

export default function PricingRulesPage() {
  const { user } = useAuth()
  const isAdmin = hasRole(user, ['admin'])
  const { data, isLoading, error } = usePricingRules()
  const { data: audit } = usePricingRuleAudit(25)
  const upsert = useUpsertPricingRule()
  const remove = useDeletePricingRule()
  const preview = usePreviewPricingRule()
  const [form, setForm] = useState<PricingRule>(emptyRule)
  const [previewForm, setPreviewForm] = useState({ tenant_id: '', provider: 'openai', model: 'gpt-4o', input_tokens: 1000, output_tokens: 500 })

  const rules = useMemo(() => data?.items ?? [], [data])

  if (!isAdmin) {
    return (
      <div style={{ padding: 32 }}>
        <div style={{ background: 'var(--layer-2)', border: '1px solid var(--layer-border)', borderRadius: 12, padding: 24 }}>
          <h1 style={{ fontSize: 22, fontWeight: 700, color: 'var(--text-primary)', margin: 0 }}>Pricing Rules</h1>
          <p style={{ color: 'var(--text-tertiary)', fontSize: 12, marginTop: 8 }}>This page is restricted to administrators.</p>
        </div>
      </div>
    )
  }

  const save = () => { upsert.mutate({ ...form, tenant_id: form.tenant_id?.trim() ? form.tenant_id.trim() : null, effective_from: form.effective_from || null, effective_to: form.effective_to || null }, { onSuccess: () => setForm(emptyRule) }) }

  return (
    <div style={{ padding: '40px 48px', maxWidth: 1440, margin: '0 auto' }}>
      <div style={{ marginBottom: 28, display: 'flex', justifyContent: 'space-between', alignItems: 'flex-end', gap: 16 }}>
        <div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
            <span style={{ width: 8, height: 8, borderRadius: '50%', background: 'var(--spend)', display: 'inline-block' }} />
            <span style={{ fontSize: 10, fontWeight: 700, color: 'var(--spend)', letterSpacing: '0.1em' }}>SPEND</span>
          </div>
          <h1 style={{ fontSize: 28, fontWeight: 700, color: 'var(--text-primary)', margin: 0, letterSpacing: '-0.02em' }}>Pricing Rules</h1>
          <p style={{ fontSize: 12, color: 'var(--text-tertiary)', marginTop: 4 }}>
            Gateway-authoritative cost calculation with tenant overrides, effective dates, preview, and audit.
          </p>
        </div>
        <a href={`${import.meta.env.VITE_API_URL ?? 'http://localhost:8080'}/api/v1/pricing/export`} style={exportLinkStyle}>Export CSV</a>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'minmax(360px, 430px) 1fr', gap: 16, marginBottom: 16 }}>
        {/* Rule form */}
        <div style={panelStyle}>
          <div style={sectionLabel}>{form.id ? 'EDIT RULE' : 'NEW RULE'}</div>
          <div style={{ display: 'grid', gap: 12 }}>
            {[
              { label: 'Tenant Override', key: 'tenant_id', placeholder: 'leave blank for global', value: form.tenant_id ?? '' },
              { label: 'Provider', key: 'provider', value: form.provider },
              { label: 'Model Pattern', key: 'model_pattern', value: form.model_pattern },
            ].map(f => (
              <label key={f.key} style={labelStyle}>
                {f.label}
                <input id={`pricing-${f.key}`} style={inputStyle} value={f.value} placeholder={f.placeholder}
                  onChange={e => setForm(r => ({ ...r, [f.key]: e.target.value }))} />
              </label>
            ))}
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
              <label style={labelStyle}>Input / 1M<input type="number" min={0} step={0.01} style={inputStyle} value={form.input_per_million} onChange={e => setForm(r => ({ ...r, input_per_million: Number(e.target.value) }))} /></label>
              <label style={labelStyle}>Output / 1M<input type="number" min={0} step={0.01} style={inputStyle} value={form.output_per_million} onChange={e => setForm(r => ({ ...r, output_per_million: Number(e.target.value) }))} /></label>
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
              <label style={labelStyle}>Priority<input type="number" min={0} style={inputStyle} value={form.priority ?? 100} onChange={e => setForm(r => ({ ...r, priority: Number(e.target.value) }))} /></label>
              <label style={{ ...labelStyle, flexDirection: 'row', alignItems: 'center', gap: 8, marginTop: 20 }}>
                <input type="checkbox" checked={form.active ?? true} onChange={e => setForm(r => ({ ...r, active: e.target.checked }))} /> Active
              </label>
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
              <label style={labelStyle}>Effective From<input type="datetime-local" style={inputStyle} value={toDateTimeLocal(form.effective_from)} onChange={e => setForm(r => ({ ...r, effective_from: fromDateTimeLocal(e.target.value) }))} /></label>
              <label style={labelStyle}>Effective To<input type="datetime-local" style={inputStyle} value={toDateTimeLocal(form.effective_to)} onChange={e => setForm(r => ({ ...r, effective_to: fromDateTimeLocal(e.target.value) }))} /></label>
            </div>
            <label style={labelStyle}>Description<textarea style={{ ...inputStyle, resize: 'vertical' } as CSSProperties} value={form.description ?? ''} rows={3} onChange={e => setForm(r => ({ ...r, description: e.target.value }))} /></label>
          </div>
          <div style={{ display: 'flex', gap: 8, marginTop: 18 }}>
            <button id="pricing-save-btn" style={primaryBtn} onClick={save} disabled={upsert.isPending || !form.model_pattern}>{upsert.isPending ? 'Saving…' : form.id ? 'Save Changes' : 'Create Rule'}</button>
            <button style={ghostBtn} onClick={() => setForm(emptyRule)}>Clear</button>
          </div>
          {upsert.isError && <div style={errorStyle}>Failed to save pricing rule.</div>}
        </div>

        {/* Pricing preview */}
        <div style={panelStyle}>
          <div style={sectionLabel}>PRICING PREVIEW</div>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(2, 1fr)', gap: 12 }}>
            <label style={labelStyle}>Tenant<input style={inputStyle} value={previewForm.tenant_id} onChange={e => setPreviewForm(f => ({ ...f, tenant_id: e.target.value }))} placeholder="optional" /></label>
            <label style={labelStyle}>Provider<input style={inputStyle} value={previewForm.provider} onChange={e => setPreviewForm(f => ({ ...f, provider: e.target.value }))} /></label>
            <label style={labelStyle}>Model<input style={inputStyle} value={previewForm.model} onChange={e => setPreviewForm(f => ({ ...f, model: e.target.value }))} /></label>
            <label style={labelStyle}>Input Tokens<input type="number" min={0} style={inputStyle} value={previewForm.input_tokens} onChange={e => setPreviewForm(f => ({ ...f, input_tokens: Number(e.target.value) }))} /></label>
            <label style={labelStyle}>Output Tokens<input type="number" min={0} style={inputStyle} value={previewForm.output_tokens} onChange={e => setPreviewForm(f => ({ ...f, output_tokens: Number(e.target.value) }))} /></label>
          </div>
          <button id="pricing-preview-btn" style={{ ...primaryBtn, marginTop: 16 }} onClick={() => preview.mutate(previewForm)} disabled={preview.isPending || !previewForm.model}>
            {preview.isPending ? 'Previewing…' : 'Run Preview'}
          </button>
          {preview.data && (
            <div style={{ marginTop: 16, display: 'grid', gridTemplateColumns: 'repeat(2, 1fr)', gap: 10 }}>
              {[
                ['Matched', preview.data.matched ? 'yes' : 'no'],
                ['Rule', preview.data.rule_id ? `${preview.data.rule_id}` : '—'],
                ['Scope', preview.data.pricing_scope ?? '—'],
                ['Pattern', preview.data.model_pattern ?? '—'],
                ['Input Cost', `$${preview.data.input_cost_usd.toFixed(6)}`],
                ['Output Cost', `$${preview.data.output_cost_usd.toFixed(6)}`],
                ['Total', `$${preview.data.total_cost_usd.toFixed(6)}`],
              ].map(([label, value]) => (
                <div key={label} style={{ background: 'var(--layer-0)', border: '1px solid var(--layer-border)', borderRadius: 8, padding: 12 }}>
                  <div style={{ fontSize: 9, color: 'var(--text-tertiary)', letterSpacing: '0.1em', fontWeight: 700 }}>{label}</div>
                  <div style={{ fontSize: 13, color: 'var(--text-primary)', marginTop: 5, fontWeight: 600 }}>{value}</div>
                </div>
              ))}
            </div>
          )}
          {preview.isError && <div style={errorStyle}>Pricing preview failed.</div>}
        </div>
      </div>

      {/* Active rules + audit */}
      <div style={{ display: 'grid', gridTemplateColumns: '1.2fr 0.8fr', gap: 16 }}>
        <div style={panelStyle}>
          <div style={sectionLabel}>ACTIVE RULES ({rules.length})</div>
          {isLoading ? <div style={{ color: 'var(--text-tertiary)', fontSize: 12 }}>Loading…</div>
            : error ? <div style={errorStyle}>Failed to load pricing rules.</div>
            : rules.length === 0 ? <div style={{ color: 'var(--text-tertiary)', fontSize: 12 }}>No pricing rules configured.</div>
            : (
              <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
                <thead>
                  <tr>
                    {['Scope', 'Provider', 'Pattern', 'Rate', 'Window', 'Actions'].map(h => (
                      <th key={h} style={{ padding: '8px', textAlign: 'left', fontSize: 9, fontWeight: 700, letterSpacing: '0.08em', color: 'var(--text-tertiary)', borderBottom: '1px solid var(--layer-border)' }}>{h}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {rules.map(rule => (
                    <tr key={rule.id} style={{ borderBottom: '1px solid var(--layer-border)', opacity: rule.active === false ? 0.5 : 1 }}>
                      <td style={tdStyle}><div style={{ color: 'var(--text-primary)' }}>{rule.tenant_id ? 'Tenant' : 'Global'}</div><div style={{ fontSize: 10, color: 'var(--text-tertiary)' }}>{rule.priority ?? 100}</div></td>
                      <td style={tdStyle}>{rule.provider || 'any'}</td>
                      <td style={tdStyle}><div style={{ color: 'var(--text-primary)' }}>{rule.model_pattern}</div>{rule.description && <div style={{ fontSize: 10, color: 'var(--text-tertiary)', marginTop: 3 }}>{rule.description}</div>}</td>
                      <td style={tdStyle}><div>${rule.input_per_million.toFixed(2)} in</div><div>${rule.output_per_million.toFixed(2)} out</div></td>
                      <td style={tdStyle}><div style={{ fontSize: 10 }}>{rule.effective_from ? new Date(rule.effective_from).toLocaleDateString() : 'now'}</div><div style={{ fontSize: 10, color: 'var(--text-tertiary)' }}>{rule.effective_to ? new Date(rule.effective_to).toLocaleDateString() : 'open'}</div></td>
                      <td style={tdStyle}>
                        <div style={{ display: 'flex', gap: 6 }}>
                          <button style={ghostBtnSm} onClick={() => setForm(rule)}>Edit</button>
                          <button style={dangerBtnSm} onClick={() => rule.id && remove.mutate(rule.id)} disabled={remove.isPending}>Delete</button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
        </div>
        <div style={panelStyle}>
          <div style={sectionLabel}>PRICING AUDIT</div>
          <div style={{ display: 'grid', gap: 8 }}>
            {(audit?.items ?? []).map(entry => (
              <div key={entry.id} style={{ border: '1px solid var(--layer-border)', borderRadius: 8, padding: 12, background: 'var(--layer-0)' }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8 }}>
                  <div style={{ color: 'var(--text-primary)', fontSize: 12, fontWeight: 600 }}>{entry.action.toUpperCase()} · Rule {entry.rule_id}</div>
                  <div style={{ color: 'var(--text-tertiary)', fontSize: 10 }}>{new Date(entry.created_at).toLocaleString()}</div>
                </div>
                <div style={{ color: 'var(--text-tertiary)', fontSize: 11, marginTop: 4 }}>{entry.actor || 'system'} · tenant {entry.tenant_id || 'default'}</div>
              </div>
            ))}
            {(audit?.items?.length ?? 0) === 0 && <div style={{ color: 'var(--text-tertiary)', fontSize: 12 }}>No pricing audit entries yet.</div>}
          </div>
        </div>
      </div>
    </div>
  )
}

function toDateTimeLocal(value?: string | null) { if (!value) return ''; return value.slice(0, 16) }
function fromDateTimeLocal(value: string) { return value ? new Date(value).toISOString() : null }

const panelStyle: CSSProperties = { background: 'var(--layer-2)', border: '1px solid var(--layer-border)', borderRadius: 12, padding: 24 }
const sectionLabel: CSSProperties = { fontSize: 10, color: 'var(--text-tertiary)', letterSpacing: '0.12em', marginBottom: 16, fontWeight: 700 }
const labelStyle: CSSProperties = { fontSize: 11, color: 'var(--text-secondary)', display: 'flex', flexDirection: 'column', gap: 5 }
const inputStyle: CSSProperties = { display: 'block', width: '100%', marginTop: 4, boxSizing: 'border-box', background: 'var(--layer-1)', border: '1px solid var(--layer-border)', borderRadius: 8, color: 'var(--text-primary)', padding: '8px 10px', fontSize: 12, outline: 'none' }
const primaryBtn: CSSProperties = { background: 'var(--spend)', border: 'none', borderRadius: 8, color: '#000', padding: '9px 18px', fontSize: 12, cursor: 'pointer', fontWeight: 700 }
const ghostBtn: CSSProperties = { background: 'none', border: '1px solid var(--layer-border)', borderRadius: 8, color: 'var(--text-secondary)', padding: '8px 16px', fontSize: 12, cursor: 'pointer' }
const ghostBtnSm: CSSProperties = { ...ghostBtn, padding: '5px 10px', fontSize: 11 }
const dangerBtnSm: CSSProperties = { background: 'rgba(255,69,58,0.12)', border: '1px solid rgba(255,69,58,0.25)', borderRadius: 6, color: 'var(--protect)', padding: '5px 10px', fontSize: 11, cursor: 'pointer' }
const tdStyle: CSSProperties = { padding: '10px 8px', color: 'var(--text-secondary)', verticalAlign: 'top' }
const exportLinkStyle: CSSProperties = { background: 'var(--layer-2)', border: '1px solid var(--layer-border)', borderRadius: 8, color: 'var(--control)', textDecoration: 'none', padding: '10px 14px', fontSize: 12 }
const errorStyle: CSSProperties = { fontSize: 12, color: 'var(--protect)', marginTop: 10 }
