package models

import "time"

type GovernanceAlert struct {
	SpanID         string    `json:"span_id"`
	TraceID        string    `json:"trace_id"`
	RiskScore      int       `json:"risk_score"`
	RiskCategory   string    `json:"risk_category"`
	Framework      string    `json:"framework"`
	Timestamp      time.Time `json:"timestamp"`
	Summary        string    `json:"summary,omitempty"`
	ActionRequired bool      `json:"action_required,omitempty"`
}

type GovernanceSummary struct {
	TotalEvents   int64          `json:"total_events"`
	HighRiskCount int64          `json:"high_risk_count"`
	Categories    map[string]int `json:"categories"`
	Trend         string         `json:"trend"`
}
