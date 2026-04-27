package normalization

import (
	"testing"
	"time"
)

func TestMapGitHubCopilotSuggestionAccepted(t *testing.T) {
	event := map[string]interface{}{
		"source":              "github-copilot",
		"event_type":          "suggestion.accepted",
		"timestamp":           time.Now().Format(time.RFC3339),
		"user_email":          "dev@company.com",
		"model":               "gpt-4",
		"suggestion_length":   30,
		"file_path":           "/app/index.js",
		"latency_ms":          800.0,
		"session_id":          "copilot_sess_123",
	}

	mapper := &VSCodeMapper{}
	canonical, err := mapper.Map(event)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if canonical.SourceVendor != "vscode" {
		t.Errorf("expected source_vendor 'vscode', got '%s'", canonical.SourceVendor)
	}
	if canonical.SourceProduct != "github-copilot" {
		t.Errorf("expected source_product 'github-copilot', got '%s'", canonical.SourceProduct)
	}
	if canonical.Provider != "openai" {
		t.Errorf("expected provider 'openai', got '%s'", canonical.Provider)
	}
}

func TestMapContinueAgentRequest(t *testing.T) {
	event := map[string]interface{}{
		"source":       "continue",
		"event_type":   "code.generated",
		"timestamp":    time.Now().Format(time.RFC3339),
		"user_email":   "dev@company.com",
		"model":        "claude-3-sonnet",
		"file_path":    "/app/utils.py",
		"latency_ms":   1200.0,
		"session_id":   "continue_sess_456",
	}

	mapper := &VSCodeMapper{}
	canonical, err := mapper.Map(event)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if canonical.SourceProduct != "continue" {
		t.Errorf("expected source_product 'continue', got '%s'", canonical.SourceProduct)
	}
	if canonical.Provider != "anthropic" {
		t.Errorf("expected provider 'anthropic', got '%s'", canonical.Provider)
	}
}

func TestMapRooCodeRefactoring(t *testing.T) {
	event := map[string]interface{}{
		"source":        "roo-code",
		"event_type":    "refactor.completed",
		"timestamp":     time.Now().Format(time.RFC3339),
		"user_email":    "dev@company.com",
		"model":         "claude-opus",
		"file_path":     "/app/service.go",
		"latency_ms":    2500.0,
		"session_id":    "roo_sess_789",
		"lines_changed": 45,
	}

	mapper := &VSCodeMapper{}
	canonical, err := mapper.Map(event)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if canonical.SourceProduct != "roo-code" {
		t.Errorf("expected source_product 'roo-code', got '%s'", canonical.SourceProduct)
	}
}

func TestMapClineCommand(t *testing.T) {
	event := map[string]interface{}{
		"source":     "cline",
		"event_type": "terminal.command",
		"timestamp":  time.Now().Format(time.RFC3339),
		"user_email": "dev@company.com",
		"model":      "claude-opus",
		"command":    "npm test",
		"latency_ms": 5000.0,
		"session_id": "cline_sess_012",
	}

	mapper := &VSCodeMapper{}
	canonical, err := mapper.Map(event)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if canonical.SourceProduct != "cline" {
		t.Errorf("expected source_product 'cline', got '%s'", canonical.SourceProduct)
	}
	if canonical.EventCategory != "tool_call" {
		t.Errorf("expected event_category 'tool_call', got '%s'", canonical.EventCategory)
	}
}

func TestVSCodeMapper_AcceptsAllTools(t *testing.T) {
	mapper := &VSCodeMapper{}

	tests := []struct {
		vendor string
		want   bool
	}{
		{"github-copilot", true},
		{"continue", true},
		{"roo-code", true},
		{"cline", true},
		{"cursor", false},
		{"codex", false},
	}

	for _, tt := range tests {
		if got := mapper.Accepts(tt.vendor, ""); got != tt.want {
			t.Errorf("Accepts(%q) = %v, want %v", tt.vendor, got, tt.want)
		}
	}
}

func TestMapVSCodeLargeSuggestion_HighRisk(t *testing.T) {
	event := map[string]interface{}{
		"source":            "github-copilot",
		"event_type":        "suggestion.accepted",
		"timestamp":         time.Now().Format(time.RFC3339),
		"user_email":        "dev@company.com",
		"model":             "gpt-4",
		"suggestion_length": 600, // Large suggestion
		"file_path":         "/app/main.js",
		"latency_ms":        900.0,
		"session_id":        "copilot_large",
	}

	mapper := &VSCodeMapper{}
	canonical, err := mapper.Map(event)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if canonical.RiskScore < 20 {
		t.Errorf("expected elevated risk for large suggestion, got %d", canonical.RiskScore)
	}
}

func TestMapVSCodeProductionFile_HighRisk(t *testing.T) {
	event := map[string]interface{}{
		"source":      "cline",
		"event_type":  "code.generated",
		"timestamp":   time.Now().Format(time.RFC3339),
		"user_email":  "dev@company.com",
		"model":       "claude-opus",
		"file_path":   "/app/k8s/deployment.yaml", // Production file
		"latency_ms":  1500.0,
		"session_id":  "cline_prod",
	}

	mapper := &VSCodeMapper{}
	canonical, err := mapper.Map(event)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if canonical.RiskScore < 40 {
		t.Errorf("expected elevated risk for prod file, got %d", canonical.RiskScore)
	}
}

func TestMapVSCodeTerminalCommand_HighRisk(t *testing.T) {
	event := map[string]interface{}{
		"source":     "cline",
		"event_type": "terminal.command",
		"timestamp":  time.Now().Format(time.RFC3339),
		"user_email": "dev@company.com",
		"model":      "claude-opus",
		"command":    "rm -rf /production",
		"latency_ms": 500.0,
		"session_id": "cline_danger",
	}

	mapper := &VSCodeMapper{}
	canonical, err := mapper.Map(event)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if canonical.RiskScore < 80 {
		t.Errorf("expected high risk for dangerous command, got %d", canonical.RiskScore)
	}
}
