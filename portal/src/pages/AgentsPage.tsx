import { useMemo, useState, type CSSProperties } from 'react'
import { useAgentScorecard, useAgentScorecards } from '../hooks/api'

const FRAMEWORK_COLORS: Record<string, string> = {
  crewai: '#FF6B35',
  langgraph: '#4ECDC4',
  google_adk: '#4285F4',
  openai_agents: '#10A37F',
  claude_agents: '#D97706',
  unknown: '#475569',
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
    if (scorecards.length === 0) {
      return { averageScore: 0, atRisk: 0, totalRuns: 0, avgCost: 0 }
    }
    const totalScore = scorecards.reduce((sum, item) => sum + item.overall_score, 0)
    const totalRuns = scorecards.reduce((sum, item) => sum + item.run_count, 0)
    const totalCost = scorecards.reduce((sum, item) => sum + item.total_cost_usd, 0)
    return {
      averageScore: totalScore / scorecards.length,
      atRisk: scorecards.filter(item => item.overall_score < 70).length,
      totalRuns,
      avgCost: totalCost / scorecards.length,
    }
  }, [scorecards])

  return (
    <div style={{ padding: 32 }}>
      <div style={{ marginBottom: 24, display: 'flex', justifyContent: 'space-between', gap: 16, alignItems: 'flex-start' }}>
        <div>
          <h1 style={titleStyle}>Agent Scorecards</h1>
          <p style={subtleText}>Executive-ready agent health across reliability, policy risk, cost, regression, and release health.</p>
        </div>
        <label style={labelStyle}>
          Window
          <select value={since} onChange={e => setSince(e.target.value)} style={inputStyle} aria-label="Score Window">
            <option value="24h">24h</option>
            <option value="72h">72h</option>
            <option value="168h">7d</option>
          </select>
        </label>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, minmax(0, 1fr))', gap: 16, marginBottom: 20 }}>
        <MetricCard label="Average Score" value={summary.averageScore.toFixed(1)} tone={scoreTone(summary.averageScore)} />
        <MetricCard label="Agents Scored" value={String(scorecards.length)} tone="#60A5FA" />
        <MetricCard label="At Risk" value={String(summary.atRisk)} tone={summary.atRisk > 0 ? '#FCA5A5' : '#10B981'} />
        <MetricCard label="Avg Cost" value={`$${summary.avgCost.toFixed(4)}`} tone="#F59E0B" />
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'minmax(360px, 420px) 1fr', gap: 16 }}>
        <div style={panelStyle}>
          <div style={sectionLabel}>AGENT HEALTH</div>
          {isLoading ? (
            <div style={subtleText}>Loading scorecards...</div>
          ) : error ? (
            <div style={errorStyle}>Failed to load agent scorecards.</div>
          ) : scorecards.length === 0 ? (
            <div style={subtleText}>No scored agents yet. Send runs and eval-linked traces to populate this view.</div>
          ) : (
            <div style={{ display: 'grid', gap: 10 }}>
              {scorecards.map(card => {
                const active = card.agent_id === resolvedSelectedAgentId
                return (
                  <button
                    key={card.agent_id}
                    style={{
                      ...scorecardBtnStyle,
                      borderColor: active ? '#2563EB' : '#0F1F35',
                      background: active ? '#0A1B33' : '#071525',
                    }}
                    onClick={() => setSelectedAgentId(card.agent_id)}
                  >
                    <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12 }}>
                      <div>
                        <div style={{ color: '#F8FAFC', fontSize: 13, fontWeight: 700 }}>{card.agent_name}</div>
                        <div style={{ color: '#64748B', fontSize: 10, marginTop: 4 }}>
                          {card.app_name || 'unknown app'} | {card.environment || 'unknown env'}
                        </div>
                      </div>
                      <div style={{ textAlign: 'right' }}>
                        <div style={{ color: scoreTone(card.overall_score), fontSize: 22, fontWeight: 700 }}>{card.overall_score.toFixed(1)}</div>
                        <div style={{ color: '#64748B', fontSize: 10 }}>{card.trend.direction} {formatSigned(card.trend.delta)}</div>
                      </div>
                    </div>
                    <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, marginTop: 10 }}>
                      <span style={{ ...tagStyle, color: FRAMEWORK_COLORS[card.framework] ?? '#94A3B8', borderColor: `${FRAMEWORK_COLORS[card.framework] ?? '#94A3B8'}40` }}>
                        {card.framework}
                      </span>
                      <span style={{ ...tagStyle, color: scoreTone(card.overall_score), borderColor: `${scoreTone(card.overall_score)}40` }}>
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
          {!activeCard ? (
            <div style={subtleText}>Select an agent to inspect its score breakdown.</div>
          ) : (
            <>
              <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, alignItems: 'flex-start' }}>
                <div>
                  <div style={{ color: '#F8FAFC', fontSize: 20, fontWeight: 700 }}>{activeCard.agent_name}</div>
                  <div style={{ color: '#64748B', fontSize: 11, marginTop: 6 }}>
                    {activeCard.app_name || 'unknown app'} | {activeCard.environment || 'unknown env'} | release {activeCard.release_tag || 'unreleased'}
                  </div>
                </div>
                <div style={{ textAlign: 'right' }}>
                  <div style={{ color: scoreTone(activeCard.overall_score), fontSize: 34, fontWeight: 700 }}>{activeCard.overall_score.toFixed(1)}</div>
                  <div style={{ color: '#94A3B8', fontSize: 11 }}>{activeCard.risk_level} risk | {activeCard.trend.direction}</div>
                </div>
              </div>

              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, minmax(0, 1fr))', gap: 12, marginTop: 16 }}>
                <DetailStat label="Runs" value={String(activeCard.run_count)} />
                <DetailStat label="Latency" value={`${activeCard.avg_latency_ms.toFixed(1)}ms`} />
                <DetailStat label="Tokens" value={activeCard.total_tokens.toLocaleString()} />
                <DetailStat label="Evals" value={String(activeCard.eval_count)} />
              </div>

              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(2, minmax(0, 1fr))', gap: 12, marginTop: 16 }}>
                {activeCard.components.map(component => (
                  <div key={component.key} style={componentCardStyle}>
                    <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12 }}>
                      <div style={{ color: '#E2E8F0', fontSize: 12, fontWeight: 600 }}>{component.label}</div>
                      <div style={{ color: scoreTone(component.score), fontSize: 12 }}>{component.score.toFixed(1)}</div>
                    </div>
                    <div style={{ marginTop: 8, height: 6, background: '#0F1F35', borderRadius: 999 }}>
                      <div style={{ width: `${Math.max(4, component.score)}%`, height: '100%', background: scoreTone(component.score), borderRadius: 999 }} />
                    </div>
                    <div style={{ color: '#94A3B8', fontSize: 10, marginTop: 8 }}>{component.summary}</div>
                  </div>
                ))}
              </div>

              <div style={{ marginTop: 16 }}>
                <div style={sectionLabel}>RECOMMENDED FOCUS</div>
                <div style={{ display: 'grid', gap: 8 }}>
                  {(activeCard.recommended_actions ?? []).map(action => (
                    <div key={action} style={recommendationStyle}>{action}</div>
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
    <div style={{ ...panelStyle, padding: 18 }}>
      <div style={{ color: '#475569', fontSize: 10, letterSpacing: '0.1em' }}>{label.toUpperCase()}</div>
      <div style={{ color: tone, fontSize: 28, fontWeight: 700, marginTop: 8 }}>{value}</div>
    </div>
  )
}

function DetailStat({ label, value }: { label: string; value: string }) {
  return (
    <div style={detailStatStyle}>
      <div style={{ color: '#475569', fontSize: 10, letterSpacing: '0.08em' }}>{label.toUpperCase()}</div>
      <div style={{ color: '#E2E8F0', fontSize: 13, marginTop: 6 }}>{value}</div>
    </div>
  )
}

function scoreTone(score: number): string {
  if (score >= 85) return '#10B981'
  if (score >= 65) return '#F59E0B'
  return '#FCA5A5'
}

function formatSigned(value: number): string {
  if (value > 0) return `+${value.toFixed(1)}`
  return value.toFixed(1)
}

const titleStyle: CSSProperties = { fontSize: 22, fontWeight: 700, color: '#F0F9FF', margin: 0 }
const subtleText: CSSProperties = { fontSize: 12, color: '#475569', marginTop: 4 }
const panelStyle: CSSProperties = { background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 10, padding: 20 }
const sectionLabel: CSSProperties = { fontSize: 10, color: '#334155', letterSpacing: '0.12em', marginBottom: 12 }
const labelStyle: CSSProperties = { display: 'grid', gap: 6, color: '#94A3B8', fontSize: 11 }
const inputStyle: CSSProperties = { background: '#071525', border: '1px solid #0F1F35', borderRadius: 8, color: '#E2E8F0', padding: '10px 12px', fontSize: 12 }
const errorStyle: CSSProperties = { color: '#FCA5A5', fontSize: 12 }
const scorecardBtnStyle: CSSProperties = { border: '1px solid #0F1F35', borderRadius: 10, padding: 14, textAlign: 'left', cursor: 'pointer' }
const tagStyle: CSSProperties = { border: '1px solid', borderRadius: 999, padding: '2px 8px', fontSize: 10, textTransform: 'uppercase' }
const detailStatStyle: CSSProperties = { border: '1px solid #0F1F35', borderRadius: 8, background: '#071525', padding: 12 }
const componentCardStyle: CSSProperties = { border: '1px solid #0F1F35', borderRadius: 8, background: '#071525', padding: 12 }
const recommendationStyle: CSSProperties = { border: '1px solid #10243D', borderRadius: 8, background: '#081221', color: '#CBD5E1', padding: 12, fontSize: 11 }
