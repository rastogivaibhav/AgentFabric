import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import React from 'react'

// EnvironmentsPage uses useQuery for environments and useCollectors from hooks/api.
vi.mock('@tanstack/react-query', () => ({
  useQuery: vi.fn(),
}))

vi.mock('../hooks/api', () => ({
  useCollectors: vi.fn(),
}))

import EnvironmentsPage from './EnvironmentsPage'
import { useCollectors } from '../hooks/api'
import { useQuery } from '@tanstack/react-query'

const mockUseQuery    = vi.mocked(useQuery)
const mockUseCollectors = vi.mocked(useCollectors)

// Minimal CollectorInfo fixture matching the CollectorInfo interface in hooks/api.
const makeCollector = (overrides = {}) => ({
  id: 'grpc',
  name: 'gRPC OTLP Receiver',
  endpoint_grpc: 'localhost:4317',
  endpoint_http: '',
  status: 'healthy',
  version: 'v1',
  ...overrides,
})

beforeEach(() => {
  vi.clearAllMocks()
})

// ─── Loading state ────────────────────────────────────────────────────────────

describe('EnvironmentsPage — loading state', () => {
  it('shows collector loading indicator while data is fetching', () => {
    mockUseQuery.mockReturnValue({ data: undefined, isLoading: true } as any)
    mockUseCollectors.mockReturnValue({ data: undefined, isLoading: true, isError: false } as any)

    render(<EnvironmentsPage />)

    expect(screen.getByText(/loading collectors/i)).toBeDefined()
  })
})

// ─── Empty environments ───────────────────────────────────────────────────────

describe('EnvironmentsPage — empty state', () => {
  it('shows empty-state message when no environments are returned', () => {
    mockUseQuery.mockReturnValue({ data: [], isLoading: false } as any)
    mockUseCollectors.mockReturnValue({
      data: [makeCollector()],
      isLoading: false,
      isError: false,
    } as any)

    render(<EnvironmentsPage />)

    expect(screen.getByText(/no environments found/i)).toBeDefined()
  })
})

// ─── Environments rendered ────────────────────────────────────────────────────

describe('EnvironmentsPage — environment cards', () => {
  const envs = [
    { name: 'production', status: 'active', span_count: 12345 },
    { name: 'staging',    status: 'active', span_count: 678 },
    { name: 'development', status: 'active', span_count: 0 },
  ]

  beforeEach(() => {
    mockUseQuery.mockReturnValue({ data: envs, isLoading: false } as any)
    mockUseCollectors.mockReturnValue({
      data: [makeCollector()],
      isLoading: false,
      isError: false,
    } as any)
  })

  it('renders the page heading', () => {
    render(<EnvironmentsPage />)
    expect(screen.getByText('Environments')).toBeDefined()
  })

  it('renders the page subtitle', () => {
    render(<EnvironmentsPage />)
    expect(screen.getByText(/collector fleet and environment status/i)).toBeDefined()
  })

  it('renders all three environment cards', () => {
    render(<EnvironmentsPage />)
    // Environment names are capitalised in the component: charAt(0).toUpperCase()
    expect(screen.getByText('Production')).toBeDefined()
    expect(screen.getByText('Staging')).toBeDefined()
    expect(screen.getByText('Development')).toBeDefined()
  })

  it('shows span counts for each environment', () => {
    render(<EnvironmentsPage />)
    expect(screen.getByText('12,345')).toBeDefined()
    expect(screen.getByText('678')).toBeDefined()
  })

  it('shows mTLS label for production environment', () => {
    render(<EnvironmentsPage />)
    expect(screen.getByText('mTLS enabled')).toBeDefined()
  })

  it('shows dev mode for non-production environments', () => {
    render(<EnvironmentsPage />)
    // staging and development both show "dev mode"
    const devModeItems = screen.getAllByText('dev mode')
    expect(devModeItems.length).toBeGreaterThanOrEqual(2)
  })
})

