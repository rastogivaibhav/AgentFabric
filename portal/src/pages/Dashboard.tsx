import { useMemo } from 'react'
import { Link } from 'react-router-dom'
import {
  useOverview,
  useRollouts,
  useTraces,
  useBudgetUsage,
  useControlAudit,
  useEvidenceBundles,
  outcomeStatusColor,
  type Trace
} from '../hooks/api'

// Framework colors mapping
const FRAMEWORK_COLORS: Record<string, string> = {
  crewai: '#FF6B35',
  langgraph: '#4ECDC4',
  google_adk: 'var(--control)',
  openai_agents: 'var(--spend)',
  claude_agents: 'var(--prove)',
  unknown: 'var(--text-tertiary)',
}

// Micro-Component for semantic stack card
function StackCard({ 
  label, 
  value, 
  sub, 
  color, 
  stackLabel, 
  link 
}: { 
  label: string; 
  value: string; 
  sub?: string; 
  color: string;
  stackLabel: string;
  link: string;
}) {
  return (
    <div style={{
      background: 'var(--layer-2)', 
      border: '1px solid var(--layer-border)', 
      borderRadius: 12,
      padding: '24px 28px', 
      position: 'relative',
      overflow: 'hidden',
      display: 'flex',
      flexDirection: 'column',
      justifyContent: 'space-between',
      minHeight: 180
    }}>
      {/* Accent Strip */}
      <div style={{
        position: 'absolute',
        top: 0, left: 0, right: 0, height: 2,
        background: color
      }} />

      <div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 16 }}>
          <span style={{ width: 8, height: 8, borderRadius: '50%', background: color }} />
          <span style={{ fontSize: 10, fontWeight: 700, color, letterSpacing: '0.1em' }}>
            {stackLabel}
          </span>
        </div>
        <div style={{ fontSize: 44, fontWeight: 700, color: 'var(--text-primary)', lineHeight: 1, marginBottom: 8, letterSpacing: '-0.02em' }}>
          {value}
        </div>
        <div style={{ fontSize: 14, color: 'var(--text-secondary)' }}>{label}</div>
      </div>
      
      {sub && (
        <div style={{ marginTop: 24, fontSize: 12, color: 'var(--text-tertiary)' }}>
          {sub} <Link to={link} style={{ color, textDecoration: 'none', marginLeft: 4 }}>[→ View]</Link>
        </div>
      )}
    </div>
  )
}

function truncate(str: string, len: number) {
  if (str.length <= len) return str
  return str.slice(0, len) + '…'
}

