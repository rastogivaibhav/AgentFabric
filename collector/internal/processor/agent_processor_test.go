package processor

// Unit tests for PII scrubbing, framework detection, and cost computation.
// These are pure functions with no external dependencies.
// Run: go test ./internal/processor/...

import (
	"math"
	"strings"
	"testing"
)

// ─── computeCost ─────────────────────────────────────────────────────────────

func TestComputeCost_ClaudeHaiku(t *testing.T) {
	// claude-3-haiku: $0.25 input / $1.25 output per 1M tokens
	inputCost, outputCost := computeCost("claude-3-haiku", 1_000_000, 1_000_000)
	assertAlmost(t, 0.25, inputCost, "claude-3-haiku input cost")
	assertAlmost(t, 1.25, outputCost, "claude-3-haiku output cost")
}

func TestComputeCost_GPT4o(t *testing.T) {
	// gpt-4o: $5.0 input / $15.0 output per 1M tokens
	inputCost, outputCost := computeCost("gpt-4o", 1_000_000, 1_000_000)
	assertAlmost(t, 5.0, inputCost, "gpt-4o input cost")
	assertAlmost(t, 15.0, outputCost, "gpt-4o output cost")
}

func TestComputeCost_GPT4oMini(t *testing.T) {
	// gpt-4o-mini: $0.15 input / $0.60 output per 1M tokens
	inputCost, outputCost := computeCost("gpt-4o-mini", 1_000_000, 1_000_000)
	assertAlmost(t, 0.15, inputCost, "gpt-4o-mini input cost")
	assertAlmost(t, 0.60, outputCost, "gpt-4o-mini output cost")
}

func TestComputeCost_ClaudeSonnet(t *testing.T) {
	// claude-3-5-sonnet: $3.0 input / $15.0 output per 1M tokens
	inputCost, outputCost := computeCost("claude-3-5-sonnet", 1_000_000, 1_000_000)
	assertAlmost(t, 3.0, inputCost, "claude-3-5-sonnet input cost")
	assertAlmost(t, 15.0, outputCost, "claude-3-5-sonnet output cost")
}

func TestComputeCost_PartialTokens(t *testing.T) {
	// 500K input + 200K output at claude-3-haiku pricing
	inputCost, outputCost := computeCost("claude-3-haiku", 500_000, 200_000)
	assertAlmost(t, 0.125, inputCost, "haiku 500K input")
	assertAlmost(t, 0.25, outputCost, "haiku 200K output")
}

func TestComputeCost_ZeroTokens(t *testing.T) {
	inputCost, outputCost := computeCost("gpt-4o", 0, 0)
	if inputCost != 0.0 || outputCost != 0.0 {
		t.Errorf("zero tokens should yield zero cost, got input=%f output=%f", inputCost, outputCost)
	}
}

func TestComputeCost_UnknownModel(t *testing.T) {
	// Unknown model should return 0,0 (not crash)
	inputCost, outputCost := computeCost("unknown-model-xyz", 1_000_000, 1_000_000)
	if inputCost != 0.0 || outputCost != 0.0 {
		t.Errorf("unknown model should return zero cost, got input=%f output=%f", inputCost, outputCost)
	}
}

func TestComputeCost_CaseInsensitive(t *testing.T) {
	lower, _ := computeCost("gpt-4o", 1_000_000, 0)
	upper, _ := computeCost("GPT-4O", 1_000_000, 0)
	assertAlmost(t, lower, upper, "cost lookup should be case-insensitive")
}

func TestComputeCost_InputOutputAreIndependent(t *testing.T) {
	// Critical: P0-1 fix verification. Input and output costs must NOT be a fixed ratio of total.
	// With 100K input and 900K output at claude-3-5-sonnet ($3/$15 per 1M):
	//   inputCost = 0.1M * $3 = $0.30
	//   outputCost = 0.9M * $15 = $13.50
	//   total = $13.80
	// A naive 60/40 split would give: 60% * 13.80 = 8.28 (wrong for input)
	inputCost, outputCost := computeCost("claude-3-5-sonnet", 100_000, 900_000)
	assertAlmost(t, 0.30, inputCost, "input cost independent of output volume")
	assertAlmost(t, 13.50, outputCost, "output cost independent of input volume")
	// Verify these are NOT a 60/40 split of total
	total := inputCost + outputCost
	if math.Abs(inputCost-total*0.6) < 0.001 {
		t.Error("P0-1: input_cost_usd appears to be hardcoded 60% split — fix not applied")
	}
}

// ─── Framework Detection ─────────────────────────────────────────────────────

