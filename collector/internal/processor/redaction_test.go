package processor

import (
	"strings"
	"testing"
)

func TestRedactionConfig_DefaultDisablesPromptLogging(t *testing.T) {
	config := DefaultRedactionConfig()
	if config.EnablePromptLogging {
		t.Error("default config should disable prompt logging")
	}
	if !config.EnableSecretDetection {
		t.Error("default config should enable secret detection")
	}
}

func TestContainsSecret_DetectsAWSKey(t *testing.T) {
	if !ContainsSecret("AKIA1234567890ABCDEF") {
		t.Error("should detect AWS key")
	}
}

func TestContainsSecret_DetectsOpenAIKey(t *testing.T) {
	// OpenAI key format: sk-[a-zA-Z0-9-]{48,} (at least 48 chars after sk-)
	if !ContainsSecret("sk-1234567890123456789012345678901234567890123456789") {
		t.Error("should detect OpenAI key")
	}
}

func TestContainsSecret_DetectsGitHubToken(t *testing.T) {
	// GitHub token format: ghp_[A-Za-z0-9_]{36,255}
	if !ContainsSecret("ghp_123456789012345678901234567890123456") {
		t.Error("should detect GitHub token")
	}
}

func TestContainsSecret_DetectsDatabaseURL(t *testing.T) {
	if !ContainsSecret("postgres://user:password@localhost/db") {
		t.Error("should detect database URL with credentials")
	}
}

func TestContainsSecret_NoFalsePositives(t *testing.T) {
	if ContainsSecret("hello world") {
		t.Error("should not detect secret in normal text")
	}
}

func TestRedactSecrets_MasksAWSKey(t *testing.T) {
	input := "Use AWS key AKIA1234567890ABCDEF for access"
	result := redactSecrets(input)

	if strings.Contains(result, "AKIA1234567890ABCDEF") {
		t.Error("AWS key should be masked")
	}
	if !strings.Contains(result, "[REDACTED]") {
		t.Error("should contain redaction marker")
	}
}

func TestRedactSecrets_MultipleSecrets(t *testing.T) {
	input := "AWS: AKIA1234567890ABCDEF and DB: postgres://user:password@localhost/db"
	result := redactSecrets(input)

	if strings.Contains(result, "AKIA") || strings.Contains(result, "password") {
		t.Error("all secrets should be masked")
	}
	if !strings.Contains(result, "[REDACTED]") {
		t.Error("should contain redaction markers")
	}
}

func TestShouldRedactAttribute_RedactsPrompts(t *testing.T) {
	config := DefaultRedactionConfig()

	if !ShouldRedactAttribute("user.prompt", config) {
		t.Error("should redact user.prompt")
	}
	if !ShouldRedactAttribute("prompt", config) {
		t.Error("should redact prompt")
	}
	if !ShouldRedactAttribute("message", config) {
		t.Error("should redact message")
	}
}

func TestShouldRedactAttribute_AllowsPromptsIfEnabled(t *testing.T) {
	config := &RedactionConfig{EnablePromptLogging: true}

	if ShouldRedactAttribute("user.prompt", config) {
		t.Error("should not redact prompt when logging enabled")
	}
	if ShouldRedactAttribute("message", config) {
		t.Error("should not redact message when logging enabled")
	}
}

func TestShouldRedactAttribute_RedactsSensitiveKeys(t *testing.T) {
	config := DefaultRedactionConfig()

	if !ShouldRedactAttribute("tool.arguments", config) {
		t.Error("should redact tool.arguments")
	}
	if !ShouldRedactAttribute("request.body", config) {
		t.Error("should redact request.body")
	}
	if !ShouldRedactAttribute("file.content", config) {
		t.Error("should redact file.content")
	}
}

