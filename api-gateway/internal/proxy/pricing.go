package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/agentfabric/api-gateway/internal/models"
)

type modelPricing struct {
	inputPerMillion  float64
	outputPerMillion float64
}

type pricingRule struct {
	Provider         string  `json:"provider"`
	ModelPattern     string  `json:"model_pattern"`
	InputPerMillion  float64 `json:"input_per_million"`
	OutputPerMillion float64 `json:"output_per_million"`
}

var (
	pricingMu          sync.RWMutex
	activePricingRules = defaultPricingRules()
)

type ruleStore interface {
	ListPricingRules(ctx context.Context) ([]models.PricingRule, error)
}

func defaultPricingRules() []pricingRule {
	return []pricingRule{
		{Provider: ProviderAnthropic, ModelPattern: "claude-3-5-sonnet", InputPerMillion: 3.0, OutputPerMillion: 15.0},
		{Provider: ProviderAnthropic, ModelPattern: "claude-3-5-haiku", InputPerMillion: 0.80, OutputPerMillion: 4.00},
		{Provider: ProviderAnthropic, ModelPattern: "claude-3-opus", InputPerMillion: 15.0, OutputPerMillion: 75.0},
		{Provider: ProviderAnthropic, ModelPattern: "claude-3-haiku", InputPerMillion: 0.25, OutputPerMillion: 1.25},
		{Provider: ProviderOpenAI, ModelPattern: "gpt-4o", InputPerMillion: 5.0, OutputPerMillion: 15.0},
		{Provider: ProviderOpenAI, ModelPattern: "gpt-4o-mini", InputPerMillion: 0.15, OutputPerMillion: 0.60},
		{Provider: ProviderOpenAI, ModelPattern: "gpt-4-turbo", InputPerMillion: 10.0, OutputPerMillion: 30.0},
		{Provider: ProviderOpenAI, ModelPattern: "gpt-3.5-turbo", InputPerMillion: 0.50, OutputPerMillion: 1.50},
		{Provider: "google", ModelPattern: "gemini-1.5-pro", InputPerMillion: 3.5, OutputPerMillion: 10.5},
		{Provider: "google", ModelPattern: "gemini-1.5-flash", InputPerMillion: 0.35, OutputPerMillion: 1.05},
		{Provider: "meta", ModelPattern: "llama-3.1-405b", InputPerMillion: 5.0, OutputPerMillion: 15.0},
	}
}

func ConfigurePricingFromEnv() error {
	raw, err := pricingConfigSource()
	if err != nil {
		return err
	}
	// Precedence layer 2/3:
	// 1. DB-backed pricing rules can override this later via LoadPricingRules.
	// 2. If no env/file config is present, configurePricing("") falls back to built-in defaults.
	return configurePricing(raw)
}

func LoadPricingRules(ctx context.Context, store ruleStore) error {
	rules, err := store.ListPricingRules(ctx)
	if err != nil {
		return err
	}
	// Precedence layer 1:
	// When pricing rules exist in the database, they are authoritative and replace
	// any env/file/bootstrap pricing already loaded into memory.
	//
	// If the DB table is empty, we intentionally keep the current in-memory rules
	// unchanged so local/dev setups can continue using env/file/bootstrap pricing.
	if len(rules) == 0 {
		return nil
	}
	SetPricingRules(rules)
	return nil
}

func SetPricingRules(rules []models.PricingRule) {
	normalized := make([]pricingRule, 0, len(rules))
	for _, rule := range rules {
		normalized = append(normalized, pricingRule{
			Provider:         strings.ToLower(strings.TrimSpace(rule.Provider)),
			ModelPattern:     strings.ToLower(strings.TrimSpace(rule.ModelPattern)),
			InputPerMillion:  rule.InputPerMillion,
			OutputPerMillion: rule.OutputPerMillion,
		})
	}
	pricingMu.Lock()
	defer pricingMu.Unlock()
	activePricingRules = normalized
}

