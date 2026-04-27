package normalization

import (
	"testing"
	"time"
)

func TestMapCursorSuggestionAccepted(t *testing.T) {
	event := map[string]interface{}{
		"source":              "cursor",
		"event_type":          "suggestion.accepted",
		"timestamp":           time.Now().Format(time.RFC3339),
		"user_email":          "dev@company.com",
		"model":               "claude-3-sonnet",
		"suggestion_length":   42,
		"file_path":           "/app/main.py",
		"latency_ms":          1200.0,
		"session_id":          "cursor_sess_123",
	}

	mapper := &CursorMapper{}
	canonical, err := mapper.Map(event)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if canonical.SourceVendor != "cursor" {
		t.Errorf("expected source_vendor 'cursor', got '%s'", canonical.SourceVendor)
	}
	if canonical.EventType != "suggestion.accepted" {
		t.Errorf("expected event_type 'suggestion.accepted', got '%s'", canonical.EventType)
	}
	if canonical.EventCategory != "tool_call" {
		t.Errorf("expected event_category 'tool_call', got '%s'", canonical.EventCategory)
	}
	if canonical.LatencyMs != 1200 {
		t.Errorf("expected latency 1200ms, got %d", canonical.LatencyMs)
	}
}

func TestMapCursorRefactorExecuted(t *testing.T) {
	event := map[string]interface{}{
		"source":        "cursor",
		"event_type":    "refactor.executed",
		"timestamp":     time.Now().Format(time.RFC3339),
		"user_email":    "dev@company.com",
		"model":         "claude-opus",
		"file_path":     "/app/utils.py",
		"latency_ms":    2500.0,
		"session_id":    "cursor_sess_456",
		"lines_changed": 15,
	}

	mapper := &CursorMapper{}
	canonical, err := mapper.Map(event)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if canonical.Action != "refactor" {
		t.Errorf("expected action 'refactor', got '%s'", canonical.Action)
	}
}

func TestMapCursorChatStarted(t *testing.T) {
	event := map[string]interface{}{
		"source":     "cursor",
		"event_type": "chat.started",
		"timestamp":  time.Now().Format(time.RFC3339),
		"user_email": "dev@company.com",
		"model":      "claude-3-sonnet",
		"session_id": "cursor_sess_789",
	}

	mapper := &CursorMapper{}
	canonical, err := mapper.Map(event)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if canonical.EventCategory != "session" {
		t.Errorf("expected event_category 'session', got '%s'", canonical.EventCategory)
	}
	if canonical.Action != "chat_initiated" {
		t.Errorf("expected action 'chat_initiated', got '%s'", canonical.Action)
	}
}

func TestCursorMapper_Accepts(t *testing.T) {
	mapper := &CursorMapper{}

	if !mapper.Accepts("cursor", "cursor-editor") {
		t.Error("should accept cursor vendor")
	}
	if mapper.Accepts("vscode", "cursor-editor") {
		t.Error("should not accept non-cursor vendor")
	}
}

func TestMapCursorLargeSuggestion_HighRisk(t *testing.T) {
	event := map[string]interface{}{
		"source":            "cursor",
		"event_type":        "suggestion.accepted",
		"timestamp":         time.Now().Format(time.RFC3339),
		"user_email":        "dev@company.com",
		"model":             "claude-opus",
		"suggestion_length": 750, // Large suggestion
		"file_path":         "/app/main.py",
		"latency_ms":        800.0,
		"session_id":        "cursor_sess_large",
	}

	mapper := &CursorMapper{}
	canonical, err := mapper.Map(event)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if canonical.RiskScore < 20 {
		t.Errorf("expected elevated risk for large suggestion, got %d", canonical.RiskScore)
	}
}

func TestMapCursorProductionFileEdit_HighRisk(t *testing.T) {
	event := map[string]interface{}{
		"source":        "cursor",
		"event_type":    "suggestion.accepted",
		"timestamp":     time.Now().Format(time.RFC3339),
		"user_email":    "dev@company.com",
		"model":         "claude-opus",
		"file_path":     "/app/prod/config.yaml", // Production file
		"latency_ms":    1200.0,
		"session_id":    "cursor_sess_prod",
	}

	mapper := &CursorMapper{}
	canonical, err := mapper.Map(event)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if canonical.RiskScore < 40 {
		t.Errorf("expected elevated risk for prod file, got %d", canonical.RiskScore)
	}
}
