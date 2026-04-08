package policy

import (
	"strconv"
	"strings"

	"github.com/govagn/api-gateway/internal/models"
)

func evaluateTrafficRules(rules []compiledRule, input EvaluationInput) Decision {
	best := Decision{}
	bestScore := -1
	for _, rule := range rules {
		if rule.normalized.RuleType != "traffic" || rule.decisionMode != "fast" {
			continue
		}
		score, traces, ok := matchTrafficRule(rule, input)
		if !ok || score <= bestScore {
			continue
		}
		bestScore = score
		best = buildDecision(rule, input, "fast-path", traces)
	}
	if best.Matched {
		return best
	}
	adapter := &regoAdapter{}
	for _, rule := range rules {
		if rule.normalized.RuleType != "traffic" || rule.decisionMode != "rego" {
			continue
		}
		if decision, ok := adapter.Evaluate(rule, input); ok {
			return decision
		}
	}
	return Decision{}
}

func evaluateDLPRules(rules []compiledRule, input EvaluationInput, findings []Finding) Decision {
	best := Decision{}
	bestScore := -1
	for _, rule := range rules {
		if rule.normalized.RuleType != "dlp" || rule.decisionMode != "fast" {
			continue
		}
		score, matchedNames, traces, ok := matchDLPRule(rule, input, findings)
		if !ok || score <= bestScore {
			continue
		}
		bestScore = score
		best = buildDecision(rule, input, "fast-path", traces)
		best.MatchedNames = matchedNames
		best.GuardrailMatches = append([]string(nil), filterGuardrailMatches(matchedNames)...)
		best.Explanation.GuardrailMatches = append([]string(nil), best.GuardrailMatches...)
	}
	if best.Matched {
		return best
	}
	adapter := &regoAdapter{}
	for _, rule := range rules {
		if rule.normalized.RuleType != "dlp" || rule.decisionMode != "rego" {
			continue
		}
		if decision, ok := adapter.Evaluate(rule, input); ok {
			decision.MatchedNames = append([]string(nil), namesFromFindings(findings)...)
			return decision
		}
	}
	return Decision{}
}

func buildDecision(rule compiledRule, input EvaluationInput, engine string, traces []ConditionTrace) Decision {
	explanation := DecisionExplanation{
		Engine:         engine,
		DecisionMode:   rule.decisionMode,
		Version:        rule.normalized.Version,
		RolloutPercent: rule.normalized.RolloutPercent,
		EvaluationPath: []string{engine, rule.normalized.RuleType, rule.normalized.Name},
		ConditionTrace: traces,
		RuleConditions: cloneConditions(rule.ruleConditions),
	}
	for _, trace := range traces {
		if trace.Matched {
			explanation.MatchedFields = append(explanation.MatchedFields, trace.Field)
		}
	}
	explanation.Explain = explainDecision(rule.normalized, explanation)
	return Decision{
		Matched:     true,
		Final:       true,
		RuleID:      rule.normalized.ID,
		PolicyName:  rule.normalized.Name,
		Action:      rule.normalized.Action,
		Reason:      decisionReason(rule.normalized, input),
		Scope:       decisionScope(rule.normalized, input.Scope),
		Explanation: explanation,
	}
}

func matchTrafficRule(rule compiledRule, input EvaluationInput) (int, []ConditionTrace, bool) {
	normalized := rule.normalized
	traces := []ConditionTrace{
		tenantTrace(normalized, input),
		valueTrace("provider", normalized.Provider, input.Provider, "rule.provider"),
		patternTrace("model", normalized.ModelPattern, input.Model, "rule.model_pattern"),
		valueTrace("environment", normalized.Environment, input.Environment, "rule.environment"),
		rolloutTrace(normalized, input),
	}
	for _, trace := range evaluateExtraConditions(rule.ruleConditions, input) {
		traces = append(traces, trace)
	}
	for _, trace := range traces {
		if !trace.Matched {
			return 0, traces, false
		}
	}
	if normalized.MaxTokens > 0 {
		tokenTrace := ConditionTrace{
			Field:    "estimated_tokens",
			Operator: ">",
			Expected: int64String(normalized.MaxTokens),
			Actual:   int64String(input.EstimatedTokens),
			Matched:  input.EstimatedTokens > normalized.MaxTokens,
			Source:   "rule.max_tokens",
		}
		traces = append(traces, tokenTrace)
		if !tokenTrace.Matched {
			return 0, traces, false
		}
	}

	score := normalized.Priority * 100
	if normalized.TenantID != nil {
		score += 100000
	}
	if exactValue(normalized.Provider, input.Provider) {
		score += 1000
	}
	if exactValue(normalized.ModelPattern, input.Model) {
		score += 500
	} else if strings.TrimSpace(normalized.ModelPattern) != "" && normalized.ModelPattern != "*" {
		score += len(normalized.ModelPattern)
	}
	if exactValue(normalized.Environment, input.Environment) {
		score += 250
	}
	if normalized.MaxTokens > 0 {
		score += 100
	}
	score += len(rule.ruleConditions) * 10
	return score, traces, true
}

