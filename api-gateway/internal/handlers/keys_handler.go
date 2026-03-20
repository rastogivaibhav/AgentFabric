package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/agentfabric/api-gateway/internal/middleware"
	"github.com/agentfabric/api-gateway/internal/vault"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// KeyHandler handles virtual key CRUD for Layer 2.
// Mounted at /api/v1/keys inside the JWT-auth route group.
type KeyHandler struct {
	vault  *vault.Vault
	logger *zap.Logger
}

func NewKeyHandler(v *vault.Vault, logger *zap.Logger) *KeyHandler {
	return &KeyHandler{vault: v, logger: logger}
}

// ─── POST /api/v1/keys ───────────────────────────────────────────────────────

type registerKeyRequest struct {
	Provider    string `json:"provider"`    // openai | anthropic | google
	RealKey     string `json:"real_key"`    // sk-... or sk-ant-...
	DisplayName string `json:"display_name"`
	TeamID      string `json:"team_id,omitempty"`
}

type registerKeyResponse struct {
	VirtualKey  string `json:"virtual_key"`
	Provider    string `json:"provider"`
	DisplayName string `json:"display_name"`
	KeyID       string `json:"key_id"` // first 8 chars — confirm which key was stored
}

// RegisterKey registers a real LLM key and returns a virtual key.
// The real key is encrypted and never returned again.
func (h *KeyHandler) RegisterKey(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantIDFromCtx(r.Context())

	var req registerKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	req.Provider = strings.ToLower(strings.TrimSpace(req.Provider))
	req.RealKey = strings.TrimSpace(req.RealKey)
	req.DisplayName = strings.TrimSpace(req.DisplayName)

	if req.Provider == "" || req.RealKey == "" || req.DisplayName == "" {
		http.Error(w, "provider, real_key, and display_name are required", http.StatusBadRequest)
		return
	}

	switch req.Provider {
	case "openai", "anthropic", "google":
		// valid
	default:
		http.Error(w, "provider must be one of: openai, anthropic, google", http.StatusBadRequest)
		return
	}

	vk, err := h.vault.Store(r.Context(), tenantID, req.Provider, req.RealKey, req.DisplayName)
	if err != nil {
		h.logger.Error("failed to store virtual key", zap.Error(err))
		http.Error(w, "failed to register key", http.StatusInternalServerError)
		return
	}

	keyID := req.RealKey
	if len(keyID) > 8 {
		keyID = keyID[:8]
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(registerKeyResponse{ //nolint:errcheck
		VirtualKey:  vk,
		Provider:    req.Provider,
		DisplayName: req.DisplayName,
		KeyID:       keyID,
	})
}

// ─── GET /api/v1/keys ────────────────────────────────────────────────────────

// ListKeys returns all virtual keys for the tenant. Real keys are never included.
func (h *KeyHandler) ListKeys(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantIDFromCtx(r.Context())

	keys, err := h.vault.List(r.Context(), tenantID)
	if err != nil {
		h.logger.Error("failed to list keys", zap.Error(err))
		http.Error(w, "failed to list keys", http.StatusInternalServerError)
		return
	}

	type keyItem struct {
		VirtualKey  string  `json:"virtual_key"`
		DisplayName string  `json:"display_name"`
		Provider    string  `json:"provider"`
		KeyID       string  `json:"key_id"`
		TeamID      string  `json:"team_id,omitempty"`
		LastUsedAt  *string `json:"last_used_at,omitempty"`
		Revoked     bool    `json:"revoked"`
		CreatedAt   string  `json:"created_at"`
	}

	items := make([]keyItem, 0, len(keys))
	for _, k := range keys {
		item := keyItem{
			VirtualKey:  k.VirtualKey,
			DisplayName: k.DisplayName,
			Provider:    k.Provider,
			KeyID:       k.KeyID,
			TeamID:      k.TeamID,
			Revoked:     k.Revoked,
			CreatedAt:   k.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
		if k.LastUsedAt != nil {
			s := k.LastUsedAt.Format("2006-01-02T15:04:05Z")
			item.LastUsedAt = &s
		}
		items = append(items, item)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"items": items, "count": len(items)}) //nolint:errcheck
}

// ─── DELETE /api/v1/keys/{virtualKey} ────────────────────────────────────────

// RevokeKey revokes a virtual key for the tenant. Immediate effect.
func (h *KeyHandler) RevokeKey(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantIDFromCtx(r.Context())
	vk := chi.URLParam(r, "virtualKey")

	if err := h.vault.Revoke(r.Context(), vk, tenantID); err != nil {
		if err == vault.ErrInvalidKey {
			http.Error(w, "key not found", http.StatusNotFound)
			return
		}
		h.logger.Error("revoke failed", zap.Error(err))
		http.Error(w, "failed to revoke key", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "revoked", "virtual_key": vk}) //nolint:errcheck
}
