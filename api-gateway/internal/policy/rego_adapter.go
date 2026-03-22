package policy

import (
	"strconv"
	"strings"
)

type regoAdapter struct{}

func (a *regoAdapter) Evaluate(rule compiledRule, input EvaluationInput) (Decision, bool) {
	module := strings.TrimSpace(rule.normalized.RegoModule)
	if module == "" {
		return Decision{}, false
	}

	query, conditions := parsePseudoRego(module)
	if query == "" || len(conditions) == 0 {
		return Decision{}, false
	}

	explanation := DecisionExplanation{
		Engine:         "rego-adapter",
		DecisionMode:   rule.decisionMode,
		EvaluationPath: []string{"rego-adapter", query},
		RegoQuery:      query,
		RuleConditions: cloneConditions(rule.ruleConditions),
	}

	for _, condition := range conditions {
		trace := evaluateCondition(condition, input)
		explanation.ConditionTrace = append(explanation.ConditionTrace, trace)
		if trace.Matched {
			explanation.MatchedFields = append(explanation.MatchedFields, trace.Field)
			continue
		}
		explanation.Explain = "rego-style expression did not match all conditions"
		return Decision{}, false
	}

	explanation.Explain = "rego-style expression matched all conditions"
	return Decision{
		Matched:     true,
		Final:       true,
		RuleID:      rule.normalized.ID,
		PolicyName:  rule.normalized.Name,
		Action:      rule.normalized.Action,
		Reason:      decisionReason(rule.normalized, input),
		Scope:       decisionScope(rule.normalized, input.Scope),
		Explanation: explanation,
	}, true
}

func parsePseudoRego(module string) (string, []string) {
	cleaned := strings.TrimSpace(module)
	if cleaned == "" {
		return "", nil
	}
	lines := strings.Split(cleaned, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lower := strings.ToLower(line)
		if !(strings.HasPrefix(lower, "allow if ") || strings.HasPrefix(lower, "deny if ") || strings.HasPrefix(lower, "warn if ") || strings.HasPrefix(lower, "redact if ")) {
			continue
		}
		query := line
		parts := strings.SplitN(line, " if ", 2)
		if len(parts) != 2 {
			return line, nil
		}
		rawConditions := strings.Split(parts[1], "&&")
		conditions := make([]string, 0, len(rawConditions))
		for _, condition := range rawConditions {
			if trimmed := strings.TrimSpace(condition); trimmed != "" {
				conditions = append(conditions, trimmed)
			}
		}
		return query, conditions
	}
	return cleaned, nil
}

func evaluateCondition(condition string, input EvaluationInput) ConditionTrace {
	condition = strings.TrimSpace(condition)
	if strings.Contains(condition, " contains ") {
		parts := strings.SplitN(condition, " contains ", 2)
		field := strings.TrimSpace(parts[0])
		expected := unquoteConditionValue(parts[1])
		actual := lookupConditionValue(field, input)
		return ConditionTrace{
			Field:    field,
			Operator: "contains",
			Expected: expected,
			Actual:   actual,
			Matched:  strings.Contains(strings.ToLower(actual), strings.ToLower(expected)),
			Source:   condition,
		}
	}
	if strings.Contains(condition, " >= ") {
		parts := strings.SplitN(condition, " >= ", 2)
		field := strings.TrimSpace(parts[0])
		expected := unquoteConditionValue(parts[1])
		actual := lookupConditionValue(field, input)
		return numericConditionTrace(field, ">=", expected, actual, condition)
	}
	if strings.Contains(condition, " <= ") {
		parts := strings.SplitN(condition, " <= ", 2)
		field := strings.TrimSpace(parts[0])
		expected := unquoteConditionValue(parts[1])
		actual := lookupConditionValue(field, input)
		return numericConditionTrace(field, "<=", expected, actual, condition)
	}
	if strings.Contains(condition, " > ") {
		parts := strings.SplitN(condition, " > ", 2)
		field := strings.TrimSpace(parts[0])
		expected := unquoteConditionValue(parts[1])
		actual := lookupConditionValue(field, input)
		return numericConditionTrace(field, ">", expected, actual, condition)
	}
	if strings.Contains(condition, " < ") {
		parts := strings.SplitN(condition, " < ", 2)
		field := strings.TrimSpace(parts[0])
		expected := unquoteConditionValue(parts[1])
		actual := lookupConditionValue(field, input)
		return numericConditionTrace(field, "<", expected, actual, condition)
	}
	if strings.Contains(condition, " != ") {
		parts := strings.SplitN(condition, " != ", 2)
		field := strings.TrimSpace(parts[0])
		expected := unquoteConditionValue(parts[1])
		actual := lookupConditionValue(field, input)
		return ConditionTrace{
			Field:    field,
			Operator: "!=",
			Expected: expected,
			Actual:   actual,
			Matched:  !strings.EqualFold(actual, expected),
			Source:   condition,
		}
	}

	parts := strings.SplitN(condition, " == ", 2)
	field := strings.TrimSpace(parts[0])
	expected := ""
	if len(parts) == 2 {
		expected = unquoteConditionValue(parts[1])
	}
	actual := lookupConditionValue(field, input)
	return ConditionTrace{
		Field:    field,
		Operator: "==",
		Expected: expected,
		Actual:   actual,
		Matched:  strings.EqualFold(actual, expected),
		Source:   condition,
	}
}

func numericConditionTrace(field, operator, expected, actual, source string) ConditionTrace {
	expectedFloat, expectedErr := strconv.ParseFloat(expected, 64)
	actualFloat, actualErr := strconv.ParseFloat(strings.TrimSpace(actual), 64)
	matched := false
	if expectedErr == nil && actualErr == nil {
		switch operator {
		case ">":
			matched = actualFloat > expectedFloat
		case "<":
			matched = actualFloat < expectedFloat
		case ">=":
			matched = actualFloat >= expectedFloat
		case "<=":
			matched = actualFloat <= expectedFloat
		}
	}
	return ConditionTrace{
		Field:    field,
		Operator: operator,
		Expected: expected,
		Actual:   actual,
		Matched:  matched,
		Source:   source,
	}
}

func lookupConditionValue(field string, input EvaluationInput) string {
	field = strings.TrimSpace(strings.TrimPrefix(field, "input."))
	switch field {
	case "tenant", "tenant_id":
		return input.TenantID
	case "environment", "env":
		return input.Environment
	case "provider":
		return input.Provider
	case "model":
		return input.Model
	case "scope":
		return input.Scope
	case "estimated_tokens":
		return strconv.FormatInt(input.EstimatedTokens, 10)
	case "actor":
		return input.Actor
	case "app":
		return input.App
	case "session":
		return input.Session
	case "request_body":
		return string(input.RequestBody)
	case "response_body":
		return string(input.ResponseBody)
	default:
		if strings.HasPrefix(field, "header.") {
			return input.RequestHeaders[strings.TrimPrefix(field, "header.")]
		}
		return ""
	}
}

func unquoteConditionValue(value string) string {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.Trim(trimmed, `"`)
	trimmed = strings.Trim(trimmed, `'`)
	return trimmed
}
