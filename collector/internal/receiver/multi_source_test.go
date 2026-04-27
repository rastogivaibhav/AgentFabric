package receiver

import (
	"context"
	"testing"
	"time"
)

func TestVSCodeExtensionReceiver_AcceptsTelemetry(t *testing.T) {
	receiver := NewVSCodeExtensionReceiver()
	event := map[string]interface{}{
		"source":     "vscode-copilot",
		"event_type": "suggestion.accepted",
		"timestamp":  time.Now().Format(time.RFC3339),
		"user_id":    "user123",
		"user_email": "dev@company.com",
		"model":      "claude-opus",
		"latency_ms": 1200.0,
		"payload": map[string]interface{}{
			"suggestion_length": 42,
			"file_path":         "/app/main.py",
		},
	}

	err := receiver.Receive(context.Background(), event)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestVSCodeExtensionReceiver_ValidatesSource(t *testing.T) {
	receiver := NewVSCodeExtensionReceiver()
	event := map[string]interface{}{
		"event_type": "suggestion.accepted",
		"timestamp":  time.Now().Format(time.RFC3339),
		// missing "source"
	}

	err := receiver.Receive(context.Background(), event)
	if err == nil {
		t.Error("expected error for missing source field")
	}
}

func TestVSCodeExtensionReceiver_ValidatesEventType(t *testing.T) {
	receiver := NewVSCodeExtensionReceiver()
	event := map[string]interface{}{
		"source": "vscode-copilot",
		// missing "event_type"
		"timestamp": time.Now().Format(time.RFC3339),
	}

	err := receiver.Receive(context.Background(), event)
	if err == nil {
		t.Error("expected error for missing event_type field")
	}
}

func TestVSCodeExtensionReceiver_HandlesDefaultTimestamp(t *testing.T) {
	receiver := NewVSCodeExtensionReceiver()
	event := map[string]interface{}{
		"source":     "cursor",
		"event_type": "refactor.executed",
		// missing timestamp - should default to now()
	}

	err := receiver.Receive(context.Background(), event)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWebhookReceiver_AcceptsEvent(t *testing.T) {
	receiver := NewWebhookReceiver()
	event := map[string]interface{}{
		"source":     "custom-integration",
		"event_type": "custom.event",
		"timestamp":  time.Now().Format(time.RFC3339),
		"payload": map[string]interface{}{
			"custom_field": "value",
		},
	}

	err := receiver.Receive(context.Background(), event)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWebhookReceiver_ValidatesSource(t *testing.T) {
	receiver := NewWebhookReceiver()
	event := map[string]interface{}{
		"event_type": "custom.event",
		// missing "source"
	}

	err := receiver.Receive(context.Background(), event)
	if err == nil {
		t.Error("expected error for missing source field")
	}
}

func TestWebhookReceiver_ValidatesEventType(t *testing.T) {
	receiver := NewWebhookReceiver()
	event := map[string]interface{}{
		"source": "custom-integration",
		// missing "event_type"
	}

	err := receiver.Receive(context.Background(), event)
	if err == nil {
		t.Error("expected error for missing event_type field")
	}
}

func TestDirectAPIReceiver_AcceptsNormalizedEvent(t *testing.T) {
	receiver := NewDirectAPIReceiver()
	event := map[string]interface{}{
		"source_vendor": "codex",
		"event_type":    "tool.call.completed",
		"user_id":       "user123",
		"risk_score":    25,
	}

	err := receiver.Receive(context.Background(), event)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDirectAPIReceiver_ValidatesSourceVendor(t *testing.T) {
	receiver := NewDirectAPIReceiver()
	event := map[string]interface{}{
		"event_type": "tool.call.completed",
		// missing "source_vendor"
	}

	err := receiver.Receive(context.Background(), event)
	if err == nil {
		t.Error("expected error for missing source_vendor field")
	}
}

func TestVSCodeExtensionReceiver_HasName(t *testing.T) {
	receiver := NewVSCodeExtensionReceiver()
	if receiver.Name() != "vscode-extension" {
		t.Errorf("expected name 'vscode-extension', got '%s'", receiver.Name())
	}
}

func TestWebhookReceiver_HasName(t *testing.T) {
	receiver := NewWebhookReceiver()
	if receiver.Name() != "webhook" {
		t.Errorf("expected name 'webhook', got '%s'", receiver.Name())
	}
}

func TestDirectAPIReceiver_HasName(t *testing.T) {
	receiver := NewDirectAPIReceiver()
	if receiver.Name() != "direct-api" {
		t.Errorf("expected name 'direct-api', got '%s'", receiver.Name())
	}
}
