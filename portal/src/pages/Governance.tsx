import { AlertTriangle, Check, X, Clock, Shield } from 'lucide-react'
import { useMemo, useState } from 'react'

import {
  useGovernanceAlerts,
  useGovernanceSummary,
  useApproveGovernanceDecision,
} from '../hooks/api'

interface RiskAlert {
  span_id: string
  trace_id: string
  risk_score: number
  risk_category: string
  framework: string
  timestamp: string
  summary: string
  action_required?: boolean
}

interface RiskSummary {
  total_events: number
  high_risk_count: number
  categories: Record<string, number>
  trend: string
}

const RISK_COLORS: Record<string, string> = {
  dangerous_command: '#EF4444',
  secret_exposure: '#DC2626',
  prod_edit: '#F59E0B',
  prod_deploy: '#F97316',
  high_token_usage: '#FBBF24',
  unusual_activity: '#FCD34D',
  mcp_usage: '#60A5FA',
  approval_required: '#EC4899',
  none: '#10B981',
}

const RISK_LABELS: Record<string, string> = {
  dangerous_command: 'Dangerous Command',
  secret_exposure: 'Secret Exposure',
  prod_edit: 'Production Edit',
  prod_deploy: 'Production Deploy',
  high_token_usage: 'High Token Usage',
  unusual_activity: 'Unusual Activity',
  mcp_usage: 'MCP Tool Usage',
  approval_required: 'Approval Required',
  none: 'None',
}

function RiskBadge({ category, score }: { category: string; score: number }) {
  const color = RISK_COLORS[category] || RISK_COLORS.none
  const bgColor = color + '20'
  return (
    <div style={{
      display: 'inline-flex',
      alignItems: 'center',
      gap: 6,
      padding: '4px 8px',
      background: bgColor,
      borderRadius: 4,
      fontSize: 12,
      fontWeight: 600,
      color,
    }}>
      <AlertTriangle size={14} />
      {RISK_LABELS[category] || category}
      <span style={{ marginLeft: 4, opacity: 0.8 }}>({score})</span>
    </div>
  )
}

function RiskSummaryCard({ label, value, color }: { label: string; value: number; color?: string }) {
  return (
    <div style={{
      background: 'var(--layer)',
      border: '1px solid var(--layer-border)',
      borderRadius: 8,
      padding: 16,
      flex: 1,
      minWidth: 150,
    }}>
      <div style={{
        fontSize: 12,
        color: 'var(--text-secondary)',
        marginBottom: 8,
      }}>
        {label}
      </div>
      <div style={{
        fontSize: 28,
        fontWeight: 700,
        color: color || 'var(--text-primary)',
      }}>
        {value}
      </div>
    </div>
  )
}

function AlertItem({
  alert,
  onApprove,
  onReject,
  isProcessing,
}: {
  alert: RiskAlert
  onApprove: () => void
  onReject: () => void
  isProcessing: boolean
}) {
  return (
    <div style={{
      background: 'var(--layer)',
      border: `1px solid ${RISK_COLORS[alert.risk_category] || RISK_COLORS.none}40`,
      borderLeft: `4px solid ${RISK_COLORS[alert.risk_category] || RISK_COLORS.none}`,
      borderRadius: 8,
      padding: 16,
      marginBottom: 12,
      display: 'flex',
      justifyContent: 'space-between',
      alignItems: 'center',
    }}>
      <div style={{ flex: 1 }}>
        <div style={{
          display: 'flex',
          alignItems: 'center',
          gap: 12,
          marginBottom: 8,
        }}>
          <RiskBadge category={alert.risk_category} score={alert.risk_score} />
          <span style={{
            fontSize: 11,
            color: 'var(--text-tertiary)',
            display: 'flex',
            alignItems: 'center',
            gap: 4,
          }}>
            <Clock size={14} />
            {new Date(alert.timestamp).toLocaleString()}
          </span>
        </div>
        <div style={{
          fontSize: 13,
          color: 'var(--text-primary)',
          marginBottom: 6,
        }}>
          {alert.summary || `${alert.framework} - Trace ${alert.trace_id.slice(0, 8)}`}
        </div>
        <div style={{
          fontSize: 11,
          color: 'var(--text-tertiary)',
        }}>
          Span: <code style={{ fontSize: 10 }}>{alert.span_id.slice(0, 12)}...</code>
        </div>
      </div>

      {alert.action_required && (
        <div style={{
          display: 'flex',
          gap: 8,
          marginLeft: 16,
        }}>
          <button
            onClick={onApprove}
            disabled={isProcessing}
            style={{
              padding: '8px 16px',
              background: 'var(--move)',
              color: 'white',
              border: 'none',
              borderRadius: 4,
              cursor: isProcessing ? 'not-allowed' : 'pointer',
              fontSize: 12,
              fontWeight: 600,
              display: 'flex',
              alignItems: 'center',
              gap: 6,
              opacity: isProcessing ? 0.6 : 1,
            }}
          >
            <Check size={14} />
            Approve
          </button>
          <button
            onClick={onReject}
            disabled={isProcessing}
            style={{
              padding: '8px 16px',
              background: 'var(--protect)',
              color: 'white',
              border: 'none',
              borderRadius: 4,
              cursor: isProcessing ? 'not-allowed' : 'pointer',
              fontSize: 12,
              fontWeight: 600,
              display: 'flex',
              alignItems: 'center',
              gap: 6,
              opacity: isProcessing ? 0.6 : 1,
            }}
          >
            <X size={14} />
            Reject
          </button>
        </div>
      )}
    </div>
  )
}

