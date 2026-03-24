import { useMemo, useState } from 'react'
import { Span, getSpanOutcomeStatus } from '../hooks/api'

const FW_COLORS: Record<string, string> = {
  crewai: '#FF6B35',
  langgraph: '#4ECDC4',
  google_adk: '#4285F4',
  openai_agents: '#10A37F',
  claude_agents: '#D97706',
  unknown: '#475569',
}

function fmtDuration(ns: number): string {
  if (ns < 1_000_000) return `${(ns / 1_000).toFixed(0)}µs`
  if (ns < 1_000_000_000) return `${(ns / 1_000_000).toFixed(1)}ms`
  return `${(ns / 1_000_000_000).toFixed(2)}s`
}

// Layout constants
const NODE_W = 160
const NODE_H = 52
const COL_GAP = 80  // horizontal gap between columns
const ROW_GAP = 20  // vertical gap between nodes in same column
const PAD_X = 24
const PAD_Y = 24
const ARROW_SIZE = 6

interface LayoutNode {
  span: Span
  x: number
  y: number
  col: number
  row: number
}

interface Edge {
  fromId: string
  toId: string
}

function buildLayout(spans: Span[]): { nodes: LayoutNode[]; edges: Edge[]; svgW: number; svgH: number } {
  if (spans.length === 0) return { nodes: [], edges: [], svgW: 0, svgH: 0 }

  // Build parent->children map and compute depth (column)
  const childrenOf: Record<string, string[]> = {}
  const depthOf: Record<string, number> = {}
  const spanById: Record<string, Span> = {}

  for (const s of spans) {
    spanById[s.id] = s
    if (!childrenOf[s.id]) childrenOf[s.id] = []
    if (s.parent_id) {
      if (!childrenOf[s.parent_id]) childrenOf[s.parent_id] = []
      childrenOf[s.parent_id].push(s.id)
    }
  }

  // BFS from roots to assign depths
  const roots = spans.filter(s => !s.parent_id || !spanById[s.parent_id])
  const queue: Array<{ id: string; depth: number }> = roots.map(r => ({ id: r.id, depth: 0 }))
  const visited = new Set<string>()

  while (queue.length > 0) {
    const { id, depth } = queue.shift()!
    if (visited.has(id)) continue
    visited.add(id)
    depthOf[id] = depth
    for (const childId of (childrenOf[id] ?? [])) {
      if (!visited.has(childId)) {
        queue.push({ id: childId, depth: depth + 1 })
      }
    }
  }

  // Any span not yet visited (orphaned cycles): assign depth 0
  for (const s of spans) {
    if (!(s.id in depthOf)) depthOf[s.id] = 0
  }

  // Group by column (depth)
  const byCol: Record<number, string[]> = {}
  for (const s of spans) {
    const col = depthOf[s.id]
    if (!byCol[col]) byCol[col] = []
    byCol[col].push(s.id)
  }

  const maxCol = Math.max(...Object.keys(byCol).map(Number))

  // Assign x, y positions
  const nodes: LayoutNode[] = []
  for (let col = 0; col <= maxCol; col++) {
    const ids = byCol[col] ?? []
    const x = PAD_X + col * (NODE_W + COL_GAP)
    ids.forEach((id, row) => {
      const y = PAD_Y + row * (NODE_H + ROW_GAP)
      nodes.push({ span: spanById[id], x, y, col, row })
    })
  }

  // Build edges
  const edges: Edge[] = []
  for (const s of spans) {
    if (s.parent_id && spanById[s.parent_id]) {
      edges.push({ fromId: s.parent_id, toId: s.id })
    }
  }

  const maxRows = Math.max(...Object.values(byCol).map(a => a.length))
  const svgW = PAD_X * 2 + (maxCol + 1) * NODE_W + maxCol * COL_GAP
  const svgH = PAD_Y * 2 + maxRows * NODE_H + Math.max(0, maxRows - 1) * ROW_GAP

  return { nodes, edges, svgW, svgH }
}

interface TopologyGraphProps {
  spans: Span[]
  selectedSpanId?: string
  onSelectSpan?: (spanId: string) => void
}

