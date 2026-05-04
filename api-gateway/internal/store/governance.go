package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/govagn/api-gateway/internal/models"
)

func (s *PostgresStore) ListGovernanceAlerts(ctx context.Context, tenantID string, limit int) ([]models.GovernanceAlert, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT span_id, trace_id, risk_score, risk_category, framework, received_at,
		       COALESCE(NULLIF(attributes->>'gen_ai.prompt', ''), NULLIF(attributes->>'ai.prompt', ''), name, '') AS summary
		FROM spans
		WHERE tenant_id = $1 AND risk_score > 0
		ORDER BY risk_score DESC, received_at DESC
		LIMIT $2
	`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	alerts := make([]models.GovernanceAlert, 0, limit)
	for rows.Next() {
		var alert models.GovernanceAlert
		if err := rows.Scan(&alert.SpanID, &alert.TraceID, &alert.RiskScore, &alert.RiskCategory, &alert.Framework, &alert.Timestamp, &alert.Summary); err != nil {
			return nil, err
		}
		alert.Summary = governanceAlertSummary(alert)
		alert.ActionRequired = alert.RiskScore >= 70
		alerts = append(alerts, alert)
	}
	return alerts, rows.Err()
}

func (s *PostgresStore) GetGovernanceSummary(ctx context.Context, tenantID string) (models.GovernanceSummary, error) {
	summary := models.GovernanceSummary{
		Categories: map[string]int{},
		Trend:      "stable",
	}
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*), COALESCE(COUNT(*) FILTER (WHERE risk_score >= 70), 0)
		FROM spans
		WHERE tenant_id = $1
	`, tenantID).Scan(&summary.TotalEvents, &summary.HighRiskCount); err != nil {
		return summary, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT risk_category, COUNT(*)
		FROM spans
		WHERE tenant_id = $1 AND risk_score > 0 AND risk_category <> ''
		GROUP BY risk_category
		ORDER BY COUNT(*) DESC
	`, tenantID)
	if err != nil {
		return summary, err
	}
	defer rows.Close()

	for rows.Next() {
		var category string
		var count int
		if err := rows.Scan(&category, &count); err != nil {
			return summary, err
		}
		summary.Categories[category] = count
	}
	if err := rows.Err(); err != nil {
		return summary, err
	}
	if summary.HighRiskCount > 0 {
		summary.Trend = "attention_required"
	}
	return summary, nil
}

func governanceAlertSummary(alert models.GovernanceAlert) string {
	category := strings.ReplaceAll(strings.TrimSpace(alert.RiskCategory), "_", " ")
	if category == "" {
		category = "risk"
	}
	framework := strings.TrimSpace(alert.Framework)
	if framework == "" {
		framework = "agent"
	}
	return fmt.Sprintf("%s event flagged for %s with score %d", framework, category, alert.RiskScore)
}
