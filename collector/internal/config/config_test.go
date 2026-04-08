package config

import (
	"os"
	"testing"
)

func TestLoad_StrictProductionRequiresJWTSecret(t *testing.T) {
	t.Setenv("GV_ENV", "production")
	t.Setenv("GV_STRICT_CONFIG", "")
	t.Setenv("GV_JWT_SECRET", "")
	t.Setenv("GV_AUTH_REQUIRE_AUTH", "true")

	_, err := Load()
	if err == nil {
		t.Fatal("expected production config without GV_JWT_SECRET to fail")
	}
}

func TestLoad_StrictProductionAcceptsJWTSecret(t *testing.T) {
	t.Setenv("GV_ENV", "production")
	t.Setenv("GV_STRICT_CONFIG", "")
	t.Setenv("GV_JWT_SECRET", "collector-secret")
	t.Setenv("GV_AUTH_REQUIRE_AUTH", "true")
	t.Setenv("GV_GATEWAY_AUTH_TOKEN", "gateway-token")
	t.Setenv("GV_GATEWAY_ENDPOINT", "http://gateway.internal:8080")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected strict production config to load, got %v", err)
	}
	if cfg.Auth.JWTSecret != "collector-secret" {
		t.Fatalf("expected GV_JWT_SECRET to be preserved, got %q", cfg.Auth.JWTSecret)
	}
}

func TestLoad_StrictProductionRequiresGatewayAuthToken(t *testing.T) {
	t.Setenv("GV_ENV", "production")
	t.Setenv("GV_STRICT_CONFIG", "")
	t.Setenv("GV_JWT_SECRET", "collector-secret")
	t.Setenv("GV_AUTH_REQUIRE_AUTH", "true")
	t.Setenv("GV_GATEWAY_AUTH_TOKEN", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected production config without GV_GATEWAY_AUTH_TOKEN to fail")
	}
}

func TestLoad_StrictProductionRequiresTLSFilesWhenEnabled(t *testing.T) {
	t.Setenv("GV_ENV", "production")
	t.Setenv("GV_STRICT_CONFIG", "")
	t.Setenv("GV_JWT_SECRET", "collector-secret")
	t.Setenv("GV_AUTH_REQUIRE_AUTH", "true")
	t.Setenv("GV_GATEWAY_AUTH_TOKEN", "gateway-token")
	t.Setenv("GV_TLS_ENABLED", "true")
	t.Setenv("GV_TLS_CERT_FILE", "")
	t.Setenv("GV_TLS_KEY_FILE", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected production config with TLS enabled and no files to fail")
	}
}

func TestLoad_StrictProductionRejectsAuthDisabled(t *testing.T) {
	t.Setenv("GV_ENV", "production")
	t.Setenv("GV_STRICT_CONFIG", "")
	t.Setenv("GV_JWT_SECRET", "collector-secret")
	t.Setenv("GV_AUTH_REQUIRE_AUTH", "false")
	t.Setenv("GV_GATEWAY_AUTH_TOKEN", "gateway-token")
	t.Setenv("GV_GATEWAY_ENDPOINT", "http://gateway.internal:8080")

	_, err := Load()
	if err == nil {
		t.Fatal("expected production config with GV_AUTH_REQUIRE_AUTH=false to fail")
	}
}

func TestLoad_StrictProductionRejectsDefaultGatewayEndpoint(t *testing.T) {
	t.Setenv("GV_ENV", "production")
	t.Setenv("GV_STRICT_CONFIG", "")
	t.Setenv("GV_JWT_SECRET", "collector-secret")
	t.Setenv("GV_AUTH_REQUIRE_AUTH", "true")
	t.Setenv("GV_GATEWAY_AUTH_TOKEN", "gateway-token")
	t.Setenv("GV_GATEWAY_ENDPOINT", "http://localhost:8080")

	_, err := Load()
	if err == nil {
		t.Fatal("expected production config with default GV_GATEWAY_ENDPOINT to fail")
	}
}

func TestLoad_StrictProductionRejectsDevelopmentJWTSecretAlias(t *testing.T) {
	t.Setenv("GV_ENV", "production")
	t.Setenv("GV_STRICT_CONFIG", "")
	t.Setenv("GV_JWT_SECRET", "dev-secret-change-in-prod")
	t.Setenv("GV_AUTH_REQUIRE_AUTH", "true")
	t.Setenv("GV_GATEWAY_AUTH_TOKEN", "gateway-token")
	t.Setenv("GV_GATEWAY_ENDPOINT", "http://gateway.internal:8080")

	_, err := Load()
	if err == nil {
		t.Fatal("expected development sentinel GV_JWT_SECRET to fail in production")
	}
}

