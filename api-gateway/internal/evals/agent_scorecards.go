package evals

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/govagn/api-gateway/internal/models"
	"github.com/jackc/pgx/v5"
)

const (
	agentReliabilityWeight = 0.30
	agentPolicyWeight      = 0.20
	agentCostWeight        = 0.15
	agentRegressionWeight  = 0.20
	agentReleaseWeight     = 0.15
)

func (s *Service) ListAgentScorecards(ctx context.Context, tenantID string, since time.Duration, limit int) ([]models.AgentScorecard, error) {
	now := time.Now().UTC()
	current, err := s.store.ListAgentScorecardMetrics(ctx, tenantID, now.Add(-since), now, limit, "")
	if err != nil {
		return nil, err
	}
	previous, err := s.store.ListAgentScorecardMetrics(ctx, tenantID, now.Add(-(2 * since)), now.Add(-since), maxInt(limit*2, 50), "")
	if err != nil {
		return nil, err
	}
	prevByAgent := make(map[string]models.AgentScorecardMetrics, len(previous))
	for _, item := range previous {
		prevByAgent[item.AgentName] = item
	}

	cards := make([]models.AgentScorecard, 0, len(current))
	for _, item := range current {
		cards = append(cards, buildAgentScorecard(item, prevByAgent[item.AgentName], now))
	}
	sort.SliceStable(cards, func(i, j int) bool {
		if cards[i].OverallScore == cards[j].OverallScore {
			return cards[i].AgentName < cards[j].AgentName
		}
		return cards[i].OverallScore > cards[j].OverallScore
	})
	return cards, nil
}

func (s *Service) GetAgentScorecard(ctx context.Context, tenantID, agentName string, since time.Duration) (models.AgentScorecard, error) {
	now := time.Now().UTC()
	current, err := s.store.ListAgentScorecardMetrics(ctx, tenantID, now.Add(-since), now, 1, strings.TrimSpace(agentName))
	if err != nil {
		return models.AgentScorecard{}, err
	}
	if len(current) == 0 {
		return models.AgentScorecard{}, pgx.ErrNoRows
	}
	previous, err := s.store.ListAgentScorecardMetrics(ctx, tenantID, now.Add(-(2 * since)), now.Add(-since), 1, strings.TrimSpace(agentName))
	if err != nil {
		return models.AgentScorecard{}, err
	}
	var previousMetrics models.AgentScorecardMetrics
	if len(previous) > 0 {
		previousMetrics = previous[0]
	}
	return buildAgentScorecard(current[0], previousMetrics, now), nil
}

func buildAgentScorecard(current, previous models.AgentScorecardMetrics, generatedAt time.Time) models.AgentScorecard {
	components := []models.AgentScoreComponent{
		scoreAgentReliability(current),
		scoreAgentPolicyRisk(current),
		scoreAgentCostEfficiency(current),
		scoreAgentRegressionRisk(current),
		scoreAgentReleaseHealth(current),
	}

	totalWeight := 0.0
	weightedTotal := 0.0
	for _, component := range components {
		totalWeight += component.Weight
		weightedTotal += component.Score * component.Weight
	}
	overall := 0.0
	if totalWeight > 0 {
		overall = clampScore(weightedTotal / totalWeight)
	}

	previousOverall := 0.0
	if strings.TrimSpace(previous.AgentName) != "" {
		prevComponents := []models.AgentScoreComponent{
			scoreAgentReliability(previous),
			scoreAgentPolicyRisk(previous),
			scoreAgentCostEfficiency(previous),
			scoreAgentRegressionRisk(previous),
			scoreAgentReleaseHealth(previous),
		}
		prevWeight := 0.0
		prevTotal := 0.0
		for _, component := range prevComponents {
			prevWeight += component.Weight
			prevTotal += component.Score * component.Weight
		}
		if prevWeight > 0 {
			previousOverall = clampScore(prevTotal / prevWeight)
		}
	}
	delta := math.Round((overall-previousOverall)*100) / 100
	direction := "flat"
	switch {
	case delta > 1:
		direction = "improving"
	case delta < -1:
		direction = "declining"
	}

	return models.AgentScorecard{
		AgentID:            current.AgentName,
		AgentName:          current.AgentName,
		Framework:          current.Framework,
		AppName:            current.AppName,
		Environment:        current.Environment,
		ReleaseTag:         current.ReleaseTag,
		RunCount:           current.RunCount,
		TotalCostUSD:       current.TotalCostUSD,
		TotalTokens:        current.TotalTokens,
		AvgLatencyMs:       math.Round(current.AvgLatencyMs*100) / 100,
		EvalCount:          current.EvalCount,
		OverallScore:       overall,
		RiskLevel:          severityForScore(overall),
		Components:         components,
		Trend:              models.AgentScoreTrend{PreviousOverallScore: previousOverall, CurrentOverallScore: overall, Delta: delta, Direction: direction},
		RecommendedActions: recommendAgentActions(components),
		GeneratedAt:        generatedAt,
	}
}

func scoreAgentReliability(metrics models.AgentScorecardMetrics) models.AgentScoreComponent {
	fallbackRate := ratio(float64(metrics.FallbackCount), float64(max64(metrics.RunCount, 1)))
	score := 100 - (metrics.RunErrorRate * 100) - (fallbackRate * 15)
	score = clampScore(score)
	return models.AgentScoreComponent{
		Key:      "reliability",
		Label:    "Reliability",
		Score:    score,
		Weight:   agentReliabilityWeight,
		Severity: severityForScore(score),
		Summary:  fmt.Sprintf("%.1f%% run errors with %d fallback events across %d runs", metrics.RunErrorRate*100, metrics.FallbackCount, metrics.RunCount),
	}
}

