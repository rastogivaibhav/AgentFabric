package models

import "time"

// ─── Span ────────────────────────────────────────────────────────────────────

type Span struct {
	ID                    string            `json:"span_id" db:"span_id"`
	TraceID               string            `json:"trace_id" db:"trace_id"`
	ParentID              string            `json:"parent_span_id,omitempty" db:"parent_span_id"`
	RunID                 string            `json:"run_id" db:"run_id"`
	Name                  string            `json:"name" db:"name"`
	Framework             string            `json:"framework" db:"framework"`
	StartTimeNs           int64             `json:"start_time_ns" db:"start_time_ns"`
	DurationNs            int64             `json:"duration_ns" db:"duration_ns"`
	StatusCode            int               `json:"status_code" db:"status_code"`
	StatusMsg             string            `json:"status_msg,omitempty" db:"status_msg"`
	Attributes            map[string]string `json:"attributes" db:"-"`
	Events                []SpanEvent       `json:"events,omitempty" db:"-"`
	InputTokens           int64             `json:"input_tokens,omitempty" db:"input_tokens"`
	OutputTokens          int64             `json:"output_tokens,omitempty" db:"output_tokens"`
	CacheReadTokens       int64             `json:"cache_read_tokens,omitempty" db:"cache_read_tokens"`
	CacheWriteTokens      int64             `json:"cache_write_tokens,omitempty" db:"cache_write_tokens"`
	ReasoningTokens       int64             `json:"reasoning_tokens,omitempty" db:"reasoning_tokens"`
	CostUSD               float64           `json:"cost_usd,omitempty" db:"cost_usd"`
	InputCostUSD          float64           `json:"input_cost_usd,omitempty" db:"input_cost_usd"`
	OutputCostUSD         float64           `json:"output_cost_usd,omitempty" db:"output_cost_usd"`
	CacheReadCostUSD      float64           `json:"cache_read_cost_usd,omitempty" db:"cache_read_cost_usd"`
	CacheWriteCostUSD     float64           `json:"cache_write_cost_usd,omitempty" db:"cache_write_cost_usd"`
	ReasoningCostUSD      float64           `json:"reasoning_cost_usd,omitempty" db:"reasoning_cost_usd"`
	Depth                 int               `json:"depth,omitempty" db:"-"`
	StepType              string            `json:"step_type,omitempty" db:"-"`
	Provider              string            `json:"provider,omitempty" db:"-"`
	Model                 string            `json:"model,omitempty" db:"-"`
	AppName               string            `json:"app_name,omitempty" db:"-"`
	Environment           string            `json:"environment,omitempty" db:"-"`
	UserID                string            `json:"user_id,omitempty" db:"-"`
	SessionID             string            `json:"session_id,omitempty" db:"-"`
	PromptID              string            `json:"prompt_id,omitempty" db:"-"`
	PromptVersion         int               `json:"prompt_version,omitempty" db:"-"`
	PromptReleaseTag      string            `json:"prompt_release_tag,omitempty" db:"-"`
	PromptEnvironment     string            `json:"prompt_environment,omitempty" db:"-"`
	ErrorClass            string            `json:"error_class,omitempty" db:"-"`
	PromptPreview         string            `json:"prompt_preview,omitempty" db:"-"`
	ResponsePreview       string            `json:"response_preview,omitempty" db:"-"`
	RetryCount            int               `json:"retry_count,omitempty" db:"-"`
	OutcomeStatus         string            `json:"outcome_status,omitempty" db:"-"`
	Blocked               bool              `json:"blocked,omitempty" db:"-"`
	BlockedReason         string            `json:"blocked_reason,omitempty" db:"-"`
	ParentName            string            `json:"parent_name,omitempty" db:"-"`
	Lineage               []string          `json:"lineage,omitempty" db:"-"`
	FailureSummary        string            `json:"failure_summary,omitempty" db:"-"`
	RedactionCount        int               `json:"redaction_count,omitempty" db:"-"`
	PolicyDecisionCount   int               `json:"policy_decision_count,omitempty" db:"-"`
	PolicyDecisionSummary []string          `json:"policy_decision_summary,omitempty" db:"-"`
	PricingRuleID         int64             `json:"pricing_rule_id,omitempty" db:"-"`
	PricingScope          string            `json:"pricing_scope,omitempty" db:"-"`
	PricingModelPattern   string            `json:"pricing_model_pattern,omitempty" db:"-"`
	TenantID              string            `json:"-" db:"tenant_id"`
	ReceivedAt            time.Time         `json:"received_at" db:"received_at"`
}

