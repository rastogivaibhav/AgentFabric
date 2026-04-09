package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/govagn/api-gateway/internal/models"
)

func (s *PostgresStore) ListEvalDatasets(ctx context.Context, tenantID string, limit int) ([]models.EvalDataset, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, dataset_id, version, name, dataset_type, description, owner_name, status, source, provenance,
		       redaction_status, approval_status, freshness_date, tags, metadata_json::text, created_at, updated_at
		FROM eval_datasets
		WHERE tenant_id = $1
		ORDER BY updated_at DESC, id DESC
		LIMIT $2
	`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	datasets := make([]models.EvalDataset, 0, limit)
	for rows.Next() {
		var item models.EvalDataset
		var rawMetadata string
		if err := rows.Scan(
			&item.ID, &item.DatasetID, &item.Version, &item.Name, &item.Type, &item.Description, &item.Owner,
			&item.Status, &item.Source, &item.Provenance, &item.RedactionStatus, &item.ApprovalStatus,
			&item.FreshnessDate, &item.Tags, &rawMetadata, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.Ref = evalDatasetRef(item.DatasetID, item.Version)
		item.Metadata = decodeAnyMap(rawMetadata)
		datasets = append(datasets, item)
	}
	return datasets, rows.Err()
}

func (s *PostgresStore) UpsertEvalDataset(ctx context.Context, tenantID string, dataset models.EvalDataset) (models.EvalDataset, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return dataset, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	metadataJSON, err := json.Marshal(dataset.Metadata)
	if err != nil {
		return dataset, err
	}
	var freshness any
	if dataset.FreshnessDate != nil {
		freshness = *dataset.FreshnessDate
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO eval_datasets (
			tenant_id, dataset_id, version, name, dataset_type, description, owner_name, status, source, provenance,
			redaction_status, approval_status, freshness_date, tags, metadata_json
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15::jsonb)
		ON CONFLICT (tenant_id, dataset_id, version) DO UPDATE SET
			name = EXCLUDED.name,
			dataset_type = EXCLUDED.dataset_type,
			description = EXCLUDED.description,
			owner_name = EXCLUDED.owner_name,
			status = EXCLUDED.status,
			source = EXCLUDED.source,
			provenance = EXCLUDED.provenance,
			redaction_status = EXCLUDED.redaction_status,
			approval_status = EXCLUDED.approval_status,
			freshness_date = EXCLUDED.freshness_date,
			tags = EXCLUDED.tags,
			metadata_json = EXCLUDED.metadata_json,
			updated_at = NOW()
		RETURNING id, created_at, updated_at
	`, tenantID, strings.TrimSpace(dataset.DatasetID), strings.TrimSpace(dataset.Version), strings.TrimSpace(dataset.Name),
		strings.TrimSpace(dataset.Type), strings.TrimSpace(dataset.Description), strings.TrimSpace(dataset.Owner),
		firstNonEmpty(strings.TrimSpace(dataset.Status), "approved"), firstNonEmpty(strings.TrimSpace(dataset.Source), "custom"),
		strings.TrimSpace(dataset.Provenance), strings.TrimSpace(dataset.RedactionStatus), strings.TrimSpace(dataset.ApprovalStatus),
		freshness, dataset.Tags, string(metadataJSON),
	).Scan(&dataset.ID, &dataset.CreatedAt, &dataset.UpdatedAt)
	if err != nil {
		return dataset, err
	}

	dataset.Ref = evalDatasetRef(dataset.DatasetID, dataset.Version)
	if _, err := tx.Exec(ctx, `DELETE FROM eval_dataset_items WHERE tenant_id = $1 AND dataset_ref = $2`, tenantID, dataset.Ref); err != nil {
		return dataset, err
	}
	items := make([]models.EvalDatasetItem, 0, len(dataset.Items))
	for _, item := range dataset.Items {
		inputJSON, err := json.Marshal(item.Input)
		if err != nil {
			return dataset, err
		}
		expectedJSON, err := json.Marshal(item.Expected)
		if err != nil {
			return dataset, err
		}
		metadataJSON, err := json.Marshal(item.Metadata)
		if err != nil {
			return dataset, err
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO eval_dataset_items (tenant_id, dataset_ref, item_key, input_json, expected_json, metadata_json, labels)
			VALUES ($1, $2, $3, $4::jsonb, $5::jsonb, $6::jsonb, $7)
			RETURNING id, created_at
		`, tenantID, dataset.Ref, strings.TrimSpace(item.ItemKey), string(inputJSON), string(expectedJSON), string(metadataJSON), item.Labels).Scan(&item.ID, &item.CreatedAt); err != nil {
			return dataset, err
		}
		item.DatasetRef = dataset.Ref
		items = append(items, item)
	}
	dataset.Items = items
	if err := tx.Commit(ctx); err != nil {
		return dataset, err
	}
	return dataset, nil
}

func (s *PostgresStore) ListEvalDatasetItems(ctx context.Context, tenantID string, datasetRefs []string, limit int) ([]models.EvalDatasetItem, error) {
	if len(datasetRefs) == 0 {
		return nil, nil
	}
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	refs := make([]string, 0, len(datasetRefs))
	for _, ref := range datasetRefs {
		if trimmed := strings.TrimSpace(ref); trimmed != "" {
			refs = append(refs, trimmed)
		}
	}
	if len(refs) == 0 {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, dataset_ref, item_key, input_json::text, expected_json::text, metadata_json::text, labels, created_at
		FROM eval_dataset_items
		WHERE tenant_id = $1 AND dataset_ref = ANY($2)
		ORDER BY dataset_ref ASC, item_key ASC
		LIMIT $3
	`, tenantID, refs, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]models.EvalDatasetItem, 0, limit)
	for rows.Next() {
		var item models.EvalDatasetItem
		var rawInput, rawExpected, rawMetadata string
		if err := rows.Scan(&item.ID, &item.DatasetRef, &item.ItemKey, &rawInput, &rawExpected, &rawMetadata, &item.Labels, &item.CreatedAt); err != nil {
			return nil, err
		}
		item.Input = decodeAnyMap(rawInput)
		item.Expected = decodeAnyMap(rawExpected)
		item.Metadata = decodeAnyMap(rawMetadata)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *PostgresStore) CreateEvalExecution(ctx context.Context, tenantID string, execution models.EvalExecution) (models.EvalExecution, error) {
	traceJSON, err := json.Marshal(execution.TraceIDs)
	if err != nil {
		return execution, err
	}
	datasetJSON, err := json.Marshal(execution.DatasetRefs)
	if err != nil {
		return execution, err
	}
	attributesJSON, err := json.Marshal(execution.Attributes)
	if err != nil {
		return execution, err
	}
	policyJSON, err := json.Marshal(execution.PolicyEffectiveness)
	if err != nil {
		return execution, err
	}
	err = s.pool.QueryRow(ctx, `
		INSERT INTO eval_executions (
			tenant_id, pack_id, mode, status, release_tag, trace_ids, dataset_refs, attributes, sample_limit,
			overall_score, risk_level, summary, policy_effectiveness, eval_run_id
		)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7::jsonb, $8::jsonb, $9, $10, $11, $12, $13::jsonb, NULLIF($14, 0))
		RETURNING id, created_at, updated_at
	`, tenantID, strings.TrimSpace(execution.PackID), firstNonEmpty(strings.TrimSpace(execution.Mode), "offline"),
		firstNonEmpty(strings.TrimSpace(execution.Status), "queued"), strings.TrimSpace(execution.ReleaseTag),
		string(traceJSON), string(datasetJSON), string(attributesJSON), execution.SampleLimit, execution.OverallScore,
		firstNonEmpty(strings.TrimSpace(execution.RiskLevel), "unknown"), strings.TrimSpace(execution.Summary), string(policyJSON), execution.RunID,
	).Scan(&execution.ID, &execution.CreatedAt, &execution.UpdatedAt)
	return execution, err
}

