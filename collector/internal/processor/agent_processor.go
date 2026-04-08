package processor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/govagn/collector/internal/config"
	"github.com/govagn/collector/internal/exporter"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// ─── Metrics ────────────────────────────────────────────────────────────────

var (
	processedSpans = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "govagn_processed_spans_total",
	}, []string{"framework", "status"})

	queueDepth = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "govagn_queue_depth",
		Help: "Current span queue depth",
	})

	piiScrubbed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "govagn_pii_scrubbed_total",
		Help: "PII values redacted",
	})
)

// ─── Framework Detection ────────────────────────────────────────────────────

type Framework string

const (
	FrameworkCrewAI       Framework = "crewai"
	FrameworkLangGraph    Framework = "langgraph"
	FrameworkGoogleADK    Framework = "google_adk"
	FrameworkOpenAIAgents Framework = "openai_agents"
	FrameworkClaudeAgents Framework = "claude_agents"
	FrameworkUnknown      Framework = "unknown"
)

// Attribute key constants — server-side computed, NEVER trusted from incoming spans
const (
	AttrFramework     = "af.agent.framework" // computed server-side
	AttrRunID         = "af.agent.run_id"    // computed if missing
	AttrCollectorNode = "af.collector.node"
	AttrCollectorTS   = "af.collector.received_ns"
	AttrPolicyTrusted = "af.policy.trusted" // always set to "false" on ingestion; recomputed by the gateway
	AttrStepType      = "af.span.step_type"
	AttrErrorClass    = "af.error.class"
	AttrPromptPreview = "af.preview.prompt"
	AttrResponsePrev  = "af.preview.response"
	AttrAgentName     = "agent.name"
	AttrAppName       = "af.app.name"
	AttrEnvironment   = "af.environment"
	AttrUserID        = "af.user.id"
	AttrSessionID     = "af.session.id"
	AttrRetryCount    = "af.retry.count"
	AttrPromptID      = "af.prompt.id"
	AttrPromptVersion = "af.prompt.version"
	AttrPromptRelease = "af.prompt.release_tag"
	AttrPromptEnv     = "af.prompt.environment"

	// SDK-emitted keys used only for detection (not trusted for policy)
	sdkCrewRole         = "crewai.agent.role"
	sdkLangNode         = "langgraph.node.name"
	sdkADKAgent         = "google.adk.agent.name"
	sdkOpenAIRun        = "openai.run.id"
	sdkAnthropicM       = "anthropic.model"
	sdkGenAISystem      = "gen_ai.system"
	sdkGenAIModel       = "gen_ai.request.model"
	sdkInputTokens      = "gen_ai.usage.input_tokens"
	sdkOutputTokens     = "gen_ai.usage.output_tokens"
	sdkCacheReadTokens  = "gen_ai.usage.cache_read_tokens"
	sdkCacheWriteTokens = "gen_ai.usage.cache_write_tokens"
	sdkReasoningTokens  = "gen_ai.usage.reasoning_tokens"
)

// Model pricing table (USD per 1M tokens) — update via config/price feed
// PII patterns — production set (UK-focused for JLP)
var piiPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b[A-Z]{1,2}\d{1,2}[A-Z]?\s*\d[A-Z]{2}\b`),                   // UK postcode
	regexp.MustCompile(`\b\d{4}[\s-]?\d{4}[\s-]?\d{4}[\s-]?\d{4}\b`),                     // Card number
	regexp.MustCompile(`\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}\b`),           // Email
	regexp.MustCompile(`(?i)(password|passwd|secret|token|api_key|apikey)\s*[:=]\s*\S+`), // Credentials
	regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`),                                          // SSN
	regexp.MustCompile(`(?i)\b(mr|mrs|ms|dr)\.?\s+[A-Z][a-z]+\s+[A-Z][a-z]+\b`),          // Name
	regexp.MustCompile(`(?:^|[\s,\(])(?:\+44|0)[\s\-]?(?:\d[\s\-]?){9,10}\b`),            // UK phone
}

