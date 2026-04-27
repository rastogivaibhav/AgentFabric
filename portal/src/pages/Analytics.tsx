import { BarChart, Bar, PieChart, Pie, Cell, XAxis, YAxis, Tooltip, ResponsiveContainer, Legend, LineChart, Line } from 'recharts'
import { useMemo, useState } from 'react'

import {
  useOverview,
  useFrameworkStats,
  useCostReport,
  type OverviewStats,
} from '../hooks/api'

const FRAMEWORK_COLORS: Record<string, string> = {
  crewai: '#FF6B35',
  langgraph: '#4ECDC4',
  google_adk: '#95E1D3',
  openai_agents: '#F38181',
  claude_agents: '#AA96DA',
  unknown: '#CCCCCC',
}

const TIME_PERIODS = [
  { label: '24 hours', value: '24h' },
  { label: '7 days', value: '168h' },
  { label: '30 days', value: '720h' },
  { label: '90 days', value: '2160h' },
]

function formatMoney(value?: number) {
  if (!value) return '$0.00'
  if (value < 0.01) return `$${value.toFixed(6)}`
  return `$${value.toFixed(2)}`
}

function formatTokens(value?: number) {
  if (!value) return '0'
  if (value < 1000) return String(value)
  if (value < 1_000_000) return `${(value / 1000).toFixed(1)}K`
  return `${(value / 1_000_000).toFixed(1)}M`
}

function StatCard({ label, value, unit, icon, color }: {
  label: string
  value: string | number
  unit?: string
  icon?: string
  color?: string
}) {
  return (
    <div style={{
      background: 'var(--layer)',
      border: '1px solid var(--layer-border)',
      borderRadius: 8,
      padding: 20,
      flex: 1,
      minWidth: 200,
    }}>
      <div style={{
        fontSize: 12,
        color: 'var(--text-secondary)',
        marginBottom: 8,
        display: 'flex',
        alignItems: 'center',
        gap: 6,
      }}>
        {icon && <span style={{ fontSize: 16 }}>{icon}</span>}
        {label}
      </div>
      <div style={{
        fontSize: 24,
        fontWeight: 700,
        color: color || 'var(--text-primary)',
      }}>
        {value} {unit && <span style={{ fontSize: 14, color: 'var(--text-secondary)', fontWeight: 400 }}>{unit}</span>}
      </div>
    </div>
  )
}

function AnalyticsChart({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div style={{
      background: 'var(--layer)',
      border: '1px solid var(--layer-border)',
      borderRadius: 8,
      padding: 20,
      marginBottom: 20,
    }}>
      <h3 style={{
        fontSize: 14,
        fontWeight: 600,
        margin: '0 0 16 0',
        color: 'var(--text-primary)',
      }}>
        {title}
      </h3>
      {children}
    </div>
  )
}

