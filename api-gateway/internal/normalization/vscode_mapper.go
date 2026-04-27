package normalization

import (
	"fmt"
	"regexp"
	"strings"
)

// VSCodeMapper implements EventMapper for all VSCode extension agents
// Supports: GitHub Copilot, Continue, Roo Code, Cline
type VSCodeMapper struct{}

// Map converts a VSCode extension event to CanonicalEvent
func (m *VSCodeMapper) Map(event interface{}) (*CanonicalEvent, error) {
	data, ok := event.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("expected map[string]interface{}, got %T", event)
	}

	// Extract common fields
	source := extractString(data, "source")
	if source == "" {
		return nil, fmt.Errorf("missing source")
	}

	eventType := extractString(data, "event_type")
	if eventType == "" {
		return nil, fmt.Errorf("missing event_type")
	}

	timestamp := extractTimestamp(data)
	latencyMs := extractInt64(data, "latency_ms")

	canonical := &CanonicalEvent{
		EventTime:       timestamp,
		SourceVendor:    "vscode",
		SourceProduct:   source,
		SourceChannel:   "extension",
		EventType:       eventType,
		UserEmail:       extractString(data, "user_email"),
		UserID:          extractString(data, "user_id"),
		SessionID:       extractString(data, "session_id"),
		ModelName:       extractString(data, "model"),
		Provider:        getProviderForSource(source),
		FilePath:        extractString(data, "file_path"),
		Command:         extractString(data, "command"),
		LatencyMs:       latencyMs,
		PromptRedacted:  true,
		Redacted:        true,
		Payload:         data,
	}

	// Derive event category and action
	canonical.EventCategory = deriveVSCodeEventCategory(eventType)
	canonical.Action = deriveVSCodeAction(eventType)

	// Score risk
	canonical.RiskScore = scoreVSCodeRisk(canonical, data, source)
	canonical.RequiresReview = canonical.RiskScore > 50

	return canonical, nil
}

// Accepts checks if this mapper handles the given vendor
func (m *VSCodeMapper) Accepts(sourceVendor, sourceProduct string) bool {
	vscodeTools := map[string]bool{
		"github-copilot": true,
		"continue":       true,
		"roo-code":       true,
		"cline":          true,
	}
	return vscodeTools[sourceVendor]
}

// getProviderForSource returns the model provider for a VSCode tool
func getProviderForSource(source string) string {
	switch source {
	case "github-copilot":
		return "openai"
	case "continue":
		return "anthropic"
	case "roo-code":
		return "anthropic"
	case "cline":
		return "anthropic"
	default:
		return "unknown"
	}
}

// deriveVSCodeEventCategory maps VSCode event types to canonical categories
func deriveVSCodeEventCategory(eventType string) string {
	switch {
	case strings.Contains(eventType, "suggestion"):
		return "tool_call"
	case strings.Contains(eventType, "code"):
		return "tool_call"
	case strings.Contains(eventType, "refactor"):
		return "tool_call"
	case strings.Contains(eventType, "terminal"):
		return "tool_call"
	case strings.Contains(eventType, "session"):
		return "session"
	default:
		return "unknown"
	}
}

// deriveVSCodeAction maps event types to specific actions
func deriveVSCodeAction(eventType string) string {
	switch eventType {
	case "suggestion.accepted":
		return "code_generation_accepted"
	case "suggestion.rejected":
		return "code_generation_rejected"
	case "suggestion.dismissed":
		return "code_generation_dismissed"
	case "code.generated":
		return "code_generated"
	case "code.completed":
		return "code_completed"
	case "refactor.started":
		return "refactor"
	case "refactor.completed":
		return "refactor_completed"
	case "terminal.command":
		return "shell_command"
	case "terminal.executed":
		return "command_executed"
	case "session.started":
		return "session_started"
	case "session.ended":
		return "session_ended"
	default:
		return "unknown"
	}
}

// scoreVSCodeRisk evaluates risk for VSCode extension events
func scoreVSCodeRisk(event *CanonicalEvent, data map[string]interface{}, source string) int {
	score := 0

	// Terminal commands are inherently risky
	if event.EventCategory == "tool_call" && event.Action == "shell_command" {
		cmd := strings.ToLower(event.Command)
		dangerousPatterns := []string{
			"rm -rf", "mkfs", "dd if=", "chmod 777",
			"curl.*\\|.*sh", "wget.*\\|.*sh",
		}
		for _, pattern := range dangerousPatterns {
			if matched, _ := regexp.MatchString(pattern, cmd); matched {
				score += 90
				break
			}
		}
	}

	// Large code generation (unreviewed changes)
	if suggestionLength := extractInt64(data, "suggestion_length"); suggestionLength > 500 {
		score += 40
	} else if suggestionLength > 200 {
		score += 20
	}

	// Production file modifications
	if event.FilePath != "" {
		filePath := strings.ToLower(event.FilePath)
		if regexp.MustCompile(`prod|terraform|k8s|kubernetes|\.env|secret`).MatchString(filePath) {
			score += 60
		}
	}

	// Refactoring operations (higher scope than suggestions)
	if event.Action == "refactor" || event.Action == "refactor_completed" {
		if linesChanged := extractInt64(data, "lines_changed"); linesChanged > 100 {
			score += 50
		} else if linesChanged > 50 {
			score += 30
		}
	}

	// High latency operations (potential long-running changes)
	if event.LatencyMs > 10000 {
		score += 25
	}

	// Cline-specific: file system operations
	if source == "cline" {
		if strings.Contains(event.EventType, "file") {
			score += 20
		}
	}

	return score
}

// Register VSCode mappers on init
func init() {
	mapper := &VSCodeMapper{}
	RegisterMapper("github-copilot", mapper)
	RegisterMapper("continue", mapper)
	RegisterMapper("roo-code", mapper)
	RegisterMapper("cline", mapper)
}
