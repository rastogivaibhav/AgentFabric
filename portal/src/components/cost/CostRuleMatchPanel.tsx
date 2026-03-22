import type { PricingPreviewResponse } from '../../hooks/api'

function money(value?: number) {
  return `$${(value ?? 0).toFixed(6)}`
}

export default function CostRuleMatchPanel({ preview }: { preview?: PricingPreviewResponse | null }) {
  if (!preview) {
    return (
      <div style={{ color: '#334155', fontSize: 12, padding: 16, textAlign: 'center' }}>
        Run a pricing preview to inspect rule match and category-level cost provenance.
      </div>
    )
  }

  return (
    <div style={{ display: 'grid', gap: 10 }}>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))', gap: 12 }}>
        {[
          ['Rule', preview.rule_id ? `${preview.rule_id}` : 'No match'],
          ['Pattern', preview.model_pattern ?? '—'],
          ['Scope', preview.pricing_scope ?? '—'],
          ['Total', money(preview.total_cost_usd)],
        ].map(([label, value]) => (
          <div key={label} style={{ background: '#071525', border: '1px solid #0F1F35', borderRadius: 8, padding: 12 }}>
            <div style={{ fontSize: 10, color: '#3B82F6', marginBottom: 4 }}>{label}</div>
            <div style={{ fontSize: 12, color: '#E2E8F0' }}>{value}</div>
          </div>
        ))}
      </div>

      <div style={{ display: 'grid', gap: 8 }}>
        {[
          ['Input', preview.input_tokens, preview.input_per_million, preview.input_cost_usd],
          ['Output', preview.output_tokens, preview.output_per_million, preview.output_cost_usd],
          ['Cache Read', preview.cache_read_tokens ?? 0, preview.cache_read_per_million, preview.cache_read_cost_usd ?? 0],
          ['Cache Write', preview.cache_write_tokens ?? 0, preview.cache_write_per_million, preview.cache_write_cost_usd ?? 0],
          ['Reasoning', preview.reasoning_tokens ?? 0, preview.reasoning_per_million, preview.reasoning_cost_usd ?? 0],
        ].map(([label, tokens, rate, cost]) => (
          <div key={String(label)} style={{ display: 'flex', justifyContent: 'space-between', gap: 12, fontSize: 11, color: '#94A3B8', borderBottom: '1px solid #0A1020', paddingBottom: 6 }}>
            <span>{label}</span>
            <span>{Number(tokens).toLocaleString()} tokens</span>
            <span>${Number(rate ?? 0).toFixed(4)}/M</span>
            <span style={{ color: '#E2E8F0' }}>{money(Number(cost ?? 0))}</span>
          </div>
        ))}
      </div>

      {(preview.explain ?? []).length > 0 && (
        <div>
          <div style={{ fontSize: 10, color: '#3B82F6', marginBottom: 6 }}>Why this cost matched</div>
          <div style={{ display: 'grid', gap: 6 }}>
            {preview.explain!.map(line => (
              <div key={line} style={{ fontSize: 11, color: '#94A3B8' }}>{line}</div>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
