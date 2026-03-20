// Package proxy implements the LLM reverse proxy for Layer 2 virtual keys.
//
// Developers point their SDK at AgentFabric:
//
//	OPENAI_BASE_URL=http://localhost:8080/proxy/openai/v1
//	OPENAI_API_KEY=af-vk-<token>
//
// The proxy resolves the virtual key, checks the tenant budget, forwards the
// request to the real upstream with the real key substituted, records a span,
// and streams the response back unchanged.
package proxy

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/agentfabric/api-gateway/internal/budget"
	"github.com/agentfabric/api-gateway/internal/models"
	"github.com/agentfabric/api-gateway/internal/vault"
	"go.uber.org/zap"
)

// provider names as stored in the virtual_keys table.
const (
	ProviderOpenAI    = "openai"
	ProviderAnthropic = "anthropic"
)

// spanStore is the minimal store interface the proxy needs to record spans.
type spanStore interface {
	BulkInsertSpans(ctx context.Context, spans []models.Span) error
}

// providerParser abstracts the differences between LLM provider APIs.
type providerParser interface {
	ParseRequest(body []byte) (model string, streaming bool, estimatedTokens int64, err error)
	ParseUsage(body []byte) (inputTokens, outputTokens int64, err error)
	ParseStreamingUsage(chunks [][]byte) (inputTokens, outputTokens int64)
	Upstream() string
	AuthHeader() string
	AuthValue(realKey string) string
}

// LLMProxy is an http.Handler that mounts at /proxy/{provider}/v1/*.
type LLMProxy struct {
	vault          *vault.Vault
	budgetEnforcer *budget.BudgetEnforcer // may be nil (budget disabled)
	store          spanStore
	httpClient     *http.Client
	logger         *zap.Logger
}

// New creates an LLMProxy.
func New(v *vault.Vault, be *budget.BudgetEnforcer, store spanStore, logger *zap.Logger) *LLMProxy {
	return &LLMProxy{
		vault:          v,
		budgetEnforcer: be,
		store:          store,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute, // generous for long streaming responses
		},
		logger: logger,
	}
}

// ServeHTTP implements http.Handler.
//
// URL layout expected by the router:
//
//	/proxy/openai/v1/chat/completions
//	/proxy/anthropic/v1/messages
//
// The router strips /proxy/{provider} before calling ServeHTTP, leaving the
// path that the upstream API expects (e.g. /v1/chat/completions).
func (p *LLMProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	provider := r.Context().Value(ctxKeyProvider{}).(string)
	parser, ok := parserFor(provider)
	if !ok {
		http.Error(w, "unsupported provider: "+provider, http.StatusBadRequest)
		return
	}

	// 1. Extract virtual key from Authorization or x-api-key.
	vk := extractVirtualKey(r)
	if vk == "" || !strings.HasPrefix(vk, "af-vk-") {
		http.Error(w, "missing or invalid virtual key", http.StatusUnauthorized)
		return
	}

	// 2. Resolve virtual key → real key + tenant.
	resolvedProvider, realKey, tenantID, err := p.vault.Resolve(r.Context(), vk)
	if err != nil {
		p.logger.Debug("virtual key resolve failed", zap.String("vk_prefix", vk[:min(14, len(vk))]), zap.Error(err))
		http.Error(w, "invalid or revoked key", http.StatusUnauthorized)
		return
	}
	if resolvedProvider != provider {
		http.Error(w, fmt.Sprintf("key is for provider %q, not %q", resolvedProvider, provider), http.StatusUnauthorized)
		return
	}

	// 3. Buffer and parse the request body.
	body, err := io.ReadAll(io.LimitReader(r.Body, 8*1024*1024)) // 8 MB cap
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	model, streaming, estimatedTokens, err := parser.ParseRequest(body)
	if err != nil {
		// Non-fatal: proceed without model info.
		p.logger.Debug("request parse failed", zap.Error(err))
	}

	// 4. Budget pre-check.
	if p.budgetEnforcer != nil {
		allowed, _ := p.budgetEnforcer.CheckAndRecord(r.Context(), tenantID, estimatedTokens, 0)
		if !allowed {
			http.Error(w, `{"error":{"message":"token budget exceeded","type":"budget_exceeded"}}`,
				http.StatusTooManyRequests)
			return
		}
	}

	// 5. Build the upstream request.
	upstreamURL := parser.Upstream() + r.URL.Path
	if r.URL.RawQuery != "" {
		upstreamURL += "?" + r.URL.RawQuery
	}

	upReq, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL, bytes.NewReader(body))
	if err != nil {
		http.Error(w, "failed to build upstream request", http.StatusInternalServerError)
		return
	}

	// Copy all original headers, then overwrite the auth header with the real key.
	for k, vv := range r.Header {
		for _, v := range vv {
			upReq.Header.Add(k, v)
		}
	}
	// Remove any virtual key from Authorization before forwarding.
	upReq.Header.Del("Authorization")
	upReq.Header.Del("x-api-key")
	upReq.Header.Set(parser.AuthHeader(), parser.AuthValue(realKey))
	upReq.Header.Set("Content-Type", "application/json")

	start := time.Now()

	// 6. Forward and handle the response.
	upResp, err := p.httpClient.Do(upReq)
	if err != nil {
		p.logger.Warn("upstream request failed", zap.Error(err))
		http.Error(w, "upstream request failed", http.StatusBadGateway)
		return
	}
	defer upResp.Body.Close()

	// Copy response headers to the client.
	for k, vv := range upResp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(upResp.StatusCode)

	var inputTokens, outputTokens int64

	if streaming && upResp.StatusCode == http.StatusOK {
		inputTokens, outputTokens = p.handleStreaming(w, upResp, parser)
	} else {
		respBody, _ := io.ReadAll(upResp.Body)
		w.Write(respBody) //nolint:errcheck
		if upResp.StatusCode == http.StatusOK {
			inputTokens, outputTokens, _ = parser.ParseUsage(respBody)
		}
	}

	duration := time.Since(start)

	// 7. Record span (non-blocking, best-effort).
	if upResp.StatusCode == http.StatusOK {
		go p.recordSpan(tenantID, provider, model, vk, inputTokens, outputTokens, duration)
	}
}

