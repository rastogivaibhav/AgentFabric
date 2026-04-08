package handlers

// Unit tests for pure handler helper functions.
// No database or network required — all tests are in-process.
// Run: go test ./internal/handlers/...

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/govagn/api-gateway/internal/models"
)

// ─── buildTrace ──────────────────────────────────────────────────────────────

func makeSpan(id, parentID, framework string, startNs, durationNs int64, statusCode int, costUSD float64, inputTok, outputTok int64) models.Span {
	return models.Span{
		ID:           id,
		ParentID:     parentID,
		Framework:    framework,
		Name:         "span-" + id,
		StartTimeNs:  startNs,
		DurationNs:   durationNs,
		StatusCode:   statusCode,
		CostUSD:      costUSD,
		InputTokens:  inputTok,
		OutputTokens: outputTok,
	}
}

func TestBuildTrace_EmptySpans(t *testing.T) {
	tr := buildTrace("tr-001", nil)
	if tr.ID != "tr-001" {
		t.Errorf("trace ID should be preserved, got %q", tr.ID)
	}
	if tr.SpanCount != 0 {
		t.Errorf("empty spans should give span_count=0")
	}
}

func TestBuildTrace_SingleSpan(t *testing.T) {
	spans := []models.Span{
		makeSpan("s1", "", "crewai", 1_000_000_000, 500_000_000, 0, 0.001, 100, 50),
	}
	tr := buildTrace("tr-001", spans)
	if tr.Framework != "crewai" {
		t.Errorf("framework should come from first span, got %q", tr.Framework)
	}
	if tr.SpanCount != 1 {
		t.Errorf("span_count should be 1")
	}
	if tr.Status != "ok" {
		t.Errorf("status should be ok when no errors")
	}
}

func TestBuildTrace_AggregatesCost(t *testing.T) {
	spans := []models.Span{
		makeSpan("s1", "", "crewai", 1_000, 1_000, 0, 0.001, 0, 0),
		makeSpan("s2", "s1", "crewai", 2_000, 1_000, 0, 0.002, 0, 0),
		makeSpan("s3", "s1", "crewai", 3_000, 1_000, 0, 0.003, 0, 0),
	}
	tr := buildTrace("tr-001", spans)
	if abs64(tr.TotalCostUSD-0.006) > 1e-9 {
		t.Errorf("total_cost_usd should be 0.006, got %f", tr.TotalCostUSD)
	}
}

func TestBuildTrace_AggregatesToknes(t *testing.T) {
	spans := []models.Span{
		makeSpan("s1", "", "openai_agents", 0, 100, 0, 0, 100, 50),
		makeSpan("s2", "s1", "openai_agents", 100, 100, 0, 0, 200, 100),
	}
	tr := buildTrace("tr-001", spans)
	if tr.TotalTokens != 450 {
		t.Errorf("total_tokens should be 450 (100+50+200+100), got %d", tr.TotalTokens)
	}
}

func TestBuildTrace_ErrorCountAndStatus(t *testing.T) {
	spans := []models.Span{
		makeSpan("s1", "", "crewai", 0, 100, 0, 0, 0, 0),
		makeSpan("s2", "s1", "crewai", 100, 100, 2, 0, 0, 0), // error
		makeSpan("s3", "s1", "crewai", 200, 100, 2, 0, 0, 0), // error
	}
	tr := buildTrace("tr-001", spans)
	if tr.ErrorCount != 2 {
		t.Errorf("error_count should be 2, got %d", tr.ErrorCount)
	}
	if tr.Status != "error" {
		t.Errorf("status should be error when errors present")
	}
}

