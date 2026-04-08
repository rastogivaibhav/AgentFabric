package evals

import (
	"testing"

	"github.com/govagn/api-gateway/internal/models"
)

func TestAggregateMetrics(t *testing.T) {
	runs := []models.TraceEvalRun{
		{Scores: []models.TraceEvalScore{{Metric: "latency", Score: 80}, {Metric: "reliability", Score: 90}}},
		{Scores: []models.TraceEvalScore{{Metric: "latency", Score: 60}, {Metric: "reliability", Score: 70}}},
	}

	got := aggregateMetrics(runs)
	if got["latency"] != 70 || got["reliability"] != 80 {
		t.Fatalf("unexpected aggregate metrics: %#v", got)
	}
}

func TestMetricUnionAndHelpers(t *testing.T) {
	union := metricUnion(map[string]float64{"b": 1, "a": 1}, map[string]float64{"c": 1, "a": 2})
	if len(union) != 3 || union[0] != "a" || union[2] != "c" {
		t.Fatalf("unexpected union: %#v", union)
	}
	if regressionSeverity(-12) != "high" || regressionSeverity(-5) != "medium" || regressionSeverity(0) != "low" {
		t.Fatalf("unexpected regression severity mapping")
	}
	if minInt(1, 2) != 1 || minInt(5, 2) != 2 {
		t.Fatalf("unexpected minInt result")
	}
}
