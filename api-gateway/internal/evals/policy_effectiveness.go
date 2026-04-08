package evals

import "github.com/govagn/api-gateway/internal/models"

func summarizePolicyEffectiveness(trace models.Trace) models.PolicyEffectivenessSummary {
	summary := models.PolicyEffectivenessSummary{
		TotalEvents:     len(trace.PolicyEvents),
		BlockedSpans:    trace.Insights.BlockedSpans,
		RedactedSpans:   trace.Insights.RedactedSpans,
		TotalLLMCalls:   trace.Insights.LLMCalls,
		CoveredLLMCalls: 0,
	}

	covered := map[string]struct{}{}
	for _, event := range trace.PolicyEvents {
		switch event.Result {
		case "allow":
			summary.Allows++
		case "deny":
			summary.Denies++
			summary.PreventedFailures++
		case "warn":
			summary.Warns++
		case "redact", "sanitize":
			summary.Redacts++
		}
		if event.SpanID != "" {
			covered[event.SpanID] = struct{}{}
		}
	}

	for _, span := range trace.Spans {
		if span.StepType != "llm" {
			continue
		}
		if _, ok := covered[span.ID]; ok {
			summary.CoveredLLMCalls++
		}
	}
	if summary.TotalLLMCalls > 0 {
		summary.CoverageRatio = float64(summary.CoveredLLMCalls) / float64(summary.TotalLLMCalls)
	}
	return summary
}
