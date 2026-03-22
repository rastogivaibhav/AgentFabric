package policy

import (
	"regexp"
	"strings"

	"github.com/agentfabric/api-gateway/internal/models"
)

var injectionPatterns = []struct {
	name string
	re   *regexp.Regexp
}{
	{name: "prompt_injection", re: regexp.MustCompile(`(?i)(ignore (all|previous) instructions|system prompt|developer message|bypass guardrails|jailbreak|reveal hidden prompt)`)},
	{name: "tool_escape", re: regexp.MustCompile(`(?i)(execute shell|run terminal command|disable safety|override policy)`)},
}

func evaluateInjectionGuard(rule models.PolicyRule, input EvaluationInput) ([]string, []ConditionTrace, bool) {
	if !hasGuardrail(rule, "prompt_injection") && !hasGuardrail(rule, "injection") {
		return nil, nil, false
	}
	body := strings.ToLower(string(decisionBody(input)))
	traces := make([]ConditionTrace, 0, len(injectionPatterns))
	matched := false
	matches := []string{}
	for _, pattern := range injectionPatterns {
		ok := pattern.re.MatchString(body)
		traces = append(traces, ConditionTrace{
			Field:    "guard." + pattern.name,
			Operator: "match",
			Expected: pattern.name,
			Actual:   boolString(ok),
			Matched:  !ok,
			Source:   "guard.prompt_injection",
		})
		if ok {
			matched = true
			matches = append(matches, pattern.name)
		}
	}
	return matches, traces, matched
}
