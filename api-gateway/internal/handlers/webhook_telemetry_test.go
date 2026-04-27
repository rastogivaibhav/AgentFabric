package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/govagn/api-gateway/internal/normalization"
	"go.uber.org/zap"
)

// MockMapperRegistry implements MapperRegistry for testing
type MockMapperRegistry struct {
	mappers map[string]normalization.EventMapper
	vendors []string
}

func NewMockMapperRegistry() *MockMapperRegistry {
	return &MockMapperRegistry{
		mappers: make(map[string]normalization.EventMapper),
		vendors: []string{},
	}
}

func (m *MockMapperRegistry) Get(vendor string) (normalization.EventMapper, error) {
	mapper, ok := m.mappers[vendor]
	if !ok {
		return nil, fmt.Errorf("no mapper for vendor: %s", vendor)
	}
	return mapper, nil
}

func (m *MockMapperRegistry) ListVendors() []string {
	return m.vendors
}

func (m *MockMapperRegistry) RegisterMapper(vendor string, mapper normalization.EventMapper) {
	m.mappers[vendor] = mapper
	m.vendors = append(m.vendors, vendor)
}

// MockEventMapper implements EventMapper for testing
type MockEventMapper struct {
	shouldFail bool
	returnID   string
}

func (m *MockEventMapper) Map(event interface{}) (*normalization.CanonicalEvent, error) {
	if m.shouldFail {
		return nil, fmt.Errorf("mapping failed")
	}

	data, ok := event.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("expected map, got %T", event)
	}

	eventID := m.returnID
	if eventID == "" {
		eventID = "evt_test_12345"
	}

	return &normalization.CanonicalEvent{
		ID:            eventID,
		EventTime:     time.Now(),
		SourceVendor:  data["source_vendor"].(string),
		EventType:     data["event_type"].(string),
		UserEmail:     "test@example.com",
		UserID:        "user_123",
		SessionID:     "sess_123",
		EventCategory: "test",
		Action:        "test_action",
		Success:       true,
		Payload:       data,
	}, nil
}

func (m *MockEventMapper) Accepts(sourceVendor, sourceProduct string) bool {
	return true
}

// MockPostgresStore implements PostgresStore for testing
type MockPostgresStore struct {
	storedEvents []*normalization.CanonicalEvent
	shouldFail   bool
}

func NewMockPostgresStore() *MockPostgresStore {
	return &MockPostgresStore{
		storedEvents: make([]*normalization.CanonicalEvent, 0),
	}
}

func (m *MockPostgresStore) StoreCanonicalEvent(ctx context.Context, event *normalization.CanonicalEvent) error {
	if m.shouldFail {
		return fmt.Errorf("storage failed")
	}
	m.storedEvents = append(m.storedEvents, event)
	return nil
}

func (m *MockPostgresStore) GetCanonicalEvent(ctx context.Context, id string) (*normalization.CanonicalEvent, error) {
	return nil, nil
}