// handleStreaming forwards SSE chunks as they arrive, buffering them for usage parsing.
func (p *LLMProxy) handleStreaming(w http.ResponseWriter, resp *http.Response, parser providerParser) (inputTokens, outputTokens int64) {
	flusher, canFlush := w.(http.Flusher)

	var chunks [][]byte
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 64*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		chunk := make([]byte, len(line))
		copy(chunk, line)
		chunks = append(chunks, chunk)

		_, err := fmt.Fprintf(w, "%s\n", line)
		if err != nil {
			break
		}
		if canFlush {
			flusher.Flush()
		}
	}
	// Trailing newline to close the SSE stream.
	fmt.Fprintln(w, "")
	if canFlush {
		flusher.Flush()
	}

	return parser.ParseStreamingUsage(chunks)
}

// recordSpan writes a single proxy span to PostgreSQL.
func (p *LLMProxy) recordSpan(tenantID, provider, model, vkPrefix string, inputTokens, outputTokens int64, duration time.Duration) {
	now := time.Now()
	traceID := newID()
	spanID := newID()

	// Mask the virtual key: show only the first 14 chars (af-vk- + 8 chars).
	maskedVK := vkPrefix
	if len(maskedVK) > 14 {
		maskedVK = maskedVK[:14] + "..."
	}

	_, costUSD := computeProxyCost(provider, model, inputTokens, outputTokens)

	span := models.Span{
		ID:           spanID,
		TraceID:      traceID,
		RunID:        traceID,
		Name:         fmt.Sprintf("proxy.%s.%s", provider, model),
		Framework:    "proxy",
		StartTimeNs:  now.Add(-duration).UnixNano(),
		DurationNs:   duration.Nanoseconds(),
		StatusCode:   200,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		CostUSD:      costUSD,
		TenantID:     tenantID,
		ReceivedAt:   now,
		Attributes: map[string]string{
			"proxy.provider":    provider,
			"proxy.model":       model,
			"proxy.virtual_key": maskedVK,
			"gen_ai.system":     provider,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := p.store.BulkInsertSpans(ctx, []models.Span{span}); err != nil {
		p.logger.Warn("failed to record proxy span", zap.Error(err))
	}
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

type ctxKeyProvider struct{}

// WithProvider stores the provider string in the request context.
// Called by the router before handing off to ServeHTTP.
func WithProvider(r *http.Request, provider string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), ctxKeyProvider{}, provider))
}

func parserFor(provider string) (providerParser, bool) {
	switch provider {
	case ProviderOpenAI:
		return &openAIParser{}, true
	case ProviderAnthropic:
		return &anthropicParser{}, true
	}
	return nil, false
}

// extractVirtualKey pulls the virtual key from Authorization: Bearer or x-api-key.
func extractVirtualKey(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); auth != "" {
		if after, ok := strings.CutPrefix(auth, "Bearer "); ok {
			return strings.TrimSpace(after)
		}
	}
	return strings.TrimSpace(r.Header.Get("x-api-key"))
}

func newID() string {
	b := make([]byte, 16)
	rand.Read(b) //nolint:errcheck
	return hex.EncodeToString(b)
}

// computeProxyCost returns (inputCostUSD, totalCostUSD) using built-in pricing.
// Only covers the most common models; unknown models return 0.
func computeProxyCost(provider, model string, inputTokens, outputTokens int64) (float64, float64) {
	type pricing struct{ in, out float64 } // per million tokens
	table := map[string]pricing{
		"gpt-4o":                {2.50, 10.00},
		"gpt-4o-mini":           {0.15, 0.60},
		"gpt-4-turbo":           {10.00, 30.00},
		"gpt-3.5-turbo":         {0.50, 1.50},
		"claude-3-5-sonnet":     {3.00, 15.00},
		"claude-3-5-haiku":      {0.80, 4.00},
		"claude-3-opus":         {15.00, 75.00},
	}
	p, ok := table[model]
	if !ok {
		// Prefix match for versioned model names.
		for k, v := range table {
			if strings.HasPrefix(model, k) {
				p = v
				ok = true
				break
			}
		}
	}
	if !ok {
		return 0, 0
	}
	inCost := float64(inputTokens) / 1_000_000 * p.in
	outCost := float64(outputTokens) / 1_000_000 * p.out
	return inCost, inCost + outCost
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// EncodeJSON is a small helper used by the key handler.
func EncodeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}
