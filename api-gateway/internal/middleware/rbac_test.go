package middleware

// Unit tests for RequireRole (RBAC) and RequireRoleOrSelf (ABAC) middleware.
// Tests use chi route context to simulate URL path parameters without a full server.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	chi "github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
)

// ─── Helpers ──────────────────────────────────────────────────────────────────

func ok200(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// claimsCtx injects *Claims into a request context (simulates JWTAuth having run).
func claimsCtx(r *http.Request, claims *Claims) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), claimsKey, claims))
}

// chiCtx injects a chi route context with a {userId} URL param.
func chiCtx(r *http.Request, userID string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("userId", userID)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// claimsAndChiCtx combines both (typical middleware chain order).
func claimsAndChiCtx(r *http.Request, claims *Claims, userID string) *http.Request {
	r = claimsCtx(r, claims)
	return chiCtx(r, userID)
}

func makeAdminClaims() *Claims {
	return &Claims{Role: "admin", RegisteredClaims: jwt.RegisteredClaims{Subject: "user-admin"}}
}
func makeEditorClaims() *Claims {
	return &Claims{Role: "editor", RegisteredClaims: jwt.RegisteredClaims{Subject: "user-editor"}}
}
func makeViewerClaims() *Claims {
	return &Claims{Role: "viewer", RegisteredClaims: jwt.RegisteredClaims{Subject: "user-viewer"}}
}

// ─── RequireRole ──────────────────────────────────────────────────────────────

func TestRequireRole_AdminAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/users", nil)
	req = claimsCtx(req, makeAdminClaims())
	rr := httptest.NewRecorder()
	RequireRole("admin")(http.HandlerFunc(ok200)).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("admin should be allowed, got %d", rr.Code)
	}
}

func TestRequireRole_ViewerForbidden(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/users", nil)
	req = claimsCtx(req, makeViewerClaims())
	rr := httptest.NewRecorder()
	RequireRole("admin")(http.HandlerFunc(ok200)).ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("viewer should be forbidden, got %d", rr.Code)
	}
}

func TestRequireRole_EditorForbidden_WhenOnlyAdminRequired(t *testing.T) {
	req := httptest.NewRequest(http.MethodDelete, "/users/x", nil)
	req = claimsCtx(req, makeEditorClaims())
	rr := httptest.NewRecorder()
	RequireRole("admin")(http.HandlerFunc(ok200)).ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("editor should be forbidden for admin-only route, got %d", rr.Code)
	}
}

func TestRequireRole_NoClaims_Forbidden(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/users", nil)
	// No claims in context
	rr := httptest.NewRecorder()
	RequireRole("admin")(http.HandlerFunc(ok200)).ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("missing claims should be forbidden, got %d", rr.Code)
	}
}

func TestRequireRole_MultipleRolesAllowed_Admin(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	req = claimsCtx(req, makeAdminClaims())
	rr := httptest.NewRecorder()
	RequireRole("admin", "editor")(http.HandlerFunc(ok200)).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("admin should be allowed when admin|editor required, got %d", rr.Code)
	}
}

func TestRequireRole_MultipleRolesAllowed_Editor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	req = claimsCtx(req, makeEditorClaims())
	rr := httptest.NewRecorder()
	RequireRole("admin", "editor")(http.HandlerFunc(ok200)).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("editor should be allowed when admin|editor required, got %d", rr.Code)
	}
}

func TestRequireRole_CaseInsensitive(t *testing.T) {
	claims := &Claims{Role: "Admin"} // mixed case from some providers
	req := httptest.NewRequest(http.MethodPost, "/users", nil)
	req = claimsCtx(req, claims)
	rr := httptest.NewRecorder()
	RequireRole("admin")(http.HandlerFunc(ok200)).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("role check should be case-insensitive, got %d", rr.Code)
	}
}

// ─── RequireRoleOrSelf ────────────────────────────────────────────────────────

func TestRequireRoleOrSelf_AdminAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodPut, "/users/other-user", nil)
	req = claimsAndChiCtx(req, makeAdminClaims(), "other-user")
	rr := httptest.NewRecorder()
	RequireRoleOrSelf("admin")(http.HandlerFunc(ok200)).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("admin should be allowed to update any user, got %d", rr.Code)
	}
}