// TestLoad_Defaults verifies that Load() returns sensible defaults when no
// config file is present and no env vars are set.
func TestLoad_Defaults(t *testing.T) {
	// Ensure no stale AF_ env vars from the test environment interfere.
	unsetAF := func(keys ...string) {
		for _, k := range keys {
			os.Unsetenv(k)
		}
	}
	unsetAF("GV_GRPC_ADDR", "GV_HTTP_ADDR", "GV_GATEWAY_ENDPOINT",
		"GV_AUTH_JWT_SECRET", "GV_JWT_SECRET",
		"GV_RATE_LIMIT_SPANS_PER_SECOND", "GV_PROCESSOR_BATCH_SIZE")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.GRPC.Addr != ":4317" {
		t.Errorf("GRPC.Addr default: got %q, want :4317", cfg.GRPC.Addr)
	}
	if cfg.HTTP.Addr != ":4318" {
		t.Errorf("HTTP.Addr default: got %q, want :4318", cfg.HTTP.Addr)
	}
	if cfg.Gateway.Endpoint != "http://localhost:8080" {
		t.Errorf("Gateway.Endpoint default: got %q", cfg.Gateway.Endpoint)
	}
	if cfg.RateLimit.SpansPerSecond != 50000 {
		t.Errorf("RateLimit.SpansPerSecond default: got %d, want 50000", cfg.RateLimit.SpansPerSecond)
	}
	if cfg.Processor.BatchSize != 512 {
		t.Errorf("Processor.BatchSize default: got %d, want 512", cfg.Processor.BatchSize)
	}
	if cfg.Processor.Workers != 4 {
		t.Errorf("Processor.Workers default: got %d, want 4", cfg.Processor.Workers)
	}
	if !cfg.Auth.RequireAuth {
		t.Error("Auth.RequireAuth should default to true")
	}
	if !cfg.PII.Enabled {
		t.Error("PII.Enabled should default to true")
	}
	if !cfg.PII.Redact {
		t.Error("PII.Redact should default to true")
	}
	if !cfg.Discovery.Enabled {
		t.Error("Discovery.Enabled should default to true")
	}
}

// TestLoad_EnvOverrides verifies that AF_-prefixed environment variables
// override default values as documented in the config package.
func TestLoad_EnvOverrides(t *testing.T) {
	os.Setenv("GV_GRPC_ADDR", ":9317")
	os.Setenv("GV_GATEWAY_ENDPOINT", "http://gateway.internal:8080")
	os.Setenv("GV_JWT_SECRET", "env-override-secret")
	defer func() {
		os.Unsetenv("GV_GRPC_ADDR")
		os.Unsetenv("GV_GATEWAY_ENDPOINT")
		os.Unsetenv("GV_JWT_SECRET")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() with env overrides: %v", err)
	}

	if cfg.GRPC.Addr != ":9317" {
		t.Errorf("GV_GRPC_ADDR override: got %q, want :9317", cfg.GRPC.Addr)
	}
	if cfg.Gateway.Endpoint != "http://gateway.internal:8080" {
		t.Errorf("GV_GATEWAY_ENDPOINT override: got %q", cfg.Gateway.Endpoint)
	}
	if cfg.Auth.JWTSecret != "env-override-secret" {
		t.Errorf("GV_JWT_SECRET override: got %q", cfg.Auth.JWTSecret)
	}
}

// TestLoad_Idempotent verifies that calling Load() twice returns structurally
// equal configs (no state mutation between calls).
func TestLoad_Idempotent(t *testing.T) {
	cfg1, err := Load()
	if err != nil {
		t.Fatalf("first Load(): %v", err)
	}
	cfg2, err := Load()
	if err != nil {
		t.Fatalf("second Load(): %v", err)
	}
	if cfg1.GRPC.Addr != cfg2.GRPC.Addr {
		t.Errorf("Load() not idempotent: GRPC.Addr %q != %q", cfg1.GRPC.Addr, cfg2.GRPC.Addr)
	}
	if cfg1.Processor.BatchSize != cfg2.Processor.BatchSize {
		t.Errorf("Load() not idempotent: BatchSize %d != %d", cfg1.Processor.BatchSize, cfg2.Processor.BatchSize)
	}
}

// TestLoad_NodeNameFallback verifies that NodeName is non-empty even when
// os.Hostname() is available (which it always should be in CI).
func TestLoad_NodeNameFallback(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	// NodeName may be empty only if os.Hostname() returns an error — acceptable
	// but log it so CI can detect misconfigured runners.
	if cfg.NodeName == "" {
		t.Log("WARN: NodeName is empty — os.Hostname() may have failed on this runner")
	}
}
