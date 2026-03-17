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

	"github.com/agentfabric/api-gateway/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
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

// GetSpansForTraces fetches spans for multiple trace IDs in a single query (P0-2 fix).
// Use this instead of calling GetTraceSpans in a loop.
func (s *PostgresStore) GetSpansForTraces(ctx context.Context, traceIDs []string, tenantID string) ([]models.Span, error) {
	if len(traceIDs) == 0 {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT span_id, trace_id, COALESCE(parent_span_id,''), run_id, name, framework,
		       start_time_ns, duration_ns, status_code, COALESCE(status_msg,''),
		       attributes, events, input_tokens, output_tokens, cost_usd, received_at
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
	query += fmt.Sprintf(" ORDER BY start_time DESC LIMIT $%d", idx)
	args = append(args, q.Limit)

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
	return &models.Page[models.Run]{Items: runs, HasMore: len(runs) == q.Limit}, nil
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

// ─── Reports ──────────────────────────────────────────────────────────────────

type CostReportRow struct {
	Model        string  `json:"model"`
	Framework    string  `json:"framework"`
	TotalCost    float64 `json:"total_cost_usd"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	TraceCount   int64   `json:"trace_count"`
}

func (s *PostgresStore) GetCostReport(ctx context.Context, tenantID string, since time.Duration) ([]CostReportRow, error) {
	cutoff := time.Now().Add(-since).UnixNano()
	rows, err := s.pool.Query(ctx, `
		SELECT
		    COALESCE(attributes->>'gen_ai.request.model', 'unknown') AS model,
		    framework,
		    SUM(cost_usd)                  AS total_cost,
		    SUM(input_tokens)              AS input_tokens,
		    SUM(output_tokens)             AS output_tokens,
		    COUNT(DISTINCT trace_id)       AS trace_count
		FROM spans
		WHERE tenant_id = $1 AND start_time_ns >= $2 AND cost_usd > 0
		GROUP BY model, framework
		ORDER BY total_cost DESC
		LIMIT 100`,
		tenantID, cutoff,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []CostReportRow
	for rows.Next() {
		var r CostReportRow
		if err := rows.Scan(&r.Model, &r.Framework, &r.TotalCost, &r.InputTokens, &r.OutputTokens, &r.TraceCount); err != nil {
			continue
		}
		result = append(result, r)
	}
	if result == nil {
		result = []CostReportRow{}
	}
	return result, nil
}

type ErrorReportRow struct {
	Framework      string `json:"framework"`
	StatusMsg      string `json:"status_msg"`
	Count          int64  `json:"count"`
	AffectedTraces int64  `json:"affected_traces"`
}

func (s *PostgresStore) GetErrorReport(ctx context.Context, tenantID string, since time.Duration) ([]ErrorReportRow, error) {
	cutoff := time.Now().Add(-since).UnixNano()
	rows, err := s.pool.Query(ctx, `
		SELECT
		    framework,
		    COALESCE(NULLIF(status_msg, ''), 'unknown error') AS status_msg,
		    COUNT(*)                                           AS count,
		    COUNT(DISTINCT trace_id)                          AS affected_traces
		FROM spans
		WHERE tenant_id = $1 AND start_time_ns >= $2 AND status_code = 2
		GROUP BY framework, status_msg
		ORDER BY count DESC
		LIMIT 50`,
		tenantID, cutoff,
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
		`SELECT id, name, description, status FROM environments WHERE tenant_id = $1 ORDER BY name`,
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

// VerifyAuditChain replays the SHA-256 hash chain for a tenant and reports
// the first broken link, if any. This is the Go equivalent of af-core's
// AuditWriter.verify_chain().
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
		// Replicate the exact payload format from af-core/src/policy/audit.rs
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

// CreateUser inserts a new user into the tenant. Password is SHA-256 hashed before storage.
// TODO(v1.1): migrate to bcrypt once golang.org/x/crypto is a declared dependency.
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

// GetUserByUsername looks up a user by username within a tenant and returns the
// minimal auth fields including the bcrypt password hash.
// Implements auth.UserLookup — called by OIDCHandler.PasswordLogin.
func (s *PostgresStore) GetUserByUsername(ctx context.Context, username, tenantID string) (*models.UserRecord, error) {
	var rec models.UserRecord
	err := s.pool.QueryRow(ctx, `
		SELECT user_id, username, email, display_name, role, COALESCE(password_hash,'')
		FROM users
		WHERE username = $1 AND tenant_id = $2 AND is_active = TRUE
	`, username, tenantID).Scan(
		&rec.ID, &rec.Username, &rec.Email, &rec.DisplayName, &rec.Role, &rec.PasswordHash,
	)
	if err != nil {
		return nil, err
	}
	return &rec, nil
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
