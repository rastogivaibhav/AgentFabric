package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ─── Helpers ─────────────────────────────────────────────────────────────────

const testSecret = "test-jwt-secret-for-middleware-tests"
const testTenantUUID = "11111111-2222-3333-4444-555555555555"

func makeToken(secret string, tenantID string, valid bool) string {
	claims := &Claims{
		TenantID: tenantID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	if !valid {
		claims.RegisteredClaims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Hour))
	}
	tok, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	return tok
}

func okHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// ─── JWTAuth ─────────────────────────────────────────────────────────────────

func TestJWTAuth_ValidToken_Passes(t *testing.T) {
	tok := makeToken(testSecret, testTenantUUID, true)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()

	JWTAuth(testSecret)(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestJWTAuth_MissingHeader_Returns401(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()

	JWTAuth(testSecret)(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestJWTAuth_InvalidFormat_Returns401(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	rr := httptest.NewRecorder()

	JWTAuth(testSecret)(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestJWTAuth_ExpiredToken_Returns401(t *testing.T) {
	tok := makeToken(testSecret, testTenantUUID, false)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()

	JWTAuth(testSecret)(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestJWTAuth_WrongSecret_Returns401(t *testing.T) {
	tok := makeToken("different-secret-xxxxx", testTenantUUID, true)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()

	JWTAuth(testSecret)(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestJWTAuth_InjectsClaims_IntoContext(t *testing.T) {
	tok := makeToken(testSecret, testTenantUUID, true)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()

	var gotClaims *Claims
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClaims = ClaimsFromCtx(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	JWTAuth(testSecret)(inner).ServeHTTP(rr, req)

	if gotClaims == nil {
		t.Fatal("claims not injected into context")
	}
	if gotClaims.TenantID != testTenantUUID {
		t.Errorf("tenant_id mismatch: %q", gotClaims.TenantID)
	}
}

func TestJWTAuth_BearerCaseInsensitive(t *testing.T) {
	tok := makeToken(testSecret, testTenantUUID, true)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "BEARER "+tok)
	rr := httptest.NewRecorder()

	JWTAuth(testSecret)(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("BEARER (caps) should be accepted, got %d", rr.Code)
	}
}

func TestJWTAuth_MissingTenantClaim_Returns401(t *testing.T) {
	tok := makeToken(testSecret, "", true)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()

	JWTAuth(testSecret)(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for missing tenant_id, got %d", rr.Code)
	}
}

// ─── TenantInjector ──────────────────────────────────────────────────────────

func TestTenantInjector_WithClaims_UsesTenantID(t *testing.T) {
	// tenant_id must be a valid UUID — the DB schema defines the column as UUID type.
	tok := makeToken(testSecret, testTenantUUID, true)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()

	var gotTenant string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTenant = TenantIDFromCtx(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	// Chain JWTAuth → TenantInjector so claims are present
	JWTAuth(testSecret)(TenantInjector(inner)).ServeHTTP(rr, req)

	if gotTenant != testTenantUUID {
		t.Errorf("expected %q, got %q", testTenantUUID, gotTenant)
	}
}

func TestTenantInjector_InvalidUUID_Returns403(t *testing.T) {
	// "tenant-prod" is NOT a valid UUID — TenantInjector must reject it to
	// prevent a PostgreSQL cast error when the value reaches a UUID column.
	tok := makeToken(testSecret, "tenant-prod", true)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("inner handler should not be reached with invalid tenant_id")
		w.WriteHeader(http.StatusOK)
	})

	JWTAuth(testSecret)(TenantInjector(inner)).ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 for non-UUID tenant_id, got %d", rr.Code)
	}
}

func TestTenantInjector_WithoutClaims_UsesDefault(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()

	var gotTenant string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTenant = TenantIDFromCtx(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	TenantInjector(inner).ServeHTTP(rr, req)

	if gotTenant != DefaultTenantID {
		t.Errorf("expected DefaultTenantID %q, got %q", DefaultTenantID, gotTenant)
	}
}

func TestTenantInjector_P2_TenantIsolation_NeverEmpty(t *testing.T) {
	// Principle 2: every request must have a tenant_id — never empty string
	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()

	var gotTenant string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTenant = TenantIDFromCtx(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	TenantInjector(inner).ServeHTTP(rr, req)

	if gotTenant == "" {
		t.Error("Principle 2 violation: tenant_id must never be empty")
	}
}

// ─── CollectorAuth ────────────────────────────────────────────────────────────

func TestCollectorAuth_ValidSourceAndToken_Passes(t *testing.T) {
	req := httptest.NewRequest("POST", "/internal/ingest", nil)
	req.Header.Set("X-AF-Source", "collector")
	req.Header.Set("Authorization", "Bearer "+testSecret)
	rr := httptest.NewRecorder()

	CollectorAuth(testSecret)(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestCollectorAuth_MissingSourceHeader_Returns403(t *testing.T) {
	req := httptest.NewRequest("POST", "/internal/ingest", nil)
	req.Header.Set("Authorization", "Bearer "+testSecret)
	rr := httptest.NewRecorder()

	CollectorAuth(testSecret)(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestCollectorAuth_WrongSource_Returns403(t *testing.T) {
	req := httptest.NewRequest("POST", "/internal/ingest", nil)
	req.Header.Set("X-AF-Source", "portal") // wrong source
	req.Header.Set("Authorization", "Bearer "+testSecret)
	rr := httptest.NewRecorder()

	CollectorAuth(testSecret)(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestCollectorAuth_MissingToken_Returns401(t *testing.T) {
	req := httptest.NewRequest("POST", "/internal/ingest", nil)
	req.Header.Set("X-AF-Source", "collector")
	rr := httptest.NewRecorder()

	CollectorAuth(testSecret)(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestCollectorAuth_InvalidToken_Returns401(t *testing.T) {
	req := httptest.NewRequest("POST", "/internal/ingest", nil)
	req.Header.Set("X-AF-Source", "collector")
	req.Header.Set("Authorization", "Bearer wrong-token")
	rr := httptest.NewRecorder()

	CollectorAuth(testSecret)(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestCollectorAuth_WrongSecret_Returns401(t *testing.T) {
	req := httptest.NewRequest("POST", "/internal/ingest", nil)
	req.Header.Set("X-AF-Source", "collector")
	req.Header.Set("Authorization", "Bearer attacker-secret-xxxxx")
	rr := httptest.NewRecorder()

	CollectorAuth(testSecret)(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestCollectorAuth_InvalidAuthScheme_Returns401(t *testing.T) {
	req := httptest.NewRequest("POST", "/internal/ingest", nil)
	req.Header.Set("X-AF-Source", "collector")
	req.Header.Set("Authorization", "Token "+testSecret)
	rr := httptest.NewRecorder()

	CollectorAuth(testSecret)(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestCollectorAuth_EmptyConfiguredToken_Returns503(t *testing.T) {
	req := httptest.NewRequest("POST", "/internal/ingest", nil)
	req.Header.Set("X-AF-Source", "collector")
	req.Header.Set("Authorization", "Bearer anything")
	rr := httptest.NewRecorder()

	CollectorAuth("")(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}
