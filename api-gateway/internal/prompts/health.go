package prompts

import (
	"context"
	"fmt"
	"math"
	"sort"

	"github.com/govagn/api-gateway/internal/models"
)

const defaultEvalSuite = "core-release"

func attachReleaseHealth(ctx context.Context, store promptStore, tenantID string, releases []models.PromptRelease) error {
	if len(releases) == 0 {
		return nil
	}

	grouped := make(map[string][]*models.PromptRelease)
	for i := range releases {
		release := &releases[i]
		runs, err := store.ListEvalRunsByRelease(ctx, tenantID, release.PromptID, release.Environment, release.ReleaseTag, defaultEvalSuite)
		if err != nil {
			return err
		}
		release.EvalSummary = summarizeReleaseRuns(runs)
		grouped[releaseKey(release.PromptID, release.Environment)] = append(grouped[releaseKey(release.PromptID, release.Environment)], release)
	}

	for _, items := range grouped {
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].CreatedAt.Equal(items[j].CreatedAt) {
				return items[i].Version > items[j].Version
			}
			return items[i].CreatedAt.After(items[j].CreatedAt)
		})
		for idx := range items {
			if idx+1 >= len(items) {
				continue
			}
			items[idx].RegressionSummary = compareReleaseSummaries(items[idx], items[idx+1])
		}
	}

	return nil
}

func summarizeReleaseRuns(runs []models.TraceEvalRun) models.PromptReleaseEvalSummary {
	if len(runs) == 0 {
		return models.PromptReleaseEvalSummary{RiskLevel: "unknown"}
	}

	var total float64
	latest := runs[0]
	for _, run := range runs {
		total += run.OverallScore
		if run.CreatedAt.After(latest.CreatedAt) {
			latest = run
		}
	}
	average := math.Round((total/float64(len(runs)))*100) / 100
	latestScore := math.Round(latest.OverallScore*100) / 100
	lastEvaluatedAt := latest.CreatedAt
	return models.PromptReleaseEvalSummary{
		EvalCount:       len(runs),
		AverageScore:    average,
		LatestScore:     latestScore,
		RiskLevel:       latest.RiskLevel,
		LastEvaluatedAt: &lastEvaluatedAt,
	}
}

func compareReleaseSummaries(candidate, baseline *models.PromptRelease) *models.PromptReleaseRegressionSummary {
	if candidate == nil || baseline == nil {
		return nil
	}
	if candidate.EvalSummary.EvalCount == 0 || baseline.EvalSummary.EvalCount == 0 {
		return &models.PromptReleaseRegressionSummary{
			BaselineTag:  baseline.ReleaseTag,
			CandidateTag: candidate.ReleaseTag,
			RiskLevel:    "unknown",
			Summary:      "No overlapping eval evidence found for release comparison.",
		}
	}

	comparedRuns := candidate.EvalSummary.EvalCount
	if baseline.EvalSummary.EvalCount < comparedRuns {
		comparedRuns = baseline.EvalSummary.EvalCount
	}
	delta := math.Round((candidate.EvalSummary.AverageScore-baseline.EvalSummary.AverageScore)*100) / 100
	summary := fmt.Sprintf("Average eval score moved by %.2f points versus %s.", delta, baseline.ReleaseTag)
	highlights := []string{}
	switch {
	case delta <= -10:
		highlights = append(highlights, "Release regressed materially against the previous active baseline.")
	case delta >= 10:
		highlights = append(highlights, "Release improved materially against the previous active baseline.")
	}

	return &models.PromptReleaseRegressionSummary{
		BaselineTag:  baseline.ReleaseTag,
		CandidateTag: candidate.ReleaseTag,
		ComparedRuns: comparedRuns,
		OverallDelta: delta,
		RiskLevel:    regressionRisk(delta),
		Highlights:   highlights,
		Summary:      summary,
	}
}

func regressionRisk(delta float64) string {
	switch {
	case delta <= -10:
		return "high"
	case delta <= -3:
		return "medium"
	default:
		return "low"
	}
}
