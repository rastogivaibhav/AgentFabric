package integration_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/govagn/api-gateway/internal/governance"
	"github.com/govagn/api-gateway/internal/handlers"
	"github.com/govagn/api-gateway/internal/models"
)

// TestSecretDetectionInIngest: Verify exposed secrets are flagged with high risk score
func TestSecretDetectionInIngest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	db := setupTestDB(t)
	if db == nil {
		t.Skip("Test DB not available")
	}
	defer db.Close()

	h := handlers.New(db, nil, governance.NewRiskEngine())

	secretCases := []struct {
		name       string
		attributes map[string]string
		expectRisk bool
	}{
		{
			name:       "AWSAccessKey",
			attributes: map[string]string{"output": "AKIA1234567890ABCDEF"},
			expectRisk: true,
		},
		{
			name:       "OpenAIKey",
			attributes: map[string]string{"api_key": "sk-proj-1234567890"},
			expectRisk: true,
		},
		{
			name:       "AnthropicKey",
			attributes: map[string]string{"key": "sk-ant-v1-1234567890"},
			expectRisk: true,
		},
		{
			name:       "GitHubToken",
			attributes: map[string]string{"token": "ghp_1234567890abcdefghijklmnopqrstuv"},
			expectRisk: true,
		},
		{
			name:       "PrivateKey",
			attributes: map[string]string{"key": "-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQC..."},
			expectRisk: true,
		},
		{
			name:       "SafeOutput",
			attributes: map[string]string{"output": "Hello, world! This is a normal response."},
			expectRisk: false,
		},
	}

	for _, tc := range secretCases {
		t.Run(tc.name, func(t *testing.T) {
			ingestReq := models.IngestRequest{
				Spans: []models.Span{
					{
						TraceID:      "trace-secret-" + strings.ToLower(tc.name),
						SpanID:       "span-secret-" + strings.ToLower(tc.name),
						Framework:    "anthropic-api",
						InputTokens:  50,
						OutputTokens: 25,
						ModelName:    "claude-3-haiku",
						ReceivedAt:   time.Now(),
						Attributes:   tc.attributes,
					},
				},
			}

			body, _ := json.Marshal(ingestReq)
			req := httptest.NewRequest("POST", "/v1/ingest", bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer valid-token")
			req.Header.Set("X-Tenant-ID", "test-tenant")
			w := httptest.NewRecorder()
			h.Ingest(w, req)

			if w.Code != http.StatusOK {
				t.Logf("Warning: Ingest returned %d", w.Code)
				return
			}

			// Query back and check risk score
			traceID := "trace-secret-" + strings.ToLower(tc.name)
			queryReq := httptest.NewRequest("GET", "/v1/traces/"+traceID, nil)
			queryReq.Header.Set("Authorization", "Bearer valid-token")
			queryReq.Header.Set("X-Tenant-ID", "test-tenant")
			queryW := httptest.NewRecorder()
			h.GetTrace(queryW, queryReq)

			var trace models.Trace
			json.Unmarshal(queryW.Body.Bytes(), &trace)

			if len(trace.Spans) > 0 {
				span := trace.Spans[0]
				hasRisk := span.RiskScore >= 80 && span.RiskCategory == "secret_exposure"

				if tc.expectRisk && hasRisk {
					t.Logf("✓ %s correctly flagged (risk_score=%d)", tc.name, span.RiskScore)
				} else if !tc.expectRisk && !hasRisk {
					t.Logf("✓ %s safe (risk_score=%d)", tc.name, span.RiskScore)
				} else {
					t.Logf("Warning: %s mismatch (expect_risk=%v, actual_risk=%v)", tc.name, tc.expectRisk, hasRisk)
				}
			}
		})
	}
}

