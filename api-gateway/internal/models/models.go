package models

import "time"

// ─── Span ────────────────────────────────────────────────────────────────────

type Span struct {
	ID           string            `json:"id" db:"span_id"`
	TraceID      string            `json:"trace_id" db:"trace_id"`
	ParentID     string            `json:"parent_id,omitempty" db:"parent_span_id"`
	RunID        string            `json:"run_id" db:"run_id"`
	Name         string            `json:"name" db:"name"`
	Framework    string            `json:"framework" db:"framework"`
	StartTimeNs  int64             `json:"start_time_ns" db:"start_time_ns"`
	DurationNs   int64             `json:"duration_ns" db:"duration_ns"`
	StatusCode   int               `json:"status_code" db:"status_code"`
	StatusMsg    string            `json:"status_msg,omitempty" db:"status_msg"`
	Attributes   map[string]string `json:"attributes" db:"-"`
	Events       []SpanEvent       `json:"events,omitempty" db:"-"`
	InputTokens  int64             `json:"input_tokens,omitempty" db:"input_tokens"`
	OutputTokens int64             `json:"output_tokens,omitempty" db:"output_tokens"`
	CostUSD      float64           `json:"cost_usd,omitempty" db:"cost_usd"`
	TenantID     string            `json:"-" db:"tenant_id"`
	ReceivedAt   time.Time         `json:"received_at" db:"received_at"`
}

type SpanEvent struct {
	Name       string            `json:"name"`
	TimeNs     int64             `json:"time_ns"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// ─── Trace ───────────────────────────────────────────────────────────────────

type Trace struct {
	ID           string    `json:"id"`
	RootSpanName string    `json:"root_span_name"`
	Framework    string    `json:"framework"`
	StartTime    time.Time `json:"start_time"`
	Duration     int64     `json:"duration_ns"`
	SpanCount    int       `json:"span_count"`
	ErrorCount   int       `json:"error_count"`
	TotalCostUSD float64   `json:"total_cost_usd"`
	TotalTokens  int64     `json:"total_tokens"`
	Status       string    `json:"status"` // ok|error|partial
	Spans        []Span    `json:"spans,omitempty"`
	TenantID     string    `json:"-"`
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
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Framework   string    `json:"framework"`
	FirstSeen   time.Time `json:"first_seen"`
	LastSeen    time.Time `json:"last_seen"`
	RunCount    int       `json:"run_count"`
	TotalCost   float64   `json:"total_cost_usd"`
	P50LatencyMs float64  `json:"p50_latency_ms"`
	P95LatencyMs float64  `json:"p95_latency_ms"`
	P99LatencyMs float64  `json:"p99_latency_ms"`
	ErrorRate   float64   `json:"error_rate"`
	TenantID    string    `json:"-"`
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
	TotalTraces     int64   `json:"total_traces"`
	ActiveAgents    int     `json:"active_agents"`
	TotalCostUSD    float64 `json:"total_cost_usd"`
	TotalTokens     int64   `json:"total_tokens"`
	ErrorRate       float64 `json:"error_rate"`
	AvgLatencyMs    float64 `json:"avg_latency_ms"`
	SpansPerSecond  float64 `json:"spans_per_second"`
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

// ─── Live stream event ───────────────────────────────────────────────────────

type LiveEvent struct {
	Type      string      `json:"type"` // span|run_start|run_end|error|policy
	Timestamp int64       `json:"ts"`
	TenantID  string      `json:"-"`
	Data      interface{} `json:"data"`
}
