import { useState, type CSSProperties } from 'react'
import { useCollectors, useEnvironments, CollectorInfo, EnvironmentInfo } from '../hooks/api'

const ENV_COLORS: Record<string, string> = {
  production: 'var(--protect)',
  staging:    'var(--prove)',
  development:'var(--spend)',
}

const COLLECTOR_STATUS_COLORS: Record<string, string> = {
  healthy:     'var(--spend)',
  degraded:    'var(--prove)',
  unreachable: 'var(--protect)',
}

const STATIC_COLLECTORS: CollectorInfo[] = [
  { id: 'grpc', name: 'gRPC OTLP Receiver', endpoint_grpc: 'localhost:4317', endpoint_http: '', status: 'healthy', version: 'v1' },
  { id: 'http', name: 'HTTP OTLP Receiver', endpoint_grpc: '', endpoint_http: 'localhost:4318', status: 'healthy', version: 'v1' },
  { id: 'gateway', name: 'API Gateway', endpoint_grpc: '', endpoint_http: 'localhost:8080', status: 'healthy', version: 'v1' },
]

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false)
  async function handleCopy() {
    try { await navigator.clipboard.writeText(text); setCopied(true); setTimeout(() => setCopied(false), 1800) } catch { /* silent */ }
  }
  return (
    <button onClick={handleCopy} title="Copy to clipboard"
      style={{ background: 'none', border: '1px solid var(--layer-border)', borderRadius: 4, padding: '2px 8px', cursor: 'pointer', fontSize: 9, color: copied ? 'var(--spend)' : 'var(--text-tertiary)', transition: 'color 0.2s', flexShrink: 0 }}>
      {copied ? 'copied ✓' : 'copy'}
    </button>
  )
}

