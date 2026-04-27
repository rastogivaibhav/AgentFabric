package normalization

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// MapClaudeCodeEvent converts an EnrichedSpan from Claude Code to CanonicalEvent
func MapClaudeCodeEvent(span *EnrichedSpan) (*CanonicalEvent, error) {
	if span.Framework != "claude_code" {
		return nil, fmt.Errorf("expected framework 'claude_code', got '%s'", span.Framework)
	}

	eventType := deriveEventTypeClaudeCode(span.Name, span.Attributes)

	event := &CanonicalEvent{
		EventTime:      time.Unix(0, int64(span.StartTimeNs)),
		SourceTool:     "claude_code",
		EventType:      eventType,
		UserID:         span.Attributes["af.user.id"],
		UserEmail:      span.Attributes["af.user.email"],
		SessionID:      span.Attributes["claude_code.session_id"],
		TraceID:        span.TraceID,
		SpanID:         span.SpanID,
		ModelName:      span.Attributes["claude_code.model"],
		Provider:       "anthropic",
		ToolName:       span.Attributes["claude_code.tool_name"],
		Command:        span.Attributes["claude_code.tool_command"],
		Severity:       "info",
		PromptRedacted: true, // Claude Code redacts by default
		RawEvent:       span,
	}

	if event.Command != "" {
		event.CommandHash = hashCommand(event.Command)
	}

	// Score risk for Claude Code
	event.RiskScore = scoreClaudeCodeRisk(event, span)
	event.RequiresReview = event.RiskScore > 50

	return event, nil
}

func deriveEventTypeClaudeCode(spanName string, attrs map[string]string) string {
	name := strings.ToLower(spanName)

	switch {
	case strings.Contains(name, "session") && strings.Contains(name, "started"):
		return "session.started"
	case strings.Contains(name, "session") && strings.Contains(name, "ended"):
		return "session.ended"
	case strings.Contains(name, "tool"):
		if strings.Contains(name, "failed") {
			return "tool.call.failed"
		}
		return "tool.call.completed"
	case strings.Contains(name, "usage"):
		return "token.usage.recorded"
	case strings.Contains(name, "cost"):
		return "cost.estimated"
	case strings.Contains(name, "approval"):
		if strings.Contains(name, "granted") {
			return "tool.approval.granted"
		}
		return "tool.approval.requested"
	case strings.Contains(name, "error"):
		return "error.detected"
	default:
		return "event.unknown"
	}
}

func scoreClaudeCodeRisk(event *CanonicalEvent, span *EnrichedSpan) int {
	score := 0

	// Dangerous shell commands
	if event.ToolName == "shell" || event.ToolName == "bash" {
		cmd := strings.ToLower(event.Command)
		patterns := []string{
			"rm -rf", "mkfs", "dd if=", "chmod 777",
			"curl.*\\|.*sh", "wget.*\\|.*sh",
		}
		for _, pattern := range patterns {
			if matched, _ := regexp.MatchString(pattern, cmd); matched {
				score += 85
				break
			}
		}
	}

	// File operations on production paths
	if strings.Contains(event.ToolName, "file") {
		path := strings.ToLower(span.Attributes["file_path"])
		if strings.Contains(path, "prod") || strings.Contains(path, ".env") {
			score += 65
		}
	}

	// MCP tool usage (monitor for unusual activity)
	if strings.Contains(event.ToolName, "mcp") {
		score += 30
	}

	return score
}
