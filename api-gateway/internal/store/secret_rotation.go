package store

import (
	"context"
	"fmt"
	"time"

	"github.com/govagn/api-gateway/internal/models"
)

// RecordSecretRotation logs a secret rotation event in the audit table.
func (s *PostgresStore) RecordSecretRotation(ctx context.Context, record *models.SecretRotationRecord) error {
	query := `
		INSERT INTO secret_rotation_log (
			key_id,
			old_key_hash,
			new_key_hash,
			items_rotated,
			status,
			encrypted_by_user,
			error_message,
			rotation_duration_seconds,
			created_at,
			completed_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
		)
		RETURNING id, created_at
	`

	var id int64
	var createdAt time.Time

	err := s.pool.QueryRow(ctx, query,
		record.KeyID,
		record.OldKeyHash,
		record.NewKeyHash,
		record.ItemsRotated,
		record.Status,
		record.EncryptedByUser,
		record.ErrorMessage,
		record.DurationSeconds,
		time.Now(),
		record.CompletedAt,
	).Scan(&id, &createdAt)

	if err != nil {
		return fmt.Errorf("failed to record rotation: %w", err)
	}

	record.ID = id
	record.CreatedAt = createdAt

	return nil
}

// GetSecretRotationHistory retrieves the rotation history for auditing.
func (s *PostgresStore) GetSecretRotationHistory(ctx context.Context, limit int) ([]models.SecretRotationRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}

	query := `
		SELECT
			id,
			key_id,
			old_key_hash,
			new_key_hash,
			items_rotated,
			status,
			encrypted_by_user,
			error_message,
			rotation_duration_seconds,
			created_at,
			completed_at
		FROM secret_rotation_log
		ORDER BY created_at DESC
		LIMIT $1
	`

	rows, err := s.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("query rotation history: %w", err)
	}
	defer rows.Close()

	var records []models.SecretRotationRecord

	for rows.Next() {
		var record models.SecretRotationRecord
		err := rows.Scan(
			&record.ID,
			&record.KeyID,
			&record.OldKeyHash,
			&record.NewKeyHash,
			&record.ItemsRotated,
			&record.Status,
			&record.EncryptedByUser,
			&record.ErrorMessage,
			&record.DurationSeconds,
			&record.CreatedAt,
			&record.CompletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan rotation record: %w", err)
		}

		records = append(records, record)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return records, nil
}

// GetLastSecretRotation retrieves the most recent rotation record.
func (s *PostgresStore) GetLastSecretRotation(ctx context.Context) (*models.SecretRotationRecord, error) {
	query := `
		SELECT
			id,
			key_id,
			old_key_hash,
			new_key_hash,
			items_rotated,
			status,
			encrypted_by_user,
			error_message,
			rotation_duration_seconds,
			created_at,
			completed_at
		FROM secret_rotation_log
		ORDER BY created_at DESC
		LIMIT 1
	`

	var record models.SecretRotationRecord

	err := s.pool.QueryRow(ctx, query).Scan(
		&record.ID,
		&record.KeyID,
		&record.OldKeyHash,
		&record.NewKeyHash,
		&record.ItemsRotated,
		&record.Status,
		&record.EncryptedByUser,
		&record.ErrorMessage,
		&record.DurationSeconds,
		&record.CreatedAt,
		&record.CompletedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("query last rotation: %w", err)
	}

	return &record, nil
}
