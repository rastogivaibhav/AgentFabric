package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/govagn/api-gateway/internal/models"
	"github.com/jackc/pgx/v5"
)

func (s *PostgresStore) CreateControlHistoryEntry(ctx context.Context, entry models.ControlHistoryEntry) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	previousHash := "genesis"
	_ = tx.QueryRow(ctx, `
		SELECT entry_hash
		FROM control_history
		WHERE tenant_id = $1
		ORDER BY id DESC
		LIMIT 1
	`, entry.TenantID).Scan(&previousHash)

	beforeJSON := "null"
	if strings.TrimSpace(entry.BeforeState) != "" {
		beforeJSON = entry.BeforeState
	}
	afterJSON := "null"
	if strings.TrimSpace(entry.AfterState) != "" {
		afterJSON = entry.AfterState
	}
	evidenceJSON, _ := json.Marshal(entry.EvidenceRefs)
	createdAt := entry.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	entryHash := controlHistoryHash(previousHash, entry, beforeJSON, afterJSON, string(evidenceJSON), createdAt)

	if _, err := tx.Exec(ctx, `
		INSERT INTO control_history (
			tenant_id, category, action, target_type, target_id, actor, reason, outcome,
			before_state, after_state, evidence_refs, previous_hash, entry_hash, created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10::jsonb, $11::jsonb, $12, $13, $14)
	`,
		entry.TenantID,
		strings.ToLower(strings.TrimSpace(entry.Category)),
		strings.ToLower(strings.TrimSpace(entry.Action)),
		strings.TrimSpace(entry.TargetType),
		strings.TrimSpace(entry.TargetID),
		strings.TrimSpace(entry.Actor),
		strings.TrimSpace(entry.Reason),
		strings.ToLower(strings.TrimSpace(entry.Outcome)),
		beforeJSON,
		afterJSON,
		string(evidenceJSON),
		previousHash,
		entryHash,
		createdAt,
	); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func controlHistoryHash(previousHash string, entry models.ControlHistoryEntry, beforeJSON, afterJSON, evidenceJSON string, createdAt time.Time) string {
	payload := strings.Join([]string{
		previousHash,
		entry.TenantID,
		strings.ToLower(strings.TrimSpace(entry.Category)),
		strings.ToLower(strings.TrimSpace(entry.Action)),
		strings.TrimSpace(entry.TargetType),
		strings.TrimSpace(entry.TargetID),
		strings.TrimSpace(entry.Actor),
		strings.TrimSpace(entry.Reason),
		strings.ToLower(strings.TrimSpace(entry.Outcome)),
		beforeJSON,
		afterJSON,
		evidenceJSON,
		createdAt.UTC().Format(time.RFC3339Nano),
	}, "|")
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

func (s *PostgresStore) ListControlHistoryEntries(ctx context.Context, query models.ControlHistoryQuery) (*models.Page[models.ControlHistoryEntry], error) {
	if query.Limit <= 0 || query.Limit > 500 {
		query.Limit = 100
	}

	args := []interface{}{query.TenantID}
	where := []string{"tenant_id = $1"}
	argIdx := 2

	if category := strings.TrimSpace(query.Category); category != "" {
		args = append(args, strings.ToLower(category))
		where = append(where, fmt.Sprintf("category = $%d", argIdx))
		argIdx++
	}
	if targetID := strings.TrimSpace(query.TargetID); targetID != "" {
		args = append(args, targetID)
		where = append(where, fmt.Sprintf("target_id = $%d", argIdx))
		argIdx++
	}

	var total int64
	countSQL := "SELECT COUNT(*) FROM control_history WHERE " + strings.Join(where, " AND ")
	if err := s.pool.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, err
	}

	args = append(args, query.Limit, query.Offset)
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, category, action, target_type, target_id, actor, reason, outcome,
		       COALESCE(before_state::text, ''), COALESCE(after_state::text, ''),
		       COALESCE(evidence_refs::text, '[]'), COALESCE(previous_hash, ''), COALESCE(entry_hash, ''), created_at
		FROM control_history
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY created_at DESC, id DESC
		LIMIT $`+fmt.Sprintf("%d", argIdx)+` OFFSET $`+fmt.Sprintf("%d", argIdx+1),
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]models.ControlHistoryEntry, 0, query.Limit)
	for rows.Next() {
		var item models.ControlHistoryEntry
		var refsJSON string
		if err := rows.Scan(
			&item.ID,
			&item.TenantID,
			&item.Category,
			&item.Action,
			&item.TargetType,
			&item.TargetID,
			&item.Actor,
			&item.Reason,
			&item.Outcome,
			&item.BeforeState,
			&item.AfterState,
			&refsJSON,
			&item.PreviousHash,
			&item.EntryHash,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		if strings.TrimSpace(refsJSON) != "" {
			_ = json.Unmarshal([]byte(refsJSON), &item.EvidenceRefs)
		}
		if item.EvidenceRefs == nil {
			item.EvidenceRefs = []string{}
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &models.Page[models.ControlHistoryEntry]{
		Items:   items,
		Total:   total,
		HasMore: int64(query.Offset+len(items)) < total,
	}, nil
}

func (s *PostgresStore) CreateEvidenceBundle(ctx context.Context, bundle models.EvidenceBundle) (models.EvidenceBundle, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return bundle, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	filtersJSON := "{}"
	if strings.TrimSpace(bundle.Filters) != "" {
		filtersJSON = bundle.Filters
	}
	summaryJSON, _ := json.Marshal(bundle.Summary)
	if err := tx.QueryRow(ctx, `
		INSERT INTO evidence_bundles (tenant_id, name, scope, status, filters, summary, created_by)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6::jsonb, $7)
		RETURNING id, created_at
	`, bundle.TenantID, strings.TrimSpace(bundle.Name), strings.TrimSpace(bundle.Scope), strings.TrimSpace(bundle.Status), filtersJSON, string(summaryJSON), strings.TrimSpace(bundle.CreatedBy)).Scan(&bundle.ID, &bundle.CreatedAt); err != nil {
		return bundle, err
	}

	for i := range bundle.Items {
		item := &bundle.Items[i]
		payloadJSON := "{}"
		if strings.TrimSpace(item.Payload) != "" {
			payloadJSON = item.Payload
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO evidence_bundle_items (
				evidence_bundle_id, tenant_id, item_type, item_title, trace_id, target_type, target_id, payload
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb)
			RETURNING id, created_at
		`, bundle.ID, bundle.TenantID, strings.TrimSpace(item.ItemType), strings.TrimSpace(item.ItemTitle), strings.TrimSpace(item.TraceID), strings.TrimSpace(item.TargetType), strings.TrimSpace(item.TargetID), payloadJSON).Scan(&item.ID, &item.CreatedAt); err != nil {
			return bundle, err
		}
		item.BundleID = bundle.ID
		item.TenantID = bundle.TenantID
	}
	bundle.ItemCount = len(bundle.Items)

	if err := tx.Commit(ctx); err != nil {
		return bundle, err
	}
	return bundle, nil
}

