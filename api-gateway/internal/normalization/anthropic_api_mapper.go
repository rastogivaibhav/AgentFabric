package normalization

import (
	"fmt"
	"strings"
)

// AnthropicAPIMapper implements EventMapper for direct Anthropic Claude API usage
type AnthropicAPIMapper struct{}

// Map converts a direct Anthropic API event to CanonicalEvent
func (m *AnthropicAPIMapper) Map(event interface{}) (*CanonicalEvent, error) {
	data, ok := event.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("expected map[string]interface{}, got %T", event)
	}

	// Extract common fields
	source := extractString(data, "source")
	if source != "anthropic-api" {
		return nil, fmt.Errorf("expected source 'anthropic-api', got '%s'", source)
	}

	eventType := extractString(data, "event_type")
	if eventType == "" {
		return nil, fmt.Errorf("missing event_type")
	}

	timestamp := extractTimestamp(data)
	latencyMs := extractInt64(data, "latency_ms")

	canonical := &CanonicalEvent{
		EventTime:       timestamp,
		SourceVendor:    "anthropic",
		SourceProduct:   "claude-api",
		SourceChannel:   "api",
		EventType:       eventType,
		UserEmail:       extractString(data, "user_email"),
		UserID:          extractString(data, "user_id"),
		SessionID:       extractString(data, "session_id"),
		ModelName:       extractString(data, "model"),
		Provider:        "anthropic",
		InputTokens:     extractInt64(data, "input_tokens"),
		OutputTokens:    extractInt64(data, "output_tokens"),
		LatencyMs:       latencyMs,
		PromptRedacted:  true,
		Redacted:        true,
		Payload:         data,
	}

	// Derive event category and action
	canonical.EventCategory = deriveAnthropicEventCategory(eventType)
	canonical.Action = deriveAnthropicAction(eventType)

	// Score risk
	canonical.RiskScore = scoreAnthropicRisk(canonical, data)
	canonical.RequiresReview = canonical.RiskScore > 50

	return canonical, nil
}

// Accepts checks if this mapper handles the given vendor
func (m *AnthropicAPIMapper) Accepts(sourceVendor, sourceProduct string) bool {
	return sourceVendor == "anthropic"
}

// deriveAnthropicEventCategory maps Anthropic API event types to canonical categories
func deriveAnthropicEventCategory(eventType string) string {
	switch {
	case strings.Contains(eventType, "message"):
		return "model_call"
	case strings.Contains(eventType, "batch"):
		return "batch"
	case strings.Contains(eventType, "vision"):
		return "model_call"
	case strings.Contains(eventType, "token"):
		return "token_usage"
	case strings.Contains(eventType, "cost"):
		return "cost"
	default:
		return "unknown"
	}
}

// deriveAnthropicAction maps event types to specific actions
func deriveAnthropicAction(eventType string) string {
	switch eventType {
	case "message.created":
		return "message_created"
	case "message.completed":
		return "model_completion"
	case "message.failed":
		return "message_failed"
	case "message.stopped":
		return "message_stopped"
	case "batch.submitted":
		return "batch_submitted"
	case "batch.completed":
		return "batch_completed"
	case "batch.failed":
		return "batch_failed"
	case "vision.processed":
		return "vision_processed"
	case "token.usage":
		return "token_counted"
	default:
		return "unknown"
	}
}

// scoreAnthropicRisk evaluates risk for Anthropic API events
func scoreAnthropicRisk(event *CanonicalEvent, data map[string]interface{}) int {
	score := 0

	// High token usage (expensive operations or potential abuse)
	totalTokens := event.InputTokens + event.OutputTokens
	if totalTokens > 200000 {
		score += 50
	} else if totalTokens > 100000 {
		score += 30
	} else if totalTokens > 50000 {
		score += 15
	}

	// Vision API usage with images (different attack surface)
	if hasImages := extractBool(data, "has_images"); hasImages {
		imageCount := extractInt64(data, "image_count")
		if imageCount > 10 {
			score += 30
		} else if imageCount > 5 {
			score += 15
		} else {
			score += 10
		}
	}

	// Failed or rate-limited operations
	if strings.Contains(event.Action, "failed") {
		errorCode := extractInt64(data, "error_code")
		if errorCode == 429 { // Rate limit
			score += 20
		} else if errorCode >= 500 { // Server error
			score += 10
		}
	}

	// Long-running operations (higher cost, potential issues)
	if event.LatencyMs > 30000 {
		score += 20
	}

	// Batch operations (higher impact)
	if event.EventCategory == "batch" {
		requestCount := extractInt64(data, "request_count")
		if requestCount > 1000 {
			score += 40
		} else if requestCount > 100 {
			score += 25
		}
	}

	// Unusual stop reasons
	stopReason := extractString(data, "stop_reason")
	if stopReason == "max_tokens" || stopReason == "stop_sequence" {
		score += 5 // Minor flag for truncation
	}

	return score
}

// Helper function to extract boolean values
func extractBool(data map[string]interface{}, key string) bool {
	if v, ok := data[key].(bool); ok {
		return v
	}
	return false
}

// Register Anthropic API mapper on init
func init() {
	RegisterMapper("anthropic", &AnthropicAPIMapper{})
}
