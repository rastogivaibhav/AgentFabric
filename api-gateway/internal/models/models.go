package models

import "time"

// ─── Span ────────────────────────────────────────────────────────────────────

type Span struct {
	ID              string            `json:"span_id" db:"span_id"`
	TraceID         string            `json:"trace_id" db:"trace_id"`
	ParentID        string            `json:"parent_span_id,omitempty" db:"parent_span_id"`
	RunID           string            `json:"run_id" db:"run_id"`
	Name            string            `json:"name" db:"name"`
	Framework       string            `json:"framework" db:"framework"`
	StartTimeNs     int64             `json:"start_time_ns" db:"start_time_ns"`
	DurationNs      int64             `json:"duration_ns" db:"duration_ns"`
	StatusCode      int               `json:"status_code" db:"status_code"`
	StatusMsg       string            `json:"status_msg,omitempty" db:"status_msg"`
	Attributes      map[string]string `json:"attributes" db:"-"`
	Events          []SpanEvent       `json:"events,omitempty" db:"-"`
	InputTokens     int64             `json:"input_tokens,omitempty" db:"input_tokens"`
	OutputTokens    int64             `json:"output_tokens,omitempty" db:"output_tokens"`
	CostUSD         float64           `json:"cost_usd,omitempty" db:"cost_usd"`
	Depth           int               `json:"depth,omitempty" db:"-"`
	StepType        string            `json:"step_type,omitempty" db:"-"`
	Provider        string            `json:"provider,omitempty" db:"-"`
	Model           string            `json:"model,omitempty" db:"-"`
	AppName         string            `json:"app_name,omitempty" db:"-"`
	Environment     string            `json:"environment,omitempty" db:"-"`
	UserID          string            `json:"user_id,omitempty" db:"-"`
	SessionID       string            `json:"session_id,omitempty" db:"-"`
	ErrorClass      string            `json:"error_class,omitempty" db:"-"`
	PromptPreview   string            `json:"prompt_preview,omitempty" db:"-"`
	ResponsePreview string            `json:"response_preview,omitempty" db:"-"`
	RetryCount      int               `json:"retry_count,omitempty" db:"-"`
	Blocked         bool              `json:"blocked,omitempty" db:"-"`
	BlockedReason   string            `json:"blocked_reason,omitempty" db:"-"`
	PricingRuleID   int64             `json:"pricing_rule_id,omitempty" db:"-"`
	PricingScope    string            `json:"pricing_scope,omitempty" db:"-"`
	TenantID        string            `json:"-" db:"tenant_id"`
	ReceivedAt      time.Time         `json:"received_at" db:"received_at"`
}

type SpanEvent struct {
	Name       string            `json:"name"`
	TimeNs     int64             `json:"time_ns"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// ─── Trace ───────────────────────────────────────────────────────────────────

type Trace struct {
	ID           string        `json:"id"`
	RootSpanName string        `json:"root_span_name"`
	Framework    string        `json:"framework"`
	StartTime    time.Time     `json:"start_time"`
	Duration     int64         `json:"duration_ns"`
	SpanCount    int           `json:"span_count"`
	ErrorCount   int           `json:"error_count"`
	TotalCostUSD float64       `json:"total_cost_usd"`
	TotalTokens  int64         `json:"total_tokens"`
	Status       string        `json:"status"` // ok|error|partial
	Insights     TraceInsights `json:"insights,omitempty"`
	Spans        []Span        `json:"spans,omitempty"`
	TenantID     string        `json:"-"`
}

type TraceInsights struct {
	Models       []string       `json:"models,omitempty"`
	Providers    []string       `json:"providers,omitempty"`
	Apps         []string       `json:"apps,omitempty"`
	Environments []string       `json:"environments,omitempty"`
	StepTypes    map[string]int `json:"step_types,omitempty"`
	ErrorClasses map[string]int `json:"error_classes,omitempty"`
	LLMCalls     int            `json:"llm_calls,omitempty"`
	ToolCalls    int            `json:"tool_calls,omitempty"`
	BlockedSpans int            `json:"blocked_spans,omitempty"`
	RetryCount   int            `json:"retry_count,omitempty"`
	MaxDepth     int            `json:"max_depth,omitempty"`
}

// ─── Run ─────────────────────────────────────────────────────────────────────

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
	Framework string
	Model     string
	AgentName string
	Status    string
	StartTime int64
	EndTime   int64
	Limit     int
	Cursor    string
	TenantID  string
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
	Username     string
	Email        string
	DisplayName  string
	Role         string
	PasswordHash string
}

type PricingRule struct {
	ID               int64      `json:"id"`
	TenantID         *string    `json:"tenant_id,omitempty"`
	Provider         string     `json:"provider"`
	ModelPattern     string     `json:"model_pattern"`
	InputPerMillion  float64    `json:"input_per_million"`
	OutputPerMillion float64    `json:"output_per_million"`
	Active           bool       `json:"active"`
	Priority         int        `json:"priority"`
	EffectiveFrom    *time.Time `json:"effective_from,omitempty"`
	EffectiveTo      *time.Time `json:"effective_to,omitempty"`
	Description      string     `json:"description,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type PricingPreviewRequest struct {
	TenantID     string     `json:"tenant_id,omitempty"`
	Provider     string     `json:"provider"`
	Model        string     `json:"model"`
	InputTokens  int64      `json:"input_tokens"`
	OutputTokens int64      `json:"output_tokens"`
	At           *time.Time `json:"at,omitempty"`
}

type PricingPreviewResponse struct {
	Matched          bool       `json:"matched"`
	RuleID           int64      `json:"rule_id,omitempty"`
	Provider         string     `json:"provider,omitempty"`
	Model            string     `json:"model,omitempty"`
	ModelPattern     string     `json:"model_pattern,omitempty"`
	PricingScope     string     `json:"pricing_scope,omitempty"`
	InputPerMillion  float64    `json:"input_per_million,omitempty"`
	OutputPerMillion float64    `json:"output_per_million,omitempty"`
	InputTokens      int64      `json:"input_tokens"`
	OutputTokens     int64      `json:"output_tokens"`
	InputCostUSD     float64    `json:"input_cost_usd"`
	OutputCostUSD    float64    `json:"output_cost_usd"`
	TotalCostUSD     float64    `json:"total_cost_usd"`
	EffectiveFrom    *time.Time `json:"effective_from,omitempty"`
	EffectiveTo      *time.Time `json:"effective_to,omitempty"`
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
	ID           int64     `json:"id"`
	TenantID     *string   `json:"tenant_id,omitempty"`
	Name         string    `json:"name"`
	RuleType     string    `json:"rule_type"`
	Enabled      bool      `json:"enabled"`
	Priority     int       `json:"priority"`
	Action       string    `json:"action"`
	Provider     string    `json:"provider,omitempty"`
	ModelPattern string    `json:"model_pattern,omitempty"`
	Environment  string    `json:"environment,omitempty"`
	MaxTokens    int64     `json:"max_tokens,omitempty"`
	Detector     string    `json:"detector,omitempty"`
	Scope        string    `json:"scope,omitempty"`
	Description  string    `json:"description,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
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
