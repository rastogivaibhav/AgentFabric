import { useEffect, useRef, useCallback, useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'

const BASE = import.meta.env.VITE_API_URL ?? 'http://localhost:8080'

export interface SupportedProviderOption {
  value: string
  label: string
  route_hint: string
  key_placeholder: string
}

export const SUPPORTED_KEY_PROVIDERS: SupportedProviderOption[] = [
  {
    value: 'openai',
    label: 'OpenAI',
    route_hint: '/proxy/openai/v1/chat/completions',
    key_placeholder: 'sk-...',
  },
  {
    value: 'anthropic',
    label: 'Anthropic',
    route_hint: '/proxy/anthropic/v1/messages',
    key_placeholder: 'sk-ant-...',
  },
  {
    value: 'google',
    label: 'Google Gemini',
    route_hint: '/proxy/google/v1beta/models/gemini-1.5-pro:generateContent',
    key_placeholder: 'AIza...',
  },
  {
    value: 'vertexai',
    label: 'Google Vertex AI',
    route_hint: '/proxy/vertexai/v1/projects/demo/locations/us-central1/publishers/google/models/gemini-1.5-pro:generateContent',
    key_placeholder: 'ya29....',
  },
  {
    value: 'bedrock',
    label: 'AWS Bedrock',
    route_hint: '/proxy/bedrock/model/anthropic.claude-3-5-sonnet-20240620-v1:0/invoke',
    key_placeholder: 'temporary bearer token',
  },
]

// apiFetch uses credentials:'include' so the browser automatically sends the
// af_token HttpOnly cookie on every request. The raw token is never read by JS.
// The api-gateway JWTAuth middleware accepts this cookie as a valid auth source.
// CLI / API callers may still use Authorization: Bearer by calling the gateway directly.
export async function apiFetch<T>(path: string, params?: Record<string, string>): Promise<T> {
  const url = new URL(BASE + '/api/v1' + path)
  if (params) Object.entries(params).forEach(([k, v]) => v && url.searchParams.set(k, v))
  const res = await fetch(url.toString(), {
    headers: { 'Content-Type': 'application/json' },
    credentials: 'include', // sends af_token HttpOnly cookie; JS never reads the token
  })
  if (!res.ok) throw new Error(`API ${res.status}: ${path}`)
  return res.json()
}

export type OutcomeStatus = 'ok' | 'blocked' | 'error' | 'degraded'

// ─── Types ────────────────────────────────────────────────────────────────────

export interface Span {
  id: string
  trace_id: string
  parent_id?: string
  run_id: string
  name: string
  framework: string
  start_time_ns: number
  duration_ns: number
  status_code: number
  status_msg?: string
  outcome_status?: OutcomeStatus
  attributes: Record<string, string>
  events?: SpanEvent[]
  input_tokens?: number
  output_tokens?: number
  cache_read_tokens?: number
  cache_write_tokens?: number
  reasoning_tokens?: number
  cost_usd?: number
  input_cost_usd?: number
  output_cost_usd?: number
  cache_read_cost_usd?: number
  cache_write_cost_usd?: number
  reasoning_cost_usd?: number
  depth?: number
  step_type?: string
  provider?: string
  model?: string
  app_name?: string
  environment?: string
  user_id?: string
  session_id?: string
  prompt_id?: string
  prompt_version?: number
  prompt_release_tag?: string
  prompt_environment?: string
  error_class?: string
  prompt_preview?: string
  response_preview?: string
  retry_count?: number
  blocked?: boolean
  blocked_reason?: string
  parent_name?: string
  lineage?: string[]
  failure_summary?: string
  redaction_count?: number
  policy_decision_count?: number
  policy_decision_summary?: string[]
  pricing_rule_id?: number
  pricing_scope?: string
  pricing_model_pattern?: string
  received_at: string
}

export interface SpanEvent {
  name: string
  time_ns: number
  attributes?: Record<string, string>
}

export interface Trace {
  id: string
  root_span_name: string
  framework: string
  start_time: string
  duration_ns: number
  span_count: number
  error_count: number
  total_cost_usd: number
  total_tokens: number
  status: 'ok' | 'error' | 'partial'
  insights?: TraceInsights
  timeline?: TraceTimeline
  spans?: Span[]
  policy_events?: PolicyEvent[]
}

export interface TraceInsights {
  models?: string[]
  providers?: string[]
  apps?: string[]
  environments?: string[]
  step_types?: Record<string, number>
  error_classes?: Record<string, number>
  policy_results?: Record<string, number>
  llm_calls?: number
  tool_calls?: number
  blocked_spans?: number
  redacted_spans?: number
  failed_spans?: number
  retry_count?: number
  max_depth?: number
  workflow_summary?: string[]
}

export interface TraceTimeline {
  trace_id: string
  start_time: string
  duration_ns: number
  items: TraceTimelineItem[]
  highlights?: string[]
  policy_event_ids?: string[]
}

export interface TraceTimelineItem {
  span_id: string
  parent_span_id?: string
  name: string
  step_type?: string
  provider?: string
  model?: string
  app_name?: string
  environment?: string
  status: string
  failure_summary?: string
  blocked?: boolean
  blocked_reason?: string
  redaction_count?: number
  depth: number
  lineage?: string[]
  start_offset_ns: number
  end_offset_ns: number
  duration_ns: number
  total_tokens: number
  cost_usd?: number
  policy_event_count?: number
}

export interface TraceComparisonSide {
  trace_id: string
  root_span_name: string
  framework: string
  start_time: string
  duration_ns: number
  status: string
  span_count: number
  error_count: number
  total_cost_usd: number
  total_tokens: number
  retry_count?: number
  blocked_spans?: number
  redacted_spans?: number
  failed_spans?: number
  models?: string[]
  providers?: string[]
  workflow_summary?: string[]
}

export interface TraceComparisonDiff {
  field: string
  left: string
  right: string
  severity: string
}

export interface TraceComparison {
  left: TraceComparisonSide
  right: TraceComparisonSide
  diffs?: TraceComparisonDiff[]
  highlights?: string[]
}

export interface TraceSavedView {
  id: number
  name: string
  description?: string
  filters: Record<string, string>
  created_by?: string
  is_pinned?: boolean
  created_at: string
  updated_at: string
}

export interface Page<T> {
  items: T[]
  total: number
  next_cursor?: string
  has_more: boolean
}

export interface LimitedPage<T> {
  items: T[]
  count: number
  limit: number
}

export interface OverviewStats {
  total_traces: number
  active_agents: number
  total_cost_usd: number
  total_tokens: number
  error_rate: number
  avg_latency_ms: number
  spans_per_second: number
  blocked_requests: number
  llm_calls: number
  tool_calls: number
  framework_counts: Record<string, number>
}

export interface AgentScoreComponent {
  key: string
  label: string
  score: number
  weight: number
  severity: string
  summary: string
}

export interface AgentScoreTrend {
  previous_overall_score: number
  current_overall_score: number
  delta: number
  direction: string
}

export interface AgentScorecard {
  agent_id: string
  agent_name: string
  framework: string
  app_name?: string
  environment?: string
  release_tag?: string
  run_count: number
  total_cost_usd: number
  total_tokens: number
  avg_latency_ms: number
  eval_count: number
  overall_score: number
  risk_level: string
  components: AgentScoreComponent[]
  trend: AgentScoreTrend
  recommended_actions?: string[]
  generated_at: string
}

export interface Recommendation {
  id: number
  key?: string
  tenant_id?: string
  type: 'rollout' | 'routing' | 'policy' | 'cost' | string
  status: 'open' | 'reviewing' | 'applied' | 'dismissed' | 'resolved' | string
  title: string
  summary: string
  target: string
  target_type?: string
  target_id?: string
  suggested_action: string
  estimated_impact?: string
  blast_radius?: string
  confidence: number
  evidence?: Record<string, unknown>
  created_at: string
  updated_at: string
  last_seen_at: string
}

export interface LiveEvent {
  type: 'span' | 'run_start' | 'run_end' | 'error' | 'policy'
  ts: number
  data: Span | PolicyLiveEventData | Record<string, unknown>
}

export interface PolicyLiveEventData {
  decision_id: string
  trace_id?: string
  span_id?: string
  policy_name: string
  result: 'allow' | 'deny' | 'warn' | 'sanitize'
  reason: string
  tenant_id: string
  provider?: string
  model?: string
  scope?: string
  matched?: string[]
  redactions?: number
}

export interface PolicyEvent {
  decision_id: string
  trace_id?: string
  span_id?: string
  policy_name: string
  result: 'allow' | 'deny' | 'warn' | 'sanitize' | 'redact'
  reason: string
  tenant_id: string
  provider?: string
  model?: string
  scope?: string
  matched?: string[]
  redactions?: number
}

export interface DecisionRecord {
  id: number
  decision_id: string
  tenant_id?: string
  trace_id?: string
  span_id?: string
  type: 'policy' | 'budget' | 'fallback' | 'routing' | 'retry' | string
  result: string
  reason?: string
  explanation?: string
  trigger?: string
  inputs?: Record<string, string>
  evidence?: Record<string, unknown>
  action_taken?: string
  source?: string
  framework?: string
  app_name?: string
  environment?: string
  provider?: string
  model?: string
  prompt_id?: string
  prompt_release_tag?: string
  created_at: string
}

// ─── Hooks ────────────────────────────────────────────────────────────────────

export interface EnvironmentInfo {
  name: string
  status: string
  span_count: number
}

function isTruthyOutcomeAttr(value?: string) {
  switch ((value ?? '').trim().toLowerCase()) {
    case '1':
    case 'true':
    case 'yes':
    case 'blocked':
      return true
    default:
      return false
  }
}

export function getSpanOutcomeStatus(span: Pick<Span, 'status_code' | 'attributes' | 'blocked' | 'outcome_status'>): OutcomeStatus {
  const explicit = span.outcome_status ?? span.attributes?.['af.outcome_status']
  if (explicit === 'ok' || explicit === 'blocked' || explicit === 'error' || explicit === 'degraded') {
    return explicit
  }
  if (span.blocked || isTruthyOutcomeAttr(span.attributes?.['af.policy.blocked']) || span.attributes?.['af.policy.decision']?.toLowerCase() === 'deny') {
    return 'blocked'
  }
  if ([401, 403, 429].includes(span.status_code)) {
    return 'blocked'
  }
  if (span.status_code === 2 || span.status_code >= 500) {
    return 'error'
  }
  if (span.attributes?.['af.gateway.route_source']?.toLowerCase() === 'fallback') {
    return 'degraded'
  }
  return 'ok'
}

export function outcomeStatusColor(status: string) {
  if (status === 'error') return '#EF4444'
  if (status === 'blocked' || status === 'degraded' || status === 'partial') return '#F59E0B'
  return '#10B981'
}

export function useOverview(since = '24h') {
  return useQuery<OverviewStats>({
    queryKey: ['overview', since],
    queryFn: () => apiFetch('/analytics/overview', { since }),
    refetchInterval: 30_000,
  })
}

export function useAgentScorecards(since = '24h', limit = 24) {
  return useQuery<Page<AgentScorecard>>({
    queryKey: ['agent-scorecards', since, limit],
    queryFn: () => apiFetch('/agents/scorecards', { since, limit: String(limit) }),
    refetchInterval: 30_000,
  })
}

export function useAgentScorecard(agentId: string, since = '24h') {
  return useQuery<AgentScorecard>({
    queryKey: ['agent-scorecard', agentId, since],
    queryFn: () => apiFetch(`/agents/${encodeURIComponent(agentId)}/scorecard`, { since }),
    enabled: !!agentId,
    refetchInterval: 30_000,
  })
}

export function useRecommendations(params?: { since?: string; limit?: number; status?: string; type?: string }) {
  const normalized = {
    since: params?.since ?? '24h',
    limit: String(params?.limit ?? 12),
    status: params?.status ?? '',
    type: params?.type ?? '',
  }
  return useQuery<Page<Recommendation>>({
    queryKey: ['recommendations', normalized],
    queryFn: () => apiFetch('/recommendations', normalized),
    refetchInterval: 60_000,
  })
}

export function useTraces(params: Record<string, string> = {}) {
  return useQuery<Page<Trace>>({
    queryKey: ['traces', params],
    queryFn: () => apiFetch('/traces', params),
  })
}

export function useTrace(traceId: string) {
  return useQuery<Trace>({
    queryKey: ['trace', traceId],
    queryFn: () => apiFetch(`/traces/${traceId}`),
    enabled: !!traceId,
  })
}

export function useDecisions(params: Record<string, string> = {}) {
  return useQuery<Page<DecisionRecord>>({
    queryKey: ['decisions', params],
    queryFn: () => apiFetch('/decisions', params),
  })
}

export function useTraceDecisions(traceId: string) {
  return useQuery<{ items: DecisionRecord[]; count: number }>({
    queryKey: ['trace-decisions', traceId],
    queryFn: () => apiFetch(`/traces/${traceId}/decisions`),
    enabled: !!traceId,
  })
}

export function useTraceTimeline(traceId: string) {
  return useQuery<TraceTimeline>({
    queryKey: ['trace-timeline', traceId],
    queryFn: () => apiFetch(`/traces/${traceId}/timeline`),
    enabled: !!traceId,
  })
}

export function useTraceGraph(traceId: string) {
  return useQuery({
    queryKey: ['trace-graph', traceId],
    queryFn: () => apiFetch(`/traces/${traceId}/graph`),
    enabled: !!traceId,
  })
}

export function useTraceComparison(left?: string, right?: string) {
  return useQuery<TraceComparison>({
    queryKey: ['trace-compare', left, right],
    queryFn: () => apiFetch('/traces/compare', { left: left ?? '', right: right ?? '' }),
    enabled: Boolean(left && right),
  })
}

export function useTraceSavedViews() {
  return useQuery<TraceSavedView[]>({
    queryKey: ['trace-saved-views'],
    queryFn: () => apiFetch('/traces/saved-views'),
  })
}

export function useFrameworkStats() {
  return useQuery({
    queryKey: ['framework-stats'],
    queryFn: () => apiFetch('/analytics/frameworks'),
    refetchInterval: 60_000,
  })
}

export interface CollectorInfo {
  id: string
  name: string
  endpoint_grpc: string
  endpoint_http: string
  status: 'healthy' | 'degraded' | 'unreachable' | string
  version: string
  last_checked?: string
}

export function useCollectors() {
  return useQuery<CollectorInfo[]>({
    queryKey: ['collectors'],
    queryFn: async () => {
      try {
        const result = await apiFetch<CollectorInfo[]>('/collectors')
        return result
      } catch (err: unknown) {
        // If the backend endpoint is not yet implemented (404), return empty array
        // so the UI can fall back to static hardcoded cards gracefully
        const msg = err instanceof Error ? err.message : String(err)
        if (msg.includes('404')) return []
        throw err
      }
    },
    refetchInterval: 30_000,
    retry: false,
  })
}

// ─── WebSocket hook ───────────────────────────────────────────────────────────

export function useEnvironments() {
  return useQuery<EnvironmentInfo[] | string[]>({
    queryKey: ['environments'],
    queryFn: () => apiFetch('/environments'),
    refetchInterval: 30_000,
  })
}

export function useLiveStream(maxEvents = 500) {
  const [events, setEvents] = useState<LiveEvent[]>([])
  const [connected, setConnected] = useState(false)
  const wsRef = useRef<WebSocket | null>(null)
  const pausedRef = useRef(false)
  // Lifecycle guard — prevents reconnect attempts after the component unmounts.
  const mountedRef = useRef(true)
  // Exponential backoff state: retry 0 → 1s, 1 → 2s, 2 → 4s … capped at 30s.
  const retryCountRef = useRef(0)
  const retryTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const connect = useCallback(() => {
    if (!mountedRef.current) return

    const wsBase = BASE.replace(/^http/, 'ws')
    const ws = new WebSocket(`${wsBase}/api/v1/stream/live`)
    wsRef.current = ws

    ws.onopen = () => {
      if (!mountedRef.current) { ws.close(); return }
      setConnected(true)
      retryCountRef.current = 0 // reset backoff on successful connection
    }

    ws.onclose = () => {
      if (!mountedRef.current) return
      setConnected(false)
      // Exponential backoff: 1s, 2s, 4s, 8s, 16s, 30s (max)
      const delay = Math.min(30_000, 1_000 * Math.pow(2, retryCountRef.current))
      retryCountRef.current = Math.min(retryCountRef.current + 1, 5)
      retryTimerRef.current = setTimeout(connect, delay)
    }

    ws.onerror = () => ws.close()

    ws.onmessage = (e) => {
      if (pausedRef.current || !mountedRef.current) return
      try {
        const event: LiveEvent = JSON.parse(e.data)
        setEvents(prev => [event, ...prev].slice(0, maxEvents))
      } catch {}
    }
  }, [maxEvents])

  useEffect(() => {
    mountedRef.current = true
    connect()
    return () => {
      mountedRef.current = false
      if (retryTimerRef.current) clearTimeout(retryTimerRef.current)
      wsRef.current?.close()
    }
  }, [connect])

  const pause = () => { pausedRef.current = true }
  const resume = () => { pausedRef.current = false }
  const clear = () => setEvents([])

  return { events, connected, pause, resume, clear }
}

// ─── Budget hooks ─────────────────────────────────────────────────────────────

export interface Budget {
  tenant_id: string
  monthly_tokens: number    // 0 = unlimited
  monthly_cost_usd: number  // 0 = unlimited
  alert_threshold: number   // 0.80 = 80%
  hard_limit: boolean
  reset_day: number
  created_at?: string
  updated_at?: string
}

export interface BudgetUsage {
  tokens_used: number
  cost_used_usd: number
  period_start: string
  period_end: string
  tokens_pct?: number
  cost_pct?: number
  budget?: Budget
}

export interface CostReportRow {
  app_name: string
  environment: string
  provider: string
  model: string
  prompt_id: string
  release_tag: string
  total_tokens: number
  total_cost_usd: number
  input_tokens: number
  output_tokens: number
  cache_read_tokens: number
  cache_write_tokens: number
  reasoning_tokens: number
  input_cost_usd: number
  output_cost_usd: number
  cache_read_cost_usd: number
  cache_write_cost_usd: number
  reasoning_cost_usd: number
  trace_count: number
  blocked_count: number
}

export interface CostReportFilters {
  since?: string
  app_name?: string
  environment?: string
  provider?: string
  model?: string
  prompt_id?: string
  release_tag?: string
  limit?: string
}

export interface CostSpikeRow {
  app_name: string
  environment: string
  provider: string
  model: string
  prompt_id: string
  release_tag: string
  current_cost_usd: number
  previous_cost_usd: number
  delta_cost_usd: number
  delta_pct: number
  current_trace_count: number
  previous_trace_count: number
  current_total_tokens: number
  previous_total_tokens: number
  explanation: string
}

export interface CostContributorRow {
  key: string
  current_cost_usd: number
  previous_cost_usd: number
  delta_cost_usd: number
  delta_pct: number
  share_of_delta: number
}

export interface CostContributorGroup {
  dimension: string
  items: CostContributorRow[]
}

export interface CostSpikeReport {
  current_window_start: string
  current_window_end: string
  previous_window_start: string
  previous_window_end: string
  filters_applied: Record<string, string>
  spikes: CostSpikeRow[]
  contributor_groups: CostContributorGroup[]
}

function normalizeCostFilters(filters?: CostReportFilters) {
  return {
    since: filters?.since ?? '24h',
    app_name: filters?.app_name ?? '',
    environment: filters?.environment ?? '',
    provider: filters?.provider ?? '',
    model: filters?.model ?? '',
    prompt_id: filters?.prompt_id ?? '',
    release_tag: filters?.release_tag ?? '',
    limit: filters?.limit ?? '100',
  }
}

export function useCostReport(filters?: CostReportFilters) {
  const normalized = normalizeCostFilters(filters)
  return useQuery<CostReportRow[]>({
    queryKey: ['cost-report', normalized],
    queryFn: () => apiFetch('/analytics/cost', normalized),
    refetchInterval: 60_000,
  })
}

export function useCostSpikes(filters?: CostReportFilters) {
  const normalized = normalizeCostFilters(filters)
  return useQuery<CostSpikeReport>({
    queryKey: ['cost-spikes', normalized],
    queryFn: () => apiFetch('/analytics/cost/spikes', normalized),
    refetchInterval: 60_000,
  })
}

export interface PricingRule {
  id?: number
  tenant_id?: string | null
  provider: string
  model_pattern: string
  input_per_million: number
  output_per_million: number
  cache_read_per_million?: number
  cache_write_per_million?: number
  reasoning_per_million?: number
  active?: boolean
  priority?: number
  effective_from?: string | null
  effective_to?: string | null
  description?: string
  created_at?: string
  updated_at?: string
}

export interface PricingPreviewRequest {
  tenant_id?: string
  provider: string
  model: string
  input_tokens: number
  output_tokens: number
  cache_read_tokens?: number
  cache_write_tokens?: number
  reasoning_tokens?: number
  at?: string
}

export interface PricingPreviewResponse {
  matched: boolean
  rule_id?: number
  provider?: string
  model?: string
  model_pattern?: string
  pricing_scope?: string
  input_per_million?: number
  output_per_million?: number
  cache_read_per_million?: number
  cache_write_per_million?: number
  reasoning_per_million?: number
  input_tokens: number
  output_tokens: number
  cache_read_tokens?: number
  cache_write_tokens?: number
  reasoning_tokens?: number
  input_cost_usd: number
  output_cost_usd: number
  cache_read_cost_usd?: number
  cache_write_cost_usd?: number
  reasoning_cost_usd?: number
  total_cost_usd: number
  explain?: string[]
  effective_from?: string | null
  effective_to?: string | null
}

export interface PricingAuditEntry {
  id: number
  rule_id: number
  action: string
  actor: string
  tenant_id?: string
  before_state?: string
  after_state?: string
  created_at: string
}

export interface PolicyRule {
  id?: number
  tenant_id?: string | null
  name: string
  rule_type: 'traffic' | 'dlp'
  decision_mode?: 'fast' | 'rego'
  enabled?: boolean
  priority?: number
  action: 'allow' | 'warn' | 'redact' | 'deny'
  provider?: string
  model_pattern?: string
  environment?: string
  max_tokens?: number
  detector?: string
  scope?: 'request' | 'response' | 'both'
  guardrails?: string[]
  schema_json?: string
  unsafe_categories?: string[]
  rollout_percent?: number
  version?: number
  rule_conditions?: Record<string, string>
  rego_module?: string
  description?: string
  created_at?: string
  updated_at?: string
}

export interface PolicyPreviewRequest {
  tenant_id?: string
  provider: string
  model: string
  environment?: string
  estimated_tokens?: number
  actor?: string
  app?: string
  session?: string
  request_body?: string
  response_body?: string
}

export interface PolicyPreviewDecision {
  matched: boolean
  rule_id?: number
  policy_name?: string
  action?: string
  reason?: string
  scope?: string
  matched_names?: string[]
  guardrail_matches?: string[]
  redactions?: number
  redacted_preview?: string
  final?: boolean
  engine?: string
  decision_mode?: string
  version?: number
  rollout_percent?: number
  evaluation_path?: string[]
  matched_fields?: string[]
  condition_trace?: Array<{
    field: string
    operator: string
    expected?: string
    actual?: string
    matched: boolean
    source?: string
  }>
  rego_query?: string
  explain?: string
  rule_conditions?: Record<string, string>
}

export interface PolicyPreviewResponse {
  traffic: PolicyPreviewDecision
  request_dlp: PolicyPreviewDecision
  response_dlp: PolicyPreviewDecision
}

export interface PolicySimulationSample {
  label?: string
  tenant_id?: string
  provider: string
  model: string
  environment?: string
  estimated_tokens?: number
  actor?: string
  app?: string
  session?: string
  request_body?: string
  response_body?: string
}

export interface PolicySimulationResult {
  label?: string
  traffic: PolicyPreviewDecision
  request_dlp: PolicyPreviewDecision
  response_dlp: PolicyPreviewDecision
}

export interface PolicySimulationResponse {
  count: number
  results: PolicySimulationResult[]
}

export interface PolicyEffectivenessSummary {
  total_events: number
  allows: number
  denies: number
  warns: number
  redacts: number
  blocked_spans: number
  redacted_spans: number
  covered_llm_calls: number
  total_llm_calls: number
  coverage_ratio: number
  prevented_failures?: number
}

export interface TraceEvalScore {
  metric: string
  score: number
  weight: number
  severity?: string
  summary?: string
}

export interface TraceEvalRun {
  id: number
  trace_id: string
  prompt_id?: string
  prompt_version?: number
  prompt_environment?: string
  release_tag?: string
  eval_suite: string
  overall_score: number
  risk_level: string
  summary?: string
  policy_effectiveness: PolicyEffectivenessSummary
  created_at: string
  scores?: TraceEvalScore[]
}

export interface TraceEvalRequest {
  trace_id: string
  release_tag?: string
  eval_suite?: string
}

export interface RegressionCompareRequest {
  baseline_tag: string
  candidate_tag: string
  prompt_id?: string
  environment?: string
  eval_suite?: string
}

export interface RegressionMetricDelta {
  metric: string
  baseline_score: number
  candidate_score: number
  delta: number
  severity: string
  summary?: string
}

export interface RegressionReport {
  baseline_tag: string
  candidate_tag: string
  prompt_id?: string
  environment?: string
  eval_suite: string
  compared_runs: number
  overall_delta: number
  risk_level: string
  highlights?: string[]
  metrics?: RegressionMetricDelta[]
  generated_at: string
}

export interface PromptReleaseEvalSummary {
  eval_count: number
  average_score: number
  latest_score: number
  risk_level: string
  last_evaluated_at?: string
}

export interface PromptReleaseRegressionSummary {
  baseline_tag: string
  candidate_tag: string
  compared_runs: number
  overall_delta: number
  risk_level: string
  highlights?: string[]
  summary?: string
}

export interface PromptRelease {
  id: number
  tenant_id?: string
  prompt_id: string
  environment: string
  version: number
  release_tag: string
  status?: string
  notes?: string
  promotion_reason?: string
  promoted_by?: string
  created_at: string
  eval_summary: PromptReleaseEvalSummary
  regression_summary?: PromptReleaseRegressionSummary
}

export interface PromptVersion {
  id?: number
  tenant_id?: string
  prompt_id: string
  version?: number
  environment: string
  release_tag?: string
  content: string
  config?: Record<string, string>
  description?: string
  created_by?: string
  created_at?: string
  updated_at?: string
  is_latest?: boolean
  promoted?: boolean
  current_release?: PromptRelease
}

export interface PromptCatalog {
  items: PromptVersion[]
  releases: PromptRelease[]
  count: number
}

export interface PromptPromotionRequest {
  prompt_id: string
  environment: string
  version: number
  release_tag: string
  status?: string
  notes?: string
  promotion_reason?: string
}

export interface RolloutRule {
  id?: number
  tenant_id?: string
  name: string
  target_type: 'model' | 'prompt_release' | 'policy_rule'
  target_id: string
  environment?: string
  percentage: number
  control_model?: string
  candidate_model?: string
  control_release_tag?: string
  candidate_release_tag?: string
  policy_rule_id?: number
  conditions?: Record<string, string>
  rollback_criteria?: Record<string, string>
  status?: 'active' | 'paused'
  created_by?: string
  updated_by?: string
  created_at?: string
  updated_at?: string
  recent_requests?: number
  recent_failures?: number
  recent_error_rate?: number
  last_event_at?: string
}

export interface RolloutAssignment {
  selected: boolean
  rule_id?: number
  rule_name?: string
  target_type?: string
  target_id?: string
  variant?: string
  assignment_key?: string
  bucket?: number
  control_model?: string
  candidate_model?: string
  release_tag?: string
  metadata?: Record<string, string>
  rollback_triggered?: boolean
}

export interface RolloutPreviewRequest {
  tenant_id?: string
  provider?: string
  model?: string
  environment?: string
  app?: string
  session?: string
  prompt_id?: string
  prompt_environment?: string
  assignment_key?: string
  policy_rule_id?: number
}

export interface RolloutPreviewResponse {
  assignment: RolloutAssignment
  rules: RolloutRule[]
}

export interface AdminAuditEntry {
  id: number
  tenant_id?: string
  actor?: string
  category: string
  action: string
  target_type: string
  target_id?: string
  outcome: string
  details?: string
  created_at: string
}

export interface ControlHistoryEntry {
  id: number
  tenant_id?: string
  category: string
  action: string
  target_type: string
  target_id?: string
  actor?: string
  reason?: string
  outcome: string
  before_state?: string
  after_state?: string
  evidence_refs?: string[]
  previous_hash?: string
  entry_hash?: string
  created_at: string
}

export interface EvidenceBundleItem {
  id: number
  bundle_id: number
  tenant_id?: string
  item_type: string
  item_title: string
  trace_id?: string
  target_type?: string
  target_id?: string
  payload: string
  created_at: string
}

export interface EvidenceBundle {
  id: number
  tenant_id?: string
  name: string
  scope: string
  status: string
  filters?: string
  summary?: string[]
  created_by?: string
  created_at: string
  item_count: number
  items?: EvidenceBundleItem[]
}

export interface EvidenceBundleRequest {
  name: string
  scope: string
  trace_id?: string
  prompt_id?: string
  environment?: string
  release_tag?: string
  rollout_rule_id?: number
  recommendation_id?: number
  reason?: string
}

export async function apiMutate<T>(path: string, method: string, body?: unknown): Promise<T> {
  const res = await fetch(BASE + '/api/v1' + path, {
    method,
    headers: { 'Content-Type': 'application/json' },
    credentials: 'include',
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })
  if (!res.ok) throw new Error(`API ${res.status}: ${path}`)
  if (res.status === 204) return undefined as T
  return res.json()
}

export function useUpsertTraceSavedView() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (view: Partial<TraceSavedView> & { name: string; filters: Record<string, string> }) =>
      apiMutate<TraceSavedView>('/traces/saved-views', 'PUT', view),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['trace-saved-views'] })
    },
  })
}