func configurePricing(raw string) error {
	rules, err := parsePricingRules(raw)
	if err != nil {
		return err
	}
	pricingMu.Lock()
	defer pricingMu.Unlock()
	activePricingRules = rules
	return nil
}

func parsePricingRules(raw string) ([]pricingRule, error) {
	if strings.TrimSpace(raw) == "" {
		return defaultPricingRules(), nil
	}

	var rules []pricingRule
	if err := json.Unmarshal([]byte(raw), &rules); err != nil {
		return nil, fmt.Errorf("parse pricing config: %w", err)
	}
	if len(rules) == 0 {
		return nil, fmt.Errorf("parse pricing config: no pricing rules defined")
	}

	normalized := make([]pricingRule, 0, len(rules))
	for _, rule := range rules {
		rule.Provider = strings.ToLower(strings.TrimSpace(rule.Provider))
		rule.ModelPattern = strings.ToLower(strings.TrimSpace(rule.ModelPattern))
		if rule.ModelPattern == "" {
			return nil, fmt.Errorf("parse pricing config: model_pattern is required")
		}
		if rule.InputPerMillion < 0 || rule.OutputPerMillion < 0 {
			return nil, fmt.Errorf("parse pricing config: pricing values must be non-negative")
		}
		normalized = append(normalized, rule)
	}
	return normalized, nil
}

func pricingConfigSource() (string, error) {
	if path := strings.TrimSpace(os.Getenv("AF_MODEL_PRICING_FILE")); path != "" {
		body, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read pricing config file: %w", err)
		}
		return string(body), nil
	}
	return os.Getenv("AF_MODEL_PRICING_JSON"), nil
}

func currentPricingRules() []pricingRule {
	pricingMu.RLock()
	defer pricingMu.RUnlock()
	out := make([]pricingRule, len(activePricingRules))
	copy(out, activePricingRules)
	return out
}

func lookupPricing(provider, model string) (modelPricing, bool) {
	normalizedProvider := strings.ToLower(strings.TrimSpace(provider))
	normalizedModel := strings.ToLower(strings.TrimSpace(model))
	if normalizedModel == "" {
		return modelPricing{}, false
	}

	rules := currentPricingRules()

	for _, rule := range rules {
		if rule.ModelPattern != normalizedModel {
			continue
		}
		if rule.Provider != "" && normalizedProvider != "" && rule.Provider != normalizedProvider {
			continue
		}
		if rule.Provider != "" && normalizedProvider == "" {
			continue
		}
		return modelPricing{
			inputPerMillion:  rule.InputPerMillion,
			outputPerMillion: rule.OutputPerMillion,
		}, true
	}

	longestPrefixLen := -1
	var best modelPricing
	var found bool
	for _, rule := range rules {
		if rule.Provider != "" && normalizedProvider != "" && rule.Provider != normalizedProvider {
			continue
		}
		if rule.Provider != "" && normalizedProvider == "" {
			continue
		}
		if strings.HasPrefix(normalizedModel, rule.ModelPattern) && len(rule.ModelPattern) > longestPrefixLen {
			longestPrefixLen = len(rule.ModelPattern)
			best = modelPricing{
				inputPerMillion:  rule.InputPerMillion,
				outputPerMillion: rule.OutputPerMillion,
			}
			found = true
		}
	}

	return best, found
}

func ComputeExactCost(provider, model string, inputTokens, outputTokens int64) (float64, float64) {
	pricing, ok := lookupPricing(provider, model)
	if !ok {
		return 0, 0
	}

	inputCost := float64(inputTokens) / 1_000_000 * pricing.inputPerMillion
	outputCost := float64(outputTokens) / 1_000_000 * pricing.outputPerMillion
	return inputCost, inputCost + outputCost
}

func ComputeEstimatedCost(provider, model string, estimatedInputTokens int64) (float64, float64) {
	return ComputeExactCost(provider, model, estimatedInputTokens, 0)
}

func computeProxyCost(provider, model string, inputTokens, outputTokens int64) (float64, float64) {
	return ComputeExactCost(provider, model, inputTokens, outputTokens)
}
