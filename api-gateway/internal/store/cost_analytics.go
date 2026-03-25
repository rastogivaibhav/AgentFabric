package store

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	costAppExpr        = "COALESCE(NULLIF(attributes->>'af.app.name', ''), NULLIF(attributes->>'service.name', ''), 'unknown')"
	costEnvExpr        = "COALESCE(NULLIF(attributes->>'af.environment', ''), NULLIF(attributes->>'deployment.environment', ''), NULLIF(attributes->>'environment', ''), NULLIF(attributes->>'env', ''), 'unknown')"
	costProviderExpr   = "COALESCE(NULLIF(attributes->>'gen_ai.system', ''), 'unknown')"
	costModelExpr      = "COALESCE(NULLIF(attributes->>'gen_ai.request.model', ''), 'unknown')"
	costPromptIDExpr   = "COALESCE(NULLIF(attributes->>'af.prompt.id', ''), 'unknown')"
	costReleaseTagExpr = "COALESCE(NULLIF(attributes->>'af.prompt.release_tag', ''), 'unreleased')"
)

type CostReportQuery struct {
	Since       time.Duration
	AppName     string
	Environment string
	Provider    string
	Model       string
	PromptID    string
	ReleaseTag  string
	Limit       int
}

type CostReportRow struct {
	AppName           string  `json:"app_name"`
	Environment       string  `json:"environment"`
	Provider          string  `json:"provider"`
	Model             string  `json:"model"`
	PromptID          string  `json:"prompt_id"`
	ReleaseTag        string  `json:"release_tag"`
	TotalTokens       int64   `json:"total_tokens"`
	TotalCost         float64 `json:"total_cost_usd"`
	InputTokens       int64   `json:"input_tokens"`
	OutputTokens      int64   `json:"output_tokens"`
	CacheReadTokens   int64   `json:"cache_read_tokens"`
	CacheWriteTokens  int64   `json:"cache_write_tokens"`
	ReasoningTokens   int64   `json:"reasoning_tokens"`
	InputCostUSD      float64 `json:"input_cost_usd"`
	OutputCostUSD     float64 `json:"output_cost_usd"`
	CacheReadCostUSD  float64 `json:"cache_read_cost_usd"`
	CacheWriteCostUSD float64 `json:"cache_write_cost_usd"`
	ReasoningCostUSD  float64 `json:"reasoning_cost_usd"`
	TraceCount        int64   `json:"trace_count"`
	BlockedCount      int64   `json:"blocked_count"`
}

type CostSpikeRow struct {
	AppName             string  `json:"app_name"`
	Environment         string  `json:"environment"`
	Provider            string  `json:"provider"`
	Model               string  `json:"model"`
	PromptID            string  `json:"prompt_id"`
	ReleaseTag          string  `json:"release_tag"`
	CurrentCostUSD      float64 `json:"current_cost_usd"`
	PreviousCostUSD     float64 `json:"previous_cost_usd"`
	DeltaCostUSD        float64 `json:"delta_cost_usd"`
	DeltaPct            float64 `json:"delta_pct"`
	CurrentTraceCount   int64   `json:"current_trace_count"`
	PreviousTraceCount  int64   `json:"previous_trace_count"`
	CurrentTotalTokens  int64   `json:"current_total_tokens"`
	PreviousTotalTokens int64   `json:"previous_total_tokens"`
	Explanation         string  `json:"explanation"`
}

type CostContributorRow struct {
	Key             string  `json:"key"`
	CurrentCostUSD  float64 `json:"current_cost_usd"`
	PreviousCostUSD float64 `json:"previous_cost_usd"`
	DeltaCostUSD    float64 `json:"delta_cost_usd"`
	DeltaPct        float64 `json:"delta_pct"`
	ShareOfDelta    float64 `json:"share_of_delta"`
}

type CostContributorGroup struct {
	Dimension string               `json:"dimension"`
	Items     []CostContributorRow `json:"items"`
}

type CostSpikeReport struct {
	CurrentWindowStart  time.Time              `json:"current_window_start"`
	CurrentWindowEnd    time.Time              `json:"current_window_end"`
	PreviousWindowStart time.Time              `json:"previous_window_start"`
	PreviousWindowEnd   time.Time              `json:"previous_window_end"`
	FiltersApplied      map[string]string      `json:"filters_applied"`
	Spikes              []CostSpikeRow         `json:"spikes"`
	ContributorGroups   []CostContributorGroup `json:"contributor_groups"`
}

type costWindow struct {
	start time.Time
	end   time.Time
}

type costBreakdownKey struct {
	AppName     string
	Environment string
	Provider    string
	Model       string
	PromptID    string
	ReleaseTag  string
}

