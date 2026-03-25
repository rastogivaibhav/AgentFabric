package prompts

import "strings"

func normalizeEnvironment(value string) string {
	if trimmed := strings.ToLower(strings.TrimSpace(value)); trimmed != "" {
		return trimmed
	}
	return "development"
}

func normalizeReleaseStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "active":
		return "active"
	case "candidate":
		return "candidate"
	case "superseded":
		return "superseded"
	case "archived":
		return "archived"
	default:
		return "active"
	}
}
