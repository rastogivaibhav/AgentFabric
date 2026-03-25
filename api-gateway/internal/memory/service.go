package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/agentfabric/api-gateway/internal/models"
)

type serviceStore interface {
	CreateControlHistoryEntry(ctx context.Context, entry models.ControlHistoryEntry) error
	ListControlHistoryEntries(ctx context.Context, query models.ControlHistoryQuery) (*models.Page[models.ControlHistoryEntry], error)
	CreateEvidenceBundle(ctx context.Context, bundle models.EvidenceBundle) (models.EvidenceBundle, error)
	ListEvidenceBundles(ctx context.Context, tenantID string, limit int) ([]models.EvidenceBundle, error)
	GetEvidenceBundle(ctx context.Context, tenantID string, bundleID int64) (models.EvidenceBundle, error)
	ListAdminAuditEntries(ctx context.Context, tenantID string, limit int) ([]models.AdminAuditEntry, error)
	ListPromptReleases(ctx context.Context, tenantID string) ([]models.PromptRelease, error)
	ListEvalRuns(ctx context.Context, tenantID string, limit int) ([]models.TraceEvalRun, error)
	ListDecisionRecords(ctx context.Context, query models.DecisionQuery) (*models.Page[models.DecisionRecord], error)
	ListRolloutEvents(ctx context.Context, tenantID string, query models.RolloutEventQuery) ([]models.RolloutEvent, error)
	ListRecommendationsForBundle(ctx context.Context, tenantID string, limit int) ([]models.Recommendation, error)
	GetRecommendation(ctx context.Context, tenantID string, id int64) (models.Recommendation, error)
}

type Service struct {
	store serviceStore
}

func NewService(store serviceStore) *Service {
	return &Service{store: store}
}

