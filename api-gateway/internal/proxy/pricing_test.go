package proxy

import (
	"context"
	"testing"

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
