import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import TopologyGraph from './TopologyGraph'

describe('TopologyGraph', () => {
  it('renders empty state when no spans are provided', () => {
    render(<TopologyGraph spans={[]} />)
    expect(screen.getByText(/no spans to display/i)).toBeInTheDocument()
  })

  it('renders nodes, legend, and selection callback', () => {
    const onSelectSpan = vi.fn()
    render(
      <TopologyGraph
        selectedSpanId="child"
        onSelectSpan={onSelectSpan}
        spans={[
          {
            id: 'root',
            parent_id: '',
            name: 'root-span',
            framework: 'openai_agents',
            duration_ns: 2_000_000,
            status_code: 1,
          },
          {
            id: 'child',
            parent_id: 'root',
            name: 'child-error-span',
            framework: 'langgraph',
            duration_ns: 900,
            status_code: 2,
          },
        ] as any}
      />,
    )

    expect(screen.getByLabelText(/span topology graph/i)).toBeInTheDocument()
    expect(screen.getByText(/root-span/i)).toBeInTheDocument()
    expect(screen.getByText(/child-error-span/i)).toBeInTheDocument()
    expect(screen.getByText(/openai agents/i)).toBeInTheDocument()
    expect(screen.getByText(/langgraph/i)).toBeInTheDocument()

    fireEvent.click(screen.getByText(/child-error-span/i))
    expect(onSelectSpan).toHaveBeenCalledWith('child')
  })
})
