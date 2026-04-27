package normalization

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// CodexMapper implements EventMapper for Codex events
type CodexMapper struct{}

// Map converts an EnrichedSpan from Codex framework to CanonicalEvent
func (m *CodexMapper) Map(event interface{}) (*CanonicalEvent, error) {
	span, ok := event.(*EnrichedSpan)
	if !ok {
		return nil, fmt.Errorf("expected *EnrichedSpan, got %T", event)
	}
	return MapCodexEvent(span)
}

// Accepts checks if this mapper handles the given vendor/product
func (m *CodexMapper) Accepts(sourceVendor, sourceProduct string) bool {
	return sourceVendor == "codex"
}

// MapCodexEvent converts an EnrichedSpan from Codex framework to CanonicalEvent
func MapCodexEvent(span *EnrichedSpan) (*CanonicalEvent, error) {
	if span.Framework != "codex" {
		return nil, fmt.Errorf("expected framework 'codex', got '%s'", span.Framework)
	}

	// Determine event type from span name
	eventType := deriveEventTypeCodex(span.Name, span.Attributes)

	event := &CanonicalEvent{
		EventTime:      time.Unix(0, int64(span.StartTimeNs)),
		SourceTool:     "codex",
		SourceVendor:   "codex",
		SourceProduct:  "codex-cli",
		SourceChannel:  "otlp",
		EventType:      eventType,
		EventCategory:  deriveEventCategoryCodex(eventType),
		UserID:         span.Attributes["af.user.id"],
		UserEmail:      span.Attributes["af.user.email"],
		SessionID:      span.Attributes["codex.session.id"],
		TraceID:        span.TraceID,
		SpanID:         span.SpanID,
		ModelName:      span.Attributes["codex.model"],
		Provider:       "openai",
		ToolName:       span.Attributes["codex.tool.name"],
		Command:        span.Attributes["codex.tool.command"],
		Severity:       deriveSeverityCodex(span.StatusCode, span.Attributes),
		PromptRedacted: true,
		Redacted:       true,
		Payload:        convertAttributesToPayload(span.Attributes),
		RawEvent:       span,
	}

	// Hash sensitive command
	if event.Command != "" {
		event.CommandHash = hashCommand(event.Command)
	}

	// Score risk
	event.RiskScore = scoreCodexRisk(event, span)
	event.RequiresReview = event.RiskScore > 50

	return event, nil
}

func deriveEventTypeCodex(spanName string, attrs map[string]string) string {
	name := strings.ToLower(spanName)

	switch {
	case strings.Contains(name, "session") && strings.Contains(name, "started"):
		return "session.started"
	case strings.Contains(name, "session") && strings.Contains(name, "ended"):
		return "session.ended"
	case strings.Contains(name, "tool") && strings.Contains(name, "call"):
		if strings.Contains(name, "completed") || strings.Contains(name, "success") {
			return "tool.call.completed"
		} else if strings.Contains(name, "failed") {
			return "tool.call.failed"
		}
		return "tool.call.started"
	case strings.Contains(name, "approval"):
		if strings.Contains(name, "granted") {
			return "tool.approval.granted"
		} else if strings.Contains(name, "denied") {
			return "tool.approval.denied"
		}
		return "tool.approval.requested"
	case strings.Contains(name, "model") && strings.Contains(name, "request"):
		if strings.Contains(name, "completed") {
			return "model.request.completed"
		} else if strings.Contains(name, "failed") {
			return "model.request.failed"
		}
		return "model.request.started"
	case strings.Contains(name, "error"):
		return "error.detected"
	default:
		return "event.unknown"
	}
}

func scoreCodexRisk(event *CanonicalEvent, span *EnrichedSpan) int {
	score := 0

	// Dangerous shell commands
	if event.ToolName == "shell" {
		cmd := strings.ToLower(event.Command)
		dangerousPatterns := []string{
			"rm -rf", "mkfs", "dd if=", "chmod 777",
			"curl.*|.*sh", "wget.*|.*sh", "> /dev/", "exec", "eval",
		}
		for _, pattern := range dangerousPatterns {
			if matched, _ := regexp.MatchString(pattern, cmd); matched {
				score += 90
				break
			}
		}
	}

	// Production file modifications
	if event.EventType == "file.updated" {
		path := strings.ToLower(span.Attributes["file_path"])
		if strings.Contains(path, "prod") || strings.Contains(path, "production") ||
			strings.Contains(path, "terraform") || strings.Contains(path, "k8s") {
			score += 70
		}
	}

	// High token usage (warn on >200k tokens)
	inputTokens := parseIntAttr(span.Attributes["gen_ai.usage.input_tokens"])
	outputTokens := parseIntAttr(span.Attributes["gen_ai.usage.output_tokens"])
	if inputTokens+outputTokens > 200000 {
		score += 40
	}

	return score
}

func deriveSeverityCodex(statusCode int32, attrs map[string]string) string {
	if statusCode != 0 {
		return "error"
	}
	if attrs["codex.tool.status"] == "failed" {
		return "warning"
	}
	return "info"
}

func hashCommand(cmd string) string {
	h := sha256.Sum256([]byte(cmd))
	return hex.EncodeToString(h[:])
}

func parseIntAttr(s string) int64 {
	var v int64
	fmt.Sscan(s, &v)
	return v
}

func deriveEventCategoryCodex(eventType string) string {
	switch {
	case strings.Contains(eventType, "session"):
		return "session"
	case strings.Contains(eventType, "model"):
		return "model_call"
	case strings.Contains(eventType, "tool"):
		return "tool_call"
	case strings.Contains(eventType, "approval"):
		return "approval"
	default:
		return "unknown"
	}
}

func convertAttributesToPayload(attrs map[string]string) map[string]interface{} {
	payload := make(map[string]interface{})
	for k, v := range attrs {
		payload[k] = v
	}
	return payload
}