func (s *PostgresStore) UpdateEvalExecution(ctx context.Context, tenantID string, execution models.EvalExecution) (models.EvalExecution, error) {
	traceJSON, err := json.Marshal(execution.TraceIDs)
	if err != nil {
		return execution, err
	}
	datasetJSON, err := json.Marshal(execution.DatasetRefs)
	if err != nil {
		return execution, err
	}
	attributesJSON, err := json.Marshal(execution.Attributes)
	if err != nil {
		return execution, err
	}
	policyJSON, err := json.Marshal(execution.PolicyEffectiveness)
	if err != nil {
		return execution, err
	}
	err = s.pool.QueryRow(ctx, `
		UPDATE eval_executions
		SET mode = $3,
		    status = $4,
		    release_tag = $5,
		    trace_ids = $6::jsonb,
		    dataset_refs = $7::jsonb,
		    attributes = $8::jsonb,
		    sample_limit = $9,
		    overall_score = $10,
		    risk_level = $11,
		    summary = $12,
		    policy_effectiveness = $13::jsonb,
		    eval_run_id = NULLIF($14, 0),
		    updated_at = NOW()
		WHERE tenant_id = $1 AND id = $2
		RETURNING created_at, updated_at
	`, tenantID, execution.ID, firstNonEmpty(strings.TrimSpace(execution.Mode), "offline"),
		firstNonEmpty(strings.TrimSpace(execution.Status), "queued"), strings.TrimSpace(execution.ReleaseTag),
		string(traceJSON), string(datasetJSON), string(attributesJSON), execution.SampleLimit, execution.OverallScore,
		firstNonEmpty(strings.TrimSpace(execution.RiskLevel), "unknown"), strings.TrimSpace(execution.Summary),
		string(policyJSON), execution.RunID,
	).Scan(&execution.CreatedAt, &execution.UpdatedAt)
	return execution, err
}

