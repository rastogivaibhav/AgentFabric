package receiver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// EventReceiver defines the interface for ingesting events from various sources
type EventReceiver interface {
	Receive(ctx context.Context, event interface{}) error
	Name() string
}

// VSCodeExtensionReceiver handles VSCode extension webhook events
type VSCodeExtensionReceiver struct {
	name string
}

func NewVSCodeExtensionReceiver() *VSCodeExtensionReceiver {
	return &VSCodeExtensionReceiver{name: "vscode-extension"}
}

// VSCodeExtensionEvent represents an event from VSCode extensions
type VSCodeExtensionEvent struct {
	Source      string                 `json:"source"`       // e.g., "vscode-copilot", "cursor"
	EventType   string                 `json:"event_type"`   // e.g., "suggestion.accepted"
	Timestamp   time.Time              `json:"timestamp"`
	UserID      string                 `json:"user_id,omitempty"`
	UserEmail   string                 `json:"user_email,omitempty"`
	SessionID   string                 `json:"session_id,omitempty"`
	Model       string                 `json:"model,omitempty"`
	LatencyMs   int64                  `json:"latency_ms,omitempty"`
	Payload     map[string]interface{} `json:"payload"`
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

	// TODO: Process event through normalization pipeline
	// fmt.Printf("Received VSCode event: %+v\n", vsEvent)

	return nil
}

func (r *VSCodeExtensionReceiver) Name() string {
	return r.name
}

// WebhookReceiver handles generic webhook events
type WebhookReceiver struct {
	name string
}

func NewWebhookReceiver() *WebhookReceiver {
	return &WebhookReceiver{name: "webhook"}
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

	// TODO: Process event through normalization pipeline

	return nil
}

func (r *WebhookReceiver) Name() string {
	return r.name
}

// DirectAPIReceiver handles direct API ingestion of pre-normalized events
type DirectAPIReceiver struct {
	name string
}

func NewDirectAPIReceiver() *DirectAPIReceiver {
	return &DirectAPIReceiver{name: "direct-api"}
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

	// TODO: Process event directly to storage

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
