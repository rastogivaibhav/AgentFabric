package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/govagn/api-gateway/internal/models"
)

func (s *PostgresStore) ListRolloutRules(ctx context.Context, tenantID string) ([]models.RolloutRule, error) {
	rows, err := s.pool.Query(ctx, `
		WITH recent AS (
			SELECT
				rollout_rule_id,
				COUNT(*) AS recent_requests,
				SUM(CASE WHEN LOWER(status) IN ('error', 'blocked') OR status_code >= 400 THEN 1 ELSE 0 END) AS recent_failures,
				MAX(created_at) AS last_event_at
			FROM rollout_events
			WHERE tenant_id = $1 AND created_at >= NOW() - INTERVAL '24 hours'
			GROUP BY rollout_rule_id
		)
		SELECT
			r.id, r.tenant_id, r.name, r.target_type, r.target_id, r.environment, r.percentage,
			COALESCE(r.control_model, ''), COALESCE(r.candidate_model, ''),
			COALESCE(r.control_release_tag, ''), COALESCE(r.candidate_release_tag, ''),
			COALESCE(r.policy_rule_id, 0), r.conditions::text, r.rollback_criteria::text,
			COALESCE(r.status, 'active'), COALESCE(r.created_by, ''), COALESCE(r.updated_by, ''),
			r.created_at, r.updated_at,
			COALESCE(recent.recent_requests, 0), COALESCE(recent.recent_failures, 0),
			CASE
				WHEN COALESCE(recent.recent_requests, 0) = 0 THEN 0
				ELSE COALESCE(recent.recent_failures, 0)::double precision / recent.recent_requests
			END AS recent_error_rate,
			recent.last_event_at
		FROM rollout_rules r
		LEFT JOIN recent ON recent.rollout_rule_id = r.id
		WHERE r.tenant_id = $1
		ORDER BY r.updated_at DESC, r.id DESC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []models.RolloutRule{}
	for rows.Next() {
		item, err := scanRolloutRule(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *PostgresStore) ListActiveRolloutRules(ctx context.Context, tenantID string) ([]models.RolloutRule, error) {
	rules, err := s.ListRolloutRules(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	active := make([]models.RolloutRule, 0, len(rules))
	for _, rule := range rules {
		if strings.EqualFold(rule.Status, models.RolloutStatusActive) {
			active = append(active, rule)
		}
	}
	return active, nil
}

func (s *PostgresStore) UpsertRolloutRule(ctx context.Context, tenantID string, rule models.RolloutRule, actor string) (models.RolloutRule, error) {
	conditionsJSON, _ := json.Marshal(rule.Conditions)
	rollbackJSON, _ := json.Marshal(rule.RollbackCriteria)
	rule.TargetType = strings.ToLower(strings.TrimSpace(rule.TargetType))
	rule.TargetID = strings.TrimSpace(rule.TargetID)
	rule.Name = strings.TrimSpace(rule.Name)
	rule.Environment = strings.ToLower(strings.TrimSpace(rule.Environment))
	rule.ControlModel = strings.TrimSpace(rule.ControlModel)
	rule.CandidateModel = strings.TrimSpace(rule.CandidateModel)
	rule.ControlReleaseTag = strings.TrimSpace(rule.ControlReleaseTag)
	rule.CandidateReleaseTag = strings.TrimSpace(rule.CandidateReleaseTag)
	if rule.Status == "" {
		rule.Status = models.RolloutStatusActive
	}
	if err := s.validateRolloutRule(rule); err != nil {
		return models.RolloutRule{}, err
	}

	if rule.ID > 0 {
		row := s.pool.QueryRow(ctx, `
			UPDATE rollout_rules
			SET name = $3,
			    target_type = $4,
			    target_id = $5,
			    environment = $6,
			    percentage = $7,
			    control_model = $8,
			    candidate_model = $9,
			    control_release_tag = $10,
			    candidate_release_tag = $11,
			    policy_rule_id = $12,
			    conditions = $13::jsonb,
			    rollback_criteria = $14::jsonb,
			    status = $15,
			    updated_by = $16,
			    updated_at = NOW()
			WHERE tenant_id = $1 AND id = $2
			RETURNING id, tenant_id, name, target_type, target_id, environment, percentage,
			          COALESCE(control_model, ''), COALESCE(candidate_model, ''),
			          COALESCE(control_release_tag, ''), COALESCE(candidate_release_tag, ''),
			          COALESCE(policy_rule_id, 0), conditions::text, rollback_criteria::text,
			          COALESCE(status, 'active'), COALESCE(created_by, ''), COALESCE(updated_by, ''),
			          created_at, updated_at, 0, 0, 0, NULL::timestamptz
		`,
			tenantID, rule.ID, rule.Name, rule.TargetType, rule.TargetID, rule.Environment, rule.Percentage,
			rule.ControlModel, rule.CandidateModel, rule.ControlReleaseTag, rule.CandidateReleaseTag, rule.PolicyRuleID,
			string(conditionsJSON), string(rollbackJSON), rule.Status, strings.TrimSpace(actor),
		)
		return scanRolloutRule(row)
	}

	row := s.pool.QueryRow(ctx, `
		INSERT INTO rollout_rules (
			tenant_id, name, target_type, target_id, environment, percentage,
			control_model, candidate_model, control_release_tag, candidate_release_tag, policy_rule_id,
			conditions, rollback_criteria, status, created_by, updated_by
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12::jsonb, $13::jsonb, $14, $15, $15)
		RETURNING id, tenant_id, name, target_type, target_id, environment, percentage,
		          COALESCE(control_model, ''), COALESCE(candidate_model, ''),
		          COALESCE(control_release_tag, ''), COALESCE(candidate_release_tag, ''),
		          COALESCE(policy_rule_id, 0), conditions::text, rollback_criteria::text,
		          COALESCE(status, 'active'), COALESCE(created_by, ''), COALESCE(updated_by, ''),
		          created_at, updated_at, 0, 0, 0, NULL::timestamptz
	`,
		tenantID, rule.Name, rule.TargetType, rule.TargetID, rule.Environment, rule.Percentage, rule.ControlModel,
		rule.CandidateModel, rule.ControlReleaseTag, rule.CandidateReleaseTag, rule.PolicyRuleID, string(conditionsJSON),
		string(rollbackJSON), rule.Status, strings.TrimSpace(actor),
	)
	return scanRolloutRule(row)
}

func (s *PostgresStore) UpdateRolloutRuleStatus(ctx context.Context, tenantID string, ruleID int64, status, actor string) (models.RolloutRule, error) {
	row := s.pool.QueryRow(ctx, `
		UPDATE rollout_rules
		SET status = $3, updated_by = $4, updated_at = NOW()
		WHERE tenant_id = $1 AND id = $2
		RETURNING id, tenant_id, name, target_type, target_id, environment, percentage,
		          COALESCE(control_model, ''), COALESCE(candidate_model, ''),
		          COALESCE(control_release_tag, ''), COALESCE(candidate_release_tag, ''),
		          COALESCE(policy_rule_id, 0), conditions::text, rollback_criteria::text,
		          COALESCE(status, 'active'), COALESCE(created_by, ''), COALESCE(updated_by, ''),
		          created_at, updated_at, 0, 0, 0, NULL::timestamptz
	`, tenantID, ruleID, strings.ToLower(strings.TrimSpace(status)), strings.TrimSpace(actor))
	return scanRolloutRule(row)
}

func (s *PostgresStore) CreateRolloutEvent(ctx context.Context, event models.RolloutEvent) (models.RolloutEvent, error) {
	err := s.pool.QueryRow(ctx, `
		INSERT INTO rollout_events (
			tenant_id, rollout_rule_id, trace_id, span_id, target_type, assigned_variant, assignment_key,
			provider, model, environment, prompt_id, prompt_release_tag, status, status_code, cost_usd,
			latency_ms, error_rate_snapshot, auto_paused
		)
		VALUES (
			$1, $2, NULLIF($3, ''), NULLIF($4, ''), $5, $6, $7,
			$8, $9, $10, $11, $12, $13, $14, $15,
			$16, $17, $18
		)
		RETURNING id, created_at
	`, event.TenantID, event.RolloutRuleID, event.TraceID, event.SpanID, event.TargetType, event.AssignedVariant,
		event.AssignmentKey, event.Provider, event.Model, event.Environment, event.PromptID, event.PromptReleaseTag,
		event.Status, event.StatusCode, event.CostUSD, event.LatencyMS, event.ErrorRateSnapshot, event.AutoPaused,
	).Scan(&event.ID, &event.CreatedAt)
	return event, err
}

func (s *PostgresStore) GetRolloutHealth(ctx context.Context, tenantID string, ruleID int64, since time.Duration) (requests int64, failures int64, errorRate float64, err error) {
	if since <= 0 {
		since = 24 * time.Hour
	}
	cutoff := time.Now().Add(-since)
	err = s.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) AS requests,
			COALESCE(SUM(CASE WHEN LOWER(status) IN ('error', 'blocked') OR status_code >= 400 THEN 1 ELSE 0 END), 0) AS failures
		FROM rollout_events
		WHERE tenant_id = $1 AND rollout_rule_id = $2 AND assigned_variant = 'canary' AND created_at >= $3
	`, tenantID, ruleID, cutoff).Scan(&requests, &failures)
	if err != nil {
		return 0, 0, 0, err
	}
	if requests > 0 {
		errorRate = float64(failures) / float64(requests)
	}
	return requests, failures, errorRate, nil
}