func scoreAgentPolicyRisk(metrics models.AgentScorecardMetrics) models.AgentScoreComponent {
	if metrics.DecisionCount == 0 {
		return models.AgentScoreComponent{
			Key:      "policy_risk",
			Label:    "Policy Risk",
			Score:    55,
			Weight:   agentPolicyWeight,
			Severity: severityForScore(55),
			Summary:  "No policy or budget decision evidence recorded in the scoring window.",
		}
	}
	blockedRate := ratio(float64(metrics.PolicyBlockCount+metrics.BudgetDeniedCount), float64(max64(metrics.RunCount, 1)))
	redactionRate := ratio(float64(metrics.PolicyRedactionCount), float64(max64(metrics.DecisionCount, 1)))
	score := 100 - (blockedRate * 70) - (redactionRate * 20)
	score = clampScore(score)
	return models.AgentScoreComponent{
		Key:      "policy_risk",
		Label:    "Policy Risk",
		Score:    score,
		Weight:   agentPolicyWeight,
		Severity: severityForScore(score),
		Summary:  fmt.Sprintf("%d policy blocks, %d redactions, %d budget denials", metrics.PolicyBlockCount, metrics.PolicyRedactionCount, metrics.BudgetDeniedCount),
	}
}

func scoreAgentCostEfficiency(metrics models.AgentScorecardMetrics) models.AgentScoreComponent {
	if metrics.TotalTokens <= 0 {
		return models.AgentScoreComponent{
			Key:      "cost_efficiency",
			Label:    "Cost Efficiency",
			Score:    60,
			Weight:   agentCostWeight,
			Severity: severityForScore(60),
			Summary:  "No token usage recorded; cost efficiency is estimated conservatively.",
		}
	}
	costPerThousand := (metrics.TotalCostUSD / math.Max(float64(metrics.TotalTokens), 1)) * 1000
	fallbackRate := ratio(float64(metrics.FallbackCount), float64(max64(metrics.RunCount, 1)))
	score := 100 - (costPerThousand * 120) - (fallbackRate * 10)
	score = clampScore(score)
	return models.AgentScoreComponent{
		Key:      "cost_efficiency",
		Label:    "Cost Efficiency",
		Score:    score,
		Weight:   agentCostWeight,
		Severity: severityForScore(score),
		Summary:  fmt.Sprintf("$%.4f per 1K tokens with %.1fms average latency", costPerThousand, metrics.AvgLatencyMs),
	}
}

func scoreAgentRegressionRisk(metrics models.AgentScorecardMetrics) models.AgentScoreComponent {
	if metrics.EvalCount == 0 {
		return models.AgentScoreComponent{
			Key:      "regression_risk",
			Label:    "Regression Risk",
			Score:    60,
			Weight:   agentRegressionWeight,
			Severity: severityForScore(60),
			Summary:  "No eval runs linked to this agent in the scoring window.",
		}
	}
	delta := metrics.RecentEvalAverage - metrics.EvalAverageScore
	score := metrics.EvalAverageScore
	if delta < 0 {
		score += delta * 2
	}
	if metrics.LatestEvalScore > 0 {
		score = (score * 0.7) + (metrics.LatestEvalScore * 0.3)
	}
	score = clampScore(score)
	return models.AgentScoreComponent{
		Key:      "regression_risk",
		Label:    "Regression Risk",
		Score:    score,
		Weight:   agentRegressionWeight,
		Severity: severityForScore(score),
		Summary:  fmt.Sprintf("%d evals, latest %.1f, recent delta %.2f", metrics.EvalCount, metrics.LatestEvalScore, delta),
	}
}

func scoreAgentReleaseHealth(metrics models.AgentScorecardMetrics) models.AgentScoreComponent {
	if metrics.RolloutCount == 0 {
		return models.AgentScoreComponent{
			Key:      "release_health",
			Label:    "Release Health",
			Score:    70,
			Weight:   agentReleaseWeight,
			Severity: severityForScore(70),
			Summary:  "No rollout events recorded; release health is neutral until canary evidence exists.",
		}
	}
	score := 100 - (metrics.RolloutErrorRate * 70) - (float64(metrics.AutoPauseCount) * 15)
	score = clampScore(score)
	return models.AgentScoreComponent{
		Key:      "release_health",
		Label:    "Release Health",
		Score:    score,
		Weight:   agentReleaseWeight,
		Severity: severityForScore(score),
		Summary:  fmt.Sprintf("%d rollout events, %.1f%% error rate, %d auto-pauses", metrics.RolloutCount, metrics.RolloutErrorRate*100, metrics.AutoPauseCount),
	}
}

func recommendAgentActions(components []models.AgentScoreComponent) []string {
	actions := make([]string, 0, 3)
	for _, component := range components {
		if component.Score >= 70 {
			continue
		}
		switch component.Key {
		case "reliability":
			actions = append(actions, "Investigate failure and fallback hotspots before increasing traffic.")
		case "policy_risk":
			actions = append(actions, "Review recent policy blocks and redactions to reduce governance risk.")
		case "cost_efficiency":
			actions = append(actions, "Audit expensive traces and candidate model choices for cost efficiency.")
		case "regression_risk":
			actions = append(actions, "Run release eval comparisons before the next promotion decision.")
		case "release_health":
			actions = append(actions, "Pause or narrow canaries until rollout error rates stabilize.")
		}
	}
	if len(actions) == 0 {
		return []string{"Maintain the current release posture and monitor for regressions."}
	}
	return actions
}

func ratio(numerator, denominator float64) float64 {
	if denominator <= 0 {
		return 0
	}
	return numerator / denominator
}

func max64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
