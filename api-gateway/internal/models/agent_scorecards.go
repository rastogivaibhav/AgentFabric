package models

import "time"

type AgentScoreComponent struct {
	Key      string  `json:"key"`
	Label    string  `json:"label"`
	Score    float64 `json:"score"`
	Weight   float64 `json:"weight"`
	Severity string  `json:"severity"`
	Summary  string  `json:"summary"`
}

type AgentScoreTrend struct {
	PreviousOverallScore float64 `json:"previous_overall_score"`
	CurrentOverallScore  float64 `json:"current_overall_score"`
	Delta                float64 `json:"delta"`
	Direction            string  `json:"direction"`
}

type AgentScorecard struct {
	AgentID            string                `json:"agent_id"`
	AgentName          string                `json:"agent_name"`
	Framework          string                `json:"framework"`
	AppName            string                `json:"app_name,omitempty"`
	Environment        string                `json:"environment,omitempty"`
	ReleaseTag         string                `json:"release_tag,omitempty"`
	RunCount           int64                 `json:"run_count"`
	TotalCostUSD       float64               `json:"total_cost_usd"`
	TotalTokens        int64                 `json:"total_tokens"`
	AvgLatencyMs       float64               `json:"avg_latency_ms"`
	EvalCount          int64                 `json:"eval_count"`
	OverallScore       float64               `json:"overall_score"`
	RiskLevel          string                `json:"risk_level"`
	Components         []AgentScoreComponent `json:"components"`
	Trend              AgentScoreTrend       `json:"trend"`
	RecommendedActions []string              `json:"recommended_actions,omitempty"`
	GeneratedAt        time.Time             `json:"generated_at"`
}

type AgentScorecardMetrics struct {
	AgentName            string
	Framework            string
	AppName              string
	Environment          string
	ReleaseTag           string
	FirstSeen            time.Time
	LastSeen             time.Time
	RunCount             int64
	TotalCostUSD         float64
	TotalTokens          int64
	AvgLatencyMs         float64
	RunErrorRate         float64
	DecisionCount        int64
	PolicyBlockCount     int64
	PolicyRedactionCount int64
	BudgetDeniedCount    int64
	FallbackCount        int64
	EvalCount            int64
	EvalAverageScore     float64
	RecentEvalAverage    float64
	LatestEvalScore      float64
	RolloutCount         int64
	RolloutErrorRate     float64
	AutoPauseCount       int64
}
