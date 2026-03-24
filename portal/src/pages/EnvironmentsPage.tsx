import { useState } from 'react'
import { useCollectors, useEnvironments, CollectorInfo, EnvironmentInfo } from '../hooks/api'

const ENV_COLORS: Record<string, string> = {
  production: '#EF4444',
  staging: '#F59E0B',
  development: '#10B981',
}

// Collector status color
const COLLECTOR_STATUS_COLORS: Record<string, string> = {
  healthy: '#10B981',
  degraded: '#F59E0B',
  unreachable: '#EF4444',
}

// Static fallback collector cards shown when the API returns no data
const STATIC_COLLECTORS: CollectorInfo[] = [
  {
    id: 'grpc',
    name: 'gRPC OTLP Receiver',
    endpoint_grpc: 'localhost:4317',
    endpoint_http: '',
    status: 'healthy',
    version: 'v1',
  },
  {
    id: 'http',
    name: 'HTTP OTLP Receiver',
    endpoint_grpc: '',
    endpoint_http: 'localhost:4318',
    status: 'healthy',
    version: 'v1',
  },
  {
    id: 'gateway',
    name: 'API Gateway',
    endpoint_grpc: '',
    endpoint_http: 'localhost:8080',
    status: 'healthy',
    version: 'v1',
  },
]

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false)

  async function handleCopy() {
    try {
      await navigator.clipboard.writeText(text)
      setCopied(true)
      setTimeout(() => setCopied(false), 1800)
    } catch {
      // clipboard may not be available in all contexts — fail silently
    }
  }

  return (
    <button
      onClick={handleCopy}
      title="Copy to clipboard"
      style={{
        background: 'none',
        border: '1px solid #1E3A5F',
        borderRadius: 4,
        padding: '2px 7px',
        cursor: 'pointer',
        fontSize: 9,
        color: copied ? '#10B981' : '#475569',
        fontFamily: "'JetBrains Mono', monospace",
        transition: 'color 0.2s',
        flexShrink: 0,
      }}
    >
      {copied ? 'copied' : 'copy'}
    </button>
  )
}

