package recommendations

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/agentfabric/api-gateway/internal/models"
	"github.com/agentfabric/api-gateway/internal/store"
)

type scorecardReader interface {
	ListAgentScorecards(ctx context.Context, tenantID string, since time.Duration, limit int) ([]models.AgentScorecard, error)
}

type recommendationStore interface {
	ListRolloutRules(ctx context.Context, tenantID string) ([]models.RolloutRule, error)
	ListRecommendationPolicySignals(ctx context.Context, tenantID string, since time.Duration, limit int) ([]models.RecommendationPolicySignal, error)
	GetCostSpikeReport(ctx context.Context, tenantID string, q store.CostReportQuery) (store.CostSpikeReport, error)
	UpsertRecommendation(ctx context.Context, record models.Recommendation) (models.Recommendation, error)
	ResolveStaleRecommendations(ctx context.Context, tenantID string, activeKeys []string) error
	ListRecommendations(ctx context.Context, query models.RecommendationQuery) (*models.Page[models.Recommendation], error)
	UpdateRecommendationStatus(ctx context.Context, tenantID string, id int64, status string) (models.Recommendation, error)
}

type Service struct {
	store      recommendationStore
	scorecards scorecardReader
}

func NewService(store recommendationStore, scorecards scorecardReader) *Service {
	return &Service{store: store, scorecards: scorecards}
}

func (s *Service) ListRecommendations(ctx context.Context, tenantID string, since time.Duration, limit int, status, recommendationType string) (*models.Page[models.Recommendation], error) {
	if limit <= 0 {
		limit = 12
	}
	generated, err := s.generate(ctx, tenantID, since, limit)
	if err != nil {
		return nil, err
	}

	activeKeys := make([]string, 0, len(generated))
	for _, item := range generated {
		activeKeys = append(activeKeys, item.Key)
		if _, err := s.store.UpsertRecommendation(ctx, item); err != nil {
			return nil, err
		}
	}
	if err := s.store.ResolveStaleRecommendations(ctx, tenantID, activeKeys); err != nil {
		return nil, err
	}

	return s.store.ListRecommendations(ctx, models.RecommendationQuery{
		TenantID: tenantID,
		Type:     recommendationType,
		Status:   status,
		Limit:    limit,
	})
}

func (s *Service) UpdateStatus(ctx context.Context, tenantID string, id int64, status string) (models.Recommendation, error) {
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case models.RecommendationStatusOpen,
		models.RecommendationStatusReviewing,
		models.RecommendationStatusApplied,
		models.RecommendationStatusDismissed,
		models.RecommendationStatusResolved:
	default:
		return models.Recommendation{}, fmt.Errorf("status must be open, reviewing, applied, dismissed, or resolved")
	}
	return s.store.UpdateRecommendationStatus(ctx, tenantID, id, status)
}