export default function TopologyGraph({ spans, selectedSpanId, onSelectSpan }: TopologyGraphProps) {
  const [hovered, setHovered] = useState<string | null>(null)

  const { nodes, edges, svgW, svgH } = useMemo(() => buildLayout(spans), [spans])

  if (spans.length === 0) {
    return (
      <div style={{
        display: 'flex', alignItems: 'center', justifyContent: 'center',
        height: 200, color: '#334155', fontSize: 12,
        background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 8,
      }}>
        No spans to display
      </div>
    )
  }

  // Build position lookup for edge drawing
  const posOf: Record<string, { x: number; y: number }> = {}
  for (const n of nodes) {
    posOf[n.span.id] = { x: n.x, y: n.y }
  }

  return (
    <div style={{ width: '100%', overflowX: 'auto', background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 8 }}>
      <svg
        viewBox={`0 0 ${svgW} ${svgH}`}
        width="100%"
        style={{ display: 'block', minWidth: svgW > 0 ? Math.min(svgW, 800) : 300 }}
        aria-label="Span topology graph"
      >
        <defs>
          {/* Arrowhead marker */}
          <marker
            id="arrowhead"
            markerWidth={ARROW_SIZE * 2}
            markerHeight={ARROW_SIZE * 2}
            refX={ARROW_SIZE * 1.5}
            refY={ARROW_SIZE}
            orient="auto"
          >
            <path
              d={`M 0 0 L ${ARROW_SIZE * 2} ${ARROW_SIZE} L 0 ${ARROW_SIZE * 2} z`}
              fill="#1E3A5F"
            />
          </marker>
          {/* Glow filter for selected node */}
          <filter id="glow" x="-30%" y="-30%" width="160%" height="160%">
            <feGaussianBlur stdDeviation="3" result="coloredBlur" />
            <feMerge>
              <feMergeNode in="coloredBlur" />
              <feMergeNode in="SourceGraphic" />
            </feMerge>
          </filter>
        </defs>

        {/* Edges */}
        {edges.map(({ fromId, toId }) => {
          const from = posOf[fromId]
          const to = posOf[toId]
          if (!from || !to) return null

          // Exit from right-center of source node, enter at left-center of target node
          const x1 = from.x + NODE_W
          const y1 = from.y + NODE_H / 2
          const x2 = to.x
          const y2 = to.y + NODE_H / 2

          // Cubic bezier for smooth curve
          const cx1 = x1 + (x2 - x1) * 0.5
          const cy1 = y1
          const cx2 = x1 + (x2 - x1) * 0.5
          const cy2 = y2

          return (
            <path
              key={`${fromId}-${toId}`}
              d={`M ${x1} ${y1} C ${cx1} ${cy1}, ${cx2} ${cy2}, ${x2} ${y2}`}
              fill="none"
              stroke="#1E3A5F"
              strokeWidth={1.5}
              markerEnd="url(#arrowhead)"
            />
          )
        })}

        {/* Nodes */}
        {nodes.map(({ span, x, y }) => {
          const color = FW_COLORS[span.framework] ?? '#475569'
          const isSelected = span.id === selectedSpanId
          const isHovered = span.id === hovered
          const isError = getSpanOutcomeStatus(span) === 'error'

          const borderColor = isError ? '#EF4444' : isSelected ? '#3B82F6' : isHovered ? color : '#1E3A5F'
          const bgColor = isSelected ? '#1E3A5F' : isHovered ? '#0F1F35' : '#060A14'

          return (
            <g
              key={span.id}
              transform={`translate(${x}, ${y})`}
              style={{ cursor: 'pointer' }}
              onClick={() => onSelectSpan?.(span.id)}
              onMouseEnter={() => setHovered(span.id)}
              onMouseLeave={() => setHovered(null)}
              filter={isSelected ? 'url(#glow)' : undefined}
            >
              {/* Node background */}
              <rect
                width={NODE_W}
                height={NODE_H}
                rx={6}
                ry={6}
                fill={bgColor}
                stroke={borderColor}
                strokeWidth={isSelected ? 2 : 1}
              />
              {/* Color accent bar on left */}
              <rect
                width={3}
                height={NODE_H}
                rx={6}
                ry={6}
                fill={color}
                opacity={0.85}
              />
              {/* Span name */}
              <text
                x={12}
                y={20}
                fill={isError ? '#EF4444' : '#CBD5E1'}
                fontSize={10}
                fontFamily="'JetBrains Mono', monospace"
                fontWeight={isSelected ? 700 : 400}
              >
                {span.name.length > 18 ? span.name.substring(0, 17) + '…' : span.name}
              </text>
              {/* Duration */}
              <text
                x={12}
                y={36}
                fill="#475569"
                fontSize={9}
                fontFamily="'JetBrains Mono', monospace"
              >
                {fmtDuration(span.duration_ns)}
              </text>
              {/* Framework dot */}
              <circle cx={NODE_W - 10} cy={NODE_H / 2} r={4} fill={color} opacity={0.7} />
              {/* Error indicator */}
              {isError && (
                <text x={NODE_W - 22} y={NODE_H / 2 + 4} fill="#EF4444" fontSize={10}>!</text>
              )}
            </g>
          )
        })}
      </svg>

      {/* Legend */}
      <div style={{ padding: '8px 16px', borderTop: '1px solid #0F1F35', display: 'flex', gap: 16, flexWrap: 'wrap' }}>
        {Object.entries(FW_COLORS).filter(([fw]) => fw !== 'unknown').map(([fw, color]) => (
          <div key={fw} style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
            <div style={{ width: 8, height: 8, borderRadius: '50%', background: color }} />
            <span style={{ fontSize: 9, color: '#475569', fontFamily: "'JetBrains Mono', monospace" }}>
              {fw.replace(/_/g, ' ')}
            </span>
          </div>
        ))}
      </div>
    </div>
  )
}