func normalizeCostReportQuery(q CostReportQuery) CostReportQuery {
	q.AppName = strings.TrimSpace(q.AppName)
	q.Environment = strings.TrimSpace(q.Environment)
	q.Provider = strings.TrimSpace(q.Provider)
	q.Model = strings.TrimSpace(q.Model)
	q.PromptID = strings.TrimSpace(q.PromptID)
	q.ReleaseTag = strings.TrimSpace(q.ReleaseTag)
	if q.Since <= 0 {
		q.Since = 24 * time.Hour
	}
	if q.Limit <= 0 {
		q.Limit = 100
	}
	if q.Limit > 250 {
		q.Limit = 250
	}
	return q
}

func (s *PostgresStore) GetCostReport(ctx context.Context, tenantID string, q CostReportQuery) ([]CostReportRow, error) {
	q = normalizeCostReportQuery(q)
	windowEnd := time.Now().UTC()
	windowStart := windowEnd.Add(-q.Since)
	return s.listCostBreakdownRows(ctx, tenantID, costWindow{start: windowStart, end: windowEnd}, q)
}

func (s *PostgresStore) GetCostSpikeReport(ctx context.Context, tenantID string, q CostReportQuery) (CostSpikeReport, error) {
	q = normalizeCostReportQuery(q)
	windowEnd := time.Now().UTC()
	currentWindow := costWindow{start: windowEnd.Add(-q.Since), end: windowEnd}
	previousWindow := costWindow{start: windowEnd.Add(-2 * q.Since), end: windowEnd.Add(-q.Since)}
	analysisQuery := q
	if analysisQuery.Limit < 200 {
		analysisQuery.Limit = 200
	}

	currentRows, err := s.listCostBreakdownRows(ctx, tenantID, currentWindow, analysisQuery)
	if err != nil {
		return CostSpikeReport{}, err
	}
	previousRows, err := s.listCostBreakdownRows(ctx, tenantID, previousWindow, analysisQuery)
	if err != nil {
		return CostSpikeReport{}, err
	}

	return buildCostSpikeReport(currentWindow, previousWindow, q, currentRows, previousRows), nil
}