func matchDLPRule(rule compiledRule, input EvaluationInput, findings []Finding) (int, []string, []ConditionTrace, bool) {
	normalized := rule.normalized
	traces := []ConditionTrace{
		tenantTrace(normalized, input),
		valueTrace("provider", normalized.Provider, input.Provider, "rule.provider"),
		patternTrace("model", normalized.ModelPattern, input.Model, "rule.model_pattern"),
		valueTrace("environment", normalized.Environment, input.Environment, "rule.environment"),
		scopeTrace(normalized.Scope, input.Scope),
		rolloutTrace(normalized, input),
	}
	for _, trace := range evaluateExtraConditions(rule.ruleConditions, input) {
		traces = append(traces, trace)
	}
	for _, trace := range traces {
		if !trace.Matched {
			return 0, nil, traces, false
		}
	}

	matchedNames := make([]string, 0, len(findings))
	target := strings.ToLower(strings.TrimSpace(normalized.Detector))
	for _, finding := range findings {
		if target == "" || target == "*" || target == finding.Name || target == finding.Category {
			matchedNames = append(matchedNames, finding.Name)
		}
	}
	traces = append(traces, ConditionTrace{
		Field:    "detector",
		Operator: "match",
		Expected: target,
		Actual:   strings.Join(matchedNames, ","),
		Matched:  len(matchedNames) > 0,
		Source:   "rule.detector",
	})
	guardrailMatches := []string{}
	guardrailTraces := []ConditionTrace{}
	guardrailMatched := false
	if matches, schemaTraces, schemaTriggered := evaluateSchemaGuard(normalized, input); len(schemaTraces) > 0 {
		guardrailTraces = append(guardrailTraces, schemaTraces...)
		if schemaTriggered {
			guardrailMatches = append(guardrailMatches, matches...)
			guardrailMatched = true
		}
	}
	if matches, injectionTraces, injectionTriggered := evaluateInjectionGuard(normalized, input); len(injectionTraces) > 0 {
		guardrailTraces = append(guardrailTraces, injectionTraces...)
		if injectionTriggered {
			guardrailMatches = append(guardrailMatches, matches...)
			guardrailMatched = true
		}
	}
	if matches, unsafeTraces, unsafeTriggered := evaluateUnsafeContentGuard(normalized, input); len(unsafeTraces) > 0 {
		guardrailTraces = append(guardrailTraces, unsafeTraces...)
		if unsafeTriggered {
			guardrailMatches = append(guardrailMatches, matches...)
			guardrailMatched = true
		}
	}
	traces = append(traces, guardrailTraces...)
	if len(matchedNames) == 0 && !guardrailMatched {
		return 0, nil, traces, false
	}
	matchedNames = append(matchedNames, guardrailMatches...)

	score := normalized.Priority * 100
	if normalized.TenantID != nil {
		score += 100000
	}
	if exactValue(normalized.Provider, input.Provider) {
		score += 1000
	}
	if exactValue(normalized.ModelPattern, input.Model) {
		score += 500
	} else if strings.TrimSpace(normalized.ModelPattern) != "" && normalized.ModelPattern != "*" {
		score += len(normalized.ModelPattern)
	}
	if exactValue(normalized.Environment, input.Environment) {
		score += 250
	}
	if strings.TrimSpace(normalized.Detector) != "" && normalized.Detector != "*" {
		score += 100
	}
	score += len(rule.ruleConditions) * 10
	return score, matchedNames, traces, true
}

