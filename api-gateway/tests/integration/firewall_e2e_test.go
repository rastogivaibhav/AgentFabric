//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/govagn/api-gateway/internal/middleware"
	"github.com/govagn/api-gateway/internal/models"
)

type firewallIngestDecision struct {
	DecisionID   string `json:"decision_id"`
	TraceID      string `json:"trace_id"`
	SpanID       string `json:"span_id"`
	Result       string `json:"result"`
	Reason       string `json:"reason"`
	RiskScore    int    `json:"risk_score"`
	RiskCategory string `json:"risk_category"`
	ActionTaken  string `json:"action_taken"`
}

type firewallIngestResponse struct {
	Status         string                   `json:"status"`
	Accepted       int                      `json:"accepted"`
	Blocked        int                      `json:"blocked"`
	ReviewRequired bool                     `json:"review_required"`
	Decisions      []firewallIngestDecision `json:"decisions"`
}

func TestFirewallAdversarialE2E(t *testing.T) {
	ts := newTestServer(t)
	server := httptest.NewServer(ts.r)
	defer server.Close()

	wsConn := dialPolicyWebSocket(t, server.URL, ts.token)
	defer wsConn.Close()

	suffix := time.Now().UnixNano()

	safeTrace := fmt.Sprintf("fw-safe-%d", suffix)
	safeDecision := postFirewallSpan(t, ts, models.Span{
		TraceID:     safeTrace,
		ID:          "span-safe",
		RunID:       "run-firewall-safe",
		Name:        "safe completion",
		Framework:   "openai",
		Model:       "gpt-4o-mini",
		InputTokens: 25,
		Attributes: map[string]string{
			"event.type": "completion",
		},
	}, http.StatusOK)
	if safeDecision.Result != "approved" {
		t.Fatalf("safe span should be approved, got %+v", safeDecision)
	}
	requireTraceStoredWithDecision(t, ts, safeTrace, "span-safe", "approved")
	requireAuditDecision(t, ts, safeTrace, "span-safe", "approved")
	requirePortalAuditDecision(t, ts, safeTrace, "span-safe", "approved")
	requireWebSocketPolicyDecision(t, wsConn, safeDecision.DecisionID, "approved")

	reviewTrace := fmt.Sprintf("fw-review-%d", suffix)
	reviewDecision := postFirewallSpan(t, ts, models.Span{
		TraceID:   reviewTrace,
		ID:        "span-review",
		RunID:     "run-firewall-review",
		Name:      "production env edit",
		Framework: "cursor",
		Model:     "claude-3-5-sonnet",
		Attributes: map[string]string{
			"event.category": "tool_call",
			"event.action":   "file_edit",
			"tool.file_path": "/srv/prod/.env",
		},
	}, http.StatusOK)
	if reviewDecision.Result != "review" || reviewDecision.ActionTaken != "require_governance_review" {
		t.Fatalf("prod edit should require review, got %+v", reviewDecision)
	}
	requireTraceStoredWithDecision(t, ts, reviewTrace, "span-review", "review")
	requireAuditDecision(t, ts, reviewTrace, "span-review", "review")
	requirePortalAuditDecision(t, ts, reviewTrace, "span-review", "review")
	requireWebSocketPolicyDecision(t, wsConn, reviewDecision.DecisionID, "review")

	blockTrace := fmt.Sprintf("fw-block-%d", suffix)
	blockDecision := postFirewallSpan(t, ts, models.Span{
		TraceID:   blockTrace,
		ID:        "span-block",
		RunID:     "run-firewall-block",
		Name:      "secret exfiltration attempt",
		Framework: "vscode",
		Model:     "claude-3-5-sonnet",
		Attributes: map[string]string{
			"event.category": "tool_call",
			"event.action":   "shell_command",
			"tool.command":   "echo sk-proj-abcdefghijklmnop into /tmp/leak",
		},
	}, http.StatusForbidden)
	if blockDecision.Result != "deny" || blockDecision.ActionTaken != "reject_ingest_span" {
		t.Fatalf("secret-bearing command should be denied, got %+v", blockDecision)
	}
	requireTraceNotStored(t, ts, blockTrace)
	requireAuditDecision(t, ts, blockTrace, "span-block", "deny")
	requirePortalAuditDecision(t, ts, blockTrace, "span-block", "deny")
	requireWebSocketPolicyDecision(t, wsConn, blockDecision.DecisionID, "deny")
}