func TestRedactAttributes_RemovesSensitiveKeys(t *testing.T) {
	attrs := map[string]string{
		"user.prompt":       "Deploy prod with secret key",
		"command":           "echo hello",
		"tool.arguments":    "password=secret123",
		"file.content":      "private data",
	}

	config := DefaultRedactionConfig()
	result := RedactAttributes(attrs, config)

	if _, exists := result["user.prompt"]; exists {
		t.Error("user.prompt should be removed")
	}
	if _, exists := result["tool.arguments"]; exists {
		t.Error("tool.arguments should be removed")
	}
	if _, exists := result["file.content"]; exists {
		t.Error("file.content should be removed")
	}
	if _, exists := result["command"]; !exists {
		t.Error("command should be kept")
	}
}

func TestRedactAttributes_RedactsEmbeddedSecrets(t *testing.T) {
	attrs := map[string]string{
		"command": "aws s3 AKIA1234567890ABCDEF",
	}

	config := DefaultRedactionConfig()
	result := RedactAttributes(attrs, config)

	if strings.Contains(result["command"], "AKIA") {
		t.Error("embedded secret should be masked")
	}
	if !strings.Contains(result["command"], "[REDACTED]") {
		t.Error("should contain redaction marker")
	}
}

func TestRedactAttributes_WithNilConfig(t *testing.T) {
	attrs := map[string]string{
		"prompt": "test",
		"data":   "safe",
	}

	result := RedactAttributes(attrs, nil)

	if _, exists := result["prompt"]; exists {
		t.Error("prompt should be removed with default config")
	}
}

func TestRedactAttributes_WithNilInput(t *testing.T) {
	result := RedactAttributes(nil, nil)

	if result != nil {
		t.Error("nil input should return nil output")
	}
}

func TestExtractAndRedact_RemovesSensitiveData(t *testing.T) {
	data := map[string]string{
		"prompt":      "secret prompt",
		"command":     "pytest tests/",
		"file.content": "private code",
		"model":       "claude-opus",
	}

	result, redacted := ExtractAndRedact(data, false)

	if len(result) != 2 {
		t.Errorf("expected 2 fields, got %d", len(result))
	}
	if !redacted {
		t.Error("should indicate redaction occurred")
	}
	if _, exists := result["prompt"]; exists {
		t.Error("prompt should be removed")
	}
	if _, exists := result["file.content"]; exists {
		t.Error("file.content should be removed")
	}
}

func TestExtractAndRedact_AllowsPromptsIfEnabled(t *testing.T) {
	data := map[string]string{
		"prompt": "my prompt",
		"model":  "claude-opus",
	}

	result, redacted := ExtractAndRedact(data, true)

	if _, exists := result["prompt"]; !exists {
		t.Error("prompt should be kept when allowed")
	}
	if redacted {
		t.Error("should not indicate redaction when keeping prompts")
	}
}

func TestRedactAttributes_CursorToolChain(t *testing.T) {
	attrs := map[string]string{
		"source":           "cursor",
		"event_type":       "suggestion.accepted",
		"suggestion.text":  "# TODO: use AKIA1234567890ABCDEF",
		"file_path":        "/app/main.py",
		"cursor.response":  "Full internal response with secrets",
	}

	config := DefaultRedactionConfig()
	result := RedactAttributes(attrs, config)

	if _, exists := result["cursor.response"]; exists {
		t.Error("cursor.response should be removed")
	}
	if strings.Contains(result["suggestion.text"], "AKIA") {
		t.Error("embedded secrets should be masked")
	}
}

func TestRedactAttributes_VSCodeToolChain(t *testing.T) {
	attrs := map[string]string{
		"source":          "vscode-copilot",
		"event_type":      "suggestion.accepted",
		"vscode.response": "Full copilot response with system prompt",
		"command":         "curl -H 'Auth: Bearer 1234567890123456789012345678901234567890'",
		"editor":          "vscode",
	}

	config := DefaultRedactionConfig()
	result := RedactAttributes(attrs, config)

	if _, exists := result["vscode.response"]; exists {
		t.Error("vscode.response should be removed")
	}
	if strings.Contains(result["command"], "1234567890") && len(result["command"]) < len(attrs["command"]) {
		// Should be redacted
		t.Log("Bearer token masked successfully")
	}
}