// ─── Collector cards ──────────────────────────────────────────────────────────

describe('EnvironmentsPage — collector cards', () => {
  beforeEach(() => {
    mockUseQuery.mockReturnValue({ data: [], isLoading: false } as any)
  })

  it('renders live collector data when API returns results', () => {
    const liveCollector = makeCollector({ name: 'Live gRPC Collector', endpoint_grpc: '10.0.0.1:4317' })
    mockUseCollectors.mockReturnValue({
      data: [liveCollector],
      isLoading: false,
      isError: false,
    } as any)

    render(<EnvironmentsPage />)
    expect(screen.getByText('Live gRPC Collector')).toBeDefined()
    expect(screen.getByText('10.0.0.1:4317')).toBeDefined()
  })

  it('falls back to static collectors when API returns empty array', () => {
    mockUseCollectors.mockReturnValue({
      data: [],
      isLoading: false,
      isError: false,
    } as any)

    render(<EnvironmentsPage />)
    // Static fallback includes the default collector names
    expect(screen.getByText('gRPC OTLP Receiver')).toBeDefined()
    // Fallback badge is shown
    expect(screen.getByText(/static config/i)).toBeDefined()
  })

  it('falls back to static collectors when API errors', () => {
    mockUseCollectors.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
    } as any)

    render(<EnvironmentsPage />)
    expect(screen.getByText(/static config/i)).toBeDefined()
  })

  it('renders collector status badge', () => {
    mockUseCollectors.mockReturnValue({
      data: [makeCollector({ status: 'degraded' })],
      isLoading: false,
      isError: false,
    } as any)

    render(<EnvironmentsPage />)
    expect(screen.getByText('degraded')).toBeDefined()
  })
})

// ─── String-format API response ───────────────────────────────────────────────

describe('EnvironmentsPage — normalises string[] API response', () => {
  it('accepts a plain string array from the API (legacy format)', () => {
    // API may return plain strings instead of EnvRecord objects
    mockUseQuery.mockReturnValue({ data: ['production', 'staging'], isLoading: false } as any)
    mockUseCollectors.mockReturnValue({
      data: [makeCollector()],
      isLoading: false,
      isError: false,
    } as any)

    render(<EnvironmentsPage />)
    expect(screen.getByText('Production')).toBeDefined()
    expect(screen.getByText('Staging')).toBeDefined()
  })
})

// ─── RBAC — page is visible to all authenticated roles ───────────────────────

describe('EnvironmentsPage — RBAC visibility', () => {
  // EnvironmentsPage has no role gate — it must be accessible to viewer, editor, admin.
  // This test verifies the page renders without crashing for any role by checking
  // that the heading is always present (role enforcement is at route level in App.tsx).
  it('renders successfully (no role gate inside the component)', () => {
    mockUseQuery.mockReturnValue({ data: [], isLoading: false } as any)
    mockUseCollectors.mockReturnValue({ data: [], isLoading: false, isError: false } as any)

    render(<EnvironmentsPage />)
    expect(screen.getByText('Environments')).toBeDefined()
  })
})

// ─── Quick-start section ─────────────────────────────────────────────────────

describe('EnvironmentsPage — quick-start section', () => {
  it('renders the quick-start heading', () => {
    mockUseQuery.mockReturnValue({ data: [], isLoading: false } as any)
    mockUseCollectors.mockReturnValue({ data: [], isLoading: false, isError: false } as any)

    render(<EnvironmentsPage />)
    expect(screen.getByText(/quick start/i)).toBeDefined()
  })

  it('shows pip install agentfabric in the quick-start code block', () => {
    mockUseQuery.mockReturnValue({ data: [], isLoading: false } as any)
    mockUseCollectors.mockReturnValue({ data: [], isLoading: false, isError: false } as any)

    render(<EnvironmentsPage />)
    expect(screen.getByText(/pip install agentfabric/i)).toBeDefined()
  })
})
