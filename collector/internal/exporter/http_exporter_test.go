package exporter

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"
)

// newTestExporter creates an HTTPExporter pointed at the given test server URL.
func newTestExporter(t *testing.T, serverURL string) *HTTPExporter {
	t.Helper()
	return NewHTTPExporter(serverURL, "test-token", zap.NewNop())
}

// ─── Export ───────────────────────────────────────────────────────────────────

func TestExport_Success(t *testing.T) {
	var receivedBody map[string]interface{}
	var receivedAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	e := newTestExporter(t, srv.URL)
	spans := []map[string]string{{"span_id": "abc123", "name": "test-span"}}

	if err := e.Export(context.Background(), spans); err != nil {
		t.Fatalf("Export() error: %v", err)
	}

	// Authorization header must be set
	if receivedAuth != "Bearer test-token" {
		t.Errorf("Authorization header: got %q, want %q", receivedAuth, "Bearer test-token")
	}

	// Body must contain "spans" key
	if _, ok := receivedBody["spans"]; !ok {
		t.Errorf("exported body missing 'spans' key: %v", receivedBody)
	}
}

func TestExport_ServerError_Returns5xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	e := newTestExporter(t, srv.URL)
	err := e.Export(context.Background(), []string{"span"})
	if err == nil {
		t.Error("Export() should return error when gateway responds 500")
	}
}

func TestExport_ContextCancelled(t *testing.T) {
	// Server that blocks until it receives a request — context will be cancelled first.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	e := newTestExporter(t, srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := e.Export(ctx, []string{"span"})
	if err == nil {
		t.Error("Export() should return error when context is cancelled")
	}
}

func TestExport_InvalidEndpoint(t *testing.T) {
	e := NewHTTPExporter("http://127.0.0.1:1", "token", zap.NewNop()) // nothing listens on port 1
	err := e.Export(context.Background(), []string{"span"})
	if err == nil {
		t.Error("Export() should return error when endpoint is unreachable")
	}
}

func TestExport_SetsRequiredHeaders(t *testing.T) {
	var headers http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	e := newTestExporter(t, srv.URL)
	e.Export(context.Background(), nil) //nolint:errcheck

	if ct := headers.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: got %q, want application/json", ct)
	}
	if src := headers.Get("X-AF-Source"); src != "collector" {
		t.Errorf("X-AF-Source: got %q, want collector", src)
	}
}

func TestExport_EmptySpans_StillSendsRequest(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	e := newTestExporter(t, srv.URL)
	if err := e.Export(context.Background(), []interface{}{}); err != nil {
		t.Fatalf("Export() with empty spans: %v", err)
	}
	if !called {
		t.Error("Export() with empty spans should still send an HTTP request")
	}
}
