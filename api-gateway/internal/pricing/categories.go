package pricing

type Usage struct {
	InputTokens      int64 `json:"input_tokens,omitempty"`
	OutputTokens     int64 `json:"output_tokens,omitempty"`
	CacheReadTokens  int64 `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int64 `json:"cache_write_tokens,omitempty"`
	ReasoningTokens  int64 `json:"reasoning_tokens,omitempty"`
}

func (u Usage) TotalTokens() int64 {
	return u.InputTokens + u.OutputTokens + u.CacheReadTokens + u.CacheWriteTokens + u.ReasoningTokens
}

type RateCard struct {
	InputPerMillion      float64 `json:"input_per_million,omitempty"`
	OutputPerMillion     float64 `json:"output_per_million,omitempty"`
	CacheReadPerMillion  float64 `json:"cache_read_per_million,omitempty"`
	CacheWritePerMillion float64 `json:"cache_write_per_million,omitempty"`
	ReasoningPerMillion  float64 `json:"reasoning_per_million,omitempty"`
}

type Result struct {
	Usage             Usage   `json:"usage"`
	InputCostUSD      float64 `json:"input_cost_usd"`
	OutputCostUSD     float64 `json:"output_cost_usd"`
	CacheReadCostUSD  float64 `json:"cache_read_cost_usd,omitempty"`
	CacheWriteCostUSD float64 `json:"cache_write_cost_usd,omitempty"`
	ReasoningCostUSD  float64 `json:"reasoning_cost_usd,omitempty"`
	TotalCostUSD      float64 `json:"total_cost_usd"`
}

func Compute(rate RateCard, usage Usage) Result {
	result := Result{
		Usage:             usage,
		InputCostUSD:      perMillionCost(usage.InputTokens, rate.InputPerMillion),
		OutputCostUSD:     perMillionCost(usage.OutputTokens, rate.OutputPerMillion),
		CacheReadCostUSD:  perMillionCost(usage.CacheReadTokens, rate.CacheReadPerMillion),
		CacheWriteCostUSD: perMillionCost(usage.CacheWriteTokens, rate.CacheWritePerMillion),
		ReasoningCostUSD:  perMillionCost(usage.ReasoningTokens, rate.ReasoningPerMillion),
	}
	result.TotalCostUSD = result.InputCostUSD + result.OutputCostUSD + result.CacheReadCostUSD + result.CacheWriteCostUSD + result.ReasoningCostUSD
	return result
}

func perMillionCost(tokens int64, rate float64) float64 {
	if tokens <= 0 || rate <= 0 {
		return 0
	}
	return float64(tokens) / 1_000_000 * rate
}
