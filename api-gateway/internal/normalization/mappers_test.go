package normalization

import (
	"testing"
)

func assertEqual(t *testing.T, expected, actual string, msg string) {
	if expected != actual {
		t.Errorf("%s: expected '%s', got '%s'", msg, expected, actual)
	}
}

func TestMapCodexSessionStarted(t *testing.T) {
	span := &EnrichedSpan{
		Framework: "codex",
		Name:      "session.started",
		Attributes: map[string]string{
			"codex.session.id": "sess_xyz",
			"codex.model":      "claude-opus",
			"af.user.id":       "user123",
			"af.user.email":    "dev@company.com",
		},
		StartTimeNs: 1000000000,
		DurationNs:  5000000000,
		StatusCode:  0,
	}

	event, err := MapCodexEvent(span)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	assertEqual(t, "session.started", event.EventType, "event type")
	assertEqual(t, "codex", event.SourceTool, "source tool")
	assertEqual(t, "user123", event.UserID, "user id")
	assertEqual(t, "sess_xyz", event.SessionID, "session id")
	assertEqual(t, "claude-opus", event.ModelName, "model name")
}

func TestMapCodexToolCallCompleted(t *testing.T) {
	span := &EnrichedSpan{
		Framework: "codex",
		Name:      "tool_call.completed",
		Attributes: map[string]string{
			"codex.session.id":   "sess_xyz",
			"codex.tool.name":    "shell",
			"codex.tool.command": "pytest tests/",
			"af.user.id":         "user123",
		},
		DurationNs: 18422000000,
		StatusCode: 0,
	}

	event, err := MapCodexEvent(span)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	assertEqual(t, "tool.call.completed", event.EventType, "event type")
	assertEqual(t, "shell", event.ToolName, "tool name")
	assertEqual(t, "pytest tests/", event.Command, "command")
}

func TestMapCodexDangerousCommand(t *testing.T) {
	span := &EnrichedSpan{
		Framework: "codex",
		Name:      "tool_call.completed",
		Attributes: map[string]string{
			"codex.session.id":   "sess_danger",
			"codex.tool.name":    "shell",
			"codex.tool.command": "rm -rf /production",
			"af.user.id":         "user123",
		},
		DurationNs: 1000000000,
		StatusCode: 0,
	}

	event, err := MapCodexEvent(span)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if event.RiskScore < 80 {
		t.Errorf("expected high risk score for 'rm -rf', got %d", event.RiskScore)
	}
	if !event.RequiresReview {
		t.Errorf("expected requires_review to be true for high risk event")
	}
}

func TestMapClaudeCodeSessionStarted(t *testing.T) {
	span := &EnrichedSpan{
		Framework: "claude_code",
		Name:      "session.started",
		Attributes: map[string]string{
			"claude_code.session_id": "claud_sess_abc",
			"claude_code.model":      "claude-sonnet",
			"af.user.id":             "user456",
		},
		StartTimeNs: 1000000000,
		DurationNs:  2000000000,
		StatusCode:  0,
	}

	event, err := MapClaudeCodeEvent(span)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	assertEqual(t, "session.started", event.EventType, "event type")
	assertEqual(t, "claude_code", event.SourceTool, "source tool")
	assertEqual(t, "user456", event.UserID, "user id")
	assertEqual(t, "claude-sonnet", event.ModelName, "model name")
	assertEqual(t, "anthropic", event.Provider, "provider")
}

func TestMapClaudeCodeToolUsage(t *testing.T) {
	span := &EnrichedSpan{
		Framework: "claude_code",
		Name:      "tool.completed",
		Attributes: map[string]string{
			"claude_code.session_id":   "claud_sess_abc",
			"claude_code.tool_name":    "shell",
			"gen_ai.usage.input_tokens": "5000",
		},
		InputTokens:  5000,
		OutputTokens: 2000,
		StatusCode:   0,
	}

	event, err := MapClaudeCodeEvent(span)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	assertEqual(t, "tool.call.completed", event.EventType, "event type")
	assertEqual(t, "claude_code", event.SourceTool, "source tool")
	if event.RiskScore > 50 {
		t.Errorf("expected low risk for normal tool call, got %d", event.RiskScore)
	}
}
