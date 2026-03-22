package store

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/agentfabric/api-gateway/internal/models"
	"github.com/jackc/pgx/v5"
)

func (s *PostgresStore) ListTraceSavedViews(ctx context.Context, tenantID string) ([]models.TraceSavedView, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, COALESCE(description,''), filters::text, COALESCE(created_by,''), is_pinned, created_at, updated_at
		FROM trace_saved_views
		WHERE tenant_id = $1
		ORDER BY is_pinned DESC, updated_at DESC, id DESC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	views := []models.TraceSavedView{}
	for rows.Next() {
		var view models.TraceSavedView
		var rawFilters string
		if err := rows.Scan(&view.ID, &view.Name, &view.Description, &rawFilters, &view.CreatedBy, &view.IsPinned, &view.CreatedAt, &view.UpdatedAt); err != nil {
			return nil, err
		}
		view.Filters = decodeTraceSavedViewFilters(rawFilters)
		views = append(views, view)
	}
	return views, rows.Err()
}

func (s *PostgresStore) UpsertTraceSavedView(ctx context.Context, tenantID string, view models.TraceSavedView) (models.TraceSavedView, error) {
	filtersJSON, err := json.Marshal(normalizeTraceSavedViewFilters(view.Filters))
	if err != nil {
		return view, err
	}
	if view.ID > 0 {
		err = s.pool.QueryRow(ctx, `
			UPDATE trace_saved_views
			SET name = $3,
			    description = $4,
			    filters = $5::jsonb,
			    created_by = $6,
			    is_pinned = $7,
			    updated_at = NOW()
			WHERE id = $1 AND tenant_id = $2
			RETURNING id, name, COALESCE(description,''), filters::text, COALESCE(created_by,''), is_pinned, created_at, updated_at
		`, view.ID, tenantID, strings.TrimSpace(view.Name), strings.TrimSpace(view.Description), string(filtersJSON), strings.TrimSpace(view.CreatedBy), view.IsPinned).Scan(
			&view.ID, &view.Name, &view.Description, &filtersJSON, &view.CreatedBy, &view.IsPinned, &view.CreatedAt, &view.UpdatedAt,
		)
		view.Filters = decodeTraceSavedViewFilters(string(filtersJSON))
		return view, err
	}
	err = s.pool.QueryRow(ctx, `
		INSERT INTO trace_saved_views (tenant_id, name, description, filters, created_by, is_pinned)
		VALUES ($1, $2, $3, $4::jsonb, $5, $6)
		RETURNING id, name, COALESCE(description,''), filters::text, COALESCE(created_by,''), is_pinned, created_at, updated_at
	`, tenantID, strings.TrimSpace(view.Name), strings.TrimSpace(view.Description), string(filtersJSON), strings.TrimSpace(view.CreatedBy), view.IsPinned).Scan(
		&view.ID, &view.Name, &view.Description, &filtersJSON, &view.CreatedBy, &view.IsPinned, &view.CreatedAt, &view.UpdatedAt,
	)
	view.Filters = decodeTraceSavedViewFilters(string(filtersJSON))
	return view, err
}

func (s *PostgresStore) DeleteTraceSavedView(ctx context.Context, tenantID string, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM trace_saved_views WHERE tenant_id = $1 AND id = $2`, tenantID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func normalizeTraceSavedViewFilters(filters map[string]string) map[string]string {
	if len(filters) == 0 {
		return map[string]string{}
	}
	normalized := make(map[string]string, len(filters))
	for key, value := range filters {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		normalized[key] = value
	}
	return normalized
}

func decodeTraceSavedViewFilters(raw string) map[string]string {
	if strings.TrimSpace(raw) == "" {
		return map[string]string{}
	}
	out := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return map[string]string{}
	}
	return normalizeTraceSavedViewFilters(out)
}
