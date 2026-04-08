import { useMemo, useState, type CSSProperties } from 'react'
import { useAgentScorecard, useAgentScorecards } from '../hooks/api'

const FRAMEWORK_COLORS: Record<string, string> = {
  crewai: '#FF6B35', langgraph: '#4ECDC4', google_adk: 'var(--control)',
  openai_agents: 'var(--spend)', claude_agents: 'var(--prove)', unknown: 'var(--text-tertiary)',
}

export default function AgentsPage() {
  const [since, setSince] = useState('24h')
  const [selectedAgentId, setSelectedAgentId] = useState('')
  const { data, isLoading, error } = useAgentScorecards(since, 24)

  const scorecards = data?.items ?? []
  const resolvedSelectedAgentId = selectedAgentId || scorecards[0]?.agent_id || ''
  const { data: detail } = useAgentScorecard(resolvedSelectedAgentId, since)
  const activeCard = detail ?? scorecards.find(item => item.agent_id === resolvedSelectedAgentId) ?? scorecards[0]

  const summary = useMemo(() => {
    if (scorecards.length === 0) return { averageScore: 0, atRisk: 0, totalRuns: 0, avgCost: 0 }
    const totalScore = scorecards.reduce((sum, item) => sum + item.overall_score, 0)
    const totalRuns = scorecards.reduce((sum, item) => sum + item.run_count, 0)
    const totalCost = scorecards.reduce((sum, item) => sum + item.total_cost_usd, 0)
    return { averageScore: totalScore / scorecards.length, atRisk: scorecards.filter(item => item.overall_score < 70).length, totalRuns, avgCost: totalCost / scorecards.length }
  }, [scorecards])

  return (
    <div style={{ padding: '40px 48px', maxWidth: 1440, margin: '0 auto' }}>
      <div style={{ marginBottom: 28, display: 'flex', justifyContent: 'space-between', gap: 16, alignItems: 'flex-end' }}>
        <div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
            <span style={{ width: 8, height: 8, borderRadius: '50%', background: 'var(--observe)', display: 'inline-block' }} />
            <span style={{ fontSize: 10, fontWeight: 700, color: 'var(--observe)', letterSpacing: '0.1em' }}>OBSERVE</span>
          </div>
          <h1 style={{ fontSize: 28, fontWeight: 700, color: 'var(--text-primary)', margin: 0, letterSpacing: '-0.02em' }}>Agent Scorecards</h1>
          <p style={{ fontSize: 12, color: 'var(--text-tertiary)', marginTop: 4 }}>Executive-ready agent health across reliability, policy risk, cost, regression, and release health.</p>
        </div>
        <label style={{ display: 'grid', gap: 6, fontSize: 11, color: 'var(--text-secondary)' }}>
          Window
          <select id="agents-window" value={since} onChange={e => setSince(e.target.value)}
            style={{ background: 'var(--layer-2)', border: '1px solid var(--layer-border)', borderRadius: 8, color: 'var(--text-primary)', padding: '8px 12px', fontSize: 12, outline: 'none' }}>
            <option value="24h">24h</option>
            <option value="72h">72h</option>
            <option value="168h">7d</option>
          </select>
        </label>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, minmax(0, 1fr))', gap: 16, marginBottom: 24 }}>
        <MetricCard label="Average Score" value={summary.averageScore.toFixed(1)} tone={scoreTone(summary.averageScore)} />
        <MetricCard label="Agents Scored" value={String(scorecards.length)} tone="var(--control)" />
        <MetricCard label="At Risk" value={String(summary.atRisk)} tone={summary.atRisk > 0 ? 'var(--protect)' : 'var(--spend)'} />
        <MetricCard label="Avg Cost" value={`$${summary.avgCost.toFixed(4)}`} tone="var(--prove)" />
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'minmax(360px, 420px) 1fr', gap: 16 }}>
        <div style={panelStyle}>
          <div style={sectionLabel}>AGENT HEALTH</div>
          {isLoading ? <div style={subtleText}>Loading scorecards...</div>
            : error ? <div style={{ color: 'var(--protect)', fontSize: 12 }}>Failed to load agent scorecards.</div>
            : scorecards.length === 0 ? <div style={subtleText}>No scored agents yet. Send runs and eval-linked traces to populate this view.</div>
            : (
              <div style={{ display: 'grid', gap: 8 }}>
                {scorecards.map(card => {
                  const active = card.agent_id === resolvedSelectedAgentId
                  return (
                    <button key={card.agent_id} id={`agent-card-${card.agent_id}`}
                      style={{ border: `1px solid ${active ? 'var(--control)' : 'var(--layer-border)'}`, borderRadius: 10, padding: 14, textAlign: 'left', cursor: 'pointer', background: active ? 'rgba(10,132,255,0.06)' : 'transparent', transition: 'all 0.15s' }}
                      onClick={() => setSelectedAgentId(card.agent_id)}>
                      <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12 }}>
                        <div>
                          <div style={{ color: 'var(--text-primary)', fontSize: 13, fontWeight: 700 }}>{card.agent_name}</div>
                          <div style={{ color: 'var(--text-tertiary)', fontSize: 10, marginTop: 4 }}>{card.app_name || 'unknown app'} | {card.environment || 'unknown env'}</div>
                        </div>
                        <div style={{ textAlign: 'right' }}>
                          <div style={{ color: scoreTone(card.overall_score), fontSize: 24, fontWeight: 700 }}>{card.overall_score.toFixed(1)}</div>
                          <div style={{ color: 'var(--text-tertiary)', fontSize: 10 }}>{card.trend.direction} {formatSigned(card.trend.delta)}</div>
                        </div>
                      </div>
                      <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, marginTop: 10 }}>
                        <span style={{ border: '1px solid', borderRadius: 999, padding: '2px 8px', fontSize: 10, textTransform: 'uppercase', color: FRAMEWORK_COLORS[card.framework] ?? 'var(--text-tertiary)', borderColor: `color-mix(in srgb, ${FRAMEWORK_COLORS[card.framework] ?? 'currentColor'} 30%, transparent)` }}>
                          {card.framework}
                        </span>
                        <span style={{ border: '1px solid', borderRadius: 999, padding: '2px 8px', fontSize: 10, textTransform: 'uppercase', color: scoreTone(card.overall_score), borderColor: `color-mix(in srgb, ${scoreTone(card.overall_score)} 30%, transparent)` }}>
                          {card.risk_level} risk
                        </span>
                      </div>
                    </button>
                  )
                })}
              </div>
            )}
        </div>

        <div style={panelStyle}>
          <div style={sectionLabel}>DRILL-DOWN</div>
          {!activeCard ? <div style={subtleText}>Select an agent to inspect its score breakdown.</div> : (
            <>
              <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, alignItems: 'flex-start' }}>
                <div>
                  <div style={{ color: 'var(--text-primary)', fontSize: 20, fontWeight: 700 }}>{activeCard.agent_name}</div>
                  <div style={{ color: 'var(--text-tertiary)', fontSize: 11, marginTop: 6 }}>
                    {activeCard.app_name || 'unknown app'} | {activeCard.environment || 'unknown env'} | release {activeCard.release_tag || 'unreleased'}
                  </div>
                </div>
                <div style={{ textAlign: 'right' }}>
                  <div style={{ color: scoreTone(activeCard.overall_score), fontSize: 36, fontWeight: 700, letterSpacing: '-0.02em' }}>{activeCard.overall_score.toFixed(1)}</div>
                  <div style={{ color: 'var(--text-tertiary)', fontSize: 11 }}>{activeCard.risk_level} risk | {activeCard.trend.direction}</div>
                </div>
              </div>

              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, minmax(0, 1fr))', gap: 10, marginTop: 18 }}>
                {[['Runs', String(activeCard.run_count)], ['Latency', `${activeCard.avg_latency_ms.toFixed(1)}ms`], ['Tokens', activeCard.total_tokens.toLocaleString()], ['Evals', String(activeCard.eval_count)]].map(([l, v]) => (
                  <div key={l} style={{ border: '1px solid var(--layer-border)', borderRadius: 8, background: 'var(--layer-0)', padding: 12 }}>
                    <div style={{ fontSize: 9, color: 'var(--text-tertiary)', letterSpacing: '0.08em', fontWeight: 700 }}>{l.toUpperCase()}</div>
                    <div style={{ color: 'var(--text-primary)', fontSize: 13, marginTop: 6 }}>{v}</div>
                  </div>
                ))}
              </div>

              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(2, minmax(0, 1fr))', gap: 10, marginTop: 14 }}>
                {activeCard.components.map(component => (
                  <div key={component.key} style={{ border: '1px solid var(--layer-border)', borderRadius: 8, background: 'var(--layer-0)', padding: 12 }}>
                    <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12 }}>
                      <div style={{ color: 'var(--text-secondary)', fontSize: 12, fontWeight: 600 }}>{component.label}</div>
                      <div style={{ color: scoreTone(component.score), fontSize: 12, fontWeight: 700 }}>{component.score.toFixed(1)}</div>
                    </div>
                    <div style={{ marginTop: 8, height: 5, background: 'var(--layer-1)', borderRadius: 999 }}>
                      <div style={{ width: `${Math.max(4, component.score)}%`, height: '100%', background: scoreTone(component.score), borderRadius: 999, transition: 'width 0.4s' }} />
                    </div>
                    <div style={{ color: 'var(--text-tertiary)', fontSize: 10, marginTop: 8 }}>{component.summary}</div>
                  </div>
                ))}
              </div>

              <div style={{ marginTop: 18 }}>
                <div style={sectionLabel}>RECOMMENDED FOCUS</div>
                <div style={{ display: 'grid', gap: 8 }}>
                  {(activeCard.recommended_actions ?? []).map(action => (
                    <div key={action} style={{ border: '1px solid var(--layer-border)', borderRadius: 8, background: 'var(--layer-0)', color: 'var(--text-secondary)', padding: 12, fontSize: 12, lineHeight: 1.5 }}>{action}</div>
                  ))}
                </div>
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  )
}

