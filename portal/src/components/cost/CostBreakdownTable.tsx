import type { CostReportRow } from '../../hooks/api'

function fmtCost(value?: number) {
  return `$${(value ?? 0).toFixed(6)}`
}

export default function CostBreakdownTable({ rows }: { rows: CostReportRow[] }) {
  return (
    <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 11 }}>
      <thead>
        <tr style={{ borderBottom: '1px solid #0F1F35' }}>
          {['App', 'Environment', 'Provider', 'Model', 'Prompt', 'Release', 'Tokens', 'Input', 'Output', 'Cache Read', 'Cache Write', 'Reasoning', 'Total'].map(h => (
            <th key={h} style={{ padding: '9px 12px', textAlign: 'left', color: '#334155', fontSize: 10, letterSpacing: '0.08em', fontWeight: 700 }}>{h}</th>
          ))}
        </tr>
      </thead>
      <tbody>
        {rows.map((row, i) => {
          const tokenSummary = `${row.total_tokens ?? row.input_tokens + row.output_tokens + row.cache_read_tokens + row.cache_write_tokens + row.reasoning_tokens}`
          return (
            <tr key={`${row.app_name}-${row.environment}-${row.provider}-${row.model}-${row.prompt_id}-${row.release_tag}-${i}`} style={{ borderBottom: '1px solid #0A1020', background: i % 2 === 0 ? 'transparent' : '#060A1430' }}>
              <td style={{ padding: '8px 12px', color: '#E2E8F0' }}>{row.app_name}</td>
              <td style={{ padding: '8px 12px', color: '#94A3B8' }}>{row.environment}</td>
              <td style={{ padding: '8px 12px', color: '#60A5FA' }}>{row.provider}</td>
              <td style={{ padding: '8px 12px', color: '#CBD5E1' }}>{row.model}</td>
              <td style={{ padding: '8px 12px', color: '#C4B5FD' }}>{row.prompt_id}</td>
              <td style={{ padding: '8px 12px', color: '#F9A8D4' }}>{row.release_tag}</td>
              <td style={{ padding: '8px 12px', color: '#94A3B8' }}>{tokenSummary}</td>
              <td style={{ padding: '8px 12px', color: '#94A3B8' }}>{fmtCost(row.input_cost_usd)}</td>
              <td style={{ padding: '8px 12px', color: '#94A3B8' }}>{fmtCost(row.output_cost_usd)}</td>
              <td style={{ padding: '8px 12px', color: row.cache_read_cost_usd > 0 ? '#38BDF8' : '#475569' }}>{fmtCost(row.cache_read_cost_usd)}</td>
              <td style={{ padding: '8px 12px', color: row.cache_write_cost_usd > 0 ? '#A78BFA' : '#475569' }}>{fmtCost(row.cache_write_cost_usd)}</td>
              <td style={{ padding: '8px 12px', color: row.reasoning_cost_usd > 0 ? '#F472B6' : '#475569' }}>{fmtCost(row.reasoning_cost_usd)}</td>
              <td style={{ padding: '8px 12px', color: '#F59E0B' }}>{fmtCost(row.total_cost_usd)}</td>
            </tr>
          )
        })}
      </tbody>
    </table>
  )
}
