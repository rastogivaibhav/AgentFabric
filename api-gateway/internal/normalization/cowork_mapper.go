package normalization

import (
	"fmt"
	"regexp"
	"strings"
)

// CoworkMapper implements EventMapper for Cowork paired assistant events
type CoworkMapper struct{}

// Map converts a Cowork event to CanonicalEvent
func (m *CoworkMapper) Map(event interface{}) (*CanonicalEvent, error) {
	data, ok := event.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("expected map[string]interface{}, got %T", event)
	}

	// Extract common fields
	source := extractString(data, "source")
	if source != "cowork" {
		return nil, fmt.Errorf("expected source 'cowork', got '%s'", source)
	}

	eventType := extractString(data, "event_type")
	if eventType == "" {
		return nil, fmt.Errorf("missing event_type")
	}

	timestamp := extractTimestamp(data)
	latencyMs := extractInt64(data, "latency_ms")

	canonical := &CanonicalEvent{
		EventTime:       timestamp,
		SourceVendor:    "cowork",
		SourceProduct:   "cowork-assistant",
		SourceChannel:   "api",
		EventType:       eventType,
		UserEmail:       extractString(data, "user_email"),
		UserID:          extractString(data, "user_id"),
		SessionID:       extractString(data, "session_id"),
		ModelName:       extractString(data, "model"),
		Provider:        "anthropic",
		FilePath:        extractString(data, "file_path"),
		InputTokens:     extractInt64(data, "input_tokens"),
		OutputTokens:    extractInt64(data, "output_tokens"),
		LatencyMs:       latencyMs,
		PromptRedacted:  true,
		Redacted:        true,
		Payload:         data,
	}

	// Derive event category and action
	canonical.EventCategory = deriveCoworkEventCategory(eventType)
	canonical.Action = deriveCoworkAction(eventType)

	// Score risk
	canonical.RiskScore = scoreCoworkRisk(canonical, data)
	canonical.RequiresReview = canonical.RiskScore > 50

	return canonical, nil
}

// Accepts checks if this mapper handles the given vendor
func (m *CoworkMapper) Accepts(sourceVendor, sourceProduct string) bool {
	return sourceVendor == "cowork"
}

// deriveCoworkEventCategory maps Cowork event types to canonical categories
func deriveCoworkEventCategory(eventType string) string {
	switch {
	case strings.Contains(eventType, "session"):
		return "session"
	case strings.Contains(eventType, "suggestion"):
		return "tool_call"
	case strings.Contains(eventType, "refactor"):
		return "tool_call"
	case strings.Contains(eventType, "review"):
		return "approval"
	case strings.Contains(eventType, "test"):
		return "tool_call"
	default:
		return "unknown"
	}
}

// deriveCoworkAction maps event types to specific actions
func deriveCoworkAction(eventType string) string {
	switch eventType {
	case "session.started":
		return "session_started"
	case "session.ended":
		return "session_ended"
	case "suggestion.accepted":
		return "code_generation_accepted"
	case "suggestion.rejected":
		return "code_generation_rejected"
	case "refactor.suggested":
		return "refactor_suggested"
	case "refactor.accepted":
		return "refactor_accepted"
	case "refactor.rejected":
		return "refactor_rejected"
	case "review.requested":
		return "review_requested"
	case "review.completed":
		return "review_completed"
	case "test.suggested":
		return "test_generation"
	case "test.generated":
		return "test_generated"
	default:
		return "unknown"
	}
}

// scoreCoworkRisk evaluates risk for Cowork events
func scoreCoworkRisk(event *CanonicalEvent, data map[string]interface{}) int {
	score := 0

	// Large code suggestions (unreviewed changes)
	if suggestionLength := extractInt64(data, "suggestion_length"); suggestionLength > 500 {
		score += 40
	} else if suggestionLength > 200 {
		score += 20
	}

	// Production file modifications
	if event.FilePath != "" {
		filePath := strings.ToLower(event.FilePath)
		if regexp.MustCompile(`prod|terraform|k8s|kubernetes|database|\.env|secret|config`).MatchString(filePath) {
			score += 60
		}
	}

	// Refactoring operations (inherently riskier than suggestions)
	if event.Action == "refactor_suggested" || event.Action == "refactor_accepted" {
		if linesChanged := extractInt64(data, "lines_changed"); linesChanged > 100 {
			score += 60
		} else if linesChanged > 50 {
			score += 40
		}
	}

	// High token usage (extended analysis/generation)
	totalTokens := event.InputTokens + event.OutputTokens
	if totalTokens > 200000 {
		score += 40
	} else if totalTokens > 100000 {
		score += 20
	}

	// Long-running operations (potential for complex/risky changes)
	if event.LatencyMs > 5000 {
		score += 20
	}

	// Review operations that were not approved (flagged)
	if event.Action == "review_requested" && !strings.Contains(event.Action, "completed") {
		score += 30
	}

	return score
}

// Register Cowork mapper on init
func init() {
	RegisterMapper("cowork", &CoworkMapper{})
}
