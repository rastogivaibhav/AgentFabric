package memory

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/agentfabric/api-gateway/internal/models"
)

type fakeStore struct {
	lastControlEntry   models.ControlHistoryEntry
	controlHistoryPage *models.Page[models.ControlHistoryEntry]
	adminAudit         []models.AdminAuditEntry
	releases           []models.PromptRelease
	evalRuns           []models.TraceEvalRun
	decisions          *models.Page[models.DecisionRecord]
	rolloutEvents      []models.RolloutEvent
	recommendations    []models.Recommendation
	recommendation     models.Recommendation
	createdBundle      models.EvidenceBundle
}

func (f *fakeStore) CreateControlHistoryEntry(_ context.Context, entry models.ControlHistoryEntry) error {
	f.lastControlEntry = entry
	return nil
}

func (f *fakeStore) ListControlHistoryEntries(context.Context, models.ControlHistoryQuery) (*models.Page[models.ControlHistoryEntry], error) {
	if f.controlHistoryPage == nil {
		return &models.Page[models.ControlHistoryEntry]{Items: []models.ControlHistoryEntry{}}, nil
	}
	return f.controlHistoryPage, nil
}

func (f *fakeStore) CreateEvidenceBundle(_ context.Context, bundle models.EvidenceBundle) (models.EvidenceBundle, error) {
	bundle.ID = 77
	bundle.CreatedAt = time.Now().UTC()
	bundle.ItemCount = len(bundle.Items)
	f.createdBundle = bundle
	return bundle, nil
}

func (f *fakeStore) ListEvidenceBundles(context.Context, string, int) ([]models.EvidenceBundle, error) {
	return nil, nil
}

func (f *fakeStore) GetEvidenceBundle(context.Context, string, int64) (models.EvidenceBundle, error) {
	return models.EvidenceBundle{}, nil
}

func (f *fakeStore) ListAdminAuditEntries(context.Context, string, int) ([]models.AdminAuditEntry, error) {
	return f.adminAudit, nil
}

func (f *fakeStore) ListPromptReleases(context.Context, string) ([]models.PromptRelease, error) {
	return f.releases, nil
}

func (f *fakeStore) ListEvalRuns(context.Context, string, int) ([]models.TraceEvalRun, error) {
	return f.evalRuns, nil
}

func (f *fakeStore) ListDecisionRecords(context.Context, models.DecisionQuery) (*models.Page[models.DecisionRecord], error) {
	if f.decisions == nil {
		return &models.Page[models.DecisionRecord]{Items: []models.DecisionRecord{}}, nil
	}
	return f.decisions, nil
}

func (f *fakeStore) ListRolloutEvents(context.Context, string, models.RolloutEventQuery) ([]models.RolloutEvent, error) {
	return f.rolloutEvents, nil
}

func (f *fakeStore) ListRecommendationsForBundle(context.Context, string, int) ([]models.Recommendation, error) {
	return f.recommendations, nil
}

func (f *fakeStore) GetRecommendation(context.Context, string, int64) (models.Recommendation, error) {
	return f.recommendation, nil
}

func TestRecordChange_NormalizesFields(t *testing.T) {
	store := &fakeStore{}
	svc := NewService(store)

	err := svc.RecordChange(context.Background(), models.ControlHistoryEntry{
		TenantID:   "tenant-1",
		Category:   " Pricing ",
		Action:     " Update ",
		TargetType: "pricing_rule",
		TargetID:   "12",
		Outcome:    "",
	})
	if err != nil {
		t.Fatalf("RecordChange returned error: %v", err)
	}
	if store.lastControlEntry.Category != "pricing" {
		t.Fatalf("expected normalized category, got %q", store.lastControlEntry.Category)
	}
	if store.lastControlEntry.Action != "update" {
		t.Fatalf("expected normalized action, got %q", store.lastControlEntry.Action)
	}
	if store.lastControlEntry.Outcome != "success" {
		t.Fatalf("expected default outcome success, got %q", store.lastControlEntry.Outcome)
	}
}

