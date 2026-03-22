package prompts

import "testing"

func TestNormalizeEnvironment(t *testing.T) {
	if got := normalizeEnvironment(" Production "); got != "production" {
		t.Fatalf("expected production, got %q", got)
	}
	if got := normalizeEnvironment(""); got != "development" {
		t.Fatalf("expected development fallback, got %q", got)
	}
}
