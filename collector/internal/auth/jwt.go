package auth

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type JWTValidator struct {
	secret      []byte
	requireAuth bool
}

func NewJWTValidator(secret string, requireAuth bool) *JWTValidator {
	return &JWTValidator{
		secret:      []byte(secret),
		requireAuth: requireAuth,
	}
}

func (v *JWTValidator) ValidateToken(tokenStr string) error {
	if !v.requireAuth {
		return nil // auth disabled
	}
	if v.secret == nil || len(v.secret) == 0 {
		return jwt.ErrTokenMalformed
	}
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return v.secret, nil
	})
	if err != nil || !token.Valid {
		return jwt.ErrTokenInvalidClaims
	}
	return nil
}

func (v *JWTValidator) ValidateHTTP(r *http.Request) error {
	if !v.requireAuth {
		return nil
	}
	token := bearerToken(r.Header.Get("Authorization"))
	if token == "" {
		// Backward compatibility for clients that still send x-af-api-key.
		token = strings.TrimSpace(r.Header.Get("x-af-api-key"))
	}
	if token == "" {
		return jwt.ErrTokenNotValidYet
	}
	return v.ValidateToken(token)
}

// GRPCTokenValidator returns an interceptor that validates the Authorization header.
func GRPCTokenValidator(v *JWTValidator) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if !v.requireAuth {
			return handler(ctx, req)
		}
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing metadata")
		}
		auths := md.Get("authorization")
		token := ""
		if len(auths) > 0 {
			token = bearerToken(auths[0])
		}
		if token == "" {
			apiKeys := md.Get("x-af-api-key")
			if len(apiKeys) > 0 {
				token = strings.TrimSpace(apiKeys[0])
			}
		}
		if token == "" {
			return nil, status.Error(codes.Unauthenticated, "missing authorization header")
		}
		if err := v.ValidateToken(token); err != nil {
			return nil, status.Error(codes.Unauthenticated, "invalid token")
		}
		return handler(ctx, req)
	}
}

func bearerToken(header string) string {
	parts := strings.Fields(header)
	if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
		return strings.TrimSpace(parts[1])
	}
	return ""
}

// ─── Rate Limiter ────────────────────────────────────────────────────────────

type tokenBucket struct {
	mu       sync.Mutex
	tokens   float64
	max      float64
	rate     float64
	lastTime time.Time
}

func newBucket(ratePerSec, burst int) *tokenBucket {
	return &tokenBucket{
		tokens:   float64(burst),
		max:      float64(burst),
		rate:     float64(ratePerSec),
		lastTime: time.Now(),
	}
}

func (b *tokenBucket) Allow(n int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(b.lastTime).Seconds()
	b.tokens = min(b.max, b.tokens+elapsed*b.rate)
	b.lastTime = now
	if b.tokens >= float64(n) {
		b.tokens -= float64(n)
		return true
	}
	return false
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

var globalBucket *tokenBucket
var bucketOnce sync.Once

// GRPCRateLimiter returns an interceptor that rate-limits span ingestion.
func GRPCRateLimiter(spansPerSecond int) grpc.UnaryServerInterceptor {
	bucketOnce.Do(func() {
		globalBucket = newBucket(spansPerSecond, spansPerSecond*2)
	})
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if !globalBucket.Allow(1) {
			return nil, status.Error(codes.ResourceExhausted, "rate limit exceeded")
		}
		return handler(ctx, req)
	}
}
