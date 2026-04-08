package store

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/govagn/api-gateway/internal/models"
)

func (s *PostgresStore) ListPromptVersions(ctx context.Context, tenantID string) ([]models.PromptVersion, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			pv.id,
			pv.tenant_id,
			pv.prompt_id,
			pv.version_num,
			pv.environment,
			COALESCE(pv.release_tag, ''),
			pv.content,
			pv.config_json::text,
			COALESCE(pv.description, ''),
			COALESCE(pv.created_by, ''),
			pv.created_at,
			pv.updated_at,
			pv.version_num = MAX(pv.version_num) OVER (PARTITION BY pv.tenant_id, pv.prompt_id) AS is_latest
		FROM prompt_versions pv
		WHERE pv.tenant_id = $1
		ORDER BY pv.prompt_id ASC, pv.version_num DESC, pv.updated_at DESC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []models.PromptVersion{}
	for rows.Next() {
		var item models.PromptVersion
		var rawConfig string
		if err := rows.Scan(
			&item.ID,
			&item.TenantID,
			&item.PromptID,
			&item.Version,
			&item.Environment,
			&item.ReleaseTag,
			&item.Content,
			&rawConfig,
			&item.Description,
			&item.CreatedBy,
			&item.CreatedAt,
			&item.UpdatedAt,
			&item.IsLatest,
		); err != nil {
			return nil, err
		}
		item.Config = decodePromptConfig(rawConfig)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *PostgresStore) ListPromptReleases(ctx context.Context, tenantID string) ([]models.PromptRelease, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, prompt_id, environment, version_num, release_tag, COALESCE(status, 'active'), COALESCE(notes, ''), COALESCE(promotion_reason, ''), COALESCE(promoted_by, ''), created_at
		FROM prompt_releases
		WHERE tenant_id = $1
		ORDER BY prompt_id ASC, environment ASC, CASE COALESCE(status, 'active') WHEN 'active' THEN 0 WHEN 'candidate' THEN 1 WHEN 'superseded' THEN 2 ELSE 3 END, created_at DESC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []models.PromptRelease{}
	for rows.Next() {
		var item models.PromptRelease
		if err := rows.Scan(&item.ID, &item.TenantID, &item.PromptID, &item.Environment, &item.Version, &item.ReleaseTag, &item.Status, &item.Notes, &item.PromotionReason, &item.PromotedBy, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *PostgresStore) UpsertPromptVersion(ctx context.Context, tenantID string, version models.PromptVersion) (models.PromptVersion, error) {
	rawConfigJSON, err := json.Marshal(version.Config)
	if err != nil {
		return version, err
	}
	promptID := strings.TrimSpace(version.PromptID)
	environment := strings.TrimSpace(version.Environment)
	if environment == "" {
		environment = "development"
	}

	var rawConfig string
	err = s.pool.QueryRow(ctx, `
		WITH next_version AS (
			SELECT CASE
				WHEN $3::integer > 0 THEN $3::integer
				ELSE COALESCE(MAX(version_num), 0) + 1
			END AS version_num
			FROM prompt_versions
			WHERE tenant_id = $1 AND prompt_id = $2
		)
		INSERT INTO prompt_versions (
			tenant_id, prompt_id, version_num, environment, release_tag, content, config_json, description, created_by
		)
		SELECT
			$1, $2, next_version.version_num, $4, $5, $6, $7::jsonb, $8, $9
		FROM next_version
		ON CONFLICT (tenant_id, prompt_id, version_num)
		DO UPDATE SET
			environment = EXCLUDED.environment,
			release_tag = EXCLUDED.release_tag,
			content = EXCLUDED.content,
			config_json = EXCLUDED.config_json,
			description = EXCLUDED.description,
			created_by = EXCLUDED.created_by,
			updated_at = NOW()
		RETURNING id, tenant_id, prompt_id, version_num, environment, COALESCE(release_tag, ''), content, config_json::text, COALESCE(description, ''), COALESCE(created_by, ''), created_at, updated_at
	`, tenantID, promptID, version.Version, environment, strings.TrimSpace(version.ReleaseTag), version.Content, string(rawConfigJSON), strings.TrimSpace(version.Description), strings.TrimSpace(version.CreatedBy)).Scan(
		&version.ID,
		&version.TenantID,
		&version.PromptID,
		&version.Version,
		&version.Environment,
		&version.ReleaseTag,
		&version.Content,
		&rawConfig,
		&version.Description,
		&version.CreatedBy,
		&version.CreatedAt,
		&version.UpdatedAt,
	)
	if err != nil {
		return version, err
	}
	version.Config = decodePromptConfig(rawConfig)
	return version, nil
}

func (s *PostgresStore) GetPromptVersion(ctx context.Context, tenantID, promptID string, versionNum int) (models.PromptVersion, error) {
	var item models.PromptVersion
	var rawConfig string
	err := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, prompt_id, version_num, environment, COALESCE(release_tag, ''), content, config_json::text, COALESCE(description, ''), COALESCE(created_by, ''), created_at, updated_at
		FROM prompt_versions
		WHERE tenant_id = $1 AND prompt_id = $2 AND version_num = $3
	`, tenantID, strings.TrimSpace(promptID), versionNum).Scan(
		&item.ID,
		&item.TenantID,
		&item.PromptID,
		&item.Version,
		&item.Environment,
		&item.ReleaseTag,
		&item.Content,
		&rawConfig,
		&item.Description,
		&item.CreatedBy,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return item, err
	}
	item.Config = decodePromptConfig(rawConfig)
	return item, nil
}

func (s *PostgresStore) PromotePromptRelease(ctx context.Context, tenantID string, release models.PromptRelease) (models.PromptRelease, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return release, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	release.Status = strings.TrimSpace(release.Status)
	if release.Status == "" {
		release.Status = "active"
	}

	if release.Status == "active" {
		if _, err := tx.Exec(ctx, `
			UPDATE prompt_releases
			SET status = 'superseded'
			WHERE tenant_id = $1 AND prompt_id = $2 AND environment = $3 AND COALESCE(status, 'active') = 'active'
		`, tenantID, strings.TrimSpace(release.PromptID), strings.TrimSpace(release.Environment)); err != nil {
			return release, err
		}
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO prompt_releases (tenant_id, prompt_id, environment, version_num, release_tag, status, notes, promotion_reason, promoted_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, tenant_id, prompt_id, environment, version_num, release_tag, COALESCE(status, 'active'), COALESCE(notes, ''), COALESCE(promotion_reason, ''), COALESCE(promoted_by, ''), created_at
	`, tenantID, strings.TrimSpace(release.PromptID), strings.TrimSpace(release.Environment), release.Version, strings.TrimSpace(release.ReleaseTag), strings.TrimSpace(release.Status), strings.TrimSpace(release.Notes), strings.TrimSpace(release.PromotionReason), strings.TrimSpace(release.PromotedBy)).Scan(
		&release.ID,
		&release.TenantID,
		&release.PromptID,
		&release.Environment,
		&release.Version,
		&release.ReleaseTag,
		&release.Status,
		&release.Notes,
		&release.PromotionReason,
		&release.PromotedBy,
		&release.CreatedAt,
	)
	if err != nil {
		return release, err
	}
	if err := tx.Commit(ctx); err != nil {
		return release, err
	}
	return release, nil
}

func decodePromptConfig(raw string) map[string]string {
	if strings.TrimSpace(raw) == "" {
		return map[string]string{}
	}
	out := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return map[string]string{}
	}
	return out
}
