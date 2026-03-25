package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agentfabric/api-gateway/internal/models"
)

func (s *PostgresStore) CreateDecisionRecord(ctx context.Context, record models.DecisionRecord) error {
	inputsJSON, _ := json.Marshal(record.Inputs)
	evidenceJSON, _ := json.Marshal(record.Evidence)
	record.Result = models.NormalizeDecisionResult(record.Result)
	record.Explanation = models.DefaultDecisionExplanation(record)

	_, err := s.pool.Exec(ctx, `
		INSERT INTO decision_records (
			decision_id, tenant_id, trace_id, span_id, decision_type, result, reason, explanation,
			trigger, inputs, evidence, action_taken, source, framework, app_name, environment,
			provider, model, prompt_id, prompt_release_tag
		)
		VALUES (
			$1, $2, NULLIF($3, ''), NULLIF($4, ''), $5, $6, $7, $8,
			$9, $10::jsonb, $11::jsonb, $12, $13, $14, $15, $16,
			$17, $18, $19, $20
		)
	`,
		record.DecisionID, record.TenantID, record.TraceID, record.SpanID, strings.ToLower(strings.TrimSpace(record.Type)),
		record.Result, record.Reason, record.Explanation, record.Trigger, string(inputsJSON), string(evidenceJSON),
		record.ActionTaken, record.Source, record.Framework, record.AppName, record.Environment, record.Provider,
		record.Model, record.PromptID, record.PromptReleaseTag,
	)
	return err
}

func (s *PostgresStore) ListDecisionRecords(ctx context.Context, query models.DecisionQuery) (*models.Page[models.DecisionRecord], error) {
	if query.Limit <= 0 || query.Limit > 500 {
		query.Limit = 100
	}

	args := []interface{}{query.TenantID}
	where := "WHERE tenant_id = $1"
	argIdx := 2

	if strings.TrimSpace(query.TraceID) != "" {
		where += fmt.Sprintf(" AND trace_id = $%d", argIdx)
		args = append(args, strings.TrimSpace(query.TraceID))
		argIdx++
	}
	if strings.TrimSpace(query.Type) != "" {
		where += fmt.Sprintf(" AND decision_type = $%d", argIdx)
		args = append(args, strings.ToLower(strings.TrimSpace(query.Type)))
		argIdx++
	}
	if strings.TrimSpace(query.Result) != "" {
		where += fmt.Sprintf(" AND result = $%d", argIdx)
		args = append(args, models.NormalizeDecisionResult(query.Result))
		argIdx++
	}

	var total int64
	countQuery := "SELECT COUNT(*) FROM decision_records " + where
	if err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, err
	}

	args = append(args, query.Limit, query.Offset)
	rows, err := s.pool.Query(ctx, `
		SELECT id, decision_id, tenant_id, COALESCE(trace_id, ''), COALESCE(span_id, ''),
		       decision_type, result, COALESCE(reason, ''), COALESCE(explanation, ''),
		       COALESCE(trigger, ''), COALESCE(inputs, '{}'::jsonb)::text, COALESCE(evidence, '{}'::jsonb)::text,
		       COALESCE(action_taken, ''), COALESCE(source, ''), COALESCE(framework, ''),
		       COALESCE(app_name, ''), COALESCE(environment, ''), COALESCE(provider, ''),
		       COALESCE(model, ''), COALESCE(prompt_id, ''), COALESCE(prompt_release_tag, ''), created_at
		FROM decision_records
		`+where+`
		ORDER BY created_at DESC, id DESC
		LIMIT $`+fmt.Sprintf("%d", argIdx)+` OFFSET $`+fmt.Sprintf("%d", argIdx+1),
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]models.DecisionRecord, 0, query.Limit)
	for rows.Next() {
		record, err := scanDecisionRecord(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &models.Page[models.DecisionRecord]{
		Items:   items,
		Total:   total,
		HasMore: int64(query.Offset+len(items)) < total,
	}, nil
}

func (s *PostgresStore) ListDecisionRecordsForTrace(ctx context.Context, tenantID, traceID string) ([]models.DecisionRecord, error) {
	page, err := s.ListDecisionRecords(ctx, models.DecisionQuery{
		TenantID: tenantID,
		TraceID:  traceID,
		Limit:    500,
	})
	if err != nil {
		return nil, err
	}
	if page.Items == nil {
		return []models.DecisionRecord{}, nil
	}
	return page.Items, nil
}

type decisionRecordScanner interface {
	Scan(dest ...interface{}) error
}

func scanDecisionRecord(scanner decisionRecordScanner) (models.DecisionRecord, error) {
	var record models.DecisionRecord
	var inputsJSON string
	var evidenceJSON string
	if err := scanner.Scan(
		&record.ID, &record.DecisionID, &record.TenantID, &record.TraceID, &record.SpanID,
		&record.Type, &record.Result, &record.Reason, &record.Explanation,
		&record.Trigger, &inputsJSON, &evidenceJSON, &record.ActionTaken, &record.Source,
		&record.Framework, &record.AppName, &record.Environment, &record.Provider,
		&record.Model, &record.PromptID, &record.PromptReleaseTag, &record.CreatedAt,
	); err != nil {
		return models.DecisionRecord{}, err
	}
	if strings.TrimSpace(inputsJSON) != "" {
		_ = json.Unmarshal([]byte(inputsJSON), &record.Inputs)
	}
	if strings.TrimSpace(evidenceJSON) != "" {
		_ = json.Unmarshal([]byte(evidenceJSON), &record.Evidence)
	}
	if record.Inputs == nil {
		record.Inputs = map[string]string{}
	}
	if record.Evidence == nil {
		record.Evidence = map[string]interface{}{}
	}
	return record, nil
}
