package policy

import (
	"regexp"
	"strings"

	"github.com/agentfabric/api-gateway/internal/models"
)

var unsafeCategoryPatterns = map[string]*regexp.Regexp{
	"violence":  regexp.MustCompile(`(?i)(build a bomb|kill someone|weaponize|violent attack)`),
	"self_harm": regexp.MustCompile(`(?i)(suicide plan|self-harm|overdose intentionally)`),
	"hate":      regexp.MustCompile(`(?i)(hate speech|ethnic cleansing|racial superiority)`),
	"sexual":    regexp.MustCompile(`(?i)(explicit sexual|pornographic content|sexual exploitation)`),
	"malware":   regexp.MustCompile(`(?i)(ransomware|keylogger|steal passwords|exploit payload)`),
}

func evaluateUnsafeContentGuard(rule models.PolicyRule, input EvaluationInput) ([]string, []ConditionTrace, bool) {
	if !hasGuardrail(rule, "unsafe_content") {
		return nil, nil, false
	}
	enabledCategories := rule.UnsafeCategories
	if len(enabledCategories) == 0 {
		for category := range unsafeCategoryPatterns {
			enabledCategories = append(enabledCategories, category)
		}
	}
	body := strings.ToLower(string(decisionBody(input)))
	traces := make([]ConditionTrace, 0, len(enabledCategories))
	matched := false
	matches := []string{}
	for _, category := range enabledCategories {
		pattern, ok := unsafeCategoryPatterns[strings.ToLower(strings.TrimSpace(category))]
		if !ok {
			continue
		}
		found := pattern.MatchString(body)
		traces = append(traces, ConditionTrace{
			Field:    "guard.unsafe_content." + category,
			Operator: "match",
			Expected: category,
			Actual:   boolString(found),
			Matched:  !found,
			Source:   "guard.unsafe_content",
		})
		if found {
			matched = true
			matches = append(matches, category)
		}
	}
	return matches, traces, matched
}

func hasGuardrail(rule models.PolicyRule, target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	for _, guard := range rule.Guardrails {
		if strings.ToLower(strings.TrimSpace(guard)) == target {
			return true
		}
	}
	return false
}
