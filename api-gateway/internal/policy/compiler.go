package policy

import (
	"sort"

	"github.com/agentfabric/api-gateway/internal/models"
)

func compileRules(rules []models.PolicyRule) []compiledRule {
	compiled := make([]compiledRule, 0, len(rules))
	for _, rule := range rules {
		normalized := normalizeRule(rule)
		compiled = append(compiled, compiledRule{
			model:           rule,
			normalized:      normalized,
			decisionMode:    normalized.DecisionMode,
			ruleConditions:  normalized.RuleConditions,
			normalizedScope: normalized.Scope,
		})
	}
	sort.SliceStable(compiled, func(i, j int) bool {
		if compiled[i].normalized.Priority != compiled[j].normalized.Priority {
			return compiled[i].normalized.Priority > compiled[j].normalized.Priority
		}
		return compiled[i].normalized.ID < compiled[j].normalized.ID
	})
	return compiled
}
