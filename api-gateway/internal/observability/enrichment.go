package observability

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/govagn/api-gateway/internal/models"
)

func BuildTrace(traceID string, spans []models.Span, policyEvents []models.PolicyEvent) models.Trace {
	enrichedSpans := EnrichSpans(spans, policyEvents)
	trace := models.Trace{
		ID:           traceID,
		Spans:        enrichedSpans,
		PolicyEvents: policyEvents,
	}
	if len(enrichedSpans) == 0 {
		return trace
	}

	trace.Framework = enrichedSpans[0].Framework
	trace.RootSpanName = enrichedSpans[0].Name
	trace.StartTime = time.Unix(0, enrichedSpans[0].StartTimeNs)
	trace.Insights = models.TraceInsights{
		StepTypes:     map[string]int{},
		ErrorClasses:  map[string]int{},
		PolicyResults: map[string]int{},
	}

	modelsSeen := map[string]struct{}{}
	providersSeen := map[string]struct{}{}
	appsSeen := map[string]struct{}{}
	envsSeen := map[string]struct{}{}
	workflow := make([]string, 0, len(enrichedSpans))
	var maxEnd int64

	for _, span := range enrichedSpans {
		end := span.StartTimeNs + span.DurationNs
		if end > maxEnd {
			maxEnd = end
		}
		trace.TotalCostUSD += span.CostUSD
		trace.TotalTokens += span.InputTokens + span.OutputTokens + span.CacheReadTokens + span.CacheWriteTokens + span.ReasoningTokens
		if span.Model != "" {
			modelsSeen[span.Model] = struct{}{}
		}
		if span.Provider != "" {
			providersSeen[span.Provider] = struct{}{}
		}
		if span.AppName != "" {
			appsSeen[span.AppName] = struct{}{}
		}
		if span.Environment != "" {
			envsSeen[span.Environment] = struct{}{}
		}
		if span.StepType != "" {
			trace.Insights.StepTypes[span.StepType]++
		}
		if span.ErrorClass != "" {
			trace.Insights.ErrorClasses[span.ErrorClass]++
		}
		if span.StepType == "llm" {
			trace.Insights.LLMCalls++
		}
		if span.StepType == "tool" {
			trace.Insights.ToolCalls++
		}
		if span.Blocked {
			trace.Insights.BlockedSpans++
		}
		if span.RedactionCount > 0 {
			trace.Insights.RedactedSpans++
		}
		if models.OutcomeStatusCountsAsFailure(span.OutcomeStatus) || span.ErrorClass != "" {
			trace.Insights.FailedSpans++
		}
		trace.Insights.RetryCount += span.RetryCount
		if span.Depth > trace.Insights.MaxDepth {
			trace.Insights.MaxDepth = span.Depth
		}
		if span.OutcomeStatus == models.OutcomeStatusError {
			trace.ErrorCount++
		}
		if len(workflow) < 8 {
			workflow = append(workflow, summarizeWorkflowSpan(span))
		}
	}

	for _, event := range policyEvents {
		if result := strings.TrimSpace(event.Result); result != "" {
			trace.Insights.PolicyResults[result]++
		}
	}

	trace.Insights.Models = sortedSet(modelsSeen)
	trace.Insights.Providers = sortedSet(providersSeen)
	trace.Insights.Apps = sortedSet(appsSeen)
	trace.Insights.Environments = sortedSet(envsSeen)
	trace.Insights.WorkflowSummary = workflow
	trace.Duration = maxEnd - enrichedSpans[0].StartTimeNs
	trace.SpanCount = len(enrichedSpans)
	trace.Timeline = BuildTimeline(traceID, enrichedSpans, policyEvents)
	if trace.ErrorCount > 0 {
		trace.Status = "error"
	} else if trace.Insights.BlockedSpans > 0 || trace.Insights.FailedSpans > 0 {
		trace.Status = "partial"
	} else {
		trace.Status = "ok"
	}
	return trace
}

func EnrichSpans(spans []models.Span, policyEvents []models.PolicyEvent) []models.Span {
	if len(spans) == 0 {
		return nil
	}
	out := make([]models.Span, len(spans))
	copy(out, spans)

	lineage := buildLineage(out)
	policyBySpan := make(map[string][]models.PolicyEvent)
	for _, event := range policyEvents {
		if strings.TrimSpace(event.SpanID) == "" {
			continue
		}
		policyBySpan[event.SpanID] = append(policyBySpan[event.SpanID], event)
	}

	for i := range out {
		enrichSpan(&out[i], lineage[out[i].ID], policyBySpan[out[i].ID])
	}
	return out
}