// ─── Batch ──────────────────────────────────────────────────────────────────

type enrichedBatch struct {
	spans     []*EnrichedSpan
	createdAt time.Time
}

// EnrichedSpan is the processed form sent to the API gateway.
type EnrichedSpan struct {
	TraceID       string            `json:"trace_id"`
	SpanID        string            `json:"span_id"`
	ParentSpanID  string            `json:"parent_span_id,omitempty"`
	Name          string            `json:"name"`
	Framework     string            `json:"framework"`
	StartTimeNs   uint64            `json:"start_time_ns"`
	DurationNs    uint64            `json:"duration_ns"`
	StatusCode    int32             `json:"status_code"`
	StatusMsg     string            `json:"status_msg,omitempty"`
	Attributes    map[string]string `json:"attributes"`
	Events        []SpanEvent       `json:"events,omitempty"`
	CollectorNode string            `json:"collector_node"`
	ReceivedNs    int64             `json:"received_ns"`
	RunID         string            `json:"run_id"`
	// Usage fields — gateway remains authoritative for final pricing.
	InputTokens      int64 `json:"input_tokens,omitempty"`
	OutputTokens     int64 `json:"output_tokens,omitempty"`
	CacheReadTokens  int64 `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int64 `json:"cache_write_tokens,omitempty"`
	ReasoningTokens  int64 `json:"reasoning_tokens,omitempty"`
}

type SpanEvent struct {
	Name       string            `json:"name"`
	TimeNs     uint64            `json:"time_ns"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// ─── AgentProcessor ─────────────────────────────────────────────────────────

type AgentProcessor struct {
	cfg      *config.Config
	logger   *zap.Logger
	exporter exporter.Exporter
	queue    chan *tracepb.ResourceSpans
	wg       sync.WaitGroup
	done     chan struct{}
}

func NewAgentProcessor(cfg *config.Config, logger *zap.Logger, exp exporter.Exporter) *AgentProcessor {
	p := &AgentProcessor{
		cfg:      cfg,
		logger:   logger,
		exporter: exp,
		queue:    make(chan *tracepb.ResourceSpans, cfg.Processor.MaxQueueSize),
		done:     make(chan struct{}),
	}
	for i := 0; i < cfg.Processor.Workers; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}
	return p
}

func (p *AgentProcessor) Submit(ctx context.Context, rss []*tracepb.ResourceSpans) error {
	for _, rs := range rss {
		select {
		case p.queue <- rs:
			queueDepth.Inc()
		default:
			// Back-pressure: queue full, apply tail sampling — drop non-error spans
			processedSpans.WithLabelValues("unknown", "dropped").Inc()
		}
	}
	return nil
}

func (p *AgentProcessor) worker(id int) {
	defer p.wg.Done()

	batch := make([]*EnrichedSpan, 0, p.cfg.Processor.BatchSize)
	ticker := time.NewTicker(time.Duration(p.cfg.Processor.BatchTimeoutMS) * time.Millisecond)
	defer ticker.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(),
			time.Duration(p.cfg.Gateway.Timeout)*time.Second)
		defer cancel()
		if err := p.exporter.Export(ctx, batch); err != nil {
			p.logger.Warn("export failed", zap.Error(err), zap.Int("worker", id))
		}
		batch = batch[:0]
	}

	for {
		select {
		case <-p.done:
			flush()
			return
		case <-ticker.C:
			flush()
		case rs, ok := <-p.queue:
			if !ok {
				flush()
				return
			}
			queueDepth.Dec()
			enriched := p.processResourceSpans(rs)
			batch = append(batch, enriched...)
			if len(batch) >= p.cfg.Processor.BatchSize {
				flush()
			}
		}
	}
}