type SpanEvent struct {
	Name       string            `json:"name"`
	TimeNs     int64             `json:"time_ns"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// ─── Trace ───────────────────────────────────────────────────────────────────

type Trace struct {
	ID           string         `json:"id"`
	RootSpanName string         `json:"root_span_name"`
	Framework    string         `json:"framework"`
	StartTime    time.Time      `json:"start_time"`
	Duration     int64          `json:"duration_ns"`
	SpanCount    int            `json:"span_count"`
	ErrorCount   int            `json:"error_count"`
	TotalCostUSD float64        `json:"total_cost_usd"`
	TotalTokens  int64          `json:"total_tokens"`
	Status       string         `json:"status"` // ok|error|partial
	Insights     TraceInsights  `json:"insights,omitempty"`
	Timeline     *TraceTimeline `json:"timeline,omitempty"`
	Spans        []Span         `json:"spans,omitempty"`
	PolicyEvents []PolicyEvent  `json:"policy_events,omitempty"`
	TenantID     string         `json:"-"`
}

type TraceInsights struct {
	Models          []string       `json:"models,omitempty"`
	Providers       []string       `json:"providers,omitempty"`
	Apps            []string       `json:"apps,omitempty"`
	Environments    []string       `json:"environments,omitempty"`
	StepTypes       map[string]int `json:"step_types,omitempty"`
	ErrorClasses    map[string]int `json:"error_classes,omitempty"`
	PolicyResults   map[string]int `json:"policy_results,omitempty"`
	LLMCalls        int            `json:"llm_calls,omitempty"`
	ToolCalls       int            `json:"tool_calls,omitempty"`
	BlockedSpans    int            `json:"blocked_spans,omitempty"`
	RedactedSpans   int            `json:"redacted_spans,omitempty"`
	FailedSpans     int            `json:"failed_spans,omitempty"`
	RetryCount      int            `json:"retry_count,omitempty"`
	MaxDepth        int            `json:"max_depth,omitempty"`
	WorkflowSummary []string       `json:"workflow_summary,omitempty"`
}

type TraceTimeline struct {
	TraceID        string              `json:"trace_id"`
	StartTime      time.Time           `json:"start_time"`
	DurationNs     int64               `json:"duration_ns"`
	Items          []TraceTimelineItem `json:"items"`
	Highlights     []string            `json:"highlights,omitempty"`
	PolicyEventIDs []string            `json:"policy_event_ids,omitempty"`
}

type TraceTimelineItem struct {
	SpanID           string   `json:"span_id"`
	ParentSpanID     string   `json:"parent_span_id,omitempty"`
	Name             string   `json:"name"`
	StepType         string   `json:"step_type,omitempty"`
	Provider         string   `json:"provider,omitempty"`
	Model            string   `json:"model,omitempty"`
	AppName          string   `json:"app_name,omitempty"`
	Environment      string   `json:"environment,omitempty"`
	Status           string   `json:"status"`
	FailureSummary   string   `json:"failure_summary,omitempty"`
	Blocked          bool     `json:"blocked,omitempty"`
	BlockedReason    string   `json:"blocked_reason,omitempty"`
	RedactionCount   int      `json:"redaction_count,omitempty"`
	Depth            int      `json:"depth"`
	Lineage          []string `json:"lineage,omitempty"`
	StartOffsetNs    int64    `json:"start_offset_ns"`
	EndOffsetNs      int64    `json:"end_offset_ns"`
	DurationNs       int64    `json:"duration_ns"`
	TotalTokens      int64    `json:"total_tokens"`
	CostUSD          float64  `json:"cost_usd,omitempty"`
	PolicyEventCount int      `json:"policy_event_count,omitempty"`
}

type TraceComparison struct {
	Left       TraceComparisonSide   `json:"left"`
	Right      TraceComparisonSide   `json:"right"`
	Diffs      []TraceComparisonDiff `json:"diffs,omitempty"`
	Highlights []string              `json:"highlights,omitempty"`
}

type TraceComparisonSide struct {
	TraceID         string    `json:"trace_id"`
	RootSpanName    string    `json:"root_span_name"`
	Framework       string    `json:"framework"`
	StartTime       time.Time `json:"start_time"`
	DurationNs      int64     `json:"duration_ns"`
	Status          string    `json:"status"`
	SpanCount       int       `json:"span_count"`
	ErrorCount      int       `json:"error_count"`
	TotalCostUSD    float64   `json:"total_cost_usd"`
	TotalTokens     int64     `json:"total_tokens"`
	RetryCount      int       `json:"retry_count,omitempty"`
	BlockedSpans    int       `json:"blocked_spans,omitempty"`
	RedactedSpans   int       `json:"redacted_spans,omitempty"`
	FailedSpans     int       `json:"failed_spans,omitempty"`
	Models          []string  `json:"models,omitempty"`
	Providers       []string  `json:"providers,omitempty"`
	WorkflowSummary []string  `json:"workflow_summary,omitempty"`
}

type TraceComparisonDiff struct {
	Field    string `json:"field"`
	Left     string `json:"left"`
	Right    string `json:"right"`
	Severity string `json:"severity"`
}

type TraceSavedView struct {
	ID          int64             `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Filters     map[string]string `json:"filters"`
	CreatedBy   string            `json:"created_by,omitempty"`
	IsPinned    bool              `json:"is_pinned,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// ─── Run ─────────────────────────────────────────────────────────────────────

type PolicyEffectivenessSummary struct {
	TotalEvents       int     `json:"total_events"`
	Allows            int     `json:"allows"`
	Denies            int     `json:"denies"`
	Warns             int     `json:"warns"`
	Redacts           int     `json:"redacts"`
	BlockedSpans      int     `json:"blocked_spans"`
	RedactedSpans     int     `json:"redacted_spans"`
	CoveredLLMCalls   int     `json:"covered_llm_calls"`
	TotalLLMCalls     int     `json:"total_llm_calls"`
	CoverageRatio     float64 `json:"coverage_ratio"`
	PreventedFailures int     `json:"prevented_failures,omitempty"`
}

type TraceEvalScore struct {
	Metric   string  `json:"metric"`
	Score    float64 `json:"score"`
	Weight   float64 `json:"weight"`
	Severity string  `json:"severity,omitempty"`
	Summary  string  `json:"summary,omitempty"`
}

type TraceEvalRun struct {
	ID                  int64                      `json:"id"`
	TraceID             string                     `json:"trace_id"`
	ReleaseTag          string                     `json:"release_tag,omitempty"`
	EvalSuite           string                     `json:"eval_suite"`
	OverallScore        float64                    `json:"overall_score"`
	RiskLevel           string                     `json:"risk_level"`
	Summary             string                     `json:"summary,omitempty"`
	PolicyEffectiveness PolicyEffectivenessSummary `json:"policy_effectiveness"`
	CreatedAt           time.Time                  `json:"created_at"`
	Scores              []TraceEvalScore           `json:"scores,omitempty"`
}

type TraceEvalRequest struct {
	TraceID    string `json:"trace_id"`
	ReleaseTag string `json:"release_tag,omitempty"`
	EvalSuite  string `json:"eval_suite,omitempty"`
}

type RegressionCompareRequest struct {
	BaselineTag  string `json:"baseline_tag"`
	CandidateTag string `json:"candidate_tag"`
	EvalSuite    string `json:"eval_suite,omitempty"`
}

type RegressionMetricDelta struct {
	Metric         string  `json:"metric"`
	BaselineScore  float64 `json:"baseline_score"`
	CandidateScore float64 `json:"candidate_score"`
	Delta          float64 `json:"delta"`
	Severity       string  `json:"severity"`
	Summary        string  `json:"summary,omitempty"`
}

type RegressionReport struct {
	BaselineTag  string                  `json:"baseline_tag"`
	CandidateTag string                  `json:"candidate_tag"`
	EvalSuite    string                  `json:"eval_suite"`
	ComparedRuns int                     `json:"compared_runs"`
	OverallDelta float64                 `json:"overall_delta"`
	RiskLevel    string                  `json:"risk_level"`
	Highlights   []string                `json:"highlights,omitempty"`
	Metrics      []RegressionMetricDelta `json:"metrics,omitempty"`
	GeneratedAt  time.Time               `json:"generated_at"`
}

type PromptVersion struct {
	ID             int64             `json:"id"`
	TenantID       string            `json:"tenant_id,omitempty"`
	PromptID       string            `json:"prompt_id"`
	Version        int               `json:"version"`
	Environment    string            `json:"environment"`
	ReleaseTag     string            `json:"release_tag,omitempty"`
	Content        string            `json:"content"`
	Config         map[string]string `json:"config,omitempty"`
	Description    string            `json:"description,omitempty"`
	CreatedBy      string            `json:"created_by,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
	IsLatest       bool              `json:"is_latest,omitempty"`
	Promoted       bool              `json:"promoted,omitempty"`
	CurrentRelease *PromptRelease    `json:"current_release,omitempty"`
}

