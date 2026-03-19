package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestSecurityHeaders verifies that SecurityHeaders sets every required
// defensive header on every response.
func TestSecurityHeaders(t *testing.T) {
	// Minimal downstream handler that always returns 200
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := SecurityHeaders(next)

	req  := httptest.NewRequest(http.MethodGet, "/", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	headers := resp.Result().Header

	cases := []struct {
		header string
		want   string // substring expected in the header value
	}{
		{"Content-Security-Policy", "default-src 'self'"},
		{"Content-Security-Policy", "frame-ancestors 'none'"},
		{"Strict-Transport-Security", "max-age=63072000"},
		{"Strict-Transport-Security", "includeSubDomains"},
		{"X-Frame-Options", "DENY"},
		{"X-Content-Type-Options", "nosniff"},
		{"Referrer-Policy", "strict-origin-when-cross-origin"},
		{"Permissions-Policy", "camera=()"},
	}

	for _, tc := range cases {
		got := headers.Get(tc.header)
		if got == "" {
			t.Errorf("%s: header not set", tc.header)
			continue
		}
		found := false
		for i := 0; i <= len(got)-len(tc.want); i++ {
			if got[i:i+len(tc.want)] == tc.want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s: want substring %q in %q", tc.header, tc.want, got)
		}
	}
}

// TestSecurityHeaders_PassesThrough verifies the middleware does not swallow
// the upstream response status code.
func TestSecurityHeaders_PassesThrough(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot) // 418
	})

	handler := SecurityHeaders(next)
	req  := httptest.NewRequest(http.MethodGet, "/", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusTeapot {
		t.Errorf("expected status 418, got %d", resp.Code)
	}
}