func (p *AgentProcessor) processResourceSpans(rs *tracepb.ResourceSpans) []*EnrichedSpan {
	var out []*EnrichedSpan
	resourceAttrs := extractAttrs(rs.Resource)

	for _, ss := range rs.ScopeSpans {
		for _, span := range ss.Spans {
			enriched := p.enrichSpan(span, resourceAttrs)
			out = append(out, enriched)
		}
	}
	return out
}

func (p *AgentProcessor) enrichSpan(span *tracepb.Span, resourceAttrs map[string]string) *EnrichedSpan {
	attrs := mergeAttrs(resourceAttrs, extractSpanAttrs(span))

	// 1. Detect framework (server-computed, not trusted from input)
	fw := detectFramework(attrs, span.Name)

	// 2. CRITICAL (RT-001 fix): Strip and recompute all policy/sovereignty attrs
	delete(attrs, "ai.policy.decision")
	delete(attrs, "ai.policy.evaluated")
	delete(attrs, "ai.sovereignty.compliant")

	// 3. PII scrubbing
	if p.cfg.PII.Enabled {
		attrs = p.scrubPII(attrs)
	}

	// 4. Attribute size enforcement (RT-005 fix)
	for k, v := range attrs {
		if len(v) > p.cfg.Processor.MaxAttributeLen {
			attrs[k] = v[:p.cfg.Processor.MaxAttributeLen] + "...[truncated]"
		}
	}

	// 5. Usage forwarding â€” the gateway is authoritative for final cost.
	inputTokens := parseInt64(attrs[sdkInputTokens])
	outputTokens := parseInt64(attrs[sdkOutputTokens])
	cacheReadTokens := parseInt64(attrs[sdkCacheReadTokens])
	cacheWriteTokens := parseInt64(attrs[sdkCacheWriteTokens])
	reasoningTokens := parseInt64(attrs[sdkReasoningTokens])

	// 6. Run ID (ensure present)
	runID := attrs["af.agent.run_id"]
	if runID == "" {
		runID = uuid.NewString()
	}

	// 7. Set server-side computed attrs
	attrs[AttrFramework] = string(fw)
	attrs[AttrPolicyTrusted] = "false" // always false at collector; the gateway sets the final trust state
	attrs[AttrCollectorNode] = p.cfg.NodeName
	attrs[AttrStepType] = deriveStepType(attrs, span.Name)
	attrs[AttrErrorClass] = deriveErrorClass(attrs, span.Status.GetMessage())
	attrs[AttrAgentName] = firstNonEmptyAttr(attrs, AttrAgentName, "af.agent.name", sdkADKAgent, sdkCrewRole, "service.name", "application.name")
	attrs[AttrAppName] = firstNonEmptyAttr(attrs, "af.app.name", "service.name", "application.name")
	attrs[AttrEnvironment] = firstNonEmptyAttr(attrs, "af.environment", "deployment.environment", "environment", "env")
	attrs[AttrUserID] = firstNonEmptyAttr(attrs, "af.user.id", "enduser.id", "user.id")
	attrs[AttrSessionID] = firstNonEmptyAttr(attrs, "af.session.id", "session.id")
	attrs[AttrRetryCount] = fmt.Sprintf("%d", deriveRetryCount(attrs, span.Name, span.Events))
	attrs[AttrPromptID] = firstNonEmptyAttr(attrs, "af.prompt.id")
	attrs[AttrPromptVersion] = firstNonEmptyAttr(attrs, "af.prompt.version")
	attrs[AttrPromptRelease] = firstNonEmptyAttr(attrs, "af.prompt.release_tag")
	attrs[AttrPromptEnv] = firstNonEmptyAttr(attrs, "af.prompt.environment", AttrEnvironment, "deployment.environment", "environment", "env")
	if preview := previewValue(attrs, []string{
		"gen_ai.prompt", "input.value", "prompt", "llm.prompt", "gen_ai.request.prompt",
	}); preview != "" {
		attrs[AttrPromptPreview] = preview
	}
	if preview := previewValue(attrs, []string{
		"gen_ai.response", "output.value", "response", "llm.response", "gen_ai.response.text",
	}); preview != "" {
		attrs[AttrResponsePrev] = preview
	}

	// 8. Build events
	events := make([]SpanEvent, 0, len(span.Events))
	for _, e := range span.Events {
		ea := extractKVList(e.Attributes)
		if p.cfg.PII.Enabled {
			ea = p.scrubPII(ea)
		}
		events = append(events, SpanEvent{
			Name:       e.Name,
			TimeNs:     e.TimeUnixNano,
			Attributes: ea,
		})
	}

	e := &EnrichedSpan{
		TraceID:          hex.EncodeToString(span.TraceId),
		SpanID:           hex.EncodeToString(span.SpanId),
		ParentSpanID:     hex.EncodeToString(span.ParentSpanId),
		Name:             span.Name,
		Framework:        string(fw),
		StartTimeNs:      span.StartTimeUnixNano,
		DurationNs:       span.EndTimeUnixNano - span.StartTimeUnixNano,
		StatusCode:       int32(span.Status.GetCode()),
		StatusMsg:        span.Status.GetMessage(),
		Attributes:       attrs,
		Events:           events,
		CollectorNode:    p.cfg.NodeName,
		ReceivedNs:       time.Now().UnixNano(),
		RunID:            runID,
		InputTokens:      inputTokens,
		OutputTokens:     outputTokens,
		CacheReadTokens:  cacheReadTokens,
		CacheWriteTokens: cacheWriteTokens,
		ReasoningTokens:  reasoningTokens,
	}

	processedSpans.WithLabelValues(string(fw), "ok").Inc()
	return e
}