type PromptRelease struct {
	ID          int64     `json:"id"`
	TenantID    string    `json:"tenant_id,omitempty"`
	PromptID    string    `json:"prompt_id"`
	Environment string    `json:"environment"`
	Version     int       `json:"version"`
	ReleaseTag  string    `json:"release_tag"`
	Notes       string    `json:"notes,omitempty"`
	PromotedBy  string    `json:"promoted_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type PromptCatalog struct {
	Items    []PromptVersion `json:"items"`
	Releases []PromptRelease `json:"releases"`
	Count    int             `json:"count"`
}

type PromptPromotionRequest struct {
	PromptID    string `json:"prompt_id"`
	Environment string `json:"environment"`
	Version     int    `json:"version"`
	ReleaseTag  string `json:"release_tag"`
	Notes       string `json:"notes,omitempty"`
}

type Run struct {
	ID           string            `json:"id" db:"run_id"`
	TraceID      string            `json:"trace_id" db:"trace_id"`
	ParentRunID  string            `json:"parent_run_id,omitempty" db:"parent_run_id"`
	Framework    string            `json:"framework" db:"framework"`
	AgentName    string            `json:"agent_name" db:"agent_name"`
	Model        string            `json:"model" db:"model"`
	StartTime    time.Time         `json:"start_time" db:"start_time"`
	EndTime      *time.Time        `json:"end_time,omitempty" db:"end_time"`
	Status       string            `json:"status" db:"status"`
	TotalTokens  int64             `json:"total_tokens" db:"total_tokens"`
	TotalCostUSD float64           `json:"total_cost_usd" db:"total_cost_usd"`
	Metadata     map[string]string `json:"metadata,omitempty" db:"-"`
	TenantID     string            `json:"-" db:"tenant_id"`
}

// ─── Agent ───────────────────────────────────────────────────────────────────

type Agent struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Framework    string    `json:"framework"`
	FirstSeen    time.Time `json:"first_seen"`
	LastSeen     time.Time `json:"last_seen"`
	RunCount     int       `json:"run_count"`
	TotalCost    float64   `json:"total_cost_usd"`
	P50LatencyMs float64   `json:"p50_latency_ms"`
	P95LatencyMs float64   `json:"p95_latency_ms"`
	P99LatencyMs float64   `json:"p99_latency_ms"`
	ErrorRate    float64   `json:"error_rate"`
	TenantID     string    `json:"-"`
}

// ─── Topology ────────────────────────────────────────────────────────────────

type TopologyNode struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"` // agent|tool|llm|human
	Framework string `json:"framework"`
	SpanCount int    `json:"span_count"`
}

