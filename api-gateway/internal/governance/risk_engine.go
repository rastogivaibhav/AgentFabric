package governance

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/govagn/api-gateway/internal/normalization"
)

// RuleMatcher defines a function that evaluates if an event matches a rule
type RuleMatcher func(*normalization.CanonicalEvent) bool

// RiskRule represents a governance rule that can be applied to events
type RiskRule struct {
	Name     string
	Match    RuleMatcher
	Score    int
	Category string
}

// RiskEngine evaluates events against governance rules
type RiskEngine struct {
	mu    sync.RWMutex
	rules []*RiskRule
}

// NewRiskEngine creates a new risk engine with default rules
func NewRiskEngine() *RiskEngine {
	return &RiskEngine{
		rules: initializeDefaultRules(),
	}
}

// Score evaluates an event and returns the maximum risk score and category
func (e *RiskEngine) Score(event *normalization.CanonicalEvent) (int, string) {
	if event == nil {
		return 0, "none"
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	maxScore := 0
	maxCategory := "none"

	for _, rule := range e.rules {
		if rule.Match(event) {
			if rule.Score > maxScore {
				maxScore = rule.Score
				maxCategory = rule.Category
			}
		}
	}

	return maxScore, maxCategory
}

// AddRule adds a custom rule to the engine
func (e *RiskEngine) AddRule(rule *RiskRule) {
	if rule == nil {
		return
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	e.rules = append(e.rules, rule)
}

// ListRules returns all active rules
func (e *RiskEngine) ListRules() []*RiskRule {
	e.mu.RLock()
	defer e.mu.RUnlock()

	rules := make([]*RiskRule, len(e.rules))
	copy(rules, e.rules)
	return rules
}

// initializeDefaultRules creates the standard set of governance rules
func initializeDefaultRules() []*RiskRule {
	return []*RiskRule{
		{
			Name:     "dangerous_shell_command",
			Category: "dangerous_command",
			Score:    90,
			Match: func(e *normalization.CanonicalEvent) bool {
				if e.EventCategory != "tool_call" || e.Action != "shell_command" {
					return false
				}
				cmd := strings.ToLower(e.Command)
				patterns := []string{
					`rm -rf`, `mkfs`, `chmod 777`, `dd if=`,
					`curl.*\|.*sh`, `wget.*\|.*sh`,
				}
				for _, pattern := range patterns {
					if matched, _ := regexp.MatchString(pattern, cmd); matched {
						return true
					}
				}
				return false
			},
		},
		{
			Name:     "secret_exposure",
			Category: "secret_exposure",
			Score:    100,
			Match: func(e *normalization.CanonicalEvent) bool {
				// Check command for secrets
				if e.Command != "" {
					if matchesSecretPattern(e.Command) {
						return true
					}
				}
				// Check payload for secrets
				if e.Payload != nil {
					payloadStr := fmt.Sprintf("%v", e.Payload)
					if matchesSecretPattern(payloadStr) {
						return true
					}
				}
				return false
			},
		},
		{
			Name:     "production_file_modification",
			Category: "prod_edit",
			Score:    70,
			Match: func(e *normalization.CanonicalEvent) bool {
				if e.EventCategory != "tool_call" || e.Action != "file_edit" {
					return false
				}
				if e.FilePath == "" {
					return false
				}
				path := strings.ToLower(e.FilePath)
				prodPatterns := []string{`prod`, `terraform`, `k8s`, `kubernetes`, `\.env`, `secret`}
				for _, pattern := range prodPatterns {
					if matched, _ := regexp.MatchString(pattern, path); matched {
						return true
					}
				}
				return false
			},
		},
		{
			Name:     "high_token_usage",
			Category: "high_token_usage",
			Score:    40,
			Match: func(e *normalization.CanonicalEvent) bool {
				totalTokens := e.InputTokens + e.OutputTokens
				return totalTokens > 200000
			},
		},
		{
			Name:     "unreviewed_production_deployment",
			Category: "prod_deploy",
			Score:    60,
			Match: func(e *normalization.CanonicalEvent) bool {
				if e.EventCategory != "tool_call" {
					return false
				}
				action := strings.ToLower(e.Action)
				cmd := strings.ToLower(e.Command)

				isDeployCommand := strings.Contains(action, "deploy") ||
					strings.Contains(action, "release") ||
					strings.Contains(cmd, "terraform apply") ||
					strings.Contains(cmd, "kubectl apply") ||
					strings.Contains(cmd, "git push") && strings.Contains(e.RepoName, "prod")

				isProdContext := strings.Contains(strings.ToLower(e.RepoName), "prod") ||
					strings.Contains(strings.ToLower(e.FilePath), "prod")

				return isDeployCommand && isProdContext && !e.RequiresReview
			},
		},
		{
			Name:     "mcp_tool_usage",
			Category: "mcp_usage",
			Score:    30,
			Match: func(e *normalization.CanonicalEvent) bool {
				return e.ToolName != "" && strings.Contains(strings.ToLower(e.ToolName), "mcp")
			},
		},
		{
			Name:     "unusual_activity_pattern",
			Category: "unusual_activity",
			Score:    50,
			Match: func(e *normalization.CanonicalEvent) bool {
				// Multiple high-token operations in quick succession
				// Or unusual tool combinations
				if e.InputTokens+e.OutputTokens > 100000 && strings.Contains(e.EventType, "error") {
					return true
				}
				return false
			},
		},
		{
			Name:     "failed_approval_required_action",
			Category: "approval_required",
			Score:    80,
			Match: func(e *normalization.CanonicalEvent) bool {
				return e.EventCategory == "approval" && e.Action == "approval_denied" && e.RiskScore > 70
			},
		},
	}
}

// matchesSecretPattern checks if a string contains common secret patterns
func matchesSecretPattern(s string) bool {
	patterns := []string{
		`AKIA[0-9A-Z]{16}`,                                    // AWS access key
		`sk-proj-[a-zA-Z0-9_-]{12,}`,                          // OpenAI project key
		`sk-[a-zA-Z0-9_-]{24,}`,                               // OpenAI-style key
		`ghp_[A-Za-z0-9_]{30,255}`,                            // GitHub personal access token
		`sk-ant-[a-zA-Z0-9_-]{24,}`,                           // Anthropic key
		`(?i)(?:postgres|mysql|mongodb|redis)://[^:]+:[^@]+@`, // Database URLs
		`-----BEGIN (?:RSA |OPENSSH )?PRIVATE KEY-----`,       // Private keys
		`(?i)(password|passwd|secret|api_key|token|access_key|secret_key)\s*=\s*\S+`, // .env style
	}

	for _, pattern := range patterns {
		if matched, _ := regexp.MatchString(pattern, s); matched {
			return true
		}
	}
	return false
}