export default function Analytics() {
  const [timePeriod, setTimePeriod] = useState('24h')
  const { data: overview, isLoading: overviewLoading } = useOverview(timePeriod)
  const { data: frameworks } = useFrameworkStats()
  const { data: costReport } = useCostReport({ since: timePeriod })

  const frameworkPieData = useMemo(() => {
    if (!overview?.framework_counts) return []
    return Object.entries(overview.framework_counts)
      .map(([name, count]) => ({
        name: name.replace(/_/g, ' ').toUpperCase(),
        value: count,
        fill: FRAMEWORK_COLORS[name] || FRAMEWORK_COLORS.unknown,
      }))
      .sort((a, b) => b.value - a.value)
  }, [overview])

  const vendorData = useMemo(() => {
    if (!costReport || !Array.isArray(costReport)) return []
    const vendors: Record<string, { vendor: string; tokens: number; cost: number; calls: number }> = {}
    costReport.forEach((row: any) => {
      const vendor = row.source_vendor || 'unknown'
      if (!vendors[vendor]) {
        vendors[vendor] = { vendor, tokens: 0, cost: 0, calls: 0 }
      }
      vendors[vendor].tokens += (row.input_tokens || 0) + (row.output_tokens || 0)
      vendors[vendor].cost += row.cost_usd || 0
      vendors[vendor].calls += 1
    })
    return Object.values(vendors).sort((a, b) => b.cost - a.cost)
  }, [costReport])

  const isLoading = overviewLoading

  return (
    <div style={{ padding: '20px 0' }}>
      <div style={{
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        marginBottom: 24,
      }}>
        <h1 style={{ margin: 0, fontSize: 28, fontWeight: 700 }}>Analytics</h1>
        <select
          value={timePeriod}
          onChange={(e) => setTimePeriod(e.target.value)}
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
          {TIME_PERIODS.map(p => (
            <option key={p.value} value={p.value}>{p.label}</option>
          ))}
        </select>
      </div>

      {/* Summary Cards */}
      <div style={{
        display: 'grid',
        gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))',
        gap: 16,
        marginBottom: 24,
      }}>
        <StatCard
          label="Total Cost"
          value={formatMoney(overview?.total_cost_usd)}
          color="var(--spend)"
          icon="💰"
        />
        <StatCard
          label="Total Tokens"
          value={formatTokens(overview?.total_tokens)}
          color="var(--control)"
          icon="📊"
        />
        <StatCard
          label="Avg Latency"
          value={Math.round(overview?.avg_latency_ms || 0)}
          unit="ms"
          color="var(--prove)"
          icon="⏱️"
        />
        <StatCard
          label="Error Rate"
          value={`${Math.round((overview?.error_rate || 0) * 100)}`}
          unit="%"
          color={((overview?.error_rate || 0) > 0.05) ? 'var(--protect)' : 'var(--move)'}
          icon="⚠️"
        />
        <StatCard
          label="Total Traces"
          value={overview?.total_traces || 0}
          color="var(--text-primary)"
          icon="🔄"
        />
        <StatCard
          label="Active Agents"
          value={overview?.active_agents || 0}
          color="var(--text-primary)"
          icon="🤖"
        />
      </div>

      {/* Charts Grid */}
      <div style={{
        display: 'grid',
        gridTemplateColumns: '1fr 1fr',
        gap: 20,
        marginBottom: 20,
      }}>
        {/* Framework Distribution */}
        <AnalyticsChart title="Framework Distribution">
          {frameworkPieData.length > 0 ? (
            <ResponsiveContainer width="100%" height={300}>
              <PieChart>
                <Pie
                  data={frameworkPieData}
                  cx="50%"
                  cy="50%"
                  labelLine={false}
                  label={({ name, percent }) => `${name} ${Math.round(percent * 100)}%`}
                  outerRadius={80}
                  fill="#8884d8"
                  dataKey="value"
                >
                  {frameworkPieData.map((entry, index) => (
                    <Cell key={`cell-${index}`} fill={entry.fill} />
                  ))}
                </Pie>
                <Tooltip />
              </PieChart>
            </ResponsiveContainer>
          ) : (
            <div style={{ padding: 40, textAlign: 'center', color: 'var(--text-secondary)' }}>
              No framework data available
            </div>
          )}
        </AnalyticsChart>

        {/* Vendor Cost Breakdown */}
        <AnalyticsChart title="Cost by Vendor">
          {vendorData.length > 0 ? (
            <ResponsiveContainer width="100%" height={300}>
              <BarChart data={vendorData}>
                <XAxis dataKey="vendor" angle={-45} textAnchor="end" height={100} />
                <YAxis />
                <Tooltip
                  formatter={(value: any) => formatMoney(value)}
                  contentStyle={{
                    background: 'var(--layer)',
                    border: '1px solid var(--layer-border)',
                    borderRadius: 6,
                  }}
                />
                <Bar dataKey="cost" fill="var(--spend)" radius={[8, 8, 0, 0]} />
              </BarChart>
            </ResponsiveContainer>
          ) : (
            <div style={{ padding: 40, textAlign: 'center', color: 'var(--text-secondary)' }}>
              No cost data available
            </div>
          )}
        </AnalyticsChart>
      </div>

      {/* Token Usage by Vendor */}
      <AnalyticsChart title="Token Usage by Vendor">
        {vendorData.length > 0 ? (
          <ResponsiveContainer width="100%" height={300}>
            <BarChart data={vendorData}>
              <XAxis dataKey="vendor" angle={-45} textAnchor="end" height={100} />
              <YAxis />
              <Tooltip
                formatter={(value: any) => formatTokens(value)}
                contentStyle={{
                  background: 'var(--layer)',
                  border: '1px solid var(--layer-border)',
                  borderRadius: 6,
                }}
              />
              <Bar dataKey="tokens" fill="var(--control)" radius={[8, 8, 0, 0]} />
            </BarChart>
          </ResponsiveContainer>
        ) : (
          <div style={{ padding: 40, textAlign: 'center', color: 'var(--text-secondary)' }}>
            No token data available
          </div>
        )}
      </AnalyticsChart>

      {/* Vendor Call Distribution */}
      {vendorData.length > 0 && (
        <AnalyticsChart title="API Calls by Vendor">
          <ResponsiveContainer width="100%" height={300}>
            <BarChart data={vendorData}>
              <XAxis dataKey="vendor" angle={-45} textAnchor="end" height={100} />
              <YAxis />
              <Tooltip
                contentStyle={{
                  background: 'var(--layer)',
                  border: '1px solid var(--layer-border)',
                  borderRadius: 6,
                }}
              />
              <Bar dataKey="calls" fill="var(--prove)" radius={[8, 8, 0, 0]} />
            </BarChart>
          </ResponsiveContainer>
        </AnalyticsChart>
      )}

      {isLoading && (
        <div style={{
          padding: 40,
          textAlign: 'center',
          color: 'var(--text-secondary)',
        }}>
          Loading analytics data...
        </div>
      )}
    </div>
  )
}
