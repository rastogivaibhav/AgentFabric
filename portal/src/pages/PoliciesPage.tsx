import { useMemo, useState, type CSSProperties } from 'react'
import RecommendationFeed from '../components/recommendations/RecommendationFeed'
import { hasRole, useAuth } from '../hooks/auth'
import {
  type PolicyRule,
  useControlAudit,
  useDeletePolicyRule,
  useRecommendations,
  usePreviewRollout,
  usePreviewPolicyRule,
  usePolicyRules,
  useRollouts,
  useUpdateRecommendationStatus,
  useUpdateRolloutStatus,
  useUpsertRolloutRule,
  useUpsertPolicyRule,
} from '../hooks/api'
import PolicyDecisionExplorer from './PolicyDecisionExplorer'
import PolicySimulationPage from './PolicySimulationPage'

const emptyRule: PolicyRule = {
  name: '',
  rule_type: 'traffic',
  decision_mode: 'fast',
  enabled: true,
  priority: 100,
  action: 'deny',
  provider: '',
  model_pattern: '',
  environment: '',
  max_tokens: 0,
  detector: '',
  scope: 'both',
  guardrails: [],
  schema_json: '',
  unsafe_categories: [],
  rollout_percent: 100,
  version: 1,
  rule_conditions: {},
  rego_module: '',
  tenant_id: null,
  description: '',
}