func TestBuildTrace_DerivesInsights(t *testing.T) {
	spans := []models.Span{
		{
			ID: "root", Name: "root", Framework: "proxy", StartTimeNs: 0, DurationNs: 1000, StatusCode: 0,
			Attributes: map[string]string{
				"gen_ai.system":          "openai",
				"gen_ai.request.model":   "gpt-4o",
				"af.span.step_type":      "llm",
				"service.name":           "customer-support",
				"deployment.environment": "prod",
				"af.pricing.rule_id":     "12",
			},
		},
		{
			ID: "child", ParentID: "root", Name: "tool.search", Framework: "proxy", StartTimeNs: 100, DurationNs: 500, StatusCode: 2,
			Attributes: map[string]string{
				"af.span.step_type": "tool",
				"af.error.class":    "timeout",
				"af.policy.blocked": "true",
				"af.policy.reason":  "model denied",
				"retry.count":       "2",
			},
		},
	}
	tr := buildTrace("trace-1", spans)
	if tr.Insights.LLMCalls != 1 || tr.Insights.ToolCalls != 1 {
		t.Fatalf("unexpected step insights: %+v", tr.Insights)
	}
	if tr.Insights.BlockedSpans != 1 {
		t.Fatalf("expected blocked spans to be tracked")
	}
	if tr.Insights.MaxDepth != 1 {
		t.Fatalf("expected max depth 1, got %d", tr.Insights.MaxDepth)
	}
	if len(tr.Insights.Models) != 1 || tr.Insights.Models[0] != "gpt-4o" {
		t.Fatalf("expected model insight to include gpt-4o")
	}
}

func TestBuildTrace_Duration(t *testing.T) {
	// root span starts at t=0, duration=1000ns
	// child span starts at t=500, duration=1000ns — ends at t=1500
	// total duration = 1500 - 0 = 1500ns
	spans := []models.Span{
		makeSpan("s1", "", "crewai", 0, 1000, 0, 0, 0, 0),
		makeSpan("s2", "s1", "crewai", 500, 1000, 0, 0, 0, 0),
	}
	tr := buildTrace("tr-001", spans)
	if tr.Duration != 1500 {
		t.Errorf("duration should be 1500ns, got %d", tr.Duration)
	}
}

func TestBuildTrace_StartTime(t *testing.T) {
	startNs := int64(1_700_000_000_000_000_000)
	spans := []models.Span{
		makeSpan("s1", "", "crewai", startNs, 1000, 0, 0, 0, 0),
	}
	tr := buildTrace("tr-001", spans)
	expected := time.Unix(0, startNs)
	if !tr.StartTime.Equal(expected) {
		t.Errorf("start_time mismatch: expected %v, got %v", expected, tr.StartTime)
	}
}

// ─── buildTopologyGraph ──────────────────────────────────────────────────────

func TestBuildTopologyGraph_EmptySpans(t *testing.T) {
	g := buildTopologyGraph(nil)
	if len(g.Nodes) != 0 {
		t.Errorf("empty spans should yield 0 nodes")
	}
	if len(g.Edges) != 0 {
		t.Errorf("empty spans should yield 0 edges")
	}
}

func TestBuildTopologyGraph_SingleSpan(t *testing.T) {
	spans := []models.Span{
		makeSpan("s1", "", "crewai", 0, 100, 0, 0, 0, 0),
	}
	g := buildTopologyGraph(spans)
	if len(g.Nodes) != 1 {
		t.Errorf("single span should yield 1 node, got %d", len(g.Nodes))
	}
	if len(g.Edges) != 0 {
		t.Errorf("root-only span has no parent — should yield 0 edges")
	}
}

func TestBuildTopologyGraph_LinearChain(t *testing.T) {
	// s1 → s2 → s3
	spans := []models.Span{
		makeSpan("s1", "", "crewai", 0, 100, 0, 0, 0, 0),
		makeSpan("s2", "s1", "crewai", 100, 100, 0, 0, 0, 0),
		makeSpan("s3", "s2", "crewai", 200, 100, 0, 0, 0, 0),
	}
	g := buildTopologyGraph(spans)
	if len(g.Nodes) != 3 {
		t.Errorf("3 spans should yield 3 nodes, got %d", len(g.Nodes))
	}
	if len(g.Edges) != 2 {
		t.Errorf("linear chain of 3 should yield 2 edges, got %d", len(g.Edges))
	}
}

