package normalization

import (
	"time"
)

// CanonicalEvent represents a vendor-neutral AI tool event
type CanonicalEvent struct {
	ID                 string
	EventTime          time.Time
	SourceTool         string // codex, claude_code, crewai, etc. (legacy, use SourceVendor)
	SourceVendor       string // codex, cursor, vscode, anthropic, cowork
	SourceProduct      string // codex-cli, cursor-editor, github-copilot, claude-api
	SourceChannel      string // otlp, extension, api, webhook
	EventType          string // session.started, tool.call.completed, etc.
	EventCategory      string // session, model_call, tool_call, approval
	Action             string // code_generation_accepted, refactor, etc.
	Severity           string // info, warning, error, critical
	UserID             string
	UserEmail          string
	OrgID              string
	TeamID             string
	SessionID          string
	TraceID            string
	SpanID             string
	RepoURL            string
	RepoName           string
	GitBranch          string
	GitCommit          string
	ModelName          string
	Provider           string // openai, anthropic, google
	ToolName           string
	Command            string
	CommandHash        string
	FilePath           string
	Success            bool
	LatencyMs          int64
	InputTokens        int64
	OutputTokens       int64
	CacheReadTokens    int64
	CacheWriteTokens   int64
	EstimatedCost      float64
	RiskScore          int
	RiskCategory       string // unsafe_command, secret_exposure, prod_edit
	RequiresReview     bool
	PromptRedacted     bool
	Payload            map[string]interface{}
	Redacted           bool
	RawEvent           interface{} // raw span data for storage
}

// EnrichedSpan mirrors collector's EnrichedSpan for use in api-gateway
// This prevents circular imports and allows independent evolution
type EnrichedSpan struct {
	TraceID          string            `json:"trace_id"`
	SpanID           string            `json:"span_id"`
	ParentSpanID     string            `json:"parent_span_id,omitempty"`
	Name             string            `json:"name"`
	Framework        string            `json:"framework"`
	StartTimeNs      uint64            `json:"start_time_ns"`
	DurationNs       uint64            `json:"duration_ns"`
	StatusCode       int32             `json:"status_code"`
	StatusMsg        string            `json:"status_msg,omitempty"`
	Attributes       map[string]string `json:"attributes"`
	Events           []interface{}     `json:"events,omitempty"`
	CollectorNode    string            `json:"collector_node"`
	ReceivedNs       int64             `json:"received_ns"`
	RunID            string            `json:"run_id"`
	InputTokens      int64             `json:"input_tokens,omitempty"`
	OutputTokens     int64             `json:"output_tokens,omitempty"`
	CacheReadTokens  int64             `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int64             `json:"cache_write_tokens,omitempty"`
	ReasoningTokens  int64             `json:"reasoning_tokens,omitempty"`
}