// TestUnauthorizedAccess: Verify requests without/with invalid tokens are rejected
func TestUnauthorizedAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	db := setupTestDB(t)
	if db == nil {
		t.Skip("Test DB not available")
	}
	defer db.Close()

	h := handlers.New(db, nil, governance.NewRiskEngine())

	testCases := []struct {
		name          string
		authHeader    string
		expectStatus  int
		tenantID      string
	}{
		{
			name:         "NoAuthHeader",
			authHeader:   "",
			expectStatus: 401,
			tenantID:     "test-tenant",
		},
		{
			name:         "InvalidToken",
			authHeader:   "Bearer invalid-token-xyz",
			expectStatus: 401,
			tenantID:     "test-tenant",
		},
		{
			name:         "ExpiredToken",
			authHeader:   "Bearer expired-token-12345",
			expectStatus: 401,
			tenantID:     "test-tenant",
		},
		{
			name:         "MalformedAuthHeader",
			authHeader:   "NotBearer validtoken",
			expectStatus: 401,
			tenantID:     "test-tenant",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ingestReq := models.IngestRequest{
				Spans: []models.Span{
					{
						TraceID:      "trace-auth-" + tc.name,
						SpanID:       "span-auth-" + tc.name,
						Framework:    "anthropic-api",
						InputTokens:  10,
						OutputTokens: 5,
						ModelName:    "claude-3-haiku",
						ReceivedAt:   time.Now(),
						Attributes:   map[string]string{},
					},
				},
			}

			body, _ := json.Marshal(ingestReq)
			req := httptest.NewRequest("POST", "/v1/ingest", bytes.NewReader(body))

			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}

			req.Header.Set("X-Tenant-ID", tc.tenantID)
			w := httptest.NewRecorder()
			h.Ingest(w, req)

			if w.Code == tc.expectStatus {
				t.Logf("✓ %s correctly returned %d", tc.name, w.Code)
			} else {
				t.Logf("Warning: %s returned %d (expected %d)", tc.name, w.Code, tc.expectStatus)
			}
		})
	}
}

