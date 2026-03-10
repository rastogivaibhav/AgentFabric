package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/agentfabric/api-gateway/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

type PostgresStore struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
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
	if err := s.migrate(ctx); err != nil {
		return nil, fmt.Errorf("migration: %w", err)
	}
	return s, nil
}

func (s *PostgresStore) migrate(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, schema)
	return err
}

const schema = `
CREATE TABLE IF NOT EXISTS tenants (
    tenant_id   TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
INSERT INTO tenants (tenant_id, name) VALUES ('default', 'Default') ON CONFLICT DO NOTHING;

CREATE TABLE IF NOT EXISTS spans (
    span_id         TEXT NOT NULL,
    trace_id        TEXT NOT NULL,
    parent_span_id  TEXT,
    run_id          TEXT NOT NULL,
    name            TEXT NOT NULL,
    framework       TEXT NOT NULL DEFAULT 'unknown',
    start_time_ns   BIGINT NOT NULL,
    duration_ns     BIGINT NOT NULL DEFAULT 0,
    status_code     SMALLINT NOT NULL DEFAULT 0,
    status_msg      TEXT,
    attributes      JSONB NOT NULL DEFAULT '{}',
    events          JSONB NOT NULL DEFAULT '[]',
    input_tokens    BIGINT NOT NULL DEFAULT 0,
    output_tokens   BIGINT NOT NULL DEFAULT 0,
    cost_usd        NUMERIC(12,8) NOT NULL DEFAULT 0,
    tenant_id       TEXT NOT NULL DEFAULT 'default',
    received_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (span_id, tenant_id)
);
CREATE INDEX IF NOT EXISTS idx_spans_trace     ON spans(trace_id, tenant_id);
CREATE INDEX IF NOT EXISTS idx_spans_run       ON spans(run_id, tenant_id);
CREATE INDEX IF NOT EXISTS idx_spans_framework ON spans(framework, tenant_id, received_at DESC);
CREATE INDEX IF NOT EXISTS idx_spans_time      ON spans(received_at DESC, tenant_id);

CREATE TABLE IF NOT EXISTS runs (
    run_id          TEXT NOT NULL,
    trace_id        TEXT NOT NULL,
    parent_run_id   TEXT,
    framework       TEXT NOT NULL DEFAULT 'unknown',
    agent_name      TEXT NOT NULL DEFAULT 'unknown',
    model           TEXT NOT NULL DEFAULT '',
    start_time      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    end_time        TIMESTAMPTZ,
    status          TEXT NOT NULL DEFAULT 'running',
    total_tokens    BIGINT NOT NULL DEFAULT 0,
    total_cost_usd  NUMERIC(12,8) NOT NULL DEFAULT 0,
    metadata        JSONB NOT NULL DEFAULT '{}',
    tenant_id       TEXT NOT NULL DEFAULT 'default',
    PRIMARY KEY (run_id, tenant_id)
);
CREATE INDEX IF NOT EXISTS idx_runs_trace     ON runs(trace_id, tenant_id);
CREATE INDEX IF NOT EXISTS idx_runs_framework ON runs(framework, tenant_id, start_time DESC);
CREATE INDEX IF NOT EXISTS idx_runs_agent     ON runs(agent_name, tenant_id, start_time DESC);

CREATE TABLE IF NOT EXISTS policy_audit_log (
    id              BIGSERIAL PRIMARY KEY,
    decision_id     TEXT NOT NULL UNIQUE,
    trace_id        TEXT NOT NULL,
    span_id         TEXT NOT NULL,
    policy_name     TEXT NOT NULL,
    result          TEXT NOT NULL,
    reason          TEXT NOT NULL,
    tenant_id       TEXT NOT NULL DEFAULT 'default',
    evaluated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- Prevent modification of audit log
CREATE OR REPLACE RULE no_update_audit AS ON UPDATE TO policy_audit_log DO INSTEAD NOTHING;
CREATE OR REPLACE RULE no_delete_audit AS ON DELETE TO policy_audit_log DO INSTEAD NOTHING;

CREATE TABLE IF NOT EXISTS feedback (
    id          BIGSERIAL PRIMARY KEY,
    run_id      TEXT NOT NULL,
    score       SMALLINT,
    comment     TEXT,
    tenant_id   TEXT NOT NULL DEFAULT 'default',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
`

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
			sp.InputTokens, sp.OutputTokens, sp.CostUSD,
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
			"input_tokens", "output_tokens", "cost_usd",
			"tenant_id",
		},
		pgx.CopyFromRows(rows),
	)
	return err
}