export function useDeleteTraceSavedView() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => apiMutate<void>(`/traces/saved-views/${id}`, 'DELETE'),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['trace-saved-views'] })
    },
  })
}

export function useBudget(tenantId: string) {
  return useQuery<Budget>({
    queryKey: ['budget', tenantId],
    queryFn: () => apiFetch(`/budgets/${tenantId}`),
    retry: false,
    enabled: !!tenantId,
  })
}

export function useBudgetUsage(tenantId: string) {
  return useQuery<BudgetUsage>({
    queryKey: ['budget-usage', tenantId],
    queryFn: () => apiFetch(`/budgets/${tenantId}/usage`),
    retry: false,
    enabled: !!tenantId,
    refetchInterval: 30_000,
  })
}

export function useUpsertBudget(tenantId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (b: Partial<Budget>) => apiMutate<Budget>(`/budgets/${tenantId}`, 'PUT', b),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['budget', tenantId] })
      qc.invalidateQueries({ queryKey: ['budget-usage', tenantId] })
    },
  })
}

export function useDeleteBudget(tenantId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => apiMutate<void>(`/budgets/${tenantId}`, 'DELETE'),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['budget', tenantId] })
      qc.invalidateQueries({ queryKey: ['budget-usage', tenantId] })
    },
  })
}