func TestRequireRoleOrSelf_SelfAllowed_AsViewer(t *testing.T) {
	// Viewer updating their own profile — allowed via ABAC self-service
	viewerClaims := &Claims{Role: "viewer", RegisteredClaims: jwt.RegisteredClaims{Subject: "user-viewer"}}
	req := httptest.NewRequest(http.MethodPut, "/users/user-viewer", nil)
	req = claimsAndChiCtx(req, viewerClaims, "user-viewer")
	rr := httptest.NewRecorder()
	RequireRoleOrSelf("admin")(http.HandlerFunc(ok200)).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("viewer updating own record should be allowed (ABAC self-service), got %d", rr.Code)
	}
}

func TestRequireRoleOrSelf_ViewerOtherUser_Forbidden(t *testing.T) {
	// Viewer trying to update someone else — blocked
	viewerClaims := &Claims{Role: "viewer", RegisteredClaims: jwt.RegisteredClaims{Subject: "user-viewer"}}
	req := httptest.NewRequest(http.MethodPut, "/users/another-user", nil)
	req = claimsAndChiCtx(req, viewerClaims, "another-user")
	rr := httptest.NewRecorder()
	RequireRoleOrSelf("admin")(http.HandlerFunc(ok200)).ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("viewer updating another user should be forbidden, got %d", rr.Code)
	}
}

func TestRequireRoleOrSelf_NoClaims_Forbidden(t *testing.T) {
	req := httptest.NewRequest(http.MethodPut, "/users/x", nil)
	req = chiCtx(req, "x") // chi context only, no claims
	rr := httptest.NewRecorder()
	RequireRoleOrSelf("admin")(http.HandlerFunc(ok200)).ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("missing claims should be forbidden, got %d", rr.Code)
	}
}

func TestRequireRoleOrSelf_NoUserIDParam_FallsBackToRoleCheck(t *testing.T) {
	// No {userId} in route — should fall through to role-based check
	req := httptest.NewRequest(http.MethodPut, "/users", nil)
	req = claimsCtx(req, makeAdminClaims())
	rr := httptest.NewRecorder()
	RequireRoleOrSelf("admin")(http.HandlerFunc(ok200)).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("admin should be allowed even without userId param, got %d", rr.Code)
	}
}

// ─── JWTAuth multi-secret ─────────────────────────────────────────────────────

func TestJWTAuth_AcceptsTokenSignedWithFirstSecret(t *testing.T) {
	secret := "secret-one"
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &Claims{
		Role:             "admin",
		RegisteredClaims: jwt.RegisteredClaims{Subject: "alice"},
	})
	signed, _ := token.SignedString([]byte(secret))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/traces", nil)
	req.Header.Set("Authorization", "Bearer "+signed)
	rr := httptest.NewRecorder()
	JWTAuth("secret-one", "secret-old")(http.HandlerFunc(ok200)).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("token signed with active secret should be accepted, got %d", rr.Code)
	}
}

func TestJWTAuth_AcceptsTokenSignedWithRotatedSecret(t *testing.T) {
	// Token signed with the OLD secret should still be accepted during rotation window
	oldSecret := "secret-old"
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &Claims{
		Role:             "viewer",
		RegisteredClaims: jwt.RegisteredClaims{Subject: "bob"},
	})
	signed, _ := token.SignedString([]byte(oldSecret))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/traces", nil)
	req.Header.Set("Authorization", "Bearer "+signed)
	rr := httptest.NewRecorder()
	JWTAuth("secret-new", "secret-old")(http.HandlerFunc(ok200)).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("token signed with rotated secret should be accepted, got %d", rr.Code)
	}
}

func TestJWTAuth_RejectsTokenWithUnknownSecret(t *testing.T) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &Claims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: "eve"},
	})
	signed, _ := token.SignedString([]byte("attacker-secret"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/traces", nil)
	req.Header.Set("Authorization", "Bearer "+signed)
	rr := httptest.NewRecorder()
	JWTAuth("secret-a", "secret-b")(http.HandlerFunc(ok200)).ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("token with unknown secret should be rejected, got %d", rr.Code)
	}
}
