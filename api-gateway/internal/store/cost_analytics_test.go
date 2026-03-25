package store

import (
	"testing"
	"time"
)

func TestBuildCostSpikeReport_RanksPositiveDeltas(t *testing.T) {
	query := CostReportQuery{Since: 24 * time.Hour, Limit: 5}
	current := []CostReportRow{
		{AppName: "support", Environment: "staging", Provider: "openai", Model: "gpt-4o", PromptID: "support.system", ReleaseTag: "candidate-7", TotalCost: 3.2, TraceCount: 18, TotalTokens: 1200},
		{AppName: "support", Environment: "prod", Provider: "openai", Model: "gpt-4o-mini", PromptID: "support.system", ReleaseTag: "stable-6", TotalCost: 0.8, TraceCount: 10, TotalTokens: 800},
	}
	previous := []CostReportRow{
		{AppName: "support", Environment: "staging", Provider: "openai", Model: "gpt-4o", PromptID: "support.system", ReleaseTag: "candidate-7", TotalCost: 0.7, TraceCount: 6, TotalTokens: 300},
		{AppName: "support", Environment: "prod", Provider: "openai", Model: "gpt-4o-mini", PromptID: "support.system", ReleaseTag: "stable-6", TotalCost: 1.0, TraceCount: 11, TotalTokens: 900},
	}

	report := buildCostSpikeReport(
		costWindow{start: time.Unix(0, 0), end: time.Unix(0, 1)},
		costWindow{start: time.Unix(0, 0), end: time.Unix(0, 1)},
		query,
		current,
		previous,
	)

	if len(report.Spikes) != 1 {
		t.Fatalf("expected 1 positive spike, got %d", len(report.Spikes))
	}
	if report.Spikes[0].ReleaseTag != "candidate-7" {
		t.Fatalf("expected candidate release to rank first, got %+v", report.Spikes[0])
	}
	if report.Spikes[0].DeltaCostUSD <= 0 {
		t.Fatalf("expected positive delta, got %f", report.Spikes[0].DeltaCostUSD)
	}
}

func TestBuildContributorGroups_AggregatesByDimension(t *testing.T) {
	current := []CostReportRow{
		{AppName: "support", Environment: "staging", Provider: "openai", Model: "gpt-4o", PromptID: "support.system", ReleaseTag: "candidate-7", TotalCost: 3.0},
		{AppName: "sales", Environment: "staging", Provider: "anthropic", Model: "claude-3-5-sonnet", PromptID: "sales.pitch", ReleaseTag: "sales-v3", TotalCost: 1.2},
	}
	previous := []CostReportRow{
		{AppName: "support", Environment: "staging", Provider: "openai", Model: "gpt-4o", PromptID: "support.system", ReleaseTag: "candidate-7", TotalCost: 1.0},
		{AppName: "sales", Environment: "staging", Provider: "anthropic", Model: "claude-3-5-sonnet", PromptID: "sales.pitch", ReleaseTag: "sales-v3", TotalCost: 0.8},
	}

	groups := buildContributorGroups(current, previous)
	if len(groups) != 6 {
		t.Fatalf("expected 6 contributor groups, got %d", len(groups))
	}

	releaseGroup := groups[5]
	if releaseGroup.Dimension != "release_tag" {
		t.Fatalf("expected release tag group, got %s", releaseGroup.Dimension)
	}
	if len(releaseGroup.Items) == 0 {
		t.Fatalf("expected contributor rows for release tags")
	}
	if releaseGroup.Items[0].DeltaCostUSD <= 0 {
		t.Fatalf("expected positive delta for top release contributor")
	}
}

func TestExplainCostSpike_HandlesNewSpend(t *testing.T) {
	current := CostReportRow{
		AppName:     "support",
		Environment: "staging",
		Provider:    "openai",
		Model:       "gpt-4o",
		PromptID:    "support.system",
		ReleaseTag:  "candidate-7",
		TotalCost:   2.5,
	}

	explanation := explainCostSpike(current, CostReportRow{})
	if explanation == "" {
		t.Fatal("expected non-empty explanation")
	}
	if explanation[:9] != "New spend" {
		t.Fatalf("expected new-spend wording, got %q", explanation)
	}
}

func TestNormalizeCostReportQuery_DefaultsAndCap(t *testing.T) {
	query := normalizeCostReportQuery(CostReportQuery{Limit: 1000})
	if query.Since != 24*time.Hour {
		t.Fatalf("expected default since to be 24h, got %s", query.Since)
	}
	if query.Limit != 250 {
		t.Fatalf("expected limit cap at 250, got %d", query.Limit)
	}
}
