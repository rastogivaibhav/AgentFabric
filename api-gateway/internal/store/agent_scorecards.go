package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/govagn/api-gateway/internal/models"
)

const (
	scorecardAppExpr     = "COALESCE(NULLIF(s.attributes->>'af.app.name', ''), NULLIF(s.attributes->>'service.name', ''), NULLIF(s.attributes->>'application.name', ''), 'unknown')"
	scorecardEnvExpr     = "COALESCE(NULLIF(s.attributes->>'af.environment', ''), NULLIF(s.attributes->>'deployment.environment', ''), NULLIF(s.attributes->>'environment', ''), NULLIF(s.attributes->>'env', ''), 'unknown')"
	scorecardReleaseExpr = "COALESCE(NULLIF(s.attributes->>'af.prompt.release_tag', ''), 'unreleased')"
)

func (s *PostgresStore) ListAgentScorecardMetrics(ctx context.Context, tenantID string, windowStart, windowEnd time.Time, limit int, agentName string) ([]models.AgentScorecardMetrics, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	agentFilter := ""
	args := []interface{}{tenantID, windowStart.UTC(), windowEnd.UTC(), windowStart.UTC().UnixNano(), windowEnd.UTC().UnixNano()}
	if trimmed := strings.TrimSpace(agentName); trimmed != "" {
		agentFilter = " AND agent_name = $6"
		args = append(args, trimmed)
	}
	args = append(args, limit)
	limitIdx := len(args)

	query := fmt.Sprintf(`
		WITH recent_runs AS (
			SELECT
				run_id,
				trace_id,
				agent_name,
				framework,
				start_time,
				COALESCE(end_time, start_time) AS end_time,
				status,
				total_tokens,
				total_cost_usd,
				EXTRACT(EPOCH FROM (COALESCE(end_time, start_time) - start_time)) * 1000 AS latency_ms
			FROM runs
			WHERE tenant_id = $1
			  AND start_time >= $2
			  AND start_time < $3
			  AND COALESCE(agent_name, '') <> ''%s
		),
		run_agg AS (
			SELECT
				agent_name,
				MAX(framework) AS framework,
				MIN(start_time) AS first_seen,
				MAX(end_time) AS last_seen,
				COUNT(*) AS run_count,
				COALESCE(SUM(total_cost_usd), 0) AS total_cost_usd,
				COALESCE(SUM(total_tokens), 0) AS total_tokens,
				COALESCE(AVG(latency_ms), 0) AS avg_latency_ms,
				COALESCE(AVG(CASE WHEN LOWER(status) = 'error' THEN 1.0 ELSE 0.0 END), 0) AS run_error_rate
			FROM recent_runs
			GROUP BY agent_name
		),
		app_rank AS (
			SELECT
				rr.agent_name,
				%s AS app_name,
				COUNT(*) AS observed_count,
				ROW_NUMBER() OVER (PARTITION BY rr.agent_name ORDER BY COUNT(*) DESC, %s) AS rn
			FROM recent_runs rr
			JOIN spans s ON s.tenant_id = $1 AND s.trace_id = rr.trace_id AND s.start_time_ns >= $4 AND s.start_time_ns < $5
			GROUP BY rr.agent_name, %s
		),
		env_rank AS (
			SELECT
				rr.agent_name,
				%s AS environment,
				COUNT(*) AS observed_count,
				ROW_NUMBER() OVER (PARTITION BY rr.agent_name ORDER BY COUNT(*) DESC, %s) AS rn
			FROM recent_runs rr
			JOIN spans s ON s.tenant_id = $1 AND s.trace_id = rr.trace_id AND s.start_time_ns >= $4 AND s.start_time_ns < $5
			GROUP BY rr.agent_name, %s
		),
		release_rank AS (
			SELECT
				rr.agent_name,
				%s AS release_tag,
				COUNT(*) AS observed_count,
				ROW_NUMBER() OVER (PARTITION BY rr.agent_name ORDER BY COUNT(*) DESC, %s) AS rn
			FROM recent_runs rr
			JOIN spans s ON s.tenant_id = $1 AND s.trace_id = rr.trace_id AND s.start_time_ns >= $4 AND s.start_time_ns < $5
			GROUP BY rr.agent_name, %s
		),
		decision_agg AS (
			SELECT
				rr.agent_name,
				COUNT(*) AS decision_count,
				COALESCE(SUM(CASE WHEN d.decision_type = 'policy' AND LOWER(d.result) IN ('deny', 'block') THEN 1 ELSE 0 END), 0) AS policy_block_count,
				COALESCE(SUM(CASE WHEN d.decision_type = 'policy' AND LOWER(d.result) IN ('sanitize', 'redact') THEN 1 ELSE 0 END), 0) AS policy_redaction_count,
				COALESCE(SUM(CASE WHEN d.decision_type = 'budget' AND LOWER(d.result) = 'deny' THEN 1 ELSE 0 END), 0) AS budget_denied_count,
				COALESCE(SUM(CASE WHEN d.decision_type IN ('fallback', 'retry') THEN 1 ELSE 0 END), 0) AS fallback_count
			FROM recent_runs rr
			JOIN decision_records d ON d.tenant_id = $1 AND d.trace_id = rr.trace_id AND d.created_at >= $2 AND d.created_at < $3
			GROUP BY rr.agent_name
		),
		eval_scored AS (
			SELECT
				rr.agent_name,
				e.overall_score,
				e.created_at,
				e.id,
				ROW_NUMBER() OVER (PARTITION BY rr.agent_name ORDER BY e.created_at DESC, e.id DESC) AS rn
			FROM recent_runs rr
			JOIN eval_runs e ON e.tenant_id = $1 AND e.trace_id = rr.trace_id AND e.created_at >= $2 AND e.created_at < $3
		),
		eval_agg AS (
			SELECT
				agent_name,
				COUNT(*) AS eval_count,
				COALESCE(AVG(overall_score), 0) AS eval_average_score,
				COALESCE(AVG(CASE WHEN rn <= 3 THEN overall_score END), 0) AS recent_eval_average,
				COALESCE(MAX(CASE WHEN rn = 1 THEN overall_score END), 0) AS latest_eval_score
			FROM eval_scored
			GROUP BY agent_name
		),
		rollout_agg AS (
			SELECT
				rr.agent_name,
				COUNT(*) AS rollout_count,
				COALESCE(AVG(CASE WHEN LOWER(re.status) IN ('error', 'blocked') OR re.status_code >= 400 THEN 1.0 ELSE 0.0 END), 0) AS rollout_error_rate,
				COALESCE(SUM(CASE WHEN re.auto_paused THEN 1 ELSE 0 END), 0) AS auto_pause_count
			FROM recent_runs rr
			JOIN rollout_events re ON re.tenant_id = $1 AND re.trace_id = rr.trace_id AND re.created_at >= $2 AND re.created_at < $3
			GROUP BY rr.agent_name
		)
		SELECT
			ra.agent_name,
			ra.framework,
			COALESCE(ar.app_name, 'unknown') AS app_name,
			COALESCE(er.environment, 'unknown') AS environment,
			COALESCE(rrk.release_tag, 'unreleased') AS release_tag,
			ra.first_seen,
			ra.last_seen,
			ra.run_count,
			ra.total_cost_usd,
			ra.total_tokens,
			ra.avg_latency_ms,
			ra.run_error_rate,
			COALESCE(da.decision_count, 0),
			COALESCE(da.policy_block_count, 0),
			COALESCE(da.policy_redaction_count, 0),
			COALESCE(da.budget_denied_count, 0),
			COALESCE(da.fallback_count, 0),
			COALESCE(ea.eval_count, 0),
			COALESCE(ea.eval_average_score, 0),
			COALESCE(ea.recent_eval_average, 0),
			COALESCE(ea.latest_eval_score, 0),
			COALESCE(ro.rollout_count, 0),
			COALESCE(ro.rollout_error_rate, 0),
			COALESCE(ro.auto_pause_count, 0)
		FROM run_agg ra
		LEFT JOIN app_rank ar ON ar.agent_name = ra.agent_name AND ar.rn = 1
		LEFT JOIN env_rank er ON er.agent_name = ra.agent_name AND er.rn = 1
		LEFT JOIN release_rank rrk ON rrk.agent_name = ra.agent_name AND rrk.rn = 1
		LEFT JOIN decision_agg da ON da.agent_name = ra.agent_name
		LEFT JOIN eval_agg ea ON ea.agent_name = ra.agent_name
		LEFT JOIN rollout_agg ro ON ro.agent_name = ra.agent_name
		ORDER BY ra.last_seen DESC, ra.agent_name ASC
		LIMIT $%d
	`,
		agentFilter,
		scorecardAppExpr, scorecardAppExpr, scorecardAppExpr,
		scorecardEnvExpr, scorecardEnvExpr, scorecardEnvExpr,
		scorecardReleaseExpr, scorecardReleaseExpr, scorecardReleaseExpr,
		limitIdx,
	)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]models.AgentScorecardMetrics, 0, limit)
	for rows.Next() {
		var item models.AgentScorecardMetrics
		if err := rows.Scan(
			&item.AgentName,
			&item.Framework,
			&item.AppName,
			&item.Environment,
			&item.ReleaseTag,
			&item.FirstSeen,
			&item.LastSeen,
			&item.RunCount,
			&item.TotalCostUSD,
			&item.TotalTokens,
			&item.AvgLatencyMs,
			&item.RunErrorRate,
			&item.DecisionCount,
			&item.PolicyBlockCount,
			&item.PolicyRedactionCount,
			&item.BudgetDeniedCount,
			&item.FallbackCount,
			&item.EvalCount,
			&item.EvalAverageScore,
			&item.RecentEvalAverage,
			&item.LatestEvalScore,
			&item.RolloutCount,
			&item.RolloutErrorRate,
			&item.AutoPauseCount,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