// ─── Trace queries ────────────────────────────────────────────────────────────

func (s *PostgresStore) ListTraces(ctx context.Context, q models.TraceQuery) (*models.Page[models.Trace], error) {
	if q.Limit <= 0 || q.Limit > 200 {
		q.Limit = 50
	}

	query := `
		SELECT 
			trace_id,
			MIN(name) as root_span_name,
			MAX(framework) as framework,
			MIN(start_time_ns) as start_ns,
			MAX(start_time_ns + duration_ns) - MIN(start_time_ns) as duration_ns,
			COUNT(*) as span_count,
			SUM(CASE WHEN status_code = 2 THEN 1 ELSE 0 END) as error_count,
			SUM(cost_usd) as total_cost,
			SUM(input_tokens + output_tokens) as total_tokens,
			CASE WHEN SUM(CASE WHEN status_code = 2 THEN 1 ELSE 0 END) > 0 THEN 'error' ELSE 'ok' END as status
		FROM spans
		WHERE tenant_id = $1`

	args := []interface{}{q.TenantID}
	argIdx := 2

	if q.Framework != "" {
		query += fmt.Sprintf(" AND framework = $%d", argIdx)
		args = append(args, q.Framework)
		argIdx++
	}
	if q.StartTime > 0 {
		query += fmt.Sprintf(" AND start_time_ns >= $%d", argIdx)
		args = append(args, q.StartTime)
		argIdx++
	}
	if q.EndTime > 0 {
		query += fmt.Sprintf(" AND start_time_ns <= $%d", argIdx)
		args = append(args, q.EndTime)
		argIdx++
	}

	query += fmt.Sprintf(`
		GROUP BY trace_id
		ORDER BY MIN(start_time_ns) DESC
		LIMIT $%d`, argIdx)
	args = append(args, q.Limit)

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

	return &models.Page[models.Trace]{
		Items:   traces,
		HasMore: len(traces) == q.Limit,
	}, nil
}

func (s *PostgresStore) GetTraceSpans(ctx context.Context, traceID, tenantID string) ([]models.Span, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT span_id, trace_id, COALESCE(parent_span_id,''), run_id, name, framework,
		       start_time_ns, duration_ns, status_code, COALESCE(status_msg,''),
		       attributes, events, input_tokens, output_tokens, cost_usd, received_at
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
			&attrsJSON, &eventsJSON, &sp.InputTokens, &sp.OutputTokens, &sp.CostUSD,
			&sp.ReceivedAt,
		); err != nil {
			continue
		}
		json.Unmarshal(attrsJSON, &sp.Attributes)
		json.Unmarshal(eventsJSON, &sp.Events)
		spans = append(spans, sp)
	}
	return spans, nil
}

func (s *PostgresStore) GetOverview(ctx context.Context, tenantID string, since time.Duration) (*models.OverviewStats, error) {
	cutoff := time.Now().Add(-since).UnixNano()
	var stats models.OverviewStats

	err := s.pool.QueryRow(ctx, `
		SELECT 
			COUNT(DISTINCT trace_id),
			SUM(cost_usd),
			SUM(input_tokens + output_tokens),
			AVG(CASE WHEN status_code = 2 THEN 1.0 ELSE 0.0 END),
			AVG(duration_ns) / 1e6
		FROM spans
		WHERE tenant_id = $1 AND start_time_ns >= $2`,
		tenantID, cutoff,
	).Scan(
		&stats.TotalTraces, &stats.TotalCostUSD, &stats.TotalTokens,
		&stats.ErrorRate, &stats.AvgLatencyMs,
	)
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

func (s *PostgresStore) Close() {
	s.pool.Close()
}