export function usePricingRules() {
  return useQuery<{ items: PricingRule[]; count: number }>({
    queryKey: ['pricing-rules'],
    queryFn: () => apiFetch('/pricing'),
    retry: false,
  })
}

export function useUpsertPricingRule() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (rule: PricingRule) => apiMutate<PricingRule>('/pricing', 'PUT', rule),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['pricing-rules'] })
      qc.invalidateQueries({ queryKey: ['control-history'] })
      qc.invalidateQueries({ queryKey: ['control-audit'] })
    },
  })
}

export function useDeletePricingRule() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => apiMutate<void>(`/pricing/${id}`, 'DELETE'),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['pricing-rules'] })
      qc.invalidateQueries({ queryKey: ['control-history'] })
      qc.invalidateQueries({ queryKey: ['control-audit'] })
    },
  })
}

export function usePreviewPricingRule() {
  return useMutation({
    mutationFn: (req: PricingPreviewRequest) => apiMutate<PricingPreviewResponse>('/pricing/preview', 'POST', req),
  })
}

export function usePricingRuleAudit(limit = 50) {
  return useQuery<{ items: PricingAuditEntry[]; count: number }>({
    queryKey: ['pricing-rule-audit', limit],
    queryFn: () => apiFetch('/pricing/audit', { limit: String(limit) }),
    retry: false,
  })
}

