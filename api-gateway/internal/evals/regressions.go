package evals

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/agentfabric/api-gateway/internal/models"
)

func (s *Service) CompareRelease(ctx context.Context, tenantID string, req models.RegressionCompareRequest) (models.RegressionReport, error) {
	suite := strings.TrimSpace(req.EvalSuite)
	if suite == "" {
		suite = defaultSuite
	}
	baselineRuns, err := s.store.ListEvalRunsByRelease(ctx, tenantID, strings.TrimSpace(req.BaselineTag), suite)
	if err != nil {
		return models.RegressionReport{}, err
	}
	candidateRuns, err := s.store.ListEvalRunsByRelease(ctx, tenantID, strings.TrimSpace(req.CandidateTag), suite)
	if err != nil {
		return models.RegressionReport{}, err
	}
	report := models.RegressionReport{
		BaselineTag:  strings.TrimSpace(req.BaselineTag),
		CandidateTag: strings.TrimSpace(req.CandidateTag),
		EvalSuite:    suite,
		ComparedRuns: minInt(len(baselineRuns), len(candidateRuns)),
		GeneratedAt:  time.Now().UTC(),
	}

	baselineAgg := aggregateMetrics(baselineRuns)
	candidateAgg := aggregateMetrics(candidateRuns)
	metricNames := metricUnion(baselineAgg, candidateAgg)
	for _, metric := range metricNames {
		base := baselineAgg[metric]
		cand := candidateAgg[metric]
		delta := math.Round((cand-base)*100) / 100
		item := models.RegressionMetricDelta{
			Metric:         metric,
			BaselineScore:  base,
			CandidateScore: cand,
			Delta:          delta,
			Severity:       regressionSeverity(delta),
			Summary:        fmt.Sprintf("%s moved by %.2f points", metric, delta),
		}
		report.Metrics = append(report.Metrics, item)
		if delta < -8 {
			report.Highlights = append(report.Highlights, fmt.Sprintf("%s regressed materially", metric))
		} else if delta > 8 {
			report.Highlights = append(report.Highlights, fmt.Sprintf("%s improved materially", metric))
		}
		report.OverallDelta += delta
	}
	if len(report.Metrics) > 0 {
		report.OverallDelta = math.Round((report.OverallDelta/float64(len(report.Metrics)))*100) / 100
	}
	switch {
	case report.OverallDelta <= -10:
		report.RiskLevel = "high"
	case report.OverallDelta <= -3:
		report.RiskLevel = "medium"
	default:
		report.RiskLevel = "low"
	}
	if report.ComparedRuns == 0 {
		report.RiskLevel = "unknown"
		report.Highlights = append(report.Highlights, "No overlapping eval evidence found for the requested release tags")
	}
	return report, nil
}

func aggregateMetrics(runs []models.TraceEvalRun) map[string]float64 {
	sum := map[string]float64{}
	count := map[string]float64{}
	for _, run := range runs {
		for _, score := range run.Scores {
			sum[score.Metric] += score.Score
			count[score.Metric]++
		}
	}
	out := map[string]float64{}
	for metric, total := range sum {
		if count[metric] == 0 {
			continue
		}
		out[metric] = math.Round((total/count[metric])*100) / 100
	}
	return out
}

func metricUnion(left, right map[string]float64) []string {
	seen := map[string]struct{}{}
	for metric := range left {
		seen[metric] = struct{}{}
	}
	for metric := range right {
		seen[metric] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for metric := range seen {
		out = append(out, metric)
	}
	sort.Strings(out)
	return out
}

func regressionSeverity(delta float64) string {
	switch {
	case delta <= -10:
		return "high"
	case delta <= -3:
		return "medium"
	default:
		return "low"
	}
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