func TestDetectFramework_CrewAIByAttribute(t *testing.T) {
	attrs := map[string]string{
		"crewai.agent.role": "researcher",
	}
	fw := detectFramework(attrs, "")
	assertEqual(t, string(FrameworkCrewAI), string(fw), "crewai.agent.role should detect CrewAI")
}

func TestDetectFramework_CrewAIByCrewID(t *testing.T) {
	attrs := map[string]string{
		"crewai.crew.id": "crew-abc",
	}
	fw := detectFramework(attrs, "")
	assertEqual(t, string(FrameworkCrewAI), string(fw), "crewai.crew.id should detect CrewAI")
}

func TestDetectFramework_LangGraphByAttribute(t *testing.T) {
	attrs := map[string]string{
		"langgraph.node.name": "process",
	}
	fw := detectFramework(attrs, "")
	assertEqual(t, string(FrameworkLangGraph), string(fw), "langgraph.node.name should detect LangGraph")
}

func TestDetectFramework_GoogleADK(t *testing.T) {
	attrs := map[string]string{
		"google.adk.agent.name": "my-agent",
	}
	fw := detectFramework(attrs, "")
	assertEqual(t, string(FrameworkGoogleADK), string(fw), "google.adk.agent.name should detect Google ADK")
}

func TestDetectFramework_OpenAIByRunID(t *testing.T) {
	attrs := map[string]string{
		"openai.run.id": "run_abc123",
	}
	fw := detectFramework(attrs, "")
	assertEqual(t, string(FrameworkOpenAIAgents), string(fw), "openai.run.id should detect OpenAI Agents")
}

func TestDetectFramework_AnthropicByModel(t *testing.T) {
	attrs := map[string]string{
		"anthropic.model": "claude-3-5-sonnet",
	}
	fw := detectFramework(attrs, "")
	assertEqual(t, string(FrameworkClaudeAgents), string(fw), "anthropic.model should detect Claude Agents")
}

func TestDetectFramework_ModelFallbackClaude(t *testing.T) {
	attrs := map[string]string{
		"gen_ai.request.model": "claude-3-haiku-20240307",
	}
	fw := detectFramework(attrs, "")
	assertEqual(t, string(FrameworkClaudeAgents), string(fw), "claude model prefix should fallback to Claude Agents")
}

func TestDetectFramework_ModelFallbackGPT(t *testing.T) {
	attrs := map[string]string{
		"gen_ai.request.model": "gpt-4o-mini",
	}
	fw := detectFramework(attrs, "")
	assertEqual(t, string(FrameworkOpenAIAgents), string(fw), "gpt model prefix should fallback to OpenAI Agents")
}

func TestDetectFramework_ModelFallbackGemini(t *testing.T) {
	attrs := map[string]string{
		"gen_ai.request.model": "gemini-1.5-pro",
	}
	fw := detectFramework(attrs, "")
	assertEqual(t, string(FrameworkGoogleADK), string(fw), "gemini model should fallback to Google ADK")
}

func TestDetectFramework_SpanNameFallbackCrewAI(t *testing.T) {
	fw := detectFramework(map[string]string{}, "crewai.agent.execute_task")
	assertEqual(t, string(FrameworkCrewAI), string(fw), "span name containing crewai should detect CrewAI")
}

func TestDetectFramework_SpanNameFallbackLangGraph(t *testing.T) {
	fw := detectFramework(map[string]string{}, "langgraph.node")
	assertEqual(t, string(FrameworkLangGraph), string(fw), "span name containing langgraph should detect LangGraph")
}

func TestDetectFramework_Unknown(t *testing.T) {
	fw := detectFramework(map[string]string{}, "some-random-span")
	assertEqual(t, string(FrameworkUnknown), string(fw), "unrecognised span should return unknown")
}

func TestDetectFramework_ServerSide_NeverTrustsClientFrameworkAttr(t *testing.T) {
	// Principle 5: Server recomputes framework; client-supplied af.agent.framework is IGNORED
	// If a span has af.agent.framework='crewai' but crewai SDK attributes are absent,
	// the detection result should be 'unknown' (not trusting the client's claim).
	attrs := map[string]string{
		"af.agent.framework": "crewai", // client-supplied — must be ignored
	}
	fw := detectFramework(attrs, "some-span")
	assertEqual(t, string(FrameworkUnknown), string(fw),
		"Principle 5: server must NOT trust client-supplied af.agent.framework")
}

// ─── PII Scrubbing ───────────────────────────────────────────────────────────

func newTestProcessor() *AgentProcessor {
	return &AgentProcessor{}
}

