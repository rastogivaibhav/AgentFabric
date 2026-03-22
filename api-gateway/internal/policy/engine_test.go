package policy

import (
	"testing"

	"github.com/agentfabric/api-gateway/internal/models"
)

func TestEvaluateTraffic_PrefersTenantSpecificMatch(t *testing.T) {
	tenantID := "tenant-a"
	engine := &Engine{
		rules: []models.PolicyRule{
			{
				ID:           1,
				Name:         "global-openai",
				RuleType:     "traffic",
				Enabled:      true,
				Priority:     50,
				Action:       "warn",
				Provider:     "openai",
				ModelPattern: "gpt-4o",
			},
			{
				ID:           2,
				Name:         "tenant-openai",
				RuleType:     "traffic",
				Enabled:      true,
				Priority:     10,
				Action:       "deny",
				Provider:     "openai",
				ModelPattern: "gpt-4o",
				TenantID:     &tenantID,
			},
		},
	}

	decision := engine.EvaluateTraffic(TrafficInput{
		TenantID:        tenantID,
		Provider:        "openai",
		Model:           "gpt-4o-mini-2026-01-01",
		Environment:     "production",
		EstimatedTokens: 1200,
	})

	if !decision.Matched {
		t.Fatal("expected traffic policy to match")
	}
	if decision.RuleID != 2 {
		t.Fatalf("expected tenant-specific rule to win, got %d", decision.RuleID)
	}
	if decision.Action != "deny" {
		t.Fatalf("expected deny action, got %q", decision.Action)
	}
}

func TestEvaluateTraffic_HonorsTokenCeiling(t *testing.T) {
	engine := &Engine{
		rules: []models.PolicyRule{
			{
				ID:           10,
				Name:         "max-tokens",
				RuleType:     "traffic",
				Enabled:      true,
				Priority:     100,
				Action:       "deny",
				Provider:     "anthropic",
				ModelPattern: "claude",
				MaxTokens:    500,
			},
		},
	}

	noMatch := engine.EvaluateTraffic(TrafficInput{
		Provider:        "anthropic",
		Model:           "claude-3-5-sonnet",
		EstimatedTokens: 500,
	})
	if noMatch.Matched {
		t.Fatal("expected no match when estimated tokens are within limit")
	}

	match := engine.EvaluateTraffic(TrafficInput{
		Provider:        "anthropic",
		Model:           "claude-3-5-sonnet",
		EstimatedTokens: 501,
	})
	if !match.Matched {
		t.Fatal("expected token ceiling rule to match")
	}
	if match.Reason == "" {
		t.Fatal("expected token ceiling match to include a reason")
	}
}

func TestEvaluateDLP_RedactsDetectedSecrets(t *testing.T) {
	engine := &Engine{
		rules: []models.PolicyRule{
			{
				ID:       20,
				Name:     "redact-secrets",
				RuleType: "dlp",
				Enabled:  true,
				Priority: 100,
				Action:   "redact",
				Detector: "secret",
				Scope:    "request",
			},
		},
	}

	decision := engine.EvaluateDLP(DLPInput{
		Provider:    "openai",
		Model:       "gpt-4o",
		Environment: "production",
		Scope:       "request",
		Body:        []byte(`{"prompt":"ship with key sk-abcdefghijklmnopqrstuvwxyz12345"}`),
	})

	if !decision.Matched {
		t.Fatal("expected DLP rule to match")
	}
	if decision.Action != "redact" {
		t.Fatalf("expected redact action, got %q", decision.Action)
	}
	if decision.Redactions == 0 {
		t.Fatal("expected at least one redaction")
	}
	if got := string(decision.RedactedBody); got == "" || got == `{"prompt":"ship with key sk-abcdefghijklmnopqrstuvwxyz12345"}` {
		t.Fatalf("expected redacted body, got %q", got)
	}
}

func TestEvaluateDLP_RespectsScope(t *testing.T) {
	engine := &Engine{
		rules: []models.PolicyRule{
			{
				ID:       30,
				Name:     "response-pii",
				RuleType: "dlp",
				Enabled:  true,
				Priority: 100,
				Action:   "deny",
				Detector: "pii",
				Scope:    "response",
			},
		},
	}

	requestDecision := engine.EvaluateDLP(DLPInput{
		Scope: "request",
		Body:  []byte(`{"email":"alice@example.com"}`),
	})
	if requestDecision.Matched {
		t.Fatal("expected response-only rule not to match request scope")
	}

	responseDecision := engine.EvaluateDLP(DLPInput{
		Scope: "response",
		Body:  []byte(`{"email":"alice@example.com"}`),
	})
	if !responseDecision.Matched {
		t.Fatal("expected response-only rule to match response scope")
	}
	if responseDecision.Action != "deny" {
		t.Fatalf("expected deny action, got %q", responseDecision.Action)
	}
}

func TestEvaluateTraffic_RegoDecisionMode(t *testing.T) {
	engine := &Engine{
		rules: []models.PolicyRule{
			{
				ID:           40,
				Name:         "rego-prod-openai-limit",
				RuleType:     "traffic",
				DecisionMode: "rego",
				Enabled:      true,
				Priority:     100,
				Action:       "deny",
				Provider:     "openai",
				ModelPattern: "gpt-4o",
				RegoModule:   `deny if input.environment == "production" && input.estimated_tokens > 1000`,
			},
		},
	}
	engine.compiled = compileRules(engine.rules)

	decision := engine.EvaluateTraffic(TrafficInput{
		Provider:        "openai",
		Model:           "gpt-4o-mini",
		Environment:     "production",
		EstimatedTokens: 1500,
	})

	if !decision.Matched {
		t.Fatal("expected rego-mode policy to match")
	}
	if decision.Explanation.Engine != "rego-adapter" {
		t.Fatalf("expected rego-adapter engine, got %q", decision.Explanation.Engine)
	}
	if len(decision.Explanation.ConditionTrace) == 0 {
		t.Fatal("expected condition trace to be populated")
	}
}

func TestEvaluateTraffic_RuleConditions(t *testing.T) {
	engine := &Engine{
		rules: []models.PolicyRule{
			{
				ID:           41,
				Name:         "ops-ui-limit",
				RuleType:     "traffic",
				DecisionMode: "fast",
				Enabled:      true,
				Priority:     100,
				Action:       "warn",
				Provider:     "openai",
				ModelPattern: "gpt-4o",
				RuleConditions: map[string]string{
					"app": "ops-ui",
				},
			},
		},
	}
	engine.compiled = compileRules(engine.rules)

	decision := engine.EvaluateTraffic(TrafficInput{
		Provider: "openai",
		Model:    "gpt-4o-mini",
		App:      "ops-ui",
	})

	if !decision.Matched {
		t.Fatal("expected rule_conditions-based policy to match")
	}
	if len(decision.Explanation.MatchedFields) == 0 {
		t.Fatal("expected explanation matched fields")
	}
}
