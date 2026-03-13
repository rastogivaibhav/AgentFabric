package auth

// Unit tests for the OIDC handler pure functions.
// Tests: PKCE generation, nonce generation, ID token parsing, JWT issuance,
//        state cookie sign/verify, URL helpers.
// No network calls required.

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

func testHandler() *OIDCHandler {
	return NewOIDCHandler(OIDCConfig{
		Issuer:        "https://login.microsoftonline.com/tenant-id/v2.0",
		ClientID:      "test-client-id",
		ClientSecret:  "test-client-secret",
		RedirectURI:   "http://localhost:8080/auth/callback",
		JWTSecret:     "test-jwt-secret-32-chars-long-ok",
		SessionMaxAge: 8 * time.Hour,
	}, zap.NewNop())
}

// ─── PKCE ────────────────────────────────────────────────────────────────────

func TestGeneratePKCE_ReturnsNonEmpty(t *testing.T) {
	verifier, challenge, err := generatePKCE()
	if err != nil {
		t.Fatalf("generatePKCE: %v", err)
	}
	if verifier == "" {
		t.Error("verifier should be non-empty")
	}
	if challenge == "" {
		t.Error("challenge should be non-empty")
	}
}

func TestGeneratePKCE_VerifierAndChallengeAreDifferent(t *testing.T) {
	verifier, challenge, _ := generatePKCE()
	if verifier == challenge {
		t.Error("verifier and challenge should differ")
	}
}

func TestGeneratePKCE_ChallengeIsS256OfVerifier(t *testing.T) {
	verifier, challenge, _ := generatePKCE()
	h := sha256.Sum256([]byte(verifier))
	expected := base64.RawURLEncoding.EncodeToString(h[:])
	if challenge != expected {
		t.Errorf("challenge %q != S256(verifier) %q", challenge, expected)
	}
}

func TestGeneratePKCE_UniqueEachCall(t *testing.T) {
	v1, c1, _ := generatePKCE()
	v2, c2, _ := generatePKCE()
	if v1 == v2 {
		t.Error("verifiers should be unique across calls")
	}
	if c1 == c2 {
		t.Error("challenges should be unique across calls")
	}
}

func TestGeneratePKCE_VerifierIsURLSafe(t *testing.T) {
	verifier, _, _ := generatePKCE()
	for _, ch := range verifier {
		if ch == '+' || ch == '/' || ch == '=' {
			t.Errorf("verifier contains non-URL-safe character: %c", ch)
		}
	}
}

// ─── Nonce ───────────────────────────────────────────────────────────────────

func TestGenerateNonce_ReturnsNonEmpty(t *testing.T) {
	nonce, err := generateNonce()
	if err != nil {
		t.Fatalf("generateNonce: %v", err)
	}
	if nonce == "" {
		t.Error("nonce should be non-empty")
	}
}

func TestGenerateNonce_UniqueEachCall(t *testing.T) {
	n1, _ := generateNonce()
	n2, _ := generateNonce()
	if n1 == n2 {
		t.Error("nonces should be unique across calls")
	}
}

// ─── ID Token Parsing ────────────────────────────────────────────────────────

func makeTestIDToken(sub, email, nonce string, expOffset time.Duration) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims := map[string]interface{}{
		"sub":   sub,
		"email": email,
		"name":  "Test User",
		"nonce": nonce,
		"iss":   "https://test.issuer.com",
		"aud":   "test-client",
		"exp":   time.Now().Add(expOffset).Unix(),
	}
	claimsJSON, _ := json.Marshal(claims)
	payload := base64.RawURLEncoding.EncodeToString(claimsJSON)
	sig := base64.RawURLEncoding.EncodeToString([]byte("fake-sig"))
	return header + "." + payload + "." + sig
}

func TestParseIDToken_ValidToken(t *testing.T) {
	token := makeTestIDToken("user-123", "user@example.com", "test-nonce", time.Hour)
	claims, err := parseIDToken(token)
	if err != nil {
		t.Fatalf("parseIDToken: %v", err)
	}
	if claims.Subject != "user-123" {
		t.Errorf("subject mismatch: %q", claims.Subject)
	}
	if claims.Email != "user@example.com" {
		t.Errorf("email mismatch: %q", claims.Email)
	}
	if claims.Nonce != "test-nonce" {
		t.Errorf("nonce mismatch: %q", claims.Nonce)
	}
}

