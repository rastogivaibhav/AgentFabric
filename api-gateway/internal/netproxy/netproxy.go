package netproxy

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/agentfabric/api-gateway/internal/budget"
	"github.com/agentfabric/api-gateway/internal/models"
	"github.com/agentfabric/api-gateway/internal/policy"
	priced "github.com/agentfabric/api-gateway/internal/pricing"
	"github.com/agentfabric/api-gateway/internal/proxy"
	"github.com/agentfabric/api-gateway/internal/vault"
	"go.uber.org/zap"
)

// knownLLMHosts maps bare hostname → provider name for TLS interception.
// Any CONNECT to one of these hosts will be MITM'd; all other hosts are
// tunnelled blindly without inspection.
var knownLLMHosts = map[string]string{
	"api.openai.com":                    proxy.ProviderOpenAI,
	"api.anthropic.com":                 proxy.ProviderAnthropic,
	"generativelanguage.googleapis.com": "gemini", // parser stub — forwards without recording
}

// spanStore is the minimal interface the netproxy needs to record spans.
type spanStore interface {
	BulkInsertSpans(ctx context.Context, spans []models.Span) error
	CreatePolicyAuditEntry(ctx context.Context, entry models.PolicyDecisionAudit) error
}

type eventBroadcaster interface {
	Broadcast(tenantID string, event interface{})
}

// NetProxy is an http.Handler that operates as an HTTP CONNECT forward proxy.
// Mount it on a plain (non-TLS) net.Listener — clients use HTTP CONNECT
// tunnelling so the inbound side never needs to be HTTPS.
//
// Usage: set HTTP_PROXY=http://localhost:8443 and HTTPS_PROXY=http://localhost:8443
// in the calling process.  Install the CA cert from GET /api/v1/netproxy/ca.crt
// into your OS trust store first.
type NetProxy struct {
	ca             *CA
	vault          *vault.Vault
	budgetEnforcer *budget.BudgetEnforcer // nil = budget disabled
	store          spanStore
	policyEngine   *policy.Engine
	broadcaster    eventBroadcaster
	httpClient     *http.Client // dials the REAL upstream with system TLS roots
	logger         *zap.Logger
}

// New creates a NetProxy. ca must be non-nil.
func New(ca *CA, v *vault.Vault, be *budget.BudgetEnforcer, store spanStore, policyEngine *policy.Engine, broadcaster eventBroadcaster, logger *zap.Logger) *NetProxy {
	return &NetProxy{
		ca:             ca,
		vault:          v,
		budgetEnforcer: be,
		store:          store,
		policyEngine:   policyEngine,
		broadcaster:    broadcaster,
		// httpClient uses the default system root pool — it verifies the REAL
		// upstream TLS cert, not our local CA.
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
			Transport: &http.Transport{
				TLSClientConfig:   &tls.Config{},
				ForceAttemptHTTP2: false,
				// Generous timeouts for long streaming LLM responses.
				ResponseHeaderTimeout: 120 * time.Second,
			},
		},
		logger: logger,
	}
}

// ServeHTTP implements http.Handler.  Only CONNECT requests are supported;
// plain HTTP proxy requests are rejected (LLM APIs are always HTTPS).
func (p *NetProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodConnect {
		http.Error(w, `{"error":"only CONNECT tunnelling supported"}`, http.StatusMethodNotAllowed)
		return
	}
	p.handleConnect(w, r)
}

// ─── CONNECT handling ─────────────────────────────────────────────────────────

// handleConnect hijacks the raw TCP connection, sends 200 Connection Established,
// then either MITMs (known LLM hosts) or passes through (everything else).
func (p *NetProxy) handleConnect(w http.ResponseWriter, r *http.Request) {
	host := r.Host // "api.openai.com:443"
	hostname := stripPort(host)
	provider, isLLM := proxy.ProviderForHost(hostname)

	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		return
	}
	conn, _, err := hj.Hijack()
	if err != nil {
		p.logger.Warn("netproxy: hijack failed", zap.String("host", host), zap.Error(err))
		return
	}

	// Respond 200 before anything else — the client starts TLS as soon as it
	// sees this, so we must not write another byte over conn before the MITM
	// branch completes its own TLS handshake.
	fmt.Fprintf(conn, "HTTP/1.1 200 Connection Established\r\nProxy-Agent: AgentFabric\r\n\r\n") //nolint:errcheck

	if isLLM {
		p.mitmConnect(conn, host, hostname, provider)
	} else {
		p.passthroughConnect(conn, host)
	}
}

