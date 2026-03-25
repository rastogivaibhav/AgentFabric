package rollouts

import (
	"context"
	"testing"
	"time"

	"github.com/agentfabric/api-gateway/internal/models"
)

type fakeStore struct {
	rules               []models.RolloutRule
	activeRules         []models.RolloutRule
	lastEvent           models.RolloutEvent
	lastStatusRuleID    int64
	lastStatus          string
	healthRequests      int64
	healthFailures      int64
	healthErrorRate     float64
	updateStatusCallCnt int
}

func (f *fakeStore) ListRolloutRules(context.Context, string) ([]models.RolloutRule, error) {
	return f.rules, nil
}

func (f *fakeStore) ListActiveRolloutRules(context.Context, string) ([]models.RolloutRule, error) {
	if f.activeRules != nil {
		return f.activeRules, nil
	}
	return f.rules, nil
}

func (f *fakeStore) UpsertRolloutRule(_ context.Context, _ string, rule models.RolloutRule, _ string) (models.RolloutRule, error) {
	if rule.ID == 0 {
		rule.ID = 1
	}
	f.rules = append(f.rules, rule)
	return rule, nil
}

func (f *fakeStore) UpdateRolloutRuleStatus(_ context.Context, _ string, ruleID int64, status, _ string) (models.RolloutRule, error) {
	f.lastStatusRuleID = ruleID
	f.lastStatus = status
	f.updateStatusCallCnt++
	return models.RolloutRule{ID: ruleID, Status: status}, nil
}

func (f *fakeStore) CreateRolloutEvent(_ context.Context, event models.RolloutEvent) (models.RolloutEvent, error) {
	f.lastEvent = event
	event.ID = 99
	event.CreatedAt = time.Now().UTC()
	return event, nil
}

func (f *fakeStore) GetRolloutHealth(context.Context, string, int64, time.Duration) (int64, int64, float64, error) {
	return f.healthRequests, f.healthFailures, f.healthErrorRate, nil
}

func TestResolve_DeterministicModelAssignment(t *testing.T) {
	store := &fakeStore{
		activeRules: []models.RolloutRule{
			{
				ID:             17,
				Name:           "GPT-4.1 canary",
				TargetType:     models.RolloutTargetModel,
				TargetID:       "gpt-4.1",
				Environment:    "production",
				Percentage:     25,
				ControlModel:   "gpt-4.1",
				CandidateModel: "gpt-4.1-mini",
				Status:         models.RolloutStatusActive,
			},
		},
	}
	svc := &Service{store: store}

	req := models.RolloutPreviewRequest{
		TenantID:      "tenant-a",
		Provider:      "openai",
		Model:         "gpt-4.1",
		Environment:   "production",
		App:           "ops-ui",
		Session:       "session-42",
		AssignmentKey: "tenant-a|session-42",
	}

	first, err := svc.Resolve(context.Background(), req)
	if err != nil {
		t.Fatalf("Resolve(): %v", err)
	}
	second, err := svc.Resolve(context.Background(), req)
	if err != nil {
		t.Fatalf("Resolve(): %v", err)
	}

	if first.RuleID != 17 || second.RuleID != 17 {
		t.Fatalf("expected matched rollout rule 17, got %d and %d", first.RuleID, second.RuleID)
	}
	if first.Bucket != second.Bucket || first.Variant != second.Variant || first.AssignmentKey != second.AssignmentKey {
		t.Fatalf("expected deterministic assignment, got %+v and %+v", first, second)
	}
}

func TestResolve_PromptReleaseUsesReleaseTag(t *testing.T) {
	store := &fakeStore{
		activeRules: []models.RolloutRule{
			{
				ID:                  23,
				Name:                "Support prompt canary",
				TargetType:          models.RolloutTargetPromptRelease,
				TargetID:            "support-bot.system",
				Environment:         "staging",
				Percentage:          100,
				ControlReleaseTag:   "2026.03",
				CandidateReleaseTag: "2026.04-rc1",
				Status:              models.RolloutStatusActive,
			},
		},
	}
	svc := &Service{store: store}

	assignment, err := svc.Resolve(context.Background(), models.RolloutPreviewRequest{
		TenantID:          "tenant-a",
		PromptID:          "support-bot.system",
		PromptEnvironment: "staging",
		Environment:       "staging",
		AssignmentKey:     "prompt-preview",
	})
	if err != nil {
		t.Fatalf("Resolve(): %v", err)
	}

	if !assignment.Selected {
		t.Fatalf("expected canary selection")
	}
	if assignment.ReleaseTag != "2026.04-rc1" {
		t.Fatalf("expected candidate release tag, got %q", assignment.ReleaseTag)
	}
}

func TestRecordOutcome_AutoPausesCanaryOnThresholdBreach(t *testing.T) {
	store := &fakeStore{
		healthRequests:  9,
		healthFailures:  6,
		healthErrorRate: 0.60,
	}
	svc := &Service{store: store}

	assignment := models.RolloutAssignment{
		RuleID:        31,
		RuleName:      "Prompt canary",
		TargetType:    models.RolloutTargetPromptRelease,
		TargetID:      "support-bot.system",
		Variant:       "canary",
		AssignmentKey: "session-99",
		Metadata: map[string]string{
			"rollback_min_requests":       "10",
			"rollback_max_error_rate_pct": "50",
		},
	}

	event, err := svc.RecordOutcome(context.Background(), "tenant-a", assignment, models.RolloutEvent{
		Status:          "error",
		StatusCode:      502,
		AssignedVariant: "canary",
	}, "system")
	if err != nil {
		t.Fatalf("RecordOutcome(): %v", err)
	}

	if !event.AutoPaused {
		t.Fatalf("expected rollout event to indicate auto pause")
	}
	if store.lastStatusRuleID != 31 || store.lastStatus != models.RolloutStatusPaused {
		t.Fatalf("expected rollout rule 31 to be auto-paused, got rule=%d status=%q", store.lastStatusRuleID, store.lastStatus)
	}
	if store.lastEvent.RolloutRuleID != 31 || store.lastEvent.StatusCode != 502 {
		t.Fatalf("expected rollout event to be recorded, got %+v", store.lastEvent)
	}
}
