package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agentfabric/collector/internal/config"
)

func TestNewHTTPServer_HardenedTimeouts(t *testing.T) {
	srv := newHTTPServer(":4318", http.NewServeMux())
	if srv.ReadTimeout != 10*time.Second {
		t.Fatalf("expected ReadTimeout=10s, got %v", srv.ReadTimeout)
	}
	if srv.ReadHeaderTimeout != 10*time.Second {
		t.Fatalf("expected ReadHeaderTimeout=10s, got %v", srv.ReadHeaderTimeout)
	}
	if srv.WriteTimeout != 30*time.Second {
		t.Fatalf("expected WriteTimeout=30s, got %v", srv.WriteTimeout)
	}
	if srv.IdleTimeout != 120*time.Second {
		t.Fatalf("expected IdleTimeout=120s, got %v", srv.IdleTimeout)
	}
	if srv.MaxHeaderBytes != 1<<20 {
		t.Fatalf("expected MaxHeaderBytes=1MiB, got %d", srv.MaxHeaderBytes)
	}
}

func TestCollectorReady_DoesNotFollowGatewayRedirects(t *testing.T) {
	redirected := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirected = true
		http.Redirect(w, r, "/healthz", http.StatusFound)
	}))
	defer ts.Close()

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr := httptest.NewRecorder()
	cfg := &config.Config{}
	cfg.Gateway.Endpoint = ts.URL
	cfg.Gateway.AuthToken = "gateway-token"

	collectorReady(rr, req, cfg)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected readyz redirect probe to degrade readiness, got %d", rr.Code)
	}
	if !redirected {
		t.Fatal("expected gateway ready probe to contact the upstream once")
	}
	if body := rr.Body.String(); body == "" || !containsAll(body, "gateway readyz returned non-200", "\"status_code\":302") {
		t.Fatalf("expected redirect failure details in readiness body, got %s", body)
	}
}

func containsAll(s string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(s, needle) {
			return false
		}
	}
	return true
}