export default function Governance() {
  const { data: alerts, isLoading: alertsLoading, refetch: refetchAlerts } = useGovernanceAlerts()
  const { data: summary } = useGovernanceSummary()
  const approve = useApproveGovernanceDecision()

  const [processingIds, setProcessingIds] = useState<Set<string>>(new Set())
  const [filterCategory, setFilterCategory] = useState<string>('')
  const [riskFilter, setRiskFilter] = useState<'all' | 'high' | 'critical'>('all')

  const categories = useMemo(() => {
    if (!summary?.categories) return []
    return Object.entries(summary.categories)
      .map(([name, count]) => ({
        name,
        count,
        label: RISK_LABELS[name] || name,
      }))
      .sort((a, b) => b.count - a.count)
  }, [summary])

  const filteredAlerts = useMemo(() => {
    if (!Array.isArray(alerts)) return []
    return alerts.filter(alert => {
      if (filterCategory && alert.risk_category !== filterCategory) return false
      if (riskFilter === 'critical' && alert.risk_score < 80) return false
      if (riskFilter === 'high' && alert.risk_score < 50) return false
      return true
    })
  }, [alerts, filterCategory, riskFilter])

  const handleApprove = async (alert: RiskAlert) => {
    setProcessingIds(prev => new Set([...prev, alert.span_id]))
    try {
      await approve.mutateAsync({
        span_id: alert.span_id,
        decision: 'approved',
      })
      refetchAlerts()
    } finally {
      setProcessingIds(prev => {
        const next = new Set(prev)
        next.delete(alert.span_id)
        return next
      })
    }
  }

  const handleReject = async (alert: RiskAlert) => {
    setProcessingIds(prev => new Set([...prev, alert.span_id]))
    try {
      await approve.mutateAsync({
        span_id: alert.span_id,
        decision: 'rejected',
      })
      refetchAlerts()
    } finally {
      setProcessingIds(prev => {
        const next = new Set(prev)
        next.delete(alert.span_id)
        return next
      })
    }
  }

  return (
    <div style={{ padding: '20px 0' }}>
      {/* Header */}
      <div style={{
        display: 'flex',
        alignItems: 'center',
        gap: 12,
        marginBottom: 24,
      }}>
        <Shield size={32} style={{ color: 'var(--protect)' }} />
        <h1 style={{ margin: 0, fontSize: 28, fontWeight: 700 }}>
          Governance
        </h1>
      </div>

      {/* Risk Summary */}
      {summary && (
        <div style={{
          display: 'grid',
          gridTemplateColumns: 'repeat(auto-fit, minmax(150px, 1fr))',
          gap: 12,
          marginBottom: 24,
        }}>
          <RiskSummaryCard
            label="Total Events"
            value={summary.total_events}
          />
          <RiskSummaryCard
            label="High Risk"
            value={summary.high_risk_count}
            color="var(--protect)"
          />
          {categories.slice(0, 2).map(cat => (
            <RiskSummaryCard
              key={cat.name}
              label={cat.label}
              value={cat.count}
            />
          ))}
        </div>
      )}

      {/* Filters */}
      <div style={{
        display: 'flex',
        gap: 12,
        marginBottom: 20,
        flexWrap: 'wrap',
      }}>
        <select
          value={riskFilter}
          onChange={(e) => setRiskFilter(e.target.value as any)}
          style={{
            padding: '8px 12px',
            borderRadius: 6,
            border: '1px solid var(--layer-border)',
            background: 'var(--layer)',
            color: 'var(--text-primary)',
            fontSize: 13,
            cursor: 'pointer',
          }}
        >
          <option value="all">All Risk Levels</option>
          <option value="high">High Risk (50+)</option>
          <option value="critical">Critical (80+)</option>
        </select>

        <select
          value={filterCategory}
          onChange={(e) => setFilterCategory(e.target.value)}
          style={{
            padding: '8px 12px',
            borderRadius: 6,
            border: '1px solid var(--layer-border)',
            background: 'var(--layer)',
            color: 'var(--text-primary)',
            fontSize: 13,
            cursor: 'pointer',
          }}
        >
          <option value="">All Categories</option>
          {categories.map(cat => (
            <option key={cat.name} value={cat.name}>{cat.label}</option>
          ))}
        </select>
      </div>

      {/* Risk Categories Breakdown */}
      {categories.length > 0 && (
        <div style={{
          background: 'var(--layer)',
          border: '1px solid var(--layer-border)',
          borderRadius: 8,
          padding: 16,
          marginBottom: 24,
        }}>
          <h3 style={{
            fontSize: 14,
            fontWeight: 600,
            margin: '0 0 12px 0',
            color: 'var(--text-primary)',
          }}>
            Risk Categories
          </h3>
          <div style={{
            display: 'grid',
            gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))',
            gap: 12,
          }}>
            {categories.map(cat => (
              <div
                key={cat.name}
                style={{
                  padding: 12,
                  background: 'var(--layer-border)',
                  borderRadius: 4,
                  cursor: 'pointer',
                  border: filterCategory === cat.name ? `2px solid ${RISK_COLORS[cat.name]}` : 'none',
                  transition: 'all 0.2s',
                }}
                onClick={() => setFilterCategory(filterCategory === cat.name ? '' : cat.name)}
              >
                <div style={{
                  display: 'flex',
                  justifyContent: 'space-between',
                  marginBottom: 4,
                }}>
                  <span style={{
                    fontSize: 12,
                    fontWeight: 600,
                    color: RISK_COLORS[cat.name],
                  }}>
                    {cat.label}
                  </span>
                  <span style={{
                    fontSize: 16,
                    fontWeight: 700,
                    color: RISK_COLORS[cat.name],
                  }}>
                    {cat.count}
                  </span>
                </div>
                <div style={{
                  height: 4,
                  background: 'var(--layer)',
                  borderRadius: 2,
                  overflow: 'hidden',
                }}>
                  <div style={{
                    width: `${Math.min(100, (cat.count / (summary?.total_events || 1)) * 100)}%`,
                    height: '100%',
                    background: RISK_COLORS[cat.name],
                  }} />
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* High-Risk Alerts */}
      <div>
        <h2 style={{
          fontSize: 16,
          fontWeight: 600,
          margin: '0 0 16px 0',
          color: 'var(--text-primary)',
          display: 'flex',
          alignItems: 'center',
          gap: 8,
        }}>
          <AlertTriangle size={18} style={{ color: 'var(--protect)' }} />
          High-Risk Events
          {filteredAlerts.length > 0 && (
            <span style={{
              background: 'var(--protect)',
              color: 'white',
              padding: '2px 8px',
              borderRadius: 12,
              fontSize: 12,
              fontWeight: 600,
            }}>
              {filteredAlerts.length}
            </span>
          )}
        </h2>

        {alertsLoading ? (
          <div style={{
            padding: 40,
            textAlign: 'center',
            color: 'var(--text-secondary)',
          }}>
            Loading governance alerts...
          </div>
        ) : filteredAlerts.length === 0 ? (
          <div style={{
            padding: 40,
            textAlign: 'center',
            color: 'var(--text-secondary)',
            background: 'var(--layer)',
            borderRadius: 8,
            border: '1px solid var(--layer-border)',
          }}>
            <Shield size={32} style={{
              opacity: 0.3,
              margin: '0 auto 12px',
              display: 'block',
            }} />
            No high-risk events to review
          </div>
        ) : (
          <div>
            {filteredAlerts.map(alert => (
              <AlertItem
                key={alert.span_id}
                alert={alert}
                onApprove={() => handleApprove(alert)}
                onReject={() => handleReject(alert)}
                isProcessing={processingIds.has(alert.span_id)}
              />
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
