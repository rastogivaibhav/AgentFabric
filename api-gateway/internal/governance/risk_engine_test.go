package governance

import (
	"testing"
	"time"

	"github.com/govagn/api-gateway/internal/normalization"
)

func TestRiskEngine_FlagsUnsafeShellCommand(t *testing.T) {
	engine := NewRiskEngine()
	event := &normalization.CanonicalEvent{
		SourceVendor:  "vscode",
		EventCategory: "tool_call",
		Action:        "shell_command",
		ToolName:      "shell",
		Command:       "rm -rf /production",
		EventTime:     time.Now(),
	}

	score, category := engine.Score(event)
	if score < 80 {
		t.Errorf("expected risk score >= 80 for 'rm -rf', got %d", score)
	}
	if category != "dangerous_command" {
		t.Errorf("expected category 'dangerous_command', got '%s'", category)
	}
}

func TestRiskEngine_FlagsSecretExposure(t *testing.T) {
	engine := NewRiskEngine()
	event := &normalization.CanonicalEvent{
		SourceVendor: "cursor",
		EventTime:    time.Now(),
		Command:      "aws s3 cp --arn AKIA1234567890ABCDEF",
	}

	score, category := engine.Score(event)
	if score < 90 {
		t.Errorf("expected risk score >= 90 for secret, got %d", score)
	}
	if category != "secret_exposure" {
		t.Errorf("expected category 'secret_exposure', got '%s'", category)
	}
}

func TestRiskEngine_FlagsProductionFileModification(t *testing.T) {
	engine := NewRiskEngine()
	event := &normalization.CanonicalEvent{
		SourceVendor:  "cursor",
		EventCategory: "tool_call",
		Action:        "file_edit",
		FilePath:      "/app/prod/config.yaml",
		EventTime:     time.Now(),
	}

	score, category := engine.Score(event)
	if score < 60 {
		t.Errorf("expected risk score >= 60 for prod file edit, got %d", score)
	}
	if category != "prod_edit" {
		t.Errorf("expected category 'prod_edit', got '%s'", category)
	}
}

func TestRiskEngine_FlagsHighTokenUsage(t *testing.T) {
	engine := NewRiskEngine()
	event := &normalization.CanonicalEvent{
		SourceVendor:  "codex",
		InputTokens:   150000,
		OutputTokens:  100000,
		EventTime:     time.Now(),
	}

	score, category := engine.Score(event)
	if score < 30 {
		t.Errorf("expected risk score >= 30 for high token usage, got %d", score)
	}
	if category != "high_token_usage" {
		t.Errorf("expected category 'high_token_usage', got '%s'", category)
	}
}

func TestRiskEngine_LowRiskForNormalOperation(t *testing.T) {
	engine := NewRiskEngine()
	event := &normalization.CanonicalEvent{
		SourceVendor:  "claude_code",
		EventCategory: "tool_call",
		Action:        "code_generation",
		ToolName:      "editor",
		Command:       "open file",
		InputTokens:   1000,
		OutputTokens:  500,
		EventTime:     time.Now(),
	}

	score, category := engine.Score(event)
	if score > 20 {
		t.Errorf("expected low risk score for normal operation, got %d", score)
	}
	if category != "none" {
		t.Errorf("expected category 'none', got '%s'", category)
	}
}

func TestRiskEngine_AddCustomRule(t *testing.T) {
	engine := NewRiskEngine()

	customRule := &RiskRule{
		Name:     "test_rule",
		Category: "test",
		Score:    75,
		Match: func(e *normalization.CanonicalEvent) bool {
			return e.SourceVendor == "test"
		},
	}

	engine.AddRule(customRule)

	event := &normalization.CanonicalEvent{
		SourceVendor: "test",
		EventTime:    time.Now(),
	}

	score, category := engine.Score(event)
	if score != 75 {
		t.Errorf("expected risk score 75 from custom rule, got %d", score)
	}
	if category != "test" {
		t.Errorf("expected category 'test', got '%s'", category)
	}
}

func TestRiskEngine_ListRules(t *testing.T) {
	engine := NewRiskEngine()
	rules := engine.ListRules()

	if len(rules) == 0 {
		t.Error("expected at least one default rule")
	}

	// Check for expected rule names
	expectedRules := map[string]bool{
		"dangerous_shell_command":     false,
		"secret_exposure":             false,
		"production_file_modification": false,
	}

	for _, rule := range rules {
		if _, exists := expectedRules[rule.Name]; exists {
			expectedRules[rule.Name] = true
		}
	}

	for ruleName, found := range expectedRules {
		if !found {
			t.Errorf("expected rule '%s' not found", ruleName)
		}
	}
}

func TestRiskEngine_SecretPatterns(t *testing.T) {
	tests := []struct {
		name   string
		secret string
		want   bool
	}{
		{"AWS Key", "AKIA1234567890ABCDEF", true},
		{"OpenAI Key", "sk-proj-abc123def456", true},
		{"GitHub Token", "ghp_abcdefghijklmnopqrstuvwxyz123456", true},
		{"Anthropic Key", "sk-ant-abcdefghijklmnopqrstuvwxyz123456", true},
		{"Database URL", "postgres://user:password@localhost/db", true},
		{"Private Key", "-----BEGIN PRIVATE KEY-----", true},
		{"Not a secret", "hello world", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesSecretPattern(tt.secret)
			if got != tt.want {
				t.Errorf("matchesSecretPattern(%q) = %v, want %v", tt.secret, got, tt.want)
			}
		})
	}
}

func TestRiskEngine_HandleNilEvent(t *testing.T) {
	engine := NewRiskEngine()
	score, category := engine.Score(nil)

	if score != 0 {
		t.Errorf("expected score 0 for nil event, got %d", score)
	}
	if category != "none" {
		t.Errorf("expected category 'none' for nil event, got '%s'", category)
	}
}

func TestRiskEngine_MultipleRulesMaxScoreTakesPriority(t *testing.T) {
	engine := NewRiskEngine()

	event := &normalization.CanonicalEvent{
		SourceVendor:  "cursor",
		EventCategory: "tool_call",
		Action:        "file_edit",
		ToolName:      "mcp",
		FilePath:      "/app/prod/secret.env",
		EventTime:     time.Now(),
	}

	score, category := engine.Score(event)

	// Should match both prod_edit (70) and mcp_usage (30)
	// Should return the max score from prod_edit
	if score < 70 {
		t.Errorf("expected max risk score >= 70, got %d", score)
	}
	if category != "prod_edit" {
		t.Errorf("expected category 'prod_edit', got '%s'", category)
	}
}