func (s *PostgresStore) InsertEvalExecutionItem(ctx context.Context, tenantID string, item models.EvalExecutionItem) (models.EvalExecutionItem, error) {
	evidenceJSON, err := json.Marshal(item.Evidence)
	if err != nil {
		return item, err
	}
	err = s.pool.QueryRow(ctx, `
		INSERT INTO eval_execution_items (
			tenant_id, execution_id, item_ref, item_type, trace_id, dataset_ref, status, overall_score, risk_level, summary, evidence_json
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::jsonb)
		RETURNING id, created_at
	`, tenantID, item.ExecutionID, strings.TrimSpace(item.ItemRef), firstNonEmpty(strings.TrimSpace(item.ItemType), "trace"),
		strings.TrimSpace(item.TraceID), strings.TrimSpace(item.DatasetRef), firstNonEmpty(strings.TrimSpace(item.Status), "completed"),
		item.OverallScore, firstNonEmpty(strings.TrimSpace(item.RiskLevel), "unknown"), strings.TrimSpace(item.Summary), string(evidenceJSON),
	).Scan(&item.ID, &item.CreatedAt)
	return item, err
}

func (s *PostgresStore) InsertEvalEvaluatorResults(ctx context.Context, tenantID string, executionID, itemID int64, results []models.EvalEvaluatorResult) error {
	for _, result := range results {
		inputJSON, err := json.Marshal(result.InputFields)
		if err != nil {
			return err
		}
		detailsJSON, err := json.Marshal(result.Details)
		if err != nil {
			return err
		}
		if _, err := s.pool.Exec(ctx, `
			INSERT INTO eval_evaluator_results (
				tenant_id, execution_id, execution_item_id, evaluator_id, dimension_id, evaluator_type, method, score,
				severity, status, summary, input_fields, details_json
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12::jsonb, $13::jsonb)
		`, tenantID, executionID, itemID, strings.TrimSpace(result.EvaluatorID), strings.TrimSpace(result.DimensionID),
			strings.TrimSpace(result.EvaluatorType), strings.TrimSpace(result.Method), result.Score, strings.TrimSpace(result.Severity),
			firstNonEmpty(strings.TrimSpace(result.Status), "completed"), strings.TrimSpace(result.Summary), string(inputJSON), string(detailsJSON)); err != nil {
			return err
		}
	}
	return nil
}

