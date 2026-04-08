package rollouts

import (
	"context"
	"fmt"
	"hash/fnv"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/govagn/api-gateway/internal/models"
	"github.com/govagn/api-gateway/internal/store"
)

type serviceStore interface {
	ListRolloutRules(ctx context.Context, tenantID string) ([]models.RolloutRule, error)
	ListActiveRolloutRules(ctx context.Context, tenantID string) ([]models.RolloutRule, error)
	UpsertRolloutRule(ctx context.Context, tenantID string, rule models.RolloutRule, actor string) (models.RolloutRule, error)
	UpdateRolloutRuleStatus(ctx context.Context, tenantID string, ruleID int64, status, actor string) (models.RolloutRule, error)
	CreateRolloutEvent(ctx context.Context, event models.RolloutEvent) (models.RolloutEvent, error)
	GetRolloutHealth(ctx context.Context, tenantID string, ruleID int64, since time.Duration) (requests int64, failures int64, errorRate float64, err error)
}

type Service struct {
	store serviceStore
}

func NewService(pg *store.PostgresStore) *Service {
	return &Service{store: pg}
}

func (s *Service) ListRules(ctx context.Context, tenantID string) ([]models.RolloutRule, error) {
	return s.store.ListRolloutRules(ctx, tenantID)
}

func (s *Service) UpsertRule(ctx context.Context, tenantID, actor string, rule models.RolloutRule) (models.RolloutRule, error) {
	rule.Name = strings.TrimSpace(rule.Name)
	rule.TargetType = strings.ToLower(strings.TrimSpace(rule.TargetType))
	rule.TargetID = strings.TrimSpace(rule.TargetID)
	rule.Environment = strings.ToLower(strings.TrimSpace(rule.Environment))
	rule.ControlModel = strings.TrimSpace(rule.ControlModel)
	rule.CandidateModel = strings.TrimSpace(rule.CandidateModel)
	rule.ControlReleaseTag = strings.TrimSpace(rule.ControlReleaseTag)
	rule.CandidateReleaseTag = strings.TrimSpace(rule.CandidateReleaseTag)
	if rule.Status == "" {
		rule.Status = models.RolloutStatusActive
	}
	if rule.Name == "" {
		return models.RolloutRule{}, fmt.Errorf("name is required")
	}
	if rule.TargetType != models.RolloutTargetModel && rule.TargetType != models.RolloutTargetPromptRelease && rule.TargetType != models.RolloutTargetPolicyRule {
		return models.RolloutRule{}, fmt.Errorf("target_type must be model, prompt_release, or policy_rule")
	}
	if rule.Percentage <= 0 || rule.Percentage > 100 {
		return models.RolloutRule{}, fmt.Errorf("percentage must be between 1 and 100")
	}
	switch rule.TargetType {
	case models.RolloutTargetModel:
		if rule.TargetID == "" {
			rule.TargetID = rule.ControlModel
		}
		if rule.TargetID == "" || rule.CandidateModel == "" {
			return models.RolloutRule{}, fmt.Errorf("model rollouts require target_id/control_model and candidate_model")
		}
	case models.RolloutTargetPromptRelease:
		if rule.TargetID == "" || rule.CandidateReleaseTag == "" {
			return models.RolloutRule{}, fmt.Errorf("prompt rollouts require target_id and candidate_release_tag")
		}
	case models.RolloutTargetPolicyRule:
		if rule.PolicyRuleID <= 0 && strings.TrimSpace(rule.TargetID) == "" {
			return models.RolloutRule{}, fmt.Errorf("policy rollouts require policy_rule_id or target_id")
		}
	}
	return s.store.UpsertRolloutRule(ctx, tenantID, rule, actor)
}

func (s *Service) UpdateStatus(ctx context.Context, tenantID string, ruleID int64, status, actor string) (models.RolloutRule, error) {
	status = strings.ToLower(strings.TrimSpace(status))
	if status != models.RolloutStatusActive && status != models.RolloutStatusPaused {
		return models.RolloutRule{}, fmt.Errorf("status must be active or paused")
	}
	return s.store.UpdateRolloutRuleStatus(ctx, tenantID, ruleID, status, actor)
}

func (s *Service) Preview(ctx context.Context, req models.RolloutPreviewRequest) (models.RolloutPreviewResponse, error) {
	tenantID := strings.TrimSpace(req.TenantID)
	rules, err := s.store.ListActiveRolloutRules(ctx, tenantID)
	if err != nil {
		return models.RolloutPreviewResponse{}, err
	}
	assignment := resolveAssignment(req, rules)
	return models.RolloutPreviewResponse{
		Assignment: assignment,
		Rules:      rules,
	}, nil
}

func (s *Service) Resolve(ctx context.Context, req models.RolloutPreviewRequest) (models.RolloutAssignment, error) {
	rules, err := s.store.ListActiveRolloutRules(ctx, strings.TrimSpace(req.TenantID))
	if err != nil {
		return models.RolloutAssignment{}, err
	}
	return resolveAssignment(req, rules), nil
}

