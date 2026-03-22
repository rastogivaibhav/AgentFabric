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
	mu    sync.RWMutex
	rules []models.PolicyRule
}

type TrafficInput struct {
	TenantID        string
	Provider        string
	Model           string
	Environment     string
	EstimatedTokens int64
}

type DLPInput struct {
	TenantID    string
	Provider    string
	Model       string
	Environment string
	Scope       string
	Body        []byte
}

type Finding struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	Count    int    `json:"count"`
}

type Decision struct {
	Matched      bool
	RuleID       int64
	PolicyName   string
	Action       string
	Reason       string
	Scope        string
	MatchedNames []string
	Redactions   int
	RedactedBody []byte
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
	e.mu.Unlock()
	return nil
}

func (e *Engine) EvaluateTraffic(input TrafficInput) Decision {
	e.mu.RLock()
	rules := append([]models.PolicyRule(nil), e.rules...)
	e.mu.RUnlock()

	best := Decision{}
	bestScore := -1
	for _, rule := range rules {
		score, ok := matchTrafficRule(rule, input)
		if !ok || score <= bestScore {
			continue
		}
		bestScore = score
		best = Decision{
			Matched:    true,
			RuleID:     rule.ID,
			PolicyName: rule.Name,
			Action:     strings.ToLower(rule.Action),
			Scope:      "request",
		}
		if rule.MaxTokens > 0 {
			best.Reason = "estimated tokens exceeded policy limit"
		} else if rule.ModelPattern != "" && rule.ModelPattern != "*" {
			best.Reason = "provider/model policy matched"
		} else {
			best.Reason = "traffic policy matched"
		}
	}
	return best
}

func (e *Engine) EvaluateDLP(input DLPInput) Decision {
	findings := scan(input.Body)
	if len(findings) == 0 {
		return Decision{}
	}

	e.mu.RLock()
	rules := append([]models.PolicyRule(nil), e.rules...)
	e.mu.RUnlock()

	best := Decision{}
	bestScore := -1
	for _, rule := range rules {
		score, matchedNames, ok := matchDLPRule(rule, input, findings)
		if !ok || score <= bestScore {
			continue
		}
		bestScore = score
		best = Decision{
			Matched:      true,
			RuleID:       rule.ID,
			PolicyName:   rule.Name,
			Action:       strings.ToLower(rule.Action),
			Scope:        input.Scope,
			MatchedNames: matchedNames,
			Reason:       "sensitive content detected",
		}
	}

	if !best.Matched {
		return Decision{}
	}
	if best.Action == "redact" {
		best.RedactedBody, best.Redactions = redact(input.Body, findings)
	}
	return best
}

func matchTrafficRule(rule models.PolicyRule, input TrafficInput) (int, bool) {
	if !rule.Enabled || strings.ToLower(rule.RuleType) != "traffic" {
		return 0, false
	}
	if rule.TenantID != nil && strings.TrimSpace(*rule.TenantID) != strings.TrimSpace(input.TenantID) {
		return 0, false
	}
	if !matchesValue(rule.Provider, input.Provider) {
		return 0, false
	}
	if !matchesPattern(rule.ModelPattern, input.Model) {
		return 0, false
	}
	if !matchesValue(rule.Environment, input.Environment) {
		return 0, false
	}
	if rule.MaxTokens > 0 && input.EstimatedTokens <= rule.MaxTokens {
		return 0, false
	}

	score := rule.Priority * 100
	if rule.TenantID != nil {
		score += 100000
	}
	if exactValue(rule.Provider, input.Provider) {
		score += 1000
	}
	if exactValue(rule.ModelPattern, input.Model) {
		score += 500
	} else if strings.TrimSpace(rule.ModelPattern) != "" && rule.ModelPattern != "*" {
		score += len(rule.ModelPattern)
	}
	if exactValue(rule.Environment, input.Environment) {
		score += 250
	}
	if rule.MaxTokens > 0 {
		score += 100
	}
	return score, true
}

func matchDLPRule(rule models.PolicyRule, input DLPInput, findings []Finding) (int, []string, bool) {
	if !rule.Enabled || strings.ToLower(rule.RuleType) != "dlp" {
		return 0, nil, false
	}
	if rule.TenantID != nil && strings.TrimSpace(*rule.TenantID) != strings.TrimSpace(input.TenantID) {
		return 0, nil, false
	}
	if !matchesValue(rule.Provider, input.Provider) {
		return 0, nil, false
	}
	if !matchesPattern(rule.ModelPattern, input.Model) {
		return 0, nil, false
	}
	if !matchesValue(rule.Environment, input.Environment) {
		return 0, nil, false
	}
	if !scopeMatches(rule.Scope, input.Scope) {
		return 0, nil, false
	}

	matchedNames := make([]string, 0, len(findings))
	target := strings.ToLower(strings.TrimSpace(rule.Detector))
	for _, finding := range findings {
		if target == "" || target == "*" || target == finding.Name || target == finding.Category {
			matchedNames = append(matchedNames, finding.Name)
		}
	}
	if len(matchedNames) == 0 {
		return 0, nil, false
	}

	score := rule.Priority * 100
	if rule.TenantID != nil {
		score += 100000
	}
	if exactValue(rule.Provider, input.Provider) {
		score += 1000
	}
	if exactValue(rule.ModelPattern, input.Model) {
		score += 500
	} else if strings.TrimSpace(rule.ModelPattern) != "" && rule.ModelPattern != "*" {
		score += len(rule.ModelPattern)
	}
	if exactValue(rule.Environment, input.Environment) {
		score += 250
	}
	if strings.TrimSpace(rule.Detector) != "" && rule.Detector != "*" {
		score += 100
	}
	return score, matchedNames, true
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
