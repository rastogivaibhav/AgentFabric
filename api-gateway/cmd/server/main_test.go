package main

// Unit tests for the serve() helper and parseSecrets() utility.
//
// Tests do NOT start the full application — they exercise the extracted helpers
// directly, keeping them fast and dependency-free.
//
// TestServe_TLSDisabled        — HTTP server accepts a real connection.
// TestServe_TLSMissingCert     — empty cert/key path returns an error immediately
//                                (fail-secure: no silent HTTP fallback).
// TestServe_TLSCertFileNotFound — valid paths but missing files → error (not panic).
// TestParseSecrets_*            — rotation secret parsing edge cases.

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

// freePort returns a randomly-assigned available TCP port.
// There is a brief TOCTOU window between Close() and the caller binding,
// which is acceptable in tests running on loopback.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

// nopLogger returns a no-op zap logger suitable for tests.
func nopLogger() *zap.Logger { return zap.NewNop() }

// ─── serve() ──────────────────────────────────────────────────────────────────

// TestServe_TLSDisabled verifies that with TLS off the server starts as plain
// HTTP and responds to a real GET request.
func TestServe_TLSDisabled(t *testing.T) {
	port := freePort(t)
	srv := &http.Server{
		Addr: fmt.Sprintf("127.0.0.1:%d", port),
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	}

	errCh := make(chan error, 1)
	go func() { errCh <- serve(srv, false, "", "", nopLogger()) }()

	// Allow the goroutine to bind.
	time.Sleep(60 * time.Millisecond)

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/healthz", port))
	if err != nil {
		t.Fatalf("HTTP GET: %v", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}

	// Shut down cleanly; serve() should return http.ErrServerClosed (not an error).
	srv.Close()
	if serveErr := <-errCh; serveErr != nil && serveErr != http.ErrServerClosed {
		t.Errorf("unexpected serve error after close: %v", serveErr)
	}
}

// TestServe_TLSMissingCert verifies fail-secure behaviour: when AF_TLS_ENABLED
// is true but cert/key paths are empty, serve() must return an error
// immediately and must NOT silently fall back to plain HTTP.
func TestServe_TLSMissingCert(t *testing.T) {
	srv := &http.Server{Addr: "127.0.0.1:0"}

	// Both empty — typical "operator forgot to set the env vars" scenario.
	err := serve(srv, true, "", "", nopLogger())
	if err == nil {
		t.Fatal("want error when TLS enabled with no cert/key, got nil")
	}

	// Error message must name the relevant env vars so the operator knows
	// exactly what to fix.
	for _, needle := range []string{"AF_TLS_CERT_FILE", "AF_TLS_KEY_FILE"} {
		if !strings.Contains(err.Error(), needle) {
			t.Errorf("error should mention %q; got: %v", needle, err)
		}
	}
}

// TestServe_TLSCertFileNotFound verifies that a non-existent cert file on disk
// causes serve() to return an error (not panic, not silent HTTP fallback).
func TestServe_TLSCertFileNotFound(t *testing.T) {
	port := freePort(t)
	srv := &http.Server{Addr: fmt.Sprintf("127.0.0.1:%d", port)}

	err := serve(srv, true, "/nonexistent/cert.crt", "/nonexistent/key.key", nopLogger())
	if err == nil {
		t.Fatal("want error for missing cert/key files on disk, got nil")
	}
	// Must not be ErrServerClosed — this is a startup configuration error.
	if err == http.ErrServerClosed {
		t.Errorf("want config/file error, got http.ErrServerClosed")
	}
}

// ─── parseSecrets() ──────────────────────────────────────────────────────────

func TestParseSecrets_MultipleSecrets(t *testing.T) {
	got := parseSecrets("new-key,old-key")
	if len(got) != 2 {
		t.Fatalf("want 2 secrets, got %d", len(got))
	}
	if got[0] != "new-key" {
		t.Errorf("want got[0]=%q, got %q", "new-key", got[0])
	}
	if got[1] != "old-key" {
		t.Errorf("want got[1]=%q, got %q", "old-key", got[1])
	}
}

func TestParseSecrets_TrimsWhitespace(t *testing.T) {
	got := parseSecrets("  alpha , beta  ,  gamma  ")
	want := []string{"alpha", "beta", "gamma"}
	if len(got) != len(want) {
		t.Fatalf("want %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] want %q, got %q", i, want[i], got[i])
		}
	}
}

