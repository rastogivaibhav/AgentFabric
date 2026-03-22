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
    <div style={panelStyle}>
      <div style={sectionLabel}>POLICY SIMULATION</div>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))', gap: 12 }}>
        <label style={labelStyle}>
          Label
          <input value={sample.label ?? ''} onChange={e => setSample(current => ({ ...current, label: e.target.value }))} style={inputStyle} />
        </label>
        <label style={labelStyle}>
          Provider
          <input value={sample.provider} onChange={e => setSample(current => ({ ...current, provider: e.target.value }))} style={inputStyle} />
        </label>
        <label style={labelStyle}>
          Model
          <input value={sample.model} onChange={e => setSample(current => ({ ...current, model: e.target.value }))} style={inputStyle} />
        </label>
        <label style={labelStyle}>
          Environment
          <input value={sample.environment ?? ''} onChange={e => setSample(current => ({ ...current, environment: e.target.value }))} style={inputStyle} />
        </label>
        <label style={labelStyle}>
          Estimated Tokens
          <input type="number" min={0} value={sample.estimated_tokens ?? 0} onChange={e => setSample(current => ({ ...current, estimated_tokens: Number(e.target.value) }))} style={inputStyle} />
        </label>
        <label style={labelStyle}>
          App
          <input value={sample.app ?? ''} onChange={e => setSample(current => ({ ...current, app: e.target.value }))} style={inputStyle} />
        </label>
      </div>
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12, marginTop: 12 }}>
        <label style={labelStyle}>
          Request Body
          <textarea value={sample.request_body ?? ''} onChange={e => setSample(current => ({ ...current, request_body: e.target.value }))} style={textareaStyle} rows={4} />
        </label>
        <label style={labelStyle}>
          Response Body
          <textarea value={sample.response_body ?? ''} onChange={e => setSample(current => ({ ...current, response_body: e.target.value }))} style={textareaStyle} rows={4} />
        </label>
      </div>
      <div style={{ display: 'flex', gap: 8, marginTop: 16 }}>
        <button
          style={primaryBtn}
          onClick={() => simulate.mutate([{ ...sample, provider: sample.provider.trim().toLowerCase(), model: sample.model.trim().toLowerCase(), environment: sample.environment?.trim().toLowerCase() }])}
          disabled={simulate.isPending}
        >
          {simulate.isPending ? 'Simulating...' : 'Run Simulation'}
        </button>
      </div>
      {simulate.isError && <div style={errorStyle}>Failed to simulate policy set.</div>}
      {simulate.data && simulate.data.results.length > 0 && (
        <div style={{ display: 'grid', gap: 10, marginTop: 16 }}>
          {simulate.data.results.map((result, index) => (
            <div key={`${result.label ?? 'sample'}-${index}`} style={{ border: '1px solid #0F1F35', borderRadius: 8, padding: 12 }}>
              <div style={{ color: '#E2E8F0', fontWeight: 700, fontSize: 12 }}>{result.label || `sample ${index + 1}`}</div>
              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: 12, marginTop: 12 }}>
                <PolicyDecisionExplorer label="Traffic" decision={result.traffic} />
                <PolicyDecisionExplorer label="Request DLP" decision={result.request_dlp} />
                <PolicyDecisionExplorer label="Response DLP" decision={result.response_dlp} />
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
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

const errorStyle: CSSProperties = {
  fontSize: 11,
  color: '#EF4444',
  marginTop: 10,
}
