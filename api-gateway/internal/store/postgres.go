package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/govagn/api-gateway/internal/budget"
	"github.com/govagn/api-gateway/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

type PostgresStore struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

const spanOutcomeExpr = `
	COALESCE(
		NULLIF(attributes->>'af.outcome_status', ''),
		CASE
			WHEN COALESCE(attributes->>'af.policy.blocked', 'false') = 'true'
				OR LOWER(COALESCE(attributes->>'af.policy.decision', '')) = 'deny'
				OR status_code IN (401, 403, 429) THEN 'blocked'
			WHEN status_code = 2 OR status_code >= 500 THEN 'error'
			WHEN LOWER(COALESCE(attributes->>'af.gateway.route_source', '')) = 'fallback' THEN 'degraded'
			ELSE 'ok'
		END
	)
`

func spanOutcomeFailureExpr() string {
	return "(" + spanOutcomeExpr + " IN ('error', 'blocked', 'degraded'))"
}

func spanOutcomeBlockedExpr() string {
	return "(" + spanOutcomeExpr + " = 'blocked')"
}

func NewPostgresStore(dsn string, logger *zap.Logger) (*PostgresStore, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	cfg.MaxConns = 25
	cfg.MinConns = 5
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.HealthCheckPeriod = 1 * time.Minute

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("pool init: %w", err)
	}

	s := &PostgresStore{pool: pool, logger: logger}
	// Schema is managed by golang-migrate: deploy/migrations/*.up.sql.
	// runMigrations() in cmd/server/main.go applies all pending migrations
	// at process startup before this store is created.
	return s, nil
}

// ─── Span writes ─────────────────────────────────────────────────────────────

func (s *PostgresStore) BulkInsertSpans(ctx context.Context, spans []models.Span) error {
	if len(spans) == 0 {
		return nil
	}

	rows := make([][]interface{}, 0, len(spans))
	for _, sp := range spans {
		attrsJSON, _ := json.Marshal(sp.Attributes)
		eventsJSON, _ := json.Marshal(sp.Events)
		rows = append(rows, []interface{}{
			sp.ID, sp.TraceID, sp.ParentID, sp.RunID, sp.Name,
			sp.Framework, sp.StartTimeNs, sp.DurationNs,
			sp.StatusCode, sp.StatusMsg,
			attrsJSON, eventsJSON,
			sp.InputTokens, sp.OutputTokens, sp.CacheReadTokens, sp.CacheWriteTokens, sp.ReasoningTokens,
			sp.CostUSD, sp.InputCostUSD, sp.OutputCostUSD, sp.CacheReadCostUSD, sp.CacheWriteCostUSD, sp.ReasoningCostUSD,
			sp.RiskScore, sp.RiskCategory,
			sp.TenantID,
		})
	}

	_, err := s.pool.CopyFrom(ctx,
		pgx.Identifier{"spans"},
		[]string{
			"span_id", "trace_id", "parent_span_id", "run_id", "name",
			"framework", "start_time_ns", "duration_ns",
			"status_code", "status_msg",
			"attributes", "events",
			"input_tokens", "output_tokens", "cache_read_tokens", "cache_write_tokens", "reasoning_tokens",
			"cost_usd", "input_cost_usd", "output_cost_usd", "cache_read_cost_usd", "cache_write_cost_usd", "reasoning_cost_usd",
			"risk_score", "risk_category",
			"tenant_id",
		},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return err
	}
	s.upsertRunsFromSpans(ctx, spans)
	return nil
}

// ─── Trace queries ────────────────────────────────────────────────────────────

type projectedRun struct {
	RunID       string
	TraceID     string
	ParentRunID string
	Framework   string
	AgentName   string
	Model       string
	StartTime   time.Time
	EndTime     time.Time
	Status      string
	TotalTokens int64
	TotalCost   float64
	TenantID    string
}

func (s *PostgresStore) upsertRunsFromSpans(ctx context.Context, spans []models.Span) {
	runs := projectRunsFromSpans(spans)
	if len(runs) == 0 {
		return
	}
	for _, run := range runs {
		metadata := `{"source":"spans_projection"}`
		if _, err := s.pool.Exec(ctx, `
			INSERT INTO runs (
				run_id, trace_id, parent_run_id, framework, agent_name, model,
				start_time, end_time, status, total_tokens, total_cost_usd, metadata, tenant_id
			)
			VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6, $7, $8, $9, $10, $11, $12::jsonb, $13)
			ON CONFLICT (run_id, tenant_id) DO UPDATE SET
				trace_id = EXCLUDED.trace_id,
				parent_run_id = COALESCE(NULLIF(EXCLUDED.parent_run_id, ''), runs.parent_run_id),
				framework = CASE WHEN runs.framework = 'unknown' THEN EXCLUDED.framework ELSE runs.framework END,
				agent_name = CASE WHEN runs.agent_name = 'unknown' THEN EXCLUDED.agent_name ELSE runs.agent_name END,
				model = COALESCE(NULLIF(EXCLUDED.model, ''), runs.model),
				start_time = LEAST(runs.start_time, EXCLUDED.start_time),
				end_time = GREATEST(COALESCE(runs.end_time, EXCLUDED.end_time), EXCLUDED.end_time),
				status = CASE
					WHEN runs.status = 'error' OR EXCLUDED.status = 'error' THEN 'error'
					WHEN GREATEST(COALESCE(runs.end_time, EXCLUDED.end_time), EXCLUDED.end_time) IS NULL THEN 'running'
					ELSE 'success'
				END,
				total_tokens = runs.total_tokens + EXCLUDED.total_tokens,
				total_cost_usd = runs.total_cost_usd + EXCLUDED.total_cost_usd,
				metadata = CASE
					WHEN runs.metadata = '{}'::jsonb THEN EXCLUDED.metadata
					ELSE runs.metadata
				END
		`,
			run.RunID, run.TraceID, run.ParentRunID, run.Framework, run.AgentName, run.Model,
			run.StartTime, run.EndTime, run.Status, run.TotalTokens, run.TotalCost, metadata, run.TenantID,
		); err != nil {
			if s.logger != nil {
				s.logger.Warn("project run from spans failed", zap.Error(err), zap.String("run_id", run.RunID), zap.String("tenant_id", run.TenantID))
			}
		}
	}
}

func projectRunsFromSpans(spans []models.Span) []projectedRun {
	grouped := make(map[string]*projectedRun, len(spans))
	for _, sp := range spans {
		tenantID := strings.TrimSpace(sp.TenantID)
		if tenantID == "" {
			continue
		}
		runID := canonicalRunID(sp)
		if runID == "" {
			continue
		}
		key := tenantID + "|" + runID
		start := time.Unix(0, sp.StartTimeNs).UTC()
		end := time.Unix(0, sp.StartTimeNs+sp.DurationNs).UTC()
		entry, ok := grouped[key]
		if !ok {
			entry = &projectedRun{
				RunID:       runID,
				TraceID:     strings.TrimSpace(sp.TraceID),
				ParentRunID: deriveParentRunID(sp.Attributes),
				Framework:   firstNonBlank(strings.TrimSpace(sp.Framework), "unknown"),
				AgentName:   firstNonBlank(deriveAgentName(sp.Attributes), "unknown"),
				Model:       deriveModelName(sp.Attributes),
				StartTime:   start,
				EndTime:     end,
				Status:      "success",
				TenantID:    tenantID,
			}
			grouped[key] = entry
		}
		if entry.TraceID == "" {
			entry.TraceID = strings.TrimSpace(sp.TraceID)
		}
		if entry.ParentRunID == "" {
			entry.ParentRunID = deriveParentRunID(sp.Attributes)
		}
		if entry.Framework == "unknown" && strings.TrimSpace(sp.Framework) != "" {
			entry.Framework = strings.TrimSpace(sp.Framework)
		}
		if entry.AgentName == "unknown" {
			if name := deriveAgentName(sp.Attributes); name != "" {
				entry.AgentName = name
			}
		}
		if entry.Model == "" {
			entry.Model = deriveModelName(sp.Attributes)
		}
		if start.Before(entry.StartTime) {
			entry.StartTime = start
		}
		if end.After(entry.EndTime) {
			entry.EndTime = end
		}
		if spanHasFailure(sp) {
			entry.Status = "error"
		}
		entry.TotalTokens += sp.InputTokens + sp.OutputTokens + sp.CacheReadTokens + sp.CacheWriteTokens + sp.ReasoningTokens
		entry.TotalCost += sp.CostUSD
	}

	out := make([]projectedRun, 0, len(grouped))
	for _, run := range grouped {
		if run.TraceID == "" {
			run.TraceID = run.RunID
		}
		if run.Framework == "" {
			run.Framework = "unknown"
		}
		if run.AgentName == "" {
			run.AgentName = "unknown"
		}
		out = append(out, *run)
	}
	return out
}

