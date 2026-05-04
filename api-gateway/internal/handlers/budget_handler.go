package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/govagn/api-gateway/internal/budget"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// ─── Budget REST Endpoints ────────────────────────────────────────────────────

// GetBudget returns the current budget + usage for a tenant.
// GET /api/v1/budgets/{tenant_id}
func (h *Handler) GetBudget(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "tenant_id required")
		return
	}

	b, err := h.pg.GetBudget(r.Context(), tenantID)
	if err != nil {
		h.logger.Error("get budget", zap.String("tenant", tenantID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "query error")
		return
	}
	if b == nil {
		writeJSON(w, http.StatusOK, &budget.Budget{
			TenantID:       tenantID,
			MonthlyTokens:  0,
			MonthlyCostUSD: 0,
			AlertThreshold: 0.80,
			HardLimit:      false,
			ResetDay:       1,
		})
		return
	}
	writeJSON(w, http.StatusOK, b)
}

// UpsertBudget sets or updates a budget for a tenant.
// PUT /api/v1/budgets/{tenant_id}
func (h *Handler) UpsertBudget(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "tenant_id required")
		return
	}

	var req struct {
		MonthlyTokens  int64   `json:"monthly_tokens"`
		MonthlyCostUSD float64 `json:"monthly_cost_usd"`
		AlertThreshold float64 `json:"alert_threshold"`
		HardLimit      bool    `json:"hard_limit"`
		ResetDay       int     `json:"reset_day"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	if req.AlertThreshold == 0 {
		req.AlertThreshold = 0.80
	}
	if req.ResetDay <= 0 || req.ResetDay > 28 {
		req.ResetDay = 1
	}
	before, _ := h.pg.GetBudget(r.Context(), tenantID)

	b := &budget.Budget{
		TenantID:       tenantID,
		MonthlyTokens:  req.MonthlyTokens,
		MonthlyCostUSD: req.MonthlyCostUSD,
		AlertThreshold: req.AlertThreshold,
		HardLimit:      req.HardLimit,
		ResetDay:       req.ResetDay,
	}
	if err := h.pg.UpsertBudget(r.Context(), b); err != nil {
		h.logger.Error("upsert budget", zap.String("tenant", tenantID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	h.writeAdminAudit(r, "budgets", "upsert", "budget", tenantID, "success", map[string]any{"before": before, "after": b})
	h.writeControlHistory(r, "budgets", "upsert", "budget", tenantID, "budget updated", "success", before, b, nil)
	writeJSON(w, http.StatusOK, b)
}

// DeleteBudget removes budget limits for a tenant (sets to unlimited).
// DELETE /api/v1/budgets/{tenant_id}
func (h *Handler) DeleteBudget(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "tenant_id required")
		return
	}

	before, _ := h.pg.GetBudget(r.Context(), tenantID)
	b := &budget.Budget{
		TenantID:       tenantID,
		MonthlyTokens:  0,
		MonthlyCostUSD: 0,
		AlertThreshold: 0.80,
		HardLimit:      false,
		ResetDay:       1,
	}
	if err := h.pg.UpsertBudget(r.Context(), b); err != nil {
		h.logger.Error("delete budget", zap.String("tenant", tenantID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	h.writeAdminAudit(r, "budgets", "delete", "budget", tenantID, "success", before)
	h.writeControlHistory(r, "budgets", "delete", "budget", tenantID, "budget removed", "success", before, b, nil)
	w.WriteHeader(http.StatusNoContent)
}

// GetBudgetAlerts returns triggered alerts for a tenant.
// GET /api/v1/budgets/{tenant_id}/alerts
func (h *Handler) GetBudgetAlerts(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "tenant_id required")
		return
	}
	limit := parseIntOr(r.URL.Query().Get("limit"), 50)

	alerts, err := h.pg.ListBudgetAlerts(r.Context(), tenantID, limit)
	if err != nil {
		h.logger.Error("list budget alerts", zap.String("tenant", tenantID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "query error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items": alerts,
		"count": len(alerts),
	})
}

// GetBudgetUsage returns current period token + cost totals for a tenant.
// GET /api/v1/budgets/{tenant_id}/usage
func (h *Handler) GetBudgetUsage(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "tenant_id required")
		return
	}

	b, err := h.pg.GetBudget(r.Context(), tenantID)
	if err != nil {
		h.logger.Error("get budget for usage", zap.String("tenant", tenantID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "query error")
		return
	}

	resetDay := 1
	if b != nil {
		resetDay = b.ResetDay
	}

	usage, err := h.pg.GetMonthlyUsage(r.Context(), tenantID, periodStart(resetDay))
	if err != nil {
		h.logger.Error("get monthly usage", zap.String("tenant", tenantID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "query error")
		return
	}

	resp := map[string]interface{}{
		"tokens_used":   usage.TokensUsed,
		"cost_used_usd": usage.CostUsedUSD,
		"period_start":  usage.PeriodStart,
		"period_end":    usage.PeriodEnd,
	}
	if b != nil {
		resp["budget"] = b
		if b.MonthlyTokens > 0 {
			resp["tokens_pct"] = float64(usage.TokensUsed) / float64(b.MonthlyTokens)
		}
		if b.MonthlyCostUSD > 0 {
			resp["cost_pct"] = usage.CostUsedUSD / b.MonthlyCostUSD
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// periodStart returns the UTC start of the current billing period for a given reset day.
func periodStart(resetDay int) time.Time {
	now := time.Now().UTC()
	year, month, day := now.Date()
	if day >= resetDay {
		return time.Date(year, month, resetDay, 0, 0, 0, 0, time.UTC)
	}
	prev := month - 1
	prevYear := year
	if prev < 1 {
		prev = 12
		prevYear--
	}
	return time.Date(prevYear, time.Month(prev), resetDay, 0, 0, 0, 0, time.UTC)
}