// mitmConnect performs TLS termination + request inspection for LLM APIs.
func (p *NetProxy) mitmConnect(clientConn net.Conn, host, hostname, provider string) {
	defer clientConn.Close()

	tlsCfg, err := p.ca.TLSConfigFor(hostname)
	if err != nil {
		p.logger.Warn("netproxy: leaf cert failed", zap.String("host", hostname), zap.Error(err))
		return
	}

	// TLS server handshake: we present our forged leaf cert to the client.
	// This succeeds only if the client trusts our CA (installed via install-ca-cert.sh).
	tlsConn := tls.Server(clientConn, tlsCfg)
	if err := tlsConn.Handshake(); err != nil {
		p.logger.Debug("netproxy: TLS handshake failed (is CA installed?)",
			zap.String("host", hostname), zap.Error(err))
		return
	}
	defer tlsConn.Close()

	// Wrap the TLS conn in a singleConnListener so we can use http.Server.Serve
	// and get a proper http.ResponseWriter (with Flush support for SSE).
	ln := newSingleConnListener(tlsConn)
	defer ln.Close()

	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Fix up the URL so the upstream dial works correctly.
			r.URL.Scheme = "https"
			r.URL.Host = host
			r.RequestURI = ""
			p.handleIntercepted(w, r, host, provider)
		}),
		// No read/write timeout here — streaming responses can be arbitrarily long.
	}
	srv.Serve(ln) //nolint:errcheck
}

// passthroughConnect blindly tunnels a CONNECT for non-LLM hosts.
func (p *NetProxy) passthroughConnect(clientConn net.Conn, host string) {
	defer clientConn.Close()

	upstreamConn, err := net.DialTimeout("tcp", host, 30*time.Second)
	if err != nil {
		p.logger.Debug("netproxy: passthrough dial failed", zap.String("host", host), zap.Error(err))
		return
	}
	defer upstreamConn.Close()

	// Bidirectional pipe — copy until either side closes.
	done := make(chan struct{}, 2)
	go func() { io.Copy(upstreamConn, clientConn); done <- struct{}{} }() //nolint:errcheck
	go func() { io.Copy(clientConn, upstreamConn); done <- struct{}{} }() //nolint:errcheck
	<-done
}

// ─── Interception logic ───────────────────────────────────────────────────────

