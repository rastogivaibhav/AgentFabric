package store

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/govagn/api-gateway/internal/models"
	"github.com/jackc/pgx/v5"
)

func (s *PostgresStore) ListEvalRuns(ctx context.Context, tenantID string, limit int) ([]models.TraceEvalRun, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, trace_id, COALESCE(release_tag,''), eval_suite, overall_score, risk_level, COALESCE(summary,''), policy_effectiveness::text, created_at
		FROM eval_runs
		WHERE tenant_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2
	`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	runs := make([]models.TraceEvalRun, 0, limit)
	for rows.Next() {
		var run models.TraceEvalRun
		var rawPolicy string
		if err := rows.Scan(&run.ID, &run.TraceID, &run.ReleaseTag, &run.EvalSuite, &run.OverallScore, &run.RiskLevel, &run.Summary, &rawPolicy, &run.CreatedAt); err != nil {
			return nil, err
		}
		run.PolicyEffectiveness = decodePolicyEffectiveness(rawPolicy)
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return s.attachEvalScores(ctx, tenantID, runs)
}

func (s *PostgresStore) ListEvalRunsByRelease(ctx context.Context, tenantID, promptID, promptEnvironment, releaseTag, evalSuite string) ([]models.TraceEvalRun, error) {
	where := []string{"tenant_id = $1", "release_tag = $2", "eval_suite = $3"}
	args := []interface{}{tenantID, strings.TrimSpace(releaseTag), strings.TrimSpace(evalSuite)}
	_ = strings.TrimSpace(promptID)
	_ = strings.TrimSpace(promptEnvironment)

	rows, err := s.pool.Query(ctx, `
		SELECT id, trace_id, COALESCE(release_tag,''), eval_suite, overall_score, risk_level, COALESCE(summary,''), policy_effectiveness::text, created_at
		FROM eval_runs
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY created_at DESC, id DESC
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	runs := []models.TraceEvalRun{}
	for rows.Next() {
		var run models.TraceEvalRun
		var rawPolicy string
		if err := rows.Scan(&run.ID, &run.TraceID, &run.ReleaseTag, &run.EvalSuite, &run.OverallScore, &run.RiskLevel, &run.Summary, &rawPolicy, &run.CreatedAt); err != nil {
			return nil, err
		}
		run.PolicyEffectiveness = decodePolicyEffectiveness(rawPolicy)
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return s.attachEvalScores(ctx, tenantID, runs)
}

func (s *PostgresStore) InsertEvalRun(ctx context.Context, tenantID string, run models.TraceEvalRun) (models.TraceEvalRun, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return run, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	rawPolicy, err := json.Marshal(run.PolicyEffectiveness)
	if err != nil {
		return run, err
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO eval_runs (tenant_id, trace_id, release_tag, eval_suite, overall_score, risk_level, summary, policy_effectiveness)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb)
		RETURNING id, created_at
	`, tenantID, strings.TrimSpace(run.TraceID), strings.TrimSpace(run.ReleaseTag), strings.TrimSpace(run.EvalSuite), run.OverallScore, strings.TrimSpace(run.RiskLevel), strings.TrimSpace(run.Summary), string(rawPolicy)).Scan(&run.ID, &run.CreatedAt)
	if err != nil {
		return run, err
	}

	for _, score := range run.Scores {
		if _, err := tx.Exec(ctx, `
			INSERT INTO eval_scores (tenant_id, eval_run_id, metric_name, score, weight, severity, summary)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`, tenantID, run.ID, strings.TrimSpace(score.Metric), score.Score, score.Weight, strings.TrimSpace(score.Severity), strings.TrimSpace(score.Summary)); err != nil {
			return run, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return run, err
	}
	return run, nil
}

func (s *PostgresStore) attachEvalScores(ctx context.Context, tenantID string, runs []models.TraceEvalRun) ([]models.TraceEvalRun, error) {
	if len(runs) == 0 {
		return runs, nil
	}
	ids := make([]int64, 0, len(runs))
	runByID := make(map[int64]*models.TraceEvalRun, len(runs))
	for i := range runs {
		ids = append(ids, runs[i].ID)
		runByID[runs[i].ID] = &runs[i]
	}
	rows, err := s.pool.Query(ctx, `
		SELECT eval_run_id, metric_name, score, weight, COALESCE(severity,''), COALESCE(summary,'')
		FROM eval_scores
		WHERE tenant_id = $1 AND eval_run_id = ANY($2)
		ORDER BY eval_run_id ASC, metric_name ASC
	`, tenantID, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var runID int64
		var score models.TraceEvalScore
		if err := rows.Scan(&runID, &score.Metric, &score.Score, &score.Weight, &score.Severity, &score.Summary); err != nil {
			return nil, err
		}
		if run := runByID[runID]; run != nil {
			run.Scores = append(run.Scores, score)
		}
	}
	return runs, rows.Err()
}

func decodePolicyEffectiveness(raw string) models.PolicyEffectivenessSummary {
	if strings.TrimSpace(raw) == "" {
		return models.PolicyEffectivenessSummary{}
	}
	var out models.PolicyEffectivenessSummary
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return models.PolicyEffectivenessSummary{}
	}
	return out
}

func (s *PostgresStore) GetEvalRun(ctx context.Context, tenantID string, runID int64) (models.TraceEvalRun, error) {
	var run models.TraceEvalRun
	var rawPolicy string
	err := s.pool.QueryRow(ctx, `
		SELECT id, trace_id, COALESCE(release_tag,''), eval_suite, overall_score, risk_level, COALESCE(summary,''), policy_effectiveness::text, created_at
		FROM eval_runs
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, runID).Scan(&run.ID, &run.TraceID, &run.ReleaseTag, &run.EvalSuite, &run.OverallScore, &run.RiskLevel, &run.Summary, &rawPolicy, &run.CreatedAt)
	if err != nil {
		return run, err
	}
	run.PolicyEffectiveness = decodePolicyEffectiveness(rawPolicy)
	runs, err := s.attachEvalScores(ctx, tenantID, []models.TraceEvalRun{run})
	if err != nil {
		return run, err
	}
	if len(runs) == 0 {
		return run, pgx.ErrNoRows
	}
	return runs[0], nil
}
