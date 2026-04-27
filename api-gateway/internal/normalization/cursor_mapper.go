package normalization

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// CursorMapper implements EventMapper for Cursor IDE events
type CursorMapper struct{}

// Map converts a VSCode extension event (Cursor) to CanonicalEvent
func (m *CursorMapper) Map(event interface{}) (*CanonicalEvent, error) {
	data, ok := event.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("expected map[string]interface{}, got %T", event)
	}

	// Extract common fields
	source := extractString(data, "source")
	if source != "cursor" {
		return nil, fmt.Errorf("expected source 'cursor', got '%s'", source)
	}

	eventType := extractString(data, "event_type")
	if eventType == "" {
		return nil, fmt.Errorf("missing event_type")
	}

	timestamp := extractTimestamp(data)
	latencyMs := extractInt64(data, "latency_ms")

	canonical := &CanonicalEvent{
		EventTime:       timestamp,
		SourceVendor:    "cursor",
		SourceProduct:   "cursor-editor",
		SourceChannel:   "extension",
		EventType:       eventType,
		UserEmail:       extractString(data, "user_email"),
		UserID:          extractString(data, "user_id"),
		SessionID:       extractString(data, "session_id"),
		ModelName:       extractString(data, "model"),
		Provider:        "anthropic",
		FilePath:        extractString(data, "file_path"),
		LatencyMs:       latencyMs,
		PromptRedacted:  true,
		Redacted:        true,
		Payload:         data,
	}

	// Derive event category and action from event type
	canonical.EventCategory = deriveCursorEventCategory(eventType)
	canonical.Action = deriveCursorAction(eventType)

	// Score risk
	canonical.RiskScore = scoreCursorRisk(canonical, data)
	canonical.RequiresReview = canonical.RiskScore > 50

	return canonical, nil
}

// Accepts checks if this mapper handles the given vendor/product
func (m *CursorMapper) Accepts(sourceVendor, sourceProduct string) bool {
	return sourceVendor == "cursor"
}

// deriveCursorEventCategory maps Cursor-specific event types to canonical categories
func deriveCursorEventCategory(eventType string) string {
	switch {
	case strings.Contains(eventType, "suggestion"):
		return "tool_call"
	case strings.Contains(eventType, "refactor"):
		return "tool_call"
	case strings.Contains(eventType, "chat"):
		return "session"
	case strings.Contains(eventType, "edit"):
		return "tool_call"
	default:
		return "unknown"
	}
}

// deriveCursorAction maps event types to specific actions
func deriveCursorAction(eventType string) string {
	switch eventType {
	case "suggestion.accepted":
		return "code_generation_accepted"
	case "suggestion.rejected":
		return "code_generation_rejected"
	case "suggestion.dismissed":
		return "code_generation_dismissed"
	case "refactor.executed":
		return "refactor"
	case "refactor.declined":
		return "refactor_declined"
	case "chat.started":
		return "chat_initiated"
	case "chat.ended":
		return "chat_completed"
	case "edit.suggested":
		return "edit_suggested"
	default:
		return "unknown"
	}
}

// scoreCursorRisk evaluates risk for Cursor-specific events
func scoreCursorRisk(event *CanonicalEvent, data map[string]interface{}) int {
	score := 0

	// Large code generation (unreviewed changes)
	if suggestionLength := extractInt64(data, "suggestion_length"); suggestionLength > 500 {
		score += 40
	} else if suggestionLength > 200 {
		score += 20
	}

	// Production file modifications
	if event.FilePath != "" {
		filePath := strings.ToLower(event.FilePath)
		if regexp.MustCompile(`prod|terraform|k8s|\.env|secret`).MatchString(filePath) {
			score += 60
		}
	}

	// Refactoring operations (higher risk due to scope)
	if event.Action == "refactor" {
		if linesChanged := extractInt64(data, "lines_changed"); linesChanged > 100 {
			score += 50
		} else if linesChanged > 50 {
			score += 30
		}
	}

	// Multiple suggestions in quick succession (unusual activity)
	if suggestionCount := extractInt64(data, "suggestion_count_session"); suggestionCount > 20 {
		score += 30
	}

	return score
}

// Helper functions for extracting and converting values
func extractString(data map[string]interface{}, key string) string {
	if v, ok := data[key].(string); ok {
		return v
	}
	return ""
}

func extractInt64(data map[string]interface{}, key string) int64 {
	if v, ok := data[key].(float64); ok {
		return int64(v)
	}
	if v, ok := data[key].(int64); ok {
		return v
	}
	if v, ok := data[key].(int); ok {
		return int64(v)
	}
	return 0
}

func extractTimestamp(data map[string]interface{}) time.Time {
	if tsStr, ok := data["timestamp"].(string); ok {
		if ts, err := time.Parse(time.RFC3339, tsStr); err == nil {
			return ts
		}
	}
	return time.Now()
}

// Register Cursor mapper on init
func init() {
	RegisterMapper("cursor", &CursorMapper{})
}
