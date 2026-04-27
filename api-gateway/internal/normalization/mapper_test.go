package normalization

import (
	"testing"
	"time"
)

// MockCodexMapper is a test implementation of EventMapper
type MockCodexMapper struct{}

func (m *MockCodexMapper) Map(event interface{}) (*CanonicalEvent, error) {
	return &CanonicalEvent{
		SourceVendor: "codex",
		EventType:    "test.event",
		EventTime:    time.Now(),
	}, nil
}

func (m *MockCodexMapper) Accepts(sourceVendor, sourceProduct string) bool {
	return sourceVendor == "codex"
}

// MockCursorMapper is a test implementation of EventMapper
type MockCursorMapper struct{}

func (m *MockCursorMapper) Map(event interface{}) (*CanonicalEvent, error) {
	return &CanonicalEvent{
		SourceVendor: "cursor",
		EventType:    "test.event",
		EventTime:    time.Now(),
	}, nil
}

func (m *MockCursorMapper) Accepts(sourceVendor, sourceProduct string) bool {
	return sourceVendor == "cursor"
}

func TestMapperRegistry_Register(t *testing.T) {
	registry := NewMapperRegistry()
	mapper := &MockCodexMapper{}

	registry.Register("codex", mapper)

	vendors := registry.ListVendors()
	if len(vendors) != 1 || vendors[0] != "codex" {
		t.Errorf("expected 1 vendor 'codex', got %v", vendors)
	}
}

func TestMapperRegistry_SelectsCorrectMapper(t *testing.T) {
	registry := NewMapperRegistry()
	registry.Register("codex", &MockCodexMapper{})
	registry.Register("cursor", &MockCursorMapper{})

	// Test codex mapper selection
	event, err := registry.Map("codex", nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if event.SourceVendor != "codex" {
		t.Errorf("expected source_vendor 'codex', got '%s'", event.SourceVendor)
	}

	// Test cursor mapper selection
	event, err = registry.Map("cursor", nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if event.SourceVendor != "cursor" {
		t.Errorf("expected source_vendor 'cursor', got '%s'", event.SourceVendor)
	}
}

func TestMapperRegistry_ReturnsErrorForUnregisteredVendor(t *testing.T) {
	registry := NewMapperRegistry()
	registry.Register("codex", &MockCodexMapper{})

	_, err := registry.Map("unknown_vendor", nil)
	if err == nil {
		t.Error("expected error for unregistered vendor")
	}
}

func TestMapperRegistry_Get(t *testing.T) {
	registry := NewMapperRegistry()
	mapper := &MockCodexMapper{}
	registry.Register("codex", mapper)

	retrieved, err := registry.Get("codex")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if retrieved == nil {
		t.Error("expected mapper to be returned")
	}
}

func TestMapperRegistry_GetReturnsErrorForUnregisteredVendor(t *testing.T) {
	registry := NewMapperRegistry()

	_, err := registry.Get("nonexistent")
	if err == nil {
		t.Error("expected error for unregistered vendor")
	}
}

func TestMapperRegistry_ListVendors(t *testing.T) {
	registry := NewMapperRegistry()
	registry.Register("codex", &MockCodexMapper{})
	registry.Register("cursor", &MockCursorMapper{})

	vendors := registry.ListVendors()
	if len(vendors) != 2 {
		t.Errorf("expected 2 vendors, got %d", len(vendors))
	}

	vendorMap := make(map[string]bool)
	for _, v := range vendors {
		vendorMap[v] = true
	}

	if !vendorMap["codex"] || !vendorMap["cursor"] {
		t.Errorf("expected vendors 'codex' and 'cursor', got %v", vendors)
	}
}

func TestGlobalRegistry_RegisterMapper(t *testing.T) {
	// Save and restore global registry state
	originalRegistry := globalRegistry
	globalRegistry = NewMapperRegistry()
	defer func() { globalRegistry = originalRegistry }()

	RegisterMapper("codex", &MockCodexMapper{})
	RegisterMapper("cursor", &MockCursorMapper{})

	registry := GetRegistry()
	vendors := registry.ListVendors()
	if len(vendors) != 2 {
		t.Errorf("expected 2 vendors in global registry, got %d", len(vendors))
	}
}

func TestCanonicalEvent_HasExpandedFields(t *testing.T) {
	event := &CanonicalEvent{
		ID:            "event_123",
		SourceVendor:  "cursor",
		SourceProduct: "cursor-editor",
		SourceChannel: "extension",
		EventCategory: "tool_call",
		Action:        "code_generation_accepted",
		Success:       true,
		LatencyMs:     1200,
		InputTokens:   1000,
		OutputTokens:  500,
		RiskScore:     25,
		RiskCategory:  "none",
		Payload: map[string]interface{}{
			"suggestion_length": 42,
		},
		Redacted: true,
	}

	if event.ID == "" {
		t.Error("event should have ID")
	}
	if event.SourceVendor != "cursor" {
		t.Errorf("expected source_vendor 'cursor', got '%s'", event.SourceVendor)
	}
	if event.EventCategory != "tool_call" {
		t.Errorf("expected event_category 'tool_call', got '%s'", event.EventCategory)
	}
	if !event.Success {
		t.Error("expected success to be true")
	}
	if event.LatencyMs != 1200 {
		t.Errorf("expected latency 1200ms, got %d", event.LatencyMs)
	}
	if event.Payload["suggestion_length"] != 42 {
		t.Errorf("expected payload suggestion_length 42, got %v", event.Payload["suggestion_length"])
	}
}