func evaluateExtraConditions(conditions map[string]string, input EvaluationInput) []ConditionTrace {
	keys := sortedConditionKeys(conditions)
	traces := make([]ConditionTrace, 0, len(keys))
	for _, key := range keys {
		expected := conditions[key]
		trace := ConditionTrace{
			Field:    key,
			Operator: "==",
			Expected: expected,
			Source:   "rule_conditions",
		}
		switch {
		case key == "app":
			trace.Actual = input.App
			trace.Matched = matchesValue(expected, input.App)
		case key == "session":
			trace.Actual = input.Session
			trace.Matched = matchesValue(expected, input.Session)
		case key == "actor":
			trace.Actual = input.Actor
			trace.Matched = matchesValue(expected, input.Actor)
		case key == "scope":
			trace.Actual = input.Scope
			trace.Matched = matchesValue(expected, input.Scope)
		case strings.HasPrefix(key, "header:"):
			headerName := strings.TrimPrefix(key, "header:")
			trace.Field = "header." + headerName
			trace.Actual = input.RequestHeaders[strings.ToLower(headerName)]
			trace.Matched = matchesValue(expected, trace.Actual)
		case key == "request_contains":
			trace.Operator = "contains"
			trace.Actual = string(input.RequestBody)
			trace.Matched = strings.Contains(strings.ToLower(trace.Actual), strings.ToLower(expected))
		case key == "response_contains":
			trace.Operator = "contains"
			trace.Actual = string(input.ResponseBody)
			trace.Matched = strings.Contains(strings.ToLower(trace.Actual), strings.ToLower(expected))
		case key == "guard":
			trace.Operator = "includes"
			trace.Actual = strings.Join(inputGuardrails(input), ",")
			trace.Matched = strings.Contains(strings.ToLower(trace.Actual), strings.ToLower(expected))
		}
		traces = append(traces, trace)
	}
	return traces
}

func inputGuardrails(input EvaluationInput) []string {
	guards := []string{}
	if len(input.RequestBody) > 0 {
		guards = append(guards, "request_body")
	}
	if len(input.ResponseBody) > 0 {
		guards = append(guards, "response_body")
	}
	return guards
}

func tenantTrace(rule models.PolicyRule, input EvaluationInput) ConditionTrace {
	expected := ""
	if rule.TenantID != nil {
		expected = strings.TrimSpace(*rule.TenantID)
	}
	matched := expected == "" || expected == strings.TrimSpace(input.TenantID)
	return ConditionTrace{
		Field:    "tenant_id",
		Operator: "==",
		Expected: expected,
		Actual:   strings.TrimSpace(input.TenantID),
		Matched:  matched,
		Source:   "rule.tenant_id",
	}
}

func valueTrace(field, expected, actual, source string) ConditionTrace {
	return ConditionTrace{
		Field:    field,
		Operator: "==",
		Expected: expected,
		Actual:   actual,
		Matched:  matchesValue(expected, actual),
		Source:   source,
	}
}

func patternTrace(field, expected, actual, source string) ConditionTrace {
	return ConditionTrace{
		Field:    field,
		Operator: "prefix",
		Expected: expected,
		Actual:   actual,
		Matched:  matchesPattern(expected, actual),
		Source:   source,
	}
}

func scopeTrace(expected, actual string) ConditionTrace {
	return ConditionTrace{
		Field:    "scope",
		Operator: "match",
		Expected: expected,
		Actual:   actual,
		Matched:  scopeMatches(expected, actual),
		Source:   "rule.scope",
	}
}

func cloneConditions(conditions map[string]string) map[string]string {
	if len(conditions) == 0 {
		return nil
	}
	out := make(map[string]string, len(conditions))
	for key, value := range conditions {
		out[key] = value
	}
	return out
}

func namesFromFindings(findings []Finding) []string {
	out := make([]string, 0, len(findings))
	for _, finding := range findings {
		out = append(out, finding.Name)
	}
	return out
}

func filterGuardrailMatches(matches []string) []string {
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		switch match {
		case "schema", "prompt_injection", "tool_escape", "violence", "self_harm", "hate", "sexual", "malware":
			out = append(out, match)
		}
	}
	return out
}

func decisionReason(rule models.PolicyRule, input EvaluationInput) string {
	if strings.TrimSpace(rule.Description) != "" {
		return strings.TrimSpace(rule.Description)
	}
	if rule.RuleType == "traffic" && rule.MaxTokens > 0 {
		return "estimated tokens exceeded policy limit"
	}
	if rule.RuleType == "dlp" {
		return "sensitive content detected"
	}
	if rule.ModelPattern != "" && rule.ModelPattern != "*" {
		return "provider/model policy matched"
	}
	return "policy matched"
}

func decisionScope(rule models.PolicyRule, fallback string) string {
	if rule.RuleType == "traffic" {
		return "request"
	}
	if strings.TrimSpace(rule.Scope) != "" && rule.Scope != "both" {
		return rule.Scope
	}
	if strings.TrimSpace(fallback) != "" {
		return fallback
	}
	return "request"
}

func int64String(value int64) string {
	return strconv.FormatInt(value, 10)
}