func (s *Service) RecordOutcome(ctx context.Context, tenantID string, assignment models.RolloutAssignment, event models.RolloutEvent, actor string) (models.RolloutEvent, error) {
	if assignment.RuleID <= 0 {
		return models.RolloutEvent{}, nil
	}
	event.TenantID = tenantID
	event.RolloutRuleID = assignment.RuleID
	event.TargetType = assignment.TargetType
	event.AssignmentKey = assignment.AssignmentKey
	if event.AssignedVariant == "" {
		event.AssignedVariant = assignment.Variant
	}

	requests, _, errorRate, err := s.store.GetRolloutHealth(ctx, tenantID, assignment.RuleID, 24*time.Hour)
	if err != nil {
		return models.RolloutEvent{}, err
	}

	minRequests := parseInt(assignment.Metadata["rollback_min_requests"], 10)
	maxErrorPct := parseFloat(assignment.Metadata["rollback_max_error_rate_pct"], 50)
	autoPaused := false
	if assignment.Variant == "canary" && requests+1 >= int64(minRequests) && (errorRate*100) >= maxErrorPct {
		if _, err := s.store.UpdateRolloutRuleStatus(ctx, tenantID, assignment.RuleID, models.RolloutStatusPaused, actor); err == nil {
			autoPaused = true
		}
	}
	event.ErrorRateSnapshot = errorRate
	event.AutoPaused = autoPaused
	return s.store.CreateRolloutEvent(ctx, event)
}

func resolveAssignment(req models.RolloutPreviewRequest, rules []models.RolloutRule) models.RolloutAssignment {
	matched := make([]models.RolloutRule, 0, len(rules))
	for _, rule := range rules {
		if rolloutMatches(rule, req) {
			matched = append(matched, rule)
		}
	}
	sort.SliceStable(matched, func(i, j int) bool {
		if matched[i].Percentage != matched[j].Percentage {
			return matched[i].Percentage > matched[j].Percentage
		}
		return matched[i].ID < matched[j].ID
	})
	if len(matched) == 0 {
		return models.RolloutAssignment{Metadata: map[string]string{}}
	}

	rule := matched[0]
	assignmentKey := strings.TrimSpace(req.AssignmentKey)
	if assignmentKey == "" {
		assignmentKey = defaultAssignmentKey(rule, req)
	}
	bucket := stableBucket(rule, assignmentKey)
	selected := bucket <= rule.Percentage
	variant := "control"
	if selected {
		variant = "canary"
	}
	metadata := map[string]string{
		"rollout_name":       rule.Name,
		"rollout_percentage": strconv.Itoa(rule.Percentage),
	}
	for key, value := range rule.RollbackCriteria {
		metadata["rollback_"+key] = value
	}

	assignment := models.RolloutAssignment{
		Selected:       selected,
		RuleID:         rule.ID,
		RuleName:       rule.Name,
		TargetType:     rule.TargetType,
		TargetID:       rule.TargetID,
		Variant:        variant,
		AssignmentKey:  assignmentKey,
		Bucket:         bucket,
		ControlModel:   rule.ControlModel,
		CandidateModel: rule.CandidateModel,
		Metadata:       metadata,
	}
	switch rule.TargetType {
	case models.RolloutTargetModel:
		if selected {
			assignment.CandidateModel = rule.CandidateModel
		}
	case models.RolloutTargetPromptRelease:
		if selected {
			assignment.ReleaseTag = rule.CandidateReleaseTag
		} else {
			assignment.ReleaseTag = rule.ControlReleaseTag
		}
	}
	return assignment
}

func rolloutMatches(rule models.RolloutRule, req models.RolloutPreviewRequest) bool {
	if !strings.EqualFold(rule.Status, models.RolloutStatusActive) {
		return false
	}
	if env := strings.TrimSpace(rule.Environment); env != "" && !strings.EqualFold(env, req.Environment) {
		return false
	}
	for key, expected := range rule.Conditions {
		if !strings.EqualFold(strings.TrimSpace(conditionValue(key, req)), strings.TrimSpace(expected)) {
			return false
		}
	}
	switch rule.TargetType {
	case models.RolloutTargetModel:
		target := strings.TrimSpace(rule.TargetID)
		if target == "" {
			target = strings.TrimSpace(rule.ControlModel)
		}
		return strings.EqualFold(target, req.Model) || strings.EqualFold(rule.ControlModel, req.Model)
	case models.RolloutTargetPromptRelease:
		return strings.EqualFold(rule.TargetID, req.PromptID)
	case models.RolloutTargetPolicyRule:
		if rule.PolicyRuleID > 0 {
			return rule.PolicyRuleID == req.PolicyRuleID
		}
		return strings.EqualFold(rule.TargetID, strconv.FormatInt(req.PolicyRuleID, 10))
	default:
		return false
	}
}

func conditionValue(key string, req models.RolloutPreviewRequest) string {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "provider":
		return req.Provider
	case "model":
		return req.Model
	case "environment":
		return req.Environment
	case "app":
		return req.App
	case "session":
		return req.Session
	case "prompt_environment":
		return req.PromptEnvironment
	default:
		return ""
	}
}

func defaultAssignmentKey(rule models.RolloutRule, req models.RolloutPreviewRequest) string {
	parts := []string{
		strconv.FormatInt(rule.ID, 10),
		req.TenantID,
		req.Provider,
		req.Model,
		req.Environment,
		req.App,
		req.Session,
		req.PromptID,
		req.PromptEnvironment,
		strconv.FormatInt(req.PolicyRuleID, 10),
	}
	return strings.Join(parts, "|")
}

func stableBucket(rule models.RolloutRule, assignmentKey string) int {
	h := fnv.New32a()
	h.Write([]byte(strconv.FormatInt(rule.ID, 10)))
	h.Write([]byte("|"))
	h.Write([]byte(strings.TrimSpace(assignmentKey)))
	return int(h.Sum32()%100) + 1
}

func parseInt(value string, def int) int {
	if parsed, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && parsed > 0 {
		return parsed
	}
	return def
}

func parseFloat(value string, def float64) float64 {
	if parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil && parsed > 0 {
		return parsed
	}
	return def
}