func TestCreateEvidenceBundle_AssemblesReleaseIncidentEvidence(t *testing.T) {
	store := &fakeStore{
		controlHistoryPage: &models.Page[models.ControlHistoryEntry]{
			Items: []models.ControlHistoryEntry{{
				ID:         1,
				Category:   "rollouts",
				Action:     "status_update",
				TargetType: "rollout_rule",
				TargetID:   "17",
				Reason:     "paused after breach",
			}},
		},
		adminAudit: []models.AdminAuditEntry{{
			ID:         2,
			Category:   "rollouts",
			Action:     "status_update",
			TargetType: "rollout_rule",
			TargetID:   "17",
			Outcome:    "success",
		}},
		releases: []models.PromptRelease{{
			ID:          3,
			PromptID:    "support.system",
			Environment: "staging",
			ReleaseTag:  "candidate-v7",
			Status:      "active",
		}},
		evalRuns: []models.TraceEvalRun{{
			ID:                4,
			TraceID:           "trace-123",
			PromptID:          "support.system",
			PromptEnvironment: "staging",
			ReleaseTag:        "candidate-v7",
			EvalSuite:         "core-release",
			OverallScore:      62,
		}},
		decisions: &models.Page[models.DecisionRecord]{
			Items: []models.DecisionRecord{{
				ID:               5,
				DecisionID:       "decision-1",
				TraceID:          "trace-123",
				Type:             "fallback",
				Result:           "warn",
				PromptID:         "support.system",
				PromptReleaseTag: "candidate-v7",
				Environment:      "staging",
			}},
		},
		rolloutEvents: []models.RolloutEvent{{
			ID:               6,
			RolloutRuleID:    17,
			TraceID:          "trace-123",
			PromptReleaseTag: "candidate-v7",
			AssignedVariant:  "canary",
			Status:           "error",
		}},
		recommendations: []models.Recommendation{{
			ID:              7,
			Type:            "rollout",
			Status:          "open",
			Title:           "Pause rollout support staging",
			TargetType:      "rollout_rule",
			TargetID:        "17",
			Evidence:        map[string]any{"candidate_release": "candidate-v7"},
			Confidence:      0.88,
			SuggestedAction: "Pause rollout",
			Summary:         "Candidate is failing",
			Target:          "rollout 17",
			CreatedAt:       time.Now().UTC(),
			UpdatedAt:       time.Now().UTC(),
			LastSeenAt:      time.Now().UTC(),
		}},
	}
	svc := NewService(store)

	bundle, err := svc.CreateEvidenceBundle(context.Background(), "tenant-1", "Admin", models.EvidenceBundleRequest{
		Name:        "Staging rollout incident",
		Scope:       "incident",
		PromptID:    "support.system",
		Environment: "staging",
		ReleaseTag:  "candidate-v7",
	})
	if err != nil {
		t.Fatalf("CreateEvidenceBundle returned error: %v", err)
	}
	if bundle.ID == 0 {
		t.Fatalf("expected created bundle id")
	}
	if bundle.ItemCount == 0 {
		t.Fatalf("expected evidence items to be assembled")
	}
	if len(bundle.Summary) == 0 {
		t.Fatalf("expected summary lines")
	}
	foundRecommendation := false
	foundRolloutEvent := false
	for _, item := range store.createdBundle.Items {
		if item.ItemType == "recommendation" {
			foundRecommendation = true
		}
		if item.ItemType == "rollout_event" {
			foundRolloutEvent = true
		}
	}
	if !foundRecommendation || !foundRolloutEvent {
		t.Fatalf("expected rollout recommendation and rollout event in bundle, got %+v", store.createdBundle.Items)
	}

	var filters map[string]any
	if err := json.Unmarshal([]byte(store.createdBundle.Filters), &filters); err != nil {
		t.Fatalf("failed to decode filters: %v", err)
	}
	if filters["release_tag"] != "candidate-v7" {
		t.Fatalf("expected release_tag filter to be persisted, got %+v", filters)
	}
}

func TestCreateEvidenceBundle_UsesExplicitRecommendationSelection(t *testing.T) {
	store := &fakeStore{
		controlHistoryPage: &models.Page[models.ControlHistoryEntry]{Items: []models.ControlHistoryEntry{}},
		decisions:          &models.Page[models.DecisionRecord]{Items: []models.DecisionRecord{}},
		recommendation: models.Recommendation{
			ID:              44,
			Type:            "cost",
			Status:          "open",
			Title:           "Adjust budget guardrail",
			Target:          "tenant spend",
			TargetType:      "budget",
			TargetID:        "tenant-1",
			Summary:         "Spend spike detected",
			SuggestedAction: "Increase alert threshold",
			Confidence:      0.73,
			CreatedAt:       time.Now().UTC(),
			UpdatedAt:       time.Now().UTC(),
			LastSeenAt:      time.Now().UTC(),
		},
	}
	svc := NewService(store)

	bundle, err := svc.CreateEvidenceBundle(context.Background(), "tenant-1", "Admin", models.EvidenceBundleRequest{
		Name:             "Budget incident",
		Scope:            "audit",
		RecommendationID: 44,
	})
	if err != nil {
		t.Fatalf("CreateEvidenceBundle returned error: %v", err)
	}

	found := false
	for _, item := range bundle.Items {
		if item.ItemType == "recommendation" && item.TargetID == "tenant-1" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected explicitly selected recommendation to be included")
	}
}