export function usePolicyRules() {
  return useQuery<{ items: PolicyRule[]; count: number }>({
    queryKey: ['policy-rules'],
    queryFn: () => apiFetch('/policies'),
    retry: false,
  })
}

export function useUpsertPolicyRule() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (rule: PolicyRule) => apiMutate<PolicyRule>('/policies', 'PUT', rule),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['policy-rules'] })
      qc.invalidateQueries({ queryKey: ['control-audit'] })
      qc.invalidateQueries({ queryKey: ['control-history'] })
    },
  })
}

export function useDeletePolicyRule() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => apiMutate<void>(`/policies/${id}`, 'DELETE'),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['policy-rules'] })
      qc.invalidateQueries({ queryKey: ['control-audit'] })
      qc.invalidateQueries({ queryKey: ['control-history'] })
    },
  })
}

export function usePreviewPolicyRule() {
  return useMutation({
    mutationFn: (req: PolicyPreviewRequest) => apiMutate<PolicyPreviewResponse>('/policies/preview', 'POST', req),
  })
}

export function useSimulatePolicyRule() {
  return useMutation({
    mutationFn: (samples: PolicySimulationSample[]) =>
      apiMutate<PolicySimulationResponse>('/policies/simulate', 'POST', { samples }),
  })
}

export function useEvalRuns(limit = 20) {
  return useQuery<Page<TraceEvalRun>>({
    queryKey: ['eval-runs', limit],
    queryFn: () => apiFetch('/evals', { limit: String(limit) }),
    retry: false,
  })
}