func TestParseSecrets_SingleSecret(t *testing.T) {
	got := parseSecrets("only-one")
	if len(got) != 1 || got[0] != "only-one" {
		t.Errorf("want [only-one], got %v", got)
	}
}

func TestParseSecrets_EmptyFallsBackToDefault(t *testing.T) {
	got := parseSecrets("")
	if len(got) != 1 {
		t.Fatalf("empty input: want 1 default secret, got %d", len(got))
	}
	if got[0] == "" {
		t.Errorf("default secret must not be empty string")
	}
}

func TestParseSecrets_SkipsEmptySegments(t *testing.T) {
	// Comma with nothing between — should be ignored, not produce an empty secret.
	got := parseSecrets("a,,b")
	if len(got) != 2 {
		t.Fatalf("want 2 (empty segment skipped), got %v", got)
	}
	if got[0] != "a" || got[1] != "b" {
		t.Errorf("want [a b], got %v", got)
	}
}

func TestValidateProductionConfig_StrictDisabled(t *testing.T) {
	t.Setenv("AF_ENV", "")
	t.Setenv("AF_STRICT_CONFIG", "false")

	err := validateProductionConfig(true, []string{"dev-secret-change-in-production"}, "", "admin", "")
	if err != nil {
		t.Fatalf("expected nil when strict config disabled, got %v", err)
	}
}

func TestValidateProductionConfig_RejectsUnsafeDefaults(t *testing.T) {
	t.Setenv("AF_ENV", "production")
	t.Setenv("AF_STRICT_CONFIG", "")
	t.Setenv("DATABASE_URL", "postgres://prod.example/agentfabric?sslmode=require")
	t.Setenv("REDIS_URL", "redis://prod-redis:6379")
	t.Setenv("AF_CORS_ORIGINS", "https://app.agentfabric.io")

	err := validateProductionConfig(true, []string{"dev-secret-change-in-production"}, "", "admin", "")
	if err == nil {
		t.Fatal("expected unsafe production config to be rejected")
	}
}

func TestValidateProductionConfig_AcceptsSafeValues(t *testing.T) {
	t.Setenv("AF_ENV", "production")
	t.Setenv("AF_STRICT_CONFIG", "")
	t.Setenv("DATABASE_URL", "postgres://prod.example/agentfabric?sslmode=require")
	t.Setenv("REDIS_URL", "redis://prod-redis:6379")
	t.Setenv("AF_CORS_ORIGINS", "https://app.agentfabric.io")

	err := validateProductionConfig(false, []string{"super-secret"}, "shared-collector-token", "strong-password", strings.Repeat("a", 64))
	if err != nil {
		t.Fatalf("expected safe production config to pass, got %v", err)
	}
}

func TestValidateProductionConfig_RequiresDatabaseURL(t *testing.T) {
	t.Setenv("AF_ENV", "production")
	t.Setenv("AF_STRICT_CONFIG", "")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("REDIS_URL", "redis://prod-redis:6379")
	t.Setenv("AF_CORS_ORIGINS", "https://app.agentfabric.io")

	err := validateProductionConfig(false, []string{"super-secret"}, "shared-collector-token", "strong-password", strings.Repeat("a", 64))
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("expected DATABASE_URL validation error, got %v", err)
	}
}

func TestValidateProductionConfig_RequiresRedisURL(t *testing.T) {
	t.Setenv("AF_ENV", "production")
	t.Setenv("AF_STRICT_CONFIG", "")
	t.Setenv("DATABASE_URL", "postgres://prod.example/agentfabric?sslmode=require")
	t.Setenv("REDIS_URL", "")
	t.Setenv("AF_CORS_ORIGINS", "https://app.agentfabric.io")

	err := validateProductionConfig(false, []string{"super-secret"}, "shared-collector-token", "strong-password", strings.Repeat("a", 64))
	if err == nil || !strings.Contains(err.Error(), "REDIS_URL") {
		t.Fatalf("expected REDIS_URL validation error, got %v", err)
	}
}