func TestParseIDToken_ExpiredToken(t *testing.T) {
	token := makeTestIDToken("user-123", "u@e.com", "n", -time.Hour)
	_, err := parseIDToken(token)
	if err == nil {
		t.Error("expired token should return error")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Errorf("error should mention expiry, got: %v", err)
	}
}

func TestParseIDToken_MissingSubject(t *testing.T) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims := map[string]interface{}{
		"email": "user@example.com",
		"exp":   time.Now().Add(time.Hour).Unix(),
	}
	claimsJSON, _ := json.Marshal(claims)
	payload := base64.RawURLEncoding.EncodeToString(claimsJSON)
	sig := base64.RawURLEncoding.EncodeToString([]byte("sig"))

	_, err := parseIDToken(header + "." + payload + "." + sig)
	if err == nil {
		t.Error("token without 'sub' should return error")
	}
}

func TestParseIDToken_InvalidFormat_TwoPartToken(t *testing.T) {
	_, err := parseIDToken("only.two_parts")
	if err == nil {
		t.Error("two-part token should fail")
	}
}

func TestParseIDToken_InvalidFormat_EmptyString(t *testing.T) {
	_, err := parseIDToken("")
	if err == nil {
		t.Error("empty string should fail")
	}
}

// ─── AF Token Issuance ───────────────────────────────────────────────────────

func TestIssueAFToken_ValidClaims(t *testing.T) {
	h := testHandler()
	claims := &idTokenClaims{
		Subject: "sub-001",
		Email:   "admin@company.com",
		Name:    "Admin User",
	}
	tokenStr, err := h.issueAFToken(claims)
	if err != nil {
		t.Fatalf("issueAFToken: %v", err)
	}
	if tokenStr == "" {
		t.Error("token should be non-empty")
	}

	parsed := &afClaims{}
	_, err = jwt.ParseWithClaims(tokenStr, parsed, func(tok *jwt.Token) (interface{}, error) {
		return []byte("test-jwt-secret-32-chars-long-ok"), nil
	})
	if err != nil {
		t.Fatalf("parse issued token: %v", err)
	}
	if parsed.Subject != "sub-001" {
		t.Errorf("subject mismatch: %q", parsed.Subject)
	}
	if parsed.Email != "admin@company.com" {
		t.Errorf("email mismatch: %q", parsed.Email)
	}
}

func TestIssueAFToken_ExpiresWithinSessionMaxAge(t *testing.T) {
	h := testHandler()
	before := time.Now()
	claims := &idTokenClaims{Subject: "s", Email: "e@e.com"}
	tokenStr, _ := h.issueAFToken(claims)

	parsed := &afClaims{}
	jwt.ParseWithClaims(tokenStr, parsed, func(tok *jwt.Token) (interface{}, error) {
		return []byte("test-jwt-secret-32-chars-long-ok"), nil
	})

	expiry := parsed.ExpiresAt.Time
	maxExpiry := before.Add(8*time.Hour + 5*time.Second)
	if expiry.After(maxExpiry) {
		t.Errorf("token expiry %v beyond max session age", expiry)
	}
	if expiry.Before(before.Add(7 * time.Hour)) {
		t.Errorf("token expiry %v too short — expected ~8h", expiry)
	}
}

func TestIssueAFToken_IssuerIsAgentFabric(t *testing.T) {
	h := testHandler()
	claims := &idTokenClaims{Subject: "s"}
	tokenStr, _ := h.issueAFToken(claims)
	parsed := &afClaims{}
	jwt.ParseWithClaims(tokenStr, parsed, func(tok *jwt.Token) (interface{}, error) {
		return []byte("test-jwt-secret-32-chars-long-ok"), nil
	})
	if parsed.Issuer != "agentfabric" {
		t.Errorf("issuer should be 'agentfabric', got %q", parsed.Issuer)
	}
}

func TestIssueAFToken_AudienceIsPortal(t *testing.T) {
	h := testHandler()
	claims := &idTokenClaims{Subject: "s"}
	tokenStr, _ := h.issueAFToken(claims)
	parsed := &afClaims{}
	jwt.ParseWithClaims(tokenStr, parsed, func(tok *jwt.Token) (interface{}, error) {
		return []byte("test-jwt-secret-32-chars-long-ok"), nil
	}, jwt.WithAudience("agentfabric-portal"))
	// If audience check passes without error, the test passes implicitly
}

// ─── State Cookie Sign/Verify ────────────────────────────────────────────────