function MetricCard({ label, value, tone }: { label: string; value: string; tone: string }) {
  return (
    <div style={{ background: 'var(--layer-2)', border: '1px solid var(--layer-border)', borderRadius: 12, padding: '18px 20px' }}>
      <div style={{ color: 'var(--text-tertiary)', fontSize: 10, letterSpacing: '0.1em', fontWeight: 700 }}>{label.toUpperCase()}</div>
      <div style={{ color: tone, fontSize: 32, fontWeight: 700, marginTop: 10, letterSpacing: '-0.02em' }}>{value}</div>
    </div>
  )
}

function scoreTone(score: number): string {
  if (score >= 85) return 'var(--spend)'
  if (score >= 65) return 'var(--prove)'
  return 'var(--protect)'
}

function formatSigned(value: number): string {
  if (value > 0) return `+${value.toFixed(1)}`
  return value.toFixed(1)
}

const subtleText: CSSProperties = { fontSize: 12, color: 'var(--text-tertiary)', marginTop: 4 }
const panelStyle: CSSProperties = { background: 'var(--layer-2)', border: '1px solid var(--layer-border)', borderRadius: 12, padding: 24 }
const sectionLabel: CSSProperties = { fontSize: 10, color: 'var(--text-tertiary)', letterSpacing: '0.12em', marginBottom: 14, fontWeight: 700 }
