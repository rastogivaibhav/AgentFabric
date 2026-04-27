package normalization

import (
	"testing"
	"time"
)

func TestMapCoworkSessionStarted(t *testing.T) {
	event := map[string]interface{}{
		"source":     "cowork",
		"event_type": "session.started",
		"timestamp":  time.Now().Format(time.RFC3339),
		"user_email": "dev@company.com",
		"model":      "claude-opus",
		"session_id": "cowork_sess_123",
	}

	mapper := &CoworkMapper{}
	canonical, err := mapper.Map(event)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if canonical.SourceVendor != "cowork" {
		t.Errorf("expected source_vendor 'cowork', got '%s'", canonical.SourceVendor)
	}
	if canonical.EventCategory != "session" {
		t.Errorf("expected event_category 'session', got '%s'", canonical.EventCategory)
	}
	if canonical.Provider != "anthropic" {
		t.Errorf("expected provider 'anthropic', got '%s'", canonical.Provider)
	}
}

func TestMapCoworkSuggestionAccepted(t *testing.T) {
	event := map[string]interface{}{
		"source":            "cowork",
		"event_type":        "suggestion.accepted",
		"timestamp":         time.Now().Format(time.RFC3339),
		"user_email":        "dev@company.com",
		"model":             "claude-sonnet",
		"suggestion_length": 150,
		"file_path":         "/app/components/Button.tsx",
		"latency_ms":        2000.0,
		"session_id":        "cowork_sess_456",
	}

	mapper := &CoworkMapper{}
	canonical, err := mapper.Map(event)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if canonical.EventType != "suggestion.accepted" {
		t.Errorf("expected event_type 'suggestion.accepted', got '%s'", canonical.EventType)
	}
	if canonical.Action != "code_generation_accepted" {
		t.Errorf("expected action 'code_generation_accepted', got '%s'", canonical.Action)
	}
}

func TestMapCoworkReviewRequested(t *testing.T) {
	event := map[string]interface{}{
		"source":     "cowork",
		"event_type": "review.requested",
		"timestamp":  time.Now().Format(time.RFC3339),
		"user_email": "dev@company.com",
		"model":      "claude-opus",
		"file_path":  "/app/service.go",
		"latency_ms": 3000.0,
		"session_id": "cowork_sess_789",
	}

	mapper := &CoworkMapper{}
	canonical, err := mapper.Map(event)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if canonical.EventCategory != "approval" {
		t.Errorf("expected event_category 'approval', got '%s'", canonical.EventCategory)
	}
}

func TestMapCoworkRefactorSuggested(t *testing.T) {
	event := map[string]interface{}{
		"source":        "cowork",
		"event_type":    "refactor.suggested",
		"timestamp":     time.Now().Format(time.RFC3339),
		"user_email":    "dev@company.com",
		"model":         "claude-opus",
		"file_path":     "/app/utils.js",
		"lines_changed": 25,
		"latency_ms":    1500.0,
		"session_id":    "cowork_sess_012",
	}

	mapper := &CoworkMapper{}
	canonical, err := mapper.Map(event)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if canonical.Action != "refactor_suggested" {
		t.Errorf("expected action 'refactor_suggested', got '%s'", canonical.Action)
	}
}

func TestCoworkMapper_Accepts(t *testing.T) {
	mapper := &CoworkMapper{}

	if !mapper.Accepts("cowork", "") {
		t.Error("should accept cowork vendor")
	}
	if mapper.Accepts("continue", "") {
		t.Error("should not accept non-cowork vendor")
	}
}

func TestMapCoworkLargeSuggestion_HighRisk(t *testing.T) {
	event := map[string]interface{}{
		"source":            "cowork",
		"event_type":        "suggestion.accepted",
		"timestamp":         time.Now().Format(time.RFC3339),
		"user_email":        "dev@company.com",
		"model":             "claude-opus",
		"suggestion_length": 800, // Large suggestion
		"file_path":         "/app/main.py",
		"latency_ms":        2000.0,
		"session_id":        "cowork_large",
	}

	mapper := &CoworkMapper{}
	canonical, err := mapper.Map(event)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if canonical.RiskScore < 30 {
		t.Errorf("expected elevated risk for large suggestion, got %d", canonical.RiskScore)
	}
}

func TestMapCoworkProductionRefactor_HighRisk(t *testing.T) {
	event := map[string]interface{}{
		"source":        "cowork",
		"event_type":    "refactor.suggested",
		"timestamp":     time.Now().Format(time.RFC3339),
		"user_email":    "dev@company.com",
		"model":         "claude-opus",
		"file_path":     "/app/prod/database.go", // Production file
		"lines_changed": 80,
		"latency_ms":    2000.0,
		"session_id":    "cowork_prod_refactor",
	}

	mapper := &CoworkMapper{}
	canonical, err := mapper.Map(event)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if canonical.RiskScore < 50 {
		t.Errorf("expected high risk for prod refactor, got %d", canonical.RiskScore)
	}
}

func TestMapCoworkHighTokenUsage_Warning(t *testing.T) {
	event := map[string]interface{}{
		"source":        "cowork",
		"event_type":    "suggestion.accepted",
		"timestamp":     time.Now().Format(time.RFC3339),
		"user_email":    "dev@company.com",
		"model":         "claude-opus",
		"input_tokens":  150000,
		"output_tokens": 80000,
		"latency_ms":    3000.0,
		"session_id":    "cowork_high_tokens",
	}

	mapper := &CoworkMapper{}
	canonical, err := mapper.Map(event)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if canonical.RiskScore < 20 {
		t.Errorf("expected elevated risk for high token usage, got %d", canonical.RiskScore)
	}
}
