package policy

import (
	"path/filepath"
	"testing"
)

func TestLoadPolicyPackDefinitions(t *testing.T) {
	root := filepath.Join("..", "..", "..", "deploy", "seed", "policy-packs")
	packs, err := LoadPackDefinitions(root)
	if err != nil {
		t.Fatalf("load policy packs: %v", err)
	}
	if len(packs) < 10 {
		t.Fatalf("expected seeded policy packs, got %d", len(packs))
	}
}

func TestCompiledNativePackRule_DeniesMissingLawfulBasis(t *testing.T) {
	root := filepath.Join("..", "..", "..", "deploy", "seed", "policy-packs")
	pack, err := GetPackDefinition(root, "pack.gdpr.eu.v1")
	if err != nil {
		t.Fatalf("get pack: %v", err)
	}
	rules, err := CompilePackRules(pack, nil, true)
	if err != nil {
		t.Fatalf("compile pack: %v", err)
	}
	if len(rules) == 0 {
		t.Fatal("expected compiled rules")
	}
	for idx := range rules {
		rules[idx].ID = int64(idx + 1)
	}

	engine := NewEngine()
	engine.rules = rules
	engine.compiled = compileRules(rules)

	decision := engine.EvaluateTraffic(TrafficInput{
		RequestBody: []byte(`{}`),
	})
	if decision.Matched {
		t.Fatalf("unexpected match without context attributes: %#v", decision)
	}

	traffic, _, _ := engine.Evaluate(EvaluationInput{
		Attributes: map[string]any{
			"context": map[string]any{
				"processing_purpose":  "support_resolution",
				"data_subject_region": "eu",
			},
		},
	})
	if !traffic.Matched {
		t.Fatal("expected native pack rule to match")
	}
	if traffic.Action != "deny" {
		t.Fatalf("expected deny action, got %q", traffic.Action)
	}
}
