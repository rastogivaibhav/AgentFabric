package policy

import (
	"hash/fnv"
	"net/http"
	"sort"
	"strings"

	"github.com/govagn/api-gateway/internal/models"
)

type EvaluationInput struct {
	TenantID        string
	Environment     string
	Provider        string
	Model           string
	Scope           string
	EstimatedTokens int64
	RequestHeaders  map[string]string
	RequestBody     []byte
	ResponseBody    []byte
	Actor           string
	App             string
	Session         string
	Attributes      map[string]any
}

type ConditionTrace = models.PolicyConditionTrace

type DecisionExplanation struct {
	Engine           string
	DecisionMode     string
	Version          int
	RolloutPercent   int
	EvaluationPath   []string
	MatchedFields    []string
	ConditionTrace   []ConditionTrace
	RegoQuery        string
	Explain          string
	RuleConditions   map[string]string
	GuardrailMatches []string
}

type compiledRule struct {
	model           models.PolicyRule
	normalized      models.PolicyRule
	decisionMode    string
	ruleConditions  map[string]string
	normalizedScope string
}

func normalizeEvaluationInput(input EvaluationInput) EvaluationInput {
	input.TenantID = strings.TrimSpace(input.TenantID)
	input.Environment = strings.ToLower(strings.TrimSpace(input.Environment))
	input.Provider = strings.ToLower(strings.TrimSpace(input.Provider))
	input.Model = strings.ToLower(strings.TrimSpace(input.Model))
	input.Scope = strings.ToLower(strings.TrimSpace(input.Scope))
	input.Actor = strings.TrimSpace(input.Actor)
	input.App = strings.TrimSpace(input.App)
	input.Session = strings.TrimSpace(input.Session)
	if input.RequestHeaders == nil {
		input.RequestHeaders = map[string]string{}
	}
	if input.Attributes == nil {
		input.Attributes = map[string]any{}
	}
	normalizedHeaders := make(map[string]string, len(input.RequestHeaders))
	for key, value := range input.RequestHeaders {
		normalizedHeaders[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
	}
	input.RequestHeaders = normalizedHeaders
	return input
}

func normalizeRule(rule models.PolicyRule) models.PolicyRule {
	rule.RuleType = strings.ToLower(strings.TrimSpace(rule.RuleType))
	rule.DecisionMode = strings.ToLower(strings.TrimSpace(rule.DecisionMode))
	if rule.DecisionMode == "" {
		rule.DecisionMode = "fast"
	}
	rule.Action = strings.ToLower(strings.TrimSpace(rule.Action))
	rule.Provider = strings.ToLower(strings.TrimSpace(rule.Provider))
	rule.ModelPattern = strings.ToLower(strings.TrimSpace(rule.ModelPattern))
	rule.Environment = strings.ToLower(strings.TrimSpace(rule.Environment))
	rule.Detector = strings.ToLower(strings.TrimSpace(rule.Detector))
	rule.Scope = strings.ToLower(strings.TrimSpace(rule.Scope))
	if rule.Scope == "" {
		rule.Scope = "both"
	}
	if rule.RolloutPercent <= 0 {
		rule.RolloutPercent = 100
	}
	if rule.Version <= 0 {
		rule.Version = 1
	}
	normalizedGuardrails := make([]string, 0, len(rule.Guardrails))
	for _, guard := range rule.Guardrails {
		if trimmed := strings.ToLower(strings.TrimSpace(guard)); trimmed != "" {
			normalizedGuardrails = append(normalizedGuardrails, trimmed)
		}
	}
	sort.Strings(normalizedGuardrails)
	rule.Guardrails = normalizedGuardrails
	rule.SchemaJSON = strings.TrimSpace(rule.SchemaJSON)
	normalizedUnsafe := make([]string, 0, len(rule.UnsafeCategories))
	for _, category := range rule.UnsafeCategories {
		if trimmed := strings.ToLower(strings.TrimSpace(category)); trimmed != "" {
			normalizedUnsafe = append(normalizedUnsafe, trimmed)
		}
	}
	sort.Strings(normalizedUnsafe)
	rule.UnsafeCategories = normalizedUnsafe
	if rule.RuleConditions == nil {
		rule.RuleConditions = map[string]string{}
	}
	normalizedConditions := make(map[string]string, len(rule.RuleConditions))
	keys := make([]string, 0, len(rule.RuleConditions))
	for key := range rule.RuleConditions {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		normalizedConditions[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(rule.RuleConditions[key])
	}
	rule.RuleConditions = normalizedConditions
	rule.RegoModule = strings.TrimSpace(rule.RegoModule)
	return rule
}

func HeadersFromHTTP(header http.Header) map[string]string {
	out := make(map[string]string, len(header))
	for key, values := range header {
		if len(values) == 0 {
			continue
		}
		out[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(values[0])
	}
	return out
}

func rolloutTrace(rule models.PolicyRule, input EvaluationInput) ConditionTrace {
	rollout := rule.RolloutPercent
	if rollout <= 0 {
		rollout = 100
	}
	matched := true
	actual := "100"
	if rollout < 100 {
		actual = stableRolloutBucket(rule, input)
		matched = atoiOrZero(actual) <= rollout
	}
	return ConditionTrace{
		Field:    "rollout_percent",
		Operator: ">=",
		Expected: intString(rollout),
		Actual:   actual,
		Matched:  matched,
		Source:   "rule.rollout_percent",
	}
}

func stableRolloutBucket(rule models.PolicyRule, input EvaluationInput) string {
	h := fnv.New32a()
	h.Write([]byte(strings.Join([]string{
		rule.Name,
		rule.RuleType,
		input.TenantID,
		input.Provider,
		input.Model,
		input.Environment,
		input.App,
		input.Session,
		string(input.RequestBody),
		string(input.ResponseBody),
	}, "|")))
	return intString(int(h.Sum32()%100) + 1)
}

func intString(value int) string {
	return strings.TrimSpace(strconvItoa(value))
}

func strconvItoa(value int) string {
	if value == 0 {
		return "0"
	}
	sign := ""
	if value < 0 {
		sign = "-"
		value = -value
	}
	digits := make([]byte, 0, 12)
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return sign + string(digits)
}

func atoiOrZero(value string) int {
	total := 0
	for _, ch := range strings.TrimSpace(value) {
		if ch < '0' || ch > '9' {
			return 0
		}
		total = total*10 + int(ch-'0')
	}
	return total
}