type TopologyEdge struct {
	From      string `json:"from"`
	To        string `json:"to"`
	EdgeType  string `json:"edge_type"` // call|return|conditional
	CallCount int    `json:"call_count"`
}

type TopologyGraph struct {
	Nodes []TopologyNode `json:"nodes"`
	Edges []TopologyEdge `json:"edges"`
}

// ─── Analytics ───────────────────────────────────────────────────────────────

type OverviewStats struct {
	TotalTraces     int64            `json:"total_traces"`
	ActiveAgents    int              `json:"active_agents"`
	TotalCostUSD    float64          `json:"total_cost_usd"`
	TotalTokens     int64            `json:"total_tokens"`
	ErrorRate       float64          `json:"error_rate"`
	AvgLatencyMs    float64          `json:"avg_latency_ms"`
	SpansPerSecond  float64          `json:"spans_per_second"`
	BlockedRequests int64            `json:"blocked_requests"`
	LLMCalls        int64            `json:"llm_calls"`
	ToolCalls       int64            `json:"tool_calls"`
	FrameworkCounts map[string]int64 `json:"framework_counts"`
}

// ─── Query params ────────────────────────────────────────────────────────────

type TraceQuery struct {
	Framework   string
	Model       string
	AgentName   string
	Search      string
	Provider    string
	AppName     string
	Environment string
	UserID      string
	SessionID   string
	Status      string
	BlockedOnly bool
	StartTime   int64
	EndTime     int64
	Limit       int
	Cursor      string
	TenantID    string
}

