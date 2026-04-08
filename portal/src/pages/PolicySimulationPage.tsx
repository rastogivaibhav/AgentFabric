import { useState, type CSSProperties } from 'react'
import { useSimulatePolicyRule, type PolicySimulationSample } from '../hooks/api'
import PolicyDecisionExplorer from './PolicyDecisionExplorer'

const emptySample: PolicySimulationSample = {
  label: 'candidate scenario',
  provider: 'openai',
  model: 'gpt-4o',
  environment: 'production',
  estimated_tokens: 256,
  actor: '',
  app: '',
  session: '',
  request_body: '',
  response_body: '',
}

export default function PolicySimulationPage() {
  const simulate = useSimulatePolicyRule()
  const [sample, setSample] = useState<PolicySimulationSample>(emptySample)

  return (
    <div style={{ padding: '40px 48px', maxWidth: 1440, margin: '0 auto' }}>
      <div style={{ marginBottom: 28 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
          <span style={{ width: 8, height: 8, borderRadius: '50%', background: 'var(--protect)', display: 'inline-block' }} />
          <span style={{ fontSize: 10, fontWeight: 700, color: 'var(--protect)', letterSpacing: '0.1em' }}>PROTECT</span>
        </div>
        <h1 style={{ fontSize: 28, fontWeight: 700, color: 'var(--text-primary)', margin: 0, letterSpacing: '-0.02em' }}>Policy Simulation</h1>
        <p style={{ fontSize: 12, color: 'var(--text-tertiary)', marginTop: 6 }}>
          Test how your active policies respond to specific request/response scenarios — without real traffic.
        </p>
      </div>

      <div style={panelStyle}>
        <div style={sectionLabel}>SIMULATION PARAMETERS</div>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))', gap: 12 }}>
          {[
            { label: 'Label', key: 'label', placeholder: 'candidate scenario' },
            { label: 'Provider', key: 'provider', placeholder: 'openai' },
            { label: 'Model', key: 'model', placeholder: 'gpt-4o' },
            { label: 'Environment', key: 'environment', placeholder: 'production' },
            { label: 'App', key: 'app', placeholder: 'my-agent' },
          ].map(f => (
            <label key={f.key} style={labelStyle}>
              {f.label}
              <input id={`sim-${f.key}`} value={(sample as any)[f.key] ?? ''} placeholder={f.placeholder}
                onChange={e => setSample(c => ({ ...c, [f.key]: e.target.value }))} style={inputStyle} />
            </label>
          ))}
          <label style={labelStyle}>
            Estimated Tokens
            <input id="sim-tokens" type="number" min={0} value={sample.estimated_tokens ?? 0}
              onChange={e => setSample(c => ({ ...c, estimated_tokens: Number(e.target.value) }))} style={inputStyle} />
          </label>
        </div>

        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12, marginTop: 14 }}>
          <label style={labelStyle}>
            Request Body
            <textarea id="sim-req" value={sample.request_body ?? ''} onChange={e => setSample(c => ({ ...c, request_body: e.target.value }))}
              style={{ ...inputStyle, resize: 'vertical' } as CSSProperties} rows={4} placeholder='{"messages":[{"role":"user","content":"Hello!"}]}' />
          </label>
          <label style={labelStyle}>
            Response Body
            <textarea id="sim-res" value={sample.response_body ?? ''} onChange={e => setSample(c => ({ ...c, response_body: e.target.value }))}
              style={{ ...inputStyle, resize: 'vertical' } as CSSProperties} rows={4} placeholder='{"choices":[{"message":{"content":"Hi!"}]}' />
          </label>
        </div>

        <div style={{ display: 'flex', gap: 10, marginTop: 20 }}>
          <button id="sim-run-btn" style={primaryBtn}
            onClick={() => simulate.mutate([{ ...sample, provider: sample.provider.trim().toLowerCase(), model: sample.model.trim().toLowerCase(), environment: sample.environment?.trim().toLowerCase() }])}
            disabled={simulate.isPending}>
            {simulate.isPending ? 'Simulating…' : '▶ Run Simulation'}
          </button>
          {simulate.isError && <span style={{ fontSize: 12, color: 'var(--protect)', alignSelf: 'center' }}>Failed to simulate policy set.</span>}
        </div>

        {simulate.data && simulate.data.results.length > 0 && (
          <div style={{ display: 'grid', gap: 12, marginTop: 24 }}>
            <div style={sectionLabel}>SIMULATION RESULTS</div>
            {simulate.data.results.map((result, index) => (
              <div key={`${result.label ?? 'sample'}-${index}`} style={{ border: '1px solid var(--layer-border)', borderRadius: 10, padding: 16, background: 'var(--layer-0)' }}>
                <div style={{ color: 'var(--text-primary)', fontWeight: 700, fontSize: 13, marginBottom: 14 }}>{result.label || `sample ${index + 1}`}</div>
                <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: 12 }}>
                  <PolicyDecisionExplorer label="Traffic" decision={result.traffic} />
                  <PolicyDecisionExplorer label="Request DLP" decision={result.request_dlp} />
                  <PolicyDecisionExplorer label="Response DLP" decision={result.response_dlp} />
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

const panelStyle: CSSProperties = { background: 'var(--layer-2)', border: '1px solid var(--layer-border)', borderRadius: 12, padding: 24 }
const sectionLabel: CSSProperties = { fontSize: 10, color: 'var(--text-tertiary)', letterSpacing: '0.12em', marginBottom: 16, fontWeight: 700 }
const labelStyle: CSSProperties = { fontSize: 11, color: 'var(--text-secondary)', display: 'flex', flexDirection: 'column', gap: 5 }
const inputStyle: CSSProperties = { display: 'block', width: '100%', marginTop: 4, boxSizing: 'border-box', background: 'var(--layer-1)', border: '1px solid var(--layer-border)', borderRadius: 8, color: 'var(--text-primary)', padding: '9px 12px', fontSize: 12, outline: 'none' }
const primaryBtn: CSSProperties = { background: 'var(--protect)', border: 'none', borderRadius: 8, color: '#fff', padding: '10px 20px', fontSize: 12, cursor: 'pointer', fontWeight: 700 }
