package models

import "time"

type ControlHistoryEntry struct {
	ID           int64     `json:"id"`
	TenantID     string    `json:"tenant_id,omitempty"`
	Category     string    `json:"category"`
	Action       string    `json:"action"`
	TargetType   string    `json:"target_type"`
	TargetID     string    `json:"target_id,omitempty"`
	Actor        string    `json:"actor,omitempty"`
	Reason       string    `json:"reason,omitempty"`
	Outcome      string    `json:"outcome"`
	BeforeState  string    `json:"before_state,omitempty"`
	AfterState   string    `json:"after_state,omitempty"`
	EvidenceRefs []string  `json:"evidence_refs,omitempty"`
	PreviousHash string    `json:"previous_hash,omitempty"`
	EntryHash    string    `json:"entry_hash,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type ControlHistoryQuery struct {
	TenantID string
	Category string
	TargetID string
	Limit    int
	Offset   int
}

type EvidenceBundleRequest struct {
	Name             string `json:"name"`
	Scope            string `json:"scope"`
	TraceID          string `json:"trace_id,omitempty"`
	PromptID         string `json:"prompt_id,omitempty"`
	Environment      string `json:"environment,omitempty"`
	ReleaseTag       string `json:"release_tag,omitempty"`
	RolloutRuleID    int64  `json:"rollout_rule_id,omitempty"`
	RecommendationID int64  `json:"recommendation_id,omitempty"`
	Reason           string `json:"reason,omitempty"`
}

type EvidenceBundle struct {
	ID        int64                `json:"id"`
	TenantID  string               `json:"tenant_id,omitempty"`
	Name      string               `json:"name"`
	Scope     string               `json:"scope"`
	Status    string               `json:"status"`
	Filters   string               `json:"filters,omitempty"`
	Summary   []string             `json:"summary,omitempty"`
	CreatedBy string               `json:"created_by,omitempty"`
	CreatedAt time.Time            `json:"created_at"`
	ItemCount int                  `json:"item_count"`
	Items     []EvidenceBundleItem `json:"items,omitempty"`
}

type EvidenceBundleItem struct {
	ID         int64     `json:"id"`
	BundleID   int64     `json:"bundle_id"`
	TenantID   string    `json:"tenant_id,omitempty"`
	ItemType   string    `json:"item_type"`
	ItemTitle  string    `json:"item_title"`
	TraceID    string    `json:"trace_id,omitempty"`
	TargetType string    `json:"target_type,omitempty"`
	TargetID   string    `json:"target_id,omitempty"`
	Payload    string    `json:"payload"`
	CreatedAt  time.Time `json:"created_at"`
}