export default function PoliciesPage() {
  const { user } = useAuth()
  const isAdmin = hasRole(user, ['admin'])
  const { data, isLoading, error } = usePolicyRules()
  const { data: rolloutData } = useRollouts()
  const { data: audit } = useControlAudit(25)
  const { data: recommendationData, isLoading: recommendationsLoading, error: recommendationsError } = useRecommendations({ since: '24h', limit: 6 })
  const upsert = useUpsertPolicyRule()
  const remove = useDeletePolicyRule()
  const preview = usePreviewPolicyRule()
  const upsertRollout = useUpsertRolloutRule()
  const previewRollout = usePreviewRollout()
  const updateRolloutStatus = useUpdateRolloutStatus()
  const updateRecommendationStatus = useUpdateRecommendationStatus()
  const [form, setForm] = useState<PolicyRule>(emptyRule)
  const [previewRequest, setPreviewRequest] = useState({
    tenant_id: '',
    provider: 'openai',
    model: 'gpt-4o',
    environment: 'production',
    estimated_tokens: 128,
    actor: '',
    app: '',
    session: '',
    request_body: '',
    response_body: '',
  })
  const [rolloutForm, setRolloutForm] = useState({
    id: 0,
    name: 'Policy canary',
    policy_rule_id: 0,
    percentage: 10,
    environment: 'production',
  })

  const rules = useMemo(() => data?.items ?? [], [data])
  const policyRollouts = useMemo(
    () => (rolloutData?.items ?? []).filter(item => item.target_type === 'policy_rule'),
    [rolloutData],
  )
  const policyRecommendations = useMemo(
    () => (recommendationData?.items ?? []).filter(item => item.type === 'policy' || item.type === 'cost'),
    [recommendationData],
  )

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
        decision_mode: form.decision_mode ?? 'fast',
        guardrails: (form.guardrails ?? []).map(value => value.trim().toLowerCase()).filter(Boolean),
        schema_json: form.schema_json ?? '',
        unsafe_categories: (form.unsafe_categories ?? []).map(value => value.trim().toLowerCase()).filter(Boolean),
        rollout_percent: form.rollout_percent ?? 100,
        version: form.version ?? 1,
        rule_conditions: form.rule_conditions ?? {},
        rego_module: form.rego_module ?? '',
      },
      { onSuccess: () => setForm(emptyRule) },
    )
  }

  return (
    <div style={{ padding: 32 }}>
      <div style={{ marginBottom: 24 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 8, color: 'var(--protect)', fontWeight: 600, fontSize: 10, letterSpacing: '0.1em' }}>
          <span style={{ width: 8, height: 8, borderRadius: '50%', background: 'var(--protect)' }} />
          PROTECT
        </div>
        <h1 style={titleStyle}>Policies</h1>
        <p style={subtleText}>
          Manage live traffic enforcement, built-in guardrails, rollout controls, and DLP rules for proxy and netproxy.
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
            <label style={labelStyle}>
              Decision Mode
              <select value={form.decision_mode ?? 'fast'} onChange={e => setForm(f => ({ ...f, decision_mode: e.target.value as PolicyRule['decision_mode'] }))} style={inputStyle}>
                <option value="fast">fast</option>
                <option value="rego">rego</option>
              </select>
            </label>
            <label style={labelStyle}>
              Guardrails (comma separated)
              <input
                value={(form.guardrails ?? []).join(', ')}
                onChange={e => setForm(f => ({ ...f, guardrails: e.target.value.split(',').map(value => value.trim()).filter(Boolean) }))}
                style={inputStyle}
                placeholder="schema, prompt_injection, unsafe_content"
              />
            </label>
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
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
              <label style={labelStyle}>
                Rule Version
                <input type="number" min={1} value={form.version ?? 1} onChange={e => setForm(f => ({ ...f, version: Number(e.target.value) }))} style={inputStyle} />
              </label>
              <label style={labelStyle}>
                Rollout Percent
                <input type="number" min={1} max={100} value={form.rollout_percent ?? 100} onChange={e => setForm(f => ({ ...f, rollout_percent: Number(e.target.value) }))} style={inputStyle} />
              </label>
            </div>
            <label style={labelStyle}>
              Provider
              <input value={form.provider ?? ''} onChange={e => setForm(f => ({ ...f, provider: e.target.value }))} style={inputStyle} placeholder="openai, anthropic, google, vertexai, bedrock, or *" />
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
              Schema JSON
              <textarea value={form.schema_json ?? ''} onChange={e => setForm(f => ({ ...f, schema_json: e.target.value }))} style={textareaStyle} rows={4} placeholder='{"required":["messages"],"types":{"messages":"array"}}' />
            </label>
            <label style={labelStyle}>
              Unsafe Categories (comma separated)
              <input value={(form.unsafe_categories ?? []).join(', ')} onChange={e => setForm(f => ({ ...f, unsafe_categories: e.target.value.split(',').map(value => value.trim()).filter(Boolean) }))} style={inputStyle} placeholder="violence, self_harm, hate, sexual, malware" />
            </label>
            <label style={labelStyle}>
              Description
              <textarea value={form.description ?? ''} onChange={e => setForm(f => ({ ...f, description: e.target.value }))} style={textareaStyle} rows={3} />
            </label>
            <label style={labelStyle}>
              Rule Conditions JSON
              <textarea
                value={JSON.stringify(form.rule_conditions ?? {}, null, 2)}
                onChange={e => {
                  try {
                    const parsed = JSON.parse(e.target.value || '{}') as Record<string, string>
                    setForm(f => ({ ...f, rule_conditions: parsed }))
                  } catch {}
                }}
                style={textareaStyle}
                rows={4}
                placeholder='{"app":"ops-ui","header:x-af-source":"plugin"}'
              />
            </label>
            {form.decision_mode === 'rego' && (
              <label style={labelStyle}>
                Rego-Style Module
                <textarea
                  value={form.rego_module ?? ''}
                  onChange={e => setForm(f => ({ ...f, rego_module: e.target.value }))}
                  style={textareaStyle}
                  rows={4}
                  placeholder={'deny if input.environment == "production" && input.estimated_tokens > 2000'}
                />
              </label>
            )}
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
                <tr style={{ textAlign: 'left', color: 'var(--text-tertiary)', borderBottom: '1px solid var(--layer-border)' }}>
                  {['Name', 'Type', 'Match', 'Action', 'Controls'].map(h => <th key={h} style={thStyle}>{h}</th>)}
                </tr>
              </thead>
              <tbody>
                {rules.map(rule => (
                  <tr key={rule.id} style={{ borderBottom: '1px solid var(--layer-border)', opacity: rule.enabled === false ? 0.6 : 1 }}>
                    <td style={tdStyle}>
                      <div>{rule.name}</div>
                      <div style={{ fontSize: 10, color: 'var(--text-tertiary)' }}>priority {rule.priority ?? 100}</div>
                    </td>
                    <td style={tdStyle}>{rule.rule_type}</td>
                    <td style={tdStyle}>
                      <div>{rule.provider || '*'}/{rule.model_pattern || '*'}</div>
                      <div style={{ fontSize: 10, color: 'var(--text-tertiary)' }}>
                        env {rule.environment || '*'} | mode {rule.decision_mode || 'fast'}
                        {rule.version ? ` | v${rule.version}` : ''}
                        {rule.rollout_percent ? ` | rollout ${rule.rollout_percent}%` : ''}
                        {rule.rule_type === 'traffic' && rule.max_tokens ? ` | max ${rule.max_tokens}` : ''}
                        {rule.rule_type === 'dlp' && rule.detector ? ` | ${rule.detector}` : ''}
                        {rule.guardrails && rule.guardrails.length > 0 ? ` | guards ${rule.guardrails.join(',')}` : ''}
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
            <input value={previewRequest.tenant_id} onChange={e => setPreviewRequest(current => ({ ...current, tenant_id: e.target.value }))} style={inputStyle} placeholder="leave blank for global" />
          </label>
          <label style={labelStyle}>
            Provider
            <input value={previewRequest.provider} onChange={e => setPreviewRequest(current => ({ ...current, provider: e.target.value }))} style={inputStyle} />
          </label>
          <label style={labelStyle}>
            Model
            <input value={previewRequest.model} onChange={e => setPreviewRequest(current => ({ ...current, model: e.target.value }))} style={inputStyle} />
          </label>
          <label style={labelStyle}>
            Environment
            <input value={previewRequest.environment} onChange={e => setPreviewRequest(current => ({ ...current, environment: e.target.value }))} style={inputStyle} />
          </label>
          <label style={labelStyle}>
            Estimated Tokens
            <input type="number" min={0} value={previewRequest.estimated_tokens} onChange={e => setPreviewRequest(current => ({ ...current, estimated_tokens: Number(e.target.value) }))} style={inputStyle} />
          </label>
          <label style={labelStyle}>
            App
            <input value={previewRequest.app} onChange={e => setPreviewRequest(current => ({ ...current, app: e.target.value }))} style={inputStyle} />
          </label>
        </div>
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12, marginTop: 12 }}>
          <label style={labelStyle}>
            Request Body
            <textarea value={previewRequest.request_body} onChange={e => setPreviewRequest(current => ({ ...current, request_body: e.target.value }))} style={textareaStyle} rows={4} />
          </label>
          <label style={labelStyle}>
            Response Body
            <textarea value={previewRequest.response_body} onChange={e => setPreviewRequest(current => ({ ...current, response_body: e.target.value }))} style={textareaStyle} rows={4} />
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
                actor: previewRequest.actor,
                app: previewRequest.app,
                session: previewRequest.session,
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
            <PolicyDecisionExplorer label="Traffic" decision={preview.data.traffic} />
            <PolicyDecisionExplorer label="Request DLP" decision={preview.data.request_dlp} />
            <PolicyDecisionExplorer label="Response DLP" decision={preview.data.response_dlp} />
          </div>
        )}
      </div>

      <div style={{ marginTop: 16 }}>
        <PolicySimulationPage />
      </div>

      <div style={{ marginTop: 16 }}>
        <RecommendationFeed
          title="POLICY RECOMMENDATIONS"
          recommendations={policyRecommendations}
          isLoading={recommendationsLoading}
          error={recommendationsError}
          emptyMessage="No active policy or budget recommendations."
          onUpdateStatus={(id, status) => updateRecommendationStatus.mutate({ id, status })}
        />
      </div>

      <div style={{ ...panelStyle, marginTop: 16 }}>
        <div style={sectionLabel}>RECENT CONTROL-PLANE AUDIT</div>
        <div style={{ display: 'grid', gap: 10 }}>
          {(audit?.items ?? []).slice(0, 12).map(entry => (
            <div key={entry.id} style={{ border: '1px solid var(--layer-border)', borderRadius: 8, padding: 12, background: 'var(--layer-1)' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8 }}>
                <div style={{ color: 'var(--text-primary)', fontSize: 12, fontWeight: 600 }}>{entry.category.toUpperCase()} | {entry.action.toUpperCase()}</div>
                <div style={{ color: 'var(--text-tertiary)', fontSize: 10 }}>{new Date(entry.created_at).toLocaleString()}</div>
              </div>
              <div style={{ color: 'var(--text-tertiary)', fontSize: 11, marginTop: 4 }}>
                {(entry.actor || 'system')} | {entry.target_type} {entry.target_id || 'n/a'} | {entry.outcome}
              </div>
            </div>
          ))}
          {(audit?.items?.length ?? 0) === 0 && <div style={subtleText}>No control-plane audit entries yet.</div>}
        </div>
      </div>

      <div style={{ ...panelStyle, marginTop: 16 }}>
        <div style={sectionLabel}>ROLLOUT CONTROL</div>
        <div style={{ display: 'grid', gridTemplateColumns: 'minmax(320px, 420px) 1fr', gap: 16 }}>
          <div>
            <div style={{ display: 'grid', gap: 12 }}>
              <label style={labelStyle}>
                Rule Name
                <input value={rolloutForm.name} onChange={e => setRolloutForm(current => ({ ...current, name: e.target.value }))} style={inputStyle} />
              </label>
              <label style={labelStyle}>
                Policy Rule
                <select value={rolloutForm.policy_rule_id} onChange={e => setRolloutForm(current => ({ ...current, policy_rule_id: Number(e.target.value) }))} style={inputStyle}>
                  <option value={0}>Select policy rule</option>
                  {rules.map(rule => (
                    <option key={rule.id} value={rule.id}>{rule.name}</option>
                  ))}
                </select>
              </label>
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
                <label style={labelStyle}>
                  Environment
                  <input value={rolloutForm.environment} onChange={e => setRolloutForm(current => ({ ...current, environment: e.target.value }))} style={inputStyle} />
                </label>
                <label style={labelStyle}>
                  Canary Percent
                  <input type="number" min={1} max={100} value={rolloutForm.percentage} onChange={e => setRolloutForm(current => ({ ...current, percentage: Number(e.target.value) }))} style={inputStyle} />
                </label>
              </div>
            </div>
            <div style={{ display: 'flex', gap: 8, marginTop: 16 }}>
              <button
                style={primaryBtn}
                onClick={() => upsertRollout.mutate({
                  id: rolloutForm.id || undefined,
                  name: rolloutForm.name.trim(),
                  target_type: 'policy_rule',
                  target_id: String(rolloutForm.policy_rule_id || ''),
                  policy_rule_id: rolloutForm.policy_rule_id,
                  environment: rolloutForm.environment.trim().toLowerCase(),
                  percentage: rolloutForm.percentage,
                  status: 'active',
                  conditions: {},
                  rollback_criteria: { min_requests: '10', max_error_rate_pct: '50' },
                })}
                disabled={upsertRollout.isPending || rolloutForm.policy_rule_id <= 0}
              >
                {upsertRollout.isPending ? 'Saving...' : 'Save Rollout'}
              </button>
              <button
                style={secondaryBtn}
                onClick={() => previewRollout.mutate({
                  environment: rolloutForm.environment.trim().toLowerCase(),
                  policy_rule_id: rolloutForm.policy_rule_id,
                  provider: previewRequest.provider,
                  model: previewRequest.model,
                  app: previewRequest.app,
                  session: previewRequest.session,
                })}
                disabled={previewRollout.isPending || rolloutForm.policy_rule_id <= 0}
              >
                {previewRollout.isPending ? 'Previewing...' : 'Preview Rollout'}
              </button>
            </div>
            {previewRollout.data?.assignment?.rule_id && (
              <div style={{ marginTop: 12, padding: 12, borderRadius: 8, border: '1px solid #10243D', background: '#081221', color: 'var(--text-secondary)', fontSize: 11 }}>
                Preview selected {previewRollout.data.assignment.variant} for rule {previewRollout.data.assignment.rule_name} at bucket {previewRollout.data.assignment.bucket}.
              </div>
            )}
          </div>

          <div style={{ display: 'grid', gap: 10 }}>
            {policyRollouts.length === 0 ? (
              <div style={subtleText}>No rollout rules configured for policy canaries yet.</div>
            ) : policyRollouts.map(rule => (
              <div key={rule.id} style={{ border: '1px solid var(--layer-border)', borderRadius: 8, background: 'var(--layer-1)', padding: 12 }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12 }}>
                  <div style={{ color: 'var(--text-primary)', fontSize: 12, fontWeight: 600 }}>{rule.name}</div>
                  <div style={{ color: rule.status === 'paused' ? 'var(--protect)' : 'var(--spend)', fontSize: 11 }}>{rule.status}</div>
                </div>
                <div style={{ color: 'var(--text-secondary)', fontSize: 11, marginTop: 4 }}>
                  policy #{rule.policy_rule_id} | {rule.percentage}% | env {rule.environment || '*'}
                </div>
                <div style={{ color: 'var(--text-tertiary)', fontSize: 10, marginTop: 4 }}>
                  {rule.recent_requests ?? 0} requests | {(Number(rule.recent_error_rate ?? 0) * 100).toFixed(1)}% error rate
                </div>
                <div style={{ display: 'flex', gap: 8, marginTop: 8 }}>
                  <button style={secondaryBtnSmall} onClick={() => setRolloutForm({
                    id: Number(rule.id ?? 0),
                    name: rule.name,
                    policy_rule_id: Number(rule.policy_rule_id ?? 0),
                    percentage: rule.percentage,
                    environment: rule.environment ?? 'production',
                  })}>
                    Edit
                  </button>
                  <button
                    style={secondaryBtnSmall}
                    onClick={() => rule.id && updateRolloutStatus.mutate({ id: rule.id, status: rule.status === 'paused' ? 'active' : 'paused' })}
                  >
                    {rule.status === 'paused' ? 'Resume' : 'Pause'}
                  </button>
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}

const panelStyle: CSSProperties = {
  background: 'var(--layer-2)',
  border: '1px solid var(--layer-border)',
  borderRadius: 12,
  padding: 24,
}

const titleStyle: CSSProperties = {
  fontSize: 28,
  fontWeight: 700,
  color: 'var(--text-primary)',
  letterSpacing: '-0.02em',
  margin: 0,
}

const subtleText: CSSProperties = {
  fontSize: 12,
  color: 'var(--text-tertiary)',
  marginTop: 6,
  lineHeight: 1.5,
}

const sectionLabel: CSSProperties = {
  fontSize: 10,
  color: 'var(--text-tertiary)',
  letterSpacing: '0.12em',
  marginBottom: 16,
  fontWeight: 700,
}

const labelStyle: CSSProperties = {
  fontSize: 11,
  color: 'var(--text-secondary)',
  display: 'flex',
  flexDirection: 'column',
  gap: 6,
}

const inputStyle: CSSProperties = {
  display: 'block',
  width: '100%',
  boxSizing: 'border-box',
  background: 'var(--layer-1)',
  border: '1px solid var(--layer-border)',
  borderRadius: 8,
  color: 'var(--text-primary)',
  padding: '9px 12px',
  fontSize: 13,
  outline: 'none',
}

const textareaStyle: CSSProperties = {
  ...inputStyle,
  resize: 'vertical',
} as CSSProperties

const primaryBtn: CSSProperties = {
  background: 'var(--protect)',
  border: 'none',
  borderRadius: 8,
  color: '#fff',
  padding: '10px 18px',
  fontSize: 13,
  cursor: 'pointer',
  fontWeight: 700,
}

const secondaryBtn: CSSProperties = {
  background: 'var(--layer-3)',
  border: '1px solid var(--layer-border)',
  borderRadius: 8,
  color: 'var(--text-secondary)',
  padding: '9px 16px',
  fontSize: 13,
  cursor: 'pointer',
}

const secondaryBtnSmall: CSSProperties = {
  ...secondaryBtn,
  padding: '5px 12px',
  fontSize: 11,
}

const dangerBtnSmall: CSSProperties = {
  background: 'rgba(255,69,58,0.15)',
  border: '1px solid rgba(255,69,58,0.3)',
  borderRadius: 6,
  color: 'var(--protect)',
  padding: '5px 12px',
  fontSize: 11,
  cursor: 'pointer',
}

const thStyle: CSSProperties = {
  padding: '10px 14px',
  fontWeight: 700,
  fontSize: 9,
  letterSpacing: '0.08em',
  color: 'var(--text-tertiary)',
  borderBottom: '1px solid var(--layer-border)',
}

const tdStyle: CSSProperties = {
  padding: '12px 14px',
  color: 'var(--text-primary)',
  verticalAlign: 'top',
  fontSize: 12,
}

const errorStyle: CSSProperties = {
  fontSize: 12,
  color: 'var(--protect)',
  marginTop: 10,
}
