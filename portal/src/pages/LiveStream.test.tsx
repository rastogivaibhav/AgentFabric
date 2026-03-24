import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import React from 'react'

const mockPause = vi.fn()
const mockResume = vi.fn()
const mockClear = vi.fn()

vi.mock('../hooks/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../hooks/api')>()
  return {
    ...actual,
    useLiveStream: vi.fn(),
  }
})

vi.mock('react-router-dom', () => ({
  useNavigate: vi.fn(() => vi.fn()),
}))

// lucide-react icons — stub them so jsdom doesn't choke
vi.mock('lucide-react', () => ({
  Pause: () => <span data-testid="icon-pause" />,
  Play: () => <span data-testid="icon-play" />,
  Trash2: () => <span data-testid="icon-trash" />,
  Download: () => <span data-testid="icon-download" />,
  Filter: () => <span data-testid="icon-filter" />,
}))

import LiveStream from './LiveStream'
import { useLiveStream } from '../hooks/api'

const mockUseLiveStream = vi.mocked(useLiveStream)

const TRACE_ID = 'ccddee11-1234-5678-abcd-ef0011223344'
const SPAN_ID = 'span-aaaa-1111-bbbb-2222-cccc3333dddd'

const MOCK_EVENTS = [
  {
    type: 'span' as const,
    ts: 1_711_000_000_000,
    data: {
      id: SPAN_ID,
      trace_id: TRACE_ID,
      parent_id: undefined,
      run_id: 'run-xyz',
      name: 'agent_step',
      framework: 'crewai',
      start_time_ns: 1_711_000_000_000_000_000,
      duration_ns: 250_000_000,
      status_code: 0,
      attributes: { 'crewai.task': 'research' },
      received_at: '2024-03-21T10:00:00Z',
    },
  },
  {
    type: 'run_start' as const,
    ts: 1_711_000_010_000,
    data: {
      id: 'span-bbbb-2222-cccc-3333-ddddeeee4444',
      trace_id: TRACE_ID,
      parent_id: undefined,
      run_id: 'run-xyz',
      name: 'run_initializer',
      framework: 'langgraph',
      start_time_ns: 1_711_000_010_000_000_000,
      duration_ns: 50_000_000,
      status_code: 0,
      attributes: {},
      received_at: '2024-03-21T10:00:10Z',
    },
  },
]

function makeMockReturn(overrides = {}) {
  return {
    events: MOCK_EVENTS,
    connected: true,
    pause: mockPause,
    resume: mockResume,
    clear: mockClear,
    ...overrides,
  }
}

