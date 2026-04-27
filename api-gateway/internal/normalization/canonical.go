package normalization

import (
	"time"
)

// CanonicalEvent represents a vendor-neutral AI tool event
type CanonicalEvent struct {
	EventTime       time.Time
	SourceTool      string // codex, claude_code, crewai, etc.
	EventType       string // session.started, tool.call.completed, etc.
	Severity        string // info, warning, error, critical
	UserID          string
	UserEmail       string
	OrgID           string
	TeamID          string
	SessionID       string
	TraceID         string
	SpanID          string
	RepoURL         string
	RepoName        string
	GitBranch       string
	GitCommit       string
	ModelName       string
	Provider        string // openai, anthropic, google
	ToolName        string
	Command         string
	CommandHash     string
	FilePath        string
	RiskScore       int
	RequiresReview  bool
	PromptRedacted  bool
	RawEvent        interface{} // raw span data for storage
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