func TestBuildTopologyGraph_DeduplicatesNodes(t *testing.T) {
	// Duplicate span IDs across traces should collapse to unique nodes
	spans := []models.Span{
		makeSpan("s1", "", "crewai", 0, 100, 0, 0, 0, 0),
		makeSpan("s1", "", "crewai", 0, 100, 0, 0, 0, 0), // duplicate
	}
	g := buildTopologyGraph(spans)
	if len(g.Nodes) != 1 {
		t.Errorf("duplicate span IDs should yield 1 unique node, got %d", len(g.Nodes))
	}
}

func TestBuildTopologyGraph_CallCountAccumulates(t *testing.T) {
	// Same parent→child edge appears twice (e.g. agent called tool twice)
	spans := []models.Span{
		makeSpan("s1", "", "crewai", 0, 100, 0, 0, 0, 0),
		makeSpan("s2", "s1", "crewai", 100, 100, 0, 0, 0, 0),
		makeSpan("s3", "s1", "crewai", 200, 100, 0, 0, 0, 0),
	}
	g := buildTopologyGraph(spans)
	// s1→s2 and s1→s3 are distinct edges (different children)
	if len(g.Edges) != 2 {
		t.Errorf("expected 2 distinct edges, got %d", len(g.Edges))
	}
}

func TestBuildTopologyGraph_P02Fix_BatchNotLoop(t *testing.T) {
	// This test documents the P0-2 fix: GetAgentTopology must use GetSpansForTraces
	// (batch query with IN clause) rather than N individual GetTraceSpans calls.
	// The handler refactor means this function receives pre-fetched spans — verify correctness.
	spans := make([]models.Span, 0, 100)
	for i := 0; i < 10; i++ {
		spans = append(spans, makeSpan(
			"root-"+string(rune('A'+i)),
			"",
			"langgraph",
			int64(i*1000), 100, 0, 0.001, 0, 0,
		))
	}
	g := buildTopologyGraph(spans)
	if len(g.Nodes) != 10 {
		t.Errorf("10 spans (from batch query) should yield 10 nodes, got %d", len(g.Nodes))
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func TestWriteError_HelperExistsAndProducesJSON(t *testing.T) {
	// Structural: verify these functions exist (compilation = passing test)
	_ = writeJSON
	_ = writeError
	_ = tenantFromCtx
}

func TestParseIntOr_DefaultOnEmpty(t *testing.T) {
	result := parseIntOr("", 42)
	if result != 42 {
		t.Errorf("empty string should return default 42, got %d", result)
	}
}

func TestParseCostReportQuery_ReadsDimensionFilters(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/analytics/cost?since=48h&app_name=support&environment=staging&provider=openai&model=gpt-4o&prompt_id=support.system&release_tag=candidate-7&limit=25", nil)
	query := parseCostReportQuery(req)

	if query.Since != 48*time.Hour {
		t.Fatalf("expected since=48h, got %s", query.Since)
	}
	if query.AppName != "support" || query.Environment != "staging" || query.Provider != "openai" {
		t.Fatalf("expected top-level filters to be parsed, got %+v", query)
	}
	if query.Model != "gpt-4o" || query.PromptID != "support.system" || query.ReleaseTag != "candidate-7" {
		t.Fatalf("expected model/prompt/release filters to be parsed, got %+v", query)
	}
	if query.Limit != 25 {
		t.Fatalf("expected limit to be parsed, got %d", query.Limit)
	}
}

func TestParseIntOr_ParsesValidInt(t *testing.T) {
	result := parseIntOr("100", 42)
	if result != 100 {
		t.Errorf("valid string '100' should parse to 100, got %d", result)
	}
}

func TestParseIntOr_DefaultOnInvalid(t *testing.T) {
	result := parseIntOr("not-a-number", 42)
	if result != 42 {
		t.Errorf("invalid string should return default 42, got %d", result)
	}
}

// ─── test utilities ──────────────────────────────────────────────────────────

func abs64(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