func canonicalRunID(sp models.Span) string {
	if runID := strings.TrimSpace(sp.RunID); runID != "" {
		return runID
	}
	if traceID := strings.TrimSpace(sp.TraceID); traceID != "" {
		return traceID
	}
	return strings.TrimSpace(sp.ID)
}

func deriveParentRunID(attrs map[string]string) string {
	if attrs == nil {
		return ""
	}
	return firstNonBlank(
		strings.TrimSpace(attrs["af.parent_run_id"]),
		strings.TrimSpace(attrs["parent.run_id"]),
		strings.TrimSpace(attrs["run.parent_id"]),
	)
}

func deriveAgentName(attrs map[string]string) string {
	if attrs == nil {
		return ""
	}
	return firstNonBlank(
		strings.TrimSpace(attrs["af.agent.name"]),
		strings.TrimSpace(attrs["agent.name"]),
		strings.TrimSpace(attrs["service.name"]),
		strings.TrimSpace(attrs["application.name"]),
	)
}

func deriveModelName(attrs map[string]string) string {
	if attrs == nil {
		return ""
	}
	return firstNonBlank(
		strings.TrimSpace(attrs["gen_ai.request.model"]),
		strings.TrimSpace(attrs["llm.model"]),
	)
}

func spanHasFailure(sp models.Span) bool {
	outcome := strings.ToLower(strings.TrimSpace(sp.Attributes["af.outcome_status"]))
	switch outcome {
	case "error", "blocked", "degraded", "partial":
		return true
	}
	if strings.EqualFold(strings.TrimSpace(sp.Attributes["af.policy.blocked"]), "true") {
		return true
	}
	decision := strings.ToLower(strings.TrimSpace(sp.Attributes["af.policy.decision"]))
	if decision == "deny" || decision == "block" {
		return true
	}
	if sp.StatusCode == 2 || sp.StatusCode >= 500 {
		return true
	}
	if sp.StatusCode == 401 || sp.StatusCode == 403 || sp.StatusCode == 429 {
		return true
	}
	return false
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (s *PostgresStore) ListTraces(ctx context.Context, q models.TraceQuery) (*models.Page[models.Trace], error) {
	if q.Limit <= 0 || q.Limit > 200 {
		q.Limit = 50
	}

	args := []interface{}{q.TenantID}
	argIdx := 2
	innerWhere := "WHERE tenant_id = $1"

	if q.Framework != "" {
		innerWhere += fmt.Sprintf(" AND framework = $%d", argIdx)
		args = append(args, q.Framework)
		argIdx++
	}
	if q.Model != "" {
		innerWhere += fmt.Sprintf(" AND COALESCE(attributes->>'gen_ai.request.model', '') ILIKE $%d", argIdx)
		args = append(args, "%"+q.Model+"%")
		argIdx++
	}
	if q.Provider != "" {
		innerWhere += fmt.Sprintf(" AND COALESCE(attributes->>'gen_ai.system', '') = $%d", argIdx)
		args = append(args, strings.ToLower(strings.TrimSpace(q.Provider)))
		argIdx++
	}
	if q.AgentName != "" {
		innerWhere += fmt.Sprintf(" AND (COALESCE(attributes->>'agent.name', '') ILIKE $%d OR COALESCE(attributes->>'service.name', '') ILIKE $%d)", argIdx, argIdx)
		args = append(args, "%"+q.AgentName+"%")
		argIdx++
	}
	if q.AppName != "" {
		innerWhere += fmt.Sprintf(" AND COALESCE(NULLIF(attributes->>'service.name', ''), NULLIF(attributes->>'af.app.name', ''), '') ILIKE $%d", argIdx)
		args = append(args, "%"+q.AppName+"%")
		argIdx++
	}
	if q.Environment != "" {
		innerWhere += fmt.Sprintf(" AND COALESCE(NULLIF(attributes->>'deployment.environment', ''), NULLIF(attributes->>'environment', ''), '') ILIKE $%d", argIdx)
		args = append(args, "%"+q.Environment+"%")
		argIdx++
	}
	if q.UserID != "" {
		innerWhere += fmt.Sprintf(" AND COALESCE(NULLIF(attributes->>'enduser.id', ''), NULLIF(attributes->>'user.id', ''), '') ILIKE $%d", argIdx)
		args = append(args, "%"+q.UserID+"%")
		argIdx++
	}
	if q.SessionID != "" {
		innerWhere += fmt.Sprintf(" AND COALESCE(NULLIF(attributes->>'session.id', ''), NULLIF(attributes->>'af.session.id', ''), '') ILIKE $%d", argIdx)
		args = append(args, "%"+q.SessionID+"%")
		argIdx++
	}
	if q.BlockedOnly {
		innerWhere += " AND " + spanOutcomeBlockedExpr()
	}
	if q.Search != "" {
		needle := "%" + strings.ToLower(strings.TrimSpace(q.Search)) + "%"
		innerWhere += fmt.Sprintf(` AND (
			LOWER(trace_id) LIKE $%d OR
			LOWER(name) LIKE $%d OR
			LOWER(COALESCE(attributes->>'gen_ai.request.model', '')) LIKE $%d OR
			LOWER(COALESCE(attributes->>'gen_ai.system', '')) LIKE $%d OR
			LOWER(COALESCE(attributes->>'service.name', '')) LIKE $%d OR
			LOWER(COALESCE(attributes->>'af.app.name', '')) LIKE $%d OR
			LOWER(COALESCE(attributes->>'deployment.environment', '')) LIKE $%d OR
			LOWER(COALESCE(attributes->>'environment', '')) LIKE $%d OR
			LOWER(COALESCE(attributes->>'enduser.id', '')) LIKE $%d OR
			LOWER(COALESCE(attributes->>'session.id', '')) LIKE $%d
		)`, argIdx, argIdx, argIdx, argIdx, argIdx, argIdx, argIdx, argIdx, argIdx, argIdx)
		args = append(args, needle)
		argIdx++
	}
	if q.StartTime > 0 {
		innerWhere += fmt.Sprintf(" AND start_time_ns >= $%d", argIdx)
		args = append(args, q.StartTime)
		argIdx++
	}
	if q.EndTime > 0 {
		innerWhere += fmt.Sprintf(" AND start_time_ns <= $%d", argIdx)
		args = append(args, q.EndTime)
		argIdx++
	}

	countArgs := append([]interface{}{}, args...)

	outerWhere := ""
	if q.Cursor != "" {
		if cursorNs, cursorTraceID, ok := models.DecodeTraceCursor(q.Cursor); ok {
			outerWhere = fmt.Sprintf(
				" WHERE (start_ns, trace_id) < ($%d, $%d)", argIdx, argIdx+1,
			)
			args = append(args, cursorNs, cursorTraceID)
			argIdx += 2
		}
	}

	statusOuterWhere := ""
	countStatusWhere := ""
	if q.Status != "" {
		statusValue := strings.ToLower(strings.TrimSpace(q.Status))
		statusOuterWhere = fmt.Sprintf("status = $%d", argIdx)
		countStatusWhere = fmt.Sprintf("status = $%d", len(countArgs)+1)
		args = append(args, statusValue)
		countArgs = append(countArgs, statusValue)
		argIdx++
	}
	countWhere := ""
	if countStatusWhere != "" {
		countWhere = " WHERE " + countStatusWhere
	}
	switch {
	case outerWhere != "" && statusOuterWhere != "":
		outerWhere += " AND " + statusOuterWhere
	case statusOuterWhere != "":
		outerWhere = " WHERE " + statusOuterWhere
	}

	countQuery := fmt.Sprintf(`
		WITH agg AS (
			SELECT
				trace_id,
				MIN(name) AS root_span_name,
				MAX(framework) AS framework,
				MIN(start_time_ns) AS start_ns,
				MAX(start_time_ns + duration_ns) - MIN(start_time_ns) AS duration_ns,
				COUNT(*) AS span_count,
				SUM(CASE WHEN %s THEN 1 ELSE 0 END) AS error_count,
				SUM(cost_usd) AS total_cost,
				SUM(input_tokens + output_tokens + cache_read_tokens + cache_write_tokens + reasoning_tokens) AS total_tokens,
				CASE
					WHEN SUM(CASE WHEN %s = 'error' THEN 1 ELSE 0 END) > 0 THEN 'error'
					WHEN SUM(CASE WHEN %s IN ('blocked', 'degraded') THEN 1 ELSE 0 END) > 0 THEN 'partial'
					ELSE 'ok'
				END AS status
			FROM spans
			%s
			GROUP BY trace_id
		)
		SELECT COUNT(*) FROM agg%s
	`, spanOutcomeFailureExpr(), spanOutcomeExpr, spanOutcomeExpr, innerWhere, countWhere)

	var total int64
	if err := s.pool.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, err
	}

	args = append(args, q.Limit+1)
	limitIdx := argIdx

	query := fmt.Sprintf(`
		WITH agg AS (
			SELECT
				trace_id,
				MIN(name)                                                            AS root_span_name,
				MAX(framework)                                                       AS framework,
				MIN(start_time_ns)                                                   AS start_ns,
				MAX(start_time_ns + duration_ns) - MIN(start_time_ns)               AS duration_ns,
				COUNT(*)                                                             AS span_count,
				SUM(CASE WHEN %s THEN 1 ELSE 0 END)                               AS error_count,
				SUM(cost_usd)                                                        AS total_cost,
				SUM(input_tokens + output_tokens + cache_read_tokens + cache_write_tokens + reasoning_tokens) AS total_tokens,
				CASE
					WHEN SUM(CASE WHEN %s = 'error' THEN 1 ELSE 0 END) > 0 THEN 'error'
					WHEN SUM(CASE WHEN %s IN ('blocked', 'degraded') THEN 1 ELSE 0 END) > 0 THEN 'partial'
					ELSE 'ok'
				END                                                                  AS status
			FROM spans
			%s
			GROUP BY trace_id
		)
		SELECT trace_id, root_span_name, framework, start_ns, duration_ns,
		       span_count, error_count, total_cost, total_tokens, status
		FROM agg
		%s
		ORDER BY start_ns DESC, trace_id DESC
		LIMIT $%d`,
		spanOutcomeFailureExpr(), spanOutcomeExpr, spanOutcomeExpr, innerWhere, outerWhere, limitIdx,
	)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var traces []models.Trace
	for rows.Next() {
		var t models.Trace
		var startNs int64
		if err := rows.Scan(
			&t.ID, &t.RootSpanName, &t.Framework,
			&startNs, &t.Duration, &t.SpanCount, &t.ErrorCount,
			&t.TotalCostUSD, &t.TotalTokens, &t.Status,
		); err != nil {
			continue
		}
		t.StartTime = time.Unix(0, startNs)
		traces = append(traces, t)
	}

	hasMore := len(traces) > q.Limit
	if hasMore {
		traces = traces[:q.Limit]
	}

	page := &models.Page[models.Trace]{Items: traces, Total: total, HasMore: hasMore}
	if hasMore && len(traces) > 0 {
		last := traces[len(traces)-1]
		page.NextCursor = models.EncodeTraceCursor(last.StartTime.UnixNano(), last.ID)
	}
	return page, nil
}

