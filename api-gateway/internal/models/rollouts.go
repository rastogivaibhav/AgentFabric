package models

import "time"

const (
	RolloutTargetModel         = "model"
	RolloutTargetPromptRelease = "prompt_release"
	RolloutTargetPolicyRule    = "policy_rule"

	RolloutStatusActive = "active"
	RolloutStatusPaused = "paused"
)

type RolloutRule struct {
	ID                  int64             `json:"id"`
	TenantID            string            `json:"tenant_id,omitempty"`
	Name                string            `json:"name"`
	TargetType          string            `json:"target_type"`
	TargetID            string            `json:"target_id"`
	Environment         string            `json:"environment,omitempty"`
	Percentage          int               `json:"percentage"`
	ControlModel        string            `json:"control_model,omitempty"`
	CandidateModel      string            `json:"candidate_model,omitempty"`
	ControlReleaseTag   string            `json:"control_release_tag,omitempty"`
	CandidateReleaseTag string            `json:"candidate_release_tag,omitempty"`
	PolicyRuleID        int64             `json:"policy_rule_id,omitempty"`
	Conditions          map[string]string `json:"conditions,omitempty"`
	RollbackCriteria    map[string]string `json:"rollback_criteria,omitempty"`
	Status              string            `json:"status,omitempty"`
	CreatedBy           string            `json:"created_by,omitempty"`
	UpdatedBy           string            `json:"updated_by,omitempty"`
	CreatedAt           time.Time         `json:"created_at"`
	UpdatedAt           time.Time         `json:"updated_at"`
	RecentRequests      int64             `json:"recent_requests,omitempty"`
	RecentFailures      int64             `json:"recent_failures,omitempty"`
	RecentErrorRate     float64           `json:"recent_error_rate,omitempty"`
	LastEventAt         *time.Time        `json:"last_event_at,omitempty"`
}

type RolloutEvent struct {
	ID                int64     `json:"id"`
	TenantID          string    `json:"tenant_id,omitempty"`
	RolloutRuleID     int64     `json:"rollout_rule_id"`
	TraceID           string    `json:"trace_id,omitempty"`
	SpanID            string    `json:"span_id,omitempty"`
	TargetType        string    `json:"target_type"`
	AssignedVariant   string    `json:"assigned_variant"`
	AssignmentKey     string    `json:"assignment_key,omitempty"`
	Provider          string    `json:"provider,omitempty"`
	Model             string    `json:"model,omitempty"`
	Environment       string    `json:"environment,omitempty"`
	PromptID          string    `json:"prompt_id,omitempty"`
	PromptReleaseTag  string    `json:"prompt_release_tag,omitempty"`
	Status            string    `json:"status"`
	StatusCode        int       `json:"status_code"`
	CostUSD           float64   `json:"cost_usd,omitempty"`
	LatencyMS         int64     `json:"latency_ms,omitempty"`
	ErrorRateSnapshot float64   `json:"error_rate_snapshot,omitempty"`
	AutoPaused        bool      `json:"auto_paused,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

type RolloutPreviewRequest struct {
	TenantID          string `json:"tenant_id,omitempty"`
	Provider          string `json:"provider,omitempty"`
	Model             string `json:"model,omitempty"`
	Environment       string `json:"environment,omitempty"`
	App               string `json:"app,omitempty"`
	Session           string `json:"session,omitempty"`
	PromptID          string `json:"prompt_id,omitempty"`
	PromptEnvironment string `json:"prompt_environment,omitempty"`
	AssignmentKey     string `json:"assignment_key,omitempty"`
	PolicyRuleID      int64  `json:"policy_rule_id,omitempty"`
}

type RolloutAssignment struct {
	Selected          bool              `json:"selected"`
	RuleID            int64             `json:"rule_id,omitempty"`
	RuleName          string            `json:"rule_name,omitempty"`
	TargetType        string            `json:"target_type,omitempty"`
	TargetID          string            `json:"target_id,omitempty"`
	Variant           string            `json:"variant,omitempty"`
	AssignmentKey     string            `json:"assignment_key,omitempty"`
	Bucket            int               `json:"bucket,omitempty"`
	ControlModel      string            `json:"control_model,omitempty"`
	CandidateModel    string            `json:"candidate_model,omitempty"`
	ReleaseTag        string            `json:"release_tag,omitempty"`
	Metadata          map[string]string `json:"metadata,omitempty"`
	RollbackTriggered bool              `json:"rollback_triggered,omitempty"`
}

type RolloutPreviewResponse struct {
	Assignment RolloutAssignment `json:"assignment"`
	Rules      []RolloutRule     `json:"rules"`
}

type RolloutStatusUpdateRequest struct {
	Status string `json:"status"`
}
