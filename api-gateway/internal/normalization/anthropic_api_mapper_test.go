package normalization

import (
	"testing"
	"time"
)

func TestMapAnthropicAPIMessageCreated(t *testing.T) {
	event := map[string]interface{}{
		"source":      "anthropic-api",
		"event_type":  "message.created",
		"timestamp":   time.Now().Format(time.RFC3339),
		"user_id":     "user_abc123",
		"user_email":  "user@company.com",
		"api_key_id":  "sk-ant-key-abc123",
		"model":       "claude-3-opus-20240229",
		"temperature": 0.7,
		"max_tokens":  2048,
		"session_id":  "api_sess_001",
	}

	mapper := &AnthropicAPIMapper{}
	canonical, err := mapper.Map(event)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if canonical.SourceVendor != "anthropic" {
		t.Errorf("expected source_vendor 'anthropic', got '%s'", canonical.SourceVendor)
	}
	if canonical.EventType != "message.created" {
		t.Errorf("expected event_type 'message.created', got '%s'", canonical.EventType)
	}
	if canonical.Provider != "anthropic" {
		t.Errorf("expected provider 'anthropic', got '%s'", canonical.Provider)
	}
}

func TestMapAnthropicAPIMessageCompleted(t *testing.T) {
	event := map[string]interface{}{
		"source":         "anthropic-api",
		"event_type":     "message.completed",
		"timestamp":      time.Now().Format(time.RFC3339),
		"user_email":     "dev@company.com",
		"model":          "claude-3-sonnet-20240229",
		"input_tokens":   500,
		"output_tokens":   1200,
		"usage_tokens":    1700,
		"stop_reason":     "end_turn",
		"latency_ms":      2500.0,
		"session_id":      "api_sess_002",
	}

	mapper := &AnthropicAPIMapper{}
	canonical, err := mapper.Map(event)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if canonical.InputTokens != 500 {
		t.Errorf("expected input_tokens 500, got %d", canonical.InputTokens)
	}
	if canonical.OutputTokens != 1200 {
		t.Errorf("expected output_tokens 1200, got %d", canonical.OutputTokens)
	}
	if canonical.Action != "model_completion" {
		t.Errorf("expected action 'model_completion', got '%s'", canonical.Action)
	}
}

func TestMapAnthropicAPIBatchSubmitted(t *testing.T) {
	event := map[string]interface{}{
		"source":       "anthropic-api",
		"event_type":   "batch.submitted",
		"timestamp":    time.Now().Format(time.RFC3339),
		"user_email":   "dev@company.com",
		"batch_id":     "batch_abc123",
		"request_count": 100,
		"latency_ms":   500.0,
		"session_id":   "api_sess_003",
	}

	mapper := &AnthropicAPIMapper{}
	canonical, err := mapper.Map(event)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if canonical.EventCategory != "batch" {
		t.Errorf("expected event_category 'batch', got '%s'", canonical.EventCategory)
	}
}

func TestAnthropicAPIMapper_Accepts(t *testing.T) {
	mapper := &AnthropicAPIMapper{}

	if !mapper.Accepts("anthropic", "") {
		t.Error("should accept anthropic vendor")
	}
	if mapper.Accepts("openai", "") {
		t.Error("should not accept non-anthropic vendor")
	}
}

func TestMapAnthropicAPIHighTokenUsage_Warning(t *testing.T) {
	event := map[string]interface{}{
		"source":       "anthropic-api",
		"event_type":   "message.completed",
		"timestamp":    time.Now().Format(time.RFC3339),
		"user_email":   "dev@company.com",
		"model":        "claude-3-opus-20240229",
		"input_tokens": 100000,
		"output_tokens": 80000,
		"latency_ms":   5000.0,
		"session_id":   "api_sess_large",
	}

	mapper := &AnthropicAPIMapper{}
	canonical, err := mapper.Map(event)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if canonical.RiskScore < 20 {
		t.Errorf("expected elevated risk for high token usage, got %d", canonical.RiskScore)
	}
}

func TestMapAnthropicAPIFailedMessage(t *testing.T) {
	event := map[string]interface{}{
		"source":       "anthropic-api",
		"event_type":   "message.failed",
		"timestamp":    time.Now().Format(time.RFC3339),
		"user_email":   "dev@company.com",
		"model":        "claude-3-sonnet-20240229",
		"error_type":   "rate_limit_error",
		"error_code":   429,
		"latency_ms":   1000.0,
		"session_id":   "api_sess_error",
	}

	mapper := &AnthropicAPIMapper{}
	canonical, err := mapper.Map(event)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if canonical.Action != "message_failed" {
		t.Errorf("expected action 'message_failed', got '%s'", canonical.Action)
	}
}

func TestMapAnthropicAPIVisionWithImages_RiskScored(t *testing.T) {
	event := map[string]interface{}{
		"source":       "anthropic-api",
		"event_type":   "message.created",
		"timestamp":    time.Now().Format(time.RFC3339),
		"user_email":   "dev@company.com",
		"model":        "claude-3-opus-20240229",
		"has_images":   true,
		"image_count":  5,
		"session_id":   "api_sess_images",
	}

	mapper := &AnthropicAPIMapper{}
	canonical, err := mapper.Map(event)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if canonical.RiskScore < 10 {
		t.Errorf("expected some risk for vision API usage, got %d", canonical.RiskScore)
	}
}