func (s *PostgresStore) GetTraceSpans(ctx context.Context, traceID, tenantID string) ([]models.Span, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT span_id, trace_id, COALESCE(parent_span_id,''), run_id, name, framework,
		       start_time_ns, duration_ns, status_code, COALESCE(status_msg,''),
		       attributes, events, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, reasoning_tokens,
		       cost_usd, input_cost_usd, output_cost_usd, cache_read_cost_usd, cache_write_cost_usd, reasoning_cost_usd, received_at
		FROM spans
		WHERE trace_id = $1 AND tenant_id = $2
		ORDER BY start_time_ns ASC
		LIMIT 5000`,
		traceID, tenantID,
	)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var spans []models.Span
	for rows.Next() {
		var sp models.Span
		var attrsJSON, eventsJSON []byte
		if err := rows.Scan(
			&sp.ID, &sp.TraceID, &sp.ParentID, &sp.RunID, &sp.Name, &sp.Framework,
			&sp.StartTimeNs, &sp.DurationNs, &sp.StatusCode, &sp.StatusMsg,
			&attrsJSON, &eventsJSON, &sp.InputTokens, &sp.OutputTokens, &sp.CacheReadTokens, &sp.CacheWriteTokens, &sp.ReasoningTokens,
			&sp.CostUSD, &sp.InputCostUSD, &sp.OutputCostUSD, &sp.CacheReadCostUSD, &sp.CacheWriteCostUSD, &sp.ReasoningCostUSD,
			&sp.ReceivedAt,
		); err != nil {
			continue
		}
		json.Unmarshal(attrsJSON, &sp.Attributes)
		json.Unmarshal(eventsJSON, &sp.Events)
		if sp.Attributes == nil {
			sp.Attributes = map[string]string{}
		}
		if sp.Events == nil {
			sp.Events = []models.SpanEvent{}
		}
		spans = append(spans, sp)
	}
	return spans, nil
}

// GetSpansForTraces fetches spans for multiple trace IDs in a single query (P0-2 fix).
// Use this instead of calling GetTraceSpans in a loop.
func (s *PostgresStore) GetSpansForTraces(ctx context.Context, traceIDs []string, tenantID string) ([]models.Span, error) {
	if len(traceIDs) == 0 {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT span_id, trace_id, COALESCE(parent_span_id,''), run_id, name, framework,
		       start_time_ns, duration_ns, status_code, COALESCE(status_msg,''),
		       attributes, events, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, reasoning_tokens,
		       cost_usd, input_cost_usd, output_cost_usd, cache_read_cost_usd, cache_write_cost_usd, reasoning_cost_usd, received_at
		FROM spans
		WHERE trace_id = ANY($1) AND tenant_id = $2
		ORDER BY start_time_ns ASC
		LIMIT 50000`,
		traceIDs, tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var spans []models.Span
	for rows.Next() {
		var sp models.Span
		var attrsJSON, eventsJSON []byte
		if err := rows.Scan(
			&sp.ID, &sp.TraceID, &sp.ParentID, &sp.RunID, &sp.Name, &sp.Framework,
			&sp.StartTimeNs, &sp.DurationNs, &sp.StatusCode, &sp.StatusMsg,
			&attrsJSON, &eventsJSON, &sp.InputTokens, &sp.OutputTokens, &sp.CacheReadTokens, &sp.CacheWriteTokens, &sp.ReasoningTokens,
			&sp.CostUSD, &sp.InputCostUSD, &sp.OutputCostUSD, &sp.CacheReadCostUSD, &sp.CacheWriteCostUSD, &sp.ReasoningCostUSD,
			&sp.ReceivedAt,
		); err != nil {
			continue
		}
		json.Unmarshal(attrsJSON, &sp.Attributes)
		json.Unmarshal(eventsJSON, &sp.Events)
		if sp.Attributes == nil {
			sp.Attributes = map[string]string{}
		}
		if sp.Events == nil {
			sp.Events = []models.SpanEvent{}
		}
		spans = append(spans, sp)
	}
	return spans, nil
}