func TestHandleWebhookEvent_SuccessfulSingleEvent(t *testing.T) {
	registry := NewMockMapperRegistry()
	registry.RegisterMapper("test-vendor", &MockEventMapper{})
	store := NewMockPostgresStore()
	logger, _ := zap.NewDevelopment()

	handler := NewWebhookTelemetryHandler(registry, store, logger)

	payload := map[string]interface{}{
		"source_vendor": "test-vendor",
		"event_type":    "test.event",
		"timestamp":     time.Now().Format(time.RFC3339),
		"user_email":    "dev@example.com",
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/webhook/event", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.HandleWebhookEvent(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("expected status %d, got %d", http.StatusAccepted, w.Code)
	}

	if len(store.storedEvents) != 1 {
		t.Errorf("expected 1 stored event, got %d", len(store.storedEvents))
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	if response["status"] != "accepted" {
		t.Errorf("expected status 'accepted', got '%v'", response["status"])
	}
	if response["event_id"] == nil {
		t.Error("expected event_id in response")
	}
}

func TestHandleWebhookEvent_MethodNotAllowed(t *testing.T) {
	registry := NewMockMapperRegistry()
	store := NewMockPostgresStore()
	logger, _ := zap.NewDevelopment()

	handler := NewWebhookTelemetryHandler(registry, store, logger)

	req := httptest.NewRequest("GET", "/webhook/event", nil)
	w := httptest.NewRecorder()

	handler.HandleWebhookEvent(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestHandleWebhookEvent_InvalidJSON(t *testing.T) {
	registry := NewMockMapperRegistry()
	store := NewMockPostgresStore()
	logger, _ := zap.NewDevelopment()

	handler := NewWebhookTelemetryHandler(registry, store, logger)

	req := httptest.NewRequest("POST", "/webhook/event", strings.NewReader("invalid json"))
	w := httptest.NewRecorder()

	handler.HandleWebhookEvent(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid JSON") {
		t.Errorf("expected 'invalid JSON' error message")
	}
}

func TestHandleWebhookEvent_MissingSourceVendor(t *testing.T) {
	registry := NewMockMapperRegistry()
	store := NewMockPostgresStore()
	logger, _ := zap.NewDevelopment()

	handler := NewWebhookTelemetryHandler(registry, store, logger)

	payload := map[string]interface{}{
		"event_type": "test.event",
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/webhook/event", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.HandleWebhookEvent(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
	if !strings.Contains(w.Body.String(), "missing source_vendor") {
		t.Errorf("expected 'missing source_vendor' error message")
	}
}

func TestHandleWebhookEvent_UnsupportedVendor(t *testing.T) {
	registry := NewMockMapperRegistry()
	registry.RegisterMapper("supported-vendor", &MockEventMapper{})
	store := NewMockPostgresStore()
	logger, _ := zap.NewDevelopment()

	handler := NewWebhookTelemetryHandler(registry, store, logger)

	payload := map[string]interface{}{
		"source_vendor": "unsupported-vendor",
		"event_type":    "test.event",
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/webhook/event", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.HandleWebhookEvent(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
	if !strings.Contains(w.Body.String(), "unsupported vendor") {
		t.Errorf("expected 'unsupported vendor' error message")
	}
}

func TestHandleWebhookEvent_MapperFails(t *testing.T) {
	registry := NewMockMapperRegistry()
	registry.RegisterMapper("failing-vendor", &MockEventMapper{shouldFail: true})
	store := NewMockPostgresStore()
	logger, _ := zap.NewDevelopment()

	handler := NewWebhookTelemetryHandler(registry, store, logger)

	payload := map[string]interface{}{
		"source_vendor": "failing-vendor",
		"event_type":    "test.event",
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/webhook/event", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.HandleWebhookEvent(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
	if !strings.Contains(w.Body.String(), "failed to process event") {
		t.Errorf("expected 'failed to process event' error message")
	}
}

func TestHandleWebhookEvent_StorageFails(t *testing.T) {
	registry := NewMockMapperRegistry()
	registry.RegisterMapper("test-vendor", &MockEventMapper{})
	store := NewMockPostgresStore()
	store.shouldFail = true
	logger, _ := zap.NewDevelopment()

	handler := NewWebhookTelemetryHandler(registry, store, logger)

	payload := map[string]interface{}{
		"source_vendor": "test-vendor",
		"event_type":    "test.event",
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/webhook/event", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.HandleWebhookEvent(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}
	if !strings.Contains(w.Body.String(), "failed to store event") {
		t.Errorf("expected 'failed to store event' error message")
	}
}

func TestHandleWebhookBatch_SuccessfulBatch(t *testing.T) {
	registry := NewMockMapperRegistry()
	registry.RegisterMapper("test-vendor", &MockEventMapper{})
	store := NewMockPostgresStore()
	logger, _ := zap.NewDevelopment()

	handler := NewWebhookTelemetryHandler(registry, store, logger)

	events := []map[string]interface{}{
		{
			"source_vendor": "test-vendor",
			"event_type":    "test.event1",
		},
		{
			"source_vendor": "test-vendor",
			"event_type":    "test.event2",
		},
		{
			"source_vendor": "test-vendor",
			"event_type":    "test.event3",
		},
	}

	body, _ := json.Marshal(events)
	req := httptest.NewRequest("POST", "/webhook/batch", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.HandleWebhookBatch(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("expected status %d, got %d", http.StatusAccepted, w.Code)
	}

	if len(store.storedEvents) != 3 {
		t.Errorf("expected 3 stored events, got %d", len(store.storedEvents))
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	if response["success"] != float64(3) {
		t.Errorf("expected success count 3, got %v", response["success"])
	}
	if response["failure"] != float64(0) {
		t.Errorf("expected failure count 0, got %v", response["failure"])
	}
}

func TestHandleWebhookBatch_PartialSuccess(t *testing.T) {
	registry := NewMockMapperRegistry()
	registry.RegisterMapper("good-vendor", &MockEventMapper{})
	registry.RegisterMapper("bad-vendor", &MockEventMapper{shouldFail: true})
	store := NewMockPostgresStore()
	logger, _ := zap.NewDevelopment()

	handler := NewWebhookTelemetryHandler(registry, store, logger)

	events := []map[string]interface{}{
		{
			"source_vendor": "good-vendor",
			"event_type":    "test.event1",
		},
		{
			"source_vendor": "bad-vendor",
			"event_type":    "test.event2",
		},
		{
			"source_vendor": "good-vendor",
			"event_type":    "test.event3",
		},
	}

	body, _ := json.Marshal(events)
	req := httptest.NewRequest("POST", "/webhook/batch", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.HandleWebhookBatch(w, req)

	if w.Code != http.StatusPartialContent {
		t.Errorf("expected status %d, got %d", http.StatusPartialContent, w.Code)
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	if response["success"] != float64(2) {
		t.Errorf("expected success count 2, got %v", response["success"])
	}
	if response["failure"] != float64(1) {
		t.Errorf("expected failure count 1, got %v", response["failure"])
	}
}

func TestHandleWebhookBatch_EmptyBatch(t *testing.T) {
	registry := NewMockMapperRegistry()
	store := NewMockPostgresStore()
	logger, _ := zap.NewDevelopment()

	handler := NewWebhookTelemetryHandler(registry, store, logger)

	events := []map[string]interface{}{}

	body, _ := json.Marshal(events)
	req := httptest.NewRequest("POST", "/webhook/batch", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.HandleWebhookBatch(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
	if !strings.Contains(w.Body.String(), "empty batch") {
		t.Errorf("expected 'empty batch' error message")
	}
}

func TestHandleWebhookBatch_ExceedsMaxSize(t *testing.T) {
	registry := NewMockMapperRegistry()
	store := NewMockPostgresStore()
	logger, _ := zap.NewDevelopment()

	handler := NewWebhookTelemetryHandler(registry, store, logger)

	events := make([]map[string]interface{}, 1001)
	for i := range events {
		events[i] = map[string]interface{}{
			"source_vendor": "test-vendor",
			"event_type":    "test.event",
		}
	}

	body, _ := json.Marshal(events)
	req := httptest.NewRequest("POST", "/webhook/batch", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.HandleWebhookBatch(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
	if !strings.Contains(w.Body.String(), "batch too large") {
		t.Errorf("expected 'batch too large' error message")
	}
}

func TestHandleWebhookBatch_MethodNotAllowed(t *testing.T) {
	registry := NewMockMapperRegistry()
	store := NewMockPostgresStore()
	logger, _ := zap.NewDevelopment()

	handler := NewWebhookTelemetryHandler(registry, store, logger)

	req := httptest.NewRequest("GET", "/webhook/batch", nil)
	w := httptest.NewRecorder()

	handler.HandleWebhookBatch(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestHandleWebhookBatch_InvalidJSON(t *testing.T) {
	registry := NewMockMapperRegistry()
	store := NewMockPostgresStore()
	logger, _ := zap.NewDevelopment()

	handler := NewWebhookTelemetryHandler(registry, store, logger)

	req := httptest.NewRequest("POST", "/webhook/batch", strings.NewReader("invalid json"))
	w := httptest.NewRecorder()

	handler.HandleWebhookBatch(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestHandleWebhookBatch_MissingSourceVendor(t *testing.T) {
	registry := NewMockMapperRegistry()
	store := NewMockPostgresStore()
	logger, _ := zap.NewDevelopment()

	handler := NewWebhookTelemetryHandler(registry, store, logger)

	events := []map[string]interface{}{
		{
			"event_type": "test.event",
		},
	}

	body, _ := json.Marshal(events)
	req := httptest.NewRequest("POST", "/webhook/batch", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.HandleWebhookBatch(w, req)

	if w.Code != http.StatusPartialContent {
		t.Errorf("expected status %d (with partial results), got %d", http.StatusPartialContent, w.Code)
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	if response["failure"] == nil || response["failure"] == float64(0) {
		t.Errorf("expected at least 1 failure for missing source_vendor")
	}
}

func TestHandleWebhookHealth_ReturnsVendorList(t *testing.T) {
	registry := NewMockMapperRegistry()
	registry.RegisterMapper("vendor1", &MockEventMapper{})
	registry.RegisterMapper("vendor2", &MockEventMapper{})
	store := NewMockPostgresStore()
	logger, _ := zap.NewDevelopment()

	handler := NewWebhookTelemetryHandler(registry, store, logger)

	req := httptest.NewRequest("GET", "/webhook/health", nil)
	w := httptest.NewRecorder()

	handler.HandleWebhookHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	if response["status"] != "healthy" {
		t.Errorf("expected status 'healthy', got '%v'", response["status"])
	}

	vendors, ok := response["supported_vendors"].([]interface{})
	if !ok {
		t.Errorf("expected supported_vendors array")
	}
	if len(vendors) != 2 {
		t.Errorf("expected 2 vendors, got %d", len(vendors))
	}

	if response["max_batch_size"] != float64(1000) {
		t.Errorf("expected max_batch_size 1000, got %v", response["max_batch_size"])
	}

	if response["timeout_seconds"] != float64(30) {
		t.Errorf("expected timeout_seconds 30, got %v", response["timeout_seconds"])
	}
}

func TestHandleWebhookHealth_MethodNotAllowed(t *testing.T) {
	registry := NewMockMapperRegistry()
	store := NewMockPostgresStore()
	logger, _ := zap.NewDevelopment()

	handler := NewWebhookTelemetryHandler(registry, store, logger)

	req := httptest.NewRequest("POST", "/webhook/health", nil)
	w := httptest.NewRecorder()

	handler.HandleWebhookHealth(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestHandleWebhookEvent_ContentType(t *testing.T) {
	registry := NewMockMapperRegistry()
	registry.RegisterMapper("test-vendor", &MockEventMapper{})
	store := NewMockPostgresStore()
	logger, _ := zap.NewDevelopment()

	handler := NewWebhookTelemetryHandler(registry, store, logger)

	payload := map[string]interface{}{
		"source_vendor": "test-vendor",
		"event_type":    "test.event",
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/webhook/event", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.HandleWebhookEvent(w, req)

	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got '%s'", w.Header().Get("Content-Type"))
	}
}

func TestHandleWebhookBatch_ContentType(t *testing.T) {
	registry := NewMockMapperRegistry()
	registry.RegisterMapper("test-vendor", &MockEventMapper{})
	store := NewMockPostgresStore()
	logger, _ := zap.NewDevelopment()

	handler := NewWebhookTelemetryHandler(registry, store, logger)

	events := []map[string]interface{}{
		{
			"source_vendor": "test-vendor",
			"event_type":    "test.event",
		},
	}

	body, _ := json.Marshal(events)
	req := httptest.NewRequest("POST", "/webhook/batch", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.HandleWebhookBatch(w, req)

	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got '%s'", w.Header().Get("Content-Type"))
	}
}

func TestHandleWebhookHealth_ContentType(t *testing.T) {
	registry := NewMockMapperRegistry()
	store := NewMockPostgresStore()
	logger, _ := zap.NewDevelopment()

	handler := NewWebhookTelemetryHandler(registry, store, logger)

	req := httptest.NewRequest("GET", "/webhook/health", nil)
	w := httptest.NewRecorder()

	handler.HandleWebhookHealth(w, req)

	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got '%s'", w.Header().Get("Content-Type"))
	}
}

func TestHandleWebhookBatch_ResultsArray(t *testing.T) {
	registry := NewMockMapperRegistry()
	registry.RegisterMapper("good-vendor", &MockEventMapper{returnID: "evt_001"})
	registry.RegisterMapper("bad-vendor", &MockEventMapper{shouldFail: true})
	store := NewMockPostgresStore()
	logger, _ := zap.NewDevelopment()

	handler := NewWebhookTelemetryHandler(registry, store, logger)

	events := []map[string]interface{}{
		{
			"source_vendor": "good-vendor",
			"event_type":    "test.event1",
		},
		{
			"source_vendor": "bad-vendor",
			"event_type":    "test.event2",
		},
	}

	body, _ := json.Marshal(events)
	req := httptest.NewRequest("POST", "/webhook/batch", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.HandleWebhookBatch(w, req)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	results, ok := response["results"].([]interface{})
	if !ok {
		t.Errorf("expected results array in response")
		return
	}

	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	result1 := results[0].(map[string]interface{})
	if result1["status"] != "accepted" {
		t.Errorf("expected first result status 'accepted', got '%v'", result1["status"])
	}

	result2 := results[1].(map[string]interface{})
	if result2["status"] != "error" {
		t.Errorf("expected second result status 'error', got '%v'", result2["status"])
	}
}
