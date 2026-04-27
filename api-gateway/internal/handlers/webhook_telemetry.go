package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/govagn/api-gateway/internal/normalization"
	"go.uber.org/zap"
)

// RegistryService defines the interface for accessing event mappers
type RegistryService interface {
	Get(vendor string) (normalization.EventMapper, error)
	ListVendors() []string
}

// EventStore defines the interface for storing events
type EventStore interface {
	StoreCanonicalEvent(ctx context.Context, event *normalization.CanonicalEvent) error
}

// WebhookTelemetryHandler handles incoming webhook telemetry from custom integrations
type WebhookTelemetryHandler struct {
	registry RegistryService
	store    EventStore
	logger   *zap.Logger
}

// NewWebhookTelemetryHandler creates a new webhook handler
func NewWebhookTelemetryHandler(
	registry RegistryService,
	store EventStore,
	logger *zap.Logger,
) *WebhookTelemetryHandler {
	return &WebhookTelemetryHandler{
		registry: registry,
		store:    store,
		logger:   logger,
	}
}

// HandleWebhookEvent processes an incoming webhook event
func (h *WebhookTelemetryHandler) HandleWebhookEvent(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse incoming event
	var rawEvent map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&rawEvent); err != nil {
		h.logger.Warn("failed to decode webhook event", zap.Error(err))
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Extract source vendor (required)
	sourceVendor, ok := rawEvent["source_vendor"].(string)
	if !ok || sourceVendor == "" {
		h.logger.Warn("webhook event missing source_vendor")
		http.Error(w, "missing source_vendor", http.StatusBadRequest)
		return
	}

	// Get the appropriate mapper from registry
	mapper, err := h.registry.Get(sourceVendor)
	if err != nil {
		h.logger.Warn("no mapper for vendor", zap.String("vendor", sourceVendor))
		http.Error(w, fmt.Sprintf("unsupported vendor: %s", sourceVendor), http.StatusBadRequest)
		return
	}

	// Map to canonical event
	canonical, err := mapper.Map(rawEvent)
	if err != nil {
		h.logger.Warn("failed to map event", zap.Error(err), zap.String("vendor", sourceVendor))
		http.Error(w, "failed to process event", http.StatusBadRequest)
		return
	}

	// Store the event
	if err := h.store.StoreCanonicalEvent(ctx, canonical); err != nil {
		h.logger.Error("failed to store event", zap.Error(err))
		http.Error(w, "failed to store event", http.StatusInternalServerError)
		return
	}

	h.logger.Debug("webhook event stored",
		zap.String("vendor", sourceVendor),
		zap.String("event_type", canonical.EventType),
		zap.String("event_id", canonical.ID),
	)

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "accepted",
		"event_id": canonical.ID,
	})
}

// HandleWebhookBatch processes a batch of webhook events
func (h *WebhookTelemetryHandler) HandleWebhookBatch(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse batch of events
	var events []map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&events); err != nil {
		h.logger.Warn("failed to decode webhook batch", zap.Error(err))
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if len(events) == 0 {
		http.Error(w, "empty batch", http.StatusBadRequest)
		return
	}

	if len(events) > 1000 {
		http.Error(w, "batch too large (max 1000)", http.StatusBadRequest)
		return
	}

	// Process each event
	results := make([]map[string]interface{}, 0, len(events))
	successCount := 0
	failureCount := 0

	for _, rawEvent := range events {
		sourceVendor, ok := rawEvent["source_vendor"].(string)
		if !ok || sourceVendor == "" {
			failureCount++
			results = append(results, map[string]interface{}{
				"status": "error",
				"error":  "missing source_vendor",
			})
			continue
		}

		// Get mapper
		mapper, err := h.registry.Get(sourceVendor)
		if err != nil {
			failureCount++
			results = append(results, map[string]interface{}{
				"status": "error",
				"error":  fmt.Sprintf("unsupported vendor: %s", sourceVendor),
			})
			continue
		}

		// Map to canonical
		canonical, err := mapper.Map(rawEvent)
		if err != nil {
			failureCount++
			results = append(results, map[string]interface{}{
				"status": "error",
				"error":  "failed to map event",
			})
			continue
		}

		// Store event
		if err := h.store.StoreCanonicalEvent(ctx, canonical); err != nil {
			failureCount++
			results = append(results, map[string]interface{}{
				"status": "error",
				"error":  "failed to store event",
			})
			continue
		}

		successCount++
		results = append(results, map[string]interface{}{
			"status":   "accepted",
			"event_id": canonical.ID,
		})
	}

	h.logger.Info("webhook batch processed",
		zap.Int("total", len(events)),
		zap.Int("success", successCount),
		zap.Int("failure", failureCount),
	)

	// Return batch results
	w.Header().Set("Content-Type", "application/json")
	if failureCount > 0 {
		w.WriteHeader(http.StatusPartialContent)
	} else {
		w.WriteHeader(http.StatusAccepted)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"total":   len(events),
		"success": successCount,
		"failure": failureCount,
		"results": results,
	})
}

// HandleWebhookHealth returns health status of webhook endpoint
func (h *WebhookTelemetryHandler) HandleWebhookHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	vendors := h.registry.ListVendors()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":        "healthy",
		"supported_vendors": vendors,
		"max_batch_size": 1000,
		"timeout_seconds": 30,
	})
}