func TestValidateProductionConfig_RequiresCORSOrigins(t *testing.T) {
	t.Setenv("AF_ENV", "production")
	t.Setenv("AF_STRICT_CONFIG", "")
	t.Setenv("DATABASE_URL", "postgres://prod.example/agentfabric?sslmode=require")
	t.Setenv("REDIS_URL", "redis://prod-redis:6379")
	t.Setenv("AF_CORS_ORIGINS", "")

	err := validateProductionConfig(false, []string{"super-secret"}, "shared-collector-token", "strong-password", strings.Repeat("a", 64))
	if err == nil || !strings.Contains(err.Error(), "AF_CORS_ORIGINS") {
		t.Fatalf("expected AF_CORS_ORIGINS validation error, got %v", err)
	}
}

func TestValidateProductionConfig_RequiresTLSFilesWhenEnabled(t *testing.T) {
	t.Setenv("AF_ENV", "production")
	t.Setenv("AF_STRICT_CONFIG", "")
	t.Setenv("DATABASE_URL", "postgres://prod.example/agentfabric?sslmode=require")
	t.Setenv("REDIS_URL", "redis://prod-redis:6379")
	t.Setenv("AF_CORS_ORIGINS", "https://app.agentfabric.io")
	t.Setenv("AF_TLS_ENABLED", "true")
	t.Setenv("AF_TLS_CERT_FILE", "")
	t.Setenv("AF_TLS_KEY_FILE", "")

	err := validateProductionConfig(false, []string{"super-secret"}, "shared-collector-token", "strong-password", strings.Repeat("a", 64))
	if err == nil || !strings.Contains(err.Error(), "AF_TLS_CERT_FILE") {
		t.Fatalf("expected TLS env validation error, got %v", err)
	}
}

func TestValidateProductionConfig_RejectsKnownDevelopmentSecretSentinel(t *testing.T) {
	t.Setenv("AF_ENV", "production")
	t.Setenv("AF_STRICT_CONFIG", "")
	t.Setenv("DATABASE_URL", "postgres://prod.example/agentfabric?sslmode=require")
	t.Setenv("REDIS_URL", "redis://prod-redis:6379")
	t.Setenv("AF_CORS_ORIGINS", "https://app.agentfabric.io")

	err := validateProductionConfig(false, []string{"dev-secret-change-in-prod"}, "shared-collector-token", "strong-password", strings.Repeat("a", 64))
	if err == nil || !strings.Contains(err.Error(), "AF_JWT_SECRET/AF_JWT_SECRETS") {
		t.Fatalf("expected development sentinel rejection, got %v", err)
	}
}

func TestValidateCollectorAuthConfig_RequiresTokenWhenEnabled(t *testing.T) {
	err := validateCollectorAuthConfig(false, "")
	if err == nil {
		t.Fatal("expected missing collector auth token to fail")
	}
}

func TestValidateCollectorAuthConfig_AllowsMissingTokenWhenAuthDisabled(t *testing.T) {
	err := validateCollectorAuthConfig(true, "")
	if err != nil {
		t.Fatalf("expected missing collector auth token to be allowed when auth is disabled, got %v", err)
	}
}

func TestLiveStreamTopologyWarning_StrictDisabled(t *testing.T) {
	if got := liveStreamTopologyWarning(false); got != "" {
		t.Fatalf("expected no warning when strict config is disabled, got %q", got)
	}
}

func TestLiveStreamTopologyWarning_StrictEnabled(t *testing.T) {
	got := liveStreamTopologyWarning(true)
	if got == "" {
		t.Fatal("expected live stream topology warning in strict mode")
	}
	for _, needle := range []string{"/api/v1/stream/live", "single api-gateway replica", "multi-replica"} {
		if !strings.Contains(got, needle) {
			t.Fatalf("expected warning to mention %q, got %q", needle, got)
		}
	}
}

func TestDefaultOIDCRedirectURI(t *testing.T) {
	if got := defaultOIDCRedirectURI(false); got != "http://localhost:8080/auth/callback" {
		t.Fatalf("expected dev OIDC redirect default, got %q", got)
	}
	if got := defaultOIDCRedirectURI(true); got != "" {
		t.Fatalf("expected strict mode to require explicit OIDC redirect URI, got %q", got)
	}
}

func TestServeSwaggerUI_DoesNotPersistAuthorization(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/docs/swagger", nil)
	rr := httptest.NewRecorder()

	serveSwaggerUI("/docs/openapi.yaml")(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `persistAuthorization: false`) {
		t.Fatalf("expected swagger UI to disable persisted authorization, got %s", body)
	}
}