export function useScoreTraceEval() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (req: TraceEvalRequest) => apiMutate<TraceEvalRun>('/evals/score', 'POST', req),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['eval-runs'] })
    },
  })
}

export function useCompareEvalRegressions() {
  return useMutation({
    mutationFn: (req: RegressionCompareRequest) => apiMutate<RegressionReport>('/evals/regressions', 'POST', req),
  })
}

export function useControlAudit(limit = 50) {
  return useQuery<LimitedPage<AdminAuditEntry>>({
    queryKey: ['control-audit', limit],
    queryFn: () => apiFetch('/audit/control', { limit: String(limit) }),
    retry: false,
  })
}

export function useControlHistory(limit = 50, offset = 0, category = '', targetID = '') {
  return useQuery<Page<ControlHistoryEntry>>({
    queryKey: ['control-history', limit, offset, category, targetID],
    queryFn: () => apiFetch('/audit/control-history', {
      limit: String(limit),
      offset: String(offset),
      category,
      target_id: targetID,
    }),
    retry: false,
  })
}

export function useEvidenceBundles(limit = 25) {
  return useQuery<LimitedPage<EvidenceBundle>>({
    queryKey: ['evidence-bundles', limit],
    queryFn: () => apiFetch('/audit/evidence-bundles', { limit: String(limit) }),
    retry: false,
  })
}

