package evals

import (
	"fmt"
	"math"

	"github.com/govagn/api-gateway/internal/models"
)

type scorer struct {
	metric string
	weight float64
	run    func(models.Trace) models.TraceEvalScore
}

func defaultScorers() []scorer {
	return []scorer{
		{metric: "reliability", weight: 0.35, run: scoreReliability},
		{metric: "latency", weight: 0.20, run: scoreLatency},
		{metric: "cost_efficiency", weight: 0.20, run: scoreCostEfficiency},
		{metric: "policy_coverage", weight: 0.25, run: scorePolicyCoverage},
	}
}

func scoreReliability(trace models.Trace) models.TraceEvalScore {
	score := 100.0
	score -= float64(trace.ErrorCount) * 12
	score -= float64(trace.Insights.FailedSpans) * 4
	score -= float64(trace.Insights.RetryCount) * 2
	if trace.Status == "partial" {
		score -= 10
	}
	if trace.Status == "error" {
		score -= 20
	}
	score = clampScore(score)
	return models.TraceEvalScore{
		Metric:   "reliability",
		Score:    score,
		Weight:   0.35,
		Severity: severityForScore(score),
		Summary:  fmt.Sprintf("%d errors, %d failed spans, %d retries", trace.ErrorCount, trace.Insights.FailedSpans, trace.Insights.RetryCount),
	}
}

func scoreLatency(trace models.Trace) models.TraceEvalScore {
	durationMs := float64(trace.Duration) / 1_000_000
	score := 100.0
	switch {
	case durationMs > 10_000:
		score = 35
	case durationMs > 5_000:
		score = 55
	case durationMs > 2_500:
		score = 72
	case durationMs > 1_000:
		score = 86
	}
	return models.TraceEvalScore{
		Metric:   "latency",
		Score:    clampScore(score),
		Weight:   0.20,
		Severity: severityForScore(score),
		Summary:  fmt.Sprintf("trace duration %.1fms", durationMs),
	}
}

func scoreCostEfficiency(trace models.Trace) models.TraceEvalScore {
	totalTokens := float64(trace.TotalTokens)
	if totalTokens <= 0 {
		return models.TraceEvalScore{
			Metric:   "cost_efficiency",
			Score:    60,
			Weight:   0.20,
			Severity: "medium",
			Summary:  "trace recorded no token usage; cost efficiency is uncertain",
		}
	}
	costPerThousand := (trace.TotalCostUSD / math.Max(totalTokens, 1)) * 1000
	score := 100 - (costPerThousand * 120)
	if trace.Insights.RetryCount > 0 {
		score -= float64(trace.Insights.RetryCount * 3)
	}
	score = clampScore(score)
	return models.TraceEvalScore{
		Metric:   "cost_efficiency",
		Score:    score,
		Weight:   0.20,
		Severity: severityForScore(score),
		Summary:  fmt.Sprintf("$%.4f per 1K tokens", costPerThousand),
	}
}

func scorePolicyCoverage(trace models.Trace) models.TraceEvalScore {
	summary := summarizePolicyEffectiveness(trace)
	score := summary.CoverageRatio * 100
	if summary.TotalEvents == 0 {
		score = 35
	}
	if summary.Denies+summary.Redacts > 0 {
		score += 10
	}
	score = clampScore(score)
	return models.TraceEvalScore{
		Metric:   "policy_coverage",
		Score:    score,
		Weight:   0.25,
		Severity: severityForScore(score),
		Summary:  fmt.Sprintf("%d/%d LLM calls covered by policy events", summary.CoveredLLMCalls, summary.TotalLLMCalls),
	}
}

func severityForScore(score float64) string {
	switch {
	case score >= 85:
		return "low"
	case score >= 65:
		return "medium"
	default:
		return "high"
	}
}

func clampScore(score float64) float64 {
	switch {
	case score < 0:
		return 0
	case score > 100:
		return 100
	default:
		return math.Round(score*100) / 100
	}
}