type RunQuery struct {
	TraceID   string
	AgentName string
	Framework string
	Limit     int
	Cursor    string
	TenantID  string
}

// ─── Pagination ──────────────────────────────────────────────────────────────

type Page[T any] struct {
	Items      []T    `json:"items"`
	Total      int64  `json:"total"`
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
}

// ─── User ─────────────────────────────────────────────────────────────────────

// User represents an AgentFabric platform user within a tenant.
// Password is never serialised to JSON — use CreateUserRequest/UpdateUserRequest for writes.
type User struct {
	ID          string     `json:"user_id"`
	TenantID    string     `json:"-"`
	Username    string     `json:"username"`
	Email       string     `json:"email"`
	DisplayName string     `json:"display_name"`
	Role        string     `json:"role"` // admin|editor|viewer
	IsActive    bool       `json:"is_active"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// CreateUserRequest is the payload for POST /api/v1/users.
type CreateUserRequest struct {
	Username    string `json:"username"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`     // admin|editor|viewer; default "viewer"
	Password    string `json:"password"` // plaintext — hashed by store before persistence
}

// UpdateUserRequest is the payload for PUT /api/v1/users/{userId}.
// Only non-nil fields are applied (partial update semantics).
type UpdateUserRequest struct {
	Email       *string `json:"email,omitempty"`
	DisplayName *string `json:"display_name,omitempty"`
	Role        *string `json:"role,omitempty"`
	IsActive    *bool   `json:"is_active,omitempty"`
	Password    *string `json:"password,omitempty"` // if set, rehashed and stored
}

// ─── Auth lookup ──────────────────────────────────────────────────────────────

// UserRecord is the auth-layer view of a user row — includes the password hash
// required for bcrypt comparison during password login.
// It is separate from User (which is the public API type, password excluded).
type UserRecord struct {
	ID           string
	TenantID     string
	Username     string
	Email        string
	DisplayName  string
	Role         string
	PasswordHash string
}

type PricingRule struct {
	ID                   int64      `json:"id"`
	TenantID             *string    `json:"tenant_id,omitempty"`
	Provider             string     `json:"provider"`
	ModelPattern         string     `json:"model_pattern"`
	InputPerMillion      float64    `json:"input_per_million"`
	OutputPerMillion     float64    `json:"output_per_million"`
	CacheReadPerMillion  float64    `json:"cache_read_per_million"`
	CacheWritePerMillion float64    `json:"cache_write_per_million"`
	ReasoningPerMillion  float64    `json:"reasoning_per_million"`
	Active               bool       `json:"active"`
	Priority             int        `json:"priority"`
	EffectiveFrom        *time.Time `json:"effective_from,omitempty"`
	EffectiveTo          *time.Time `json:"effective_to,omitempty"`
	Description          string     `json:"description,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

type PricingPreviewRequest struct {
	TenantID         string     `json:"tenant_id,omitempty"`
	Provider         string     `json:"provider"`
	Model            string     `json:"model"`
	InputTokens      int64      `json:"input_tokens"`
	OutputTokens     int64      `json:"output_tokens"`
	CacheReadTokens  int64      `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int64      `json:"cache_write_tokens,omitempty"`
	ReasoningTokens  int64      `json:"reasoning_tokens,omitempty"`
	At               *time.Time `json:"at,omitempty"`
}

type PricingPreviewResponse struct {
	Matched              bool       `json:"matched"`
	RuleID               int64      `json:"rule_id,omitempty"`
	Provider             string     `json:"provider,omitempty"`
	Model                string     `json:"model,omitempty"`
	ModelPattern         string     `json:"model_pattern,omitempty"`
	PricingScope         string     `json:"pricing_scope,omitempty"`
	InputPerMillion      float64    `json:"input_per_million,omitempty"`
	OutputPerMillion     float64    `json:"output_per_million,omitempty"`
	CacheReadPerMillion  float64    `json:"cache_read_per_million,omitempty"`
	CacheWritePerMillion float64    `json:"cache_write_per_million,omitempty"`
	ReasoningPerMillion  float64    `json:"reasoning_per_million,omitempty"`
	InputTokens          int64      `json:"input_tokens"`
	OutputTokens         int64      `json:"output_tokens"`
	CacheReadTokens      int64      `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens     int64      `json:"cache_write_tokens,omitempty"`
	ReasoningTokens      int64      `json:"reasoning_tokens,omitempty"`
	InputCostUSD         float64    `json:"input_cost_usd"`
	OutputCostUSD        float64    `json:"output_cost_usd"`
	CacheReadCostUSD     float64    `json:"cache_read_cost_usd,omitempty"`
	CacheWriteCostUSD    float64    `json:"cache_write_cost_usd,omitempty"`
	ReasoningCostUSD     float64    `json:"reasoning_cost_usd,omitempty"`
	TotalCostUSD         float64    `json:"total_cost_usd"`
	Explain              []string   `json:"explain,omitempty"`
	EffectiveFrom        *time.Time `json:"effective_from,omitempty"`
	EffectiveTo          *time.Time `json:"effective_to,omitempty"`
}

type PricingAuditEntry struct {
	ID          int64     `json:"id"`
	RuleID      int64     `json:"rule_id"`
	Action      string    `json:"action"`
	Actor       string    `json:"actor"`
	TenantID    string    `json:"tenant_id,omitempty"`
	BeforeState string    `json:"before_state,omitempty"`
	AfterState  string    `json:"after_state,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type PolicyRule struct {
	ID               int64             `json:"id"`
	TenantID         *string           `json:"tenant_id,omitempty"`
	Name             string            `json:"name"`
	RuleType         string            `json:"rule_type"`
	DecisionMode     string            `json:"decision_mode,omitempty"`
	Enabled          bool              `json:"enabled"`
	Priority         int               `json:"priority"`
	Action           string            `json:"action"`
	Provider         string            `json:"provider,omitempty"`
	ModelPattern     string            `json:"model_pattern,omitempty"`
	Environment      string            `json:"environment,omitempty"`
	MaxTokens        int64             `json:"max_tokens,omitempty"`
	Detector         string            `json:"detector,omitempty"`
	Scope            string            `json:"scope,omitempty"`
	Guardrails       []string          `json:"guardrails,omitempty"`
	SchemaJSON       string            `json:"schema_json,omitempty"`
	UnsafeCategories []string          `json:"unsafe_categories,omitempty"`
	RolloutPercent   int               `json:"rollout_percent,omitempty"`
	Version          int               `json:"version,omitempty"`
	RuleConditions   map[string]string `json:"rule_conditions,omitempty"`
	RegoModule       string            `json:"rego_module,omitempty"`
	Description      string            `json:"description,omitempty"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

type PolicyDecisionAudit struct {
	DecisionID  string    `json:"decision_id"`
	TraceID     string    `json:"trace_id"`
	SpanID      string    `json:"span_id"`
	PolicyName  string    `json:"policy_name"`
	Result      string    `json:"result"`
	Reason      string    `json:"reason"`
	TenantID    string    `json:"tenant_id"`
	EvaluatedAt time.Time `json:"evaluated_at"`
	Framework   string    `json:"framework,omitempty"`
	Model       string    `json:"model,omitempty"`
	Environment string    `json:"environment,omitempty"`
	CloudRegion string    `json:"cloud_region,omitempty"`
}

type PolicyEvent struct {
	DecisionID string   `json:"decision_id"`
	TraceID    string   `json:"trace_id,omitempty"`
	SpanID     string   `json:"span_id,omitempty"`
	PolicyName string   `json:"policy_name"`
	Result     string   `json:"result"`
	Reason     string   `json:"reason"`
	TenantID   string   `json:"tenant_id"`
	Provider   string   `json:"provider,omitempty"`
	Model      string   `json:"model,omitempty"`
	Scope      string   `json:"scope,omitempty"`
	Matched    []string `json:"matched,omitempty"`
	Redactions int      `json:"redactions,omitempty"`
}

type PolicyPreviewRequest struct {
	TenantID        string `json:"tenant_id,omitempty"`
	Provider        string `json:"provider"`
	Model           string `json:"model"`
	Environment     string `json:"environment,omitempty"`
	EstimatedTokens int64  `json:"estimated_tokens,omitempty"`
	Actor           string `json:"actor,omitempty"`
	App             string `json:"app,omitempty"`
	Session         string `json:"session,omitempty"`
	RequestBody     string `json:"request_body,omitempty"`
	ResponseBody    string `json:"response_body,omitempty"`
}

type PolicyPreviewDecision struct {
	Matched          bool                   `json:"matched"`
	RuleID           int64                  `json:"rule_id,omitempty"`
	PolicyName       string                 `json:"policy_name,omitempty"`
	Action           string                 `json:"action,omitempty"`
	Reason           string                 `json:"reason,omitempty"`
	Scope            string                 `json:"scope,omitempty"`
	MatchedNames     []string               `json:"matched_names,omitempty"`
	GuardrailMatches []string               `json:"guardrail_matches,omitempty"`
	Redactions       int                    `json:"redactions,omitempty"`
	RedactedPreview  string                 `json:"redacted_preview,omitempty"`
	Final            bool                   `json:"final,omitempty"`
	Engine           string                 `json:"engine,omitempty"`
	DecisionMode     string                 `json:"decision_mode,omitempty"`
	Version          int                    `json:"version,omitempty"`
	RolloutPercent   int                    `json:"rollout_percent,omitempty"`
	EvaluationPath   []string               `json:"evaluation_path,omitempty"`
	MatchedFields    []string               `json:"matched_fields,omitempty"`
	ConditionTrace   []PolicyConditionTrace `json:"condition_trace,omitempty"`
	RegoQuery        string                 `json:"rego_query,omitempty"`
	Explain          string                 `json:"explain,omitempty"`
	RuleConditions   map[string]string      `json:"rule_conditions,omitempty"`
}

type PolicyPreviewResponse struct {
	Traffic     PolicyPreviewDecision `json:"traffic"`
	RequestDLP  PolicyPreviewDecision `json:"request_dlp"`
	ResponseDLP PolicyPreviewDecision `json:"response_dlp"`
}

type PolicySimulationSample struct {
	Label           string `json:"label,omitempty"`
	TenantID        string `json:"tenant_id,omitempty"`
	Provider        string `json:"provider"`
	Model           string `json:"model"`
	Environment     string `json:"environment,omitempty"`
	EstimatedTokens int64  `json:"estimated_tokens,omitempty"`
	Actor           string `json:"actor,omitempty"`
	App             string `json:"app,omitempty"`
	Session         string `json:"session,omitempty"`
	RequestBody     string `json:"request_body,omitempty"`
	ResponseBody    string `json:"response_body,omitempty"`
}

type PolicySimulationRequest struct {
	Samples []PolicySimulationSample `json:"samples"`
}

type PolicySimulationResult struct {
	Label       string                `json:"label,omitempty"`
	Traffic     PolicyPreviewDecision `json:"traffic"`
	RequestDLP  PolicyPreviewDecision `json:"request_dlp"`
	ResponseDLP PolicyPreviewDecision `json:"response_dlp"`
}

type PolicySimulationResponse struct {
	Count   int                      `json:"count"`
	Results []PolicySimulationResult `json:"results"`
}

type PolicyConditionTrace struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`
	Matched  bool   `json:"matched"`
	Source   string `json:"source,omitempty"`
}

type AdminAuditEntry struct {
	ID         int64     `json:"id"`
	TenantID   string    `json:"tenant_id,omitempty"`
	Actor      string    `json:"actor,omitempty"`
	Category   string    `json:"category"`
	Action     string    `json:"action"`
	TargetType string    `json:"target_type"`
	TargetID   string    `json:"target_id,omitempty"`
	Outcome    string    `json:"outcome"`
	Details    string    `json:"details,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// ─── Live stream event ───────────────────────────────────────────────────────

type LiveEvent struct {
	Type      string      `json:"type"` // span|run_start|run_end|error|policy
	Timestamp int64       `json:"ts"`
	TenantID  string      `json:"-"`
	Data      interface{} `json:"data"`
}
