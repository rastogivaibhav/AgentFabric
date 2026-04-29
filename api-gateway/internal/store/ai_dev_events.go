package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/govagn/api-gateway/internal/normalization"
	"github.com/jackc/pgx/v5"
)

func prepareCanonicalEventForStorage(event *normalization.CanonicalEvent) {
	if event == nil {
		return
	}
	if event.ID == "" {
		event.ID = uuid.New().String()
	}
	if event.EventTime.IsZero() {
		event.EventTime = time.Now()
	}
}

// StoreCanonicalEvent persists a canonical event to the ai_dev_events table
func (ps *PostgresStore) StoreCanonicalEvent(ctx context.Context, event *normalization.CanonicalEvent) error {
	if event == nil {
		return fmt.Errorf("event cannot be nil")
	}

	prepareCanonicalEventForStorage(event)

	// Marshal payload to JSON
	var payloadJSON []byte
	if event.Payload != nil {
		var err error
		payloadJSON, err = json.Marshal(event.Payload)
		if err != nil {
			return fmt.Errorf("marshal payload: %w", err)
		}
	} else {
		payloadJSON = []byte("{}")
	}

	query := `
		INSERT INTO ai_dev_events (
			id, ts, source_vendor, source_product, source_channel,
			user_id, user_email, team_id, repo, model,
			event_type, event_category, action, success,
			latency_ms, input_tokens, output_tokens,
			cache_read_tokens, cache_write_tokens,
			estimated_cost_usd, risk_score, risk_category,
			requires_review, payload, redacted, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15, $16, $17,
			$18, $19, $20, $21, $22, $23, $24, $25, $26
		)
		RETURNING id
	`

	err := ps.pool.QueryRow(ctx, query,
		event.ID,
		event.EventTime,
		event.SourceVendor,
		event.SourceProduct,
		event.SourceChannel,
		event.UserID,
		event.UserEmail,
		event.TeamID,
		event.RepoName,
		event.ModelName,
		event.EventType,
		event.EventCategory,
		event.Action,
		event.Success,
		event.LatencyMs,
		event.InputTokens,
		event.OutputTokens,
		event.CacheReadTokens,
		event.CacheWriteTokens,
		event.EstimatedCost,
		event.RiskScore,
		event.RiskCategory,
		event.RequiresReview,
		payloadJSON,
		event.Redacted,
		time.Now(),
	).Scan(&event.ID)

	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}

	return nil
}

// GetCanonicalEvent retrieves a canonical event by ID
func (ps *PostgresStore) GetCanonicalEvent(ctx context.Context, eventID string) (*normalization.CanonicalEvent, error) {
	query := `
		SELECT
			id, ts, source_vendor, source_product, source_channel,
			user_id, user_email, team_id, repo, model,
			event_type, event_category, action, success,
			latency_ms, input_tokens, output_tokens,
			cache_read_tokens, cache_write_tokens,
			estimated_cost_usd, risk_score, risk_category,
			requires_review, payload, redacted, created_at
		FROM ai_dev_events
		WHERE id = $1
	`

	var payloadJSON []byte
	event := &normalization.CanonicalEvent{}

	err := ps.pool.QueryRow(ctx, query, eventID).Scan(
		&event.ID,
		&event.EventTime,
		&event.SourceVendor,
		&event.SourceProduct,
		&event.SourceChannel,
		&event.UserID,
		&event.UserEmail,
		&event.TeamID,
		&event.RepoName,
		&event.ModelName,
		&event.EventType,
		&event.EventCategory,
		&event.Action,
		&event.Success,
		&event.LatencyMs,
		&event.InputTokens,
		&event.OutputTokens,
		&event.CacheReadTokens,
		&event.CacheWriteTokens,
		&event.EstimatedCost,
		&event.RiskScore,
		&event.RiskCategory,
		&event.RequiresReview,
		&payloadJSON,
		&event.Redacted,
	)

	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("event not found")
	}
	if err != nil {
		return nil, fmt.Errorf("query event: %w", err)
	}

	// Unmarshal payload
	if err := json.Unmarshal(payloadJSON, &event.Payload); err != nil {
		return nil, fmt.Errorf("unmarshal payload: %w", err)
	}

	return event, nil
}