func (s *PostgresStore) listCostBreakdownRows(ctx context.Context, tenantID string, window costWindow, q CostReportQuery) ([]CostReportRow, error) {
	query, args := buildCostBreakdownQuery(tenantID, window, q)
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []CostReportRow{}
	for rows.Next() {
		var row CostReportRow
		if err := rows.Scan(
			&row.AppName,
			&row.Environment,
			&row.Provider,
			&row.Model,
			&row.PromptID,
			&row.ReleaseTag,
			&row.TotalTokens,
			&row.TotalCost,
			&row.InputTokens,
			&row.OutputTokens,
			&row.CacheReadTokens,
			&row.CacheWriteTokens,
			&row.ReasoningTokens,
			&row.InputCostUSD,
			&row.OutputCostUSD,
			&row.CacheReadCostUSD,
			&row.CacheWriteCostUSD,
			&row.ReasoningCostUSD,
			&row.TraceCount,
			&row.BlockedCount,
		); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func buildCostBreakdownQuery(tenantID string, window costWindow, q CostReportQuery) (string, []any) {
	args := []any{tenantID, window.start.UnixNano(), window.end.UnixNano()}
	where := []string{
		"tenant_id = $1",
		"start_time_ns >= $2",
		"start_time_ns < $3",
		"cost_usd > 0",
	}
	addFilter := func(expr, value string) {
		if value == "" {
			return
		}
		args = append(args, strings.ToLower(value))
		where = append(where, fmt.Sprintf("LOWER(%s) = $%d", expr, len(args)))
	}

	addFilter(costAppExpr, q.AppName)
	addFilter(costEnvExpr, q.Environment)
	addFilter(costProviderExpr, q.Provider)
	addFilter(costModelExpr, q.Model)
	addFilter(costPromptIDExpr, q.PromptID)
	addFilter(costReleaseTagExpr, q.ReleaseTag)

	args = append(args, q.Limit)

	query := fmt.Sprintf(`
		SELECT
			%s AS app_name,
			%s AS environment,
			%s AS provider,
			%s AS model,
			%s AS prompt_id,
			%s AS release_tag,
			SUM(input_tokens + output_tokens + cache_read_tokens + cache_write_tokens + reasoning_tokens) AS total_tokens,
			SUM(cost_usd) AS total_cost,
			SUM(input_tokens) AS input_tokens,
			SUM(output_tokens) AS output_tokens,
			SUM(cache_read_tokens) AS cache_read_tokens,
			SUM(cache_write_tokens) AS cache_write_tokens,
			SUM(reasoning_tokens) AS reasoning_tokens,
			SUM(input_cost_usd) AS input_cost_usd,
			SUM(output_cost_usd) AS output_cost_usd,
			SUM(cache_read_cost_usd) AS cache_read_cost_usd,
			SUM(cache_write_cost_usd) AS cache_write_cost_usd,
			SUM(reasoning_cost_usd) AS reasoning_cost_usd,
			COUNT(DISTINCT trace_id) AS trace_count,
			SUM(CASE WHEN %s THEN 1 ELSE 0 END) AS blocked_count
		FROM spans
		WHERE %s
		GROUP BY app_name, environment, provider, model, prompt_id, release_tag
		ORDER BY total_cost DESC
		LIMIT $%d
	`,
		costAppExpr,
		costEnvExpr,
		costProviderExpr,
		costModelExpr,
		costPromptIDExpr,
		costReleaseTagExpr,
		spanOutcomeBlockedExpr(),
		strings.Join(where, " AND "),
		len(args),
	)

	return query, args
}

func buildCostSpikeReport(currentWindow, previousWindow costWindow, q CostReportQuery, currentRows, previousRows []CostReportRow) CostSpikeReport {
	currentByKey := map[costBreakdownKey]CostReportRow{}
	previousByKey := map[costBreakdownKey]CostReportRow{}
	keys := map[costBreakdownKey]struct{}{}

	for _, row := range currentRows {
		key := row.costBreakdownKey()
		currentByKey[key] = row
		keys[key] = struct{}{}
	}
	for _, row := range previousRows {
		key := row.costBreakdownKey()
		previousByKey[key] = row
		keys[key] = struct{}{}
	}

	spikes := make([]CostSpikeRow, 0, len(keys))
	for key := range keys {
		current := currentByKey[key]
		previous := previousByKey[key]
		delta := current.TotalCost - previous.TotalCost
		if delta <= 0 {
			continue
		}
		spikes = append(spikes, CostSpikeRow{
			AppName:             key.AppName,
			Environment:         key.Environment,
			Provider:            key.Provider,
			Model:               key.Model,
			PromptID:            key.PromptID,
			ReleaseTag:          key.ReleaseTag,
			CurrentCostUSD:      current.TotalCost,
			PreviousCostUSD:     previous.TotalCost,
			DeltaCostUSD:        delta,
			DeltaPct:            percentDelta(current.TotalCost, previous.TotalCost),
			CurrentTraceCount:   current.TraceCount,
			PreviousTraceCount:  previous.TraceCount,
			CurrentTotalTokens:  current.TotalTokens,
			PreviousTotalTokens: previous.TotalTokens,
			Explanation:         explainCostSpike(current, previous),
		})
	}

	sort.Slice(spikes, func(i, j int) bool {
		if spikes[i].DeltaCostUSD == spikes[j].DeltaCostUSD {
			return spikes[i].CurrentCostUSD > spikes[j].CurrentCostUSD
		}
		return spikes[i].DeltaCostUSD > spikes[j].DeltaCostUSD
	})
	if len(spikes) > q.Limit {
		spikes = spikes[:q.Limit]
	}

	return CostSpikeReport{
		CurrentWindowStart:  currentWindow.start,
		CurrentWindowEnd:    currentWindow.end,
		PreviousWindowStart: previousWindow.start,
		PreviousWindowEnd:   previousWindow.end,
		FiltersApplied:      filtersApplied(q),
		Spikes:              spikes,
		ContributorGroups:   buildContributorGroups(currentRows, previousRows),
	}
}

func buildContributorGroups(currentRows, previousRows []CostReportRow) []CostContributorGroup {
	type contributorSource struct {
		dimension string
		key       func(CostReportRow) string
	}

	sources := []contributorSource{
		{dimension: "app_name", key: func(row CostReportRow) string { return row.AppName }},
		{dimension: "environment", key: func(row CostReportRow) string { return row.Environment }},
		{dimension: "provider", key: func(row CostReportRow) string { return row.Provider }},
		{dimension: "model", key: func(row CostReportRow) string { return row.Model }},
		{dimension: "prompt_id", key: func(row CostReportRow) string { return row.PromptID }},
		{dimension: "release_tag", key: func(row CostReportRow) string { return row.ReleaseTag }},
	}

	groups := make([]CostContributorGroup, 0, len(sources))
	for _, source := range sources {
		currentTotals := aggregateContributorCost(currentRows, source.key)
		previousTotals := aggregateContributorCost(previousRows, source.key)
		keys := map[string]struct{}{}
		for key := range currentTotals {
			keys[key] = struct{}{}
		}
		for key := range previousTotals {
			keys[key] = struct{}{}
		}

		items := make([]CostContributorRow, 0, len(keys))
		totalPositiveDelta := 0.0
		for key := range keys {
			currentCost := currentTotals[key]
			previousCost := previousTotals[key]
			delta := currentCost - previousCost
			if delta <= 0 {
				continue
			}
			totalPositiveDelta += delta
			items = append(items, CostContributorRow{
				Key:             key,
				CurrentCostUSD:  currentCost,
				PreviousCostUSD: previousCost,
				DeltaCostUSD:    delta,
				DeltaPct:        percentDelta(currentCost, previousCost),
			})
		}

		sort.Slice(items, func(i, j int) bool {
			if items[i].DeltaCostUSD == items[j].DeltaCostUSD {
				return items[i].CurrentCostUSD > items[j].CurrentCostUSD
			}
			return items[i].DeltaCostUSD > items[j].DeltaCostUSD
		})
		if len(items) > 5 {
			items = items[:5]
		}
		for i := range items {
			if totalPositiveDelta > 0 {
				items[i].ShareOfDelta = items[i].DeltaCostUSD / totalPositiveDelta
			}
		}
		groups = append(groups, CostContributorGroup{
			Dimension: source.dimension,
			Items:     items,
		})
	}

	return groups
}

func aggregateContributorCost(rows []CostReportRow, keyFn func(CostReportRow) string) map[string]float64 {
	result := make(map[string]float64, len(rows))
	for _, row := range rows {
		key := keyFn(row)
		if strings.TrimSpace(key) == "" {
			key = "unknown"
		}
		result[key] += row.TotalCost
	}
	return result
}

func explainCostSpike(current, previous CostReportRow) string {
	scope := describeCostScope(current)
	delta := current.TotalCost - previous.TotalCost
	if previous.TotalCost <= 0 {
		return fmt.Sprintf("New spend surfaced for %s, adding $%.6f in the current window.", scope, current.TotalCost)
	}

	traceDelta := current.TraceCount - previous.TraceCount
	if traceDelta > 0 {
		return fmt.Sprintf(
			"Spend increased %.1f%% ($%.6f) for %s, with %d more traces than the prior window.",
			percentDelta(current.TotalCost, previous.TotalCost),
			delta,
			scope,
			traceDelta,
		)
	}

	return fmt.Sprintf(
		"Spend increased %.1f%% ($%.6f) for %s compared with the prior window.",
		percentDelta(current.TotalCost, previous.TotalCost),
		delta,
		scope,
	)
}

func describeCostScope(row CostReportRow) string {
	parts := []string{}
	if row.AppName != "" && row.AppName != "unknown" {
		parts = append(parts, "app "+row.AppName)
	}
	if row.Environment != "" && row.Environment != "unknown" {
		parts = append(parts, "env "+row.Environment)
	}
	if row.Provider != "" && row.Provider != "unknown" {
		parts = append(parts, "provider "+row.Provider)
	}
	if row.Model != "" && row.Model != "unknown" {
		parts = append(parts, "model "+row.Model)
	}
	if row.PromptID != "" && row.PromptID != "unknown" {
		parts = append(parts, "prompt "+row.PromptID)
	}
	if row.ReleaseTag != "" && row.ReleaseTag != "unreleased" {
		parts = append(parts, "release "+row.ReleaseTag)
	}
	if len(parts) == 0 {
		return "unknown workload"
	}
	return strings.Join(parts, ", ")
}

func percentDelta(current, previous float64) float64 {
	if previous <= 0 {
		if current <= 0 {
			return 0
		}
		return 100
	}
	return ((current - previous) / previous) * 100
}

func filtersApplied(q CostReportQuery) map[string]string {
	filters := map[string]string{
		"since": q.Since.String(),
	}
	if q.AppName != "" {
		filters["app_name"] = q.AppName
	}
	if q.Environment != "" {
		filters["environment"] = q.Environment
	}
	if q.Provider != "" {
		filters["provider"] = q.Provider
	}
	if q.Model != "" {
		filters["model"] = q.Model
	}
	if q.PromptID != "" {
		filters["prompt_id"] = q.PromptID
	}
	if q.ReleaseTag != "" {
		filters["release_tag"] = q.ReleaseTag
	}
	return filters
}

func (r CostReportRow) costBreakdownKey() costBreakdownKey {
	return costBreakdownKey{
		AppName:     r.AppName,
		Environment: r.Environment,
		Provider:    r.Provider,
		Model:       r.Model,
		PromptID:    r.PromptID,
		ReleaseTag:  r.ReleaseTag,
	}
}
