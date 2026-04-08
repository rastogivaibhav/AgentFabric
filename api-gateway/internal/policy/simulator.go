package policy

import "github.com/govagn/api-gateway/internal/models"

func (e *Engine) Simulate(req models.PolicySimulationRequest) models.PolicySimulationResponse {
	results := make([]models.PolicySimulationResult, 0, len(req.Samples))
	for _, sample := range req.Samples {
		traffic, requestDLP, responseDLP := e.Evaluate(EvaluationInput{
			TenantID:        sample.TenantID,
			Environment:     sample.Environment,
			Provider:        sample.Provider,
			Model:           sample.Model,
			EstimatedTokens: sample.EstimatedTokens,
			Actor:           sample.Actor,
			App:             sample.App,
			Session:         sample.Session,
			RequestBody:     []byte(sample.RequestBody),
			ResponseBody:    []byte(sample.ResponseBody),
		})
		results = append(results, models.PolicySimulationResult{
			Label:       sample.Label,
			Traffic:     previewDecision(traffic),
			RequestDLP:  previewDecision(requestDLP),
			ResponseDLP: previewDecision(responseDLP),
		})
	}
	return models.PolicySimulationResponse{
		Count:   len(results),
		Results: results,
	}
}

func previewDecision(decision Decision) models.PolicyPreviewDecision {
	preview := models.PolicyPreviewDecision{
		Matched:          decision.Matched,
		RuleID:           decision.RuleID,
		PolicyName:       decision.PolicyName,
		Action:           decision.Action,
		Reason:           decision.Reason,
		Scope:            decision.Scope,
		MatchedNames:     append([]string(nil), decision.MatchedNames...),
		GuardrailMatches: append([]string(nil), decision.Explanation.GuardrailMatches...),
		Redactions:       decision.Redactions,
		Final:            decision.Final,
		Engine:           decision.Explanation.Engine,
		DecisionMode:     decision.Explanation.DecisionMode,
		Version:          decision.Explanation.Version,
		RolloutPercent:   decision.Explanation.RolloutPercent,
		EvaluationPath:   append([]string(nil), decision.Explanation.EvaluationPath...),
		MatchedFields:    append([]string(nil), decision.Explanation.MatchedFields...),
		ConditionTrace:   append([]models.PolicyConditionTrace(nil), decision.Explanation.ConditionTrace...),
		RegoQuery:        decision.Explanation.RegoQuery,
		Explain:          decision.Explanation.Explain,
		RuleConditions:   cloneConditions(decision.Explanation.RuleConditions),
	}
	if len(decision.RedactedBody) > 0 {
		preview.RedactedPreview = string(decision.RedactedBody)
	}
	return preview
}
