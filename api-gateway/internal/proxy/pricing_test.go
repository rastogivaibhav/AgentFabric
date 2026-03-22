package proxy

import (
	"context"
	"testing"
	"time"

	"github.com/agentfabric/api-gateway/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockRuleStore struct {
	rules []models.PricingRule
	err   error
}

func (m mockRuleStore) ListPricingRules(context.Context) ([]models.PricingRule, error) {
	return m.rules, m.err
}

func TestPricingPrecedence_DBOverridesEnvConfig(t *testing.T) {
	require.NoError(t, configurePricing(`[{"provider":"openai","model_pattern":"gpt-4o","input_per_million":1.0,"output_per_million":2.0}]`))
	t.Cleanup(func() {
		require.NoError(t, configurePricing(""))
	})

	err := LoadPricingRules(context.Background(), mockRuleStore{
		rules: []models.PricingRule{
			{
				Provider:         "openai",
				ModelPattern:     "gpt-4o",
				InputPerMillion:  7.0,
				OutputPerMillion: 11.0,
				Active:           true,
				Priority:         100,
			},
		},
	})
	require.NoError(t, err)

	_, total := computeProxyCost(ProviderOpenAI, "gpt-4o", 1_000_000, 1_000_000)
	assert.InDelta(t, 18.0, total, 0.01)
}

func TestPricingPrecedence_EmptyDBKeepsBootstrapConfig(t *testing.T) {
	require.NoError(t, configurePricing(`[{"provider":"openai","model_pattern":"gpt-4o","input_per_million":1.0,"output_per_million":2.0}]`))
	t.Cleanup(func() {
		require.NoError(t, configurePricing(""))
	})

	err := LoadPricingRules(context.Background(), mockRuleStore{})
	require.NoError(t, err)

	_, total := computeProxyCost(ProviderOpenAI, "gpt-4o", 1_000_000, 1_000_000)
	assert.InDelta(t, 3.0, total, 0.01)
}

func TestResolvePricing_PrefersTenantAndEffectiveWindow(t *testing.T) {
	tenantID := "tenant-123"
	start := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	SetPricingRules([]models.PricingRule{
		{ID: 1, Provider: "openai", ModelPattern: "gpt-4o", InputPerMillion: 5, OutputPerMillion: 15, Active: true, Priority: 100},
		{ID: 2, TenantID: &tenantID, Provider: "openai", ModelPattern: "gpt-4o", InputPerMillion: 9, OutputPerMillion: 19, Active: true, Priority: 200, EffectiveFrom: &start, EffectiveTo: &end},
	})
	t.Cleanup(func() {
		require.NoError(t, configurePricing(""))
	})

	match, _, total := ComputeExactCostForTenant("openai", "gpt-4o-2026-03-01", tenantID, time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC), 1_000_000, 1_000_000)
	assert.Equal(t, int64(2), match.RuleID)
	assert.Equal(t, "tenant", match.Scope)
	assert.InDelta(t, 28.0, total, 0.01)
}
