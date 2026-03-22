import { useMemo, useState, type CSSProperties } from 'react'
import { hasRole, useAuth } from '../hooks/auth'
import {
  PolicyRule,
  useControlAudit,
  useDeletePolicyRule,
  usePreviewPolicyRule,
  usePolicyRules,
  useUpsertPolicyRule,
} from '../hooks/api'

const emptyRule: PolicyRule = {
  name: '',
  rule_type: 'traffic',
  enabled: true,
  priority: 100,
  action: 'deny',
  provider: '',
  model_pattern: '',
  environment: '',
  max_tokens: 0,
  detector: '',
  scope: 'both',
  tenant_id: null,
  description: '',
}

export default function PoliciesPage() {
  const { user } = useAuth()
  const isAdmin = hasRole(user, ['admin'])
  const { data, isLoading, error } = usePolicyRules()
  const { data: audit } = useControlAudit(25)
  const upsert = useUpsertPolicyRule()
  const remove = useDeletePolicyRule()
  const preview = usePreviewPolicyRule()
  const [form, setForm] = useState<PolicyRule>(emptyRule)
  const [previewRequest, setPreviewRequest] = useState({
    tenant_id: '',
    provider: 'openai',
    model: 'gpt-4o',
    environment: 'production',
    estimated_tokens: 128,
    request_body: '',
    response_body: '',
  })

  const rules = useMemo(() => data?.items ?? [], [data])

  if (!isAdmin) {
    return (
      <div style={{ padding: 32 }}>
        <div style={panelStyle}>
          <h1 style={titleStyle}>Policies</h1>
          <p style={subtleText}>This page is restricted to administrators.</p>
        </div>
      </div>
    )
  }

  const save = () => {
    upsert.mutate(
      {
        ...form,
        tenant_id: form.tenant_id?.trim() ? form.tenant_id.trim() : null,
        provider: form.provider?.trim().toLowerCase() ?? '',
        model_pattern: form.model_pattern?.trim().toLowerCase() ?? '',
        environment: form.environment?.trim().toLowerCase() ?? '',
        detector: form.detector?.trim().toLowerCase() ?? '',
      },
      { onSuccess: () => setForm(emptyRule) },
    )
  }

  return (
    <div style={{ padding: 32 }}>
      <div style={{ marginBottom: 24 }}>
        <h1 style={titleStyle}>Policies</h1>
        <p style={subtleText}>
          Manage live traffic enforcement and DLP rules for the proxy and netproxy paths.
        </p>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'minmax(360px, 430px) 1fr', gap: 16 }}>
        <div style={panelStyle}>
          <div style={sectionLabel}>{form.id ? 'EDIT RULE' : 'NEW RULE'}</div>
          <div style={{ display: 'grid', gap: 12 }}>
            <label style={labelStyle}>
              Tenant Override
              <input value={form.tenant_id ?? ''} onChange={e => setForm(f => ({ ...f, tenant_id: e.target.value }))} style={inputStyle} placeholder="leave blank for global" />
            </label>
            <label style={labelStyle}>
              Name
              <input value={form.name} onChange={e => setForm(f => ({ ...f, name: e.target.value }))} style={inputStyle} />
            </label>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
              <label style={labelStyle}>
                Rule Type
                <select value={form.rule_type} onChange={e => setForm(f => ({ ...f, rule_type: e.target.value as PolicyRule['rule_type'], action: e.target.value === 'dlp' ? 'warn' : 'deny' }))} style={inputStyle}>
                  <option value="traffic">traffic</option>
                  <option value="dlp">dlp</option>
                </select>
              </label>
              <label style={labelStyle}>
                Action
                <select value={form.action} onChange={e => setForm(f => ({ ...f, action: e.target.value as PolicyRule['action'] }))} style={inputStyle}>
                  <option value="allow">allow</option>
                  <option value="warn">warn</option>
                  {form.rule_type === 'dlp' && <option value="redact">redact</option>}
                  <option value="deny">deny</option>
                </select>
              </label>
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
              <label style={labelStyle}>
                Priority
                <input type="number" min={0} value={form.priority ?? 100} onChange={e => setForm(f => ({ ...f, priority: Number(e.target.value) }))} style={inputStyle} />
              </label>
              <label style={{ ...labelStyle, display: 'flex', alignItems: 'center', gap: 8 }}>
                <input type="checkbox" checked={form.enabled ?? true} onChange={e => setForm(f => ({ ...f, enabled: e.target.checked }))} />
                Enabled
              </label>
            </div>
            <label style={labelStyle}>
              Provider
              <input value={form.provider ?? ''} onChange={e => setForm(f => ({ ...f, provider: e.target.value }))} style={inputStyle} placeholder="openai, anthropic, or *" />
            </label>
            <label style={labelStyle}>
              Model Pattern
              <input value={form.model_pattern ?? ''} onChange={e => setForm(f => ({ ...f, model_pattern: e.target.value }))} style={inputStyle} placeholder="gpt-4o or *" />
            </label>
            <label style={labelStyle}>
              Environment
              <input value={form.environment ?? ''} onChange={e => setForm(f => ({ ...f, environment: e.target.value }))} style={inputStyle} placeholder="production or *" />
            </label>
            {form.rule_type === 'traffic' ? (
              <label style={labelStyle}>
                Max Tokens
                <input type="number" min={0} value={form.max_tokens ?? 0} onChange={e => setForm(f => ({ ...f, max_tokens: Number(e.target.value) }))} style={inputStyle} placeholder="0 = no token ceiling" />
              </label>
            ) : (
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
                <label style={labelStyle}>
                  Detector
                  <select value={form.detector ?? ''} onChange={e => setForm(f => ({ ...f, detector: e.target.value }))} style={inputStyle}>
                    <option value="">any</option>
                    <option value="secret">secret</option>
                    <option value="pii">pii</option>
                    <option value="openai_api_key">openai_api_key</option>
                    <option value="anthropic_api_key">anthropic_api_key</option>
                    <option value="github_token">github_token</option>
                    <option value="aws_access_key">aws_access_key</option>
                    <option value="email">email</option>
                    <option value="ssn">ssn</option>
                  </select>
                </label>
                <label style={labelStyle}>
                  Scope
                  <select value={form.scope ?? 'both'} onChange={e => setForm(f => ({ ...f, scope: e.target.value as PolicyRule['scope'] }))} style={inputStyle}>
                    <option value="request">request</option>
                    <option value="response">response</option>
                    <option value="both">both</option>
                  </select>
                </label>
              </div>
            )}
            <label style={labelStyle}>
              Description
              <textarea value={form.description ?? ''} onChange={e => setForm(f => ({ ...f, description: e.target.value }))} style={textareaStyle} rows={3} />
            </label>
          </div>
          <div style={{ display: 'flex', gap: 8, marginTop: 16 }}>
            <button style={primaryBtn} onClick={save} disabled={upsert.isPending || !form.name}>
              {upsert.isPending ? 'Saving...' : form.id ? 'Save Changes' : 'Create Rule'}
            </button>
            <button style={secondaryBtn} onClick={() => setForm(emptyRule)}>Clear</button>
          </div>
          {upsert.isError && <div style={errorStyle}>Failed to save policy rule.</div>}
        </div>

        <div style={panelStyle}>
          <div style={sectionLabel}>ACTIVE POLICY SET</div>
          {isLoading ? (
            <div style={subtleText}>Loading...</div>
          ) : error ? (
            <div style={errorStyle}>Failed to load policy rules.</div>
          ) : rules.length === 0 ? (
            <div style={subtleText}>No policy rules configured.</div>
          ) : (
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
              <thead>
                <tr style={{ textAlign: 'left', color: '#64748B', borderBottom: '1px solid #0F1F35' }}>
                  {['Name', 'Type', 'Match', 'Action', 'Controls'].map(h => <th key={h} style={thStyle}>{h}</th>)}
                </tr>
              </thead>
              <tbody>
                {rules.map(rule => (
                  <tr key={rule.id} style={{ borderBottom: '1px solid #0B1627', opacity: rule.enabled === false ? 0.6 : 1 }}>
                    <td style={tdStyle}>
                      <div>{rule.name}</div>
                      <div style={{ fontSize: 10, color: '#64748B' }}>priority {rule.priority ?? 100}</div>
                    </td>
                    <td style={tdStyle}>{rule.rule_type}</td>
                    <td style={tdStyle}>
                      <div>{rule.provider || '*'}/{rule.model_pattern || '*'}</div>
                      <div style={{ fontSize: 10, color: '#64748B' }}>
                        env {rule.environment || '*'}
                        {rule.rule_type === 'traffic' && rule.max_tokens ? ` · max ${rule.max_tokens}` : ''}
                        {rule.rule_type === 'dlp' && rule.detector ? ` · ${rule.detector}` : ''}
                      </div>
                    </td>
                    <td style={tdStyle}>{rule.action}</td>
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
      </div>

      <div style={{ ...panelStyle, marginTop: 16 }}>
        <div style={sectionLabel}>POLICY PREVIEW</div>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))', gap: 12 }}>
          <label style={labelStyle}>
            Tenant Override
            <input
              value={previewRequest.tenant_id}
              onChange={e => setPreviewRequest(current => ({ ...current, tenant_id: e.target.value }))}
              style={inputStyle}
              placeholder="leave blank for global"
            />
          </label>
          <label style={labelStyle}>
            Provider
            <input
              value={previewRequest.provider}
              onChange={e => setPreviewRequest(current => ({ ...current, provider: e.target.value }))}
              style={inputStyle}
            />
          </label>
          <label style={labelStyle}>
            Model
            <input
              value={previewRequest.model}
              onChange={e => setPreviewRequest(current => ({ ...current, model: e.target.value }))}
              style={inputStyle}
            />
          </label>
          <label style={labelStyle}>
            Environment
            <input
              value={previewRequest.environment}
              onChange={e => setPreviewRequest(current => ({ ...current, environment: e.target.value }))}
              style={inputStyle}
            />
          </label>
          <label style={labelStyle}>
            Estimated Tokens
            <input
              type="number"
              min={0}
              value={previewRequest.estimated_tokens}
              onChange={e => setPreviewRequest(current => ({ ...current, estimated_tokens: Number(e.target.value) }))}
              style={inputStyle}
            />
          </label>
        </div>
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12, marginTop: 12 }}>
          <label style={labelStyle}>
            Request Body
            <textarea
              value={previewRequest.request_body}
              onChange={e => setPreviewRequest(current => ({ ...current, request_body: e.target.value }))}
              style={textareaStyle}
              rows={4}
            />
          </label>
          <label style={labelStyle}>
            Response Body
            <textarea
              value={previewRequest.response_body}
              onChange={e => setPreviewRequest(current => ({ ...current, response_body: e.target.value }))}
              style={textareaStyle}
              rows={4}
            />
          </label>
        </div>
        <div style={{ display: 'flex', gap: 8, marginTop: 16 }}>
          <button
            style={primaryBtn}
            onClick={() =>
              preview.mutate({
                tenant_id: previewRequest.tenant_id.trim() || undefined,
                provider: previewRequest.provider.trim().toLowerCase(),
                model: previewRequest.model.trim().toLowerCase(),
                environment: previewRequest.environment.trim().toLowerCase(),
                estimated_tokens: previewRequest.estimated_tokens,
                request_body: previewRequest.request_body,
                response_body: previewRequest.response_body,
              })
            }
            disabled={preview.isPending}
          >
            {preview.isPending ? 'Previewing...' : 'Preview Policy Match'}
          </button>
        </div>
        {preview.isError && <div style={errorStyle}>Failed to preview policy decision.</div>}
        {preview.data && (
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: 12, marginTop: 16 }}>
            {[
              { label: 'Traffic', decision: preview.data.traffic },
              { label: 'Request DLP', decision: preview.data.request_dlp },
              { label: 'Response DLP', decision: preview.data.response_dlp },
            ].map(({ label, decision }) => {
              return (
                <div key={label} style={{ border: '1px solid #0F1F35', borderRadius: 8, padding: 12, background: '#071525' }}>
                  <div style={{ color: '#E2E8F0', fontSize: 12, fontWeight: 600 }}>{label}</div>
                  <div style={{ color: decision.matched ? '#10B981' : '#64748B', fontSize: 11, marginTop: 6 }}>
                    {decision.matched ? `${decision.action || 'matched'}${decision.policy_name ? ` via ${decision.policy_name}` : ''}` : 'no matching rule'}
                  </div>
                  {decision.reason && <div style={{ color: '#94A3B8', fontSize: 11, marginTop: 6 }}>{decision.reason}</div>}
                  {decision.matched_names && decision.matched_names.length > 0 && (
                    <div style={{ color: '#64748B', fontSize: 10, marginTop: 6 }}>detectors: {decision.matched_names.join(', ')}</div>
                  )}
                  {decision.redacted_preview && (
                    <div style={{ color: '#CBD5E1', fontSize: 10, marginTop: 8, whiteSpace: 'pre-wrap' }}>{decision.redacted_preview}</div>
                  )}
                </div>
              )
            })}
          </div>
        )}
      </div>

      <div style={{ ...panelStyle, marginTop: 16 }}>
        <div style={sectionLabel}>RECENT CONTROL-PLANE AUDIT</div>
        <div style={{ display: 'grid', gap: 10 }}>
          {(audit?.items ?? []).slice(0, 12).map(entry => (
            <div key={entry.id} style={{ border: '1px solid #0F1F35', borderRadius: 8, padding: 12, background: '#071525' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8 }}>
                <div style={{ color: '#E2E8F0', fontSize: 12, fontWeight: 600 }}>{entry.category.toUpperCase()} · {entry.action.toUpperCase()}</div>
                <div style={{ color: '#475569', fontSize: 10 }}>{new Date(entry.created_at).toLocaleString()}</div>
              </div>
              <div style={{ color: '#64748B', fontSize: 11, marginTop: 4 }}>
                {(entry.actor || 'system')} · {entry.target_type} {entry.target_id || '—'} · {entry.outcome}
              </div>
            </div>
          ))}
          {(audit?.items?.length ?? 0) === 0 && <div style={subtleText}>No control-plane audit entries yet.</div>}
        </div>
      </div>
    </div>
  )
}

const panelStyle: CSSProperties = {
  background: '#0D1B2A',
  border: '1px solid #0F1F35',
  borderRadius: 10,
  padding: 24,
}

const titleStyle: CSSProperties = {
  fontSize: 22,
  fontWeight: 700,
  color: '#F0F9FF',
  margin: 0,
}

const subtleText: CSSProperties = {
  fontSize: 12,
  color: '#475569',
  marginTop: 8,
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

const errorStyle: CSSProperties = {
  fontSize: 11,
  color: '#EF4444',
  marginTop: 10,
}
