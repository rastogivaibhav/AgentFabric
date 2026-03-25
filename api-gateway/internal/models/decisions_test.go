package models

import "testing"

func TestNormalizeDecisionResult(t *testing.T) {
	if got := NormalizeDecisionResult("redact"); got != "sanitize" {
		t.Fatalf("expected redact -> sanitize, got %q", got)
	}
	if got := NormalizeDecisionResult("block"); got != "deny" {
		t.Fatalf("expected block -> deny, got %q", got)
	}
}

func TestDefaultDecisionExplanation_PolicyDeny(t *testing.T) {
	record := DecisionRecord{
		Type:   DecisionTypePolicy,
		Result: "deny",
		Reason: "provider/model policy matched",
		Model:  "gpt-4o",
	}
	got := DefaultDecisionExplanation(record)
	if got == "" || got == record.Reason {
		t.Fatalf("expected synthesized explanation, got %q", got)
	}
}

func TestDefaultDecisionExplanation_Fallback(t *testing.T) {
	record := DecisionRecord{
		Type:   DecisionTypeFallback,
		Result: "retry",
		Reason: "upstream_error",
	}
	got := DefaultDecisionExplanation(record)
	if got == "" {
		t.Fatalf("expected fallback explanation")
	}
}
