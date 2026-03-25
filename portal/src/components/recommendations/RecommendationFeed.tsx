import type { CSSProperties } from 'react'
import type { Recommendation } from '../../hooks/api'

type RecommendationStatus = 'open' | 'reviewing' | 'applied' | 'dismissed' | 'resolved'

export default function RecommendationFeed({
  title,
  recommendations,
  isLoading,
  error,
  emptyMessage,
  onUpdateStatus,
}: {
  title: string
  recommendations: Recommendation[]
  isLoading?: boolean
  error?: unknown
  emptyMessage: string
  onUpdateStatus?: (id: number, status: RecommendationStatus) => void
}) {
  return (
    <div style={panelStyle}>
      <div style={sectionLabel}>{title}</div>
      {isLoading ? (
        <div style={subtleText}>Loading recommendations...</div>
      ) : error ? (
        <div style={errorStyle}>Failed to load recommendations.</div>
      ) : recommendations.length === 0 ? (
        <div style={subtleText}>{emptyMessage}</div>
      ) : (
        <div style={{ display: 'grid', gap: 10 }}>
          {recommendations.map(item => (
            <div key={item.id} style={cardStyle}>
              <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, alignItems: 'flex-start' }}>
                <div>
                  <div style={{ color: '#F8FAFC', fontSize: 13, fontWeight: 700 }}>{item.title}</div>
                  <div style={{ color: '#64748B', fontSize: 10, marginTop: 4 }}>
                    {item.type.toUpperCase()} | {item.target}
                  </div>
                </div>
                <div style={{ textAlign: 'right' }}>
                  <div style={{ ...badgeStyle, color: statusColor(item.status), borderColor: `${statusColor(item.status)}40` }}>
                    {item.status}
                  </div>
                  <div style={{ color: '#94A3B8', fontSize: 10, marginTop: 6 }}>
                    {(item.confidence * 100).toFixed(0)}% confidence
                  </div>
                </div>
              </div>
              <div style={{ color: '#CBD5E1', fontSize: 11, marginTop: 10 }}>{item.summary}</div>
              <div style={{ color: '#94A3B8', fontSize: 11, marginTop: 8 }}>
                Suggested action: {item.suggested_action}
              </div>
              {item.estimated_impact && (
                <div style={{ color: '#64748B', fontSize: 10, marginTop: 6 }}>
                  Impact: {item.estimated_impact}
                </div>
              )}
              {item.blast_radius && (
                <div style={{ color: '#64748B', fontSize: 10, marginTop: 4 }}>
                  Blast radius: {item.blast_radius}
                </div>
              )}
              {onUpdateStatus && (
                <div style={{ display: 'flex', gap: 8, marginTop: 12 }}>
                  <button style={actionBtnStyle} onClick={() => onUpdateStatus(item.id, 'reviewing')}>Reviewing</button>
                  <button style={actionBtnStyle} onClick={() => onUpdateStatus(item.id, 'applied')}>Applied</button>
                  <button style={dismissBtnStyle} onClick={() => onUpdateStatus(item.id, 'dismissed')}>Dismiss</button>
                </div>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

function statusColor(status: string) {
  switch (status) {
    case 'applied':
      return '#10B981'
    case 'reviewing':
      return '#60A5FA'
    case 'dismissed':
      return '#64748B'
    case 'resolved':
      return '#94A3B8'
    default:
      return '#F59E0B'
  }
}

const panelStyle: CSSProperties = { background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 10, padding: 20 }
const sectionLabel: CSSProperties = { fontSize: 10, color: '#334155', letterSpacing: '0.12em', marginBottom: 12 }
const subtleText: CSSProperties = { color: '#475569', fontSize: 12 }
const errorStyle: CSSProperties = { color: '#FCA5A5', fontSize: 12 }
const cardStyle: CSSProperties = { border: '1px solid #0F1F35', borderRadius: 8, background: '#071525', padding: 12 }
const badgeStyle: CSSProperties = { border: '1px solid', borderRadius: 999, padding: '2px 8px', fontSize: 10, textTransform: 'uppercase' }
const actionBtnStyle: CSSProperties = { background: '#1E3A5F', color: '#F8FAFC', border: 'none', borderRadius: 8, padding: '8px 10px', fontSize: 11, cursor: 'pointer' }
const dismissBtnStyle: CSSProperties = { background: '#7F1D1D', color: '#F8FAFC', border: 'none', borderRadius: 8, padding: '8px 10px', fontSize: 11, cursor: 'pointer' }