describe('LiveStream', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // Reset URL.createObjectURL for export tests
    global.URL.createObjectURL = vi.fn(() => 'blob:http://localhost/test')
    global.URL.revokeObjectURL = vi.fn()
  })

  it('renders table headers', () => {
    mockUseLiveStream.mockReturnValue(makeMockReturn() as any)
    render(<LiveStream />)
    expect(screen.getByText('TIME')).toBeInTheDocument()
    expect(screen.getByText('TYPE')).toBeInTheDocument()
    expect(screen.getByText('TRACE ID')).toBeInTheDocument()
    expect(screen.getByText('SPAN ID')).toBeInTheDocument()
    expect(screen.getByText('NAME')).toBeInTheDocument()
    expect(screen.getByText('FRAMEWORK')).toBeInTheDocument()
    expect(screen.getByText('DURATION')).toBeInTheDocument()
    expect(screen.getByText('COST')).toBeInTheDocument()
  })

  it('renders LIVE status indicator when connected and not paused', () => {
    mockUseLiveStream.mockReturnValue(makeMockReturn({ connected: true }) as any)
    render(<LiveStream />)
    expect(screen.getByText('LIVE')).toBeInTheDocument()
  })

  it('pause button changes status to PAUSED', () => {
    mockUseLiveStream.mockReturnValue(makeMockReturn({ connected: true }) as any)
    render(<LiveStream />)
    const pauseBtn = screen.getByText(/pause/i)
    fireEvent.click(pauseBtn)
    expect(mockPause).toHaveBeenCalled()
    // After clicking, the component state changes to paused — re-render check
    expect(screen.getByText('PAUSED')).toBeInTheDocument()
  })

  it('resume button changes status back to LIVE', () => {
    mockUseLiveStream.mockReturnValue(makeMockReturn({ connected: true }) as any)
    render(<LiveStream />)
    // First pause
    const pauseBtn = screen.getByText(/pause/i)
    fireEvent.click(pauseBtn)
    expect(screen.getByText('PAUSED')).toBeInTheDocument()
    // Then resume
    const resumeBtn = screen.getByText(/resume/i)
    fireEvent.click(resumeBtn)
    expect(mockResume).toHaveBeenCalled()
    expect(screen.getByText('LIVE')).toBeInTheDocument()
  })

  it('clear button calls the clear function', () => {
    mockUseLiveStream.mockReturnValue(makeMockReturn() as any)
    render(<LiveStream />)
    const clearBtn = screen.getByText(/clear/i)
    fireEvent.click(clearBtn)
    expect(mockClear).toHaveBeenCalled()
  })

  it('CSV Export button triggers a download', () => {
    mockUseLiveStream.mockReturnValue(makeMockReturn() as any)
    // Spy on document.createElement to intercept the anchor click
    const clickSpy = vi.fn()
    const originalCreateElement = document.createElement.bind(document)
    vi.spyOn(document, 'createElement').mockImplementation((tag: string) => {
      const el = originalCreateElement(tag)
      if (tag === 'a') {
        Object.defineProperty(el, 'click', { value: clickSpy })
      }
      return el
    })
    render(<LiveStream />)
    const exportBtn = screen.getByText(/export/i)
    fireEvent.click(exportBtn)
    expect(clickSpy).toHaveBeenCalled()
    vi.restoreAllMocks()
  })

  it('filter input shows only matching rows', () => {
    mockUseLiveStream.mockReturnValue(makeMockReturn() as any)
    render(<LiveStream />)
    const filterInput = screen.getByPlaceholderText(/filter: framework, name/i)
    fireEvent.change(filterInput, { target: { value: 'crewai' } })
    expect(screen.getByText('agent_step')).toBeInTheDocument()
    expect(screen.queryByText('run_initializer')).not.toBeInTheDocument()
  })

  it('filter input with no match shows empty table', () => {
    mockUseLiveStream.mockReturnValue(makeMockReturn() as any)
    render(<LiveStream />)
    const filterInput = screen.getByPlaceholderText(/filter: framework, name/i)
    fireEvent.change(filterInput, { target: { value: 'zzzxxx_no_match' } })
    expect(screen.queryByText('agent_step')).not.toBeInTheDocument()
    expect(screen.queryByText('run_initializer')).not.toBeInTheDocument()
  })

  it('auto-scroll toggle changes button label', () => {
    mockUseLiveStream.mockReturnValue(makeMockReturn() as any)
    render(<LiveStream />)
    // Initially ON
    expect(screen.getByText(/auto-scroll on/i)).toBeInTheDocument()
    const toggleBtn = screen.getByText(/auto-scroll on/i)
    fireEvent.click(toggleBtn)
    expect(screen.getByText(/auto-scroll off/i)).toBeInTheDocument()
  })

  it('clicking a row shows the detail panel with span info', () => {
    mockUseLiveStream.mockReturnValue(makeMockReturn() as any)
    render(<LiveStream />)
    // Click the first event row
    const spanName = screen.getByText('agent_step')
    const row = spanName.closest('div[style]')!
    fireEvent.click(row)
    expect(screen.getByText('SPAN DETAIL')).toBeInTheDocument()
  })

  it('useLiveStream hook is called with maxEvents=1000', () => {
    mockUseLiveStream.mockReturnValue(makeMockReturn() as any)
    render(<LiveStream />)
    expect(mockUseLiveStream).toHaveBeenCalledWith(1000)
  })

  it('renders events from mock useLiveStream in table', () => {
    mockUseLiveStream.mockReturnValue(makeMockReturn() as any)
    render(<LiveStream />)
    expect(screen.getByText('agent_step')).toBeInTheDocument()
    expect(screen.getByText('run_initializer')).toBeInTheDocument()
    expect(screen.getByText('crewai')).toBeInTheDocument()
    expect(screen.getByText('langgraph')).toBeInTheDocument()
  })
})
