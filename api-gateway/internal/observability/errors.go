package observability

import (
	"strings"

	"github.com/govagn/api-gateway/internal/models"
)

func classifyError(span models.Span) string {
	attrs := span.Attributes
	if attrs == nil {
		attrs = map[string]string{}
	}
	for _, key := range []string{"af.error.class", "error.type", "exception.type"} {
		if value := strings.TrimSpace(attrs[key]); value != "" {
			return value
		}
	}
	switch {
	case strings.EqualFold(attrs["af.error.type"], "budget"):
		return "budget_limit"
	case strings.EqualFold(attrs["af.error.type"], "policy"):
		return "policy_denied"
	case strings.EqualFold(attrs["af.error.type"], "upstream"):
		return "upstream_error"
	case span.OutcomeStatus == models.OutcomeStatusError && strings.TrimSpace(span.StatusMsg) != "":
		return "runtime_error"
	default:
		return ""
	}
}

func failureSummary(span models.Span) string {
	if span.Blocked {
		if span.BlockedReason != "" {
			return span.BlockedReason
		}
		return "request blocked by policy"
	}
	if strings.TrimSpace(span.StatusMsg) != "" {
		return strings.TrimSpace(span.StatusMsg)
	}
	switch span.ErrorClass {
	case "budget_limit":
		return "monthly budget exceeded"
	case "policy_denied":
		return "request denied by policy"
	case "upstream_error":
		return "upstream provider request failed"
	default:
		return ""
	}
}