export function useCreateEvidenceBundle() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (req: EvidenceBundleRequest) => apiMutate<EvidenceBundle>('/audit/evidence-bundles', 'POST', req),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['evidence-bundles'] })
      qc.invalidateQueries({ queryKey: ['control-history'] })
      qc.invalidateQueries({ queryKey: ['control-audit'] })
    },
  })
}

export function usePrompts() {
  return useQuery<PromptCatalog>({
    queryKey: ['prompts'],
    queryFn: () => apiFetch('/prompts'),
    retry: false,
  })
}

export function useUpsertPromptVersion() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (version: PromptVersion) => apiMutate<PromptVersion>('/prompts', 'PUT', version),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['prompts'] })
      qc.invalidateQueries({ queryKey: ['control-audit'] })
      qc.invalidateQueries({ queryKey: ['control-history'] })
    },
  })
}

export function usePromotePromptRelease() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (req: PromptPromotionRequest) => apiMutate<PromptRelease>('/prompts/promote', 'POST', req),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['prompts'] })
      qc.invalidateQueries({ queryKey: ['control-audit'] })
      qc.invalidateQueries({ queryKey: ['control-history'] })
    },
  })
}

export function useRollouts() {
  return useQuery<{ items: RolloutRule[]; count: number }>({
    queryKey: ['rollouts'],
    queryFn: () => apiFetch('/rollouts'),
    retry: false,
  })
}

