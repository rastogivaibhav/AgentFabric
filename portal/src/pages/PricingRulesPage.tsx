import { useMemo, useState, type CSSProperties } from 'react'
import { hasRole, useAuth } from '../hooks/auth'
import {
  PricingRule,
  useDeletePricingRule,
  usePricingRules,
  useUpsertPricingRule,
  usePreviewPricingRule,
  usePricingRuleAudit,
} from '../hooks/api'

const emptyRule: PricingRule = {
  provider: 'openai',
  model_pattern: '',
  input_per_million: 0,
  output_per_million: 0,
  active: true,
  priority: 100,
  tenant_id: null,
  effective_from: null,
  effective_to: null,
  description: '',
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
  const [previewForm, setPreviewForm] = useState({
    tenant_id: '',
    provider: 'openai',
    model: 'gpt-4o',
    input_tokens: 1000,
    output_tokens: 500,
  })

  const rules = useMemo(() => data?.items ?? [], [data])

  if (!isAdmin) {
    return (
      <div style={{ padding: 32 }}>
        <div style={{ background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 10, padding: 24 }}>
          <h1 style={{ fontSize: 22, fontWeight: 700, color: '#F0F9FF', margin: 0 }}>Pricing Rules</h1>
          <p style={{ color: '#475569', fontSize: 12, marginTop: 8 }}>This page is restricted to administrators.</p>
        </div>
      </div>
    )
  }

  const save = () => {
    upsert.mutate(
      {
        ...form,
        tenant_id: form.tenant_id?.trim() ? form.tenant_id.trim() : null,
        effective_from: form.effective_from || null,
        effective_to: form.effective_to || null,
      },
      { onSuccess: () => setForm(emptyRule) },
    )
  }

  return (
    <div style={{ padding: 32 }}>
      <div style={{ marginBottom: 24, display: 'flex', justifyContent: 'space-between', alignItems: 'flex-end', gap: 16 }}>
        <div>
          <h1 style={{ fontSize: 22, fontWeight: 700, color: '#F0F9FF', margin: 0 }}>Pricing Rules</h1>
          <p style={{ fontSize: 12, color: '#475569', marginTop: 4 }}>
            Govern gateway-authoritative pricing with tenant overrides, effective dates, preview, audit, and export.
          </p>
        </div>
        <a href={`${import.meta.env.VITE_API_URL ?? 'http://localhost:8080'}/api/v1/pricing/export`} style={exportLinkStyle}>
          Export CSV
        </a>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'minmax(360px, 430px) 1fr', gap: 16, marginBottom: 16 }}>
        <div style={panelStyle}>
          <div style={sectionLabel}>{form.id ? 'EDIT RULE' : 'NEW RULE'}</div>
          <div style={{ display: 'grid', gap: 12 }}>
            <label style={labelStyle}>
              Tenant Override
              <input
                value={form.tenant_id ?? ''}
                onChange={e => setForm(f => ({ ...f, tenant_id: e.target.value }))}
                style={inputStyle}
                placeholder="leave blank for global"
              />
            </label>
            <label style={labelStyle}>
              Provider
              <input value={form.provider} onChange={e => setForm(f => ({ ...f, provider: e.target.value }))} style={inputStyle} />
            </label>
            <label style={labelStyle}>
              Model Pattern
              <input value={form.model_pattern} onChange={e => setForm(f => ({ ...f, model_pattern: e.target.value }))} style={inputStyle} />
            </label>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
              <label style={labelStyle}>
                Input / 1M
                <input type="number" min={0} step={0.01} value={form.input_per_million} onChange={e => setForm(f => ({ ...f, input_per_million: Number(e.target.value) }))} style={inputStyle} />
              </label>
              <label style={labelStyle}>
                Output / 1M
                <input type="number" min={0} step={0.01} value={form.output_per_million} onChange={e => setForm(f => ({ ...f, output_per_million: Number(e.target.value) }))} style={inputStyle} />
              </label>
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
              <label style={labelStyle}>
                Priority
                <input type="number" min={0} step={1} value={form.priority ?? 100} onChange={e => setForm(f => ({ ...f, priority: Number(e.target.value) }))} style={inputStyle} />
              </label>
              <label style={{ ...labelStyle, display: 'flex', alignItems: 'center', gap: 8 }}>
                <input type="checkbox" checked={form.active ?? true} onChange={e => setForm(f => ({ ...f, active: e.target.checked }))} />
                Active
              </label>
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
              <label style={labelStyle}>
                Effective From
                <input type="datetime-local" value={toDateTimeLocal(form.effective_from)} onChange={e => setForm(f => ({ ...f, effective_from: fromDateTimeLocal(e.target.value) }))} style={inputStyle} />
              </label>
              <label style={labelStyle}>
                Effective To
                <input type="datetime-local" value={toDateTimeLocal(form.effective_to)} onChange={e => setForm(f => ({ ...f, effective_to: fromDateTimeLocal(e.target.value) }))} style={inputStyle} />
              </label>
            </div>
            <label style={labelStyle}>
              Description
              <textarea value={form.description ?? ''} onChange={e => setForm(f => ({ ...f, description: e.target.value }))} style={textareaStyle} rows={3} />
            </label>
          </div>

          <div style={{ display: 'flex', gap: 8, marginTop: 16 }}>
            <button style={primaryBtn} onClick={save} disabled={upsert.isPending || !form.model_pattern}>
              {upsert.isPending ? 'Saving…' : form.id ? 'Save Changes' : 'Create Rule'}
            </button>
            <button style={secondaryBtn} onClick={() => setForm(emptyRule)}>Clear</button>
          </div>
          {upsert.isError && <div style={errorStyle}>Failed to save pricing rule.</div>}
        </div>

        <div style={panelStyle}>
          <div style={sectionLabel}>PRICING PREVIEW</div>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(2, minmax(0, 1fr))', gap: 12 }}>
            <label style={labelStyle}>
              Tenant
              <input value={previewForm.tenant_id} onChange={e => setPreviewForm(f => ({ ...f, tenant_id: e.target.value }))} style={inputStyle} placeholder="optional tenant override" />
            </label>
            <label style={labelStyle}>
              Provider
              <input value={previewForm.provider} onChange={e => setPreviewForm(f => ({ ...f, provider: e.target.value }))} style={inputStyle} />
            </label>
            <label style={labelStyle}>
              Model
              <input value={previewForm.model} onChange={e => setPreviewForm(f => ({ ...f, model: e.target.value }))} style={inputStyle} />
            </label>
            <label style={labelStyle}>
              Input Tokens
              <input type="number" min={0} value={previewForm.input_tokens} onChange={e => setPreviewForm(f => ({ ...f, input_tokens: Number(e.target.value) }))} style={inputStyle} />
            </label>
            <label style={labelStyle}>
              Output Tokens
              <input type="number" min={0} value={previewForm.output_tokens} onChange={e => setPreviewForm(f => ({ ...f, output_tokens: Number(e.target.value) }))} style={inputStyle} />
            </label>
          </div>
          <div style={{ marginTop: 16 }}>
            <button
              style={primaryBtn}
              onClick={() => preview.mutate(previewForm)}
              disabled={preview.isPending || !previewForm.model}
            >
              {preview.isPending ? 'Previewing…' : 'Run Preview'}
            </button>
          </div>
          {preview.data && (
            <div style={{ marginTop: 16, display: 'grid', gridTemplateColumns: 'repeat(2, minmax(0, 1fr))', gap: 12 }}>
              {[
                ['Matched', preview.data.matched ? 'yes' : 'no'],
                ['Rule', preview.data.rule_id ? `${preview.data.rule_id}` : '—'],
                ['Scope', preview.data.pricing_scope ?? '—'],
                ['Pattern', preview.data.model_pattern ?? '—'],
                ['Input Cost', `$${preview.data.input_cost_usd.toFixed(6)}`],
                ['Output Cost', `$${preview.data.output_cost_usd.toFixed(6)}`],
                ['Total', `$${preview.data.total_cost_usd.toFixed(6)}`],
              ].map(([label, value]) => (
                <div key={label} style={previewStatStyle}>
                  <div style={{ fontSize: 9, color: '#334155', letterSpacing: '0.1em' }}>{label}</div>
                  <div style={{ fontSize: 12, color: '#E2E8F0', marginTop: 4 }}>{value}</div>
                </div>
              ))}
            </div>
          )}
          {preview.isError && <div style={errorStyle}>Pricing preview failed.</div>}
        </div>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: '1.2fr 0.8fr', gap: 16 }}>
        <div style={panelStyle}>
          <div style={sectionLabel}>ACTIVE RULES</div>
          {isLoading ? (
            <div style={{ color: '#334155', fontSize: 12 }}>Loading…</div>
          ) : error ? (
            <div style={errorStyle}>Failed to load pricing rules.</div>
          ) : rules.length === 0 ? (
            <div style={{ color: '#334155', fontSize: 12 }}>No pricing rules configured.</div>
          ) : (
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
              <thead>
                <tr style={{ textAlign: 'left', color: '#64748B', borderBottom: '1px solid #0F1F35' }}>
                  {['Scope', 'Provider', 'Pattern', 'Rate', 'Window', 'Actions'].map(h => <th key={h} style={thStyle}>{h}</th>)}
                </tr>
              </thead>
              <tbody>
                {rules.map(rule => (
                  <tr key={rule.id} style={{ borderBottom: '1px solid #0B1627', opacity: rule.active === false ? 0.6 : 1 }}>
                    <td style={tdStyle}>
                      <div>{rule.tenant_id ? 'Tenant' : 'Global'}</div>
                      <div style={{ fontSize: 10, color: '#64748B' }}>{rule.priority ?? 100}</div>
                    </td>
                    <td style={tdStyle}>{rule.provider || 'any'}</td>
                    <td style={tdStyle}>
                      <div>{rule.model_pattern}</div>
                      {rule.description && <div style={{ fontSize: 10, color: '#64748B', marginTop: 4 }}>{rule.description}</div>}
                    </td>
                    <td style={tdStyle}>
                      <div>${rule.input_per_million.toFixed(2)} in</div>
                      <div>${rule.output_per_million.toFixed(2)} out</div>
                    </td>
                    <td style={tdStyle}>
                      <div>{rule.effective_from ? new Date(rule.effective_from).toLocaleString() : 'now'}</div>
                      <div>{rule.effective_to ? new Date(rule.effective_to).toLocaleString() : 'open-ended'}</div>
                    </td>
                    <td style={tdStyle}>
                      <div style={{ display: 'flex', gap: 8 }}>
                        <button style={secondaryBtnSmall} onClick={() => setForm(rule)}>Edit</button>
                        <button style={dangerBtnSmall} onClick={() => rule.id && remove.mutate(rule.id)} disabled={remove.isPending}>Delete</button>
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
          <div style={{ display: 'grid', gap: 10 }}>
            {(audit?.items ?? []).map(entry => (
              <div key={entry.id} style={{ border: '1px solid #0F1F35', borderRadius: 8, padding: 12, background: '#071525' }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8 }}>
                  <div style={{ color: '#E2E8F0', fontSize: 12, fontWeight: 600 }}>{entry.action.toUpperCase()} · Rule {entry.rule_id}</div>
                  <div style={{ color: '#475569', fontSize: 10 }}>{new Date(entry.created_at).toLocaleString()}</div>
                </div>
                <div style={{ color: '#64748B', fontSize: 11, marginTop: 4 }}>{entry.actor || 'system'} · tenant {entry.tenant_id || 'default'}</div>
              </div>
            ))}
            {(audit?.items?.length ?? 0) === 0 && <div style={{ color: '#334155', fontSize: 12 }}>No pricing audit entries yet.</div>}
          </div>
        </div>
      </div>
    </div>
  )
}

function toDateTimeLocal(value?: string | null) {
  if (!value) return ''
  return value.slice(0, 16)
}

function fromDateTimeLocal(value: string) {
  return value ? new Date(value).toISOString() : null
}

const panelStyle: CSSProperties = {
  background: '#0D1B2A',
  border: '1px solid #0F1F35',
  borderRadius: 10,
  padding: 24,
}

const sectionLabel: CSSProperties = {
  fontSize: 12,
  color: '#475569',
  letterSpacing: '0.1em',
  marginBottom: 16,
}

const labelStyle: CSSProperties = {
  fontSize: 11,
  color: '#94A3B8',
}

const inputStyle: CSSProperties = {
  display: 'block',
  width: '100%',
  marginTop: 4,
  boxSizing: 'border-box',
  background: '#071525',
  border: '1px solid #1E3A5F',
  borderRadius: 6,
  color: '#F0F9FF',
  padding: '8px 10px',
  fontSize: 12,
}

const textareaStyle: CSSProperties = {
  ...inputStyle,
  resize: 'vertical',
} as CSSProperties

const primaryBtn: CSSProperties = {
  background: '#1D4ED8',
  border: 'none',
  borderRadius: 6,
  color: '#fff',
  padding: '8px 16px',
  fontSize: 12,
  cursor: 'pointer',
  fontWeight: 600,
}

const secondaryBtn: CSSProperties = {
  background: '#1E3A5F',
  border: 'none',
  borderRadius: 6,
  color: '#fff',
  padding: '8px 16px',
  fontSize: 12,
  cursor: 'pointer',
}

const secondaryBtnSmall: CSSProperties = {
  ...secondaryBtn,
  padding: '6px 10px',
  fontSize: 11,
}

const dangerBtnSmall: CSSProperties = {
  background: '#7F1D1D',
  border: 'none',
  borderRadius: 6,
  color: '#fff',
  padding: '6px 10px',
  fontSize: 11,
  cursor: 'pointer',
}

const thStyle: CSSProperties = {
  padding: '10px 8px',
  fontWeight: 600,
}

const tdStyle: CSSProperties = {
  padding: '12px 8px',
  color: '#E2E8F0',
  verticalAlign: 'top',
}

const previewStatStyle: CSSProperties = {
  background: '#071525',
  border: '1px solid #0F1F35',
  borderRadius: 8,
  padding: 12,
}

const exportLinkStyle: CSSProperties = {
  background: '#071525',
  border: '1px solid #1E3A5F',
  borderRadius: 8,
  color: '#60A5FA',
  textDecoration: 'none',
  padding: '10px 14px',
  fontSize: 12,
}

const errorStyle: CSSProperties = {
  fontSize: 11,
  color: '#EF4444',
  marginTop: 10,
}