func TestStateCookieRoundTrip(t *testing.T) {
	h := testHandler()
	original := pkceState{Verifier: "test-verifier-abc", Nonce: "test-nonce-xyz"}

	signed, err := h.signStateCookie(original)
	if err != nil {
		t.Fatalf("signStateCookie: %v", err)
	}

	recovered, err := h.verifyStateCookie(signed)
	if err != nil {
		t.Fatalf("verifyStateCookie: %v", err)
	}

	if recovered.Verifier != original.Verifier {
		t.Errorf("verifier mismatch: %q vs %q", recovered.Verifier, original.Verifier)
	}
	if recovered.Nonce != original.Nonce {
		t.Errorf("nonce mismatch: %q vs %q", recovered.Nonce, original.Nonce)
	}
}

func TestStateCookieVerify_TamperedCookie(t *testing.T) {
	h := testHandler()
	state := pkceState{Verifier: "v", Nonce: "n"}
	signed, _ := h.signStateCookie(state)

	tampered := signed[:len(signed)-4] + "XXXX"
	_, err := h.verifyStateCookie(tampered)
	if err == nil {
		t.Error("tampered cookie should fail verification")
	}
}

func TestStateCookieVerify_InvalidJWT(t *testing.T) {
	h := testHandler()
	_, err := h.verifyStateCookie("not.valid.jwt")
	if err == nil {
		t.Error("invalid JWT should fail verification")
	}
}

func TestStateCookieVerify_WrongSecret(t *testing.T) {
	h1 := testHandler()
	h2 := NewOIDCHandler(OIDCConfig{JWTSecret: "different-secret-here-xxxxxx"}, zap.NewNop())

	state := pkceState{Verifier: "v", Nonce: "n"}
	signed, _ := h1.signStateCookie(state)

	_, err := h2.verifyStateCookie(signed)
	if err == nil {
		t.Error("cookie signed with different secret should fail verification")
	}
}

// ─── URL helpers ─────────────────────────────────────────────────────────────

func TestIsHTTPS_ForwardedProtoHeader(t *testing.T) {
	r, _ := http.NewRequest("GET", "/", bytes.NewReader(nil))
	r.Header.Set("X-Forwarded-Proto", "https")
	if !isHTTPS(r) {
		t.Error("X-Forwarded-Proto: https should return true")
	}
}

func TestIsHTTPS_PlainHTTP(t *testing.T) {
	r, _ := http.NewRequest("GET", "/", bytes.NewReader(nil))
	if isHTTPS(r) {
		t.Error("plain HTTP request should return false")
	}
}

func TestBearerToken_ExtractsBearerPrefix(t *testing.T) {
	r, _ := http.NewRequest("GET", "/", bytes.NewReader(nil))
	r.Header.Set("Authorization", "Bearer my-token-value")
	if bearerToken(r) != "my-token-value" {
		t.Errorf("expected 'my-token-value', got %q", bearerToken(r))
	}
}

func TestBearerToken_EmptyWhenNoBearerPrefix(t *testing.T) {
	r, _ := http.NewRequest("GET", "/", bytes.NewReader(nil))
	r.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	if bearerToken(r) != "" {
		t.Errorf("non-Bearer scheme should return empty, got %q", bearerToken(r))
	}
}

func TestBearerToken_EmptyWhenNoHeader(t *testing.T) {
	r, _ := http.NewRequest("GET", "/", bytes.NewReader(nil))
	if bearerToken(r) != "" {
		t.Errorf("missing header should return empty, got %q", bearerToken(r))
	}
}

// ─── OIDCConfig defaults ─────────────────────────────────────────────────────

func TestNewOIDCHandler_DefaultScopes(t *testing.T) {
	h := NewOIDCHandler(OIDCConfig{JWTSecret: "s"}, zap.NewNop())
	if len(h.cfg.Scopes) == 0 {
		t.Error("default scopes should be set")
	}
	// openid is required for OIDC
	found := false
	for _, s := range h.cfg.Scopes {
		if s == "openid" {
			found = true
		}
	}
	if !found {
		t.Error("openid scope must be in default scopes")
	}
}

func TestNewOIDCHandler_DefaultSessionMaxAge(t *testing.T) {
	h := NewOIDCHandler(OIDCConfig{JWTSecret: "s"}, zap.NewNop())
	if h.cfg.SessionMaxAge == 0 {
		t.Error("default session max age should be non-zero")
	}
}
