package models

import (
	"fmt"
	"strings"
	"time"
)

const (
	DecisionTypePolicy   = "policy"
	DecisionTypeBudget   = "budget"
	DecisionTypeFallback = "fallback"
	DecisionTypeRouting  = "routing"
	DecisionTypeRetry    = "retry"
)

type DecisionRecord struct {
	ID               int64                  `json:"id"`
	DecisionID       string                 `json:"decision_id"`
	TenantID         string                 `json:"tenant_id,omitempty"`
	TraceID          string                 `json:"trace_id,omitempty"`
	SpanID           string                 `json:"span_id,omitempty"`
	Type             string                 `json:"type"`
	Result           string                 `json:"result"`
	Reason           string                 `json:"reason,omitempty"`
	Explanation      string                 `json:"explanation,omitempty"`
	Trigger          string                 `json:"trigger,omitempty"`
	Inputs           map[string]string      `json:"inputs,omitempty"`
	Evidence         map[string]interface{} `json:"evidence,omitempty"`
	ActionTaken      string                 `json:"action_taken,omitempty"`
	Source           string                 `json:"source,omitempty"`
	Framework        string                 `json:"framework,omitempty"`
	AppName          string                 `json:"app_name,omitempty"`
	Environment      string                 `json:"environment,omitempty"`
	Provider         string                 `json:"provider,omitempty"`
	Model            string                 `json:"model,omitempty"`
	PromptID         string                 `json:"prompt_id,omitempty"`
	PromptReleaseTag string                 `json:"prompt_release_tag,omitempty"`
	CreatedAt        time.Time              `json:"created_at"`
}

type DecisionQuery struct {
	TenantID string
	TraceID  string
	Type     string
	Result   string
	Limit    int
	Offset   int
}

func NormalizeDecisionResult(result string) string {
	switch strings.ToLower(strings.TrimSpace(result)) {
	case "block":
		return "deny"
	case "redact":
		return "sanitize"
	default:
		return strings.ToLower(strings.TrimSpace(result))
	}
}

func DefaultDecisionExplanation(record DecisionRecord) string {
	if strings.TrimSpace(record.Explanation) != "" {
		return strings.TrimSpace(record.Explanation)
	}

	target := strings.TrimSpace(record.Model)
	if target == "" {
		target = strings.TrimSpace(record.Provider)
	}
	if target == "" {
		target = "the request"
	}

	reason := strings.TrimSpace(record.Reason)
	switch record.Type {
	case DecisionTypePolicy:
		switch NormalizeDecisionResult(record.Result) {
		case "deny":
			if reason != "" {
				return fmt.Sprintf("Policy blocked %s because %s.", target, reason)
			}
			return fmt.Sprintf("Policy blocked %s.", target)
		case "sanitize":
			if reason != "" {
				return fmt.Sprintf("Policy redacted sensitive content for %s because %s.", target, reason)
			}
			return fmt.Sprintf("Policy redacted sensitive content for %s.", target)
		case "warn":
			if reason != "" {
				return fmt.Sprintf("Policy warned on %s because %s.", target, reason)
			}
			return fmt.Sprintf("Policy raised a warning for %s.", target)
		default:
			if reason != "" {
				return fmt.Sprintf("Policy evaluated %s because %s.", target, reason)
			}
			return fmt.Sprintf("Policy evaluated %s.", target)
		}
	case DecisionTypeBudget:
		if reason != "" {
			return fmt.Sprintf("Budget control %s because %s.", NormalizeDecisionResult(record.Result), reason)
		}
		return "Budget control evaluated current usage."
	case DecisionTypeFallback:
		if reason != "" {
			return fmt.Sprintf("Gateway switched execution path because %s.", reason)
		}
		return "Gateway switched execution path to a fallback route."
	case DecisionTypeRouting:
		if reason != "" {
			return fmt.Sprintf("Gateway routing changed because %s.", reason)
		}
		return "Gateway selected a non-default route."
	case DecisionTypeRetry:
		if reason != "" {
			return fmt.Sprintf("Gateway retried execution because %s.", reason)
		}
		return "Gateway retried execution after an earlier failure."
	default:
		if reason != "" {
			return reason
		}
		return "Decision evidence recorded."
	}
}
