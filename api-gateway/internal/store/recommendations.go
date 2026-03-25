package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/agentfabric/api-gateway/internal/models"
)

func (s *PostgresStore) UpsertRecommendation(ctx context.Context, record models.Recommendation) (models.Recommendation, error) {
	evidenceJSON, _ := json.Marshal(record.Evidence)
	record.Type = strings.ToLower(strings.TrimSpace(record.Type))
	record.Status = models.NormalizeRecommendationStatus(record.Status)
	record.TargetType = strings.TrimSpace(record.TargetType)
	record.TargetID = strings.TrimSpace(record.TargetID)
	record.Target = strings.TrimSpace(record.Target)
	record.Title = strings.TrimSpace(record.Title)
	record.Summary = strings.TrimSpace(record.Summary)
	record.SuggestedAction = strings.TrimSpace(record.SuggestedAction)
	record.EstimatedImpact = strings.TrimSpace(record.EstimatedImpact)
	record.BlastRadius = strings.TrimSpace(record.BlastRadius)
	record.Key = strings.TrimSpace(record.Key)

	row := s.pool.QueryRow(ctx, `
		INSERT INTO recommendations (
			recommendation_key, tenant_id, recommendation_type, status, title, summary,
			target, target_type, target_id, suggested_action, estimated_impact,
			blast_radius, confidence, evidence
		)
		VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11,
			$12, $13, $14::jsonb
		)
		ON CONFLICT (tenant_id, recommendation_key) DO UPDATE SET
			recommendation_type = EXCLUDED.recommendation_type,
			status = CASE
				WHEN recommendations.status IN ('reviewing', 'applied', 'dismissed') THEN recommendations.status
				WHEN recommendations.status = 'resolved' THEN 'open'
				ELSE EXCLUDED.status
			END,
			title = EXCLUDED.title,
			summary = EXCLUDED.summary,
			target = EXCLUDED.target,
			target_type = EXCLUDED.target_type,
			target_id = EXCLUDED.target_id,
			suggested_action = EXCLUDED.suggested_action,
			estimated_impact = EXCLUDED.estimated_impact,
			blast_radius = EXCLUDED.blast_radius,
			confidence = EXCLUDED.confidence,
			evidence = EXCLUDED.evidence,
			updated_at = NOW(),
			last_seen_at = NOW()
		RETURNING
			id, recommendation_key, tenant_id, recommendation_type, status, title, summary,
			target, target_type, target_id, suggested_action, estimated_impact, blast_radius,
			confidence, evidence::text, created_at, updated_at, last_seen_at
	`,
		record.Key,
		record.TenantID,
		record.Type,
		record.Status,
		record.Title,
		record.Summary,
		record.Target,
		record.TargetType,
		record.TargetID,
		record.SuggestedAction,
		record.EstimatedImpact,
		record.BlastRadius,
		record.Confidence,
		string(evidenceJSON),
	)
	return scanRecommendation(row)
}

func (s *PostgresStore) ResolveStaleRecommendations(ctx context.Context, tenantID string, activeKeys []string) error {
	if len(activeKeys) == 0 {
		_, err := s.pool.Exec(ctx, `
			UPDATE recommendations
			SET status = 'resolved', updated_at = NOW()
			WHERE tenant_id = $1 AND status IN ('open', 'reviewing')
		`, tenantID)
		return err
	}

	_, err := s.pool.Exec(ctx, `
		UPDATE recommendations
		SET status = 'resolved', updated_at = NOW()
		WHERE tenant_id = $1
		  AND status IN ('open', 'reviewing')
		  AND recommendation_key <> ALL($2)
	`, tenantID, activeKeys)
	return err
}