// handleIntercepted is called after TLS termination with a proper http.Request.
// It applies vault key substitution, budget enforcement, and span recording.
func (p *NetProxy) handleIntercepted(w http.ResponseWriter, r *http.Request, host, provider string) {
	provider = proxy.NormalizeProvider(provider)
	// 1. Get a parser for this provider (Gemini has no parser yet → passthrough).
	parser, hasParser := proxy.ParserFor(provider)
	if !hasParser {
		p.logger.Warn("netproxy: intercepted provider missing parser", zap.String("provider", provider), zap.String("host", host))
		http.Error(w, "unsupported provider", http.StatusBadGateway)
		return
	}

	// 2. Resolve auth — three cases:
	//    a) af-vk-* virtual key → vault resolves to real key + tenant
	//    b) raw real key (sk-...) → pass through as-is; still record span
	//    c) no key → forward without modification; no span
	rawKey := extractKey(r)
	tenantID := ""

	if strings.HasPrefix(rawKey, "af-vk-") {
		resolvedProvider, realKey, tid, err := p.vault.Resolve(r.Context(), rawKey)
		if err != nil {
			http.Error(w, "invalid or revoked virtual key", http.StatusUnauthorized)
			return
		}
		tenantID = tid
		_ = resolvedProvider // provider already known from hostname
		r.Header.Del("Authorization")
		r.Header.Del("x-api-key")
		r.Header.Set(parser.AuthHeader(), parser.AuthValue(realKey))
	}

	// 3. Buffer request body for token estimation + re-sending.
	body, err := io.ReadAll(io.LimitReader(r.Body, 8*1024*1024))
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	r.Body.Close()

	model, streaming, estimatedTokens, _ := parser.ParseRequest(r, body)
	traceID := newID()
	spanID := newID()
	environment := proxyEnvironmentFromRequest(r)

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
			p.recordPolicyDecision(tenantID, traceID, spanID, provider, model, environment, "netproxy", trafficDecision)
			if trafficDecision.Action == "deny" {
				p.recordSpan(traceID, spanID, tenantID, provider, model, rawKey, http.StatusForbidden, priced.Usage{}, 0, map[string]string{
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
			p.recordPolicyDecision(tenantID, traceID, spanID, provider, model, environment, "netproxy", dlpDecision)
			switch dlpDecision.Action {
			case "deny":
				p.recordSpan(traceID, spanID, tenantID, provider, model, rawKey, http.StatusForbidden, priced.Usage{}, 0, map[string]string{
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

	// 4. Budget pre-check (only when tenant is known from vault).
	if p.budgetEnforcer != nil && tenantID != "" {
		_, _, estimatedCostUSD := proxy.ComputeEstimatedCostForTenant(provider, model, tenantID, time.Now().UTC(), estimatedTokens)
		allowed, _ := p.budgetEnforcer.CheckAndRecord(r.Context(), tenantID, estimatedTokens, estimatedCostUSD)
		if !allowed {
			go p.recordSpan(traceID, spanID, tenantID, provider, model, rawKey, http.StatusTooManyRequests, priced.Usage{}, 0, map[string]string{
				"af.error.type":     "budget_exceeded",
				"af.policy.blocked": "true",
				"af.span.step_type": "policy",
			})
			http.Error(w,
				`{"error":{"message":"token budget exceeded","type":"budget_exceeded"}}`,
				http.StatusTooManyRequests)
			return
		}
	}

	// 5. Build upstream request.
	upstreamURL := "https://" + host + r.URL.RequestURI()
	upReq, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL, bytes.NewReader(body))
	if err != nil {
		http.Error(w, "failed to build upstream request", http.StatusInternalServerError)
		return
	}

	// Copy headers, strip hop-by-hop, set correct Content-Length.
	copyHeaders(upReq.Header, r.Header)
	removeHopByHopHeaders(upReq.Header)
	upReq.ContentLength = int64(len(body))

	start := time.Now()

	// 6. Forward to real upstream.
	upResp, err := p.httpClient.Do(upReq)
	if err != nil {
		p.logger.Warn("netproxy: upstream request failed",
			zap.String("host", host), zap.Error(err))
		go p.recordSpan(traceID, spanID, tenantID, provider, model, rawKey, http.StatusBadGateway, priced.Usage{}, time.Since(start), map[string]string{
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
		usage = forwardStreaming(w, upResp, provider, parser)
	} else {
		respBody, _ := io.ReadAll(upResp.Body)
		if upResp.StatusCode == http.StatusOK {
			usage, _ = proxy.ParseDetailedUsage(provider, respBody)
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
					p.recordPolicyDecision(tenantID, traceID, spanID, provider, model, environment, "netproxy", dlpDecision)
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

	// 8. Record span (best-effort, non-blocking) for all keyed requests.
	if rawKey != "" {
		if upResp.StatusCode >= 400 {
			extraAttrs["af.error.type"] = classifyNetproxyStatus(upResp.StatusCode)
		}
		go p.recordSpan(traceID, spanID, tenantID, provider, model, rawKey, upResp.StatusCode, usage, duration, extraAttrs)
	}
}

// forwardRaw is retained as an emergency passthrough helper for unexpected hosts.
func (p *NetProxy) forwardRaw(w http.ResponseWriter, r *http.Request, host string) {
	upstreamURL := "https://" + host + r.URL.RequestURI()
	upReq, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL, r.Body)
	if err != nil {
		http.Error(w, "forward failed", http.StatusInternalServerError)
		return
	}
	copyHeaders(upReq.Header, r.Header)
	removeHopByHopHeaders(upReq.Header)

	resp, err := p.httpClient.Do(upReq)
	if err != nil {
		http.Error(w, "upstream failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body) //nolint:errcheck
}

// ─── Streaming ────────────────────────────────────────────────────────────────

// forwardStreaming forwards SSE chunks as they arrive, buffering for usage parsing.
// Mirrors proxy.LLMProxy.handleStreaming.
func forwardStreaming(w http.ResponseWriter, resp *http.Response, provider string, parser proxy.ProviderParser) priced.Usage {
	flusher, canFlush := w.(http.Flusher)

	var chunks [][]byte
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 64*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		chunk := make([]byte, len(line))
		copy(chunk, line)
		chunks = append(chunks, chunk)

		fmt.Fprintf(w, "%s\n", line) //nolint:errcheck
		if canFlush {
			flusher.Flush()
		}
	}
	fmt.Fprintln(w, "")
	if canFlush {
		flusher.Flush()
	}

	return proxy.ParseDetailedStreamingUsage(provider, chunks, parser)
}

// ─── Span recording ───────────────────────────────────────────────────────────

func (p *NetProxy) recordSpan(traceID, spanID, tenantID, provider, model, rawKey string, statusCode int, usage priced.Usage, duration time.Duration, extraAttrs map[string]string) {
	now := time.Now()
	if traceID == "" {
		traceID = newID()
	}
	if spanID == "" {
		spanID = newID()
	}

	maskedKey := rawKey
	if len(maskedKey) > 14 {
		maskedKey = maskedKey[:14] + "..."
	}

	match, costResult := proxy.ComputeDetailedCostForTenant(provider, model, tenantID, now, usage)

	span := models.Span{
		ID:                spanID,
		TraceID:           traceID,
		RunID:             traceID,
		Name:              fmt.Sprintf("netproxy.%s.%s", provider, model),
		Framework:         "netproxy",
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
			"netproxy.provider":    provider,
			"netproxy.model":       model,
			"netproxy.key":         maskedKey,
			"netproxy.layer":       "3",
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
	if tenantID == "" {
		span.TenantID = "00000000-0000-0000-0000-000000000001" // default tenant
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := p.store.BulkInsertSpans(ctx, []models.Span{span}); err != nil {
		p.logger.Warn("netproxy: failed to record span", zap.Error(err))
	}
}

func classifyNetproxyStatus(statusCode int) string {
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

func (p *NetProxy) recordPolicyDecision(tenantID, traceID, spanID, provider, model, environment, framework string, decision policy.Decision) {
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
		p.logger.Warn("netproxy: failed to write policy audit entry", zap.Error(err))
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

// ─── singleConnListener ───────────────────────────────────────────────────────

// singleConnListener wraps a single net.Conn as a net.Listener.
// http.Server.Serve calls Accept() once to get the conn, then processes
// HTTP requests on it.  The next Accept() call returns an error, causing
// Serve to exit cleanly.
type singleConnListener struct {
	conn net.Conn
	once sync.Once
	ch   chan net.Conn
}

func newSingleConnListener(conn net.Conn) *singleConnListener {
	l := &singleConnListener{conn: conn, ch: make(chan net.Conn, 1)}
	l.ch <- conn
	return l
}

func (l *singleConnListener) Accept() (net.Conn, error) {
	conn, ok := <-l.ch
	if !ok {
		return nil, fmt.Errorf("singleConnListener: closed")
	}
	return conn, nil
}

func (l *singleConnListener) Close() error {
	l.once.Do(func() { close(l.ch) })
	return nil
}

func (l *singleConnListener) Addr() net.Addr {
	return l.conn.LocalAddr()
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// extractKey reads the virtual or real key from Authorization: Bearer or x-api-key.
func extractKey(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); auth != "" {
		if after, ok := strings.CutPrefix(auth, "Bearer "); ok {
			return strings.TrimSpace(after)
		}
	}
	return strings.TrimSpace(r.Header.Get("x-api-key"))
}

// stripPort removes the ":443" suffix from a hostport string.
func stripPort(hostport string) string {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		return hostport
	}
	return host
}

func proxyEnvironmentFromRequest(r *http.Request) string {
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

// copyHeaders copies all headers from src to dst.
func copyHeaders(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

// hopByHopHeaders lists headers that must not be forwarded to the upstream.
var hopByHopHeaders = []string{
	"Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization",
	"Proxy-Connection", "TE", "Trailers", "Transfer-Encoding", "Upgrade",
}

// removeHopByHopHeaders strips hop-by-hop headers before forwarding.
func removeHopByHopHeaders(h http.Header) {
	for _, name := range hopByHopHeaders {
		h.Del(name)
	}
}

func newID() string {
	b := make([]byte, 16)
	rand.Read(b) //nolint:errcheck
	return hex.EncodeToString(b)
}
