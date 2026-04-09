package evals

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/govagn/api-gateway/internal/models"
)

type executionSubject struct {
	ItemRef    string
	ItemType   string
	TraceID    string
	DatasetRef string
	Input      map[string]any
	Expected   map[string]any
	Metadata   map[string]any
	Trace      *models.Trace
	Policy     models.PolicyEffectivenessSummary
	Lineage    promptLineage
}

type evalEvidence struct {
	Subject    executionSubject
	Attributes map[string]any
}

func (e evalEvidence) field(path string) any {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	switch {
	case strings.HasPrefix(path, "input."):
		return nestedLookup(e.Subject.Input, strings.TrimPrefix(path, "input."))
	case strings.HasPrefix(path, "expected."):
		return nestedLookup(e.Subject.Expected, strings.TrimPrefix(path, "expected."))
	case strings.HasPrefix(path, "metadata."):
		return nestedLookup(e.Subject.Metadata, strings.TrimPrefix(path, "metadata."))
	case strings.HasPrefix(path, "attributes."):
		return nestedLookup(e.Attributes, strings.TrimPrefix(path, "attributes."))
	case strings.HasPrefix(path, "trace."):
		return nestedLookup(traceEvidenceMap(e.Subject), strings.TrimPrefix(path, "trace."))
	case strings.HasPrefix(path, "policy."):
		return nestedLookup(policyEvidenceMap(e.Subject.Policy), strings.TrimPrefix(path, "policy."))
	}
	if value := nestedLookup(e.Subject.Input, path); value != nil {
		return value
	}
	if value := nestedLookup(e.Subject.Expected, path); value != nil {
		return value
	}
	if value := nestedLookup(e.Subject.Metadata, path); value != nil {
		return value
	}
	if value := nestedLookup(e.Attributes, path); value != nil {
		return value
	}
	if value := nestedLookup(traceEvidenceMap(e.Subject), path); value != nil {
		return value
	}
	if value := nestedLookup(policyEvidenceMap(e.Subject.Policy), path); value != nil {
		return value
	}
	return nil
}

func (e evalEvidence) fields(paths []string) map[string]any {
	out := make(map[string]any, len(paths))
	for _, path := range paths {
		if trimmed := strings.TrimSpace(path); trimmed != "" {
			out[trimmed] = e.field(trimmed)
		}
	}
	return out
}

func (e evalEvidence) evidenceLinks() []models.EvalEvidenceLink {
	links := []models.EvalEvidenceLink{}
	if e.Subject.TraceID != "" {
		links = append(links, models.EvalEvidenceLink{LinkType: "trace", RefID: e.Subject.TraceID, Label: "trace"})
	}
	if e.Subject.DatasetRef != "" {
		links = append(links, models.EvalEvidenceLink{LinkType: "dataset", RefID: e.Subject.DatasetRef, Label: e.Subject.ItemRef})
	}
	if strings.TrimSpace(e.Subject.Lineage.ReleaseTag) != "" {
		links = append(links, models.EvalEvidenceLink{LinkType: "prompt_release", RefID: e.Subject.Lineage.ReleaseTag, Label: e.Subject.Lineage.PromptID})
	}
	return links
}