func (s *PostgresStore) ListRecommendations(ctx context.Context, query models.RecommendationQuery) (*models.Page[models.Recommendation], error) {
	if query.Limit <= 0 || query.Limit > 250 {
		query.Limit = 50
	}

	args := []interface{}{query.TenantID}
	where := []string{"tenant_id = $1"}
	argIdx := 2

	if typeFilter := strings.TrimSpace(query.Type); typeFilter != "" {
		args = append(args, strings.ToLower(typeFilter))
		where = append(where, fmt.Sprintf("recommendation_type = $%d", argIdx))
		argIdx++
	}
	if statusFilter := strings.TrimSpace(query.Status); statusFilter != "" {
		args = append(args, models.NormalizeRecommendationStatus(statusFilter))
		where = append(where, fmt.Sprintf("status = $%d", argIdx))
		argIdx++
	} else {
		where = append(where, "status NOT IN ('dismissed', 'resolved')")
	}

	countSQL := `SELECT COUNT(*) FROM recommendations WHERE ` + strings.Join(where, " AND ")
	var total int64
	if err := s.pool.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, err
	}

	args = append(args, query.Limit, query.Offset)
	rows, err := s.pool.Query(ctx, `
		SELECT
			id, recommendation_key, tenant_id, recommendation_type, status, title, summary,
			target, target_type, target_id, suggested_action, estimated_impact, blast_radius,
			confidence, evidence::text, created_at, updated_at, last_seen_at
		FROM recommendations
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY
			CASE status
				WHEN 'open' THEN 0
				WHEN 'reviewing' THEN 1
				WHEN 'applied' THEN 2
				WHEN 'dismissed' THEN 3
				ELSE 4
			END,
			confidence DESC,
			updated_at DESC,
			id DESC
		LIMIT $`+fmt.Sprintf("%d", argIdx)+` OFFSET $`+fmt.Sprintf("%d", argIdx+1),
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]models.Recommendation, 0, query.Limit)
	for rows.Next() {
		item, err := scanRecommendation(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &models.Page[models.Recommendation]{
		Items:   items,
		Total:   total,
		HasMore: int64(query.Offset+len(items)) < total,
	}, nil
}

func (s *PostgresStore) UpdateRecommendationStatus(ctx context.Context, tenantID string, id int64, status string) (models.Recommendation, error) {
	row := s.pool.QueryRow(ctx, `
		UPDATE recommendations
		SET status = $3, updated_at = NOW()
		WHERE tenant_id = $1 AND id = $2
		RETURNING
			id, recommendation_key, tenant_id, recommendation_type, status, title, summary,
			target, target_type, target_id, suggested_action, estimated_impact, blast_radius,
			confidence, evidence::text, created_at, updated_at, last_seen_at
	`, tenantID, id, models.NormalizeRecommendationStatus(status))
	return scanRecommendation(row)
}

func (s *PostgresStore) ListRecommendationPolicySignals(ctx context.Context, tenantID string, since time.Duration, limit int) ([]models.RecommendationPolicySignal, error) {
	if since <= 0 {
		since = 24 * time.Hour
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		SELECT
			COALESCE(NULLIF(app_name, ''), 'unknown') AS app_name,
			COALESCE(NULLIF(environment, ''), 'unknown') AS environment,
			COALESCE(NULLIF(provider, ''), 'unknown') AS provider,
			COALESCE(NULLIF(model, ''), 'unknown') AS model,
			COALESCE(NULLIF(prompt_id, ''), 'unknown') AS prompt_id,
			COALESCE(NULLIF(prompt_release_tag, ''), 'unreleased') AS prompt_release_tag,
			SUM(CASE WHEN result = 'warn' THEN 1 ELSE 0 END) AS warn_count,
			SUM(CASE WHEN result = 'sanitize' THEN 1 ELSE 0 END) AS sanitize_count,
			SUM(CASE WHEN result = 'deny' THEN 1 ELSE 0 END) AS deny_count,
			COUNT(*) AS total_count
		FROM decision_records
		WHERE tenant_id = $1
		  AND decision_type = 'policy'
		  AND created_at >= NOW() - $2::interval
		GROUP BY app_name, environment, provider, model, prompt_id, prompt_release_tag
		HAVING COUNT(*) > 0
		ORDER BY total_count DESC, sanitize_count DESC, warn_count DESC
		LIMIT $3
	`), tenantID, since.String(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	signals := []models.RecommendationPolicySignal{}
	for rows.Next() {
		var signal models.RecommendationPolicySignal
		if err := rows.Scan(
			&signal.AppName,
			&signal.Environment,
			&signal.Provider,
			&signal.Model,
			&signal.PromptID,
			&signal.PromptReleaseTag,
			&signal.WarnCount,
			&signal.SanitizeCount,
			&signal.DenyCount,
			&signal.TotalCount,
		); err != nil {
			return nil, err
		}
		signals = append(signals, signal)
	}
	return signals, rows.Err()
}

type recommendationScanner interface {
	Scan(dest ...interface{}) error
}

func scanRecommendation(scanner recommendationScanner) (models.Recommendation, error) {
	var item models.Recommendation
	var evidenceJSON string
	if err := scanner.Scan(
		&item.ID,
		&item.Key,
		&item.TenantID,
		&item.Type,
		&item.Status,
		&item.Title,
		&item.Summary,
		&item.Target,
		&item.TargetType,
		&item.TargetID,
		&item.SuggestedAction,
		&item.EstimatedImpact,
		&item.BlastRadius,
		&item.Confidence,
		&evidenceJSON,
		&item.CreatedAt,
		&item.UpdatedAt,
		&item.LastSeenAt,
	); err != nil {
		return models.Recommendation{}, err
	}
	if strings.TrimSpace(evidenceJSON) != "" {
		_ = json.Unmarshal([]byte(evidenceJSON), &item.Evidence)
	}
	if item.Evidence == nil {
		item.Evidence = map[string]interface{}{}
	}
	return item, nil
}