function CollectorCard({ collector }: { collector: CollectorInfo }) {
  const statusColor = COLLECTOR_STATUS_COLORS[collector.status] ?? 'var(--text-tertiary)'
  return (
    <div style={{ background: 'var(--layer-0)', border: `1px solid color-mix(in srgb, ${statusColor} 25%, transparent)`, borderLeft: `3px solid ${statusColor}`, borderRadius: 10, padding: 16 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 12 }}>
        <div style={{ width: 8, height: 8, borderRadius: '50%', background: statusColor, boxShadow: `0 0 8px ${statusColor}`, flexShrink: 0 }} />
        <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--text-primary)', flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{collector.name}</span>
        <span style={{ fontSize: 9, color: statusColor, padding: '1px 6px', borderRadius: 3, border: `1px solid color-mix(in srgb, ${statusColor} 30%, transparent)` }}>{collector.status}</span>
      </div>
      {collector.endpoint_grpc && (
        <div style={{ marginBottom: 8 }}>
          <div style={{ fontSize: 9, color: 'var(--text-tertiary)', letterSpacing: '0.1em', marginBottom: 3 }}>gRPC</div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
            <code style={{ fontSize: 12, fontWeight: 700, color: 'var(--control)', fontFamily: 'monospace', flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{collector.endpoint_grpc}</code>
            <CopyButton text={collector.endpoint_grpc} />
          </div>
        </div>
      )}
      {collector.endpoint_http && (
        <div style={{ marginBottom: 8 }}>
          <div style={{ fontSize: 9, color: 'var(--text-tertiary)', letterSpacing: '0.1em', marginBottom: 3 }}>HTTP</div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
            <code style={{ fontSize: 12, fontWeight: 700, color: 'var(--spend)', fontFamily: 'monospace', flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{collector.endpoint_http}</code>
            <CopyButton text={collector.endpoint_http} />
          </div>
        </div>
      )}
      <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: 10, paddingTop: 8, borderTop: '1px solid var(--layer-border)' }}>
        <span style={{ fontSize: 9, color: 'var(--text-tertiary)' }}>{collector.version ? `v${collector.version.replace(/^v/, '')}` : ''}</span>
        {collector.last_checked && <span style={{ fontSize: 9, color: 'var(--text-tertiary)' }}>Checked {new Date(collector.last_checked).toLocaleTimeString()}</span>}
      </div>
    </div>
  )
}

export default function EnvironmentsPage() {
  const { data: rawEnvs, isLoading: envsLoading } = useEnvironments()
  const { data: collectorsData, isLoading: collectorsLoading, isError: collectorsError } = useCollectors()

  const envs: EnvironmentInfo[] = (rawEnvs ?? []).map(e => {
    if (typeof e === 'string') return { name: e, status: 'active', span_count: 0 }
    return {
      ...e,
      name: e.name || e.id || 'unnamed',
      status: e.status || 'active',
      span_count: e.span_count ?? 0,
    }
  })
  const collectors: CollectorInfo[] = (collectorsData && collectorsData.length > 0) ? collectorsData : STATIC_COLLECTORS
  const usedFallback = !collectorsLoading && (collectorsError || !collectorsData || collectorsData.length === 0)

  return (
    <div style={{ padding: '40px 48px', maxWidth: 1440, margin: '0 auto', display: 'flex', flexDirection: 'column', gap: 24 }}>
      <div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
          <span style={{ width: 8, height: 8, borderRadius: '50%', background: 'var(--control)', display: 'inline-block' }} />
          <span style={{ fontSize: 10, fontWeight: 700, color: 'var(--control)', letterSpacing: '0.1em' }}>CONTROL</span>
        </div>
        <h1 style={titleStyle}>Environments</h1>
        <p style={subtleText}>Collector fleet status and environment health overview.</p>
      </div>

      {/* Collector endpoints */}
      <div style={panelStyle}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 16 }}>
          <div style={sectionLabel}>COLLECTOR ENDPOINTS</div>
          {usedFallback && <span style={{ fontSize: 10, color: 'var(--text-tertiary)', padding: '2px 8px', border: '1px solid var(--layer-border)', borderRadius: 4 }}>static config — live API not available</span>}
          {collectorsLoading && <span style={{ fontSize: 11, color: 'var(--text-tertiary)' }}>Checking collectors…</span>}
        </div>
        {collectorsLoading ? (
          <div style={{ color: 'var(--text-tertiary)', fontSize: 12, padding: '24px 0', textAlign: 'center' }}>Loading collectors…</div>
        ) : (
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(220px, 1fr))', gap: 14 }}>
            {collectors.map(c => <CollectorCard key={c.id} collector={c} />)}
          </div>
        )}
      </div>

      {/* Quick start */}
      <div style={panelStyle}>
        <div style={sectionLabel}>QUICK START — SEND A TEST SPAN</div>
        <pre style={{ margin: 0, background: 'var(--layer-0)', border: '1px solid var(--layer-border)', borderRadius: 8, padding: 16, fontSize: 11, color: 'var(--text-secondary)', overflowX: 'auto', fontFamily: 'monospace', lineHeight: 1.7 }}>{`# Python — instrument any agent framework
pip install govagn

from govagn import instrument
instrument(endpoint="http://localhost:4317", service_name="my-agent")

# Your existing CrewAI / LangGraph / OpenAI / Claude code is now traced

# Or send a raw OTLP span via HTTP:
curl -X POST http://localhost:4318/v1/traces \\
  -H "Content-Type: application/json" \\
  -d '{"resourceSpans":[]}'`}</pre>
      </div>

      {/* Environments list */}
      <div style={panelStyle}>
        <div style={sectionLabel}>CONFIGURED ENVIRONMENTS</div>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 16 }}>
          {envsLoading && <div style={{ color: 'var(--text-tertiary)', fontSize: 12 }}>Loading…</div>}
          {envs.map(env => {
            const envName = env.name || 'unnamed'
            const color = ENV_COLORS[envName.toLowerCase()] ?? 'var(--text-tertiary)'
            const spanCount = env.span_count ?? 0
            return (
              <div key={env.id || envName} style={{ background: 'var(--layer-0)', border: `1px solid color-mix(in srgb, ${color} 25%, transparent)`, borderLeft: `3px solid ${color}`, borderRadius: 10, padding: 20 }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 14 }}>
                  <div style={{ width: 8, height: 8, borderRadius: '50%', background: color, boxShadow: `0 0 8px ${color}` }} />
                  <span style={{ fontSize: 14, fontWeight: 600, color: 'var(--text-primary)' }}>
                    {envName.charAt(0).toUpperCase() + envName.slice(1)}
                  </span>
                </div>
                <div style={{ fontSize: 12, color: 'var(--text-tertiary)', display: 'grid', gap: 6 }}>
                  <div>Spans: <span style={{ color: 'var(--text-secondary)' }}>{spanCount.toLocaleString()}</span></div>
                  <div>Status: <span style={{ color: env.status === 'active' ? 'var(--spend)' : 'var(--text-tertiary)' }}>{env.status}</span></div>
                  {env.description && <div>Target: <span style={{ color: 'var(--text-secondary)' }}>{env.description}</span></div>}
                  <div>Auth: <span style={{ color: envName.toLowerCase() === 'production' ? 'var(--protect)' : 'var(--spend)' }}>
                    {envName.toLowerCase() === 'production' ? 'mTLS enabled' : 'dev mode'}
                  </span></div>
                </div>
              </div>
            )
          })}
          {!envsLoading && envs.length === 0 && (
            <div style={{ color: 'var(--text-tertiary)', fontSize: 12, gridColumn: '1/-1', textAlign: 'center', padding: 32 }}>
              No environments found. Send spans to register an environment.
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

const titleStyle: CSSProperties = { fontSize: 28, fontWeight: 700, color: 'var(--text-primary)', margin: 0, letterSpacing: '-0.02em' }
const subtleText: CSSProperties = { fontSize: 12, color: 'var(--text-tertiary)', marginTop: 4 }
const panelStyle: CSSProperties = { background: 'var(--layer-2)', border: '1px solid var(--layer-border)', borderRadius: 12, padding: 24 }
const sectionLabel: CSSProperties = { fontSize: 10, color: 'var(--text-tertiary)', letterSpacing: '0.12em', marginBottom: 16, fontWeight: 700 }
