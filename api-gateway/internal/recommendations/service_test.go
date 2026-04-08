package recommendations

import (
	"context"
	"testing"
	"time"

	"github.com/govagn/api-gateway/internal/models"
	"github.com/govagn/api-gateway/internal/store"
)

type fakeScorecardReader struct {
	items []models.AgentScorecard
}

func (f fakeScorecardReader) ListAgentScorecards(context.Context, string, time.Duration, int) ([]models.AgentScorecard, error) {
	return f.items, nil
}

type fakeRecommendationStore struct {
	rollouts        []models.RolloutRule
	policySignals   []models.RecommendationPolicySignal
	spikeReport     store.CostSpikeReport
	recommendations map[string]models.Recommendation
}

func (f *fakeRecommendationStore) ListRolloutRules(context.Context, string) ([]models.RolloutRule, error) {
	return f.rollouts, nil
}

func (f *fakeRecommendationStore) ListRecommendationPolicySignals(context.Context, string, time.Duration, int) ([]models.RecommendationPolicySignal, error) {
	return f.policySignals, nil
}

func (f *fakeRecommendationStore) GetCostSpikeReport(context.Context, string, store.CostReportQuery) (store.CostSpikeReport, error) {
	return f.spikeReport, nil
}

func (f *fakeRecommendationStore) UpsertRecommendation(_ context.Context, record models.Recommendation) (models.Recommendation, error) {
	if f.recommendations == nil {
		f.recommendations = map[string]models.Recommendation{}
	}
	record.ID = int64(len(f.recommendations) + 1)
	record.Status = models.NormalizeRecommendationStatus(record.Status)
	f.recommendations[record.Key] = record
	return record, nil
}

func (f *fakeRecommendationStore) ResolveStaleRecommendations(context.Context, string, []string) error {
	return nil
}

func (f *fakeRecommendationStore) ListRecommendations(_ context.Context, query models.RecommendationQuery) (*models.Page[models.Recommendation], error) {
	items := make([]models.Recommendation, 0, len(f.recommendations))
	for _, item := range f.recommendations {
		if query.Type != "" && item.Type != query.Type {
			continue
		}
		if query.Status != "" && item.Status != query.Status {
			continue
		}
		if query.Status == "" && (item.Status == models.RecommendationStatusDismissed || item.Status == models.RecommendationStatusResolved) {
			continue
		}
		items = append(items, item)
	}
	return &models.Page[models.Recommendation]{
		Items:   items,
		Total:   int64(len(items)),
		HasMore: false,
	}, nil
}

func (f *fakeRecommendationStore) UpdateRecommendationStatus(_ context.Context, _ string, id int64, status string) (models.Recommendation, error) {
	for key, item := range f.recommendations {
		if item.ID == id {
			item.Status = status
			f.recommendations[key] = item
			return item, nil
		}
	}
	return models.Recommendation{}, nil
}

func TestListRecommendations_GeneratesActionableRecommendations(t *testing.T) {
	fakeStore := &fakeRecommendationStore{
		rollouts: []models.RolloutRule{
			{
				ID:                  11,
				Name:                "Prompt canary",
				TargetType:          models.RolloutTargetPromptRelease,
				TargetID:            "support-bot.system",
				Environment:         "production",
				Percentage:          10,
				CandidateReleaseTag: "candidate-7",
				Status:              models.RolloutStatusActive,
				RecentRequests:      18,
				RecentFailures:      5,
				RecentErrorRate:     0.28,
				RollbackCriteria:    map[string]string{"max_error_rate_pct": "30"},
			},
		},
		policySignals: []models.RecommendationPolicySignal{
			{
				AppName:          "ops-ui",
				Environment:      "production",
				Provider:         "openai",
				Model:            "gpt-4o",
				PromptID:         "support-bot.system",
				PromptReleaseTag: "candidate-7",
				WarnCount:        2,
				SanitizeCount:    2,
				DenyCount:        1,
				TotalCount:       5,
			},
		},
		spikeReport: store.CostSpikeReport{
			Spikes: []store.CostSpikeRow{
				{
					AppName:            "ops-ui",
					Environment:        "production",
					Provider:           "openai",
					Model:              "gpt-4o",
					PromptID:           "support-bot.system",
					ReleaseTag:         "candidate-7",
					CurrentCostUSD:     14.5,
					PreviousCostUSD:    6.0,
					DeltaCostUSD:       8.5,
					DeltaPct:           141.6,
					CurrentTraceCount:  44,
					PreviousTraceCount: 19,
					Explanation:        "Spend increased 141.6% after the candidate release.",
				},
			},
		},
	}
	scorecards := fakeScorecardReader{
		items: []models.AgentScorecard{
			{
				AgentID:      "billing-agent",
				AgentName:    "billing-agent",
				AppName:      "billing-ui",
				Environment:  "staging",
				ReleaseTag:   "candidate-7",
				RunCount:     16,
				OverallScore: 58.1,
				RiskLevel:    "high",
				Components: []models.AgentScoreComponent{
					{Key: "reliability", Score: 61},
					{Key: "cost_efficiency", Score: 44},
				},
			},
		},
	}

	svc := NewService(fakeStore, scorecards)
	page, err := svc.ListRecommendations(context.Background(), "tenant-a", 24*time.Hour, 10, "", "")
	if err != nil {
		t.Fatalf("ListRecommendations(): %v", err)
	}
	if page.Total < 4 {
		t.Fatalf("expected at least 4 recommendations, got %d", page.Total)
	}
	typeCounts := map[string]int{}
	for _, item := range page.Items {
		typeCounts[item.Type]++
		if item.Confidence <= 0 {
			t.Fatalf("expected positive confidence for %+v", item)
		}
		if item.SuggestedAction == "" {
			t.Fatalf("expected suggested action for %+v", item)
		}
	}
	for _, key := range []string{
		models.RecommendationTypeRollout,
		models.RecommendationTypeRouting,
		models.RecommendationTypePolicy,
		models.RecommendationTypeCost,
	} {
		if typeCounts[key] == 0 {
			t.Fatalf("expected recommendation type %s to be present; got %+v", key, typeCounts)
		}
	}
}

func TestUpdateStatus_RejectsUnknownStatuses(t *testing.T) {
	svc := NewService(&fakeRecommendationStore{}, fakeScorecardReader{})
	if _, err := svc.UpdateStatus(context.Background(), "tenant-a", 1, "ship-it"); err == nil {
		t.Fatalf("expected invalid status error")
	}
}
