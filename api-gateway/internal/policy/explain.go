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
	return fmt.Sprintf("%s matched on %s via %s", action, strings.Join(explanation.MatchedFields, ", "), explanation.Engine)
}