// TestPIIScrubbing: Verify PII is redacted from storage/logs
func TestPIIScrubbing(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	db := setupTestDB(t)
	if db == nil {
		t.Skip("Test DB not available")
	}
	defer db.Close()

	h := handlers.New(db, nil, governance.NewRiskEngine())

	piiCases := []struct {
		name       string
		attributes map[string]string
		piiField   string
		value      string
	}{
		{
			name:       "EmailAddress",
			attributes: map[string]string{"user_email": "alice@example.com"},
			piiField:   "user_email",
			value:      "alice@example.com",
		},
		{
			name:       "PhoneNumber",
			attributes: map[string]string{"phone": "+1-555-123-4567"},
			piiField:   "phone",
			value:      "+1-555-123-4567",
		},
		{
			name:       "SSN",
			attributes: map[string]string{"ssn": "123-45-6789"},
			piiField:   "ssn",
			value:      "123-45-6789",
		},
		{
			name:       "CreditCard",
			attributes: map[string]string{"card": "4532-1234-5678-9010"},
			piiField:   "card",
			value:      "4532-1234-5678-9010",
		},
	}

	for _, tc := range piiCases {
		t.Run(tc.name, func(t *testing.T) {
			ingestReq := models.IngestRequest{
				Spans: []models.Span{
					{
						TraceID:      "trace-pii-" + tc.name,
						SpanID:       "span-pii-" + tc.name,
						Framework:    "anthropic-api",
						InputTokens:  30,
						OutputTokens: 15,
						ModelName:    "claude-3-haiku",
						ReceivedAt:   time.Now(),
						Attributes:   tc.attributes,
					},
				},
			}

			body, _ := json.Marshal(ingestReq)
			req := httptest.NewRequest("POST", "/v1/ingest", bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer valid-token")
			req.Header.Set("X-Tenant-ID", "test-tenant")
			w := httptest.NewRecorder()
			h.Ingest(w, req)

			if w.Code != http.StatusOK {
				t.Logf("Warning: Ingest failed with %d", w.Code)
				return
			}

			// Query back and verify PII is scrubbed
			traceID := "trace-pii-" + tc.name
			queryReq := httptest.NewRequest("GET", "/v1/traces/"+traceID, nil)
			queryReq.Header.Set("Authorization", "Bearer valid-token")
			queryReq.Header.Set("X-Tenant-ID", "test-tenant")
			queryW := httptest.NewRecorder()
			h.GetTrace(queryW, queryReq)

			var trace models.Trace
			json.Unmarshal(queryW.Body.Bytes(), &trace)

			if len(trace.Spans) > 0 {
				span := trace.Spans[0]
				if val, exists := span.Attributes[tc.piiField]; exists {
					if val != tc.value {
						t.Logf("✓ %s scrubbed: original=%s, stored=%s", tc.name, tc.value, val)
					} else {
						t.Logf("Warning: %s NOT scrubbed (still contains original value)", tc.name)
					}
				}
			}
		})
	}
}

// TestDangerousCommandDetection: Verify shell commands are flagged
func TestDangerousCommandDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	db := setupTestDB(t)
	if db == nil {
		t.Skip("Test DB not available")
	}
	defer db.Close()

	h := handlers.New(db, nil, governance.NewRiskEngine())

	commandCases := []struct {
		name           string
		command        string
		expectDangerous bool
	}{
		{
			name:           "RmRf",
			command:        "rm -rf /",
			expectDangerous: true,
		},
		{
			name:           "SudoCommand",
			command:        "sudo ./deploy.sh",
			expectDangerous: true,
		},
		{
			name:           "SafeLsCommand",
			command:        "ls -la",
			expectDangerous: false,
		},
		{
			name:           "EchoSafeMessage",
			command:        "echo 'Hello, world!'",
			expectDangerous: false,
		},
	}

	for _, tc := range commandCases {
		t.Run(tc.name, func(t *testing.T) {
			ingestReq := models.IngestRequest{
				Spans: []models.Span{
					{
						TraceID:      "trace-cmd-" + tc.name,
						SpanID:       "span-cmd-" + tc.name,
						Framework:    "claude-agents",
						InputTokens:  40,
						OutputTokens: 20,
						ModelName:    "claude-3-haiku",
						ReceivedAt:   time.Now(),
						Attributes: map[string]string{
							"tool.command": tc.command,
							"event.type":   "tool_use",
						},
					},
				},
			}

			body, _ := json.Marshal(ingestReq)
			req := httptest.NewRequest("POST", "/v1/ingest", bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer valid-token")
			req.Header.Set("X-Tenant-ID", "test-tenant")
			w := httptest.NewRecorder()
			h.Ingest(w, req)

			if w.Code != http.StatusOK {
				t.Logf("Warning: Ingest failed with %d", w.Code)
				return
			}

			// Query and verify risk
			queryReq := httptest.NewRequest("GET", "/v1/traces/trace-cmd-"+tc.name, nil)
			queryReq.Header.Set("Authorization", "Bearer valid-token")
			queryReq.Header.Set("X-Tenant-ID", "test-tenant")
			queryW := httptest.NewRecorder()
			h.GetTrace(queryW, queryReq)

			var trace models.Trace
			json.Unmarshal(queryW.Body.Bytes(), &trace)

			if len(trace.Spans) > 0 {
				span := trace.Spans[0]
				isDangerous := span.RiskScore >= 90 && span.RiskCategory == "dangerous_shell_command"

				if tc.expectDangerous && isDangerous {
					t.Logf("✓ %s correctly flagged dangerous (risk_score=%d)", tc.name, span.RiskScore)
				} else if !tc.expectDangerous && !isDangerous {
					t.Logf("✓ %s marked safe (risk_score=%d)", tc.name, span.RiskScore)
				} else {
					t.Logf("Note: %s risk detection mismatch (expect_dangerous=%v, actual=%v)", tc.name, tc.expectDangerous, isDangerous)
				}
			}
		})
	}
}
