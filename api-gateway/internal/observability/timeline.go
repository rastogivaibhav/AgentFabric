package observability

import (
	"fmt"
	"sort"
	"time"

	"github.com/agentfabric/api-gateway/internal/models"
)

func BuildTimeline(traceID string, spans []models.Span, policyEvents []models.PolicyEvent) *models.TraceTimeline {
	if len(spans) == 0 {
		return &models.TraceTimeline{
			TraceID: traceID,
			Items:   []models.TraceTimelineItem{},
		}
	}

	enriched := spans
	if len(policyEvents) > 0 {
		enriched = EnrichSpans(spans, policyEvents)
	}

	minStart := enriched[0].StartTimeNs
	maxEnd := enriched[0].StartTimeNs + enriched[0].DurationNs
	policyCounts := make(map[string]int, len(policyEvents))
	policyIDs := make([]string, 0, len(policyEvents))
	for _, event := range policyEvents {
		if event.SpanID != "" {
			policyCounts[event.SpanID]++
		}
		if event.DecisionID != "" {
			policyIDs = append(policyIDs, event.DecisionID)
		}
	}

	items := make([]models.TraceTimelineItem, 0, len(enriched))
	highlights := make([]string, 0, 4)
	var longest *models.Span
	for _, span := range enriched {
		if span.StartTimeNs < minStart {
			minStart = span.StartTimeNs
		}
		if end := span.StartTimeNs + span.DurationNs; end > maxEnd {
			maxEnd = end
		}
		if longest == nil || span.DurationNs > longest.DurationNs {
			copySpan := span
			longest = &copySpan
		}
		status := span.OutcomeStatus
		if status == "" {
			status = models.NormalizeOutcomeStatus(span.StatusCode, span.Attributes)
		}
		totalTokens := span.InputTokens + span.OutputTokens + span.CacheReadTokens + span.CacheWriteTokens + span.ReasoningTokens
		items = append(items, models.TraceTimelineItem{
			SpanID:           span.ID,
			ParentSpanID:     span.ParentID,
			Name:             span.Name,
			StepType:         span.StepType,
			Provider:         span.Provider,
			Model:            span.Model,
			AppName:          span.AppName,
			Environment:      span.Environment,
			Status:           status,
			FailureSummary:   span.FailureSummary,
			Blocked:          span.Blocked,
			BlockedReason:    span.BlockedReason,
			RedactionCount:   span.RedactionCount,
			Depth:            span.Depth,
			Lineage:          append([]string(nil), span.Lineage...),
			StartOffsetNs:    span.StartTimeNs - minStart,
			EndOffsetNs:      span.StartTimeNs - minStart + span.DurationNs,
			DurationNs:       span.DurationNs,
			TotalTokens:      totalTokens,
			CostUSD:          span.CostUSD,
			PolicyEventCount: policyCounts[span.ID],
		})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].StartOffsetNs == items[j].StartOffsetNs {
			return items[i].DurationNs > items[j].DurationNs
		}
		return items[i].StartOffsetNs < items[j].StartOffsetNs
	})

	if longest != nil {
		highlights = append(highlights, fmt.Sprintf("Longest step: %s (%s)", longest.Name, formatTimelineDuration(longest.DurationNs)))
	}
	for _, span := range enriched {
		if span.Blocked {
			highlights = append(highlights, fmt.Sprintf("Blocked step: %s", span.Name))
			break
		}
	}
	for _, span := range enriched {
		if models.OutcomeStatusCountsAsFailure(span.OutcomeStatus) || span.FailureSummary != "" {
			if span.FailureSummary != "" {
				highlights = append(highlights, fmt.Sprintf("Failure: %s", span.FailureSummary))
			} else {
				highlights = append(highlights, fmt.Sprintf("Errored step: %s", span.Name))
			}
			break
		}
	}

	return &models.TraceTimeline{
		TraceID:        traceID,
		StartTime:      time.Unix(0, minStart),
		DurationNs:     maxEnd - minStart,
		Items:          items,
		Highlights:     highlights,
		PolicyEventIDs: policyIDs,
	}
}

func formatTimelineDuration(ns int64) string {
	switch {
	case ns < 1_000_000:
		return fmt.Sprintf("%dus", ns/1_000)
	case ns < 1_000_000_000:
		return fmt.Sprintf("%.1fms", float64(ns)/1_000_000)
	default:
		return fmt.Sprintf("%.2fs", float64(ns)/1_000_000_000)
	}
}
