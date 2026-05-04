package receiver

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// EventReceiver defines the interface for ingesting events from various sources
type EventReceiver interface {
	Receive(ctx context.Context, event interface{}) error
	Name() string
}

type spanSubmitter interface {
	Submit(ctx context.Context, rss []*tracepb.ResourceSpans) error
}

// VSCodeExtensionReceiver handles VSCode extension webhook events
type VSCodeExtensionReceiver struct {
	name      string
	submitter spanSubmitter
}

func NewVSCodeExtensionReceiver() *VSCodeExtensionReceiver {
	return &VSCodeExtensionReceiver{name: "vscode-extension"}
}

func (r *VSCodeExtensionReceiver) SetSubmitter(submitter spanSubmitter) {
	r.submitter = submitter
}

// VSCodeExtensionEvent represents an event from VSCode extensions
type VSCodeExtensionEvent struct {
	Source    string                 `json:"source"`     // e.g., "vscode-copilot", "cursor"
	EventType string                 `json:"event_type"` // e.g., "suggestion.accepted"
	Timestamp time.Time              `json:"timestamp"`
	UserID    string                 `json:"user_id,omitempty"`
	UserEmail string                 `json:"user_email,omitempty"`
	SessionID string                 `json:"session_id,omitempty"`
	Model     string                 `json:"model,omitempty"`
	LatencyMs int64                  `json:"latency_ms,omitempty"`
	Payload   map[string]interface{} `json:"payload"`
}

func (r *VSCodeExtensionReceiver) Receive(ctx context.Context, event interface{}) error {
	data, ok := event.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid VSCode extension event format")
	}

	// Validate required fields
	source, ok := data["source"].(string)
	if !ok || source == "" {
		return fmt.Errorf("missing or invalid 'source' field")
	}

	eventType, ok := data["event_type"].(string)
	if !ok || eventType == "" {
		return fmt.Errorf("missing or invalid 'event_type' field")
	}

	// Parse timestamp
	var ts time.Time
	if tsStr, ok := data["timestamp"].(string); ok {
		var err error
		ts, err = time.Parse(time.RFC3339, tsStr)
		if err != nil {
			ts = time.Now()
		}
	} else {
		ts = time.Now()
	}

	vsEvent := &VSCodeExtensionEvent{
		Source:    source,
		EventType: eventType,
		Timestamp: ts,
		Payload:   data,
	}

	// Extract optional fields
	if uid, ok := data["user_id"].(string); ok {
		vsEvent.UserID = uid
	}
	if email, ok := data["user_email"].(string); ok {
		vsEvent.UserEmail = email
	}
	if sid, ok := data["session_id"].(string); ok {
		vsEvent.SessionID = sid
	}
	if model, ok := data["model"].(string); ok {
		vsEvent.Model = model
	}
	if latency, ok := data["latency_ms"].(float64); ok {
		vsEvent.LatencyMs = int64(latency)
	}

	if r.submitter != nil {
		if err := submitDeviceEvent(ctx, r.submitter, vsEvent.Timestamp, source, eventType, data); err != nil {
			return err
		}
	}

	return nil
}

func (r *VSCodeExtensionReceiver) Name() string {
	return r.name
}

// WebhookReceiver handles generic webhook events
type WebhookReceiver struct {
	name      string
	submitter spanSubmitter
}

func NewWebhookReceiver() *WebhookReceiver {
	return &WebhookReceiver{name: "webhook"}
}

func (r *WebhookReceiver) SetSubmitter(submitter spanSubmitter) {
	r.submitter = submitter
}

// WebhookEvent represents a generic webhook event
type WebhookEvent struct {
	Source    string                 `json:"source"`
	EventType string                 `json:"event_type"`
	Timestamp time.Time              `json:"timestamp"`
	Payload   map[string]interface{} `json:"payload"`
}

func (r *WebhookReceiver) Receive(ctx context.Context, event interface{}) error {
	data, ok := event.(map[string]interface{})
	if !ok {
		// Try to unmarshal from JSON bytes
		var err error
		data, err = unmarshalEvent(event)
		if err != nil {
			return fmt.Errorf("invalid webhook event format: %w", err)
		}
	}

	// Validate required fields
	if _, ok := data["source"]; !ok {
		return fmt.Errorf("missing 'source' field")
	}

	if _, ok := data["event_type"]; !ok {
		return fmt.Errorf("missing 'event_type' field")
	}

	if r.submitter != nil {
		ts := parseEventTime(data)
		source := asString(data["source"], "webhook")
		eventType := asString(data["event_type"], "event")
		if err := submitDeviceEvent(ctx, r.submitter, ts, source, eventType, data); err != nil {
			return err
		}
	}

	return nil
}

