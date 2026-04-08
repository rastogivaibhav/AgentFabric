package policy

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/govagn/api-gateway/internal/models"
)

type simpleSchema struct {
	Required []string          `json:"required"`
	Types    map[string]string `json:"types"`
}

func evaluateSchemaGuard(rule models.PolicyRule, input EvaluationInput) ([]string, []ConditionTrace, bool) {
	if !hasGuardrail(rule, "schema") || strings.TrimSpace(rule.SchemaJSON) == "" {
		return nil, nil, false
	}
	body := decisionBody(input)
	traces := []ConditionTrace{}
	if len(body) == 0 {
		traces = append(traces, ConditionTrace{
			Field:    "schema.body",
			Operator: "present",
			Expected: "json body",
			Actual:   "",
			Matched:  false,
			Source:   "guard.schema",
		})
		return []string{"schema"}, traces, true
	}
	var schema simpleSchema
	if err := json.Unmarshal([]byte(rule.SchemaJSON), &schema); err != nil {
		traces = append(traces, ConditionTrace{
			Field:    "schema.definition",
			Operator: "parse",
			Expected: "valid schema json",
			Actual:   err.Error(),
			Matched:  false,
			Source:   "guard.schema",
		})
		return []string{"schema"}, traces, true
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		traces = append(traces, ConditionTrace{
			Field:    "schema.body",
			Operator: "parse",
			Expected: "valid json",
			Actual:   err.Error(),
			Matched:  false,
			Source:   "guard.schema",
		})
		return []string{"schema"}, traces, true
	}
	matched := false
	for _, field := range schema.Required {
		_, ok := payload[field]
		traces = append(traces, ConditionTrace{
			Field:    "schema.required." + field,
			Operator: "present",
			Expected: "true",
			Actual:   boolString(ok),
			Matched:  ok,
			Source:   "guard.schema",
		})
		if !ok {
			matched = true
		}
	}
	for field, expectedType := range schema.Types {
		actualType := typeName(payload[field])
		ok := actualType == "" || strings.EqualFold(actualType, expectedType)
		traces = append(traces, ConditionTrace{
			Field:    "schema.type." + field,
			Operator: "type",
			Expected: expectedType,
			Actual:   actualType,
			Matched:  ok,
			Source:   "guard.schema",
		})
		if !ok {
			matched = true
		}
	}
	return []string{"schema"}, traces, matched
}

func typeName(value any) string {
	switch value.(type) {
	case string:
		return "string"
	case float64, float32, int, int64, int32:
		return "number"
	case bool:
		return "boolean"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	case nil:
		return ""
	default:
		return fmt.Sprintf("%T", value)
	}
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
