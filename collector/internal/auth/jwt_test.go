package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const testSecret = "test-secret-32-bytes-for-hs256!!"

func signedToken(t *testing.T, secret string, claims jwtlib.MapClaims) string {
	t.Helper()
	tok := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, claims)
	s, err := tok.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return s
}

// ─── JWTValidator ─────────────────────────────────────────────────────────────

func TestValidateToken_Valid(t *testing.T) {
	v := NewJWTValidator(testSecret, true)
	tok := signedToken(t, testSecret, jwtlib.MapClaims{
		"sub": "collector",
		"exp": jwtlib.NewNumericDate(time.Now().Add(5 * time.Minute)),
	})
	if err := v.ValidateToken(tok); err != nil {
		t.Errorf("valid token rejected: %v", err)
	}
}

func TestValidateToken_WrongSecret(t *testing.T) {
	v := NewJWTValidator(testSecret, true)
	tok := signedToken(t, "different-secret-32-bytes-aaaaa!", jwtlib.MapClaims{
		"sub": "collector",
		"exp": jwtlib.NewNumericDate(time.Now().Add(5 * time.Minute)),
	})
	if err := v.ValidateToken(tok); err == nil {
		t.Error("token signed with wrong secret should be rejected")
	}
}

func TestValidateToken_Expired(t *testing.T) {
	v := NewJWTValidator(testSecret, true)
	tok := signedToken(t, testSecret, jwtlib.MapClaims{
		"sub": "collector",
		"exp": jwtlib.NewNumericDate(time.Now().Add(-1 * time.Minute)), // already expired
	})
	if err := v.ValidateToken(tok); err == nil {
		t.Error("expired token should be rejected")
	}
}

func TestValidateToken_Malformed(t *testing.T) {
	v := NewJWTValidator(testSecret, true)
	if err := v.ValidateToken("not.a.jwt"); err == nil {
		t.Error("malformed token should be rejected")
	}
}

func TestValidateToken_EmptySecret_AllowsAll(t *testing.T) {
	// Empty secret = auth disabled; all tokens pass.
	v := NewJWTValidator("", false)
	if err := v.ValidateToken("literally-anything"); err != nil {
		t.Errorf("empty-secret validator should pass all tokens, got: %v", err)
	}
}

func TestValidateHTTP_ValidBearerHeader(t *testing.T) {
	v := NewJWTValidator(testSecret, true)
	tok := signedToken(t, testSecret, jwtlib.MapClaims{
		"sub": "collector",
		"exp": jwtlib.NewNumericDate(time.Now().Add(5 * time.Minute)),
	})
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	if err := v.ValidateHTTP(req); err != nil {
		t.Errorf("valid bearer header rejected: %v", err)
	}
}

func TestValidateHTTP_MissingHeader(t *testing.T) {
	v := NewJWTValidator(testSecret, true)
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	if err := v.ValidateHTTP(req); err == nil {
		t.Error("missing Authorization header should be rejected")
	}
}

func TestValidateHTTP_MalformedHeader(t *testing.T) {
	v := NewJWTValidator(testSecret, true)
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Token abc123") // not "Bearer"
	if err := v.ValidateHTTP(req); err == nil {
		t.Error("non-bearer auth header should be rejected")
	}
}

func TestValidateHTTP_BearerCaseInsensitive(t *testing.T) {
	v := NewJWTValidator(testSecret, true)
	tok := signedToken(t, testSecret, jwtlib.MapClaims{
		"sub": "collector",
		"exp": jwtlib.NewNumericDate(time.Now().Add(5 * time.Minute)),
	})
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "BEARER "+tok)
	if err := v.ValidateHTTP(req); err != nil {
		t.Errorf("case-insensitive bearer header rejected: %v", err)
	}
}

func TestValidateHTTP_EmptyBearerTokenRejected(t *testing.T) {
	v := NewJWTValidator(testSecret, true)
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer ")
	if err := v.ValidateHTTP(req); err == nil {
		t.Error("empty bearer token should be rejected")
	}
}

func TestGRPCTokenValidator_BearerCaseInsensitive(t *testing.T) {
	v := NewJWTValidator(testSecret, true)
	tok := signedToken(t, testSecret, jwtlib.MapClaims{
		"sub": "collector",
		"exp": jwtlib.NewNumericDate(time.Now().Add(5 * time.Minute)),
	})
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "BEARER "+tok))
	interceptor := GRPCTokenValidator(v)
	called := false
	_, err := interceptor(ctx, "req", &grpc.UnaryServerInfo{FullMethod: "/otlp.TraceService/Export"}, func(ctx context.Context, req interface{}) (interface{}, error) {
		called = true
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("expected case-insensitive bearer token to pass gRPC validation, got %v", err)
	}
	if !called {
		t.Fatal("expected gRPC handler to be invoked")
	}
}

// ─── tokenBucket ──────────────────────────────────────────────────────────────

func TestTokenBucket_AllowsUnderLimit(t *testing.T) {
	b := newBucket(10, 10) // 10 rps, burst 10
	for i := 0; i < 10; i++ {
		if !b.Allow(1) {
			t.Fatalf("request %d should be allowed within burst", i+1)
		}
	}
}

func TestTokenBucket_RejectsOverBurst(t *testing.T) {
	b := newBucket(10, 3) // burst=3
	// drain the bucket
	b.Allow(1)
	b.Allow(1)
	b.Allow(1)
	if b.Allow(1) {
		t.Error("4th request beyond burst=3 should be rejected")
	}
}

func TestTokenBucket_RefillsOverTime(t *testing.T) {
	b := newBucket(1000, 1) // 1000 rps — refills quickly; burst=1
	b.Allow(1)              // drain
	// Manually advance lastTime by 10ms to simulate 10 tokens refilled at 1000 rps
	b.mu.Lock()
	b.lastTime = b.lastTime.Add(-10 * time.Millisecond)
	b.mu.Unlock()
	if !b.Allow(1) {
		t.Error("bucket should have refilled after time advance")
	}
}