func (r *WebhookReceiver) Name() string {
	return r.name
}

// DirectAPIReceiver handles direct API ingestion of pre-normalized events
type DirectAPIReceiver struct {
	name      string
	submitter spanSubmitter
}

func NewDirectAPIReceiver() *DirectAPIReceiver {
	return &DirectAPIReceiver{name: "direct-api"}
}

func (r *DirectAPIReceiver) SetSubmitter(submitter spanSubmitter) {
	r.submitter = submitter
}

func (r *DirectAPIReceiver) Receive(ctx context.Context, event interface{}) error {
	data, ok := event.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid direct API event format")
	}

	// Validate required fields
	if _, ok := data["source_vendor"]; !ok {
		return fmt.Errorf("missing 'source_vendor' field")
	}

	if _, ok := data["event_type"]; !ok {
		return fmt.Errorf("missing 'event_type' field")
	}

	if r.submitter != nil {
		ts := parseEventTime(data)
		source := asString(data["source_vendor"], "direct-api")
		eventType := asString(data["event_type"], "event")
		if err := submitDeviceEvent(ctx, r.submitter, ts, source, eventType, data); err != nil {
			return err
		}
	}

	return nil
}

func (r *DirectAPIReceiver) Name() string {
	return r.name
}

// Helper function to unmarshal event data
func unmarshalEvent(event interface{}) (map[string]interface{}, error) {
	var data map[string]interface{}

	switch v := event.(type) {
	case []byte:
		if err := json.Unmarshal(v, &data); err != nil {
			return nil, err
		}
	case string:
		if err := json.Unmarshal([]byte(v), &data); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported event type: %T", event)
	}

	return data, nil
}

func submitDeviceEvent(ctx context.Context, submitter spanSubmitter, ts time.Time, source, eventType string, data map[string]interface{}) error {
	if submitter == nil {
		return nil
	}
	rss := buildResourceSpans(ts, source, eventType, data)
	return submitter.Submit(ctx, []*tracepb.ResourceSpans{rss})
}

func buildResourceSpans(ts time.Time, source, eventType string, data map[string]interface{}) *tracepb.ResourceSpans {
	if ts.IsZero() {
		ts = time.Now().UTC()
	}

	start := uint64(ts.UnixNano())
	end := start + uint64(time.Millisecond)
	traceID := make([]byte, 16)
	spanID := make([]byte, 8)
	if _, err := rand.Read(traceID); err != nil {
		for i := range traceID {
			traceID[i] = byte(i + 1)
		}
	}
	if _, err := rand.Read(spanID); err != nil {
		for i := range spanID {
			spanID[i] = byte(i + 1)
		}
	}

	spanAttrs := mapToAttrs(data)
	spanAttrs = append(spanAttrs,
		kv("source", source),
		kv("event_type", eventType),
		kv("af.agent.name", source),
	)
	spanAttrs = append(spanAttrs, extractPromptResponseAttrs(data)...)
	spanAttrs = injectFrameworkHints(spanAttrs, source, data)

	resourceAttrs := []*commonpb.KeyValue{
		kv("service.name", "device-telemetry"),
	}
	if env := asString(data["environment"], ""); env != "" {
		resourceAttrs = append(resourceAttrs, kv("deployment.environment", env))
	}

	span := &tracepb.Span{
		TraceId:           traceID,
		SpanId:            spanID,
		Name:              fmt.Sprintf("%s.%s", sanitizeForSpanName(source), sanitizeForSpanName(eventType)),
		Kind:              tracepb.Span_SPAN_KIND_INTERNAL,
		StartTimeUnixNano: start,
		EndTimeUnixNano:   end,
		Attributes:        spanAttrs,
		Status:            &tracepb.Status{Code: tracepb.Status_STATUS_CODE_OK},
	}

	return &tracepb.ResourceSpans{
		Resource: &resourcepb.Resource{Attributes: resourceAttrs},
		ScopeSpans: []*tracepb.ScopeSpans{
			{
				Spans: []*tracepb.Span{span},
			},
		},
	}
}