func (s *PostgresStore) ListEvidenceBundles(ctx context.Context, tenantID string, limit int) ([]models.EvidenceBundle, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT b.id, b.tenant_id, b.name, b.scope, b.status, COALESCE(b.filters::text, '{}'),
		       COALESCE(b.summary::text, '[]'), COALESCE(b.created_by, ''), b.created_at,
		       COALESCE(COUNT(i.id), 0) AS item_count
		FROM evidence_bundles b
		LEFT JOIN evidence_bundle_items i ON i.evidence_bundle_id = b.id
		WHERE b.tenant_id = $1
		GROUP BY b.id
		ORDER BY b.created_at DESC, b.id DESC
		LIMIT $2
	`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []models.EvidenceBundle{}
	for rows.Next() {
		var item models.EvidenceBundle
		var summaryJSON string
		if err := rows.Scan(&item.ID, &item.TenantID, &item.Name, &item.Scope, &item.Status, &item.Filters, &summaryJSON, &item.CreatedBy, &item.CreatedAt, &item.ItemCount); err != nil {
			return nil, err
		}
		if strings.TrimSpace(summaryJSON) != "" {
			_ = json.Unmarshal([]byte(summaryJSON), &item.Summary)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *PostgresStore) GetEvidenceBundle(ctx context.Context, tenantID string, bundleID int64) (models.EvidenceBundle, error) {
	var bundle models.EvidenceBundle
	var summaryJSON string
	err := s.pool.QueryRow(ctx, `
		SELECT b.id, b.tenant_id, b.name, b.scope, b.status, COALESCE(b.filters::text, '{}'),
		       COALESCE(b.summary::text, '[]'), COALESCE(b.created_by, ''), b.created_at,
		       COALESCE(COUNT(i.id), 0) AS item_count
		FROM evidence_bundles b
		LEFT JOIN evidence_bundle_items i ON i.evidence_bundle_id = b.id
		WHERE b.tenant_id = $1 AND b.id = $2
		GROUP BY b.id
	`, tenantID, bundleID).Scan(&bundle.ID, &bundle.TenantID, &bundle.Name, &bundle.Scope, &bundle.Status, &bundle.Filters, &summaryJSON, &bundle.CreatedBy, &bundle.CreatedAt, &bundle.ItemCount)
	if err != nil {
		return bundle, err
	}
	if strings.TrimSpace(summaryJSON) != "" {
		_ = json.Unmarshal([]byte(summaryJSON), &bundle.Summary)
	}
	items, err := s.ListEvidenceBundleItems(ctx, tenantID, bundleID)
	if err != nil {
		return bundle, err
	}
	bundle.Items = items
	return bundle, nil
}

func (s *PostgresStore) ListEvidenceBundleItems(ctx context.Context, tenantID string, bundleID int64) ([]models.EvidenceBundleItem, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, evidence_bundle_id, tenant_id, item_type, item_title, COALESCE(trace_id, ''), COALESCE(target_type, ''), COALESCE(target_id, ''), payload::text, created_at
		FROM evidence_bundle_items
		WHERE tenant_id = $1 AND evidence_bundle_id = $2
		ORDER BY created_at ASC, id ASC
	`, tenantID, bundleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []models.EvidenceBundleItem{}
	for rows.Next() {
		var item models.EvidenceBundleItem
		if err := rows.Scan(&item.ID, &item.BundleID, &item.TenantID, &item.ItemType, &item.ItemTitle, &item.TraceID, &item.TargetType, &item.TargetID, &item.Payload, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *PostgresStore) ListRolloutEvents(ctx context.Context, tenantID string, query models.RolloutEventQuery) ([]models.RolloutEvent, error) {
	if query.Limit <= 0 || query.Limit > 500 {
		query.Limit = 100
	}
	args := []interface{}{tenantID}
	where := []string{"tenant_id = $1"}
	argIdx := 2

	if traceID := strings.TrimSpace(query.TraceID); traceID != "" {
		args = append(args, traceID)
		where = append(where, fmt.Sprintf("trace_id = $%d", argIdx))
		argIdx++
	}
	if query.RolloutRuleID > 0 {
		args = append(args, query.RolloutRuleID)
		where = append(where, fmt.Sprintf("rollout_rule_id = $%d", argIdx))
		argIdx++
	}
	if releaseTag := strings.TrimSpace(query.ReleaseTag); releaseTag != "" {
		args = append(args, releaseTag)
		where = append(where, fmt.Sprintf("prompt_release_tag = $%d", argIdx))
		argIdx++
	}
	if environment := strings.TrimSpace(query.Environment); environment != "" {
		args = append(args, environment)
		where = append(where, fmt.Sprintf("environment = $%d", argIdx))
		argIdx++
	}

	args = append(args, query.Limit)
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, rollout_rule_id, COALESCE(trace_id, ''), COALESCE(span_id, ''), target_type,
		       assigned_variant, COALESCE(assignment_key, ''), COALESCE(provider, ''), COALESCE(model, ''),
		       COALESCE(environment, ''), COALESCE(prompt_id, ''), COALESCE(prompt_release_tag, ''),
		       COALESCE(status, ''), COALESCE(status_code, 0), COALESCE(cost_usd, 0), COALESCE(latency_ms, 0),
		       COALESCE(error_rate_snapshot, 0), COALESCE(auto_paused, false), created_at
		FROM rollout_events
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY created_at DESC, id DESC
		LIMIT $`+fmt.Sprintf("%d", argIdx),
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []models.RolloutEvent{}
	for rows.Next() {
		var item models.RolloutEvent
		if err := rows.Scan(
			&item.ID, &item.TenantID, &item.RolloutRuleID, &item.TraceID, &item.SpanID, &item.TargetType,
			&item.AssignedVariant, &item.AssignmentKey, &item.Provider, &item.Model, &item.Environment,
			&item.PromptID, &item.PromptReleaseTag, &item.Status, &item.StatusCode, &item.CostUSD,
			&item.LatencyMS, &item.ErrorRateSnapshot, &item.AutoPaused, &item.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *PostgresStore) GetRecommendation(ctx context.Context, tenantID string, id int64) (models.Recommendation, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT
			id, recommendation_key, tenant_id, recommendation_type, status, title, summary,
			target, target_type, target_id, suggested_action, estimated_impact, blast_radius,
			confidence, evidence::text, created_at, updated_at, last_seen_at
		FROM recommendations
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, id)
	return scanRecommendation(row)
}

func (s *PostgresStore) ListRecommendationsForBundle(ctx context.Context, tenantID string, limit int) ([]models.Recommendation, error) {
	if limit <= 0 || limit > 250 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT
			id, recommendation_key, tenant_id, recommendation_type, status, title, summary,
			target, target_type, target_id, suggested_action, estimated_impact, blast_radius,
			confidence, evidence::text, created_at, updated_at, last_seen_at
		FROM recommendations
		WHERE tenant_id = $1
		ORDER BY updated_at DESC, id DESC
		LIMIT $2
	`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]models.Recommendation, 0, limit)
	for rows.Next() {
		item, err := scanRecommendation(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
