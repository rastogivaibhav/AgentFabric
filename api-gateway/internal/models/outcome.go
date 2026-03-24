package models

import (
	"net/http"
	"strings"
)

const (
	OutcomeStatusOK       = "ok"
	OutcomeStatusBlocked  = "blocked"
	OutcomeStatusError    = "error"
	OutcomeStatusDegraded = "degraded"
)

func NormalizeOutcomeStatus(statusCode int, attrs map[string]string) string {
	if attrs == nil {
		attrs = map[string]string{}
	}
	if explicit := strings.ToLower(strings.TrimSpace(attrs["af.outcome_status"])); explicit != "" {
		switch explicit {
		case OutcomeStatusOK, OutcomeStatusBlocked, OutcomeStatusError, OutcomeStatusDegraded:
			return explicit
		}
	}

	if isTruthyOutcomeAttr(attrs["af.policy.blocked"]) || strings.EqualFold(strings.TrimSpace(attrs["af.policy.decision"]), "deny") {
		return OutcomeStatusBlocked
	}
	switch statusCode {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests:
		return OutcomeStatusBlocked
	}
	if statusCode == 2 || statusCode >= http.StatusInternalServerError {
		return OutcomeStatusError
	}
	if strings.EqualFold(strings.TrimSpace(attrs["af.gateway.route_source"]), "fallback") {
		return OutcomeStatusDegraded
	}
	return OutcomeStatusOK
}

func OutcomeStatusCountsAsFailure(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case OutcomeStatusBlocked, OutcomeStatusError, OutcomeStatusDegraded:
		return true
	default:
		return false
	}
}

func isTruthyOutcomeAttr(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "blocked":
		return true
	default:
		return false
	}
}
