package ws

import "testing"

func TestBuildAllowedWSOrigins_DevIncludesLocalhostDefaults(t *testing.T) {
	allowed := buildAllowedWSOrigins("", false)
	if !allowed["http://localhost:3000"] || !allowed["http://localhost:5173"] {
		t.Fatal("expected dev mode websocket origins to include localhost defaults")
	}
}

func TestBuildAllowedWSOrigins_StrictRequiresExplicitOrigins(t *testing.T) {
	allowed := buildAllowedWSOrigins("", true)
	if allowed["http://localhost:3000"] || allowed["http://localhost:5173"] {
		t.Fatal("strict mode must not inherit localhost websocket origins by default")
	}
}

func TestIsAllowedWSOrigin_StrictRejectsMissingOrigin(t *testing.T) {
	if isAllowedWSOrigin("", map[string]bool{}, true) {
		t.Fatal("strict mode should reject websocket upgrades without an Origin header")
	}
}

func TestIsAllowedWSOrigin_DevAllowsMissingOrigin(t *testing.T) {
	if !isAllowedWSOrigin("", map[string]bool{}, false) {
		t.Fatal("dev mode should allow websocket upgrades without an Origin header")
	}
}

func TestIsAllowedWSOrigin_ExplicitOriginAllowed(t *testing.T) {
	allowed := buildAllowedWSOrigins("https://app.govagn.io", true)
	if !isAllowedWSOrigin("https://app.govagn.io", allowed, true) {
		t.Fatal("expected explicit websocket origin to be allowed")
	}
}
