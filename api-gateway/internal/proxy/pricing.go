package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/agentfabric/api-gateway/internal/models"
)

type modelPricing struct {
	inputPerMillion  float64
	outputPerMillion float64
}

type pricingRule struct {
	ID               int64
	TenantID         string
	Provider         string     `json:"provider"`
	ModelPattern     string     `json:"model_pattern"`
	InputPerMillion  float64    `json:"input_per_million"`
	OutputPerMillion float64    `json:"output_per_million"`
	Active           bool       `json:"active"`
	Priority         int        `json:"priority"`
	EffectiveFrom    *time.Time `json:"effective_from"`
	EffectiveTo      *time.Time `json:"effective_to"`
	Description      string     `json:"description"`
}

type PricingMatch struct {
	RuleID           int64
	Provider         string
	ModelPattern     string
	MatchedModel     string
	InputPerMillion  float64
	OutputPerMillion float64
	Scope            string
	Priority         int
	EffectiveFrom    *time.Time
	EffectiveTo      *time.Time
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
		{Provider: ProviderAnthropic, ModelPattern: "claude-3-5-sonnet", InputPerMillion: 3.0, OutputPerMillion: 15.0, Active: true, Priority: 100},
		{Provider: ProviderAnthropic, ModelPattern: "claude-3-5-haiku", InputPerMillion: 0.80, OutputPerMillion: 4.00, Active: true, Priority: 100},
		{Provider: ProviderAnthropic, ModelPattern: "claude-3-opus", InputPerMillion: 15.0, OutputPerMillion: 75.0, Active: true, Priority: 100},
		{Provider: ProviderAnthropic, ModelPattern: "claude-3-haiku", InputPerMillion: 0.25, OutputPerMillion: 1.25, Active: true, Priority: 100},
		{Provider: ProviderOpenAI, ModelPattern: "gpt-4o", InputPerMillion: 5.0, OutputPerMillion: 15.0, Active: true, Priority: 100},
		{Provider: ProviderOpenAI, ModelPattern: "gpt-4o-mini", InputPerMillion: 0.15, OutputPerMillion: 0.60, Active: true, Priority: 100},
		{Provider: ProviderOpenAI, ModelPattern: "gpt-4-turbo", InputPerMillion: 10.0, OutputPerMillion: 30.0, Active: true, Priority: 100},
		{Provider: ProviderOpenAI, ModelPattern: "gpt-3.5-turbo", InputPerMillion: 0.50, OutputPerMillion: 1.50, Active: true, Priority: 100},
		{Provider: ProviderGoogle, ModelPattern: "gemini-1.5-pro", InputPerMillion: 3.5, OutputPerMillion: 10.5, Active: true, Priority: 100},
		{Provider: ProviderGoogle, ModelPattern: "gemini-1.5-flash", InputPerMillion: 0.35, OutputPerMillion: 1.05, Active: true, Priority: 100},
		{Provider: "meta", ModelPattern: "llama-3.1-405b", InputPerMillion: 5.0, OutputPerMillion: 15.0, Active: true, Priority: 100},
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
		tenantID := ""
		if rule.TenantID != nil {
			tenantID = strings.TrimSpace(*rule.TenantID)
		}
		normalized = append(normalized, pricingRule{
			ID:               rule.ID,
			TenantID:         tenantID,
			Provider:         NormalizeProvider(rule.Provider),
			ModelPattern:     strings.ToLower(strings.TrimSpace(rule.ModelPattern)),
			InputPerMillion:  rule.InputPerMillion,
			OutputPerMillion: rule.OutputPerMillion,
			Active:           rule.Active,
			Priority:         rule.Priority,
			EffectiveFrom:    rule.EffectiveFrom,
			EffectiveTo:      rule.EffectiveTo,
			Description:      rule.Description,
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
		rule.Provider = NormalizeProvider(rule.Provider)
		rule.ModelPattern = strings.ToLower(strings.TrimSpace(rule.ModelPattern))
		if rule.ModelPattern == "" {
			return nil, fmt.Errorf("parse pricing config: model_pattern is required")
		}
		if rule.InputPerMillion < 0 || rule.OutputPerMillion < 0 {
			return nil, fmt.Errorf("parse pricing config: pricing values must be non-negative")
		}
		rule.Active = true
		if rule.Priority == 0 {
			rule.Priority = 100
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

func ResolvePricing(provider, model, tenantID string, at time.Time) (PricingMatch, bool) {
	normalizedProvider := NormalizeProvider(provider)
	normalizedModel := strings.ToLower(strings.TrimSpace(model))
	normalizedTenant := strings.TrimSpace(tenantID)
	if normalizedModel == "" {
		return PricingMatch{}, false
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}

	rules := currentPricingRules()
	type candidate struct {
		rule        pricingRule
		scope       string
		exact       bool
		prefixLen   int
		tenantScore int
	}
	candidates := make([]candidate, 0, len(rules))
	for _, rule := range rules {
		if !rule.Active {
			continue
		}
		if rule.Provider != "" && normalizedProvider != "" && rule.Provider != normalizedProvider {
			continue
		}
		if rule.TenantID != "" && rule.TenantID != normalizedTenant {
			continue
		}
		if rule.EffectiveFrom != nil && at.Before(rule.EffectiveFrom.UTC()) {
			continue
		}
		if rule.EffectiveTo != nil && at.After(rule.EffectiveTo.UTC()) {
			continue
		}
		scope := "global"
		tenantScore := 1
		if rule.TenantID != "" {
			scope = "tenant"
			tenantScore = 2
		}
		if rule.ModelPattern == normalizedModel {
			candidates = append(candidates, candidate{
				rule:        rule,
				scope:       scope,
				exact:       true,
				prefixLen:   len(rule.ModelPattern),
				tenantScore: tenantScore,
			})
			continue
		}
		if strings.HasPrefix(normalizedModel, rule.ModelPattern) {
			candidates = append(candidates, candidate{
				rule:        rule,
				scope:       scope,
				exact:       false,
				prefixLen:   len(rule.ModelPattern),
				tenantScore: tenantScore,
			})
		}
	}
	if len(candidates) == 0 {
		return PricingMatch{}, false
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		left := candidates[i]
		right := candidates[j]
		if left.tenantScore != right.tenantScore {
			return left.tenantScore > right.tenantScore
		}
		if left.exact != right.exact {
			return left.exact
		}
		if left.prefixLen != right.prefixLen {
			return left.prefixLen > right.prefixLen
		}
		if left.rule.Priority != right.rule.Priority {
			return left.rule.Priority > right.rule.Priority
		}
		if left.rule.ID != right.rule.ID {
			return left.rule.ID < right.rule.ID
		}
		return left.rule.ModelPattern < right.rule.ModelPattern
	})

	best := candidates[0]
	return PricingMatch{
		RuleID:           best.rule.ID,
		Provider:         best.rule.Provider,
		ModelPattern:     best.rule.ModelPattern,
		MatchedModel:     normalizedModel,
		InputPerMillion:  best.rule.InputPerMillion,
		OutputPerMillion: best.rule.OutputPerMillion,
		Scope:            best.scope,
		Priority:         best.rule.Priority,
		EffectiveFrom:    best.rule.EffectiveFrom,
		EffectiveTo:      best.rule.EffectiveTo,
	}, true
}

func lookupPricing(provider, model string) (modelPricing, bool) {
	match, ok := ResolvePricing(provider, model, "", time.Now().UTC())
	if !ok {
		return modelPricing{}, false
	}
	return modelPricing{
		inputPerMillion:  match.InputPerMillion,
		outputPerMillion: match.OutputPerMillion,
	}, true
}

func ComputeExactCost(provider, model string, inputTokens, outputTokens int64) (float64, float64) {
	_, inputCost, totalCost := ComputeExactCostForTenant(provider, model, "", time.Now().UTC(), inputTokens, outputTokens)
	return inputCost, totalCost
}

func ComputeExactCostForTenant(provider, model, tenantID string, at time.Time, inputTokens, outputTokens int64) (PricingMatch, float64, float64) {
	match, ok := ResolvePricing(provider, model, tenantID, at)
	if !ok {
		return PricingMatch{}, 0, 0
	}

	inputCost := float64(inputTokens) / 1_000_000 * match.InputPerMillion
	outputCost := float64(outputTokens) / 1_000_000 * match.OutputPerMillion
	return match, inputCost, inputCost + outputCost
}

func ComputeEstimatedCost(provider, model string, estimatedInputTokens int64) (float64, float64) {
	return ComputeExactCost(provider, model, estimatedInputTokens, 0)
}

func ComputeEstimatedCostForTenant(provider, model, tenantID string, at time.Time, estimatedInputTokens int64) (PricingMatch, float64, float64) {
	return ComputeExactCostForTenant(provider, model, tenantID, at, estimatedInputTokens, 0)
}

func computeProxyCost(provider, model string, inputTokens, outputTokens int64) (float64, float64) {
	return ComputeExactCost(provider, model, inputTokens, outputTokens)
}