func (s *PostgresStore) GetOverview(ctx context.Context, tenantID string, since time.Duration) (*models.OverviewStats, error) {
	cutoff := time.Now().Add(-since).UnixNano()
	cutoffTime := time.Unix(0, cutoff).UTC()
	var stats models.OverviewStats

	err := s.pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT
			COUNT(DISTINCT trace_id),
			COALESCE(SUM(cost_usd), 0),
			COALESCE(SUM(input_tokens + output_tokens + cache_read_tokens + cache_write_tokens + reasoning_tokens), 0),
			COALESCE(AVG(CASE WHEN %s THEN 1.0 ELSE 0.0 END), 0),
			COALESCE(AVG(duration_ns) / 1e6, 0),
			COALESCE(SUM(CASE WHEN %s THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN COALESCE(attributes->>'af.span.step_type', '') = 'llm' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN COALESCE(attributes->>'af.span.step_type', '') = 'tool' THEN 1 ELSE 0 END), 0)
		FROM spans
		WHERE tenant_id = $1 AND start_time_ns >= $2`,
		spanOutcomeFailureExpr(), spanOutcomeBlockedExpr(),
	), tenantID, cutoff,
	).Scan(
		&stats.TotalTraces, &stats.TotalCostUSD, &stats.TotalTokens,
		&stats.ErrorRate, &stats.AvgLatencyMs, &stats.BlockedRequests, &stats.LLMCalls, &stats.ToolCalls,
	)
	if err != nil {
		return nil, err
	}

	err = s.pool.QueryRow(ctx, `
		SELECT COALESCE(COUNT(DISTINCT agent_name), 0)
		FROM runs
		WHERE tenant_id = $1 AND start_time >= $2
	`, tenantID, cutoffTime).Scan(&stats.ActiveAgents)
	if err != nil {
		return nil, err
	}

	// Framework counts
	rows, _ := s.pool.Query(ctx, `
		SELECT framework, COUNT(DISTINCT trace_id) FROM spans
		WHERE tenant_id = $1 AND start_time_ns >= $2
		GROUP BY framework`, tenantID, cutoff)
	defer rows.Close()

	stats.FrameworkCounts = map[string]int64{}
	for rows.Next() {
		var fw string
		var cnt int64
		rows.Scan(&fw, &cnt)
		stats.FrameworkCounts[fw] = cnt
	}

	return &stats, nil
}

// ─── Runs ─────────────────────────────────────────────────────────────────────

func (s *PostgresStore) ListRuns(ctx context.Context, q models.RunQuery) (*models.Page[models.Run], error) {
	if q.Limit <= 0 || q.Limit > 200 {
		q.Limit = 50
	}
	query := `
		SELECT run_id, trace_id, COALESCE(parent_run_id,''), framework, agent_name, model,
		       start_time, end_time, status, total_tokens, total_cost_usd
		FROM runs WHERE tenant_id = $1`
	args := []interface{}{q.TenantID}
	idx := 2
	if q.AgentName != "" {
		query += fmt.Sprintf(" AND agent_name = $%d", idx)
		args = append(args, q.AgentName)
		idx++
	}
	if q.TraceID != "" {
		query += fmt.Sprintf(" AND trace_id = $%d", idx)
		args = append(args, q.TraceID)
		idx++
	}
	if q.Framework != "" {
		query += fmt.Sprintf(" AND framework = $%d", idx)
		args = append(args, q.Framework)
		idx++
	}
	// Keyset cursor: (start_time, run_id) < (cursor_time, cursor_run_id)
	// ensures stable, gap-free pagination even under concurrent writes.
	if q.Cursor != "" {
		if cursorTime, cursorRunID, ok := models.DecodeRunCursor(q.Cursor); ok {
			query += fmt.Sprintf(" AND (start_time, run_id) < ($%d, $%d)", idx, idx+1)
			args = append(args, cursorTime, cursorRunID)
			idx += 2
		}
	}
	// Fetch limit+1 to detect whether a next page exists.
	query += fmt.Sprintf(" ORDER BY start_time DESC, run_id DESC LIMIT $%d", idx)
	args = append(args, q.Limit+1)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []models.Run
	for rows.Next() {
		var r models.Run
		if err := rows.Scan(
			&r.ID, &r.TraceID, &r.ParentRunID, &r.Framework, &r.AgentName, &r.Model,
			&r.StartTime, &r.EndTime, &r.Status, &r.TotalTokens, &r.TotalCostUSD,
		); err != nil {
			continue
		}
		runs = append(runs, r)
	}
	if runs == nil {
		runs = []models.Run{}
	}

	hasMore := len(runs) > q.Limit
	if hasMore {
		runs = runs[:q.Limit]
	}

	page := &models.Page[models.Run]{Items: runs, HasMore: hasMore}
	if hasMore && len(runs) > 0 {
		last := runs[len(runs)-1]
		page.NextCursor = models.EncodeRunCursor(last.StartTime, last.ID)
	}
	return page, nil
}

func (s *PostgresStore) GetRun(ctx context.Context, runID, tenantID string) (*models.Run, error) {
	var r models.Run
	var metaJSON []byte
	err := s.pool.QueryRow(ctx, `
		SELECT run_id, trace_id, COALESCE(parent_run_id,''), framework, agent_name, model,
		       start_time, end_time, status, total_tokens, total_cost_usd, metadata
		FROM runs WHERE run_id = $1 AND tenant_id = $2`,
		runID, tenantID,
	).Scan(
		&r.ID, &r.TraceID, &r.ParentRunID, &r.Framework, &r.AgentName, &r.Model,
		&r.StartTime, &r.EndTime, &r.Status, &r.TotalTokens, &r.TotalCostUSD, &metaJSON,
	)
	if err != nil {
		return nil, err
	}
	json.Unmarshal(metaJSON, &r.Metadata)
	r.TenantID = tenantID
	return &r, nil
}

func (s *PostgresStore) GetRunChildren(ctx context.Context, runID, tenantID string) ([]models.Run, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT run_id, trace_id, COALESCE(parent_run_id,''), framework, agent_name, model,
		       start_time, end_time, status, total_tokens, total_cost_usd
		FROM runs WHERE parent_run_id = $1 AND tenant_id = $2
		ORDER BY start_time ASC LIMIT 100`,
		runID, tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []models.Run
	for rows.Next() {
		var r models.Run
		if err := rows.Scan(
			&r.ID, &r.TraceID, &r.ParentRunID, &r.Framework, &r.AgentName, &r.Model,
			&r.StartTime, &r.EndTime, &r.Status, &r.TotalTokens, &r.TotalCostUSD,
		); err != nil {
			continue
		}
		runs = append(runs, r)
	}
	if runs == nil {
		runs = []models.Run{}
	}
	return runs, nil
}

func (s *PostgresStore) InsertFeedback(ctx context.Context, runID, tenantID string, score *int16, comment string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO feedback (run_id, score, comment, tenant_id) VALUES ($1, $2, $3, $4)`,
		runID, score, comment, tenantID,
	)
	return err
}

// ─── Agents ───────────────────────────────────────────────────────────────────

func (s *PostgresStore) ListAgents(ctx context.Context, tenantID string, limit int) ([]models.Agent, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT
		    agent_name,
		    MAX(framework)                                                   AS framework,
		    MIN(start_time)                                                  AS first_seen,
		    MAX(COALESCE(end_time, NOW()))                                   AS last_seen,
		    COUNT(*)                                                         AS run_count,
		    SUM(total_cost_usd)                                              AS total_cost,
		    AVG(CASE WHEN status = 'error' THEN 1.0 ELSE 0.0 END)           AS error_rate
		FROM runs
		WHERE tenant_id = $1
		GROUP BY agent_name
		ORDER BY last_seen DESC
		LIMIT $2`,
		tenantID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []models.Agent
	for rows.Next() {
		var a models.Agent
		if err := rows.Scan(
			&a.Name, &a.Framework, &a.FirstSeen, &a.LastSeen,
			&a.RunCount, &a.TotalCost, &a.ErrorRate,
		); err != nil {
			continue
		}
		a.ID = a.Name
		agents = append(agents, a)
	}
	if agents == nil {
		agents = []models.Agent{}
	}
	return agents, nil
}

// GetAgentByName returns a single agent aggregated from the runs table.
// O(1) indexed lookup — avoids the O(n) full-scan in ListAgents.
// Returns pgx.ErrNoRows when no runs exist for the given agent name.
func (s *PostgresStore) GetAgentByName(ctx context.Context, tenantID, agentName string) (models.Agent, error) {
	var a models.Agent
	err := s.pool.QueryRow(ctx, `
		SELECT
		    agent_name,
		    MAX(framework)                                             AS framework,
		    MIN(start_time)                                           AS first_seen,
		    MAX(COALESCE(end_time, NOW()))                            AS last_seen,
		    COUNT(*)                                                  AS run_count,
		    SUM(total_cost_usd)                                       AS total_cost,
		    AVG(CASE WHEN status = 'error' THEN 1.0 ELSE 0.0 END)    AS error_rate
		FROM runs
		WHERE tenant_id = $1
		  AND agent_name = $2
		GROUP BY agent_name`,
		tenantID, agentName,
	).Scan(&a.Name, &a.Framework, &a.FirstSeen, &a.LastSeen,
		&a.RunCount, &a.TotalCost, &a.ErrorRate)
	if err != nil {
		return models.Agent{}, err
	}
	a.ID = a.Name
	return a, nil
}

// ─── Reports ──────────────────────────────────────────────────────────────────

type ErrorReportRow struct {
	Framework      string `json:"framework"`
	StatusMsg      string `json:"status_msg"`
	Count          int64  `json:"count"`
	AffectedTraces int64  `json:"affected_traces"`
}

func (s *PostgresStore) GetErrorReport(ctx context.Context, tenantID string, since time.Duration) ([]ErrorReportRow, error) {
	cutoff := time.Now().Add(-since).UnixNano()
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		SELECT
		    framework,
		    COALESCE(NULLIF(status_msg, ''), 'unknown error') AS status_msg,
		    COUNT(*)                                           AS count,
		    COUNT(DISTINCT trace_id)                          AS affected_traces
		FROM spans
		WHERE tenant_id = $1 AND start_time_ns >= $2 AND %s
		GROUP BY framework, status_msg
		ORDER BY count DESC
		LIMIT 50`,
		spanOutcomeFailureExpr(),
	), tenantID, cutoff,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ErrorReportRow
	for rows.Next() {
		var r ErrorReportRow
		if err := rows.Scan(&r.Framework, &r.StatusMsg, &r.Count, &r.AffectedTraces); err != nil {
			continue
		}
		result = append(result, r)
	}
	if result == nil {
		result = []ErrorReportRow{}
	}
	return result, nil
}

// ─── Environments ─────────────────────────────────────────────────────────────

type Environment struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

func (s *PostgresStore) ListEnvironments(ctx context.Context, tenantID string) ([]Environment, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT
			env_id::text AS id,
			name,
			TRIM(BOTH ' ' FROM CONCAT_WS(' ',
				NULLIF(cluster, ''),
				NULLIF(cloud, ''),
				NULLIF(region, '')
			)) AS description,
			'active' AS status
		FROM environments
		WHERE tenant_id::text = $1
		ORDER BY name`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var envs []Environment
	for rows.Next() {
		var e Environment
		if err := rows.Scan(&e.ID, &e.Name, &e.Description, &e.Status); err != nil {
			continue
		}
		envs = append(envs, e)
	}
	if envs == nil {
		envs = []Environment{}
	}
	return envs, nil
}

