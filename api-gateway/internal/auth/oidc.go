// api-gateway/internal/auth/oidc.go
// OIDC Authorization Code + PKCE flow for enterprise SSO (Okta, Azure AD, Auth0).
// Supports P0-4: portal login without manual JWT injection.
//
// Flow:
//   GET /auth/login     → redirect to OIDC provider with PKCE challenge
//   GET /auth/callback  → exchange code for tokens, issue AgentFabric JWT
//   GET /auth/logout    → clear session cookie, redirect to provider logout

package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/agentfabric/api-gateway/internal/models"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

// ─── UserLookup ──────────────────────────────────────────────────────────────

// UserLookup is the minimal store interface needed by PasswordLogin to verify
// user credentials against the database.
// Implemented by *store.PostgresStore; accepts nil for env-var-only auth (dev/test).
type UserLookup interface {
	GetUserByUsername(ctx context.Context, username, tenantID string) (*models.UserRecord, error)
}

// OIDCConfig holds OIDC provider configuration from environment.
type OIDCConfig struct {
	Issuer        string // e.g. https://login.microsoftonline.com/{tenantId}/v2.0
	ClientID      string
	ClientSecret  string
	RedirectURI   string // e.g. https://portal.agentfabric.io/auth/callback
	Scopes        []string
	JWTSecret     string   // primary secret (legacy / single-secret mode)
	JWTSecrets    []string // ordered list for zero-downtime rotation: [0]=active, rest=old
	SessionMaxAge time.Duration
	// Optional: provider-specific logout endpoint override
	LogoutURL string
	// Enterprise login controls.
	RequireSSO           bool
	DisablePasswordLogin bool
	// Local password login (non-OIDC). Defaults: admin / admin.
	// Override via AF_ADMIN_USER + AF_ADMIN_PASSWORD environment variables.
	AdminUser     string
	AdminPassword string
}

// pkceState holds the PKCE verifier + nonce between /login and /callback.
// In production this should be stored server-side (Redis) keyed by state param.
// For Sprint 1 we embed it as a signed cookie (simpler, stateless).
type pkceState struct {
	Verifier string
	Nonce    string
}

// jwkKey represents a single JSON Web Key (RFC 7517).
type jwkKey struct {
	Kty string `json:"kty"` // "RSA"
	Kid string `json:"kid"` // key ID — matched against JWT header
	Use string `json:"use"` // "sig"
	Alg string `json:"alg"` // "RS256"
	N   string `json:"n"`   // base64url RSA modulus
	E   string `json:"e"`   // base64url RSA public exponent
}

type jwksResponse struct {
	Keys []jwkKey `json:"keys"`
}

// jwksCache caches the provider's public keys to avoid fetching on every callback.
type jwksCache struct {
	mu        sync.RWMutex
	keys      map[string]*rsa.PublicKey // kid → public key
	fetchedAt time.Time
	ttl       time.Duration
}

func newJWKSCache() *jwksCache {
	return &jwksCache{
		keys: make(map[string]*rsa.PublicKey),
		ttl:  15 * time.Minute,
	}
}

// get returns cached keys, or nil if the cache is expired.
func (c *jwksCache) get() map[string]*rsa.PublicKey {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if time.Since(c.fetchedAt) > c.ttl {
		return nil
	}
	return c.keys
}

// set atomically replaces the cached key set.
func (c *jwksCache) set(keys map[string]*rsa.PublicKey) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.keys = keys
	c.fetchedAt = time.Now()
}

