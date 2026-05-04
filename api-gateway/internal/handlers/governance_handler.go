package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/govagn/api-gateway/internal/models"
)

func (h *Handler) ListGovernanceAlerts(w http.ResponseWriter, r *http.Request) {
	limit := parseIntOr(r.URL.Query().Get("limit"), 50)
	alerts, err := h.pg.ListGovernanceAlerts(r.Context(), tenantFromCtx(r), limit)
	if err != nil {
		h.logger.Error("list governance alerts")
		writeError(w, http.StatusInternalServerError, "failed to query governance alerts")
		return
	}
	writeJSON(w, http.StatusOK, alerts)
}

func (h *Handler) GetGovernanceSummary(w http.ResponseWriter, r *http.Request) {
	summary, err := h.pg.GetGovernanceSummary(r.Context(), tenantFromCtx(r))
	if err != nil {
		h.logger.Error("get governance summary")
		writeError(w, http.StatusInternalServerError, "failed to query governance summary")
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (h *Handler) ApproveGovernanceDecision(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SpanID   string `json:"span_id"`
		Decision string `json:"decision"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.SpanID = strings.TrimSpace(req.SpanID)
	req.Decision = strings.ToLower(strings.TrimSpace(req.Decision))
	if req.SpanID == "" || (req.Decision != "approved" && req.Decision != "rejected") {
		writeError(w, http.StatusBadRequest, "span_id and approved/rejected decision are required")
		return
	}

	h.writeControlHistory(
		r,
		"governance",
		req.Decision,
		"span",
		req.SpanID,
		"governance decision recorded from portal",
		"success",
		nil,
		map[string]string{"decision": req.Decision, "span_id": req.SpanID},
		[]string{req.SpanID},
	)
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

var _ = models.GovernanceAlert{}