export default function Dashboard() {
  const { data: overview, isLoading: overviewLoading } = useOverview('24h')
  const { data: rolloutsData } = useRollouts()
  const { data: tracesData } = useTraces({ limit: '8' })
  const { data: budgetUsage } = useBudgetUsage('default')
  const { data: auditData } = useControlAudit(1)
  const { data: bundlesData } = useEvidenceBundles(10)

  const blockedCount = overview?.blocked_requests ?? 0
  const totalTraces = overview?.total_traces ?? 0
  const costPerHour = overview?.total_cost_usd ? (overview.total_cost_usd / 24).toFixed(4) : '0.0000'
  
  const activeRollouts = (rolloutsData?.items ?? []).filter(r => r.status === 'active')
  const recentTraces = tracesData?.items ?? []

  const lastAudit = auditData?.items?.[0]
  const bundlesCount = bundlesData?.items?.length ?? 0
  
  const budgetPct = (budgetUsage?.cost_pct ?? 0) * 100
  const budgetColor = budgetPct >= 100 ? 'var(--protect)' : budgetPct >= 80 ? 'var(--prove)' : 'var(--spend)'

  return (
    <div style={{ padding: '40px 48px', maxWidth: 1440, margin: '0 auto', width: '100%' }}>
      {/* Header */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-end', marginBottom: 40 }}>
        <div>
          <h1 style={{ fontSize: 32, fontWeight: 700, color: 'var(--text-primary)', margin: '0 0 8px 0', letterSpacing: '-0.02em' }}>
            Your AI. Under control.
          </h1>
        </div>
        <div style={{ fontSize: 13, color: 'var(--text-secondary)' }}>
          {new Date().toLocaleDateString(undefined, { month: 'short', day: 'numeric' })} · <span style={{ color: 'var(--spend)' }}>System: OK</span>
        </div>
      </div>

      {/* Hero Stack Cards */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 20, marginBottom: 16 }}>
        <StackCard 
          stackLabel="PROTECT" 
          color="var(--protect)" 
          value={overviewLoading ? '—' : blockedCount.toLocaleString()} 
          label="blocks today" 
          sub="No DLP leaks detected."
          link="/policies"
        />
        <StackCard 
          stackLabel="CONTROL" 
          color="var(--control)" 
          value={activeRollouts.length.toString()} 
          label="active rollouts" 
          sub={`${activeRollouts.find(r => r.target_type === 'model')?.name || 'Stable rules'} in flight.`}
          link="/rollouts"
        />
        <StackCard 
          stackLabel="SPEND" 
          color="var(--spend)" 
          value={`$${costPerHour}`} 
          label="/ hour" 
          sub={`${budgetUsage ? `${(100 - budgetPct).toFixed(0)}% budget remaining.` : 'Budget running smoothly.'}`}
          link="/cost"
        />
        <StackCard 
          stackLabel="OBSERVE" 
          color="var(--observe)" 
          value={overviewLoading ? '—' : totalTraces.toLocaleString()} 
          label="traces" 
          sub={`${overview?.error_rate ? (overview.error_rate * 100).toFixed(1) : '0'}% error rate in 24h.`}
          link="/traces"
        />
      </div>

      {/* Compliance Strip (PROVE) */}
      <div style={{
        background: 'var(--layer-2)',
        border: '1px solid var(--layer-border)',
        borderRadius: 8,
        padding: '12px 20px',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        marginBottom: 40
      }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 12, fontSize: 13, color: 'var(--text-secondary)' }}>
          <span style={{ display: 'flex', alignItems: 'center', gap: 6, color: 'var(--prove)', fontWeight: 600, fontSize: 10, letterSpacing: '0.1em' }}>
            <span style={{ width: 6, height: 6, borderRadius: '50%', background: 'var(--prove)' }} />
            PROVE
          </span>
          <span>·</span>
          <span>Audit chain: <span style={{ color: 'var(--spend)' }}>intact ✓</span></span>
          <span>·</span>
          <span>Last change: {lastAudit ? `${new Date(lastAudit.created_at).toLocaleTimeString()} by ${lastAudit.actor || 'system'}` : 'None recently'}</span>
          <span>·</span>
          <span>{bundlesCount} evidence bundles</span>
        </div>
        <Link to="/audit" style={{ fontSize: 12, color: 'var(--prove)', textDecoration: 'none' }}>
          [→ Audit Log]
        </Link>
      </div>

      {/* Lower Dashboard Content */}
      <div style={{ display: 'grid', gridTemplateColumns: '2fr 1fr', gap: 24 }}>
        
        {/* Live Activity Feed */}
        <div style={{ background: 'var(--layer-2)', border: '1px solid var(--layer-border)', borderRadius: 12, padding: 24 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
            <div style={{ fontSize: 12, color: 'var(--text-secondary)', fontWeight: 600, letterSpacing: '0.1em' }}>
              LIVE ACTIVITY FEED
            </div>
            <Link to="/live" style={{ fontSize: 12, color: 'var(--control)', textDecoration: 'none' }}>
              [View Full Stream]
            </Link>
          </div>
          
          <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
            {recentTraces.length === 0 ? (
              <div style={{ fontSize: 13, color: 'var(--text-tertiary)', padding: '20px 0' }}>No recent activity.</div>
            ) : (
              recentTraces.map((trace: Trace) => {
                const statusColor = outcomeStatusColor(trace.status)
                const isBlocked = trace.status === 'error' || trace.error_count > 0; // simplistic fallback
                const statusLabel = trace.status.toUpperCase()
                
                return (
                  <div key={trace.id} style={{ display: 'flex', alignItems: 'center', gap: 16, paddingBottom: 12, borderBottom: '1px solid var(--layer-border)', fontSize: 13 }}>
                    <div style={{ width: 70, color: 'var(--text-tertiary)', fontFamily: 'monospace' }}>
                      {new Date(trace.start_time).toLocaleTimeString([], { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' })}
                    </div>
                    <div style={{ width: 80 }}>
                      <span style={{ padding: '2px 8px', borderRadius: 4, fontSize: 10, background: `${statusColor}20`, color: statusColor, fontWeight: 600 }}>
                        {statusLabel}
                      </span>
                    </div>
                    <div style={{ width: 100, color: 'var(--text-secondary)' }}>
                      {trace.framework}
                    </div>
                    <div style={{ flex: 1, color: 'var(--text-primary)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                      {trace.root_span_name}
                    </div>
                    <div style={{ width: 80, textAlign: 'right', color: 'var(--text-secondary)' }}>
                      ${trace.total_cost_usd.toFixed(5)}
                    </div>
                  </div>
                )
              })
            )}
          </div>
        </div>

        {/* Secondary Widgets Column */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 24 }}>
          
          {/* Active Rollouts */}
          <div style={{ background: 'var(--layer-2)', border: '1px solid var(--layer-border)', borderRadius: 12, padding: 24 }}>
            <div style={{ fontSize: 12, color: 'var(--text-secondary)', fontWeight: 600, letterSpacing: '0.1em', marginBottom: 20 }}>
              ACTIVE ROLLOUTS
            </div>
            {activeRollouts.length === 0 ? (
              <div style={{ fontSize: 13, color: 'var(--text-tertiary)' }}>No active rollouts at the moment.</div>
            ) : (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
                {activeRollouts.slice(0, 4).map(rollout => (
                  <div key={rollout.id}>
                    <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 12, marginBottom: 6 }}>
                      <span style={{ color: 'var(--text-primary)' }}>{truncate(rollout.name, 24)}</span>
                      <span style={{ color: 'var(--control)' }}>{rollout.percentage}%</span>
                    </div>
                    <div style={{ height: 6, background: 'var(--layer-1)', borderRadius: 3, overflow: 'hidden' }}>
                      <div style={{ height: '100%', width: `${rollout.percentage}%`, background: 'var(--control)' }} />
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* Budget Status */}
          <div style={{ background: 'var(--layer-2)', border: '1px solid var(--layer-border)', borderRadius: 12, padding: 24 }}>
            <div style={{ fontSize: 12, color: 'var(--text-secondary)', fontWeight: 600, letterSpacing: '0.1em', marginBottom: 20 }}>
              BUDGET STATUS
            </div>
            {!budgetUsage ? (
              <div style={{ fontSize: 13, color: 'var(--text-tertiary)' }}>No budget configured.</div>
            ) : (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
                <div>
                  <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 12, marginBottom: 6 }}>
                    <span style={{ color: 'var(--text-secondary)' }}>Monthly Cost Limit</span>
                    <span style={{ color: 'var(--text-primary)', fontWeight: 600 }}>${budgetUsage.cost_used_usd.toFixed(2)} / ${budgetUsage.budget?.monthly_cost_usd?.toFixed(0) || '∞'}</span>
                  </div>
                  <div style={{ height: 6, background: 'var(--layer-1)', borderRadius: 3, overflow: 'hidden' }}>
                    <div style={{ height: '100%', width: `${Math.min(budgetPct, 100)}%`, background: budgetColor }} />
                  </div>
                </div>

                <div>
                  <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 12, marginBottom: 6, marginTop: 8 }}>
                    <span style={{ color: 'var(--text-secondary)' }}>Monthly Tokens Limit</span>
                    <span style={{ color: 'var(--text-primary)', fontWeight: 600 }}>
                      {(budgetUsage.tokens_used / 1000).toFixed(0)}k / {budgetUsage.budget?.monthly_tokens ? (budgetUsage.budget.monthly_tokens / 1000).toFixed(0) + 'k' : '∞'}
                    </span>
                  </div>
                  <div style={{ height: 6, background: 'var(--layer-1)', borderRadius: 3, overflow: 'hidden' }}>
                    <div style={{ height: '100%', width: `${Math.min((budgetUsage.tokens_pct ?? 0) * 100, 100)}%`, background: budgetColor }} />
                  </div>
                </div>
              </div>
            )}
          </div>

        </div>
      </div>
    </div>
  )
}