func TestScrubPII_SSN(t *testing.T) {
	p := newTestProcessor()
	attrs := map[string]string{"note": "SSN is 123-45-6789"}
	result := p.scrubPII(attrs)
	if result["note"] == attrs["note"] {
		t.Error("SSN should be redacted")
	}
	assertContains(t, result["note"], "[REDACTED]")
}

func TestScrubPII_CreditCard(t *testing.T) {
	p := newTestProcessor()
	attrs := map[string]string{"payment": "card 4532-0151-1283-0366"}
	result := p.scrubPII(attrs)
	assertContains(t, result["payment"], "[REDACTED]")
}

func TestScrubPII_Email(t *testing.T) {
	p := newTestProcessor()
	attrs := map[string]string{"user": "contact: alice@example.com"}
	result := p.scrubPII(attrs)
	assertContains(t, result["user"], "[REDACTED]")
}

func TestScrubPII_Credentials(t *testing.T) {
	p := newTestProcessor()
	attrs := map[string]string{"config": "api_key=sk-abc123secret"}
	result := p.scrubPII(attrs)
	assertContains(t, result["config"], "[REDACTED]")
}

func TestScrubPII_Password(t *testing.T) {
	p := newTestProcessor()
	attrs := map[string]string{"auth": "password: supersecret123"}
	result := p.scrubPII(attrs)
	assertContains(t, result["auth"], "[REDACTED]")
}

func TestScrubPII_UKPostcode(t *testing.T) {
	p := newTestProcessor()
	attrs := map[string]string{"addr": "address at SW1A 1AA"}
	result := p.scrubPII(attrs)
	assertContains(t, result["addr"], "[REDACTED]")
}

func TestScrubPII_UKPhone(t *testing.T) {
	p := newTestProcessor()
	attrs := map[string]string{"contact": "call +44 7700 900123"}
	result := p.scrubPII(attrs)
	assertContains(t, result["contact"], "[REDACTED]")
}

func TestScrubPII_Name(t *testing.T) {
	p := newTestProcessor()
	attrs := map[string]string{"customer": "Dr. John Smith visited"}
	result := p.scrubPII(attrs)
	assertContains(t, result["customer"], "[REDACTED]")
}

func TestScrubPII_CleanDataUnchanged(t *testing.T) {
	p := newTestProcessor()
	attrs := map[string]string{
		"framework": "crewai",
		"model":     "gpt-4o",
		"status":    "completed",
	}
	result := p.scrubPII(attrs)
	if result["framework"] != "crewai" {
		t.Error("clean attribute should not be modified")
	}
	if result["model"] != "gpt-4o" {
		t.Error("clean attribute should not be modified")
	}
}

func TestScrubPII_AllSevenPatterns(t *testing.T) {
	// Verify all 7 production PII patterns fire correctly (Principle 7)
	p := newTestProcessor()
	cases := []struct {
		name  string
		value string
	}{
		{"SSN", "patient SSN: 987-65-4321"},
		{"Card", "VISA 4111-1111-1111-1111"},
		{"Email", "user@domain.co.uk"},
		{"Credential", "secret=abc123xyz"},
		{"UKPostcode", "EC1A 1BB"},
		{"UKPhone", "+447700123456"},
		{"Name", "Mrs. Jane Doe"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := p.scrubPII(map[string]string{"v": tc.value})
			assertContains(t, result["v"], "[REDACTED]",
				"PII pattern '%s' must be redacted (Principle 7)", tc.name)
		})
	}
}

func TestScrubPII_EmptyAttrs(t *testing.T) {
	p := newTestProcessor()
	result := p.scrubPII(map[string]string{})
	if len(result) != 0 {
		t.Error("empty input should return empty output")
	}
}

// ─── Test Helpers ─────────────────────────────────────────────────────────────

func assertAlmost(t *testing.T, expected, actual float64, msg string) {
	t.Helper()
	if math.Abs(expected-actual) > 1e-9 {
		t.Errorf("%s: expected %.10f, got %.10f", msg, expected, actual)
	}
}

func assertEqual(t *testing.T, expected, actual, msg string) {
	t.Helper()
	if expected != actual {
		t.Errorf("%s: expected %q, got %q", msg, expected, actual)
	}
}

func assertContains(t *testing.T, s, substr string, msgAndArgs ...interface{}) {
	t.Helper()
	if !strings.Contains(s, substr) {
		if len(msgAndArgs) > 0 {
			t.Errorf("%s: expected %q to contain %q", msgAndArgs[0], s, substr)
		} else {
			t.Errorf("expected %q to contain %q", s, substr)
		}
	}
}
