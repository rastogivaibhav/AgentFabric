package evals

import (
	"testing"

	"github.com/agentfabric/api-gateway/internal/models"
)

func TestDefaultScorers(t *testing.T) {
	if got := len(defaultScorers()); got != 4 {
		t.Fatalf("expected 4 scorers, got %d", got)
	}
}

func TestScoreReliability(t *testing.T) {
	trace := models.Trace{
		ErrorCount: 1,
		Status:     "partial",
		Insights: models.TraceInsights{
			FailedSpans: 2,
			RetryCount: 1,
		},
	}

	score := scoreReliability(trace)
	if score.Metric != "reliability" {
		t.Fatalf("unexpected metric %q", score.Metric)
	}
	if score.Score >= 100 {
		t.Fatalf("expected reliability penalty, got %.2f", score.Score)
	}
}

func TestScoreLatencyThresholds(t *testing.T) {
	cases := []struct {
		name     string
		duration int64
		want     float64
	}{
		{"fast", 500 * 1_000_000, 100},
		{"moderate", 1500 * 1_000_000, 86},
		{"slow", 6000 * 1_000_000, 55},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			score := scoreLatency(models.Trace{Duration: tc.duration})
			if score.Score != tc.want {
				t.Fatalf("expected %.2f, got %.2f", tc.want, score.Score)
			}
		})
	}
}

func TestScoreCostEfficiency(t *testing.T) {
	trace := models.Trace{
		TotalTokens:  1000,
		TotalCostUSD: 0.5,
		Insights: models.TraceInsights{
			RetryCount: 2,
		},
	}
	score := scoreCostEfficiency(trace)
	if score.Metric != "cost_efficiency" {
		t.Fatalf("unexpected metric %q", score.Metric)
	}
	if score.Score <= 0 || score.Score > 100 {
		t.Fatalf("unexpected score %.2f", score.Score)
	}
}

func TestScorePolicyCoverage(t *testing.T) {
	trace := models.Trace{
		Insights: models.TraceInsights{
			LLMCalls: 1,
		},
		Spans: []models.Span{{ID: "llm-1", StepType: "llm"}},
		PolicyEvents: []models.PolicyEvent{
			{SpanID: "llm-1", Result: "deny"},
		},
	}

	score := scorePolicyCoverage(trace)
	if score.Score <= 0 {
		t.Fatalf("expected positive score, got %.2f", score.Score)
	}
	if score.Severity == "" {
		t.Fatalf("expected severity to be set")
	}
}

func TestClampAndSeverityHelpers(t *testing.T) {
	if clampScore(-1) != 0 {
		t.Fatalf("expected clamp to floor at 0")
	}
	if clampScore(101) != 100 {
		t.Fatalf("expected clamp to cap at 100")
	}
	if severityForScore(90) != "low" || severityForScore(70) != "medium" || severityForScore(10) != "high" {
		t.Fatalf("unexpected severity mapping")
	}
}
