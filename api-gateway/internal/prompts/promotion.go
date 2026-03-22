package prompts

import "strings"

func normalizeEnvironment(value string) string {
	if trimmed := strings.ToLower(strings.TrimSpace(value)); trimmed != "" {
		return trimmed
	}
	return "development"
}