// ─── Audit Log ────────────────────────────────────────────────────────────────

// AuditEntry is the api-gateway view of a policy_audit_log row.
type AuditEntry struct {
	ID           int64     `json:"id"`
	DecisionID   string    `json:"decision_id"`
	TraceID      string    `json:"trace_id"`
	SpanID       string    `json:"span_id"`
	PolicyName   string    `json:"policy_name"`
	Result       string    `json:"result"`
	Reason       string    `json:"reason"`
	TenantID     string    `json:"tenant_id"`
	EvaluatedAt  time.Time `json:"evaluated_at"`
	PreviousHash string    `json:"previous_hash"`
	EntryHash    string    `json:"entry_hash"`
}

// ChainVerification is the result of replaying the hash chain.
type ChainVerification struct {
	Valid          bool   `json:"valid"`
	EntriesChecked int    `json:"entries_checked"`
	FirstBrokenAt  *int   `json:"first_broken_at,omitempty"`
	Message        string `json:"message"`
}

func (s *PostgresStore) CreatePolicyAuditEntry(ctx context.Context, entry models.PolicyDecisionAudit) error {
	evaluatedAt := entry.EvaluatedAt.UTC()
	if evaluatedAt.IsZero() {
		evaluatedAt = time.Now().UTC()
	}
	evaluatedNs := evaluatedAt.UnixNano()

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	previousHash := "genesis"
	_ = tx.QueryRow(ctx, `
		SELECT entry_hash
		FROM policy_audit_log
		WHERE tenant_id = $1
		ORDER BY id DESC
		LIMIT 1
	`, entry.TenantID).Scan(&previousHash)

	payload := fmt.Sprintf("%s:%s:%s:%s:%d:%s",
		entry.DecisionID, entry.TraceID, entry.PolicyName, normalizeAuditResult(entry.Result), evaluatedNs, previousHash)
	sum := sha256.Sum256([]byte(payload))
	entryHash := fmt.Sprintf("%x", sum)

	_, err = tx.Exec(ctx, `
		INSERT INTO policy_audit_log (
			decision_id, tenant_id, trace_id, span_id, policy_name, result, reason,
			evaluated_at, evaluated_ns, previous_hash, entry_hash, framework, model, cloud_region, environment
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`, entry.DecisionID, entry.TenantID, entry.TraceID, entry.SpanID, entry.PolicyName, normalizeAuditResult(entry.Result), entry.Reason,
		evaluatedAt, evaluatedNs, previousHash, entryHash, entry.Framework, entry.Model, entry.CloudRegion, entry.Environment)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func normalizeAuditResult(result string) string {
	switch strings.ToLower(strings.TrimSpace(result)) {
	case "redact":
		return "sanitize"
	case "block":
		return "deny"
	default:
		return strings.ToLower(strings.TrimSpace(result))
	}
}

func (s *PostgresStore) CreateAdminAuditEntry(ctx context.Context, entry models.AdminAuditEntry) error {
	detailsJSON := "{}"
	if strings.TrimSpace(entry.Details) != "" {
		detailsJSON = entry.Details
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO control_plane_audit (
			tenant_id, actor, category, action, target_type, target_id, outcome, details
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb)
	`, entry.TenantID, entry.Actor, entry.Category, entry.Action, entry.TargetType, entry.TargetID, entry.Outcome, detailsJSON)
	return err
}

func (s *PostgresStore) ListAdminAuditEntries(ctx context.Context, tenantID string, limit int) ([]models.AdminAuditEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, actor, category, action, target_type, target_id, outcome, details::text, created_at
		FROM control_plane_audit
		WHERE tenant_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2
	`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []models.AdminAuditEntry
	for rows.Next() {
		var entry models.AdminAuditEntry
		if err := rows.Scan(
			&entry.ID, &entry.TenantID, &entry.Actor, &entry.Category, &entry.Action,
			&entry.TargetType, &entry.TargetID, &entry.Outcome, &entry.Details, &entry.CreatedAt,
		); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if entries == nil {
		entries = []models.AdminAuditEntry{}
	}
	return entries, rows.Err()
}

// ListAuditEntries returns paginated audit log entries for a tenant, oldest first.
func (s *PostgresStore) ListAuditEntries(ctx context.Context, tenantID string, limit, offset int) ([]AuditEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, decision_id, trace_id, span_id,
		       policy_name, result, reason, tenant_id, evaluated_at,
		       COALESCE(previous_hash,''), COALESCE(entry_hash,'')
		FROM policy_audit_log
		WHERE tenant_id = $1
		ORDER BY id ASC
		LIMIT $2 OFFSET $3`,
		tenantID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []AuditEntry
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(
			&e.ID, &e.DecisionID, &e.TraceID, &e.SpanID,
			&e.PolicyName, &e.Result, &e.Reason, &e.TenantID, &e.EvaluatedAt,
			&e.PreviousHash, &e.EntryHash,
		); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	if entries == nil {
		entries = []AuditEntry{}
	}
	return entries, nil
}

func (s *PostgresStore) ListAuditEntriesForTrace(ctx context.Context, tenantID, traceID string) ([]AuditEntry, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, decision_id, trace_id, span_id,
		       policy_name, result, reason, tenant_id, evaluated_at,
		       COALESCE(previous_hash,''), COALESCE(entry_hash,'')
		FROM policy_audit_log
		WHERE tenant_id = $1 AND trace_id = $2
		ORDER BY id ASC
		LIMIT 500
	`, tenantID, traceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []AuditEntry
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(
			&e.ID, &e.DecisionID, &e.TraceID, &e.SpanID,
			&e.PolicyName, &e.Result, &e.Reason, &e.TenantID, &e.EvaluatedAt,
			&e.PreviousHash, &e.EntryHash,
		); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	if entries == nil {
		entries = []AuditEntry{}
	}
	return entries, rows.Err()
}

// VerifyAuditChain replays the SHA-256 hash chain for a tenant and reports
// the first broken link, if any. This mirrors the current gateway audit
// verification logic.
func (s *PostgresStore) VerifyAuditChain(ctx context.Context, tenantID string) (*ChainVerification, error) {
	// Load all entries in insertion order — limit to 100k for safety
	rows, err := s.pool.Query(ctx, `
		SELECT decision_id, trace_id, policy_name, result,
		       EXTRACT(EPOCH FROM evaluated_at)::BIGINT * 1000000000 AS evaluated_ns,
		       previous_hash, entry_hash
		FROM policy_audit_log
		WHERE tenant_id = $1
		ORDER BY id ASC
		LIMIT 100000`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type chainRow struct {
		decisionID   string
		traceID      string
		policyName   string
		result       string
		evaluatedNs  int64
		previousHash string
		entryHash    string
	}

	var chain []chainRow
	for rows.Next() {
		var r chainRow
		if err := rows.Scan(
			&r.decisionID, &r.traceID, &r.policyName, &r.result,
			&r.evaluatedNs, &r.previousHash, &r.entryHash,
		); err != nil {
			continue
		}
		chain = append(chain, r)
	}

	if len(chain) == 0 {
		return &ChainVerification{Valid: true, EntriesChecked: 0, Message: "no audit entries"}, nil
	}

	prevHash := "genesis"
	for i, r := range chain {
		// Keep the payload format stable so verification matches historical writes.
		payload := fmt.Sprintf("%s:%s:%s:%s:%d:%s",
			r.decisionID, r.traceID, r.policyName, r.result, r.evaluatedNs, prevHash)

		h := sha256.Sum256([]byte(payload))
		expected := fmt.Sprintf("%x", h)

		if r.entryHash != "" && expected != r.entryHash {
			idx := i
			return &ChainVerification{
				Valid:          false,
				EntriesChecked: i + 1,
				FirstBrokenAt:  &idx,
				Message:        fmt.Sprintf("chain broken at entry %d (decision_id=%s)", i, r.decisionID),
			}, nil
		}
		prevHash = r.entryHash
	}

	return &ChainVerification{
		Valid:          true,
		EntriesChecked: len(chain),
		Message:        "chain intact",
	}, nil
}

// Ping verifies the Postgres connection is alive with a short timeout.
// Used by the /healthz handler to detect degraded storage.
func (s *PostgresStore) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func (s *PostgresStore) Close() {
	s.pool.Close()
}

// Pool exposes the underlying pgxpool.Pool for packages (e.g. vault) that need
// direct pool access without going through the store abstraction.
func (s *PostgresStore) Pool() *pgxpool.Pool {
	return s.pool
}

// ─── Users CRUD ───────────────────────────────────────────────────────────────

// ListUsers returns all users for a tenant, newest first.
func (s *PostgresStore) ListUsers(ctx context.Context, tenantID string, limit, offset int) ([]models.User, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT user_id, tenant_id, username, email, display_name, role,
		       is_active, last_login_at, created_at, updated_at
		FROM users
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, tenantID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(
			&u.ID, &u.TenantID, &u.Username, &u.Email, &u.DisplayName, &u.Role,
			&u.IsActive, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt,
		); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	if users == nil {
		users = []models.User{}
	}
	return users, rows.Err()
}

// GetUser returns a single user by ID within a tenant.
func (s *PostgresStore) GetUser(ctx context.Context, userID, tenantID string) (*models.User, error) {
	var u models.User
	err := s.pool.QueryRow(ctx, `
		SELECT user_id, tenant_id, username, email, display_name, role,
		       is_active, last_login_at, created_at, updated_at
		FROM users
		WHERE user_id = $1 AND tenant_id = $2
	`, userID, tenantID).Scan(
		&u.ID, &u.TenantID, &u.Username, &u.Email, &u.DisplayName, &u.Role,
		&u.IsActive, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// CreateUser inserts a new user into the tenant. Password is bcrypt-hashed (cost 12)
// before storage. golang.org/x/crypto/bcrypt is declared in go.mod.
func (s *PostgresStore) CreateUser(ctx context.Context, tenantID string, req models.CreateUserRequest) (*models.User, error) {
	if req.Role == "" {
		req.Role = "viewer"
	}
	var u models.User
	err := s.pool.QueryRow(ctx, `
		INSERT INTO users (user_id, tenant_id, username, password_hash, email, display_name, role)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING user_id, tenant_id, username, email, display_name, role,
		          is_active, last_login_at, created_at, updated_at
	`, generateStoreID(), tenantID, req.Username, hashPassword(req.Password),
		req.Email, req.DisplayName, req.Role).Scan(
		&u.ID, &u.TenantID, &u.Username, &u.Email, &u.DisplayName, &u.Role,
		&u.IsActive, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// UpdateUser applies non-nil fields from req to the user record.
func (s *PostgresStore) UpdateUser(ctx context.Context, userID, tenantID string, req models.UpdateUserRequest) (*models.User, error) {
	sets := []string{"updated_at = NOW()"}
	args := []interface{}{userID, tenantID}
	argIdx := 3

	if req.Email != nil {
		sets = append(sets, fmt.Sprintf("email = $%d", argIdx))
		args = append(args, *req.Email)
		argIdx++
	}
	if req.DisplayName != nil {
		sets = append(sets, fmt.Sprintf("display_name = $%d", argIdx))
		args = append(args, *req.DisplayName)
		argIdx++
	}
	if req.Role != nil {
		sets = append(sets, fmt.Sprintf("role = $%d", argIdx))
		args = append(args, *req.Role)
		argIdx++
	}
	if req.IsActive != nil {
		sets = append(sets, fmt.Sprintf("is_active = $%d", argIdx))
		args = append(args, *req.IsActive)
		argIdx++
	}
	if req.Password != nil {
		sets = append(sets, fmt.Sprintf("password_hash = $%d", argIdx))
		args = append(args, hashPassword(*req.Password))
		argIdx++
	}

	query := fmt.Sprintf(`
		UPDATE users SET %s
		WHERE user_id = $1 AND tenant_id = $2
		RETURNING user_id, tenant_id, username, email, display_name, role,
		          is_active, last_login_at, created_at, updated_at
	`, strings.Join(sets, ", "))

	var u models.User
	err := s.pool.QueryRow(ctx, query, args...).Scan(
		&u.ID, &u.TenantID, &u.Username, &u.Email, &u.DisplayName, &u.Role,
		&u.IsActive, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// DeleteUser removes a user from the tenant. Returns an error if the user doesn't exist.
func (s *PostgresStore) DeleteUser(ctx context.Context, userID, tenantID string) error {
	result, err := s.pool.Exec(ctx,
		`DELETE FROM users WHERE user_id = $1 AND tenant_id = $2`,
		userID, tenantID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("user %q not found in tenant %q", userID, tenantID)
	}
	return nil
}

// GetUserByUsername looks up a user by username across active tenants and returns
// the minimal auth fields including the bcrypt password hash.
// Login fails closed if the username is ambiguous across tenants.
func (s *PostgresStore) GetUserByUsername(ctx context.Context, username string) (*models.UserRecord, error) {
	var rec models.UserRecord
	rows, err := s.pool.Query(ctx, `
		SELECT user_id, tenant_id, username, email, display_name, role, COALESCE(password_hash,'')
		FROM users
		WHERE username = $1 AND is_active = TRUE
		ORDER BY created_at DESC
		LIMIT 2
	`, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		if err := rows.Scan(&rec.ID, &rec.TenantID, &rec.Username, &rec.Email, &rec.DisplayName, &rec.Role, &rec.PasswordHash); err != nil {
			return nil, err
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	switch count {
	case 0:
		return nil, pgx.ErrNoRows
	case 1:
		return &rec, nil
	default:
		return nil, fmt.Errorf("ambiguous username %q across tenants", username)
	}
}

// GetUserByEmail looks up an active user by email across tenants.
// OIDC resolution fails closed if multiple tenants share the same email.
func (s *PostgresStore) GetUserByEmail(ctx context.Context, email string) (*models.UserRecord, error) {
	var rec models.UserRecord
	rows, err := s.pool.Query(ctx, `
		SELECT user_id, tenant_id, username, email, display_name, role, COALESCE(password_hash,'')
		FROM users
		WHERE LOWER(email) = LOWER($1) AND is_active = TRUE
		ORDER BY created_at DESC
		LIMIT 2
	`, email)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		if err := rows.Scan(&rec.ID, &rec.TenantID, &rec.Username, &rec.Email, &rec.DisplayName, &rec.Role, &rec.PasswordHash); err != nil {
			return nil, err
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	switch count {
	case 0:
		return nil, pgx.ErrNoRows
	case 1:
		return &rec, nil
	default:
		return nil, fmt.Errorf("ambiguous email %q across tenants", email)
	}
}

// generateStoreID creates a random UUID-format string for new records.
func generateStoreID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// hashPassword hashes a password with bcrypt (cost=12) for storage.
// Falls back to SHA-256 hex on the extremely unlikely event bcrypt fails.
func hashPassword(password string) string {
	if password == "" {
		return ""
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		// Fallback: should never happen; bcrypt only fails on invalid cost or OOM.
		h := sha256.Sum256([]byte(password))
		return hex.EncodeToString(h[:])
	}
	return string(hash)
}

// ─── Budget operations ────────────────────────────────────────────────────────

// GetBudget retrieves the budget for a tenant
func (s *PostgresStore) GetBudget(ctx context.Context, tenantID string) (*budget.Budget, error) {
	var b budget.Budget

	err := s.pool.QueryRow(ctx, `
		SELECT tenant_id, monthly_tokens, monthly_cost_usd, alert_threshold,
		       hard_limit, reset_day, created_at, updated_at
		FROM tenant_budgets
		WHERE tenant_id = $1
	`, tenantID).Scan(
		&b.TenantID, &b.MonthlyTokens, &b.MonthlyCostUSD, &b.AlertThreshold,
		&b.HardLimit, &b.ResetDay, &b.CreatedAt, &b.UpdatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &b, nil
}

// UpsertBudget creates or updates a budget for a tenant
func (s *PostgresStore) UpsertBudget(ctx context.Context, b *budget.Budget) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO tenant_budgets
		(tenant_id, monthly_tokens, monthly_cost_usd, alert_threshold, hard_limit, reset_day)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (tenant_id) DO UPDATE SET
		  monthly_tokens = $2,
		  monthly_cost_usd = $3,
		  alert_threshold = $4,
		  hard_limit = $5,
		  reset_day = $6,
		  updated_at = NOW()
	`, b.TenantID, b.MonthlyTokens, b.MonthlyCostUSD, b.AlertThreshold, b.HardLimit, b.ResetDay,
	)
	return err
}

// GetMonthlyUsage returns current period usage for a tenant
func (s *PostgresStore) GetMonthlyUsage(ctx context.Context, tenantID string, since time.Time) (*budget.UsageSummary, error) {
	var usage budget.UsageSummary

	err := s.pool.QueryRow(ctx, `
		SELECT
		  COALESCE(SUM(input_tokens + output_tokens + cache_read_tokens + cache_write_tokens + reasoning_tokens), 0),
		  COALESCE(SUM(cost_usd), 0),
		  $2::timestamptz,
		  NOW()
		FROM spans
		WHERE tenant_id = $1 AND received_at >= $2::timestamptz
	`, tenantID, since).Scan(
		&usage.TokensUsed, &usage.CostUsedUSD, &usage.PeriodStart, &usage.PeriodEnd,
	)

	if err != nil {
		return nil, err
	}
	return &usage, nil
}

// RecordAlert logs a budget alert
func (s *PostgresStore) RecordAlert(ctx context.Context, tenantID, alertType string, usage budget.UsageSummary, b budget.Budget) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO usage_alerts
		(tenant_id, alert_type, tokens_used, cost_used_usd, budget_tokens, budget_cost_usd)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, tenantID, alertType, usage.TokensUsed, usage.CostUsedUSD, b.MonthlyTokens, b.MonthlyCostUSD,
	)
	return err
}

// ListBudgetAlerts returns recent alerts for a tenant
func (s *PostgresStore) ListBudgetAlerts(ctx context.Context, tenantID string, limit int) ([]map[string]interface{}, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, alert_type, tokens_used, cost_used_usd,
		       budget_tokens, budget_cost_usd, triggered_at
		FROM usage_alerts
		WHERE tenant_id = $1
		ORDER BY triggered_at DESC
		LIMIT $2
	`, tenantID, limit)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []map[string]interface{}
	for rows.Next() {
		var (
			id, triggeredAt interface{}
			alertType       string
			tokensUsed      interface{}
			costUsedUsd     interface{}
			budgetTokens    interface{}
			budgetCostUsd   interface{}
			tid             string
		)
		err := rows.Scan(
			&id, &tid, &alertType, &tokensUsed, &costUsedUsd,
			&budgetTokens, &budgetCostUsd, &triggeredAt,
		)
		if err != nil {
			return nil, err
		}
		alerts = append(alerts, map[string]interface{}{
			"id":              id,
			"alert_type":      alertType,
			"tokens_used":     tokensUsed,
			"cost_used_usd":   costUsedUsd,
			"budget_tokens":   budgetTokens,
			"budget_cost_usd": budgetCostUsd,
			"triggered_at":    triggeredAt,
		})
	}

	return alerts, rows.Err()
}

func (s *PostgresStore) ListPricingRules(ctx context.Context) ([]models.PricingRule, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, provider, model_pattern, input_per_million, output_per_million,
		       cache_read_per_million, cache_write_per_million, reasoning_per_million,
		       active, priority, effective_from, effective_to, description, created_at, updated_at
		FROM pricing_rules
		ORDER BY COALESCE(tenant_id, '') ASC, provider ASC, priority DESC, model_pattern ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []models.PricingRule
	for rows.Next() {
		var rule models.PricingRule
		var tenantID *string
		if err := rows.Scan(
			&rule.ID, &tenantID, &rule.Provider, &rule.ModelPattern, &rule.InputPerMillion,
			&rule.OutputPerMillion, &rule.CacheReadPerMillion, &rule.CacheWritePerMillion, &rule.ReasoningPerMillion, &rule.Active, &rule.Priority, &rule.EffectiveFrom,
			&rule.EffectiveTo, &rule.Description, &rule.CreatedAt, &rule.UpdatedAt,
		); err != nil {
			return nil, err
		}
		rule.TenantID = tenantID
		rules = append(rules, rule)
	}
	if rules == nil {
		rules = []models.PricingRule{}
	}
	return rules, rows.Err()
}

func (s *PostgresStore) ListPolicyRules(ctx context.Context) ([]models.PolicyRule, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, name, rule_type, decision_mode, enabled, priority, action, provider, model_pattern,
		       environment, max_tokens, detector, scope, guardrails, schema_json, unsafe_categories, rollout_percent, version,
		       rule_conditions, rego_module, description, created_at, updated_at
		FROM policy_rules
		ORDER BY priority DESC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []models.PolicyRule
	for rows.Next() {
		var rule models.PolicyRule
		var tenantID *string
		var rawConditions []byte
		var rawGuardrails []byte
		var rawUnsafeCategories []byte
		if err := rows.Scan(
			&rule.ID, &tenantID, &rule.Name, &rule.RuleType, &rule.DecisionMode, &rule.Enabled, &rule.Priority, &rule.Action,
			&rule.Provider, &rule.ModelPattern, &rule.Environment, &rule.MaxTokens, &rule.Detector,
			&rule.Scope, &rawGuardrails, &rule.SchemaJSON, &rawUnsafeCategories, &rule.RolloutPercent, &rule.Version,
			&rawConditions, &rule.RegoModule, &rule.Description, &rule.CreatedAt, &rule.UpdatedAt,
		); err != nil {
			return nil, err
		}
		rule.TenantID = tenantID
		rule.Guardrails = decodePolicyStringSlice(rawGuardrails)
		rule.UnsafeCategories = decodePolicyStringSlice(rawUnsafeCategories)
		rule.RuleConditions = decodePolicyConditions(rawConditions)
		rules = append(rules, rule)
	}
	if rules == nil {
		rules = []models.PolicyRule{}
	}
	return rules, rows.Err()
}

func (s *PostgresStore) GetPolicyRule(ctx context.Context, id int64) (*models.PolicyRule, error) {
	var rule models.PolicyRule
	var rawConditions []byte
	var rawGuardrails []byte
	var rawUnsafeCategories []byte
	err := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, name, rule_type, decision_mode, enabled, priority, action, provider, model_pattern,
		       environment, max_tokens, detector, scope, guardrails, schema_json, unsafe_categories, rollout_percent, version,
		       rule_conditions, rego_module, description, created_at, updated_at
		FROM policy_rules
		WHERE id = $1
	`, id).Scan(
		&rule.ID, &rule.TenantID, &rule.Name, &rule.RuleType, &rule.DecisionMode, &rule.Enabled, &rule.Priority, &rule.Action,
		&rule.Provider, &rule.ModelPattern, &rule.Environment, &rule.MaxTokens, &rule.Detector,
		&rule.Scope, &rawGuardrails, &rule.SchemaJSON, &rawUnsafeCategories, &rule.RolloutPercent, &rule.Version,
		&rawConditions, &rule.RegoModule, &rule.Description, &rule.CreatedAt, &rule.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	rule.Guardrails = decodePolicyStringSlice(rawGuardrails)
	rule.UnsafeCategories = decodePolicyStringSlice(rawUnsafeCategories)
	rule.RuleConditions = decodePolicyConditions(rawConditions)
	return &rule, nil
}

func (s *PostgresStore) UpsertPolicyRule(ctx context.Context, rule models.PolicyRule) (models.PolicyRule, error) {
	if rule.Priority == 0 {
		rule.Priority = 100
	}
	if strings.TrimSpace(rule.DecisionMode) == "" {
		rule.DecisionMode = "fast"
	}
	if rule.Scope == "" {
		rule.Scope = "both"
	}
	rule.RuleConditions = clonePolicyConditions(rule.RuleConditions)
	rule.Guardrails = clonePolicyStringSlice(rule.Guardrails)
	rule.UnsafeCategories = clonePolicyStringSlice(rule.UnsafeCategories)
	conditionsJSON, err := encodePolicyConditions(rule.RuleConditions)
	if err != nil {
		return rule, err
	}
	guardrailsJSON, err := encodePolicyStringSlice(rule.Guardrails)
	if err != nil {
		return rule, err
	}
	unsafeCategoriesJSON, err := encodePolicyStringSlice(rule.UnsafeCategories)
	if err != nil {
		return rule, err
	}
	if rule.ID > 0 {
		err := s.pool.QueryRow(ctx, `
			UPDATE policy_rules
			SET tenant_id = $2,
			    name = $3,
			    rule_type = $4,
			    decision_mode = $5,
			    enabled = $6,
			    priority = $7,
			    action = $8,
			    provider = $9,
			    model_pattern = $10,
			    environment = $11,
			    max_tokens = $12,
			    detector = $13,
			    scope = $14,
			    guardrails = $15,
			    schema_json = $16,
			    unsafe_categories = $17,
			    rollout_percent = $18,
			    version = $19,
			    rule_conditions = $20,
			    rego_module = $21,
			    description = $22,
			    updated_at = NOW()
			WHERE id = $1
			RETURNING id, tenant_id, name, rule_type, decision_mode, enabled, priority, action, provider, model_pattern,
			          environment, max_tokens, detector, scope, guardrails, schema_json, unsafe_categories, rollout_percent, version,
			          rule_conditions, rego_module, description, created_at, updated_at
		`, rule.ID, rule.TenantID, rule.Name, rule.RuleType, rule.DecisionMode, rule.Enabled, rule.Priority, rule.Action,
			rule.Provider, rule.ModelPattern, rule.Environment, rule.MaxTokens, rule.Detector, rule.Scope, guardrailsJSON,
			rule.SchemaJSON, unsafeCategoriesJSON, rule.RolloutPercent, rule.Version, conditionsJSON,
			rule.RegoModule, rule.Description).Scan(
			&rule.ID, &rule.TenantID, &rule.Name, &rule.RuleType, &rule.DecisionMode, &rule.Enabled, &rule.Priority, &rule.Action,
			&rule.Provider, &rule.ModelPattern, &rule.Environment, &rule.MaxTokens, &rule.Detector,
			&rule.Scope, &guardrailsJSON, &rule.SchemaJSON, &unsafeCategoriesJSON, &rule.RolloutPercent, &rule.Version,
			&conditionsJSON, &rule.RegoModule, &rule.Description, &rule.CreatedAt, &rule.UpdatedAt,
		)
		rule.Guardrails = decodePolicyStringSlice(guardrailsJSON)
		rule.UnsafeCategories = decodePolicyStringSlice(unsafeCategoriesJSON)
		rule.RuleConditions = decodePolicyConditions(conditionsJSON)
		return rule, err
	}
	err = s.pool.QueryRow(ctx, `
		INSERT INTO policy_rules (
			tenant_id, name, rule_type, decision_mode, enabled, priority, action, provider, model_pattern,
			environment, max_tokens, detector, scope, guardrails, schema_json, unsafe_categories, rollout_percent, version,
			rule_conditions, rego_module, description
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
		RETURNING id, tenant_id, name, rule_type, decision_mode, enabled, priority, action, provider, model_pattern,
		          environment, max_tokens, detector, scope, guardrails, schema_json, unsafe_categories, rollout_percent, version,
		          rule_conditions, rego_module, description, created_at, updated_at
	`, rule.TenantID, rule.Name, rule.RuleType, rule.DecisionMode, rule.Enabled, rule.Priority, rule.Action, rule.Provider,
		rule.ModelPattern, rule.Environment, rule.MaxTokens, rule.Detector, rule.Scope, guardrailsJSON, rule.SchemaJSON,
		unsafeCategoriesJSON, rule.RolloutPercent, rule.Version, conditionsJSON, rule.RegoModule, rule.Description).Scan(
		&rule.ID, &rule.TenantID, &rule.Name, &rule.RuleType, &rule.DecisionMode, &rule.Enabled, &rule.Priority, &rule.Action,
		&rule.Provider, &rule.ModelPattern, &rule.Environment, &rule.MaxTokens, &rule.Detector,
		&rule.Scope, &guardrailsJSON, &rule.SchemaJSON, &unsafeCategoriesJSON, &rule.RolloutPercent, &rule.Version,
		&conditionsJSON, &rule.RegoModule, &rule.Description, &rule.CreatedAt, &rule.UpdatedAt,
	)
	rule.Guardrails = decodePolicyStringSlice(guardrailsJSON)
	rule.UnsafeCategories = decodePolicyStringSlice(unsafeCategoriesJSON)
	rule.RuleConditions = decodePolicyConditions(conditionsJSON)
	return rule, err
}

func (s *PostgresStore) DeletePolicyRule(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM policy_rules WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func encodePolicyConditions(conditions map[string]string) ([]byte, error) {
	if len(conditions) == 0 {
		return []byte(`{}`), nil
	}
	return json.Marshal(conditions)
}

func decodePolicyConditions(raw []byte) map[string]string {
	if len(raw) == 0 {
		return map[string]string{}
	}
	out := map[string]string{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]string{}
	}
	return out
}

func clonePolicyConditions(conditions map[string]string) map[string]string {
	if len(conditions) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(conditions))
	for key, value := range conditions {
		out[key] = value
	}
	return out
}

func encodePolicyStringSlice(values []string) ([]byte, error) {
	if len(values) == 0 {
		return []byte(`[]`), nil
	}
	return json.Marshal(values)
}

func decodePolicyStringSlice(raw []byte) []string {
	if len(raw) == 0 {
		return []string{}
	}
	out := []string{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return []string{}
	}
	return out
}

func clonePolicyStringSlice(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}

func (s *PostgresStore) UpsertPricingRule(ctx context.Context, rule models.PricingRule) (models.PricingRule, error) {
	rule.Provider = strings.ToLower(strings.TrimSpace(rule.Provider))
	rule.ModelPattern = strings.ToLower(strings.TrimSpace(rule.ModelPattern))
	if rule.Priority == 0 {
		rule.Priority = 100
	}
	if rule.ID == 0 && !rule.Active {
		rule.Active = true
	}

	if rule.ID > 0 {
		err := s.pool.QueryRow(ctx, `
			UPDATE pricing_rules
			SET tenant_id = $2,
			    provider = $3,
			    model_pattern = $4,
			    input_per_million = $5,
			    output_per_million = $6,
			    cache_read_per_million = $7,
			    cache_write_per_million = $8,
			    reasoning_per_million = $9,
			    active = $10,
			    priority = $11,
			    effective_from = $12,
			    effective_to = $13,
			    description = $14,
			    updated_at = NOW()
			WHERE id = $1
			RETURNING id, tenant_id, provider, model_pattern, input_per_million, output_per_million,
			          cache_read_per_million, cache_write_per_million, reasoning_per_million,
			          active, priority, effective_from, effective_to, description, created_at, updated_at
		`, rule.ID, rule.TenantID, rule.Provider, rule.ModelPattern, rule.InputPerMillion, rule.OutputPerMillion,
			rule.CacheReadPerMillion, rule.CacheWritePerMillion, rule.ReasoningPerMillion, rule.Active, rule.Priority, rule.EffectiveFrom, rule.EffectiveTo, rule.Description).Scan(
			&rule.ID, &rule.TenantID, &rule.Provider, &rule.ModelPattern, &rule.InputPerMillion,
			&rule.OutputPerMillion, &rule.CacheReadPerMillion, &rule.CacheWritePerMillion, &rule.ReasoningPerMillion, &rule.Active, &rule.Priority, &rule.EffectiveFrom,
			&rule.EffectiveTo, &rule.Description, &rule.CreatedAt, &rule.UpdatedAt,
		)
		return rule, err
	}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO pricing_rules (
			tenant_id, provider, model_pattern, input_per_million, output_per_million,
			cache_read_per_million, cache_write_per_million, reasoning_per_million,
			active, priority, effective_from, effective_to, description
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id, tenant_id, provider, model_pattern, input_per_million, output_per_million,
		          cache_read_per_million, cache_write_per_million, reasoning_per_million,
		          active, priority, effective_from, effective_to, description, created_at, updated_at
	`, rule.TenantID, rule.Provider, rule.ModelPattern, rule.InputPerMillion, rule.OutputPerMillion,
		rule.CacheReadPerMillion, rule.CacheWritePerMillion, rule.ReasoningPerMillion,
		rule.Active, rule.Priority, rule.EffectiveFrom, rule.EffectiveTo, rule.Description).Scan(
		&rule.ID, &rule.TenantID, &rule.Provider, &rule.ModelPattern, &rule.InputPerMillion,
		&rule.OutputPerMillion, &rule.CacheReadPerMillion, &rule.CacheWritePerMillion, &rule.ReasoningPerMillion, &rule.Active, &rule.Priority, &rule.EffectiveFrom,
		&rule.EffectiveTo, &rule.Description, &rule.CreatedAt, &rule.UpdatedAt,
	)
	return rule, err
}

func (s *PostgresStore) GetPricingRule(ctx context.Context, id int64) (*models.PricingRule, error) {
	var rule models.PricingRule
	err := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, provider, model_pattern, input_per_million, output_per_million,
		       cache_read_per_million, cache_write_per_million, reasoning_per_million,
		       active, priority, effective_from, effective_to, description, created_at, updated_at
		FROM pricing_rules
		WHERE id = $1
	`, id).Scan(
		&rule.ID, &rule.TenantID, &rule.Provider, &rule.ModelPattern, &rule.InputPerMillion,
		&rule.OutputPerMillion, &rule.CacheReadPerMillion, &rule.CacheWritePerMillion, &rule.ReasoningPerMillion, &rule.Active, &rule.Priority, &rule.EffectiveFrom,
		&rule.EffectiveTo, &rule.Description, &rule.CreatedAt, &rule.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

func (s *PostgresStore) DeletePricingRule(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM pricing_rules WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *PostgresStore) CreatePricingRuleAudit(ctx context.Context, entry models.PricingAuditEntry) error {
	beforeJSON := "null"
	afterJSON := "null"
	if strings.TrimSpace(entry.BeforeState) != "" {
		beforeJSON = entry.BeforeState
	}
	if strings.TrimSpace(entry.AfterState) != "" {
		afterJSON = entry.AfterState
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO pricing_rule_audit (rule_id, action, actor, tenant_id, before_state, after_state)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6::jsonb)
	`, entry.RuleID, entry.Action, entry.Actor, entry.TenantID, beforeJSON, afterJSON)
	return err
}

func (s *PostgresStore) ListPricingRuleAudit(ctx context.Context, limit int) ([]models.PricingAuditEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, rule_id, action, actor, tenant_id,
		       before_state::text, after_state::text, created_at
		FROM pricing_rule_audit
		ORDER BY created_at DESC, id DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []models.PricingAuditEntry
	for rows.Next() {
		var entry models.PricingAuditEntry
		if err := rows.Scan(
			&entry.ID, &entry.RuleID, &entry.Action, &entry.Actor, &entry.TenantID,
			&entry.BeforeState, &entry.AfterState, &entry.CreatedAt,
		); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if entries == nil {
		entries = []models.PricingAuditEntry{}
	}
	return entries, rows.Err()
}
