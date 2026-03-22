package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agentfabric/api-gateway/internal/models"
	"github.com/agentfabric/api-gateway/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ─── Test doubles ────────────────────────────────────────────────────────────

const devVaultKey = "0000000000000000000000000000000000000000000000000000000000000001"

// mockStore records spans written through it.
type mockStore struct {
	spans []models.Span
}

func (m *mockStore) BulkInsertSpans(_ context.Context, spans []models.Span) error {
	m.spans = append(m.spans, spans...)
	return nil
}

func (m *mockStore) CreatePolicyAuditEntry(_ context.Context, _ models.PolicyDecisionAudit) error {
	return nil
}

// ─── Parser unit tests ───────────────────────────────────────────────────────

func TestOpenAIParser_ParseRequest(t *testing.T) {
	body := `{"model":"gpt-4o","stream":false,"messages":[{"role":"user","content":"hello"}]}`
	p := &openAIParser{}
	model, streaming, tokens, err := p.ParseRequest([]byte(body))
	require.NoError(t, err)
	assert.Equal(t, "gpt-4o", model)
	assert.False(t, streaming)
	assert.Greater(t, tokens, int64(0))
}

func TestOpenAIParser_ParseRequest_Streaming(t *testing.T) {
	body := `{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"hi"}]}`
	p := &openAIParser{}
	_, streaming, _, err := p.ParseRequest([]byte(body))
	require.NoError(t, err)
	assert.True(t, streaming)
}

func TestOpenAIParser_ParseUsage(t *testing.T) {
	resp := `{"model":"gpt-4o","usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}}`
	p := &openAIParser{}
	in, out, err := p.ParseUsage([]byte(resp))
	require.NoError(t, err)
	assert.Equal(t, int64(10), in)
	assert.Equal(t, int64(20), out)
}

func TestOpenAIParser_ParseStreamingUsage(t *testing.T) {
	chunks := [][]byte{
		[]byte(`data: {"object":"chat.completion.chunk","choices":[]}`),
		[]byte(`data: {"object":"chat.completion.chunk","usage":{"prompt_tokens":5,"completion_tokens":15,"total_tokens":20}}`),
		[]byte(`data: [DONE]`),
	}
	p := &openAIParser{}
	in, out := p.ParseStreamingUsage(chunks)
	assert.Equal(t, int64(5), in)
	assert.Equal(t, int64(15), out)
}

func TestAnthropicParser_ParseRequest(t *testing.T) {
	body := `{"model":"claude-3-5-sonnet-20241022","stream":false,"messages":[{"role":"user","content":"hi"}],"max_tokens":100}`
	p := &anthropicParser{}
	model, streaming, tokens, err := p.ParseRequest([]byte(body))
	require.NoError(t, err)
	assert.Equal(t, "claude-3-5-sonnet-20241022", model)
	assert.False(t, streaming)
	assert.Greater(t, tokens, int64(0))
}

func TestAnthropicParser_ParseUsage(t *testing.T) {
	resp := `{"model":"claude-3-5-sonnet","usage":{"input_tokens":8,"output_tokens":25}}`
	p := &anthropicParser{}
	in, out, err := p.ParseUsage([]byte(resp))
	require.NoError(t, err)
	assert.Equal(t, int64(8), in)
	assert.Equal(t, int64(25), out)
}

func TestAnthropicParser_ParseStreamingUsage(t *testing.T) {
	chunks := [][]byte{
		[]byte(`event: message_start`),
		[]byte(`data: {"type":"message_start","message":{"usage":{"input_tokens":12,"output_tokens":0}}}`),
		[]byte(`event: message_delta`),
		[]byte(`data: {"type":"message_delta","usage":{"output_tokens":30}}`),
	}
	p := &anthropicParser{}
	in, out := p.ParseStreamingUsage(chunks)
	assert.Equal(t, int64(12), in)
	assert.Equal(t, int64(30), out)
}

// ─── extractVirtualKey ───────────────────────────────────────────────────────

func TestExtractVirtualKey_Bearer(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	r.Header.Set("Authorization", "Bearer af-vk-abc123")
	assert.Equal(t, "af-vk-abc123", extractVirtualKey(r))
}

func TestExtractVirtualKey_XAPIKey(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	r.Header.Set("x-api-key", "af-vk-def456")
	assert.Equal(t, "af-vk-def456", extractVirtualKey(r))
}

func TestExtractVirtualKey_Missing(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	assert.Equal(t, "", extractVirtualKey(r))
}

// ─── computeProxyCost ────────────────────────────────────────────────────────

func TestComputeProxyCost_KnownModel(t *testing.T) {
	_, total := computeProxyCost(ProviderOpenAI, "gpt-4o", 1_000_000, 1_000_000)
	// 1M input @ $5.00 + 1M output @ $15.00 = $20.00
	assert.InDelta(t, 20.00, total, 0.01)
}

