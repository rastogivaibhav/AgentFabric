package pricing

import "fmt"

type Explanation struct {
	RuleID       int64
	Provider     string
	MatchedModel string
	ModelPattern string
	Scope        string
	RateCard     RateCard
	Result       Result
}

func BuildExplanation(explanation Explanation) []string {
	lines := []string{}
	if explanation.RuleID > 0 {
		lines = append(lines, fmt.Sprintf("rule %d matched %s for %s", explanation.RuleID, explanation.ModelPattern, explanation.MatchedModel))
	}
	if explanation.Scope != "" {
		lines = append(lines, fmt.Sprintf("pricing scope: %s", explanation.Scope))
	}
	appendCategory := func(label string, tokens int64, rate, cost float64) {
		if tokens <= 0 {
			return
		}
		lines = append(lines, fmt.Sprintf("%s: %d tokens at $%.4f/M = $%.6f", label, tokens, rate, cost))
	}
	appendCategory("input", explanation.Result.Usage.InputTokens, explanation.RateCard.InputPerMillion, explanation.Result.InputCostUSD)
	appendCategory("output", explanation.Result.Usage.OutputTokens, explanation.RateCard.OutputPerMillion, explanation.Result.OutputCostUSD)
	appendCategory("cache read", explanation.Result.Usage.CacheReadTokens, explanation.RateCard.CacheReadPerMillion, explanation.Result.CacheReadCostUSD)
	appendCategory("cache write", explanation.Result.Usage.CacheWriteTokens, explanation.RateCard.CacheWritePerMillion, explanation.Result.CacheWriteCostUSD)
	appendCategory("reasoning", explanation.Result.Usage.ReasoningTokens, explanation.RateCard.ReasoningPerMillion, explanation.Result.ReasoningCostUSD)
	if explanation.Result.TotalCostUSD > 0 {
		lines = append(lines, fmt.Sprintf("total cost: $%.6f", explanation.Result.TotalCostUSD))
	}
	return lines
}