func (s *Service) generate(ctx context.Context, tenantID string, since time.Duration, limit int) ([]models.Recommendation, error) {
	if since <= 0 {
		since = 24 * time.Hour
	}

	recommendations := make([]models.Recommendation, 0, limit)

	rolloutRules, err := s.store.ListRolloutRules(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	recommendations = append(recommendations, buildRolloutRecommendations(tenantID, rolloutRules)...)

	if s.scorecards != nil {
		scorecards, err := s.scorecards.ListAgentScorecards(ctx, tenantID, since, 100)
		if err != nil {
			return nil, err
		}
		recommendations = append(recommendations, buildRoutingRecommendations(tenantID, scorecards)...)
	}

	policySignals, err := s.store.ListRecommendationPolicySignals(ctx, tenantID, since, 20)
	if err != nil {
		return nil, err
	}
	recommendations = append(recommendations, buildPolicyRecommendations(tenantID, policySignals)...)

	spikeReport, err := s.store.GetCostSpikeReport(ctx, tenantID, store.CostReportQuery{
		Since: since,
		Limit: maxInt(limit*3, 20),
	})
	if err != nil {
		return nil, err
	}
	recommendations = append(recommendations, buildCostRecommendations(tenantID, spikeReport)...)

	sort.SliceStable(recommendations, func(i, j int) bool {
		if recommendations[i].Confidence == recommendations[j].Confidence {
			if recommendations[i].Type == recommendations[j].Type {
				return recommendations[i].Title < recommendations[j].Title
			}
			return recommendations[i].Type < recommendations[j].Type
		}
		return recommendations[i].Confidence > recommendations[j].Confidence
	})
	if len(recommendations) > limit {
		recommendations = recommendations[:limit]
	}
	return recommendations, nil
}

func buildRolloutRecommendations(tenantID string, rules []models.RolloutRule) []models.Recommendation {
	result := []models.Recommendation{}
	for _, rule := range rules {
		if !strings.EqualFold(rule.Status, models.RolloutStatusActive) || rule.RecentRequests < 5 {
			continue
		}
		rollbackThreshold := parseRuleThreshold(rule.RollbackCriteria["max_error_rate_pct"], 50)
		observeThreshold := math.Max(15, rollbackThreshold*0.6)
		errorRatePct := rule.RecentErrorRate * 100
		if errorRatePct < observeThreshold {
			continue
		}
		targetSummary := describeRolloutTarget(rule)
		confidence := clampUnit((errorRatePct/100)*0.7 + minUnit(float64(rule.RecentRequests)/40, 0.3))
		result = append(result, models.Recommendation{
			Key:             fmt.Sprintf("rollout:pause:%d", rule.ID),
			TenantID:        tenantID,
			Type:            models.RecommendationTypeRollout,
			Status:          models.RecommendationStatusOpen,
			Title:           fmt.Sprintf("Pause rollout %s", rule.Name),
			Summary:         fmt.Sprintf("%s has %d recent requests with a %.1f%% error rate, which is above the observation threshold of %.1f%%.", targetSummary, rule.RecentRequests, errorRatePct, observeThreshold),
			Target:          fmt.Sprintf("rollout %s", rule.Name),
			TargetType:      "rollout_rule",
			TargetID:        strconv.FormatInt(rule.ID, 10),
			SuggestedAction: fmt.Sprintf("Pause %s and inspect the canary path before increasing traffic again.", rule.Name),
			EstimatedImpact: "Reduce failed canary traffic and contain regression spread.",
			BlastRadius:     fmt.Sprintf("%d%% of %s traffic in %s", rule.Percentage, describeShortTarget(rule.TargetType), fallbackString(rule.Environment, "all environments")),
			Confidence:      confidence,
			Evidence: map[string]interface{}{
				"rule_id":              rule.ID,
				"rule_name":            rule.Name,
				"recent_requests":      rule.RecentRequests,
				"recent_failures":      rule.RecentFailures,
				"recent_error_rate":    rule.RecentErrorRate,
				"rollback_error_limit": rollbackThreshold,
				"target_type":          rule.TargetType,
				"target_id":            rule.TargetID,
				"candidate_model":      rule.CandidateModel,
				"candidate_release":    rule.CandidateReleaseTag,
			},
		})
	}
	return result
}

func buildRoutingRecommendations(tenantID string, scorecards []models.AgentScorecard) []models.Recommendation {
	result := []models.Recommendation{}
	for _, card := range scorecards {
		reliability := findComponent(card.Components, "reliability")
		cost := findComponent(card.Components, "cost_efficiency")
		if card.OverallScore >= 70 {
			continue
		}
		if reliability.Score >= 70 && cost.Score >= 55 {
			continue
		}
		confidence := clampUnit((1-card.OverallScore/100)*0.65 + (1-reliability.Score/100)*0.2 + (1-cost.Score/100)*0.15)
		result = append(result, models.Recommendation{
			Key:             fmt.Sprintf("routing:reroute:%s:%s:%s", strings.TrimSpace(card.AgentID), strings.TrimSpace(card.Environment), strings.TrimSpace(card.ReleaseTag)),
			TenantID:        tenantID,
			Type:            models.RecommendationTypeRouting,
			Status:          models.RecommendationStatusOpen,
			Title:           fmt.Sprintf("Reroute %s to a safer baseline", card.AgentName),
			Summary:         fmt.Sprintf("%s is scoring %.1f overall in %s with reliability %.1f and cost efficiency %.1f.", card.AgentName, card.OverallScore, fallbackString(card.Environment, "unknown env"), reliability.Score, cost.Score),
			Target:          fmt.Sprintf("%s | %s | %s", fallbackString(card.AppName, "unknown app"), fallbackString(card.Environment, "unknown env"), fallbackString(card.ReleaseTag, "unreleased")),
			TargetType:      "agent",
			TargetID:        card.AgentID,
			SuggestedAction: fmt.Sprintf("Shift %s traffic away from the current route and prefer the stable control path until health recovers.", card.AgentName),
			EstimatedImpact: "Lower fallback volume and stabilize user-facing latency.",
			BlastRadius:     fmt.Sprintf("%d runs in %s", card.RunCount, fallbackString(card.Environment, "unknown env")),
			Confidence:      confidence,
			Evidence: map[string]interface{}{
				"agent_id":              card.AgentID,
				"agent_name":            card.AgentName,
				"app_name":              card.AppName,
				"environment":           card.Environment,
				"release_tag":           card.ReleaseTag,
				"overall_score":         card.OverallScore,
				"reliability_score":     reliability.Score,
				"cost_efficiency_score": cost.Score,
				"risk_level":            card.RiskLevel,
				"recommended_actions":   card.RecommendedActions,
			},
		})
	}
	return result
}

func buildPolicyRecommendations(tenantID string, signals []models.RecommendationPolicySignal) []models.Recommendation {
	result := []models.Recommendation{}
	for _, signal := range signals {
		if signal.TotalCount < 3 {
			continue
		}
		if signal.WarnCount+signal.SanitizeCount < 2 && signal.DenyCount == 0 {
			continue
		}
		surfaceCount := float64(signal.WarnCount + signal.SanitizeCount + signal.DenyCount)
		confidence := clampUnit((surfaceCount/math.Max(float64(signal.TotalCount), 1))*0.55 + minUnit(float64(signal.TotalCount)/8, 0.45))
		targetID := strings.Join([]string{
			fallbackString(signal.Provider, "unknown"),
			fallbackString(signal.Model, "unknown"),
			fallbackString(signal.Environment, "unknown"),
		}, "|")
		result = append(result, models.Recommendation{
			Key:             fmt.Sprintf("policy:tighten:%s:%s:%s:%s", signal.AppName, signal.Provider, signal.Model, signal.Environment),
			TenantID:        tenantID,
			Type:            models.RecommendationTypePolicy,
			Status:          models.RecommendationStatusOpen,
			Title:           fmt.Sprintf("Tighten policy guardrails for %s", describePolicyScope(signal)),
			Summary:         fmt.Sprintf("%d policy interventions landed on %s: %d warns, %d redactions, %d denies.", signal.TotalCount, describePolicyScope(signal), signal.WarnCount, signal.SanitizeCount, signal.DenyCount),
			Target:          describePolicyScope(signal),
			TargetType:      "policy_scope",
			TargetID:        targetID,
			SuggestedAction: "Promote the matching guardrail from warn to redact or deny, or increase its rollout percentage after validation.",
			EstimatedImpact: "Reduce unsafe traffic that currently needs manual review or runtime redaction.",
			BlastRadius:     fmt.Sprintf("%s | %s", fallbackString(signal.AppName, "unknown app"), fallbackString(signal.Environment, "unknown env")),
			Confidence:      confidence,
			Evidence: map[string]interface{}{
				"app_name":           signal.AppName,
				"environment":        signal.Environment,
				"provider":           signal.Provider,
				"model":              signal.Model,
				"prompt_id":          signal.PromptID,
				"prompt_release_tag": signal.PromptReleaseTag,
				"warn_count":         signal.WarnCount,
				"sanitize_count":     signal.SanitizeCount,
				"deny_count":         signal.DenyCount,
				"total_count":        signal.TotalCount,
			},
		})
	}
	return result
}

func buildCostRecommendations(tenantID string, report store.CostSpikeReport) []models.Recommendation {
	result := []models.Recommendation{}
	for _, spike := range report.Spikes {
		if spike.DeltaCostUSD < 1 && spike.DeltaPct < 50 {
			continue
		}
		confidence := clampUnit(minUnit(spike.DeltaPct/150, 0.6) + minUnit(spike.DeltaCostUSD/10, 0.4))
		targetID := strings.Join([]string{
			fallbackString(spike.AppName, "unknown"),
			fallbackString(spike.Environment, "unknown"),
			fallbackString(spike.ReleaseTag, "unreleased"),
		}, "|")
		result = append(result, models.Recommendation{
			Key:             fmt.Sprintf("cost:adjust:%s:%s:%s:%s:%s", spike.AppName, spike.Environment, spike.Provider, spike.Model, spike.ReleaseTag),
			TenantID:        tenantID,
			Type:            models.RecommendationTypeCost,
			Status:          models.RecommendationStatusOpen,
			Title:           fmt.Sprintf("Adjust budget guardrails for %s", describeSpikeScope(spike)),
			Summary:         spike.Explanation,
			Target:          describeSpikeScope(spike),
			TargetType:      "cost_scope",
			TargetID:        targetID,
			SuggestedAction: "Raise alert sensitivity or update the scoped budget before this spend increase becomes a sustained burn.",
			EstimatedImpact: "Catch cost regressions earlier and constrain surprise spend.",
			BlastRadius:     fmt.Sprintf("%d traces in %s", spike.CurrentTraceCount, fallbackString(spike.Environment, "unknown env")),
			Confidence:      confidence,
			Evidence: map[string]interface{}{
				"app_name":             spike.AppName,
				"environment":          spike.Environment,
				"provider":             spike.Provider,
				"model":                spike.Model,
				"prompt_id":            spike.PromptID,
				"release_tag":          spike.ReleaseTag,
				"current_cost_usd":     spike.CurrentCostUSD,
				"previous_cost_usd":    spike.PreviousCostUSD,
				"delta_cost_usd":       spike.DeltaCostUSD,
				"delta_pct":            spike.DeltaPct,
				"current_trace_count":  spike.CurrentTraceCount,
				"previous_trace_count": spike.PreviousTraceCount,
			},
		})
	}
	return result
}

func findComponent(components []models.AgentScoreComponent, key string) models.AgentScoreComponent {
	for _, component := range components {
		if component.Key == key {
			return component
		}
	}
	return models.AgentScoreComponent{Key: key, Score: 100}
}

func parseRuleThreshold(raw string, fallback float64) float64 {
	if parsed, err := strconv.ParseFloat(strings.TrimSpace(raw), 64); err == nil && parsed > 0 {
		return parsed
	}
	return fallback
}

func clampUnit(value float64) float64 {
	switch {
	case value < 0:
		return 0
	case value > 1:
		return 1
	default:
		return math.Round(value*100) / 100
	}
}

func minUnit(value, cap float64) float64 {
	if value > cap {
		return cap
	}
	if value < 0 {
		return 0
	}
	return value
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func describeRolloutTarget(rule models.RolloutRule) string {
	switch rule.TargetType {
	case models.RolloutTargetPromptRelease:
		return fmt.Sprintf("Prompt release rollout %s", fallbackString(rule.CandidateReleaseTag, rule.Name))
	case models.RolloutTargetModel:
		return fmt.Sprintf("Model rollout %s", fallbackString(rule.CandidateModel, rule.Name))
	case models.RolloutTargetPolicyRule:
		return fmt.Sprintf("Policy rollout %s", rule.Name)
	default:
		return rule.Name
	}
}

func describeShortTarget(targetType string) string {
	switch targetType {
	case models.RolloutTargetPromptRelease:
		return "prompt release"
	case models.RolloutTargetModel:
		return "model"
	case models.RolloutTargetPolicyRule:
		return "policy"
	default:
		return "target"
	}
}

func describePolicyScope(signal models.RecommendationPolicySignal) string {
	parts := []string{}
	if strings.TrimSpace(signal.Provider) != "" && signal.Provider != "unknown" {
		parts = append(parts, signal.Provider)
	}
	if strings.TrimSpace(signal.Model) != "" && signal.Model != "unknown" {
		parts = append(parts, signal.Model)
	}
	if strings.TrimSpace(signal.Environment) != "" && signal.Environment != "unknown" {
		parts = append(parts, signal.Environment)
	}
	if len(parts) == 0 {
		return "current traffic scope"
	}
	return strings.Join(parts, " / ")
}

func describeSpikeScope(spike store.CostSpikeRow) string {
	parts := []string{}
	if spike.AppName != "" && spike.AppName != "unknown" {
		parts = append(parts, spike.AppName)
	}
	if spike.Environment != "" && spike.Environment != "unknown" {
		parts = append(parts, spike.Environment)
	}
	if spike.Model != "" && spike.Model != "unknown" {
		parts = append(parts, spike.Model)
	}
	if spike.ReleaseTag != "" && spike.ReleaseTag != "unreleased" {
		parts = append(parts, spike.ReleaseTag)
	}
	if len(parts) == 0 {
		return "current spend scope"
	}
	return strings.Join(parts, " / ")
}

func fallbackString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