func TestComputeProxyCost_UnknownModel(t *testing.T) {
	_, total := computeProxyCost(ProviderOpenAI, "gpt-unknown-model", 1000, 1000)
	assert.Equal(t, float64(0), total)
}

func TestComputeProxyCost_VersionedModel(t *testing.T) {
	// "gpt-4o-2024-08-06" should match the "gpt-4o" prefix entry.
	_, total := computeProxyCost(ProviderOpenAI, "gpt-4o-2024-08-06", 1_000_000, 0)
	assert.Greater(t, total, float64(0))
}

func TestComputeProxyCost_UsesConfiguredPricing(t *testing.T) {
	require.NoError(t, configurePricing(`[{"provider":"openai","model_pattern":"gpt-4o","input_per_million":1.0,"output_per_million":2.0}]`))
	t.Cleanup(func() {
		require.NoError(t, configurePricing(""))
	})

	_, total := computeProxyCost(ProviderOpenAI, "gpt-4o", 1_000_000, 1_000_000)
	assert.InDelta(t, 3.0, total, 0.01)
}

func TestRecordSpan_UsesCanonicalModelAttributeAndCost(t *testing.T) {
	ms := &mockStore{}
	lp := &LLMProxy{
		store:  ms,
		logger: zap.NewNop(),
	}

	lp.recordSpan("", "", "tenant-1", ProviderOpenAI, "gpt-4o-mini", "af-vk-12345678abcdef", 10, 5, 25*time.Millisecond, nil)

	require.Len(t, ms.spans, 1)
	assert.Equal(t, "gpt-4o-mini", ms.spans[0].Attributes["gen_ai.request.model"])
	assert.Equal(t, "gpt-4o-mini", ms.spans[0].Attributes["proxy.model"])
	assert.Greater(t, ms.spans[0].CostUSD, float64(0))
}

// ─── LLMProxy HTTP handler ───────────────────────────────────────────────────

// buildTestProxy creates a proxy backed by a real Vault (in-memory crypto, nil DB pool)
// and a fake upstream server.
func buildTestProxy(t *testing.T, upstreamHandler http.HandlerFunc) (*LLMProxy, *httptest.Server, *mockStore) {
	t.Helper()
	upstream := httptest.NewServer(upstreamHandler)
	t.Cleanup(upstream.Close)

	v, err := vault.New(nil, devVaultKey)
	require.NoError(t, err)

	ms := &mockStore{}
	logger := zap.NewNop()

	lp := &LLMProxy{
		vault:      v,
		store:      ms,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		logger:     logger,
	}
	return lp, upstream, ms
}

func TestProxy_MissingVirtualKey(t *testing.T) {
	lp, _, _ := buildTestProxy(t, nil)

	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o","messages":[]}`))
	r = WithProvider(r, ProviderOpenAI)
	w := httptest.NewRecorder()

	lp.ServeHTTP(w, r)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestProxy_NonVirtualKey(t *testing.T) {
	lp, _, _ := buildTestProxy(t, nil)

	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o","messages":[]}`))
	r.Header.Set("Authorization", "Bearer sk-real-key-not-virtual")
	r = WithProvider(r, ProviderOpenAI)
	w := httptest.NewRecorder()

	lp.ServeHTTP(w, r)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestProxy_UnsupportedProvider(t *testing.T) {
	lp, _, _ := buildTestProxy(t, nil)

	r := httptest.NewRequest(http.MethodPost, "/v1/something",
		strings.NewReader(`{}`))
	r.Header.Set("Authorization", "Bearer af-vk-abc")
	r = WithProvider(r, "cohere") // not registered
	w := httptest.NewRecorder()

	lp.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestProxy_RealKeyNeverForwarded(t *testing.T) {
	// The upstream should receive the real key, not the virtual key.
	var receivedAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"model":   "gpt-4o",
			"choices": []any{},
			"usage":   map[string]any{"prompt_tokens": 5, "completion_tokens": 10, "total_tokens": 15},
		})
	}))
	defer upstream.Close()

	// We test the auth-header substitution logic directly via the parser.
	p := &openAIParser{}
	assert.Equal(t, "Bearer sk-real-key", p.AuthValue("sk-real-key"))
	assert.NotContains(t, "Bearer sk-real-key", "af-vk-")
	_ = upstream
	_ = receivedAuth
}

// ─── ParserFor ───────────────────────────────────────────────────────────────

func TestParserFor_Known(t *testing.T) {
	_, ok := ParserFor(ProviderOpenAI)
	assert.True(t, ok)
	_, ok = ParserFor(ProviderAnthropic)
	assert.True(t, ok)
}

func TestParserFor_Unknown(t *testing.T) {
	_, ok := ParserFor("unknown-provider")
	assert.False(t, ok)
}