export function useUpsertRolloutRule() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (rule: RolloutRule) => apiMutate<RolloutRule>('/rollouts', 'PUT', rule),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['rollouts'] })
      qc.invalidateQueries({ queryKey: ['prompts'] })
      qc.invalidateQueries({ queryKey: ['policy-rules'] })
      qc.invalidateQueries({ queryKey: ['control-audit'] })
      qc.invalidateQueries({ queryKey: ['control-history'] })
    },
  })
}

export function usePreviewRollout() {
  return useMutation({
    mutationFn: (req: RolloutPreviewRequest) => apiMutate<RolloutPreviewResponse>('/rollouts/preview', 'POST', req),
  })
}

export function useUpdateRolloutStatus() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, status }: { id: number; status: 'active' | 'paused' }) => apiMutate<RolloutRule>(`/rollouts/${id}/status`, 'POST', { status }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['rollouts'] })
      qc.invalidateQueries({ queryKey: ['prompts'] })
      qc.invalidateQueries({ queryKey: ['policy-rules'] })
      qc.invalidateQueries({ queryKey: ['control-audit'] })
      qc.invalidateQueries({ queryKey: ['control-history'] })
    },
  })
}

export function useUpdateRecommendationStatus() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, status }: { id: number; status: 'open' | 'reviewing' | 'applied' | 'dismissed' | 'resolved' }) =>
      apiMutate<Recommendation>(`/recommendations/${id}/status`, 'POST', { status }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['recommendations'] })
      qc.invalidateQueries({ queryKey: ['control-history'] })
      qc.invalidateQueries({ queryKey: ['control-audit'] })
    },
  })
}