type rolloutRuleScanner interface {
	Scan(dest ...interface{}) error
}

func scanRolloutRule(scanner rolloutRuleScanner) (models.RolloutRule, error) {
	var item models.RolloutRule
	var conditionsJSON string
	var rollbackJSON string
	err := scanner.Scan(
		&item.ID, &item.TenantID, &item.Name, &item.TargetType, &item.TargetID, &item.Environment, &item.Percentage,
		&item.ControlModel, &item.CandidateModel, &item.ControlReleaseTag, &item.CandidateReleaseTag,
		&item.PolicyRuleID, &conditionsJSON, &rollbackJSON, &item.Status, &item.CreatedBy, &item.UpdatedBy,
		&item.CreatedAt, &item.UpdatedAt, &item.RecentRequests, &item.RecentFailures, &item.RecentErrorRate, &item.LastEventAt,
	)
	if err != nil {
		return models.RolloutRule{}, err
	}
	if strings.TrimSpace(conditionsJSON) != "" {
		_ = json.Unmarshal([]byte(conditionsJSON), &item.Conditions)
	}
	if strings.TrimSpace(rollbackJSON) != "" {
		_ = json.Unmarshal([]byte(rollbackJSON), &item.RollbackCriteria)
	}
	if item.Conditions == nil {
		item.Conditions = map[string]string{}
	}
	if item.RollbackCriteria == nil {
		item.RollbackCriteria = map[string]string{}
	}
	return item, nil
}

func (s *PostgresStore) validateRolloutRule(rule models.RolloutRule) error {
	if strings.TrimSpace(rule.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if strings.TrimSpace(rule.TargetType) == "" {
		return fmt.Errorf("target_type is required")
	}
	if rule.Percentage <= 0 || rule.Percentage > 100 {
		return fmt.Errorf("percentage must be between 1 and 100")
	}
	return nil
}
