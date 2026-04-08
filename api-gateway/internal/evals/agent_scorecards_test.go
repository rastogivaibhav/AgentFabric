package evals

import (
	"context"
	"testing"
	"time"

	"github.com/govagn/api-gateway/internal/models"
	"github.com/govagn/api-gateway/internal/store"
)

type fakeAgentScoreStore struct {
	current  []models.AgentScorecardMetrics
	previous []models.AgentScorecardMetrics
}

func (f *fakeAgentScoreStore) LoadTraceViewInputs(context.Context, string, string) (*store.TraceViewInputs, error) {
	return nil, nil
}

func (f *fakeAgentScoreStore) InsertEvalRun(context.Context, string, models.TraceEvalRun) (models.TraceEvalRun, error) {
	return models.TraceEvalRun{}, nil
}

func (f *fakeAgentScoreStore) ListEvalRuns(context.Context, string, int) ([]models.TraceEvalRun, error) {
	return nil, nil
}

func (f *fakeAgentScoreStore) ListEvalRunsByRelease(context.Context, string, string, string, string, string) ([]models.TraceEvalRun, error) {
	return nil, nil
}

func (f *fakeAgentScoreStore) ListAgentScorecardMetrics(_ context.Context, _ string, windowStart, windowEnd time.Time, _ int, agentName string) ([]models.AgentScorecardMetrics, error) {
	now := time.Now().UTC()
	var source []models.AgentScorecardMetrics
	if windowStart.After(now.Add(-36 * time.Hour)) {
		source = f.current
	} else {
		source = f.previous
	}
	if agentName == "" {
		return source, nil
	}
	filtered := make([]models.AgentScorecardMetrics, 0, len(source))
	for _, item := range source {
		if item.AgentName == agentName {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}

func TestListAgentScorecards_ComputesOverallAndTrend(t *testing.T) {
	svc := &Service{store: &fakeAgentScoreStore{
		current: []models.AgentScorecardMetrics{
			{
				AgentName:            "support-bot",
				Framework:            "openai_agents",
				AppName:              "ops-ui",
				Environment:          "production",
				ReleaseTag:           "2026.04",
				RunCount:             40,
				TotalCostUSD:         4.2,
				TotalTokens:          56000,
				AvgLatencyMs:         420,
				RunErrorRate:         0.04,
				DecisionCount:        14,
				PolicyBlockCount:     2,
				PolicyRedactionCount: 1,
				BudgetDeniedCount:    0,
				FallbackCount:        3,
				EvalCount:            5,
				EvalAverageScore:     84,
				RecentEvalAverage:    88,
				LatestEvalScore:      90,
				RolloutCount:         12,
				RolloutErrorRate:     0.08,
				AutoPauseCount:       0,
			},
		},
		previous: []models.AgentScorecardMetrics{
			{
				AgentName:            "support-bot",
				Framework:            "openai_agents",
				AppName:              "ops-ui",
				Environment:          "production",
				ReleaseTag:           "2026.03",
				RunCount:             38,
				TotalCostUSD:         4.0,
				TotalTokens:          55000,
				AvgLatencyMs:         450,
				RunErrorRate:         0.09,
				DecisionCount:        14,
				PolicyBlockCount:     3,
				PolicyRedactionCount: 2,
				BudgetDeniedCount:    1,
				FallbackCount:        5,
				EvalCount:            4,
				EvalAverageScore:     76,
				RecentEvalAverage:    74,
				LatestEvalScore:      72,
				RolloutCount:         10,
				RolloutErrorRate:     0.15,
				AutoPauseCount:       1,
			},
		},
	}}

	cards, err := svc.ListAgentScorecards(context.Background(), "tenant-a", 24*time.Hour, 10)
	if err != nil {
		t.Fatalf("ListAgentScorecards(): %v", err)
	}
	if len(cards) != 1 {
		t.Fatalf("expected 1 scorecard, got %d", len(cards))
	}
	card := cards[0]
	if card.AgentName != "support-bot" {
		t.Fatalf("expected support-bot, got %q", card.AgentName)
	}
	if card.OverallScore <= 0 {
		t.Fatalf("expected positive overall score")
	}
	if card.Trend.Direction != "improving" {
		t.Fatalf("expected improving trend, got %q", card.Trend.Direction)
	}
	if len(card.Components) != 5 {
		t.Fatalf("expected 5 components, got %d", len(card.Components))
	}
}

func TestGetAgentScorecard_RecommendsActionsForWeakComponents(t *testing.T) {
	svc := &Service{store: &fakeAgentScoreStore{
		current: []models.AgentScorecardMetrics{
			{
				AgentName:            "billing-agent",
				Framework:            "langgraph",
				AppName:              "billing-ui",
				Environment:          "staging",
				ReleaseTag:           "candidate-7",
				RunCount:             12,
				TotalCostUSD:         6.5,
				TotalTokens:          12000,
				AvgLatencyMs:         980,
				RunErrorRate:         0.22,
				DecisionCount:        0,
				PolicyBlockCount:     0,
				PolicyRedactionCount: 0,
				BudgetDeniedCount:    0,
				FallbackCount:        4,
				EvalCount:            1,
				EvalAverageScore:     58,
				RecentEvalAverage:    52,
				LatestEvalScore:      52,
				RolloutCount:         6,
				RolloutErrorRate:     0.35,
				AutoPauseCount:       1,
			},
		},
	}}

	card, err := svc.GetAgentScorecard(context.Background(), "tenant-a", "billing-agent", 24*time.Hour)
	if err != nil {
		t.Fatalf("GetAgentScorecard(): %v", err)
	}
	if card.RiskLevel == "" {
		t.Fatalf("expected risk level")
	}
	if len(card.RecommendedActions) == 0 {
		t.Fatalf("expected recommended actions")
	}
	foundReleaseAction := false
	for _, action := range card.RecommendedActions {
		if action == "Pause or narrow canaries until rollout error rates stabilize." {
			foundReleaseAction = true
			break
		}
	}
	if !foundReleaseAction {
		t.Fatalf("expected release-health recommendation, got %+v", card.RecommendedActions)
	}
}