func traceEvidenceMap(subject executionSubject) map[string]any {
	if subject.Trace == nil {
		return map[string]any{}
	}
	trace := subject.Trace
	prompt := ""
	response := ""
	toolTrace := []string{}
	for _, span := range trace.Spans {
		if prompt == "" && strings.TrimSpace(span.PromptPreview) != "" {
			prompt = strings.TrimSpace(span.PromptPreview)
		}
		if response == "" && strings.TrimSpace(span.ResponsePreview) != "" {
			response = strings.TrimSpace(span.ResponsePreview)
		}
		if span.StepType == "tool" || strings.Contains(strings.ToLower(span.Name), "tool") {
			toolTrace = append(toolTrace, span.Name)
		}
	}
	sort.Strings(toolTrace)
	workflow := append([]string{}, trace.Insights.WorkflowSummary...)
	if trace.Timeline != nil {
		for _, item := range trace.Timeline.Items {
			if len(workflow) >= 12 {
				break
			}
			if strings.TrimSpace(item.Name) != "" {
				workflow = append(workflow, strings.TrimSpace(item.Name))
			}
		}
	}
	workflow = uniqueStrings(workflow)
	return map[string]any{
		"trace_id":                   trace.ID,
		"status":                     trace.Status,
		"duration_ns":                trace.Duration,
		"latency_ms":                 float64(trace.Duration) / 1_000_000.0,
		"total_tokens":               trace.TotalTokens,
		"token_usage":                trace.TotalTokens,
		"total_cost_usd":             trace.TotalCostUSD,
		"error_count":                trace.ErrorCount,
		"span_count":                 trace.SpanCount,
		"step_count":                 trace.SpanCount,
		"llm_calls":                  trace.Insights.LLMCalls,
		"tool_calls":                 trace.Insights.ToolCalls,
		"retry_count":                trace.Insights.RetryCount,
		"max_depth":                  trace.Insights.MaxDepth,
		"blocked_spans":              trace.Insights.BlockedSpans,
		"redacted_spans":             trace.Insights.RedactedSpans,
		"failed_spans":               trace.Insights.FailedSpans,
		"prompt":                     prompt,
		"response":                   response,
		"tool_trace":                 toolTrace,
		"workflow_trace":             workflow,
		"action_trace":               workflow,
		"interaction_trace":          workflow,
		"policy_events_count":        len(trace.PolicyEvents),
		"escalated":                  trace.Insights.BlockedSpans > 0 || trace.Status == "partial",
		"confidence":                 1 - math.Min(float64(trace.ErrorCount+trace.Insights.RetryCount)/math.Max(float64(trace.SpanCount), 1), 1),
		"risk_level":                 severityForScore(compositeScore(*trace)),
		"goal":                       firstNonEmpty(prompt, trace.RootSpanName),
		"final_state":                trace.Status,
		"expected_budget":            1.0,
		"processing_metadata":        map[string]any{"prompt_id": subject.Lineage.PromptID, "prompt_environment": subject.Lineage.PromptEnvironment},
		"requested_fields":           collectRequestedFields(trace.Spans),
		"allowed_fields_for_purpose": collectRequestedFields(trace.Spans),
	}
}

func policyEvidenceMap(summary models.PolicyEffectivenessSummary) map[string]any {
	return map[string]any{
		"total_events":       summary.TotalEvents,
		"allows":             summary.Allows,
		"denies":             summary.Denies,
		"warns":              summary.Warns,
		"redacts":            summary.Redacts,
		"blocked_spans":      summary.BlockedSpans,
		"redacted_spans":     summary.RedactedSpans,
		"covered_llm_calls":  summary.CoveredLLMCalls,
		"total_llm_calls":    summary.TotalLLMCalls,
		"coverage_ratio":     summary.CoverageRatio,
		"prevented_failures": summary.PreventedFailures,
	}
}

func nestedLookup(root map[string]any, path string) any {
	if len(root) == 0 || strings.TrimSpace(path) == "" {
		return nil
	}
	parts := strings.Split(path, ".")
	var current any = root
	for _, part := range parts {
		switch typed := current.(type) {
		case map[string]any:
			current = typed[part]
		case map[string]string:
			value, ok := typed[part]
			if !ok {
				return nil
			}
			current = value
		default:
			return nil
		}
	}
	return current
}

func toStringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string{}, typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if trimmed := strings.TrimSpace(fmt.Sprint(item)); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		return []string{strings.TrimSpace(typed)}
	default:
		return nil
	}
}

func toFloat(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case uint64:
		return float64(typed), true
	case uint32:
		return float64(typed), true
	case string:
		if strings.TrimSpace(typed) == "" {
			return 0, false
		}
		var parsed float64
		if _, err := fmt.Sscanf(strings.TrimSpace(typed), "%f", &parsed); err == nil {
			return parsed, true
		}
	}
	return 0, false
}

func toBool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "1", "yes", "on", "passed", "approved", "present", "complete", "clear":
			return true
		}
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	}
	return false
}

func collectRequestedFields(spans []models.Span) []string {
	fields := []string{}
	for _, span := range spans {
		for key := range span.Attributes {
			if strings.HasPrefix(key, "af.input.") {
				fields = append(fields, strings.TrimPrefix(key, "af.input."))
			}
		}
	}
	return uniqueStrings(fields)
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			if _, ok := seen[trimmed]; ok {
				continue
			}
			seen[trimmed] = struct{}{}
			out = append(out, trimmed)
		}
	}
	return out
}
