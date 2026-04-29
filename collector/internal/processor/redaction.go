package processor

import (
	"regexp"
	"strings"
)

// RedactionConfig defines redaction behavior
type RedactionConfig struct {
	EnablePromptLogging   bool // If false, prompts are always redacted
	EnableSecretDetection bool
}

// DefaultRedactionConfig returns conservative defaults (maximum privacy)
func DefaultRedactionConfig() *RedactionConfig {
	return &RedactionConfig{
		EnablePromptLogging:   false, // Prompts never stored by default
		EnableSecretDetection: true,  // Always detect and redact secrets
	}
}

// RedactionPatterns defines patterns for detecting sensitive data
var redactionPatterns = []*regexp.Regexp{
	// AWS credentials
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	// OpenAI/Anthropic keys
	regexp.MustCompile(`sk-(?:proj-)?[a-zA-Z0-9-]{48,}`),
	regexp.MustCompile(`sk-ant-[a-zA-Z0-9_]{36,}`),
	// GitHub tokens
	regexp.MustCompile(`ghp_[A-Za-z0-9_]{36,255}`),
	// GitHub OAuth tokens
	regexp.MustCompile(`ghu_[0-9a-zA-Z_]{36,255}`),
	// Database URLs with credentials
	regexp.MustCompile(`(?i)(?:postgres|mysql|mongodb|redis|oracle)://[^:]+:[^@]+@`),
	// Private keys
	regexp.MustCompile(`-----BEGIN (?:RSA |OPENSSH )?PRIVATE KEY-----`),
	// .env style secrets
	regexp.MustCompile(`(?i)(password|passwd|secret|api_key|token|access_key|secret_key|private_key)\s*=\s*\S+`),
	// Bearer tokens
	regexp.MustCompile(`Bearer\s+[A-Za-z0-9._\-~+/]+=*`),
	// JWT tokens
	regexp.MustCompile(`eyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`),
}

// SensitiveKeys defines JSONB keys and attribute names that should never be stored
var sensitiveKeys = map[string]bool{
	// User prompts and inputs
	"user.prompt":  true,
	"prompt":       true,
	"user_prompt":  true,
	"input_prompt": true,
	"message":      true,
	"user_message": true,
	"user_input":   true,
	"query":        true,
	// Tool arguments
	"tool.arguments": true,
	"arguments":      true,
	"args":           true,
	"tool_args":      true,
	"tool_arguments": true,
	// Request/response bodies
	"request.body":  true,
	"response.body": true,
	"request_body":  true,
	"response_body": true,
	"body":          true,
	// File contents
	"file.content":  true,
	"file_content":  true,
	"code_content":  true,
	"file_contents": true,
	"content":       true,
	// Full commands (containing potential secrets)
	"command.full": true,
	"full_command": true,
	// IDE-specific fields
	"cursor.full_response":  true,
	"cursor.response":       true,
	"cursor_response":       true,
	"vscode.full_response":  true,
	"vscode.response":       true,
	"vscode_response":       true,
	"copilot.full_response": true,
	// Cowork-specific
	"cowork.instructions":  true,
	"cowork.system_prompt": true,
	"instructions":         true,
	"system_prompt":        true,
}

// ExtractAndRedact removes and masks sensitive data from attributes/payloads
// Returns cleaned data and a boolean indicating if any redaction was performed
func ExtractAndRedact(data map[string]string, allowPrompts bool) (map[string]string, bool) {
	if data == nil {
		return nil, false
	}

	cleaned := make(map[string]string)
	redacted := false

	for key, value := range data {
		keyLower := strings.ToLower(key)

		// Check if this is a prompt-related sensitive key
		isPromptKey := strings.Contains(keyLower, "prompt") ||
			strings.Contains(keyLower, "message") ||
			strings.Contains(keyLower, "input")

		// Skip sensitive keys entirely (unless it's a prompt key and prompts are allowed)
		if sensitiveKeys[keyLower] {
			if !(allowPrompts && isPromptKey) {
				redacted = true
				continue
			}
		} else if isPromptKey && !allowPrompts {
			// Redact prompts if not allowed and not a sensitive key we already checked
			redacted = true
			continue
		}

		// Check for embedded secrets
		cleanedValue := redactSecrets(value)
		if cleanedValue != value {
			redacted = true
		}

		cleaned[key] = cleanedValue
	}

	return cleaned, redacted
}

// RedactSecrets masks secret patterns in a string
func redactSecrets(s string) string {
	if s == "" {
		return s
	}

	result := s
	for _, pattern := range redactionPatterns {
		result = pattern.ReplaceAllString(result, "[REDACTED]")
	}

	return result
}

// ContainsSecret checks if a string contains any detected secret pattern
func ContainsSecret(s string) bool {
	if s == "" {
		return false
	}

	for _, pattern := range redactionPatterns {
		if pattern.MatchString(s) {
			return true
		}
	}

	return false
}

// ShouldRedactAttribute determines if an attribute should be redacted
func ShouldRedactAttribute(key string, config *RedactionConfig) bool {
	if config == nil {
		config = DefaultRedactionConfig()
	}

	keyLower := strings.ToLower(key)

	// Always redact sensitive keys (unless it's a prompt key and prompt logging is enabled)
	if sensitiveKeys[keyLower] {
		// Check if this is a prompt-related key and logging is enabled
		if config.EnablePromptLogging && (strings.Contains(keyLower, "prompt") ||
			strings.Contains(keyLower, "message") ||
			strings.Contains(keyLower, "input")) {
			return false
		}
		return true
	}

	return false
}

// RedactAttributes removes sensitive attributes from a span's attributes
// This is used during span processing in the collector
func RedactAttributes(attributes map[string]string, config *RedactionConfig) map[string]string {
	if attributes == nil {
		return nil
	}

	if config == nil {
		config = DefaultRedactionConfig()
	}

	cleaned := make(map[string]string)

	for key, value := range attributes {
		if ShouldRedactAttribute(key, config) {
			continue
		}

		// Check for embedded secrets
		if config.EnableSecretDetection {
			value = redactSecrets(value)
		}

		cleaned[key] = value
	}

	return cleaned
}
