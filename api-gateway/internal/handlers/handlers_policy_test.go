package handlers

import (
	"testing"

	"github.com/agentfabric/api-gateway/internal/models"
	"github.com/agentfabric/api-gateway/internal/policy"
	"github.com/agentfabric/api-gateway/internal/store"
)

func TestAuditEntriesToPolicyEvents_CorrelatesSpanMetadata(t *testing.T) {
	entries := []store.AuditEntry{{
		DecisionID: "dec-1",
		TraceID:    "trace-1",
		SpanID:     "span-1",
		PolicyName: "deny-gpt4o",
		Result:     "deny",
		Reason:     "provider/model policy matched",
		TenantID:   "tenant-1",
	}}
	spans := []models.Span{{
		ID:       "span-1",
		Provider: "openai",
		Model:    "gpt-4o",
		Attributes: map[string]string{
			"af.policy.scope":      "request",
			"af.policy.matched":    "email,ssn",
			"af.policy.redactions": "2",
		},
	}}

	events := auditEntriesToPolicyEvents(entries, spans)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Provider != "openai" || events[0].Model != "gpt-4o" {
		t.Fatalf("expected provider/model to be correlated from span: %+v", events[0])
	}
	if events[0].Scope != "request" {
		t.Fatalf("expected scope to be hydrated from span attrs")
	}
	if len(events[0].Matched) != 2 {
		t.Fatalf("expected matched detector names to be split from CSV")
	}
	if events[0].Redactions != 2 {
		t.Fatalf("expected redaction count to be hydrated from span attrs")
	}
}

func TestPreviewPolicyDecision_MapsDecisionFields(t *testing.T) {
	preview := previewPolicyDecision(policy.Decision{
		Matched:      true,
		RuleID:       42,
		PolicyName:   "dlp-redact",
		Action:       "redact",
		Reason:       "sensitive content detected",
		Scope:        "response",
		MatchedNames: []string{"email"},
		Redactions:   1,
		RedactedBody: []byte("hello [REDACTED:pii]"),
	})

	if !preview.Matched || preview.RuleID != 42 {
		t.Fatalf("expected preview to preserve match state: %+v", preview)
	}
	if preview.RedactedPreview == "" {
		t.Fatalf("expected redacted preview to be populated")
	}
	if len(preview.MatchedNames) != 1 || preview.MatchedNames[0] != "email" {
		t.Fatalf("expected matched detector names to be preserved")
	}
}
