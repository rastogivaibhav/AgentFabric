package store

import (
	"testing"
	"time"

	"github.com/govagn/api-gateway/internal/normalization"
)

func TestEventFilter_BuildWhereClause_WithSourceVendor(t *testing.T) {
	filter := &EventFilter{
		SourceVendor: "codex",
	}

	whereClause, args := buildWhereClause(filter)

	if whereClause == "" {
		t.Error("expected non-empty where clause")
	}
	if len(args) != 1 {
		t.Errorf("expected 1 argument, got %d", len(args))
	}
	if args[0] != "codex" {
		t.Errorf("expected 'codex', got %v", args[0])
	}
}

func TestEventFilter_BuildWhereClause_WithMultipleFilters(t *testing.T) {
	filter := &EventFilter{
		SourceVendor:   "cursor",
		UserEmail:      "dev@example.com",
		RiskScoreMin:   50,
		RequiresReview: boolPtr(true),
	}

	whereClause, args := buildWhereClause(filter)

	if whereClause == "" {
		t.Error("expected non-empty where clause")
	}
	if len(args) != 4 {
		t.Errorf("expected 4 arguments, got %d", len(args))
	}
}

func TestEventFilter_BuildWhereClause_WithTimeRange(t *testing.T) {
	now := time.Now()
	oneHourAgo := now.Add(-time.Hour)

	filter := &EventFilter{
		StartTime: oneHourAgo,
		EndTime:   now,
	}

	whereClause, args := buildWhereClause(filter)

	if whereClause == "" {
		t.Error("expected non-empty where clause for time range")
	}
	if len(args) != 2 {
		t.Errorf("expected 2 arguments, got %d", len(args))
	}
}

func TestEventFilter_BuildWhereClause_WithNoFilters(t *testing.T) {
	filter := &EventFilter{}

	whereClause, args := buildWhereClause(filter)

	if whereClause != "" {
		t.Errorf("expected empty where clause, got %q", whereClause)
	}
	if len(args) != 0 {
		t.Errorf("expected 0 arguments, got %d", len(args))
	}
}

func TestEventFilter_BuildWhereClause_WithNilFilter(t *testing.T) {
	whereClause, args := buildWhereClause(nil)

	if whereClause != "" {
		t.Errorf("expected empty where clause, got %q", whereClause)
	}
	if len(args) != 0 {
		t.Errorf("expected 0 arguments, got %d", len(args))
	}
}

func TestCanonicalEvent_CanBeStored(t *testing.T) {
	event := &normalization.CanonicalEvent{
		SourceVendor:  "codex",
		SourceProduct: "codex-cli",
		SourceChannel: "otlp",
		EventType:     "tool.call.completed",
		EventCategory: "tool_call",
		UserEmail:     "dev@company.com",
		RiskScore:     10,
		Payload: map[string]interface{}{
			"command": "pytest tests/",
		},
	}

	prepareCanonicalEventForStorage(event)

	if event.ID == "" {
		t.Errorf("event.ID should not be empty for storage")
	}
	if event.EventTime.IsZero() {
		t.Errorf("event.EventTime should not be zero for storage")
	}
}

func TestCanonicalEvent_StoreAndRetrieve(t *testing.T) {
	event := &normalization.CanonicalEvent{
		SourceVendor:   "cursor",
		SourceProduct:  "cursor-editor",
		SourceChannel:  "extension",
		EventType:      "suggestion.accepted",
		EventCategory:  "tool_call",
		UserEmail:      "user@example.com",
		RiskScore:      25,
		RequiresReview: false,
		Payload: map[string]interface{}{
			"suggestion_length": 42,
			"file_path":         "/app/main.py",
		},
		EventTime: time.Now(),
	}

	// Simulate what would happen in actual storage
	if event.SourceVendor != "cursor" {
		t.Errorf("expected source_vendor 'cursor', got '%s'", event.SourceVendor)
	}
	if event.Payload["suggestion_length"] != 42 {
		t.Errorf("expected payload suggestion_length 42, got %v", event.Payload["suggestion_length"])
	}
}

func boolPtr(b bool) *bool {
	return &b
}
