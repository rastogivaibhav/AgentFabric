package policy

import (
	"net/http"
	"sort"
	"strings"

	"github.com/agentfabric/api-gateway/internal/models"
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
}

type ConditionTrace = models.PolicyConditionTrace

type DecisionExplanation struct {
	Engine         string
	DecisionMode   string
	EvaluationPath []string
	MatchedFields  []string
	ConditionTrace []ConditionTrace
	RegoQuery      string
	Explain        string
	RuleConditions map[string]string
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
