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
	"github.com/agentfabric/api-gateway/internal/policy"
	priced "github.com/agentfabric/api-gateway/internal/pricing"
	"github.com/agentfabric/api-gateway/internal/vault"
	"go.uber.org/zap"
)

// spanStore is the minimal store interface the proxy needs to record spans.
type spanStore interface {
	BulkInsertSpans(ctx context.Context, spans []models.Span) error
	CreatePolicyAuditEntry(ctx context.Context, entry models.PolicyDecisionAudit) error
}

type eventBroadcaster interface {
	Broadcast(tenantID string, event interface{})
}

// ProviderParser abstracts the differences between LLM provider APIs.
// Exported so the netproxy package can reuse parsers without duplicating logic.
type ProviderParser interface {
	ParseRequest(r *http.Request, body []byte) (model string, streaming bool, estimatedTokens int64, err error)
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
	policyEngine   *policy.Engine
	broadcaster    eventBroadcaster
	httpClient     *http.Client
	logger         *zap.Logger
}

// ParserFor returns the ProviderParser for the given provider name.
// Exported for use by the netproxy package.
func ParserFor(provider string) (ProviderParser, bool) {
	return parserFor(provider)
}

// New creates an LLMProxy.
func New(v *vault.Vault, be *budget.BudgetEnforcer, store spanStore, policyEngine *policy.Engine, broadcaster eventBroadcaster, logger *zap.Logger) *LLMProxy {
	return &LLMProxy{
		vault:          v,
		budgetEnforcer: be,
		store:          store,
		policyEngine:   policyEngine,
		broadcaster:    broadcaster,
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
	provider := canonicalProviderFromRequest(r)
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
	resolvedProvider = NormalizeProvider(resolvedProvider)
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
	model, streaming, estimatedTokens, err := parser.ParseRequest(r, body)
	if err != nil {
		// Non-fatal: proceed without model info.
		p.logger.Debug("request parse failed", zap.Error(err))
	}
	traceID := newID()
	spanID := newID()
	environment := environmentFromRequest(r)

	if p.policyEngine != nil {
		trafficDecision := p.policyEngine.EvaluateTraffic(policy.TrafficInput{
			TenantID:        tenantID,
			Provider:        provider,
			Model:           model,
			Environment:     environment,
			EstimatedTokens: estimatedTokens,
			RequestHeaders:  policy.HeadersFromHTTP(r.Header),
			RequestBody:     body,
			App:             strings.TrimSpace(r.Header.Get("X-AF-App")),
			Session:         strings.TrimSpace(r.Header.Get("X-AF-Session")),
		})
		if trafficDecision.Matched {
			p.recordPolicyDecision(tenantID, traceID, spanID, provider, model, environment, "proxy", trafficDecision)
			if trafficDecision.Action == "deny" {
				p.recordSpan(traceID, spanID, tenantID, provider, model, vk, http.StatusForbidden, priced.Usage{}, 0, map[string]string{
					"af.policy.blocked":  "true",
					"af.policy.reason":   trafficDecision.Reason,
					"af.policy.decision": trafficDecision.Action,
					"af.error.type":      "policy_denied",
					"af.span.step_type":  "policy",
				})
				http.Error(w, `{"error":{"message":"request blocked by policy","type":"policy_denied"}}`, http.StatusForbidden)
				return
			}
		}

		dlpDecision := p.policyEngine.EvaluateDLP(policy.DLPInput{
			TenantID:       tenantID,
			Provider:       provider,
			Model:          model,
			Environment:    environment,
			Scope:          "request",
			Body:           body,
			RequestHeaders: policy.HeadersFromHTTP(r.Header),
			App:            strings.TrimSpace(r.Header.Get("X-AF-App")),
			Session:        strings.TrimSpace(r.Header.Get("X-AF-Session")),
		})
		if dlpDecision.Matched {
			p.recordPolicyDecision(tenantID, traceID, spanID, provider, model, environment, "proxy", dlpDecision)
			switch dlpDecision.Action {
			case "deny":
				p.recordSpan(traceID, spanID, tenantID, provider, model, vk, http.StatusForbidden, priced.Usage{}, 0, map[string]string{
					"af.policy.blocked":  "true",
					"af.policy.reason":   dlpDecision.Reason,
					"af.policy.decision": dlpDecision.Action,
					"af.error.type":      "policy_denied",
					"af.span.step_type":  "policy",
				})
				http.Error(w, `{"error":{"message":"request blocked by DLP policy","type":"policy_denied"}}`, http.StatusForbidden)
				return
			case "redact":
				body = dlpDecision.RedactedBody
			}
		}
	}

	// 4. Budget pre-check.
	if p.budgetEnforcer != nil {
		_, _, estimatedCostUSD := ComputeEstimatedCostForTenant(provider, model, tenantID, time.Now().UTC(), estimatedTokens)
		allowed, _ := p.budgetEnforcer.CheckAndRecord(r.Context(), tenantID, estimatedTokens, estimatedCostUSD)
		if !allowed {
			go p.recordSpan(traceID, spanID, tenantID, provider, model, vk, http.StatusTooManyRequests, priced.Usage{}, 0, map[string]string{
				"af.error.type":     "budget_exceeded",
				"af.policy.blocked": "true",
				"af.span.step_type": "policy",
			})
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
		go p.recordSpan(traceID, spanID, tenantID, provider, model, vk, http.StatusBadGateway, priced.Usage{}, time.Since(start), map[string]string{
			"af.error.type":     "upstream_request_failed",
			"af.error.message":  err.Error(),
			"af.span.step_type": "llm",
		})
		http.Error(w, "upstream request failed", http.StatusBadGateway)
		return
	}
	defer upResp.Body.Close()

	usage := priced.Usage{}
	extraAttrs := map[string]string{}

	if streaming && upResp.StatusCode == http.StatusOK {
		for k, vv := range upResp.Header {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(upResp.StatusCode)
		usage = p.handleStreaming(w, upResp, provider, parser)
	} else {
		respBody, _ := io.ReadAll(upResp.Body)
		if upResp.StatusCode == http.StatusOK {
			usage, _ = ParseDetailedUsage(provider, respBody)
			if usage.TotalTokens() == 0 {
				usage.InputTokens, usage.OutputTokens, _ = parser.ParseUsage(respBody)
			}
			if p.policyEngine != nil {
				dlpDecision := p.policyEngine.EvaluateDLP(policy.DLPInput{
					TenantID:       tenantID,
					Provider:       provider,
					Model:          model,
					Environment:    environment,
					Scope:          "response",
					Body:           respBody,
					RequestHeaders: policy.HeadersFromHTTP(r.Header),
					App:            strings.TrimSpace(r.Header.Get("X-AF-App")),
					Session:        strings.TrimSpace(r.Header.Get("X-AF-Session")),
				})
				if dlpDecision.Matched {
					p.recordPolicyDecision(tenantID, traceID, spanID, provider, model, environment, "proxy", dlpDecision)
					extraAttrs["af.policy.reason"] = dlpDecision.Reason
					extraAttrs["af.policy.decision"] = dlpDecision.Action
					if dlpDecision.Action == "deny" {
						extraAttrs["af.policy.blocked"] = "true"
						respBody = []byte(`{"error":{"message":"response blocked by DLP policy","type":"policy_denied"}}`)
						upResp.StatusCode = http.StatusForbidden
					} else if dlpDecision.Action == "redact" {
						respBody = dlpDecision.RedactedBody
						extraAttrs["af.policy.redacted"] = "true"
					}
				}
			}
		}
		for k, vv := range upResp.Header {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.Header().Del("Content-Length")
		w.WriteHeader(upResp.StatusCode)
		w.Write(respBody) //nolint:errcheck
	}

	duration := time.Since(start)

	// 7. Record span (non-blocking, best-effort).
	if upResp.StatusCode >= 400 {
		extraAttrs["af.error.type"] = classifyProxyStatus(upResp.StatusCode)
	}
	go p.recordSpan(traceID, spanID, tenantID, provider, model, vk, upResp.StatusCode, usage, duration, extraAttrs)
}

// handleStreaming forwards SSE chunks as they arrive, buffering them for usage parsing.
func (p *LLMProxy) handleStreaming(w http.ResponseWriter, resp *http.Response, provider string, parser ProviderParser) priced.Usage {
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

	return ParseDetailedStreamingUsage(provider, chunks, parser)
}

// recordSpan writes a single proxy span to PostgreSQL.
func (p *LLMProxy) recordSpan(traceID, spanID, tenantID, provider, model, vkPrefix string, statusCode int, usage priced.Usage, duration time.Duration, extraAttrs map[string]string) {
	now := time.Now()
	if traceID == "" {
		traceID = newID()
	}
	if spanID == "" {
		spanID = newID()
	}

	// Mask the virtual key: show only the first 14 chars (af-vk- + 8 chars).
	maskedVK := vkPrefix
	if len(maskedVK) > 14 {
		maskedVK = maskedVK[:14] + "..."
	}

	match, costResult := ComputeDetailedCostForTenant(provider, model, tenantID, now, usage)

	span := models.Span{
		ID:                spanID,
		TraceID:           traceID,
		RunID:             traceID,
		Name:              fmt.Sprintf("proxy.%s.%s", provider, model),
		Framework:         "proxy",
		StartTimeNs:       now.Add(-duration).UnixNano(),
		DurationNs:        duration.Nanoseconds(),
		StatusCode:        statusCode,
		InputTokens:       usage.InputTokens,
		OutputTokens:      usage.OutputTokens,
		CacheReadTokens:   usage.CacheReadTokens,
		CacheWriteTokens:  usage.CacheWriteTokens,
		ReasoningTokens:   usage.ReasoningTokens,
		CostUSD:           costResult.TotalCostUSD,
		InputCostUSD:      costResult.InputCostUSD,
		OutputCostUSD:     costResult.OutputCostUSD,
		CacheReadCostUSD:  costResult.CacheReadCostUSD,
		CacheWriteCostUSD: costResult.CacheWriteCostUSD,
		ReasoningCostUSD:  costResult.ReasoningCostUSD,
		TenantID:          tenantID,
		ReceivedAt:        now,
		Attributes: map[string]string{
			"proxy.provider":       provider,
			"proxy.model":          model,
			"proxy.virtual_key":    maskedVK,
			"af.span.step_type":    "llm",
			"gen_ai.system":        provider,
			"gen_ai.request.model": model,
		},
	}
	for key, value := range extraAttrs {
		if strings.TrimSpace(value) != "" {
			span.Attributes[key] = value
		}
	}
	if match.RuleID > 0 {
		span.Attributes["af.pricing.rule_id"] = fmt.Sprintf("%d", match.RuleID)
		span.Attributes["af.pricing.model_pattern"] = match.ModelPattern
		span.Attributes["af.pricing.scope"] = match.Scope
		span.Attributes["af.pricing.cache_read_per_million"] = fmt.Sprintf("%.6f", match.CacheReadPerMillion)
		span.Attributes["af.pricing.cache_write_per_million"] = fmt.Sprintf("%.6f", match.CacheWritePerMillion)
		span.Attributes["af.pricing.reasoning_per_million"] = fmt.Sprintf("%.6f", match.ReasoningPerMillion)
	}
	if usage.CacheReadTokens > 0 {
		span.Attributes["gen_ai.usage.cache_read_tokens"] = fmt.Sprintf("%d", usage.CacheReadTokens)
	}
	if usage.CacheWriteTokens > 0 {
		span.Attributes["gen_ai.usage.cache_write_tokens"] = fmt.Sprintf("%d", usage.CacheWriteTokens)
	}
	if usage.ReasoningTokens > 0 {
		span.Attributes["gen_ai.usage.reasoning_tokens"] = fmt.Sprintf("%d", usage.ReasoningTokens)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := p.store.BulkInsertSpans(ctx, []models.Span{span}); err != nil {
		p.logger.Warn("failed to record proxy span", zap.Error(err))
	}
}

func classifyProxyStatus(statusCode int) string {
	switch {
	case statusCode == http.StatusForbidden:
		return "policy_denied"
	case statusCode == http.StatusTooManyRequests:
		return "budget_exceeded"
	case statusCode >= 500:
		return "upstream_error"
	case statusCode >= 400:
		return "client_error"
	default:
		return ""
	}
}

func (p *LLMProxy) recordPolicyDecision(tenantID, traceID, spanID, provider, model, environment, framework string, decision policy.Decision) {
	if !decision.Matched {
		return
	}
	tenantID = normalizeTenantID(tenantID)
	result := decision.Action
	if result == "redact" {
		result = "sanitize"
	}
	decisionID := newUUID()
	now := time.Now().UTC()
	if err := p.store.CreatePolicyAuditEntry(context.Background(), models.PolicyDecisionAudit{
		DecisionID:  decisionID,
		TraceID:     traceID,
		SpanID:      spanID,
		PolicyName:  decision.PolicyName,
		Result:      result,
		Reason:      decision.Reason,
		TenantID:    tenantID,
		EvaluatedAt: now,
		Framework:   framework,
		Model:       model,
		Environment: environment,
	}); err != nil {
		p.logger.Warn("failed to write policy audit entry", zap.Error(err))
	}
	if p.broadcaster != nil {
		p.broadcaster.Broadcast(tenantID, &models.LiveEvent{
			Type:      "policy",
			Timestamp: now.UnixMilli(),
			TenantID:  tenantID,
			Data: models.PolicyEvent{
				DecisionID: decisionID,
				TraceID:    traceID,
				SpanID:     spanID,
				PolicyName: decision.PolicyName,
				Result:     result,
				Reason:     decision.Reason,
				TenantID:   tenantID,
				Provider:   provider,
				Model:      model,
				Scope:      decision.Scope,
				Matched:    decision.MatchedNames,
				Redactions: decision.Redactions,
			},
		})
	}
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

type ctxKeyProvider struct{}

// WithProvider stores the provider string in the request context.
// Called by the router before handing off to ServeHTTP.
func WithProvider(r *http.Request, provider string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), ctxKeyProvider{}, NormalizeProvider(provider)))
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func environmentFromRequest(r *http.Request) string {
	if r == nil {
		return "unknown"
	}
	for _, value := range []string{r.Header.Get("X-AF-Environment"), r.Header.Get("X-Environment")} {
		if trimmed := strings.ToLower(strings.TrimSpace(value)); trimmed != "" {
			return trimmed
		}
	}
	return "unknown"
}

func normalizeTenantID(tenantID string) string {
	if strings.TrimSpace(tenantID) == "" {
		return "00000000-0000-0000-0000-000000000001"
	}
	return tenantID
}

func newUUID() string {
	b := make([]byte, 16)
	rand.Read(b) //nolint:errcheck
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// EncodeJSON is a small helper used by the key handler.
func EncodeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}