// parseJWKSPublicKey converts a JWK into an *rsa.PublicKey using only stdlib.
func parseJWKSPublicKey(k jwkKey) (*rsa.PublicKey, error) {
	if k.Kty != "RSA" {
		return nil, fmt.Errorf("unsupported key type: %s", k.Kty)
	}
	nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		return nil, fmt.Errorf("decode modulus: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
	if err != nil {
		return nil, fmt.Errorf("decode exponent: %w", err)
	}
	n := new(big.Int).SetBytes(nBytes)
	e := int(new(big.Int).SetBytes(eBytes).Int64())
	if e == 0 {
		return nil, fmt.Errorf("invalid RSA exponent")
	}
	return &rsa.PublicKey{N: n, E: e}, nil
}

// OIDCHandler implements the login/callback/logout HTTP handlers.
type OIDCHandler struct {
	cfg       OIDCConfig
	users     UserLookup // nil means env-var-only auth (dev / break-glass)
	logger    *zap.Logger
	jwksCache *jwksCache
}

// NewOIDCHandler creates a new OIDC handler.
// users may be nil; when nil, PasswordLogin falls back to the env-var admin
// credential only (useful in tests and dev environments with no DB).
func NewOIDCHandler(cfg OIDCConfig, users UserLookup, logger *zap.Logger) *OIDCHandler {
	if len(cfg.Scopes) == 0 {
		cfg.Scopes = []string{"openid", "email", "profile"}
	}
	if cfg.SessionMaxAge == 0 {
		cfg.SessionMaxAge = 8 * time.Hour
	}
	// Normalise secret list: JWTSecrets overrides JWTSecret when present.
	if len(cfg.JWTSecrets) == 0 && cfg.JWTSecret != "" {
		cfg.JWTSecrets = []string{cfg.JWTSecret}
	}
	if cfg.JWTSecret == "" && len(cfg.JWTSecrets) > 0 {
		cfg.JWTSecret = cfg.JWTSecrets[0]
	}
	return &OIDCHandler{cfg: cfg, users: users, logger: logger, jwksCache: newJWKSCache()}
}

func ValidateConfig(cfg OIDCConfig, strict bool) error {
	if !strict {
		return nil
	}
	if cfg.RequireSSO {
		switch {
		case strings.TrimSpace(cfg.Issuer) == "":
			return fmt.Errorf("AF_SSO_REQUIRED=true requires AF_OIDC_ISSUER")
		case strings.TrimSpace(cfg.ClientID) == "":
			return fmt.Errorf("AF_SSO_REQUIRED=true requires AF_OIDC_CLIENT_ID")
		case strings.TrimSpace(cfg.ClientSecret) == "":
			return fmt.Errorf("AF_SSO_REQUIRED=true requires AF_OIDC_CLIENT_SECRET")
		case strings.TrimSpace(cfg.RedirectURI) == "":
			return fmt.Errorf("AF_SSO_REQUIRED=true requires AF_OIDC_REDIRECT_URI")
		}
	}
	if cfg.DisablePasswordLogin && !cfg.RequireSSO && strings.TrimSpace(cfg.Issuer) == "" {
		return fmt.Errorf("AF_PASSWORD_LOGIN_DISABLED=true requires OIDC SSO to be configured")
	}
	return nil
}

func (h *OIDCHandler) SSOEnabled() bool {
	return strings.TrimSpace(h.cfg.Issuer) != ""
}

func (h *OIDCHandler) PasswordLoginEnabled() bool {
	return !h.cfg.DisablePasswordLogin
}

// ─── Multi-secret helpers (zero-downtime key rotation) ────────────────────────

// activeSecret returns the current signing secret (first in the rotation list).
func (h *OIDCHandler) activeSecret() string {
	if len(h.cfg.JWTSecrets) > 0 && h.cfg.JWTSecrets[0] != "" {
		return h.cfg.JWTSecrets[0]
	}
	return h.cfg.JWTSecret
}

// allSecrets returns all accepted secrets for verification (current + old rotated keys).
func (h *OIDCHandler) allSecrets() []string {
	if len(h.cfg.JWTSecrets) > 0 {
		return h.cfg.JWTSecrets
	}
	return []string{h.cfg.JWTSecret}
}

// parseAFToken validates an AF JWT against all known secrets and returns its claims.
// Accepts tokens signed with any secret in the rotation list (current or previous).
func (h *OIDCHandler) parseAFToken(tokenStr string) (*afClaims, error) {
	secrets := h.allSecrets()
	var lastErr error
	for _, secret := range secrets {
		claims := &afClaims{}
		_, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return []byte(secret), nil
		})
		if err == nil {
			return claims, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// ─── GET /auth/login ─────────────────────────────────────────────────────────
//
// Generates PKCE code_verifier + code_challenge, saves state in a signed cookie,
// then redirects the browser to the OIDC provider's authorization endpoint.

func (h *OIDCHandler) Login(w http.ResponseWriter, r *http.Request) {
	verifier, challenge, err := generatePKCE()
	if err != nil {
		h.logger.Error("pkce generation failed", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	nonce, err := generateNonce()
	if err != nil {
		h.logger.Error("nonce generation failed", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	state, err := generateNonce() // opaque state parameter
	if err != nil {
		h.logger.Error("state generation failed", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Persist PKCE verifier + nonce in a signed, HttpOnly cookie so we can
	// verify them in /callback without server-side session storage.
	stateCookie, err := h.signStateCookie(pkceState{Verifier: verifier, Nonce: nonce})
	if err != nil {
		h.logger.Error("state cookie signing failed", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "af_oidc_state",
		Value:    stateCookie,
		Path:     "/auth",
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600, // 10 minutes to complete login
	})

	// Build authorization URL
	authURL, err := h.buildAuthURL(state, challenge, nonce)
	if err != nil {
		h.logger.Error("auth URL build failed", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	h.logger.Info("OIDC login initiated", zap.String("state", state[:8]+"..."))
	http.Redirect(w, r, authURL, http.StatusFound)
}

// ─── GET /auth/callback ──────────────────────────────────────────────────────
//
// Receives the authorization code from the OIDC provider, validates the state,
// exchanges the code for tokens using PKCE, validates the ID token,
// then issues an AgentFabric JWT and redirects the browser to the portal.

func (h *OIDCHandler) Callback(w http.ResponseWriter, r *http.Request) {
	// Validate state anti-CSRF parameter
	q := r.URL.Query()

	if errParam := q.Get("error"); errParam != "" {
		h.logger.Warn("OIDC provider returned error",
			zap.String("error", errParam),
			zap.String("description", q.Get("error_description")))
		http.Error(w, fmt.Sprintf("login failed: %s", errParam), http.StatusBadRequest)
		return
	}

	code := q.Get("code")
	if code == "" {
		http.Error(w, "missing authorization code", http.StatusBadRequest)
		return
	}

	// Retrieve and verify PKCE state cookie
	cookie, err := r.Cookie("af_oidc_state")
	if err != nil {
		http.Error(w, "missing state cookie — possible CSRF", http.StatusBadRequest)
		return
	}

	state, err := h.verifyStateCookie(cookie.Value)
	if err != nil {
		h.logger.Warn("OIDC state cookie verification failed", zap.Error(err))
		http.Error(w, "invalid state — possible CSRF or expired session", http.StatusBadRequest)
		return
	}

	// Clear the state cookie
	http.SetCookie(w, &http.Cookie{
		Name:    "af_oidc_state",
		Value:   "",
		Path:    "/auth",
		MaxAge:  -1,
		Expires: time.Unix(0, 0),
	})

	// Exchange authorization code for tokens
	tokenResp, err := h.exchangeCode(r.Context(), code, state.Verifier)
	if err != nil {
		h.logger.Error("token exchange failed", zap.Error(err))
		http.Error(w, "token exchange failed", http.StatusInternalServerError)
		return
	}

	// Parse and validate ID token — RS256 signature verified via JWKS (Sprint 2)
	claims, err := h.verifyIDToken(tokenResp.IDToken)
	if err != nil {
		h.logger.Error("ID token parse failed", zap.Error(err))
		http.Error(w, "invalid ID token", http.StatusUnauthorized)
		return
	}

	// Verify nonce anti-replay
	if claims.Nonce != state.Nonce {
		h.logger.Warn("OIDC nonce mismatch — possible replay attack")
		http.Error(w, "nonce mismatch", http.StatusUnauthorized)
		return
	}

	// Issue AgentFabric JWT
	afJWT, err := h.issueAFToken(claims)
	if err != nil {
		h.logger.Error("AF token issuance failed", zap.Error(err))
		http.Error(w, "token issuance failed", http.StatusInternalServerError)
		return
	}

	h.logger.Info("OIDC login successful",
		zap.String("subject", claims.Subject),
		zap.String("email", claims.Email))

	// Set JWT in an HttpOnly + SameSite=Strict cookie.
	// HttpOnly: true  — JS cannot read the token; eliminates XSS token theft.
	// SameSite=Strict — cookie is not sent on cross-site requests, blocking CSRF.
	// The portal authenticates via GET /auth/me with credentials:'include';
	// it never reads or stores the raw JWT.
	http.SetCookie(w, &http.Cookie{
		Name:     "af_token",
		Value:    afJWT,
		Path:     "/",
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(h.cfg.SessionMaxAge.Seconds()),
	})

	// Redirect portal to dashboard
	http.Redirect(w, r, "/#/dashboard", http.StatusFound)
}

// ─── GET /auth/logout ────────────────────────────────────────────────────────

func (h *OIDCHandler) Logout(w http.ResponseWriter, r *http.Request) {
	// Clear the session cookie
	http.SetCookie(w, &http.Cookie{
		Name:    "af_token",
		Value:   "",
		Path:    "/",
		MaxAge:  -1,
		Expires: time.Unix(0, 0),
	})

	// Redirect to OIDC provider logout if configured
	if h.cfg.LogoutURL != "" {
		http.Redirect(w, r, h.cfg.LogoutURL, http.StatusFound)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

// ─── GET /auth/me ─────────────────────────────────────────────────────────────

// Me returns the current user's identity.
// Accepts the JWT from the Authorization: Bearer header (API/CLI) or the
// af_token HttpOnly cookie (browser). Returns the exp Unix timestamp so the
// portal can schedule a silent token refresh without reading the token itself.
func (h *OIDCHandler) Me(w http.ResponseWriter, r *http.Request) {
	tokenStr := bearerToken(r)
	if tokenStr == "" {
		if c, err := r.Cookie("af_token"); err == nil && c.Value != "" {
			tokenStr = c.Value
		}
	}
	if tokenStr == "" {
		http.Error(w, "no token", http.StatusUnauthorized)
		return
	}

	claims, err := h.parseAFToken(tokenStr)
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	exp := int64(0)
	if claims.ExpiresAt != nil {
		exp = claims.ExpiresAt.Unix()
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"sub":   claims.Subject,
		"email": claims.Email,
		"name":  claims.Name,
		"role":  claims.Role,
		"exp":   exp, // Unix timestamp; portal uses this to schedule silent refresh
	})
}

// ─── PKCE helpers ─────────────────────────────────────────────────────────────

// generatePKCE returns (verifier, S256_challenge, error)
func generatePKCE() (string, string, error) {
	verifierBytes := make([]byte, 32)
	if _, err := rand.Read(verifierBytes); err != nil {
		return "", "", err
	}
	verifier := base64.RawURLEncoding.EncodeToString(verifierBytes)
	hash := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(hash[:])
	return verifier, challenge, nil
}

func generateNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// ─── Authorization URL ────────────────────────────────────────────────────────

func (h *OIDCHandler) buildAuthURL(state, challenge, nonce string) (string, error) {
	// Discover the authorization endpoint from the issuer's well-known config
	authEndpoint, err := h.discoverAuthEndpoint()
	if err != nil {
		return "", err
	}

	params := url.Values{
		"response_type":         {"code"},
		"client_id":             {h.cfg.ClientID},
		"redirect_uri":          {h.cfg.RedirectURI},
		"scope":                 {strings.Join(h.cfg.Scopes, " ")},
		"state":                 {state},
		"nonce":                 {nonce},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}

	return authEndpoint + "?" + params.Encode(), nil
}

// ─── Token Exchange ───────────────────────────────────────────────────────────

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	IDToken      string `json:"id_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

func (h *OIDCHandler) exchangeCode(ctx interface{ Done() <-chan struct{} }, code, verifier string) (*tokenResponse, error) {
	tokenEndpoint, err := h.discoverTokenEndpoint()
	if err != nil {
		return nil, err
	}

	body := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {h.cfg.RedirectURI},
		"client_id":     {h.cfg.ClientID},
		"client_secret": {h.cfg.ClientSecret},
		"code_verifier": {verifier},
	}

	resp, err := http.PostForm(tokenEndpoint, body)
	if err != nil {
		return nil, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, data)
	}

	var tr tokenResponse
	if err := json.Unmarshal(data, &tr); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}

	return &tr, nil
}

// ─── ID Token parsing ─────────────────────────────────────────────────────────

type idTokenClaims struct {
	Subject string `json:"sub"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	Nonce   string `json:"nonce"`
	Iss     string `json:"iss"`
	Aud     string `json:"aud"`
	Exp     int64  `json:"exp"`
	// Role is set locally (not from OIDC provider) to carry the platform role.
	Role string `json:"role,omitempty"`
}

// ─── JWKS fetching ────────────────────────────────────────────────────────────

// fetchJWKS retrieves public keys from the OIDC provider's JWKS endpoint.
// Results are cached in h.jwksCache for 15 minutes.
func (h *OIDCHandler) fetchJWKS() (map[string]*rsa.PublicKey, error) {
	// Return cached keys if still fresh
	if cached := h.jwksCache.get(); cached != nil {
		return cached, nil
	}

	d, err := h.discover()
	if err != nil {
		return nil, fmt.Errorf("discover JWKS URI: %w", err)
	}
	if d.JwksURI == "" {
		return nil, fmt.Errorf("OIDC discovery missing jwks_uri")
	}

	resp, err := http.Get(d.JwksURI) //nolint:gosec — URL from operator-configured issuer
	if err != nil {
		return nil, fmt.Errorf("fetch JWKS: %w", err)
	}
	defer resp.Body.Close()

	var jwks jwksResponse
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return nil, fmt.Errorf("parse JWKS: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey, len(jwks.Keys))
	for _, k := range jwks.Keys {
		pub, err := parseJWKSPublicKey(k)
		if err != nil {
			h.logger.Warn("skipping malformed JWK", zap.String("kid", k.Kid), zap.Error(err))
			continue
		}
		keys[k.Kid] = pub
	}

	h.jwksCache.set(keys)
	return keys, nil
}

// verifyIDToken validates an ID token's RS256 signature using the provider's JWKS.
// Falls back to signature-less claim parsing when no OIDC issuer is configured (dev/test).
func (h *OIDCHandler) verifyIDToken(idToken string) (*idTokenClaims, error) {
	// Dev mode: no issuer configured — skip signature verification
	if h.cfg.Issuer == "" {
		return parseIDTokenUnsafe(idToken)
	}

	keys, err := h.fetchJWKS()
	if err != nil {
		return nil, fmt.Errorf("JWKS fetch failed: %w", err)
	}

	// Parse and verify the JWT signature
	var claims idTokenClaims
	_, err = jwt.NewParser().ParseWithClaims(idToken, &jwtIDClaims{idTokenClaims: &claims},
		func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
				return nil, fmt.Errorf("expected RS256, got %v", t.Header["alg"])
			}
			kid, _ := t.Header["kid"].(string)
			if kid == "" {
				// No kid — try each key (some providers omit kid)
				for _, pub := range keys {
					return pub, nil
				}
				return nil, fmt.Errorf("no kid in token and no keys available")
			}
			pub, ok := keys[kid]
			if !ok {
				// kid not in cache — may be a key rotation; force refresh once
				h.jwksCache.set(nil) // invalidate
				refreshed, refreshErr := h.fetchJWKS()
				if refreshErr != nil {
					return nil, fmt.Errorf("JWKS refresh failed: %w", refreshErr)
				}
				pub, ok = refreshed[kid]
				if !ok {
					return nil, fmt.Errorf("unknown kid: %s", kid)
				}
			}
			return pub, nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("ID token verification failed: %w", err)
	}

	if claims.Subject == "" {
		return nil, fmt.Errorf("ID token missing 'sub' claim")
	}

	return &claims, nil
}

// jwtIDClaims wraps idTokenClaims to satisfy jwt.Claims interface.
type jwtIDClaims struct {
	*idTokenClaims
}

func (c *jwtIDClaims) GetExpirationTime() (*jwt.NumericDate, error) {
	if c.Exp == 0 {
		return nil, nil
	}
	return jwt.NewNumericDate(time.Unix(c.Exp, 0)), nil
}
func (c *jwtIDClaims) GetIssuedAt() (*jwt.NumericDate, error)  { return nil, nil }
func (c *jwtIDClaims) GetNotBefore() (*jwt.NumericDate, error) { return nil, nil }
func (c *jwtIDClaims) GetIssuer() (string, error)              { return c.Iss, nil }
func (c *jwtIDClaims) GetSubject() (string, error)             { return c.Subject, nil }
func (c *jwtIDClaims) GetAudience() (jwt.ClaimStrings, error)  { return jwt.ClaimStrings{c.Aud}, nil }

// parseIDTokenUnsafe parses ID token claims WITHOUT signature verification.
// Used only when no OIDC issuer is configured (local dev / integration tests).
func parseIDTokenUnsafe(idToken string) (*idTokenClaims, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT format: expected 3 parts, got %d", len(parts))
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode JWT payload: %w", err)
	}

	var claims idTokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("parse JWT claims: %w", err)
	}

	if claims.Exp > 0 && time.Now().Unix() > claims.Exp {
		return nil, fmt.Errorf("ID token expired at %d", claims.Exp)
	}
	if claims.Subject == "" {
		return nil, fmt.Errorf("ID token missing 'sub' claim")
	}

	return &claims, nil
}

// ─── AgentFabric JWT issuance ─────────────────────────────────────────────────

type afClaims struct {
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role"` // admin|editor|viewer — propagated into every AF JWT
	jwt.RegisteredClaims
}

func (h *OIDCHandler) issueAFToken(idClaims *idTokenClaims) (string, error) {
	role := idClaims.Role
	if role == "" {
		role = "viewer" // safe default for OIDC logins without an explicit role claim
	}
	claims := afClaims{
		Email: idClaims.Email,
		Name:  idClaims.Name,
		Role:  role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   idClaims.Subject,
			Issuer:    "agentfabric",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(h.cfg.SessionMaxAge)),
			Audience:  jwt.ClaimStrings{"agentfabric-portal"},
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(h.activeSecret()))
}

// ─── State cookie signing ────────────────────────────────────────────────────

// signStateCookie encodes the PKCE state as a signed JWT cookie.
func (h *OIDCHandler) signStateCookie(state pkceState) (string, error) {
	type stateClaims struct {
		Verifier string `json:"v"`
		Nonce    string `json:"n"`
		jwt.RegisteredClaims
	}
	claims := stateClaims{
		Verifier: state.Verifier,
		Nonce:    state.Nonce,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(10 * time.Minute)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(h.activeSecret()))
}

// verifyStateCookie decodes and validates a signed state cookie.
func (h *OIDCHandler) verifyStateCookie(value string) (*pkceState, error) {
	type stateClaims struct {
		Verifier string `json:"v"`
		Nonce    string `json:"n"`
		jwt.RegisteredClaims
	}
	claims := &stateClaims{}
	_, err := jwt.ParseWithClaims(value, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return []byte(h.activeSecret()), nil
	})
	if err != nil {
		return nil, fmt.Errorf("invalid state cookie: %w", err)
	}
	return &pkceState{Verifier: claims.Verifier, Nonce: claims.Nonce}, nil
}

// ─── OIDC Discovery ──────────────────────────────────────────────────────────

type oidcDiscovery struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	EndSessionEndpoint    string `json:"end_session_endpoint"`
	JwksURI               string `json:"jwks_uri"`
}

func (h *OIDCHandler) discover() (*oidcDiscovery, error) {
	wellKnown := strings.TrimRight(h.cfg.Issuer, "/") + "/.well-known/openid-configuration"
	resp, err := http.Get(wellKnown) //nolint:gosec — URL is operator-configured, not user-supplied
	if err != nil {
		return nil, fmt.Errorf("discovery fetch: %w", err)
	}
	defer resp.Body.Close()
	var d oidcDiscovery
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return nil, fmt.Errorf("discovery parse: %w", err)
	}
	return &d, nil
}

func (h *OIDCHandler) discoverAuthEndpoint() (string, error) {
	d, err := h.discover()
	if err != nil {
		return "", err
	}
	return d.AuthorizationEndpoint, nil
}

func (h *OIDCHandler) discoverTokenEndpoint() (string, error) {
	d, err := h.discover()
	if err != nil {
		return "", err
	}
	return d.TokenEndpoint, nil
}

// ─── Utilities ────────────────────────────────────────────────────────────────

func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}

func bearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// ─── POST /auth/refresh ──────────────────────────────────────────────────────
//
// Exchange a valid AF JWT for a fresh one with a renewed expiry window.
// No additional credentials required — the existing token IS the proof of identity.
//
// Request:  Authorization: Bearer <existing_token>
// Response: {"token":"<new_jwt>","expires_in":28800}

// Refresh exchanges a valid AF JWT for a fresh one with a renewed expiry.
// Accepts the token from Authorization: Bearer (CLI/API) or the af_token
// HttpOnly cookie (browser). On success it rotates the HttpOnly cookie so the
// browser session is extended without JS ever touching the raw token.
//
// Response: {"exp": <unix_timestamp>} — the new token's expiry, so the portal
// can schedule the next silent refresh. The token itself is NOT returned in the
// body; browser clients receive it exclusively via the Set-Cookie header.
// CLI/API clients may inspect the Set-Cookie header or call /auth/login to get
// a fresh Bearer token.
func (h *OIDCHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	// Accept Bearer header (CLI/API) or HttpOnly cookie (browser)
	tokenStr := bearerToken(r)
	if tokenStr == "" {
		if c, err := r.Cookie("af_token"); err == nil && c.Value != "" {
			tokenStr = c.Value
		}
	}
	if tokenStr == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing token"})
		return
	}

	// parseAFToken tries all rotation keys — accepts tokens issued before a key rotation.
	claims, err := h.parseAFToken(tokenStr)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired token"})
		return
	}

	// Re-issue with the same identity + role but a fresh expiry, signed with the active key.
	freshClaims := &idTokenClaims{
		Subject: claims.Subject,
		Email:   claims.Email,
		Name:    claims.Name,
		Role:    claims.Role, // preserve role across refresh
	}
	newToken, err := h.issueAFToken(freshClaims)
	if err != nil {
		h.logger.Error("token refresh: issuance failed", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	// Rotate the HttpOnly cookie — browser receives new token without JS reading it.
	http.SetCookie(w, &http.Cookie{
		Name:     "af_token",
		Value:    newToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(h.cfg.SessionMaxAge.Seconds()),
	})

	newExp := time.Now().Add(h.cfg.SessionMaxAge).Unix()
	h.logger.Info("token refreshed", zap.String("subject", claims.Subject), zap.String("role", claims.Role))
	writeJSON(w, http.StatusOK, map[string]interface{}{
		// exp: new token's expiry as Unix timestamp (portal uses this to re-arm the
		// silent-refresh timer without reading the token value itself).
		"exp":        newExp,
		"expires_in": int(h.cfg.SessionMaxAge.Seconds()),
	})
}

// ─── POST /auth/login ─────────────────────────────────────────────────────────
//
// Username + password login for environments without an OIDC provider.
// Default credentials: admin / admin
// Override via AF_ADMIN_USER + AF_ADMIN_PASSWORD environment variables.
//
// On success: returns {"token":"<JWT>"} with Content-Type: application/json
// The portal stores the token in localStorage and includes it as Bearer on all API calls.

type passwordLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// defaultTenantID is the canonical fallback tenant for password login when no
// tenant claim is present. Must match the seed row in migration 001.
const defaultTenantID = "00000000-0000-0000-0000-000000000001"

func (h *OIDCHandler) PasswordLogin(w http.ResponseWriter, r *http.Request) {
	if h.cfg.DisablePasswordLogin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "password login disabled"})
		return
	}
	var req passwordLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	req.Password = strings.TrimSpace(req.Password)

	if req.Username == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username and password are required"})
		return
	}

	var idClaims *idTokenClaims

	// ── Primary path: look up user in the database and bcrypt-compare ──────
	if h.users != nil {
		rec, err := h.users.GetUserByUsername(r.Context(), req.Username, defaultTenantID)
		if err == nil && rec.PasswordHash != "" {
			// bcrypt.CompareHashAndPassword is constant-time; no timing leak.
			if bcryptErr := bcrypt.CompareHashAndPassword([]byte(rec.PasswordHash), []byte(req.Password)); bcryptErr == nil {
				idClaims = &idTokenClaims{
					Subject: rec.ID,
					Email:   rec.Email,
					Name:    rec.DisplayName,
					Role:    rec.Role,
				}
				h.logger.Info("password login: db auth successful", zap.String("username", req.Username))
			}
		}
	}

	// ── Break-glass fallback: env-var admin credential ──────────────────────
	// Activated when: DB lookup is unavailable (nil users), user not found,
	// password hash is empty (seed row), or bcrypt comparison fails but
	// the env-var credential matches.
	if idClaims == nil {
		wantUser := h.cfg.AdminUser
		if wantUser == "" {
			wantUser = "admin"
		}
		wantPass := h.cfg.AdminPassword
		if wantPass == "" {
			wantPass = "admin"
		}
		// Constant-time compare both fields; both always run to prevent
		// timing-based username enumeration.
		userOK := subtle.ConstantTimeCompare([]byte(req.Username), []byte(wantUser)) == 1
		passOK := subtle.ConstantTimeCompare([]byte(req.Password), []byte(wantPass)) == 1
		if userOK && passOK {
			idClaims = &idTokenClaims{
				Subject: wantUser,
				Email:   wantUser + "@agentfabric.local",
				Name:    "Admin",
				Role:    "admin",
			}
			h.logger.Info("password login: break-glass env-var auth successful", zap.String("username", wantUser))
		}
	}

	if idClaims == nil {
		h.logger.Warn("password login: invalid credentials", zap.String("username", req.Username))
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid credentials"})
		return
	}

	token, err := h.issueAFToken(idClaims)
	if err != nil {
		h.logger.Error("password login: token issuance failed", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Set JWT in an HttpOnly + SameSite=Strict cookie.
	// Browser clients (portal) never read the token — they use GET /auth/me
	// with credentials:'include' for identity, and the cookie is sent automatically
	// on all same-origin API requests by the browser.
	http.SetCookie(w, &http.Cookie{
		Name:     "af_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(h.cfg.SessionMaxAge.Seconds()),
	})

	// Return the token in the response body for CLI / API clients that use
	// Authorization: Bearer. Browser clients should ignore this field.
	h.logger.Info("password login successful", zap.String("username", idClaims.Subject))
	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}