func dialPolicyWebSocket(t *testing.T, serverURL, token string) *websocket.Conn {
	t.Helper()

	wsURL := "ws" + strings.TrimPrefix(serverURL, "http") + "/api/v1/ws"
	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	return conn
}

func postFirewallSpan(t *testing.T, ts *testServer, span models.Span, wantStatus int) firewallIngestDecision {
	t.Helper()

	span.ReceivedAt = time.Now()
	payload := map[string]any{"spans": []models.Span{span}}
	rr := ts.do("POST", "/internal/ingest", payload, collectorHeaders())
	if rr.Code != wantStatus {
		t.Fatalf("ingest %s: expected %d, got %d: %s", span.ID, wantStatus, rr.Code, rr.Body.String())
	}

	var resp firewallIngestResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode ingest response: %v\nbody=%s", err, rr.Body.String())
	}
	if len(resp.Decisions) != 1 {
		t.Fatalf("expected one firewall decision, got %+v", resp)
	}
	return resp.Decisions[0]
}

func requireTraceStoredWithDecision(t *testing.T, ts *testServer, traceID, spanID, result string) {
	t.Helper()

	inputs, err := ts.pg.LoadTraceViewInputs(context.Background(), traceID, middleware.DefaultTenantID)
	if err != nil {
		t.Fatalf("load trace inputs: %v", err)
	}
	if len(inputs.Spans) != 1 {
		t.Fatalf("expected one stored span for %s, got %d", traceID, len(inputs.Spans))
	}
	if inputs.Spans[0].ID != spanID {
		t.Fatalf("expected span %s, got %s", spanID, inputs.Spans[0].ID)
	}
	if got := inputs.Spans[0].Attributes["af.firewall.result"]; got != result {
		t.Fatalf("stored span firewall result: expected %q, got %q", result, got)
	}
}

func requireTraceNotStored(t *testing.T, ts *testServer, traceID string) {
	t.Helper()

	inputs, err := ts.pg.LoadTraceViewInputs(context.Background(), traceID, middleware.DefaultTenantID)
	if err != nil {
		t.Fatalf("load trace inputs: %v", err)
	}
	if len(inputs.Spans) != 0 {
		t.Fatalf("blocked trace %s should not store spans, got %d", traceID, len(inputs.Spans))
	}
}

func requireAuditDecision(t *testing.T, ts *testServer, traceID, spanID, result string) {
	t.Helper()

	entries, err := ts.pg.ListAuditEntriesForTrace(context.Background(), middleware.DefaultTenantID, traceID)
	if err != nil {
		t.Fatalf("list audit entries: %v", err)
	}
	for _, entry := range entries {
		if entry.SpanID == spanID && entry.PolicyName == "ingest_firewall" && entry.Result == result {
			return
		}
	}
	t.Fatalf("missing audit decision trace=%s span=%s result=%s entries=%+v", traceID, spanID, result, entries)
}

func requirePortalAuditDecision(t *testing.T, ts *testServer, traceID, spanID, result string) {
	t.Helper()

	rr := ts.do("GET", "/api/v1/audit?limit=200", nil, ts.authHeader())
	if rr.Code != http.StatusOK {
		t.Fatalf("portal audit endpoint: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Items []struct {
			TraceID    string `json:"trace_id"`
			SpanID     string `json:"span_id"`
			PolicyName string `json:"policy_name"`
			Result     string `json:"result"`
		} `json:"items"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode portal audit response: %v", err)
	}
	for _, item := range resp.Items {
		if item.TraceID == traceID && item.SpanID == spanID && item.PolicyName == "ingest_firewall" && item.Result == result {
			return
		}
	}
	t.Fatalf("portal audit missing trace=%s span=%s result=%s items=%+v", traceID, spanID, result, resp.Items)
}

func requireWebSocketPolicyDecision(t *testing.T, conn *websocket.Conn, decisionID, result string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
			t.Fatalf("set websocket deadline: %v", err)
		}
		_, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				t.Fatalf("websocket closed before policy event")
			}
			continue
		}

		var event struct {
			Type string `json:"type"`
			Data struct {
				DecisionID string `json:"decision_id"`
				Result     string `json:"result"`
			} `json:"data"`
		}
		if err := json.Unmarshal(data, &event); err != nil {
			t.Fatalf("decode websocket event: %v", err)
		}
		if event.Type == "policy" && event.Data.DecisionID == decisionID && event.Data.Result == result {
			return
		}
	}
	t.Fatalf("did not receive websocket policy decision %s=%s", decisionID, result)
}