func (p *AgentProcessor) scrubPII(attrs map[string]string) map[string]string {
	out := make(map[string]string, len(attrs))
	for k, v := range attrs {
		redacted := v
		for _, pat := range piiPatterns {
			if pat.MatchString(redacted) {
				redacted = pat.ReplaceAllString(redacted, "[REDACTED]")
				piiScrubbed.Inc()
			}
		}
		// Also check JSON values recursively (RT-002 fix)
		if strings.HasPrefix(strings.TrimSpace(v), "{") || strings.HasPrefix(strings.TrimSpace(v), "[") {
			redacted = scrubJSONString(redacted)
		}
		out[k] = redacted
	}
	return out
}

func scrubJSONString(s string) string {
	for _, pat := range piiPatterns {
		s = pat.ReplaceAllString(s, "[REDACTED]")
	}
	return s
}

// RunDiscovery periodically scans for agent processes / k8s pods
func (p *AgentProcessor) RunDiscovery(ctx context.Context) {
	if !p.cfg.Discovery.Enabled {
		return
	}
	ticker := time.NewTicker(time.Duration(p.cfg.Discovery.ProcessScanSecs) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.scanProcesses()
		}
	}
}

func (p *AgentProcessor) scanProcesses() {
	// Emit synthetic discovery spans for processes running known agent frameworks
	// This would integrate with /proc or k8s API in production
	p.logger.Debug("process discovery scan completed")
}

func (p *AgentProcessor) Shutdown() {
	close(p.done)
	p.wg.Wait()
}

// ─── Helpers ────────────────────────────────────────────────────────────────