func injectFrameworkHints(attrs []*commonpb.KeyValue, source string, data map[string]interface{}) []*commonpb.KeyValue {
	sourceLower := strings.ToLower(source)
	switch {
	case strings.Contains(sourceLower, "codex"):
		if session := asString(data["session_id"], ""); session != "" {
			attrs = append(attrs, kv("codex.session.id", session))
		}
	case strings.Contains(sourceLower, "claude_code"), strings.Contains(sourceLower, "claude-code"):
		if session := asString(data["session_id"], ""); session != "" {
			attrs = append(attrs, kv("claude_code.session_id", session))
		}
	case strings.Contains(sourceLower, "vscode"), strings.Contains(sourceLower, "cursor"):
		attrs = append(attrs, kv("af.app.name", "vstudio"))
	}

	if model := asString(data["model"], ""); model != "" {
		attrs = append(attrs, kv("gen_ai.request.model", model))
	}
	if userID := asString(data["user_id"], ""); userID != "" {
		attrs = append(attrs, kv("af.user.id", userID))
	}
	return attrs
}

func mapToAttrs(data map[string]interface{}) []*commonpb.KeyValue {
	attrs := make([]*commonpb.KeyValue, 0, len(data))
	for key, value := range data {
		switch typed := value.(type) {
		case string:
			attrs = append(attrs, kv(key, typed))
		case float64, bool, int, int64:
			attrs = append(attrs, kv(key, fmt.Sprintf("%v", typed)))
		case map[string]interface{}, []interface{}:
			encoded, _ := json.Marshal(typed)
			attrs = append(attrs, kv(key, string(encoded)))
		default:
			attrs = append(attrs, kv(key, fmt.Sprintf("%v", typed)))
		}
	}
	return attrs
}

func extractPromptResponseAttrs(data map[string]interface{}) []*commonpb.KeyValue {
	attrs := make([]*commonpb.KeyValue, 0, 4)

	find := func(keys ...string) string {
		for _, key := range keys {
			if value := asString(data[key], ""); value != "" {
				return value
			}
		}
		if payload, ok := data["payload"].(map[string]interface{}); ok {
			for _, key := range keys {
				if value := asString(payload[key], ""); value != "" {
					return value
				}
			}
		}
		return ""
	}

	if prompt := find("gen_ai.prompt", "prompt", "input", "query", "message"); prompt != "" {
		attrs = append(attrs, kv("gen_ai.prompt", prompt))
	}
	if response := find("gen_ai.response", "response", "output", "completion", "result"); response != "" {
		attrs = append(attrs, kv("gen_ai.response", response))
	}
	return attrs
}

func kv(key, value string) *commonpb.KeyValue {
	return &commonpb.KeyValue{
		Key: key,
		Value: &commonpb.AnyValue{
			Value: &commonpb.AnyValue_StringValue{StringValue: value},
		},
	}
}

func sanitizeForSpanName(value string) string {
	clean := strings.ToLower(strings.TrimSpace(value))
	clean = strings.ReplaceAll(clean, " ", "_")
	clean = strings.ReplaceAll(clean, "/", "_")
	clean = strings.ReplaceAll(clean, ".", "_")
	if clean == "" {
		return "event"
	}
	return clean
}

func parseEventTime(data map[string]interface{}) time.Time {
	if raw, ok := data["timestamp"].(string); ok && strings.TrimSpace(raw) != "" {
		if ts, err := time.Parse(time.RFC3339, raw); err == nil {
			return ts
		}
	}
	return time.Now().UTC()
}

func asString(value interface{}, def string) string {
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return def
		}
		return typed
	default:
		return def
	}
}

// HTTP Handlers for each receiver type

// VSCodeExtensionHandler handles HTTP POST requests from VSCode extensions
func VSCodeExtensionHandler(receiver *VSCodeExtensionReceiver) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var event map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		if err := receiver.Receive(r.Context(), event); err != nil {
			http.Error(w, fmt.Sprintf("error: %v", err), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
	}
}

// WebhookHandler handles HTTP POST requests from generic webhooks
func WebhookHandler(receiver *WebhookReceiver) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var event map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		if err := receiver.Receive(r.Context(), event); err != nil {
			http.Error(w, fmt.Sprintf("error: %v", err), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
	}
}

// DirectAPIHandler handles HTTP POST requests for direct API ingestion
func DirectAPIHandler(receiver *DirectAPIReceiver) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var event map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		if err := receiver.Receive(r.Context(), event); err != nil {
			http.Error(w, fmt.Sprintf("error: %v", err), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
	}
}
