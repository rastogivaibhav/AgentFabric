package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestOpenAPISpec_IsWellFormedAndCoversCoreRoutes(t *testing.T) {
	specPath := openAPISpecRepoPath(t)

	raw, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("read openapi spec: %v", err)
	}

	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse openapi yaml: %v", err)
	}

	if got, _ := doc["openapi"].(string); got == "" {
		t.Fatal("openapi field missing")
	}
	if _, ok := doc["info"].(map[string]any); !ok {
		t.Fatal("info section missing")
	}
	paths, ok := doc["paths"].(map[string]any)
	if !ok || len(paths) == 0 {
		t.Fatal("paths section missing or empty")
	}
	components, ok := doc["components"].(map[string]any)
	if !ok || len(components) == 0 {
		t.Fatal("components section missing or empty")
	}

	requiredPaths := []string{
		"/healthz",
		"/auth/login",
		"/auth/me",
		"/internal/ingest",
		"/api/v1/traces",
		"/api/v1/agents",
		"/api/v1/runs",
		"/api/v1/analytics/overview",
		"/api/v1/users",
		"/api/v1/budgets/{tenant_id}",
		"/api/v1/pricing",
		"/api/v1/keys",
		"/proxy/{provider}/v1/{path}",
	}
	for _, p := range requiredPaths {
		if _, ok := paths[p]; !ok {
			t.Fatalf("required path missing from openapi spec: %s", p)
		}
	}
}

func TestServeSwaggerUI_EmbedsSpecURL(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/docs/swagger", nil)
	rec := httptest.NewRecorder()

	serveSwaggerUI("/docs/openapi.yaml").ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "SwaggerUIBundle") {
		t.Fatal("swagger ui bootstrap missing from response")
	}
	if !strings.Contains(body, "/docs/openapi.yaml") {
		t.Fatal("spec URL missing from swagger ui response")
	}
}

func TestServeOpenAPISpec_ServesConfiguredFile(t *testing.T) {
	specPath := openAPISpecRepoPath(t)
	req := httptest.NewRequest(http.MethodGet, "/docs/openapi.yaml", nil)
	rec := httptest.NewRecorder()

	serveOpenAPISpec(specPath).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/yaml") {
		t.Fatalf("want content-type containing application/yaml, got %q", ct)
	}
	if !strings.Contains(rec.Body.String(), "openapi: 3.0.3") {
		t.Fatal("served file does not look like the openapi spec")
	}
}

func openAPISpecRepoPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "docs", "openapi.yaml"))
}
