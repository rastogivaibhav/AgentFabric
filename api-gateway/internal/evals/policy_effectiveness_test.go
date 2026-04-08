package evals

import (
	"testing"

	"github.com/govagn/api-gateway/internal/models"
)

func TestSummarizePolicyEffectiveness(t *testing.T) {
	trace := models.Trace{
		Insights: models.TraceInsights{
			BlockedSpans:  1,
			RedactedSpans: 2,
			LLMCalls:      2,
		},
		Spans: []models.Span{
			{ID: "llm-1", StepType: "llm"},
			{ID: "llm-2", StepType: "llm"},
			{ID: "tool-1", StepType: "tool"},
		},
		PolicyEvents: []models.PolicyEvent{
			{SpanID: "llm-1", Result: "allow"},
			{SpanID: "llm-2", Result: "deny"},
			{SpanID: "llm-2", Result: "redact"},
		},
	}

	got := summarizePolicyEffectiveness(trace)
	if got.Allows != 1 || got.Denies != 1 || got.Redacts != 1 {
		t.Fatalf("unexpected action counts: %#v", got)
	}
	if got.CoveredLLMCalls != 2 || got.TotalLLMCalls != 2 {
		t.Fatalf("unexpected coverage counts: %#v", got)
	}
	if got.CoverageRatio != 1 {
		t.Fatalf("expected coverage ratio 1, got %v", got.CoverageRatio)
	}
}
