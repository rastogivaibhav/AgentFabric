import { useQuery } from '@tanstack/react-query'

const BASE = import.meta.env.VITE_API_URL ?? 'http://localhost:8080'

const ENV_COLORS: Record<string, string> = {
  production: '#EF4444',
  staging: '#F59E0B',
  development: '#10B981',
}

export default function EnvironmentsPage() {
  const { data: envs, isLoading } = useQuery<string[]>({
    queryKey: ['environments'],
    queryFn: async () => {
      const res = await fetch(`${BASE}/api/v1/environments`)
      if (!res.ok) throw new Error('fetch failed')
      return res.json()
    },
    refetchInterval: 30_000,
  })

  return (
    <div style={{ padding: 32 }}>
      <div style={{ marginBottom: 28 }}>
        <h1 style={{ fontSize: 22, fontWeight: 700, color: '#F0F9FF', margin: 0 }}>Environments</h1>
        <p style={{ fontSize: 12, color: '#475569', marginTop: 4 }}>Collector fleet and environment status</p>
      </div>

      {/* OTLP endpoints info */}
      <div style={{ background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 10, padding: 24, marginBottom: 24 }}>
        <div style={{ fontSize: 12, color: '#475569', marginBottom: 16, letterSpacing: '0.1em' }}>COLLECTOR ENDPOINTS</div>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 16 }}>
          {[
            { label: 'gRPC OTLP', addr: 'localhost:4317', proto: 'grpc', color: '#3B82F6' },
            { label: 'HTTP OTLP', addr: 'localhost:4318', proto: 'http', color: '#10B981' },
            { label: 'API Gateway', addr: 'localhost:8080', proto: 'http', color: '#8B5CF6' },
          ].map(({ label, addr, proto, color }) => (
            <div key={label} style={{ background: '#060A14', border: `1px solid ${color}30`, borderRadius: 8, padding: 16 }}>
              <div style={{ fontSize: 10, color: '#475569', letterSpacing: '0.1em', marginBottom: 8 }}>{label}</div>
              <div style={{ fontSize: 14, fontWeight: 700, color, fontFamily: 'monospace' }}>{addr}</div>
              <div style={{ fontSize: 10, color: '#334155', marginTop: 4 }}>{proto.toUpperCase()}</div>
            </div>
          ))}
        </div>
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
curl -X POST http://localhost:4318/v1/traces \
  -H "Content-Type: application/json" \
  -d '{"resourceSpans":[]}'`}</pre>
      </div>

      {/* Environments list */}
      <div style={{ background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 10 }}>
        <div style={{ padding: '16px 24px', borderBottom: '1px solid #0F1F35', fontSize: 12, color: '#475569', letterSpacing: '0.1em' }}>
          CONFIGURED ENVIRONMENTS
        </div>
        <div style={{ padding: 24, display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 16 }}>
          {isLoading && (
            <div style={{ color: '#334155', fontSize: 12 }}>Loading…</div>
          )}
          {(envs ?? []).map((env) => {
            const color = ENV_COLORS[env] ?? '#475569'
            return (
              <div key={env} style={{
                background: '#060A14', border: `1px solid ${color}30`,
                borderLeft: `3px solid ${color}`, borderRadius: 8, padding: 20
              }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 12 }}>
                  <div style={{ width: 8, height: 8, borderRadius: '50%', background: color, boxShadow: `0 0 6px ${color}` }} />
                  <span style={{ fontSize: 14, fontWeight: 600, color: '#F0F9FF' }}>
                    {env.charAt(0).toUpperCase() + env.slice(1)}
                  </span>
                </div>
                <div style={{ fontSize: 11, color: '#475569', lineHeight: 1.8 }}>
                  <div>Collector: <span style={{ color: '#94A3B8' }}>DaemonSet</span></div>
                  <div>Auth: <span style={{ color: env === 'production' ? '#EF4444' : '#10B981' }}>
                    {env === 'production' ? 'mTLS enabled' : 'dev mode'}
                  </span></div>
                </div>
              </div>
            )
          })}
        </div>
      </div>
    </div>
  )
}