function CollectorCard({ collector }: { collector: CollectorInfo }) {
  const statusColor = COLLECTOR_STATUS_COLORS[collector.status] ?? '#475569'
  const hasGrpc = !!collector.endpoint_grpc
  const hasHttp = !!collector.endpoint_http

  return (
    <div style={{
      background: '#060A14',
      border: `1px solid ${statusColor}30`,
      borderLeft: `3px solid ${statusColor}`,
      borderRadius: 8,
      padding: 16,
    }}>
      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 10 }}>
        <div style={{
          width: 8, height: 8, borderRadius: '50%',
          background: statusColor,
          boxShadow: `0 0 6px ${statusColor}`,
          flexShrink: 0,
        }} />
        <span style={{ fontSize: 13, fontWeight: 600, color: '#F0F9FF', flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
          {collector.name}
        </span>
        <span style={{ fontSize: 9, color: statusColor, padding: '1px 5px', borderRadius: 3, border: `1px solid ${statusColor}40`, flexShrink: 0 }}>
          {collector.status}
        </span>
      </div>

      {/* Endpoints */}
      {hasGrpc && (
        <div style={{ marginBottom: 8 }}>
          <div style={{ fontSize: 9, color: '#334155', letterSpacing: '0.1em', marginBottom: 3 }}>gRPC</div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
            <code style={{ fontSize: 12, fontWeight: 700, color: '#3B82F6', fontFamily: 'monospace', flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
              {collector.endpoint_grpc}
            </code>
            <CopyButton text={collector.endpoint_grpc} />
          </div>
        </div>
      )}
      {hasHttp && (
        <div style={{ marginBottom: 8 }}>
          <div style={{ fontSize: 9, color: '#334155', letterSpacing: '0.1em', marginBottom: 3 }}>HTTP</div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
            <code style={{ fontSize: 12, fontWeight: 700, color: '#10B981', fontFamily: 'monospace', flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
              {collector.endpoint_http}
            </code>
            <CopyButton text={collector.endpoint_http} />
          </div>
        </div>
      )}

      {/* Footer */}
      <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: 10, paddingTop: 8, borderTop: '1px solid #0F1F35' }}>
        <span style={{ fontSize: 9, color: '#334155' }}>
          {collector.version ? `v${collector.version.replace(/^v/, '')}` : ''}
        </span>
        {collector.last_checked && (
          <span style={{ fontSize: 9, color: '#334155' }}>
            Checked {new Date(collector.last_checked).toLocaleTimeString()}
          </span>
        )}
      </div>
    </div>
  )
}

export default function EnvironmentsPage() {
  const { data: rawEnvs, isLoading: envsLoading } = useEnvironments()

  const { data: collectorsData, isLoading: collectorsLoading, isError: collectorsError } = useCollectors()

  // Normalise environments — API may return objects or plain strings
  const envs: EnvironmentInfo[] = (rawEnvs ?? []).map((e) =>
    typeof e === 'string' ? { name: e, status: 'active', span_count: 0 } : e as EnvironmentInfo
  )

  // Decide which collector cards to show: dynamic if data returned, otherwise static fallback
  const collectors: CollectorInfo[] = (collectorsData && collectorsData.length > 0)
    ? collectorsData
    : STATIC_COLLECTORS

  const usedFallback = !collectorsLoading && (collectorsError || !collectorsData || collectorsData.length === 0)

  return (
    <div style={{ padding: 32 }}>
      <div style={{ marginBottom: 28 }}>
        <h1 style={{ fontSize: 22, fontWeight: 700, color: '#F0F9FF', margin: 0 }}>Environments</h1>
        <p style={{ fontSize: 12, color: '#475569', marginTop: 4 }}>Collector fleet and environment status</p>
      </div>

      {/* Collector endpoints */}
      <div style={{ background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 10, padding: 24, marginBottom: 24 }}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 16 }}>
          <div style={{ fontSize: 12, color: '#475569', letterSpacing: '0.1em' }}>COLLECTOR ENDPOINTS</div>
          {usedFallback && (
            <span style={{ fontSize: 9, color: '#334155', padding: '2px 6px', border: '1px solid #1E3A5F', borderRadius: 3 }}>
              static config — live API not available
            </span>
          )}
          {collectorsLoading && (
            <span style={{ fontSize: 9, color: '#334155' }}>Checking collectors…</span>
          )}
        </div>

        {collectorsLoading ? (
          <div style={{ color: '#334155', fontSize: 12, padding: 16, textAlign: 'center' }}>Loading collectors…</div>
        ) : (
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(220px, 1fr))', gap: 16 }}>
            {collectors.map((c) => (
              <CollectorCard key={c.id} collector={c} />
            ))}
          </div>
        )}
      </div>

      {/* Quick start */}
      <div style={{ background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 10, padding: 24, marginBottom: 24 }}>
        <div style={{ fontSize: 12, color: '#475569', marginBottom: 16, letterSpacing: '0.1em' }}>QUICK START — SEND A TEST SPAN</div>
        <pre style={{
          background: '#060A14', border: '1px solid #0F1F35', borderRadius: 6,
          padding: 16, fontSize: 11, color: '#94A3B8', overflowX: 'auto',
          fontFamily: "'JetBrains Mono', monospace", lineHeight: 1.6
        }}>{`# Python — instrument any agent framework
pip install agentfabric

from agentfabric import instrument
instrument(endpoint="http://localhost:4317", service_name="my-agent")

# Your existing CrewAI / LangGraph / OpenAI / Claude code is now traced

# Or send a raw OTLP span via HTTP:
curl -X POST http://localhost:4318/v1/traces \\
  -H "Content-Type: application/json" \\
  -d '{"resourceSpans":[]}'`}</pre>
      </div>

      {/* Environments list */}
      <div style={{ background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 10 }}>
        <div style={{ padding: '16px 24px', borderBottom: '1px solid #0F1F35', fontSize: 12, color: '#475569', letterSpacing: '0.1em' }}>
          CONFIGURED ENVIRONMENTS
        </div>
        <div style={{ padding: 24, display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 16 }}>
          {envsLoading && (
            <div style={{ color: '#334155', fontSize: 12 }}>Loading…</div>
          )}
          {envs.map((env) => {
            const color = ENV_COLORS[env.name] ?? '#475569'
            return (
              <div key={env.name} style={{
                background: '#060A14', border: `1px solid ${color}30`,
                borderLeft: `3px solid ${color}`, borderRadius: 8, padding: 20
              }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 12 }}>
                  <div style={{ width: 8, height: 8, borderRadius: '50%', background: color, boxShadow: `0 0 6px ${color}` }} />
                  <span style={{ fontSize: 14, fontWeight: 600, color: '#F0F9FF' }}>
                    {env.name.charAt(0).toUpperCase() + env.name.slice(1)}
                  </span>
                </div>
                <div style={{ fontSize: 11, color: '#475569', lineHeight: 1.8 }}>
                  <div>Spans: <span style={{ color: '#94A3B8' }}>{env.span_count.toLocaleString()}</span></div>
                  <div>Status: <span style={{ color: env.status === 'active' ? '#10B981' : '#475569' }}>{env.status}</span></div>
                  <div>Auth: <span style={{ color: env.name === 'production' ? '#EF4444' : '#10B981' }}>
                    {env.name === 'production' ? 'mTLS enabled' : 'dev mode'}
                  </span></div>
                </div>
              </div>
            )
          })}
          {!envsLoading && envs.length === 0 && (
            <div style={{ color: '#334155', fontSize: 12, gridColumn: '1/-1', textAlign: 'center', padding: 32 }}>
              No environments found. Send spans to register an environment.
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
