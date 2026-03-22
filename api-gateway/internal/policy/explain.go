package policy

import (
	"fmt"
	"strings"

	"github.com/agentfabric/api-gateway/internal/models"
)

func explainDecision(rule models.PolicyRule, explanation DecisionExplanation) string {
	if len(explanation.MatchedFields) == 0 {
		return "no policy fields matched"
	}
	action := strings.TrimSpace(rule.Action)
	if action == "" {
		action = "match"
	}
	base := fmt.Sprintf("%s matched on %s via %s", action, strings.Join(explanation.MatchedFields, ", "), explanation.Engine)
	details := []string{}
	if len(explanation.GuardrailMatches) > 0 {
		details = append(details, "guards="+strings.Join(explanation.GuardrailMatches, ","))
	}
	if explanation.Version > 0 {
		details = append(details, fmt.Sprintf("v%d", explanation.Version))
	}
	if explanation.RolloutPercent > 0 && explanation.RolloutPercent < 100 {
		details = append(details, fmt.Sprintf("rollout=%d%%", explanation.RolloutPercent))
	}
	if len(details) == 0 {
		return base
	}
	return base + " (" + strings.Join(details, " · ") + ")"
}