func (s *PostgresStore) InsertEvalEvidenceLinks(ctx context.Context, tenantID string, executionID, itemID int64, links []models.EvalEvidenceLink) error {
	for _, link := range links {
		metadataJSON, err := json.Marshal(link.Metadata)
		if err != nil {
			return err
		}
		if _, err := s.pool.Exec(ctx, `
			INSERT INTO eval_evidence_links (tenant_id, execution_id, execution_item_id, link_type, ref_id, label, metadata_json)
			VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb)
		`, tenantID, executionID, itemID, strings.TrimSpace(link.LinkType), strings.TrimSpace(link.RefID), strings.TrimSpace(link.Label), string(metadataJSON)); err != nil {
			return err
		}
	}
	return nil
}

func (s *PostgresStore) ListEvalExecutions(ctx context.Context, tenantID string, limit int) ([]models.EvalExecution, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, pack_id, mode, status, release_tag, trace_ids::text, dataset_refs::text, attributes::text, sample_limit,
		       overall_score, risk_level, summary, policy_effectiveness::text, COALESCE(eval_run_id, 0), created_at, updated_at
		FROM eval_executions
		WHERE tenant_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2
	`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	executions := make([]models.EvalExecution, 0, limit)
	for rows.Next() {
		item, err := scanEvalExecutionRow(rows)
		if err != nil {
			return nil, err
		}
		executions = append(executions, item)
	}
	return executions, rows.Err()
}

func (s *PostgresStore) GetEvalExecution(ctx context.Context, tenantID string, executionID int64) (models.EvalExecution, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, pack_id, mode, status, release_tag, trace_ids::text, dataset_refs::text, attributes::text, sample_limit,
		       overall_score, risk_level, summary, policy_effectiveness::text, COALESCE(eval_run_id, 0), created_at, updated_at
		FROM eval_executions
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, executionID)
	execution, err := scanEvalExecutionRow(row)
	if err != nil {
		return execution, err
	}
	items, err := s.listEvalExecutionItems(ctx, tenantID, execution.ID)
	if err != nil {
		return execution, err
	}
	execution.Items = items
	return execution, nil
}

func (s *PostgresStore) listEvalExecutionItems(ctx context.Context, tenantID string, executionID int64) ([]models.EvalExecutionItem, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, execution_id, item_ref, item_type, trace_id, dataset_ref, status, overall_score, risk_level, summary, evidence_json::text, created_at
		FROM eval_execution_items
		WHERE tenant_id = $1 AND execution_id = $2
		ORDER BY id ASC
	`, tenantID, executionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []models.EvalExecutionItem{}
	for rows.Next() {
		var item models.EvalExecutionItem
		var rawEvidence string
		if err := rows.Scan(&item.ID, &item.ExecutionID, &item.ItemRef, &item.ItemType, &item.TraceID, &item.DatasetRef,
			&item.Status, &item.OverallScore, &item.RiskLevel, &item.Summary, &rawEvidence, &item.CreatedAt); err != nil {
			return nil, err
		}
		item.Evidence = decodeAnyMap(rawEvidence)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return items, nil
	}
	itemIDs := make([]int64, 0, len(items))
	itemIndex := make(map[int64]int, len(items))
	for i := range items {
		itemIDs = append(itemIDs, items[i].ID)
		itemIndex[items[i].ID] = i
	}

	resultRows, err := s.pool.Query(ctx, `
		SELECT id, execution_id, execution_item_id, evaluator_id, dimension_id, evaluator_type, method, score, severity, status, summary, input_fields::text, details_json::text, created_at
		FROM eval_evaluator_results
		WHERE tenant_id = $1 AND execution_item_id = ANY($2)
		ORDER BY execution_item_id ASC, evaluator_id ASC
	`, tenantID, itemIDs)
	if err != nil {
		return nil, err
	}
	defer resultRows.Close()
	for resultRows.Next() {
		var result models.EvalEvaluatorResult
		var itemID int64
		var rawFields, rawDetails string
		if err := resultRows.Scan(&result.ID, &result.ExecutionID, &itemID, &result.EvaluatorID, &result.DimensionID, &result.EvaluatorType,
			&result.Method, &result.Score, &result.Severity, &result.Status, &result.Summary, &rawFields, &rawDetails, &result.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(rawFields), &result.InputFields)
		result.Details = decodeAnyMap(rawDetails)
		result.ItemID = itemID
		if idx, ok := itemIndex[itemID]; ok {
			items[idx].EvaluatorResults = append(items[idx].EvaluatorResults, result)
		}
	}
	if err := resultRows.Err(); err != nil {
		return nil, err
	}

	linkRows, err := s.pool.Query(ctx, `
		SELECT id, execution_id, execution_item_id, link_type, ref_id, label, metadata_json::text, created_at
		FROM eval_evidence_links
		WHERE tenant_id = $1 AND execution_item_id = ANY($2)
		ORDER BY execution_item_id ASC, id ASC
	`, tenantID, itemIDs)
	if err != nil {
		return nil, err
	}
	defer linkRows.Close()
	for linkRows.Next() {
		var link models.EvalEvidenceLink
		var itemID int64
		var rawMetadata string
		if err := linkRows.Scan(&link.ID, &link.ExecutionID, &itemID, &link.LinkType, &link.RefID, &link.Label, &rawMetadata, &link.CreatedAt); err != nil {
			return nil, err
		}
		link.ItemID = itemID
		link.Metadata = decodeAnyMap(rawMetadata)
		if idx, ok := itemIndex[itemID]; ok {
			items[idx].EvidenceLinks = append(items[idx].EvidenceLinks, link)
		}
	}
	return items, linkRows.Err()
}