// ListCanonicalEvents queries events with filtering and pagination
func (ps *PostgresStore) ListCanonicalEvents(ctx context.Context, filter *EventFilter) ([]*normalization.CanonicalEvent, int64, error) {
	baseQuery := `
		SELECT
			id, ts, source_vendor, source_product, source_channel,
			user_id, user_email, team_id, repo, model,
			event_type, event_category, action, success,
			latency_ms, input_tokens, output_tokens,
			cache_read_tokens, cache_write_tokens,
			estimated_cost_usd, risk_score, risk_category,
			requires_review, payload, redacted, created_at
		FROM ai_dev_events
	`

	countQuery := "SELECT COUNT(*) FROM ai_dev_events"

	whereClause, args := buildWhereClause(filter)

	if whereClause != "" {
		baseQuery += " WHERE " + whereClause
		countQuery += " WHERE " + whereClause
	}

	// Get total count
	var total int64
	err := ps.pool.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count events: %w", err)
	}

	// Add ordering and pagination
	baseQuery += " ORDER BY ts DESC LIMIT $" + fmt.Sprintf("%d", len(args)+1) + " OFFSET $" + fmt.Sprintf("%d", len(args)+2)
	args = append(args, filter.Limit, filter.Offset)

	rows, err := ps.pool.Query(ctx, baseQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	var events []*normalization.CanonicalEvent
	for rows.Next() {
		var payloadJSON []byte
		event := &normalization.CanonicalEvent{}

		err := rows.Scan(
			&event.ID,
			&event.EventTime,
			&event.SourceVendor,
			&event.SourceProduct,
			&event.SourceChannel,
			&event.UserID,
			&event.UserEmail,
			&event.TeamID,
			&event.RepoName,
			&event.ModelName,
			&event.EventType,
			&event.EventCategory,
			&event.Action,
			&event.Success,
			&event.LatencyMs,
			&event.InputTokens,
			&event.OutputTokens,
			&event.CacheReadTokens,
			&event.CacheWriteTokens,
			&event.EstimatedCost,
			&event.RiskScore,
			&event.RiskCategory,
			&event.RequiresReview,
			&payloadJSON,
			&event.Redacted,
		)

		if err != nil {
			return nil, 0, fmt.Errorf("scan event: %w", err)
		}

		// Unmarshal payload
		if err := json.Unmarshal(payloadJSON, &event.Payload); err != nil {
			return nil, 0, fmt.Errorf("unmarshal payload: %w", err)
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows error: %w", err)
	}

	return events, total, nil
}

// EventFilter provides query filters for canonical events
type EventFilter struct {
	SourceVendor   string
	EventType      string
	UserEmail      string
	SessionID      string
	RiskScoreMin   int
	RiskScoreMax   int
	RequiresReview *bool
	StartTime      time.Time
	EndTime        time.Time
	Limit          int64
	Offset         int64
}

// buildWhereClause constructs a WHERE clause from filter parameters
func buildWhereClause(filter *EventFilter) (string, []interface{}) {
	if filter == nil {
		return "", nil
	}

	var clauses []string
	var args []interface{}
	argCount := 1

	if filter.SourceVendor != "" {
		clauses = append(clauses, fmt.Sprintf("source_vendor = $%d", argCount))
		args = append(args, filter.SourceVendor)
		argCount++
	}

	if filter.EventType != "" {
		clauses = append(clauses, fmt.Sprintf("event_type = $%d", argCount))
		args = append(args, filter.EventType)
		argCount++
	}

	if filter.UserEmail != "" {
		clauses = append(clauses, fmt.Sprintf("user_email = $%d", argCount))
		args = append(args, filter.UserEmail)
		argCount++
	}

	if filter.SessionID != "" {
		clauses = append(clauses, fmt.Sprintf("payload->>'session_id' = $%d", argCount))
		args = append(args, filter.SessionID)
		argCount++
	}

	if filter.RiskScoreMin > 0 {
		clauses = append(clauses, fmt.Sprintf("risk_score >= $%d", argCount))
		args = append(args, filter.RiskScoreMin)
		argCount++
	}

	if filter.RiskScoreMax > 0 {
		clauses = append(clauses, fmt.Sprintf("risk_score <= $%d", argCount))
		args = append(args, filter.RiskScoreMax)
		argCount++
	}

	if filter.RequiresReview != nil {
		clauses = append(clauses, fmt.Sprintf("requires_review = $%d", argCount))
		args = append(args, *filter.RequiresReview)
		argCount++
	}

	if !filter.StartTime.IsZero() {
		clauses = append(clauses, fmt.Sprintf("ts >= $%d", argCount))
		args = append(args, filter.StartTime)
		argCount++
	}

	if !filter.EndTime.IsZero() {
		clauses = append(clauses, fmt.Sprintf("ts <= $%d", argCount))
		args = append(args, filter.EndTime)
		argCount++
	}

	if len(clauses) == 0 {
		return "", nil
	}

	whereClause := ""
	for i, clause := range clauses {
		if i > 0 {
			whereClause += " AND "
		}
		whereClause += clause
	}

	return whereClause, args
}