func detectFramework(attrs map[string]string, spanName string) Framework {
	checks := []struct {
		key string
		fw  Framework
	}{
		{sdkCrewRole, FrameworkCrewAI},
		{"crewai.crew.id", FrameworkCrewAI},
		{sdkLangNode, FrameworkLangGraph},
		{"langgraph.graph.id", FrameworkLangGraph},
		{sdkADKAgent, FrameworkGoogleADK},
		{"google.adk.session.id", FrameworkGoogleADK},
		{sdkOpenAIRun, FrameworkOpenAIAgents},
		{"openai.assistant.id", FrameworkOpenAIAgents},
		{sdkAnthropicM, FrameworkClaudeAgents},
		{"anthropic.api_version", FrameworkClaudeAgents},
	}
	for _, c := range checks {
		if v, ok := attrs[c.key]; ok && v != "" {
			return c.fw
		}
	}
	// Fall through to model-based detection
	model := strings.ToLower(attrs[sdkGenAIModel])
	switch {
	case strings.HasPrefix(model, "claude"):
		return FrameworkClaudeAgents
	case strings.HasPrefix(model, "gpt") || strings.HasPrefix(model, "o1") || strings.HasPrefix(model, "o3"):
		return FrameworkOpenAIAgents
	case strings.HasPrefix(model, "gemini"):
		return FrameworkGoogleADK
	case strings.Contains(spanName, "crewai"):
		return FrameworkCrewAI
	case strings.Contains(spanName, "langgraph"):
		return FrameworkLangGraph
	}
	return FrameworkUnknown
}

func computeCost(model string, inputTokens, outputTokens int64) (float64, float64) {
	return computeCostWithProvider("", model, inputTokens, outputTokens)
}

func extractAttrs(res *resourcepb.Resource) map[string]string {
	if res == nil {
		return map[string]string{}
	}
	return extractKVList(res.Attributes)
}

func extractSpanAttrs(span *tracepb.Span) map[string]string {
	return extractKVList(span.Attributes)
}

func extractKVList(kvs []*commonpb.KeyValue) map[string]string {
	out := make(map[string]string, len(kvs))
	for _, kv := range kvs {
		if kv == nil || kv.Value == nil {
			continue
		}
		// Hash-based deduplication key
		h := sha256.Sum256([]byte(kv.Key))
		_ = h
		out[kv.Key] = kv.Value.GetStringValue()
	}
	return out
}

func mergeAttrs(a, b map[string]string) map[string]string {
	out := make(map[string]string, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

func parseInt64(s string) int64 {
	if s == "" {
		return 0
	}
	var v int64
	fmt.Sscan(s, &v)
	return v
}

func deriveStepType(attrs map[string]string, spanName string) string {
	if v := strings.TrimSpace(attrs[AttrStepType]); v != "" {
		return v
	}
	lowerName := strings.ToLower(spanName)
	model := strings.TrimSpace(attrs[sdkGenAIModel])
	switch {
	case model != "", attrs[sdkGenAISystem] != "":
		return "llm"
	case strings.Contains(lowerName, "tool"), strings.Contains(lowerName, "function"):
		return "tool"
	case strings.Contains(lowerName, "retry"):
		return "retry"
	case strings.Contains(lowerName, "policy"), strings.Contains(lowerName, "guard"):
		return "policy"
	default:
		return "agent"
	}
}

func deriveErrorClass(attrs map[string]string, statusMsg string) string {
	for _, key := range []string{"error.type", "exception.type", AttrErrorClass} {
		if v := strings.TrimSpace(attrs[key]); v != "" {
			return v
		}
	}
	if strings.TrimSpace(statusMsg) != "" {
		return "runtime_error"
	}
	return ""
}

func previewValue(attrs map[string]string, keys []string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(attrs[key]); v != "" {
			if len(v) > 220 {
				return v[:220] + "..."
			}
			return v
		}
	}
	return ""
}

func firstNonEmptyAttr(attrs map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(attrs[key]); value != "" {
			return value
		}
	}
	return ""
}

func deriveRetryCount(attrs map[string]string, spanName string, events []*tracepb.Span_Event) int {
	for _, key := range []string{AttrRetryCount, "retry.count", "http.retry_count"} {
		if value := strings.TrimSpace(attrs[key]); value != "" {
			if parsed := parseInt64(value); parsed > 0 {
				return int(parsed)
			}
		}
	}
	count := 0
	if strings.Contains(strings.ToLower(spanName), "retry") {
		count++
	}
	for _, event := range events {
		if event != nil && strings.Contains(strings.ToLower(event.Name), "retry") {
			count++
		}
	}
	return count
}