func scanEvalExecutionRow(scanner interface {
	Scan(dest ...any) error
}) (models.EvalExecution, error) {
	var execution models.EvalExecution
	var rawTraceIDs, rawDatasetRefs, rawAttributes, rawPolicy string
	if err := scanner.Scan(&execution.ID, &execution.PackID, &execution.Mode, &execution.Status, &execution.ReleaseTag,
		&rawTraceIDs, &rawDatasetRefs, &rawAttributes, &execution.SampleLimit, &execution.OverallScore, &execution.RiskLevel,
		&execution.Summary, &rawPolicy, &execution.RunID, &execution.CreatedAt, &execution.UpdatedAt); err != nil {
		return execution, err
	}
	_ = json.Unmarshal([]byte(rawTraceIDs), &execution.TraceIDs)
	_ = json.Unmarshal([]byte(rawDatasetRefs), &execution.DatasetRefs)
	execution.Attributes = decodeAnyMap(rawAttributes)
	execution.PolicyEffectiveness = decodePolicyEffectiveness(rawPolicy)
	return execution, nil
}

func decodeAnyMap(raw string) map[string]any {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}
	}
	out := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return map[string]any{}
	}
	return out
}

func evalDatasetRef(datasetID, version string) string {
	if strings.TrimSpace(datasetID) == "" {
		return ""
	}
	if strings.TrimSpace(version) == "" {
		return strings.TrimSpace(datasetID)
	}
	return strings.TrimSpace(datasetID) + "." + strings.TrimSpace(version)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func (s *PostgresStore) DeleteEvalExecutionDetails(ctx context.Context, tenantID string, executionID int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM eval_execution_items WHERE tenant_id = $1 AND execution_id = $2`, tenantID, executionID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() < 0 {
		return fmt.Errorf("unexpected rows affected when clearing execution details")
	}
	return nil
}