func (s *Service) RecordChange(ctx context.Context, entry models.ControlHistoryEntry) error {
	entry.Category = strings.ToLower(strings.TrimSpace(entry.Category))
	entry.Action = strings.ToLower(strings.TrimSpace(entry.Action))
	entry.Outcome = strings.ToLower(strings.TrimSpace(entry.Outcome))
	if entry.Outcome == "" {
		entry.Outcome = "success"
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	return s.store.CreateControlHistoryEntry(ctx, entry)
}

func (s *Service) ListControlHistory(ctx context.Context, tenantID, category, targetID string, limit, offset int) (*models.Page[models.ControlHistoryEntry], error) {
	return s.store.ListControlHistoryEntries(ctx, models.ControlHistoryQuery{
		TenantID: tenantID,
		Category: category,
		TargetID: targetID,
		Limit:    limit,
		Offset:   offset,
	})
}

func (s *Service) ListEvidenceBundles(ctx context.Context, tenantID string, limit int) ([]models.EvidenceBundle, error) {
	return s.store.ListEvidenceBundles(ctx, tenantID, limit)
}

func (s *Service) GetEvidenceBundle(ctx context.Context, tenantID string, bundleID int64) (models.EvidenceBundle, error) {
	return s.store.GetEvidenceBundle(ctx, tenantID, bundleID)
}

func (s *Service) CreateEvidenceBundle(ctx context.Context, tenantID, actor string, req models.EvidenceBundleRequest) (models.EvidenceBundle, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.Scope = strings.TrimSpace(req.Scope)
	if req.Name == "" {
		req.Name = "Incident bundle"
	}
	if req.Scope == "" {
		req.Scope = "incident"
	}

	filters := map[string]any{
		"trace_id":          strings.TrimSpace(req.TraceID),
		"prompt_id":         strings.TrimSpace(req.PromptID),
		"environment":       strings.TrimSpace(req.Environment),
		"release_tag":       strings.TrimSpace(req.ReleaseTag),
		"rollout_rule_id":   req.RolloutRuleID,
		"recommendation_id": req.RecommendationID,
		"reason":            strings.TrimSpace(req.Reason),
	}
	filterJSON, _ := json.Marshal(filters)

	items := make([]models.EvidenceBundleItem, 0, 32)
	summary := make([]string, 0, 8)

	historyPage, err := s.store.ListControlHistoryEntries(ctx, models.ControlHistoryQuery{
		TenantID: tenantID,
		Limit:    200,
	})
	if err != nil {
		return models.EvidenceBundle{}, err
	}
	historyCount := 0
	for _, entry := range historyPage.Items {
		if !matchesBundleRequest(entry.Category, entry.TargetType, entry.TargetID, req) {
			continue
		}
		items = append(items, marshalBundleItem("control_history", fmt.Sprintf("%s %s", entry.Category, entry.Action), "", entry.TargetType, entry.TargetID, entry))
		historyCount++
	}
	if historyCount > 0 {
		summary = append(summary, fmt.Sprintf("%d control-plane changes captured in the incident window.", historyCount))
	}

	adminAudit, err := s.store.ListAdminAuditEntries(ctx, tenantID, 100)
	if err != nil {
		return models.EvidenceBundle{}, err
	}
	adminCount := 0
	for _, entry := range adminAudit {
		if !matchesBundleRequest(entry.Category, entry.TargetType, entry.TargetID, req) {
			continue
		}
		items = append(items, marshalBundleItem("control_audit", fmt.Sprintf("%s %s", entry.Category, entry.Action), "", entry.TargetType, entry.TargetID, entry))
		adminCount++
	}
	if adminCount > 0 {
		summary = append(summary, fmt.Sprintf("%d legacy control-plane audit records were linked for operator context.", adminCount))
	}

	releases, err := s.store.ListPromptReleases(ctx, tenantID)
	if err != nil {
		return models.EvidenceBundle{}, err
	}
	releaseCount := 0
	for _, release := range releases {
		if !matchesRelease(release, req) {
			continue
		}
		items = append(items, marshalBundleItem("prompt_release", fmt.Sprintf("%s %s", release.PromptID, release.ReleaseTag), "", "prompt_release", release.ReleaseTag, release))
		releaseCount++
	}
	if releaseCount > 0 {
		summary = append(summary, fmt.Sprintf("%d prompt releases were tied to the selected scope.", releaseCount))
	}

	evalRuns, err := s.store.ListEvalRuns(ctx, tenantID, 200)
	if err != nil {
		return models.EvidenceBundle{}, err
	}
	evalCount := 0
	for _, run := range evalRuns {
		if !matchesEvalRun(run, req) {
			continue
		}
		items = append(items, marshalBundleItem("eval_run", fmt.Sprintf("%s %.1f", run.EvalSuite, run.OverallScore), run.TraceID, "release", run.ReleaseTag, run))
		evalCount++
	}
	if evalCount > 0 {
		summary = append(summary, fmt.Sprintf("%d evaluation runs contribute release-health evidence.", evalCount))
	}

	decisionsPage, err := s.store.ListDecisionRecords(ctx, models.DecisionQuery{
		TenantID:    tenantID,
		TraceID:     strings.TrimSpace(req.TraceID),
		PromptID:    strings.TrimSpace(req.PromptID),
		ReleaseTag:  strings.TrimSpace(req.ReleaseTag),
		Environment: strings.TrimSpace(req.Environment),
		Limit:       250,
	})
	if err != nil {
		return models.EvidenceBundle{}, err
	}
	if len(decisionsPage.Items) > 0 {
		for _, record := range decisionsPage.Items {
			items = append(items, marshalBundleItem("decision_record", fmt.Sprintf("%s %s", record.Type, record.Result), record.TraceID, "decision", record.DecisionID, record))
		}
		summary = append(summary, fmt.Sprintf("%d runtime decisions explain enforcement, fallback, budget, or routing behavior.", len(decisionsPage.Items)))
	}

	rolloutEvents, err := s.store.ListRolloutEvents(ctx, tenantID, models.RolloutEventQuery{
		TraceID:       strings.TrimSpace(req.TraceID),
		RolloutRuleID: req.RolloutRuleID,
		ReleaseTag:    strings.TrimSpace(req.ReleaseTag),
		Environment:   strings.TrimSpace(req.Environment),
		Limit:         200,
	})
	if err != nil {
		return models.EvidenceBundle{}, err
	}
	if len(rolloutEvents) > 0 {
		for _, event := range rolloutEvents {
			items = append(items, marshalBundleItem("rollout_event", fmt.Sprintf("rollout %d %s", event.RolloutRuleID, event.AssignedVariant), event.TraceID, "rollout_rule", strconv.FormatInt(event.RolloutRuleID, 10), event))
		}
		summary = append(summary, fmt.Sprintf("%d rollout events show assignment and auto-pause behavior.", len(rolloutEvents)))
	}

	if req.RecommendationID > 0 {
		recommendation, err := s.store.GetRecommendation(ctx, tenantID, req.RecommendationID)
		if err != nil {
			return models.EvidenceBundle{}, err
		}
		items = append(items, marshalBundleItem("recommendation", recommendation.Title, "", recommendation.TargetType, recommendation.TargetID, recommendation))
		summary = append(summary, "The selected recommendation is included with its confidence, blast radius, and evidence.")
	} else {
		recommendations, err := s.store.ListRecommendationsForBundle(ctx, tenantID, 100)
		if err != nil {
			return models.EvidenceBundle{}, err
		}
		recommendationCount := 0
		for _, recommendation := range recommendations {
			if !matchesRecommendation(recommendation, req) {
				continue
			}
			items = append(items, marshalBundleItem("recommendation", recommendation.Title, "", recommendation.TargetType, recommendation.TargetID, recommendation))
			recommendationCount++
		}
		if recommendationCount > 0 {
			summary = append(summary, fmt.Sprintf("%d recommendations are linked to the incident scope.", recommendationCount))
		}
	}

	if len(items) == 0 {
		summary = append(summary, "No linked evidence matched the requested scope yet; broaden the filters or generate the bundle after more runtime activity.")
	}

	return s.store.CreateEvidenceBundle(ctx, models.EvidenceBundle{
		TenantID:  tenantID,
		Name:      req.Name,
		Scope:     req.Scope,
		Status:    "ready",
		Filters:   string(filterJSON),
		Summary:   summary,
		CreatedBy: actor,
		Items:     items,
	})
}

func marshalBundleItem(itemType, title, traceID, targetType, targetID string, payload any) models.EvidenceBundleItem {
	raw, _ := json.Marshal(payload)
	return models.EvidenceBundleItem{
		ItemType:   itemType,
		ItemTitle:  strings.TrimSpace(title),
		TraceID:    strings.TrimSpace(traceID),
		TargetType: strings.TrimSpace(targetType),
		TargetID:   strings.TrimSpace(targetID),
		Payload:    string(raw),
	}
}

func matchesBundleRequest(category, targetType, targetID string, req models.EvidenceBundleRequest) bool {
	if req.RolloutRuleID > 0 && targetType == "rollout_rule" && targetID == strconv.FormatInt(req.RolloutRuleID, 10) {
		return true
	}
	if req.RecommendationID > 0 && targetType == "recommendation" && targetID == strconv.FormatInt(req.RecommendationID, 10) {
		return true
	}
	if releaseTag := strings.TrimSpace(req.ReleaseTag); releaseTag != "" && (targetID == releaseTag || strings.Contains(targetID, releaseTag)) {
		return true
	}
	if promptID := strings.TrimSpace(req.PromptID); promptID != "" && (targetID == promptID || strings.Contains(targetID, promptID)) {
		return true
	}
	if strings.TrimSpace(req.TraceID) == "" && strings.TrimSpace(req.ReleaseTag) == "" && strings.TrimSpace(req.PromptID) == "" && req.RolloutRuleID == 0 && req.RecommendationID == 0 {
		return category != ""
	}
	return false
}

func matchesRelease(release models.PromptRelease, req models.EvidenceBundleRequest) bool {
	if promptID := strings.TrimSpace(req.PromptID); promptID != "" && !strings.EqualFold(release.PromptID, promptID) {
		return false
	}
	if environment := strings.TrimSpace(req.Environment); environment != "" && !strings.EqualFold(release.Environment, environment) {
		return false
	}
	if releaseTag := strings.TrimSpace(req.ReleaseTag); releaseTag != "" && !strings.EqualFold(release.ReleaseTag, releaseTag) {
		return false
	}
	if strings.TrimSpace(req.TraceID) != "" || req.RolloutRuleID > 0 || req.RecommendationID > 0 {
		return strings.TrimSpace(req.ReleaseTag) != ""
	}
	return true
}

func matchesEvalRun(run models.TraceEvalRun, req models.EvidenceBundleRequest) bool {
	if traceID := strings.TrimSpace(req.TraceID); traceID != "" && run.TraceID != traceID {
		return false
	}
	if promptID := strings.TrimSpace(req.PromptID); promptID != "" && !strings.EqualFold(run.PromptID, promptID) {
		return false
	}
	if environment := strings.TrimSpace(req.Environment); environment != "" && !strings.EqualFold(run.PromptEnvironment, environment) {
		return false
	}
	if releaseTag := strings.TrimSpace(req.ReleaseTag); releaseTag != "" && !strings.EqualFold(run.ReleaseTag, releaseTag) {
		return false
	}
	return true
}

func matchesRecommendation(recommendation models.Recommendation, req models.EvidenceBundleRequest) bool {
	if releaseTag := strings.TrimSpace(req.ReleaseTag); releaseTag != "" {
		if raw, ok := recommendation.Evidence["release_tag"].(string); ok && strings.EqualFold(raw, releaseTag) {
			return true
		}
		if raw, ok := recommendation.Evidence["candidate_release"].(string); ok && strings.EqualFold(raw, releaseTag) {
			return true
		}
	}
	if req.RolloutRuleID > 0 && recommendation.TargetType == "rollout_rule" && recommendation.TargetID == strconv.FormatInt(req.RolloutRuleID, 10) {
		return true
	}
	if promptID := strings.TrimSpace(req.PromptID); promptID != "" {
		if raw, ok := recommendation.Evidence["prompt_id"].(string); ok && strings.EqualFold(raw, promptID) {
			return true
		}
	}
	if strings.TrimSpace(req.TraceID) == "" && strings.TrimSpace(req.ReleaseTag) == "" && strings.TrimSpace(req.PromptID) == "" && req.RolloutRuleID == 0 {
		return recommendation.Status != ""
	}
	return false
}
