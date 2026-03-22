package policy

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/agentfabric/api-gateway/internal/models"
)

type RuleStore interface {
	ListPolicyRules(ctx context.Context) ([]models.PolicyRule, error)
}

type Engine struct {
	mu       sync.RWMutex
	rules    []models.PolicyRule
	compiled []compiledRule
}

func (e *Engine) RuleCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if len(e.compiled) > 0 {
		return len(e.compiled)
	}
	return len(e.rules)
}

type TrafficInput struct {
	TenantID        string
	Provider        string
	Model           string
	Environment     string
	EstimatedTokens int64
	RequestHeaders  map[string]string
	RequestBody     []byte
	Actor           string
	App             string
	Session         string
}

type DLPInput struct {
	TenantID       string
	Provider       string
	Model          string
	Environment    string
	Scope          string
	Body           []byte
	RequestHeaders map[string]string
	Actor          string
	App            string
	Session        string
}

type Finding struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	Count    int    `json:"count"`
}

type Decision struct {
	Matched      bool
	Final        bool
	RuleID       int64
	PolicyName   string
	Action       string
	Reason       string
	Scope        string
	MatchedNames []string
	Redactions   int
	RedactedBody []byte
	Explanation  DecisionExplanation
}

type detector struct {
	name     string
	category string
	re       *regexp.Regexp
}

var detectors = []detector{
	{name: "openai_api_key", category: "secret", re: regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{20,}\b`)},
	{name: "anthropic_api_key", category: "secret", re: regexp.MustCompile(`\bsk-ant-[A-Za-z0-9_-]{20,}\b`)},
	{name: "github_token", category: "secret", re: regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{20,}\b`)},
	{name: "aws_access_key", category: "secret", re: regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)},
	{name: "email", category: "pii", re: regexp.MustCompile(`(?i)\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}\b`)},
	{name: "ssn", category: "pii", re: regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`)},
}

func NewEngine() *Engine {
	return &Engine{}
}

func (e *Engine) LoadRules(ctx context.Context, store RuleStore) error {
	rules, err := store.ListPolicyRules(ctx)
	if err != nil {
		return err
	}
	sort.SliceStable(rules, func(i, j int) bool {
		if rules[i].Priority != rules[j].Priority {
			return rules[i].Priority > rules[j].Priority
		}
		return rules[i].ID < rules[j].ID
	})
	e.mu.Lock()
	e.rules = rules
	e.compiled = compileRules(rules)
	e.mu.Unlock()
	return nil
}

func (e *Engine) EvaluateTraffic(input TrafficInput) Decision {
	return e.evaluateTrafficInput(EvaluationInput{
		TenantID:        input.TenantID,
		Environment:     input.Environment,
		Provider:        input.Provider,
		Model:           input.Model,
		Scope:           "request",
		EstimatedTokens: input.EstimatedTokens,
		RequestHeaders:  input.RequestHeaders,
		RequestBody:     input.RequestBody,
		Actor:           input.Actor,
		App:             input.App,
		Session:         input.Session,
	})
}

func (e *Engine) EvaluateDLP(input DLPInput) Decision {
	evaluationInput := EvaluationInput{
		TenantID:       input.TenantID,
		Environment:    input.Environment,
		Provider:       input.Provider,
		Model:          input.Model,
		Scope:          input.Scope,
		RequestHeaders: input.RequestHeaders,
		Actor:          input.Actor,
		App:            input.App,
		Session:        input.Session,
	}
	if strings.EqualFold(input.Scope, "response") {
		evaluationInput.ResponseBody = input.Body
	} else {
		evaluationInput.RequestBody = input.Body
	}
	return e.evaluateDLPInput(evaluationInput)
}

func (e *Engine) Evaluate(input EvaluationInput) (Decision, Decision, Decision) {
	return e.evaluateTrafficInput(input), e.evaluateDLPInput(withScope(input, "request")), e.evaluateDLPInput(withScope(input, "response"))
}

func (e *Engine) evaluateTrafficInput(input EvaluationInput) Decision {
	compiled := e.currentCompiledRules()
	return evaluateTrafficRules(compiled, normalizeEvaluationInput(input))
}

func (e *Engine) evaluateDLPInput(input EvaluationInput) Decision {
	input = normalizeEvaluationInput(input)
	findings := scan(decisionBody(input))
	if len(findings) == 0 {
		return Decision{}
	}
	compiled := e.currentCompiledRules()
	decision := evaluateDLPRules(compiled, input, findings)
	if !decision.Matched {
		return Decision{}
	}
	if decision.Action == "redact" {
		decision.RedactedBody, decision.Redactions = redact(decisionBody(input), findings)
	}
	return decision
}

func (e *Engine) currentCompiledRules() []compiledRule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	source := e.compiled
	if len(source) == 0 && len(e.rules) > 0 {
		source = compileRules(e.rules)
	}
	out := make([]compiledRule, len(source))
	copy(out, source)
	return out
}

func decisionBody(input EvaluationInput) []byte {
	if strings.EqualFold(input.Scope, "response") {
		return input.ResponseBody
	}
	return input.RequestBody
}

func withScope(input EvaluationInput, scope string) EvaluationInput {
	input.Scope = scope
	return input
}

func scan(body []byte) []Finding {
	if len(body) == 0 {
		return nil
	}
	text := string(body)
	findings := make([]Finding, 0, len(detectors))
	for _, detector := range detectors {
		matches := detector.re.FindAllStringIndex(text, -1)
		if len(matches) == 0 {
			continue
		}
		findings = append(findings, Finding{
			Name:     detector.name,
			Category: detector.category,
			Count:    len(matches),
		})
	}
	return findings
}

func redact(body []byte, findings []Finding) ([]byte, int) {
	if len(body) == 0 {
		return body, 0
	}
	text := string(body)
	redactions := 0
	for _, detector := range detectors {
		shouldRedact := false
		for _, finding := range findings {
			if finding.Name == detector.name {
				shouldRedact = true
				break
			}
		}
		if !shouldRedact {
			continue
		}
		count := len(detector.re.FindAllString(text, -1))
		if count == 0 {
			continue
		}
		redactions += count
		text = detector.re.ReplaceAllString(text, "[REDACTED:"+detector.category+"]")
	}
	return []byte(text), redactions
}

func matchesValue(ruleValue, actual string) bool {
	ruleValue = strings.ToLower(strings.TrimSpace(ruleValue))
	actual = strings.ToLower(strings.TrimSpace(actual))
	return ruleValue == "" || ruleValue == "*" || ruleValue == actual
}

func exactValue(ruleValue, actual string) bool {
	ruleValue = strings.ToLower(strings.TrimSpace(ruleValue))
	actual = strings.ToLower(strings.TrimSpace(actual))
	return ruleValue != "" && ruleValue != "*" && ruleValue == actual
}

func matchesPattern(pattern, actual string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	actual = strings.ToLower(strings.TrimSpace(actual))
	if pattern == "" || pattern == "*" {
		return true
	}
	return strings.HasPrefix(actual, pattern)
}

func scopeMatches(ruleScope, actual string) bool {
	ruleScope = strings.ToLower(strings.TrimSpace(ruleScope))
	actual = strings.ToLower(strings.TrimSpace(actual))
	if ruleScope == "" || ruleScope == "both" {
		return true
	}
	return ruleScope == actual
}

func sortedConditionKeys(conditions map[string]string) []string {
	keys := make([]string, 0, len(conditions))
	for key := range conditions {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
