import { useMemo, useState, type CSSProperties } from 'react'
import { hasRole, useAuth } from '../hooks/auth'
import { PricingRule, useDeletePricingRule, usePricingRules, useUpsertPricingRule } from '../hooks/api'

const emptyRule: PricingRule = {
  provider: 'openai',
  model_pattern: '',
  input_per_million: 0,
  output_per_million: 0,
}

export default function PricingRulesPage() {
  const { user } = useAuth()
  const isAdmin = hasRole(user, ['admin'])
  const { data, isLoading, error } = usePricingRules()
  const upsert = useUpsertPricingRule()
  const remove = useDeletePricingRule()
  const [form, setForm] = useState<PricingRule>(emptyRule)

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
    upsert.mutate(form, {
      onSuccess: () => setForm(emptyRule),
    })
  }

  return (
    <div style={{ padding: 32 }}>
      <div style={{ marginBottom: 24 }}>
        <h1 style={{ fontSize: 22, fontWeight: 700, color: '#F0F9FF', margin: 0 }}>Pricing Rules</h1>
        <p style={{ fontSize: 12, color: '#475569', marginTop: 4 }}>
          Edit model pricing used by proxy, budgets, and ingested cost recomputation.
        </p>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'minmax(360px, 420px) 1fr', gap: 16 }}>
        <div style={{ background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 10, padding: 24 }}>
          <div style={{ fontSize: 12, color: '#475569', letterSpacing: '0.1em', marginBottom: 16 }}>
            {form.id ? 'EDIT RULE' : 'NEW RULE'}
          </div>

          <div style={{ display: 'grid', gap: 12 }}>
            <label style={{ fontSize: 11, color: '#94A3B8' }}>
              Provider
              <input
                value={form.provider}
                onChange={e => setForm(f => ({ ...f, provider: e.target.value }))}
                style={inputStyle}
                placeholder="openai"
              />
            </label>
            <label style={{ fontSize: 11, color: '#94A3B8' }}>
              Model Pattern
              <input
                value={form.model_pattern}
                onChange={e => setForm(f => ({ ...f, model_pattern: e.target.value }))}
                style={inputStyle}
                placeholder="gpt-4o"
              />
            </label>
            <label style={{ fontSize: 11, color: '#94A3B8' }}>
              Input Price Per 1M
              <input
                type="number"
                min={0}
                step={0.01}
                value={form.input_per_million}
                onChange={e => setForm(f => ({ ...f, input_per_million: Number(e.target.value) }))}
                style={inputStyle}
              />
            </label>
            <label style={{ fontSize: 11, color: '#94A3B8' }}>
              Output Price Per 1M
              <input
                type="number"
                min={0}
                step={0.01}
                value={form.output_per_million}
                onChange={e => setForm(f => ({ ...f, output_per_million: Number(e.target.value) }))}
                style={inputStyle}
              />
            </label>
          </div>

          <div style={{ display: 'flex', gap: 8, marginTop: 16 }}>
            <button style={primaryBtn} onClick={save} disabled={upsert.isPending || !form.model_pattern}>
              {upsert.isPending ? 'Saving…' : form.id ? 'Save Changes' : 'Create Rule'}
            </button>
            <button style={secondaryBtn} onClick={() => setForm(emptyRule)}>
              Clear
            </button>
          </div>
          {upsert.isError && (
            <div style={{ fontSize: 11, color: '#EF4444', marginTop: 10 }}>
              Failed to save pricing rule.
            </div>
          )}
        </div>

        <div style={{ background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 10, padding: 24 }}>
          <div style={{ fontSize: 12, color: '#475569', letterSpacing: '0.1em', marginBottom: 16 }}>
            ACTIVE RULES
          </div>

          {isLoading ? (
            <div style={{ color: '#334155', fontSize: 12 }}>Loading…</div>
          ) : error ? (
            <div style={{ color: '#EF4444', fontSize: 12 }}>Failed to load pricing rules.</div>
          ) : rules.length === 0 ? (
            <div style={{ color: '#334155', fontSize: 12 }}>No pricing rules configured.</div>
          ) : (
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
              <thead>
                <tr style={{ textAlign: 'left', color: '#64748B', borderBottom: '1px solid #0F1F35' }}>
                  <th style={thStyle}>Provider</th>
                  <th style={thStyle}>Model Pattern</th>
                  <th style={thStyle}>Input / 1M</th>
                  <th style={thStyle}>Output / 1M</th>
                  <th style={thStyle}>Actions</th>
                </tr>
              </thead>
              <tbody>
                {rules.map(rule => (
                  <tr key={rule.id} style={{ borderBottom: '1px solid #0B1627' }}>
                    <td style={tdStyle}>{rule.provider || 'any'}</td>
                    <td style={tdStyle}>{rule.model_pattern}</td>
                    <td style={tdStyle}>${rule.input_per_million.toFixed(2)}</td>
                    <td style={tdStyle}>${rule.output_per_million.toFixed(2)}</td>
                    <td style={tdStyle}>
                      <div style={{ display: 'flex', gap: 8 }}>
                        <button style={secondaryBtnSmall} onClick={() => setForm(rule)}>Edit</button>
                        <button
                          style={dangerBtnSmall}
                          onClick={() => rule.id && remove.mutate(rule.id)}
                          disabled={remove.isPending}
                        >
                          Delete
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>
    </div>
  )
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
}
