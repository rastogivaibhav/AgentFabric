package observability

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/govagn/api-gateway/internal/models"
)

func CompareTraces(left, right models.Trace) models.TraceComparison {
	comparison := models.TraceComparison{
		Left:       buildComparisonSide(left),
		Right:      buildComparisonSide(right),
		Diffs:      []models.TraceComparisonDiff{},
		Highlights: []string{},
	}

	appendDiff := func(field, leftValue, rightValue, severity string) {
		if leftValue == rightValue {
			return
		}
		comparison.Diffs = append(comparison.Diffs, models.TraceComparisonDiff{
			Field:    field,
			Left:     leftValue,
			Right:    rightValue,
			Severity: severity,
		})
	}

	appendDiff("framework", comparison.Left.Framework, comparison.Right.Framework, "medium")
	appendDiff("status", comparison.Left.Status, comparison.Right.Status, "high")
	appendDiff("models", strings.Join(comparison.Left.Models, ", "), strings.Join(comparison.Right.Models, ", "), "medium")
	appendDiff("providers", strings.Join(comparison.Left.Providers, ", "), strings.Join(comparison.Right.Providers, ", "), "medium")
	appendDiff("workflow_summary", strings.Join(comparison.Left.WorkflowSummary, " -> "), strings.Join(comparison.Right.WorkflowSummary, " -> "), "low")
	appendDiff("blocked_spans", fmt.Sprintf("%d", comparison.Left.BlockedSpans), fmt.Sprintf("%d", comparison.Right.BlockedSpans), "high")
	appendDiff("failed_spans", fmt.Sprintf("%d", comparison.Left.FailedSpans), fmt.Sprintf("%d", comparison.Right.FailedSpans), "high")
	appendDiff("retry_count", fmt.Sprintf("%d", comparison.Left.RetryCount), fmt.Sprintf("%d", comparison.Right.RetryCount), "medium")
	appendDiff("span_count", fmt.Sprintf("%d", comparison.Left.SpanCount), fmt.Sprintf("%d", comparison.Right.SpanCount), "low")
	appendDiff("total_tokens", fmt.Sprintf("%d", comparison.Left.TotalTokens), fmt.Sprintf("%d", comparison.Right.TotalTokens), "medium")
	appendDiff("total_cost_usd", fmt.Sprintf("%.6f", comparison.Left.TotalCostUSD), fmt.Sprintf("%.6f", comparison.Right.TotalCostUSD), "medium")

	if left.TotalCostUSD > 0 || right.TotalCostUSD > 0 {
		delta := right.TotalCostUSD - left.TotalCostUSD
		if math.Abs(delta) > 0.000001 {
			comparison.Highlights = append(comparison.Highlights, fmt.Sprintf("Cost delta: %+.6f USD", delta))
		}
	}
	if left.TotalTokens > 0 || right.TotalTokens > 0 {
		comparison.Highlights = append(comparison.Highlights, fmt.Sprintf("Token delta: %+d", right.TotalTokens-left.TotalTokens))
	}
	if left.Insights.BlockedSpans != right.Insights.BlockedSpans {
		comparison.Highlights = append(comparison.Highlights, fmt.Sprintf("Blocked path changed from %d to %d spans", left.Insights.BlockedSpans, right.Insights.BlockedSpans))
	}

	sort.Slice(comparison.Diffs, func(i, j int) bool {
		return comparison.Diffs[i].Field < comparison.Diffs[j].Field
	})

	return comparison
}

func buildComparisonSide(trace models.Trace) models.TraceComparisonSide {
	side := models.TraceComparisonSide{
		TraceID:         trace.ID,
		RootSpanName:    trace.RootSpanName,
		Framework:       trace.Framework,
		StartTime:       trace.StartTime,
		DurationNs:      trace.Duration,
		Status:          trace.Status,
		SpanCount:       trace.SpanCount,
		ErrorCount:      trace.ErrorCount,
		TotalCostUSD:    trace.TotalCostUSD,
		TotalTokens:     trace.TotalTokens,
		RetryCount:      trace.Insights.RetryCount,
		BlockedSpans:    trace.Insights.BlockedSpans,
		RedactedSpans:   trace.Insights.RedactedSpans,
		FailedSpans:     trace.Insights.FailedSpans,
		WorkflowSummary: append([]string(nil), trace.Insights.WorkflowSummary...),
	}
	side.Models = append([]string(nil), trace.Insights.Models...)
	side.Providers = append([]string(nil), trace.Insights.Providers...)
	sort.Strings(side.Models)
	sort.Strings(side.Providers)
	return side
}
