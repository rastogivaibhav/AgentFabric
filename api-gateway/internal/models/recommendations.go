package models

import (
	"strings"
	"time"
)

const (
	RecommendationTypeRollout = "rollout"
	RecommendationTypeRouting = "routing"
	RecommendationTypePolicy  = "policy"
	RecommendationTypeCost    = "cost"

	RecommendationStatusOpen      = "open"
	RecommendationStatusReviewing = "reviewing"
	RecommendationStatusApplied   = "applied"
	RecommendationStatusDismissed = "dismissed"
	RecommendationStatusResolved  = "resolved"
)

type Recommendation struct {
	ID              int64                  `json:"id"`
	Key             string                 `json:"key,omitempty"`
	TenantID        string                 `json:"tenant_id,omitempty"`
	Type            string                 `json:"type"`
	Status          string                 `json:"status"`
	Title           string                 `json:"title"`
	Summary         string                 `json:"summary"`
	Target          string                 `json:"target"`
	TargetType      string                 `json:"target_type,omitempty"`
	TargetID        string                 `json:"target_id,omitempty"`
	SuggestedAction string                 `json:"suggested_action"`
	EstimatedImpact string                 `json:"estimated_impact,omitempty"`
	BlastRadius     string                 `json:"blast_radius,omitempty"`
	Confidence      float64                `json:"confidence"`
	Evidence        map[string]interface{} `json:"evidence,omitempty"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
	LastSeenAt      time.Time              `json:"last_seen_at"`
}

type RecommendationQuery struct {
	TenantID string
	Type     string
	Status   string
	Limit    int
	Offset   int
}

type RecommendationStatusUpdateRequest struct {
	Status string `json:"status"`
}

type RecommendationPolicySignal struct {
	AppName          string
	Environment      string
	Provider         string
	Model            string
	PromptID         string
	PromptReleaseTag string
	WarnCount        int64
	SanitizeCount    int64
	DenyCount        int64
	TotalCount       int64
}

func NormalizeRecommendationStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case RecommendationStatusOpen,
		RecommendationStatusReviewing,
		RecommendationStatusApplied,
		RecommendationStatusDismissed,
		RecommendationStatusResolved:
		return strings.ToLower(strings.TrimSpace(status))
	default:
		return RecommendationStatusOpen
	}
}
