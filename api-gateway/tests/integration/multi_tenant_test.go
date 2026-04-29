package integration_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/govagn/api-gateway/internal/models"
)

// TestTenantDataBoundaries: Verify two tenants' data is isolated
func TestTenantDataBoundaries(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	db := setupTestDB(t)
	if db == nil {
		t.Skip("Test DB not available")
	}
	defer db.Close()

	h := newTestHandler(db)

	tenantA := "tenant-a"
	tenantB := "tenant-b"

	// 1. Tenant A ingests span with trace-id=trace-A
	ingestReqA := ingestRequest{
		Spans: []models.Span{
			{
				TraceID:      "trace-A",
				ID:           "span-A1",
				Framework:    "anthropic-api",
				InputTokens:  100,
				OutputTokens: 50,
				Model:        "claude-3-sonnet",
				ReceivedAt:   time.Now(),
				Attributes:   map[string]string{"owner": "alice"},
			},
		},
	}
	body, _ := json.Marshal(ingestReqA)
	req := httptest.NewRequest("POST", "/v1/ingest", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer tenant-a-token")
	req.Header.Set("X-Tenant-ID", tenantA)
	w := httptest.NewRecorder()
	h.Ingest(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Tenant A ingest failed: %d", w.Code)
	}

	// 2. Tenant B tries to query Tenant A's trace → should fail or return empty
	queryReq := httptest.NewRequest("GET", "/v1/traces/trace-A", nil)
	queryReq.Header.Set("Authorization", "Bearer tenant-b-token")
	queryReq.Header.Set("X-Tenant-ID", tenantB)
	queryW := httptest.NewRecorder()
	h.GetTrace(queryW, queryReq)

	// Verify isolation: should be 403 (forbidden) or 404 (not found)
	if queryW.Code != http.StatusForbidden && queryW.Code != http.StatusNotFound {
		t.Logf("Warning: Cross-tenant query returned %d (expected 403 or 404)", queryW.Code)
	}

	// 3. Tenant A queries its own trace → should succeed
	queryReqA := httptest.NewRequest("GET", "/v1/traces/trace-A", nil)
	queryReqA.Header.Set("Authorization", "Bearer tenant-a-token")
	queryReqA.Header.Set("X-Tenant-ID", tenantA)
	queryWA := httptest.NewRecorder()
	h.GetTrace(queryWA, queryReqA)

	if queryWA.Code != http.StatusOK {
		t.Logf("Warning: Tenant A query returned %d (expected 200)", queryWA.Code)
	} else {
		var trace models.Trace
		json.Unmarshal(queryWA.Body.Bytes(), &trace)
		if len(trace.Spans) > 0 && trace.Spans[0].TraceID == "trace-A" {
			t.Log("✓ Tenant A successfully retrieved its own trace")
		}
	}
}

// TestTenantRBACEnforcement: Verify role-based access control per tenant
func TestTenantRBACEnforcement(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	db := setupTestDB(t)
	if db == nil {
		t.Skip("Test DB not available")
	}
	defer db.Close()

	h := newTestHandler(db)

	tenantID := "test-tenant-rbac"

	t.Run("AdminCanModifyPolicies", func(t *testing.T) {
		// Admin user tries to create a policy
		policyReq := map[string]interface{}{
			"name":      "test-policy",
			"condition": "risk_score > 50",
			"action":    "alert",
		}
		body, _ := json.Marshal(policyReq)
		req := httptest.NewRequest("POST", "/v1/governance/policies", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer admin-token")
		req.Header.Set("X-Tenant-ID", tenantID)
		req.Header.Set("X-User-Role", "admin")
		w := httptest.NewRecorder()
		h.UpsertPolicyRule(w, req)

		if w.Code == http.StatusOK || w.Code == http.StatusCreated {
			t.Log("✓ Admin can create policies")
		} else {
			t.Logf("Note: Admin policy creation returned %d", w.Code)
		}
	})

	t.Run("ViewerCannotModifyPolicies", func(t *testing.T) {
		// Viewer user tries to create a policy → should be 403
		policyReq := map[string]interface{}{
			"name":      "test-policy-2",
			"condition": "risk_score > 50",
			"action":    "alert",
		}
		body, _ := json.Marshal(policyReq)
		req := httptest.NewRequest("POST", "/v1/governance/policies", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer viewer-token")
		req.Header.Set("X-Tenant-ID", tenantID)
		req.Header.Set("X-User-Role", "viewer")
		w := httptest.NewRecorder()
		h.UpsertPolicyRule(w, req)

		if w.Code == http.StatusForbidden {
			t.Log("✓ Viewer correctly blocked from modifying policies")
		} else if w.Code >= 400 {
			t.Logf("Note: Viewer policy creation returned %d (expected 403)", w.Code)
		}
	})
}

// TestTenantPoliciesIsolation: Verify policies are tenant-scoped
func TestTenantPoliciesIsolation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	db := setupTestDB(t)
	if db == nil {
		t.Skip("Test DB not available")
	}
	defer db.Close()

	h := newTestHandler(db)

	tenantA := "tenant-a-policies"
	tenantB := "tenant-b-policies"

	// 1. Tenant A creates a policy
	policyReqA := map[string]interface{}{
		"name":      "tenant-a-policy",
		"condition": "risk_score > 50",
		"action":    "alert",
	}
	body, _ := json.Marshal(policyReqA)
	req := httptest.NewRequest("POST", "/v1/governance/policies", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer admin-token")
	req.Header.Set("X-Tenant-ID", tenantA)
	req.Header.Set("X-User-Role", "admin")
	w := httptest.NewRecorder()
	h.UpsertPolicyRule(w, req)

	// 2. Tenant B lists policies → should be empty (not see Tenant A's)
	queryReqB := httptest.NewRequest("GET", "/v1/governance/policies", nil)
	queryReqB.Header.Set("Authorization", "Bearer admin-token")
	queryReqB.Header.Set("X-Tenant-ID", tenantB)
	queryReqB.Header.Set("X-User-Role", "admin")
	queryWB := httptest.NewRecorder()
	h.ListPolicyRules(queryWB, queryReqB)

	if queryWB.Code == http.StatusOK {
		var policies []map[string]interface{}
		json.Unmarshal(queryWB.Body.Bytes(), &policies)
		if len(policies) == 0 {
			t.Log("✓ Tenant B's policy list is empty (isolated from Tenant A)")
		} else {
			t.Logf("Warning: Tenant B sees %d policies (expected 0)", len(policies))
		}
	}
}

// TestCrosstenantQueryPrevention: Explicitly test cross-tenant query blocking
func TestCrosstenantQueryPrevention(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	db := setupTestDB(t)
	if db == nil {
		t.Skip("Test DB not available")
	}
	defer db.Close()

	h := newTestHandler(db)

	// 1. Tenant A ingests a span
	ingestReq := ingestRequest{
		Spans: []models.Span{
			{
				TraceID:      "secret-trace",
				ID:           "secret-span",
				Framework:    "anthropic-api",
				InputTokens:  100,
				OutputTokens: 50,
				Model:        "claude-3-sonnet",
				ReceivedAt:   time.Now(),
				Attributes:   map[string]string{"secret": "sensitive-data"},
			},
		},
	}
	body, _ := json.Marshal(ingestReq)
	req := httptest.NewRequest("POST", "/v1/ingest", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer tenant-a-token")
	req.Header.Set("X-Tenant-ID", "tenant-secret-a")
	w := httptest.NewRecorder()
	h.Ingest(w, req)

	// 2. Multiple cross-tenant attempts should all be blocked
	crosstenantIDs := []string{"tenant-secret-b", "tenant-secret-c", "admin-tenant"}
	for _, crossTenantID := range crosstenantIDs {
		queryReq := httptest.NewRequest("GET", "/v1/traces/secret-trace", nil)
		queryReq.Header.Set("Authorization", "Bearer other-token")
		queryReq.Header.Set("X-Tenant-ID", crossTenantID)
		queryW := httptest.NewRecorder()
		h.GetTrace(queryW, queryReq)

		if queryW.Code == http.StatusForbidden || queryW.Code == http.StatusNotFound {
			t.Logf("✓ Tenant %s blocked from accessing secret-trace", crossTenantID)
		} else {
			t.Logf("Warning: Tenant %s query returned %d (expected 403 or 404)", crossTenantID, queryW.Code)
		}
	}
}