func enrichSpan(span *models.Span, lineage lineageMeta, policyEvents []models.PolicyEvent) {
	if span == nil {
		return
	}
	if span.Attributes == nil {
		span.Attributes = map[string]string{}
	}
	span.Depth = lineage.Depth
	span.ParentName = lineage.ParentName
	if len(lineage.Path) > 0 {
		span.Lineage = lineage.Path
	}
	span.Provider = firstNonEmpty(span.Attributes["gen_ai.system"], span.Attributes["proxy.provider"], span.Attributes["netproxy.provider"])
	span.Model = firstNonEmpty(span.Attributes["gen_ai.request.model"], span.Attributes["proxy.model"], span.Attributes["netproxy.model"])
	span.StepType = firstNonEmpty(span.Attributes["af.span.step_type"], inferStepType(span.Name, span.Provider, span.Model))
	span.AppName = firstNonEmpty(span.Attributes["af.app.name"], span.Attributes["service.name"], span.Attributes["application.name"])
	span.Environment = firstNonEmpty(span.Attributes["af.environment"], span.Attributes["deployment.environment"], span.Attributes["environment"], span.Attributes["env"])
	span.UserID = firstNonEmpty(span.Attributes["af.user.id"], span.Attributes["enduser.id"], span.Attributes["user.id"])
	span.SessionID = firstNonEmpty(span.Attributes["af.session.id"], span.Attributes["session.id"])
	span.PromptID = firstNonEmpty(span.Attributes["af.prompt.id"])
	span.PromptVersion = firstInt(span.Attributes, "af.prompt.version")
	span.PromptReleaseTag = firstNonEmpty(span.Attributes["af.prompt.release_tag"])
	span.PromptEnvironment = firstNonEmpty(span.Attributes["af.prompt.environment"], span.Environment)
	span.OutcomeStatus = models.NormalizeOutcomeStatus(span.StatusCode, span.Attributes)
	span.ErrorClass = classifyError(*span)
	span.PromptPreview = firstPreview(span.Attributes, "af.preview.prompt", "gen_ai.prompt", "input.value", "prompt", "llm.prompt")
	span.ResponsePreview = firstPreview(span.Attributes, "af.preview.response", "gen_ai.response", "output.value", "response", "llm.response")
	span.RetryCount = firstInt(span.Attributes, "af.retry.count", "retry.count", "http.retry_count")
	span.BlockedReason = firstNonEmpty(span.Attributes["af.policy.reason"], span.Attributes["policy.reason"], span.Attributes["budget.reason"])
	span.Blocked = span.OutcomeStatus == models.OutcomeStatusBlocked || isTrue(span.Attributes["af.policy.blocked"]) || strings.EqualFold(span.Attributes["af.policy.decision"], "deny") || span.BlockedReason != ""
	span.PricingRuleID = firstInt64(span.Attributes, "af.pricing.rule_id")
	span.PricingScope = span.Attributes["af.pricing.scope"]
	span.PricingModelPattern = span.Attributes["af.pricing.model_pattern"]
	if span.CacheReadTokens == 0 {
		span.CacheReadTokens = firstInt64(span.Attributes, "gen_ai.usage.cache_read_tokens")
	}
	if span.CacheWriteTokens == 0 {
		span.CacheWriteTokens = firstInt64(span.Attributes, "gen_ai.usage.cache_write_tokens")
	}
	if span.ReasoningTokens == 0 {
		span.ReasoningTokens = firstInt64(span.Attributes, "gen_ai.usage.reasoning_tokens")
	}
	span.RedactionCount = firstInt(span.Attributes, "af.policy.redactions", "policy.redactions")
	span.PolicyDecisionCount = len(policyEvents)
	if len(policyEvents) > 0 {
		span.PolicyDecisionSummary = make([]string, 0, len(policyEvents))
		for _, event := range policyEvents {
			label := strings.TrimSpace(event.PolicyName)
			if label == "" {
				label = "policy"
			}
			result := strings.TrimSpace(event.Result)
			if result != "" {
				label += ": " + result
			}
			if event.Redactions > 0 {
				label += " (" + strconv.Itoa(event.Redactions) + " redactions)"
			}
			span.PolicyDecisionSummary = append(span.PolicyDecisionSummary, label)
			if event.Redactions > span.RedactionCount {
				span.RedactionCount = event.Redactions
			}
			if !span.Blocked && strings.EqualFold(event.Result, "deny") {
				span.Blocked = true
			}
			if span.BlockedReason == "" {
				span.BlockedReason = strings.TrimSpace(event.Reason)
			}
		}
	}
	span.FailureSummary = failureSummary(*span)
}

func summarizeWorkflowSpan(span models.Span) string {
	parts := []string{span.Name}
	if span.StepType != "" {
		parts = append(parts, span.StepType)
	}
	if span.Model != "" {
		parts = append(parts, span.Model)
	} else if span.Provider != "" {
		parts = append(parts, span.Provider)
	}
	if span.Blocked {
		parts = append(parts, "blocked")
	} else if span.RedactionCount > 0 {
		parts = append(parts, "redacted")
	} else if span.ErrorClass != "" {
		parts = append(parts, span.ErrorClass)
	}
	return strings.Join(parts, " | ")
}

func inferStepType(name, provider, model string) string {
	lowerName := strings.ToLower(name)
	switch {
	case provider != "" || model != "":
		return "llm"
	case strings.Contains(lowerName, "tool"), strings.Contains(lowerName, "function"):
		return "tool"
	case strings.Contains(lowerName, "policy"), strings.Contains(lowerName, "guard"):
		return "policy"
	case strings.Contains(lowerName, "retry"):
		return "retry"
	default:
		return "agent"
	}
}

func firstPreview(attrs map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(attrs[key]); value != "" {
			if len(value) > 220 {
				return value[:220] + "..."
			}
			return value
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func firstInt(attrs map[string]string, keys ...string) int {
	for _, key := range keys {
		if value, err := strconv.Atoi(strings.TrimSpace(attrs[key])); err == nil {
			return value
		}
	}
	return 0
}

func firstInt64(attrs map[string]string, keys ...string) int64 {
	for _, key := range keys {
		if value, err := strconv.ParseInt(strings.TrimSpace(attrs[key]), 10, 64); err == nil {
			return value
		}
	}
	return 0
}

func isTrue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "blocked":
		return true
	default:
		return false
	}
}

func sortedSet(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
