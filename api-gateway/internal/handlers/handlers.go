package handlers

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/govagn/api-gateway/internal/budget"
	"github.com/govagn/api-gateway/internal/evals"
	"github.com/govagn/api-gateway/internal/governance"
	"github.com/govagn/api-gateway/internal/managedruntime"
	"github.com/govagn/api-gateway/internal/memory"
	"github.com/govagn/api-gateway/internal/middleware"
	"github.com/govagn/api-gateway/internal/models"
	"github.com/govagn/api-gateway/internal/normalization"
	"github.com/govagn/api-gateway/internal/observability"
	"github.com/govagn/api-gateway/internal/policy"
	"github.com/govagn/api-gateway/internal/pricing"
	"github.com/govagn/api-gateway/internal/prompts"
	"github.com/govagn/api-gateway/internal/proxy"
	"github.com/govagn/api-gateway/internal/recommendations"
	"github.com/govagn/api-gateway/internal/rollouts"
	"github.com/govagn/api-gateway/internal/store"
	"github.com/govagn/api-gateway/internal/ws"
	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"
)

type Handler struct {
	pg             *store.PostgresStore
	redis          *store.RedisClient
	hub            *ws.Hub
	logger         *zap.Logger
	jwtSecret      string
	budgetEnforcer *budget.BudgetEnforcer
	policyEngine   *policy.Engine
	riskEngine     *governance.RiskEngine
	policyPackRoot string
	evalPackRoot   string
}

func New(pg *store.PostgresStore, redis *store.RedisClient, hub *ws.Hub, logger *zap.Logger, jwtSecret string, budgetEnforcer *budget.BudgetEnforcer, policyEngine *policy.Engine, riskEngine *governance.RiskEngine) *Handler {
	return NewWithPackRoots(pg, redis, hub, logger, jwtSecret, budgetEnforcer, policyEngine, riskEngine, "", "")
}

func NewWithPackRoots(pg *store.PostgresStore, redis *store.RedisClient, hub *ws.Hub, logger *zap.Logger, jwtSecret string, budgetEnforcer *budget.BudgetEnforcer, policyEngine *policy.Engine, riskEngine *governance.RiskEngine, policyPackRoot, evalPackRoot string) *Handler {
	return &Handler{
		pg:             pg,
		redis:          redis,
		hub:            hub,
		logger:         logger,
		jwtSecret:      jwtSecret,
		budgetEnforcer: budgetEnforcer,
		policyEngine:   policyEngine,
		riskEngine:     riskEngine,
		policyPackRoot: strings.TrimSpace(policyPackRoot),
		evalPackRoot:   strings.TrimSpace(evalPackRoot),
	}
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// isNotFound returns true when err is a pgx "no rows" sentinel,
// used to translate DB misses into HTTP 404 responses.
func isNotFound(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func parseBoolOr(raw string, def bool) bool {
	raw = strings.TrimSpace(strings.ToLower(raw))
	switch raw {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}

func parseInt64Or(raw string, def int64) int64 {
	if trimmed := strings.TrimSpace(raw); trimmed != "" {
		if value, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
			return value
		}
	}
	return def
}

func tenantFromCtx(r *http.Request) string {
	return middleware.TenantIDFromCtx(r.Context())
}

func actorFromCtx(r *http.Request) string {
	claims := middleware.ClaimsFromCtx(r.Context())
	if claims == nil {
		return ""
	}
	switch {
	case strings.TrimSpace(claims.Name) != "":
		return strings.TrimSpace(claims.Name)
	case strings.TrimSpace(claims.Email) != "":
		return strings.TrimSpace(claims.Email)
	default:
		return strings.TrimSpace(claims.Subject)
	}
}

func (h *Handler) managedRuntimeService() managedruntime.Service {
	return managedruntime.NewService(h.pg)
}

func (h *Handler) evalService() *evals.Service {
	return evals.NewServiceWithPackRoot(h.pg, h.resolvedEvalPackRoot())
}

func (h *Handler) resolvedPolicyPackRoot() string {
	if strings.TrimSpace(h.policyPackRoot) != "" {
		return strings.TrimSpace(h.policyPackRoot)
	}
	return "deploy/seed/policy-packs"
}

func (h *Handler) resolvedEvalPackRoot() string {
	if strings.TrimSpace(h.evalPackRoot) != "" {
		return strings.TrimSpace(h.evalPackRoot)
	}
	return "deploy/seed/eval-packs"
}

func (h *Handler) writeAdminAudit(r *http.Request, category, action, targetType, targetID, outcome string, details any) {
	if h == nil || h.pg == nil {
		return
	}
	detailsJSON := "{}"
	if details != nil {
		if b, err := json.Marshal(details); err == nil {
			detailsJSON = string(b)
		}
	}
	_ = h.pg.CreateAdminAuditEntry(r.Context(), models.AdminAuditEntry{
		TenantID:   tenantFromCtx(r),
		Actor:      actorFromCtx(r),
		Category:   category,
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		Outcome:    outcome,
		Details:    detailsJSON,
	})
}

func jsonStringOrEmpty(value any) string {
	if value == nil {
		return ""
	}
	if raw, ok := value.(string); ok {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return ""
		}
		return trimmed
	}
	if b, err := json.Marshal(value); err == nil {
		return string(b)
	}
	return ""
}

func firstNonEmptyStringPtr(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func (h *Handler) writeControlHistory(r *http.Request, category, action, targetType, targetID, reason, outcome string, before, after any, evidenceRefs []string) {
	if h == nil || h.pg == nil {
		return
	}
	_ = memory.NewService(h.pg).RecordChange(r.Context(), models.ControlHistoryEntry{
		TenantID:     tenantFromCtx(r),
		Category:     category,
		Action:       action,
		TargetType:   targetType,
		TargetID:     targetID,
		Actor:        actorFromCtx(r),
		Reason:       reason,
		Outcome:      outcome,
		BeforeState:  jsonStringOrEmpty(before),
		AfterState:   jsonStringOrEmpty(after),
		EvidenceRefs: evidenceRefs,
	})
}

// ─── Health ──────────────────────────────────────────────────────────────────

// Health checks both Postgres and Redis with a 2-second timeout each.
// Returns 200 {"status":"ok"} when all deps are reachable.
// Returns 503 {"status":"degraded","error":"..."} if any dep fails.
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	h.writeReadiness(w, r, false)
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	h.writeReadiness(w, r, true)
}

func (h *Handler) writeReadiness(w http.ResponseWriter, r *http.Request, ready bool) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	checks := map[string]map[string]any{
		"postgres": {"status": "ok"},
		"redis":    {"status": "ok"},
	}
	response := map[string]any{
		"status": "ok",
		"checks": checks,
	}

	if err := h.pg.Ping(ctx); err != nil {
		h.logger.Warn("readiness: postgres ping failed", zap.Error(err), zap.Bool("ready", ready))
		response["status"] = "degraded"
		response["error"] = "postgres unavailable"
		checks["postgres"] = map[string]any{"status": "unavailable", "error": err.Error()}
		writeJSON(w, http.StatusServiceUnavailable, response)
		return
	}
	if err := h.redis.Ping(ctx); err != nil {
		h.logger.Warn("readiness: redis ping failed", zap.Error(err), zap.Bool("ready", ready))
		response["status"] = "degraded"
		response["error"] = "redis unavailable"
		checks["redis"] = map[string]any{"status": "unavailable", "error": err.Error()}
		writeJSON(w, http.StatusServiceUnavailable, response)
		return
	}
	if ready {
		response["mode"] = "ready"
		pricingRuleCount := proxy.ActivePricingRuleCount()
		if pricingRuleCount <= 0 {
			h.logger.Warn("readiness: no pricing rules active")
			response["status"] = "degraded"
			response["error"] = "pricing rules not loaded"
			checks["pricing_rules"] = map[string]any{"status": "unavailable", "count": pricingRuleCount}
			writeJSON(w, http.StatusServiceUnavailable, response)
			return
		}
		checks["pricing_rules"] = map[string]any{"status": "loaded", "count": pricingRuleCount}

		if h.policyEngine == nil {
			h.logger.Warn("readiness: policy engine unavailable")
			response["status"] = "degraded"
			response["error"] = "policy engine unavailable"
			checks["policy_engine"] = map[string]any{"status": "unavailable", "count": 0}
			writeJSON(w, http.StatusServiceUnavailable, response)
			return
		}
		policyRuleCount := h.policyEngine.RuleCount()
		checks["policy_engine"] = map[string]any{"status": "loaded", "count": policyRuleCount}
		checks["startup_state"] = map[string]any{"status": "healthy"}
	} else {
		response["mode"] = "health"
	}
	writeJSON(w, http.StatusOK, response)
}

// ─── Ingest (internal, called by collector) ──────────────────────────────────

type ingestRequest struct {
	Spans []models.Span `json:"spans"`
}

const (
	firewallReviewThreshold = 70
	firewallBlockThreshold  = 90
)

type ingestDecision struct {
	DecisionID   string `json:"decision_id"`
	TraceID      string `json:"trace_id"`
	SpanID       string `json:"span_id"`
	Result       string `json:"result"`
	Reason       string `json:"reason"`
	RiskScore    int    `json:"risk_score"`
	RiskCategory string `json:"risk_category"`
	ActionTaken  string `json:"action_taken"`
}

type ingestResponse struct {
	Status         string           `json:"status"`
	Accepted       int              `json:"accepted"`
	Blocked        int              `json:"blocked"`
	ReviewRequired bool             `json:"review_required"`
	Decisions      []ingestDecision `json:"decisions"`
}

func (h *Handler) Ingest(w http.ResponseWriter, r *http.Request) {
	const maxBody = 32 << 20 // 32 MiB
	// Fast path: reject immediately if Content-Length is known and too large.
	if r.ContentLength > maxBody {
		writeError(w, http.StatusRequestEntityTooLarge, "request body exceeds 32 MiB limit")
		return
	}
	// Fix D: cap request body at 32 MiB to prevent DoS via unbounded reads.
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)

	var req ingestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// MaxBytesReader wraps the error when the limit is exceeded.
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) || err.Error() == "http: request body too large" {
			writeError(w, http.StatusRequestEntityTooLarge, "request body exceeds 32 MiB limit")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	if len(req.Spans) == 0 {
		w.WriteHeader(http.StatusOK)
		return
	}
	if len(req.Spans) > 10000 {
		writeError(w, http.StatusRequestEntityTooLarge, "batch too large")
		return
	}

	// Tenant from collector header; fall back to the canonical default UUID.
	tenantID := r.Header.Get("X-AF-Tenant")
	if tenantID == "" {
		tenantID = middleware.DefaultTenantID
	}
	for i := range req.Spans {
		req.Spans[i].TenantID = tenantID
		if req.Spans[i].ReceivedAt.IsZero() {
			req.Spans[i].ReceivedAt = time.Now()
		}
		h.repriceSpan(&req.Spans[i])
		h.scoreSpanRisk(&req.Spans[i])
	}

	// Budget enforcement — check before writing to DB.
	// Fail open: if the enforcer errors, we still allow the ingest.
	decisions, blocked := h.evaluateFirewallDecisions(r.Context(), tenantID, req.Spans)
	if blocked > 0 {
		writeJSON(w, http.StatusForbidden, ingestResponse{
			Status:         "blocked",
			Accepted:       0,
			Blocked:        blocked,
			ReviewRequired: true,
			Decisions:      decisions,
		})
		return
	}

	if h.budgetEnforcer != nil {
		totalTokens, totalCost := sumSpanUsage(req.Spans)
		allowed, err := h.budgetEnforcer.CheckAndRecord(r.Context(), tenantID, totalTokens, totalCost)
		if err != nil {
			h.logger.Warn("budget check error (fail open)", zap.String("tenant", tenantID), zap.Error(err))
		}
		if !allowed {
			_ = h.pg.CreateDecisionRecord(r.Context(), models.DecisionRecord{
				DecisionID:  fmt.Sprintf("budget-%d", time.Now().UnixNano()),
				TenantID:    tenantID,
				Type:        models.DecisionTypeBudget,
				Result:      "deny",
				Reason:      "monthly budget exceeded",
				Trigger:     "budget_hard_limit",
				ActionTaken: "reject_ingest_batch",
				Source:      "ingest",
				Framework:   "collector",
				Inputs: map[string]string{
					"batch_tokens": fmt.Sprintf("%d", totalTokens),
					"batch_cost":   fmt.Sprintf("%.8f", totalCost),
				},
			})
			writeError(w, http.StatusTooManyRequests, "monthly budget exceeded")
			return
		}
	}

	if err := h.pg.BulkInsertSpans(r.Context(), req.Spans); err != nil {
		h.logger.Error("bulk insert failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "storage error")
		return
	}

	// Broadcast to WebSocket clients
	if h.hub != nil {
		for _, sp := range req.Spans {
			h.hub.Broadcast(tenantID, &models.LiveEvent{
				Type:      "span",
				Timestamp: time.Now().UnixMilli(),
				TenantID:  tenantID,
				Data:      sp,
			})
		}
	}

	reviewRequired := false
	for _, decision := range decisions {
		if decision.Result == "review" {
			reviewRequired = true
			break
		}
	}
	writeJSON(w, http.StatusOK, ingestResponse{
		Status:         "accepted",
		Accepted:       len(req.Spans),
		Blocked:        0,
		ReviewRequired: reviewRequired,
		Decisions:      decisions,
	})
}

// ─── Traces ──────────────────────────────────────────────────────────────────

func (h *Handler) ListTraces(w http.ResponseWriter, r *http.Request) {
	q := models.TraceQuery{
		TenantID:    tenantFromCtx(r),
		Framework:   r.URL.Query().Get("framework"),
		Model:       r.URL.Query().Get("model"),
		AgentName:   r.URL.Query().Get("agent"),
		Search:      r.URL.Query().Get("search"),
		Provider:    r.URL.Query().Get("provider"),
		AppName:     r.URL.Query().Get("app_name"),
		Environment: r.URL.Query().Get("environment"),
		UserID:      r.URL.Query().Get("user_id"),
		SessionID:   r.URL.Query().Get("session_id"),
		Status:      r.URL.Query().Get("status"),
		BlockedOnly: parseBoolOr(r.URL.Query().Get("blocked"), false),
		Limit:       parseIntOr(r.URL.Query().Get("limit"), 50),
		Cursor:      r.URL.Query().Get("cursor"),
	}
	if s := r.URL.Query().Get("start"); s != "" {
		q.StartTime, _ = strconv.ParseInt(s, 10, 64)
	}
	if e := r.URL.Query().Get("end"); e != "" {
		q.EndTime, _ = strconv.ParseInt(e, 10, 64)
	}

	page, err := h.pg.ListTraces(r.Context(), q)
	if err != nil {
		h.logger.Error("list traces", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "query error")
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *Handler) GetTrace(w http.ResponseWriter, r *http.Request) {
	traceID := chi.URLParam(r, "traceId")
	tenantID := tenantFromCtx(r)

	cacheKey := "trace:" + tenantID + ":" + traceID
	var trace models.Trace
	if err := h.redis.GetJSON(r.Context(), cacheKey, &trace); err == nil {
		writeJSON(w, http.StatusOK, trace)
		return
	}

	inputs, err := h.pg.LoadTraceViewInputs(r.Context(), traceID, tenantID)
	if err != nil || len(inputs.Spans) == 0 {
		writeError(w, http.StatusNotFound, "trace not found")
		return
	}

	enrichedSpans := observability.EnrichSpans(inputs.Spans, nil)
	policyEvents := auditEntriesToPolicyEvents(inputs.AuditEntries, enrichedSpans)
	trace = observability.BuildTrace(traceID, inputs.Spans, policyEvents)
	trace.Timeline = observability.BuildTimeline(traceID, trace.Spans, policyEvents)
	h.redis.SetJSON(r.Context(), cacheKey, trace, 5*time.Minute)
	writeJSON(w, http.StatusOK, trace)
}

func (h *Handler) GetTraceGraph(w http.ResponseWriter, r *http.Request) {
	traceID := chi.URLParam(r, "traceId")
	spans, err := h.pg.GetTraceSpans(r.Context(), traceID, tenantFromCtx(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	graph := buildTopologyGraph(spans)
	writeJSON(w, http.StatusOK, graph)
}

func (h *Handler) GetTraceTimeline(w http.ResponseWriter, r *http.Request) {
	traceID := chi.URLParam(r, "traceId")
	tenantID := tenantFromCtx(r)
	inputs, err := h.pg.LoadTraceViewInputs(r.Context(), traceID, tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	baseSpans := observability.EnrichSpans(inputs.Spans, nil)
	policyEvents := auditEntriesToPolicyEvents(inputs.AuditEntries, baseSpans)
	spans := observability.EnrichSpans(inputs.Spans, policyEvents)
	writeJSON(w, http.StatusOK, observability.BuildTimeline(traceID, spans, policyEvents))
}

func (h *Handler) GetTraceComparison(w http.ResponseWriter, r *http.Request) {
	leftID := strings.TrimSpace(r.URL.Query().Get("left"))
	rightID := strings.TrimSpace(r.URL.Query().Get("right"))
	if leftID == "" || rightID == "" {
		writeError(w, http.StatusBadRequest, "left and right trace IDs are required")
		return
	}
	tenantID := tenantFromCtx(r)
	loadTrace := func(traceID string) (models.Trace, error) {
		inputs, err := h.pg.LoadTraceViewInputs(r.Context(), traceID, tenantID)
		if err != nil {
			return models.Trace{}, err
		}
		enrichedSpans := observability.EnrichSpans(inputs.Spans, nil)
		policyEvents := auditEntriesToPolicyEvents(inputs.AuditEntries, enrichedSpans)
		trace := observability.BuildTrace(traceID, inputs.Spans, policyEvents)
		trace.Timeline = observability.BuildTimeline(traceID, trace.Spans, policyEvents)
		return trace, nil
	}

	left, err := loadTrace(leftID)
	if err != nil || len(left.Spans) == 0 {
		writeError(w, http.StatusNotFound, "left trace not found")
		return
	}
	right, err := loadTrace(rightID)
	if err != nil || len(right.Spans) == 0 {
		writeError(w, http.StatusNotFound, "right trace not found")
		return
	}
	writeJSON(w, http.StatusOK, observability.CompareTraces(left, right))
}

func (h *Handler) ListTraceSavedViews(w http.ResponseWriter, r *http.Request) {
	views, err := h.pg.ListTraceSavedViews(r.Context(), tenantFromCtx(r))
	if err != nil {
		h.logger.Error("list trace saved views", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "query error")
		return
	}
	writeJSON(w, http.StatusOK, views)
}

func (h *Handler) UpsertTraceSavedView(w http.ResponseWriter, r *http.Request) {
	var view models.TraceSavedView
	if err := json.NewDecoder(r.Body).Decode(&view); err != nil {
		writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	view.CreatedBy = actorFromCtx(r)
	saved, err := h.pg.UpsertTraceSavedView(r.Context(), tenantFromCtx(r), view)
	if err != nil {
		h.logger.Error("upsert trace saved view", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "write error")
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (h *Handler) DeleteTraceSavedView(w http.ResponseWriter, r *http.Request) {
	viewID, err := strconv.ParseInt(chi.URLParam(r, "viewId"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid view id")
		return
	}
	if err := h.pg.DeleteTraceSavedView(r.Context(), tenantFromCtx(r), viewID); err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "saved view not found")
			return
		}
		h.logger.Error("delete trace saved view", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "delete error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListEvalRuns(w http.ResponseWriter, r *http.Request) {
	service := h.evalService()
	runs, err := service.ListRuns(r.Context(), tenantFromCtx(r), parseIntOr(r.URL.Query().Get("limit"), 20))
	if err != nil {
		h.logger.Error("list eval runs", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "query error")
		return
	}
	writeJSON(w, http.StatusOK, models.Page[models.TraceEvalRun]{
		Items:   runs,
		Total:   int64(len(runs)),
		HasMore: false,
	})
}

func (h *Handler) ScoreTraceEval(w http.ResponseWriter, r *http.Request) {
	var req models.TraceEvalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	service := h.evalService()
	run, err := service.ScoreTrace(r.Context(), tenantFromCtx(r), req)
	if err != nil {
		h.logger.Error("score trace eval", zap.Error(err), zap.String("trace_id", req.TraceID))
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "trace not found")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.writeAdminAudit(r, "evals", "score", "trace", req.TraceID, "success", map[string]any{
		"eval_run_id":   run.ID,
		"trace_id":      run.TraceID,
		"release_tag":   run.ReleaseTag,
		"eval_suite":    run.EvalSuite,
		"overall_score": run.OverallScore,
	})
	writeJSON(w, http.StatusOK, run)
}

func (h *Handler) ExecuteEvalPack(w http.ResponseWriter, r *http.Request) {
	var req models.TraceEvalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	service := h.evalService()
	execution, run, err := service.ExecutePack(r.Context(), tenantFromCtx(r), req)
	if err != nil {
		h.logger.Error("execute eval pack", zap.Error(err), zap.String("pack_id", firstNonEmptyString(req.PackID, req.EvalSuite)))
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "eval subject not found")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.writeAdminAudit(r, "evals", "execute", "pack", execution.PackID, "success", map[string]any{
		"execution_id":   execution.ID,
		"eval_run_id":    run.ID,
		"pack_id":        execution.PackID,
		"overall_score":  execution.OverallScore,
		"execution_mode": execution.Mode,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"execution": execution,
		"run":       run,
	})
}

func (h *Handler) ListEvalExecutions(w http.ResponseWriter, r *http.Request) {
	service := h.evalService()
	items, err := service.ListExecutions(r.Context(), tenantFromCtx(r), parseIntOr(r.URL.Query().Get("limit"), 20))
	if err != nil {
		h.logger.Error("list eval executions", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "query error")
		return
	}
	writeJSON(w, http.StatusOK, models.Page[models.EvalExecution]{
		Items:   items,
		Total:   int64(len(items)),
		HasMore: false,
	})
}

func (h *Handler) GetEvalExecution(w http.ResponseWriter, r *http.Request) {
	executionID := parseInt64Or(chi.URLParam(r, "executionId"), 0)
	if executionID <= 0 {
		writeError(w, http.StatusBadRequest, "execution id is required")
		return
	}
	service := h.evalService()
	item, err := service.GetExecution(r.Context(), tenantFromCtx(r), executionID)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "eval execution not found")
			return
		}
		h.logger.Error("get eval execution", zap.Error(err), zap.Int64("execution_id", executionID))
		writeError(w, http.StatusInternalServerError, "query error")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) ListEvalDatasets(w http.ResponseWriter, r *http.Request) {
	service := h.evalService()
	items, err := service.ListDatasets(r.Context(), tenantFromCtx(r), parseIntOr(r.URL.Query().Get("limit"), 50))
	if err != nil {
		h.logger.Error("list eval datasets", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "query error")
		return
	}
	writeJSON(w, http.StatusOK, models.Page[models.EvalDataset]{
		Items:   items,
		Total:   int64(len(items)),
		HasMore: false,
	})
}

func (h *Handler) UpsertEvalDataset(w http.ResponseWriter, r *http.Request) {
	var dataset models.EvalDataset
	if err := json.NewDecoder(r.Body).Decode(&dataset); err != nil {
		writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	if strings.TrimSpace(dataset.DatasetID) == "" || strings.TrimSpace(dataset.Version) == "" {
		writeError(w, http.StatusBadRequest, "dataset_id and version are required")
		return
	}
	service := h.evalService()
	saved, err := service.UpsertDataset(r.Context(), tenantFromCtx(r), dataset)
	if err != nil {
		h.logger.Error("upsert eval dataset", zap.Error(err), zap.String("dataset_id", dataset.DatasetID))
		writeError(w, http.StatusInternalServerError, "save failed")
		return
	}
	h.writeAdminAudit(r, "evals", "upsert_dataset", "dataset", saved.Ref, "success", map[string]any{
		"dataset_id": saved.DatasetID,
		"version":    saved.Version,
		"item_count": len(saved.Items),
	})
	writeJSON(w, http.StatusOK, saved)
}

func (h *Handler) CompareEvalRegressions(w http.ResponseWriter, r *http.Request) {
	var req models.RegressionCompareRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	service := h.evalService()
	report, err := service.CompareRelease(r.Context(), tenantFromCtx(r), req)
	if err != nil {
		h.logger.Error("compare eval regressions", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "comparison error")
		return
	}
	h.writeAdminAudit(r, "evals", "regression_compare", "release", req.CandidateTag, "success", map[string]any{
		"baseline_tag":  req.BaselineTag,
		"candidate_tag": req.CandidateTag,
		"eval_suite":    report.EvalSuite,
		"overall_delta": report.OverallDelta,
		"risk_level":    report.RiskLevel,
	})
	writeJSON(w, http.StatusOK, report)
}

func (h *Handler) ListPrompts(w http.ResponseWriter, r *http.Request) {
	service := prompts.NewService(h.pg)
	catalog, err := service.ListCatalog(r.Context(), tenantFromCtx(r))
	if err != nil {
		h.logger.Error("list prompts", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "query error")
		return
	}
	writeJSON(w, http.StatusOK, catalog)
}

func (h *Handler) UpsertPromptVersion(w http.ResponseWriter, r *http.Request) {
	var version models.PromptVersion
	if err := json.NewDecoder(r.Body).Decode(&version); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if version.Config == nil {
		version.Config = map[string]string{}
	}
	var before any
	if strings.TrimSpace(version.PromptID) != "" && version.Version > 0 {
		if existing, err := h.pg.GetPromptVersion(r.Context(), tenantFromCtx(r), version.PromptID, version.Version); err == nil {
			before = existing
		}
	}
	service := prompts.NewService(h.pg)
	saved, err := service.UpsertVersion(r.Context(), tenantFromCtx(r), actorFromCtx(r), version)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.writeAdminAudit(r, "prompts", map[bool]string{true: "update", false: "create"}[version.ID > 0 || version.Version > 0], "prompt_version", saved.PromptID, "success", map[string]any{
		"prompt_id":    saved.PromptID,
		"version":      saved.Version,
		"environment":  saved.Environment,
		"release_tag":  saved.ReleaseTag,
		"description":  saved.Description,
		"content_size": len(saved.Content),
	})
	h.writeControlHistory(r, "prompts", map[bool]string{true: "update", false: "create"}[version.ID > 0 || version.Version > 0], "prompt_version", saved.PromptID, saved.ReleaseTag, "success", before, saved, []string{saved.ReleaseTag})
	writeJSON(w, http.StatusOK, saved)
}

func (h *Handler) PromotePromptRelease(w http.ResponseWriter, r *http.Request) {
	var req models.PromptPromotionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	var before any
	if releases, err := h.pg.ListPromptReleases(r.Context(), tenantFromCtx(r)); err == nil {
		for _, candidate := range releases {
			if candidate.PromptID == strings.TrimSpace(req.PromptID) && candidate.Environment == strings.TrimSpace(req.Environment) && candidate.Status == "active" {
				before = candidate
				break
			}
		}
	}
	service := prompts.NewService(h.pg)
	release, err := service.Promote(r.Context(), tenantFromCtx(r), actorFromCtx(r), req)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "prompt version not found")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.writeAdminAudit(r, "prompts", "promote", "prompt_release", release.PromptID, "success", release)
	h.writeControlHistory(r, "prompts", "promote", "prompt_release", release.ReleaseTag, strings.TrimSpace(req.PromotionReason), "success", before, release, []string{release.ReleaseTag})
	writeJSON(w, http.StatusOK, release)
}

func (h *Handler) ListRollouts(w http.ResponseWriter, r *http.Request) {
	service := rollouts.NewService(h.pg)
	items, err := service.ListRules(r.Context(), tenantFromCtx(r))
	if err != nil {
		h.logger.Error("list rollouts", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "query error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "count": len(items)})
}

func (h *Handler) UpsertRolloutRule(w http.ResponseWriter, r *http.Request) {
	var rule models.RolloutRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	var before any
	if rule.ID > 0 {
		service := rollouts.NewService(h.pg)
		if items, err := service.ListRules(r.Context(), tenantFromCtx(r)); err == nil {
			for _, existing := range items {
				if existing.ID == rule.ID {
					before = existing
					break
				}
			}
		}
	}
	service := rollouts.NewService(h.pg)
	saved, err := service.UpsertRule(r.Context(), tenantFromCtx(r), actorFromCtx(r), rule)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.writeAdminAudit(r, "rollouts", map[bool]string{true: "update", false: "create"}[rule.ID > 0], "rollout_rule", strconv.FormatInt(saved.ID, 10), "success", saved)
	h.writeControlHistory(r, "rollouts", map[bool]string{true: "update", false: "create"}[rule.ID > 0], "rollout_rule", strconv.FormatInt(saved.ID, 10), saved.Name, "success", before, saved, []string{saved.CandidateReleaseTag})
	writeJSON(w, http.StatusOK, saved)
}

func (h *Handler) PreviewRollout(w http.ResponseWriter, r *http.Request) {
	var req models.RolloutPreviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.TenantID = tenantFromCtx(r)
	service := rollouts.NewService(h.pg)
	preview, err := service.Preview(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "preview failed")
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (h *Handler) UpdateRolloutStatus(w http.ResponseWriter, r *http.Request) {
	ruleID, err := strconv.ParseInt(chi.URLParam(r, "rolloutId"), 10, 64)
	if err != nil || ruleID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid rollout id")
		return
	}
	var req models.RolloutStatusUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	var before any
	service := rollouts.NewService(h.pg)
	if items, err := service.ListRules(r.Context(), tenantFromCtx(r)); err == nil {
		for _, existing := range items {
			if existing.ID == ruleID {
				before = existing
				break
			}
		}
	}
	updated, err := service.UpdateStatus(r.Context(), tenantFromCtx(r), ruleID, req.Status, actorFromCtx(r))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.writeAdminAudit(r, "rollouts", "status_update", "rollout_rule", strconv.FormatInt(updated.ID, 10), "success", updated)
	h.writeControlHistory(r, "rollouts", "status_update", "rollout_rule", strconv.FormatInt(updated.ID, 10), req.Status, "success", before, updated, nil)
	writeJSON(w, http.StatusOK, updated)
}

func (h *Handler) ListRecommendations(w http.ResponseWriter, r *http.Request) {
	since := parseDurationOr(r.URL.Query().Get("since"), 24*time.Hour)
	limit := parseIntOr(r.URL.Query().Get("limit"), 12)
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	recommendationType := strings.TrimSpace(r.URL.Query().Get("type"))

	svc := recommendations.NewService(h.pg, h.evalService())
	page, err := svc.ListRecommendations(r.Context(), tenantFromCtx(r), since, limit, status, recommendationType)
	if err != nil {
		h.logger.Error("list recommendations", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "query error")
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *Handler) UpdateRecommendationStatus(w http.ResponseWriter, r *http.Request) {
	recommendationID, err := strconv.ParseInt(chi.URLParam(r, "recommendationId"), 10, 64)
	if err != nil || recommendationID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid recommendation id")
		return
	}
	var req models.RecommendationStatusUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	var before any
	if existing, err := h.pg.GetRecommendation(r.Context(), tenantFromCtx(r), recommendationID); err == nil {
		before = existing
	}
	svc := recommendations.NewService(h.pg, h.evalService())
	updated, err := svc.UpdateStatus(r.Context(), tenantFromCtx(r), recommendationID, req.Status)
	if err != nil {
		h.logger.Error("update recommendation status", zap.Error(err))
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.writeAdminAudit(r, "recommendations", "status_update", "recommendation", strconv.FormatInt(updated.ID, 10), "success", updated)
	h.writeControlHistory(r, "recommendations", "status_update", "recommendation", strconv.FormatInt(updated.ID, 10), req.Status, "success", before, updated, nil)
	writeJSON(w, http.StatusOK, updated)
}

func (h *Handler) GetTraceCost(w http.ResponseWriter, r *http.Request) {
	traceID := chi.URLParam(r, "traceId")
	spans, err := h.pg.GetTraceSpans(r.Context(), traceID, tenantFromCtx(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	spans = observability.EnrichSpans(spans, nil)
	type costBreakdown struct {
		SpanID            string  `json:"span_id"`
		Name              string  `json:"name"`
		Model             string  `json:"model"`
		InputTok          int64   `json:"input_tokens"`
		OutputTok         int64   `json:"output_tokens"`
		CacheReadTokens   int64   `json:"cache_read_tokens,omitempty"`
		CacheWriteTokens  int64   `json:"cache_write_tokens,omitempty"`
		ReasoningTokens   int64   `json:"reasoning_tokens,omitempty"`
		InputCostUSD      float64 `json:"input_cost_usd,omitempty"`
		OutputCostUSD     float64 `json:"output_cost_usd,omitempty"`
		CacheReadCostUSD  float64 `json:"cache_read_cost_usd,omitempty"`
		CacheWriteCostUSD float64 `json:"cache_write_cost_usd,omitempty"`
		ReasoningCostUSD  float64 `json:"reasoning_cost_usd,omitempty"`
		CostUSD           float64 `json:"cost_usd"`
		PricingRuleID     int64   `json:"pricing_rule_id,omitempty"`
		PricingScope      string  `json:"pricing_scope,omitempty"`
	}
	var total float64
	breakdown := make([]costBreakdown, 0, len(spans))
	for _, sp := range spans {
		if sp.CostUSD > 0 {
			breakdown = append(breakdown, costBreakdown{
				SpanID:            sp.ID,
				Name:              sp.Name,
				Model:             sp.Model,
				InputTok:          sp.InputTokens,
				OutputTok:         sp.OutputTokens,
				CacheReadTokens:   sp.CacheReadTokens,
				CacheWriteTokens:  sp.CacheWriteTokens,
				ReasoningTokens:   sp.ReasoningTokens,
				InputCostUSD:      sp.InputCostUSD,
				OutputCostUSD:     sp.OutputCostUSD,
				CacheReadCostUSD:  sp.CacheReadCostUSD,
				CacheWriteCostUSD: sp.CacheWriteCostUSD,
				ReasoningCostUSD:  sp.ReasoningCostUSD,
				CostUSD:           sp.CostUSD,
				PricingRuleID:     sp.PricingRuleID,
				PricingScope:      sp.PricingScope,
			})
			total += sp.CostUSD
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_usd": total,
		"breakdown": breakdown,
	})
}

// ─── Agents ──────────────────────────────────────────────────────────────────

func (h *Handler) ListAgents(w http.ResponseWriter, r *http.Request) {
	limit := parseIntOr(r.URL.Query().Get("limit"), 50)
	agents, err := h.pg.ListAgents(r.Context(), tenantFromCtx(r), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, models.Page[models.Agent]{Items: agents, Total: int64(len(agents))})
}

func (h *Handler) GetAgent(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")
	// O(1) indexed SQL lookup — replaces the previous O(n) ListAgents+scan pattern.
	agent, err := h.pg.GetAgentByName(r.Context(), tenantFromCtx(r), agentID)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "agent not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, agent)
}

func (h *Handler) GetAgentRuns(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")
	page, err := h.pg.ListRuns(r.Context(), models.RunQuery{
		TenantID:  tenantFromCtx(r),
		AgentName: agentID,
		Limit:     parseIntOr(r.URL.Query().Get("limit"), 50),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *Handler) GetAgentMetrics(w http.ResponseWriter, r *http.Request) {
	since := 24 * time.Hour
	if s := r.URL.Query().Get("since"); s != "" {
		if d, err := time.ParseDuration(s); err == nil {
			since = d
		}
	}
	stats, err := h.pg.GetOverview(r.Context(), tenantFromCtx(r), since)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (h *Handler) ListAgentScorecards(w http.ResponseWriter, r *http.Request) {
	since := 24 * time.Hour
	if s := strings.TrimSpace(r.URL.Query().Get("since")); s != "" {
		if d, err := time.ParseDuration(s); err == nil {
			since = d
		}
	}
	limit := parseIntOr(r.URL.Query().Get("limit"), 25)
	svc := h.evalService()
	items, err := svc.ListAgentScorecards(r.Context(), tenantFromCtx(r), since, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, models.Page[models.AgentScorecard]{
		Items: items,
		Total: int64(len(items)),
	})
}

func (h *Handler) GetAgentScorecard(w http.ResponseWriter, r *http.Request) {
	since := 24 * time.Hour
	if s := strings.TrimSpace(r.URL.Query().Get("since")); s != "" {
		if d, err := time.ParseDuration(s); err == nil {
			since = d
		}
	}
	svc := h.evalService()
	card, err := svc.GetAgentScorecard(r.Context(), tenantFromCtx(r), chi.URLParam(r, "agentId"), since)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "agent scorecard not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, card)
}

func (h *Handler) GetAgentTopology(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")
	tenantID := tenantFromCtx(r)

	// Fetch recent runs to collect unique trace IDs
	page, err := h.pg.ListRuns(r.Context(), models.RunQuery{
		TenantID:  tenantID,
		AgentName: agentID,
		Limit:     10,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Deduplicate trace IDs, then fetch all spans in a single batch query (P0-2 fix)
	seen := map[string]bool{}
	traceIDs := make([]string, 0, len(page.Items))
	for _, run := range page.Items {
		if !seen[run.TraceID] {
			seen[run.TraceID] = true
			traceIDs = append(traceIDs, run.TraceID)
		}
	}

	allSpans, err := h.pg.GetSpansForTraces(r.Context(), traceIDs, tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, buildTopologyGraph(allSpans))
}

// ─── Runs ─────────────────────────────────────────────────────────────────────

func (h *Handler) ListManagedAgents(w http.ResponseWriter, r *http.Request) {
	page, err := h.managedRuntimeService().ListAgents(r.Context(), tenantFromCtx(r), parseIntOr(r.URL.Query().Get("limit"), 50))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *Handler) GetManagedAgent(w http.ResponseWriter, r *http.Request) {
	item, err := h.managedRuntimeService().GetAgent(r.Context(), tenantFromCtx(r), chi.URLParam(r, "agentId"))
	if err != nil {
		if errors.Is(err, managedruntime.ErrNotFound) {
			writeError(w, http.StatusNotFound, "managed agent not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) UpsertManagedAgent(w http.ResponseWriter, r *http.Request) {
	var req models.ManagedAgentUpsertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	service := h.managedRuntimeService()
	var before any
	if id := strings.TrimSpace(req.ID); id != "" {
		if existing, err := service.GetAgent(r.Context(), tenantFromCtx(r), id); err == nil {
			before = existing
		}
	}
	item, err := service.UpsertAgent(r.Context(), tenantFromCtx(r), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	action := "create"
	status := http.StatusCreated
	if before != nil {
		action = "update"
		status = http.StatusOK
	}
	h.writeAdminAudit(r, "managed_agents", action, "managed_agent", item.ID, "success", item)
	h.writeControlHistory(r, "managed_agents", action, "managed_agent", item.ID, item.Name, "success", before, item, []string{item.ID})
	writeJSON(w, status, item)
}

func (h *Handler) ListManagedEnvironments(w http.ResponseWriter, r *http.Request) {
	page, err := h.managedRuntimeService().ListEnvironments(r.Context(), tenantFromCtx(r), parseIntOr(r.URL.Query().Get("limit"), 50))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *Handler) GetManagedEnvironment(w http.ResponseWriter, r *http.Request) {
	item, err := h.managedRuntimeService().GetEnvironment(r.Context(), tenantFromCtx(r), chi.URLParam(r, "environmentId"))
	if err != nil {
		if errors.Is(err, managedruntime.ErrNotFound) {
			writeError(w, http.StatusNotFound, "managed environment not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) UpsertManagedEnvironment(w http.ResponseWriter, r *http.Request) {
	var req models.ManagedEnvironmentUpsertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	service := h.managedRuntimeService()
	var before any
	if id := strings.TrimSpace(req.ID); id != "" {
		if existing, err := service.GetEnvironment(r.Context(), tenantFromCtx(r), id); err == nil {
			before = existing
		}
	}
	item, err := service.UpsertEnvironment(r.Context(), tenantFromCtx(r), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	action := "create"
	status := http.StatusCreated
	if before != nil {
		action = "update"
		status = http.StatusOK
	}
	h.writeAdminAudit(r, "managed_agents", action, "managed_environment", item.ID, "success", item)
	h.writeControlHistory(r, "managed_agents", action, "managed_environment", item.ID, item.Name, "success", before, item, []string{item.ID})
	writeJSON(w, status, item)
}

func (h *Handler) ListManagedSessions(w http.ResponseWriter, r *http.Request) {
	page, err := h.managedRuntimeService().ListSessions(
		r.Context(),
		tenantFromCtx(r),
		r.URL.Query().Get("agent_id"),
		parseIntOr(r.URL.Query().Get("limit"), 50),
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *Handler) GetManagedSession(w http.ResponseWriter, r *http.Request) {
	item, err := h.managedRuntimeService().GetSession(r.Context(), tenantFromCtx(r), chi.URLParam(r, "sessionId"))
	if err != nil {
		if errors.Is(err, managedruntime.ErrNotFound) {
			writeError(w, http.StatusNotFound, "managed session not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) CreateManagedSession(w http.ResponseWriter, r *http.Request) {
	var req models.ManagedSessionCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	item, err := h.managedRuntimeService().CreateSession(r.Context(), tenantFromCtx(r), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.writeAdminAudit(r, "managed_agents", "create", "managed_session", item.ID, "success", item)
	h.writeControlHistory(r, "managed_agents", "create", "managed_session", item.ID, item.AgentID, "success", nil, item, []string{item.AgentID, item.ID})
	writeJSON(w, http.StatusCreated, item)
}

func (h *Handler) ListManagedSessionEvents(w http.ResponseWriter, r *http.Request) {
	page, err := h.managedRuntimeService().ListSessionEvents(
		r.Context(),
		tenantFromCtx(r),
		chi.URLParam(r, "sessionId"),
		parseIntOr(r.URL.Query().Get("limit"), 200),
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *Handler) CreateManagedSessionEvent(w http.ResponseWriter, r *http.Request) {
	var req models.ManagedSessionEventCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	sessionID := chi.URLParam(r, "sessionId")
	item, err := h.managedRuntimeService().CreateSessionEvent(r.Context(), tenantFromCtx(r), sessionID, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.writeAdminAudit(r, "managed_agents", "append_event", "managed_session", sessionID, "success", item)
	h.writeControlHistory(r, "managed_agents", "append_event", "managed_session", sessionID, item.Type, "success", nil, item, []string{sessionID, item.ID})
	writeJSON(w, http.StatusCreated, item)
}

func (h *Handler) ListManagedSessionTasks(w http.ResponseWriter, r *http.Request) {
	page, err := h.managedRuntimeService().ListSessionTasks(
		r.Context(),
		tenantFromCtx(r),
		chi.URLParam(r, "sessionId"),
		parseIntOr(r.URL.Query().Get("limit"), 100),
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *Handler) GetManagedTask(w http.ResponseWriter, r *http.Request) {
	item, err := h.managedRuntimeService().GetTask(r.Context(), tenantFromCtx(r), chi.URLParam(r, "taskId"))
	if err != nil {
		if errors.Is(err, managedruntime.ErrNotFound) {
			writeError(w, http.StatusNotFound, "managed task not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) UpsertManagedTask(w http.ResponseWriter, r *http.Request) {
	var req models.ManagedTaskUpsertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	service := h.managedRuntimeService()
	var before any
	if id := strings.TrimSpace(req.ID); id != "" {
		if existing, err := service.GetTask(r.Context(), tenantFromCtx(r), id); err == nil {
			before = existing
		}
	}
	item, err := service.UpsertTask(r.Context(), tenantFromCtx(r), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	action := "create"
	status := http.StatusCreated
	if before != nil {
		action = "update"
		status = http.StatusOK
	}
	h.writeAdminAudit(r, "managed_agents", action, "managed_task", item.ID, "success", item)
	h.writeControlHistory(r, "managed_agents", action, "managed_task", item.ID, item.Status, "success", before, item, []string{item.SessionID, item.ID})
	writeJSON(w, status, item)
}

func (h *Handler) ListManagedTaskArtifacts(w http.ResponseWriter, r *http.Request) {
	page, err := h.managedRuntimeService().ListTaskArtifacts(
		r.Context(),
		tenantFromCtx(r),
		chi.URLParam(r, "taskId"),
		parseIntOr(r.URL.Query().Get("limit"), 100),
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *Handler) CreateManagedTaskArtifact(w http.ResponseWriter, r *http.Request) {
	var req models.ManagedArtifactCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	taskID := chi.URLParam(r, "taskId")
	item, err := h.managedRuntimeService().CreateArtifact(r.Context(), tenantFromCtx(r), taskID, req)
	if err != nil {
		if errors.Is(err, managedruntime.ErrNotFound) {
			writeError(w, http.StatusNotFound, "managed task not found")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.writeAdminAudit(r, "managed_agents", "create", "managed_artifact", item.ID, "success", item)
	h.writeControlHistory(r, "managed_agents", "create", "managed_artifact", item.ID, item.Name, "success", nil, item, []string{taskID, item.ID})
	writeJSON(w, http.StatusCreated, item)
}

func (h *Handler) ApproveManagedTask(w http.ResponseWriter, r *http.Request) {
	h.decideManagedTask(w, r, true)
}

func (h *Handler) DenyManagedTask(w http.ResponseWriter, r *http.Request) {
	h.decideManagedTask(w, r, false)
}

func (h *Handler) decideManagedTask(w http.ResponseWriter, r *http.Request, approve bool) {
	var req models.ManagedTaskDecisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	taskID := chi.URLParam(r, "taskId")
	service := h.managedRuntimeService()
	var (
		item models.ManagedTask
		err  error
	)
	if approve {
		item, err = service.ApproveTask(r.Context(), tenantFromCtx(r), taskID, actorFromCtx(r), req)
	} else {
		item, err = service.DenyTask(r.Context(), tenantFromCtx(r), taskID, actorFromCtx(r), req)
	}
	if err != nil {
		if errors.Is(err, managedruntime.ErrNotFound) {
			writeError(w, http.StatusNotFound, "managed task not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	action := "approve"
	if !approve {
		action = "deny"
	}
	h.writeAdminAudit(r, "managed_agents", action, "managed_task", taskID, "success", map[string]any{
		"task_id": taskID,
		"reason":  req.Reason,
	})
	h.writeControlHistory(r, "managed_agents", action, "managed_task", taskID, req.Reason, "success", nil, item, nil)
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) ListRuns(w http.ResponseWriter, r *http.Request) {
	page, err := h.pg.ListRuns(r.Context(), models.RunQuery{
		TenantID:  tenantFromCtx(r),
		TraceID:   r.URL.Query().Get("trace_id"),
		Framework: r.URL.Query().Get("framework"),
		AgentName: r.URL.Query().Get("agent"),
		Limit:     parseIntOr(r.URL.Query().Get("limit"), 50),
		Cursor:    r.URL.Query().Get("cursor"),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *Handler) GetRun(w http.ResponseWriter, r *http.Request) {
	run, err := h.pg.GetRun(r.Context(), chi.URLParam(r, "runId"), tenantFromCtx(r))
	if err != nil {
		writeError(w, http.StatusNotFound, "run not found")
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (h *Handler) GetRunChildren(w http.ResponseWriter, r *http.Request) {
	children, err := h.pg.GetRunChildren(r.Context(), chi.URLParam(r, "runId"), tenantFromCtx(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, children)
}

func (h *Handler) PostFeedback(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runId")
	var body struct {
		Score   *int16 `json:"score"`
		Comment string `json:"comment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := h.pg.InsertFeedback(r.Context(), runID, tenantFromCtx(r), body.Score, body.Comment); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// ─── Analytics ───────────────────────────────────────────────────────────────

func (h *Handler) GetOverview(w http.ResponseWriter, r *http.Request) {
	since := 24 * time.Hour
	if s := r.URL.Query().Get("since"); s != "" {
		if d, err := time.ParseDuration(s); err == nil {
			since = d
		}
	}
	stats, err := h.pg.GetOverview(r.Context(), tenantFromCtx(r), since)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (h *Handler) GetFrameworkStats(w http.ResponseWriter, r *http.Request) {
	since := 24 * time.Hour
	if s := r.URL.Query().Get("since"); s != "" {
		if d, err := time.ParseDuration(s); err == nil {
			since = d
		}
	}
	stats, err := h.pg.GetOverview(r.Context(), tenantFromCtx(r), since)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, stats.FrameworkCounts)
}

func (h *Handler) GetCostReport(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pg.GetCostReport(r.Context(), tenantFromCtx(r), parseCostReportQuery(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *Handler) GetCostSpikes(w http.ResponseWriter, r *http.Request) {
	report, err := h.pg.GetCostSpikeReport(r.Context(), tenantFromCtx(r), parseCostReportQuery(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h *Handler) GetErrorReport(w http.ResponseWriter, r *http.Request) {
	since := 24 * time.Hour
	if s := r.URL.Query().Get("since"); s != "" {
		if d, err := time.ParseDuration(s); err == nil {
			since = d
		}
	}
	rows, err := h.pg.GetErrorReport(r.Context(), tenantFromCtx(r), since)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *Handler) ListEnvironments(w http.ResponseWriter, r *http.Request) {
	envs, err := h.pg.ListEnvironments(r.Context(), tenantFromCtx(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, envs)
}

func (h *Handler) ListDecisions(w http.ResponseWriter, r *http.Request) {
	page, err := h.pg.ListDecisionRecords(r.Context(), models.DecisionQuery{
		TenantID: tenantFromCtx(r),
		TraceID:  strings.TrimSpace(r.URL.Query().Get("trace_id")),
		Type:     strings.TrimSpace(r.URL.Query().Get("type")),
		Result:   strings.TrimSpace(r.URL.Query().Get("result")),
		Limit:    parseIntOr(r.URL.Query().Get("limit"), 100),
		Offset:   parseIntOr(r.URL.Query().Get("offset"), 0),
	})
	if err != nil {
		h.logger.Error("list decisions", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to query decision records")
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *Handler) ListTraceDecisions(w http.ResponseWriter, r *http.Request) {
	traceID := chi.URLParam(r, "traceId")
	records, err := h.pg.ListDecisionRecordsForTrace(r.Context(), tenantFromCtx(r), traceID)
	if err != nil {
		h.logger.Error("list trace decisions", zap.Error(err), zap.String("trace_id", traceID))
		writeError(w, http.StatusInternalServerError, "failed to query trace decisions")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": records,
		"count": len(records),
	})
}

// ─── Live stream ──────────────────────────────────────────────────────────────

func (h *Handler) LiveStream(w http.ResponseWriter, r *http.Request) {
	h.hub.ServeWS(w, r, tenantFromCtx(r))
}

// ─── Audit log ────────────────────────────────────────────────────────────────

// ListAudit returns a paginated list of policy decisions for the tenant.
// GET /api/v1/audit?limit=100&offset=0
func (h *Handler) ListAudit(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantFromCtx(r)
	limit := parseIntOr(r.URL.Query().Get("limit"), 100)
	offset := parseIntOr(r.URL.Query().Get("offset"), 0)

	entries, err := h.pg.ListAuditEntries(r.Context(), tenantID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query audit log")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items":  entries,
		"limit":  limit,
		"offset": offset,
		"count":  len(entries),
	})
}

// VerifyAuditChain replays the SHA-256 chain and reports any broken links.
// GET /api/v1/audit/verify
func (h *Handler) VerifyAuditChain(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantFromCtx(r)

	result, err := h.pg.VerifyAuditChain(r.Context(), tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "chain verification failed")
		return
	}

	status := http.StatusOK
	if !result.Valid {
		status = http.StatusConflict // 409: data conflict — chain broken
	}
	writeJSON(w, status, result)
}

func (h *Handler) ListControlAudit(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantFromCtx(r)
	limit := parseIntOr(r.URL.Query().Get("limit"), 100)

	entries, err := h.pg.ListAdminAuditEntries(r.Context(), tenantID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query control-plane audit")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": entries,
		"count": len(entries),
		"limit": limit,
	})
}

func (h *Handler) ListControlHistory(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantFromCtx(r)
	limit := parseIntOr(r.URL.Query().Get("limit"), 100)
	offset := parseIntOr(r.URL.Query().Get("offset"), 0)
	category := strings.TrimSpace(r.URL.Query().Get("category"))
	targetID := strings.TrimSpace(r.URL.Query().Get("target_id"))

	page, err := memory.NewService(h.pg).ListControlHistory(r.Context(), tenantID, category, targetID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query control history")
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *Handler) ListEvidenceBundles(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantFromCtx(r)
	limit := parseIntOr(r.URL.Query().Get("limit"), 25)

	items, err := memory.NewService(h.pg).ListEvidenceBundles(r.Context(), tenantID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query evidence bundles")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"count": len(items),
		"limit": limit,
	})
}

func (h *Handler) CreateEvidenceBundle(w http.ResponseWriter, r *http.Request) {
	var req models.EvidenceBundleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	bundle, err := memory.NewService(h.pg).CreateEvidenceBundle(r.Context(), tenantFromCtx(r), actorFromCtx(r), req)
	if err != nil {
		h.logger.Error("create evidence bundle", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to create evidence bundle")
		return
	}
	h.writeAdminAudit(r, "enterprise_memory", "bundle_create", "evidence_bundle", strconv.FormatInt(bundle.ID, 10), "success", bundle)
	h.writeControlHistory(r, "enterprise_memory", "bundle_create", "evidence_bundle", strconv.FormatInt(bundle.ID, 10), strings.TrimSpace(req.Reason), "success", nil, bundle, nil)
	writeJSON(w, http.StatusCreated, bundle)
}

func (h *Handler) GetEvidenceBundle(w http.ResponseWriter, r *http.Request) {
	bundleID, err := strconv.ParseInt(chi.URLParam(r, "bundleId"), 10, 64)
	if err != nil || bundleID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid bundle id")
		return
	}

	bundle, err := memory.NewService(h.pg).GetEvidenceBundle(r.Context(), tenantFromCtx(r), bundleID)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "evidence bundle not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load evidence bundle")
		return
	}
	writeJSON(w, http.StatusOK, bundle)
}

func (h *Handler) ExportEvidenceBundle(w http.ResponseWriter, r *http.Request) {
	bundleID, err := strconv.ParseInt(chi.URLParam(r, "bundleId"), 10, 64)
	if err != nil || bundleID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid bundle id")
		return
	}

	bundle, err := memory.NewService(h.pg).GetEvidenceBundle(r.Context(), tenantFromCtx(r), bundleID)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "evidence bundle not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to export evidence bundle")
		return
	}
	filename := fmt.Sprintf("evidence-bundle-%d.json", bundle.ID)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	writeJSON(w, http.StatusOK, bundle)
}

// ─── Users ────────────────────────────────────────────────────────────────────

// ListUsers returns all users for the current tenant.
// GET /api/v1/users?limit=50&offset=0
func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	limit := parseIntOr(r.URL.Query().Get("limit"), 50)
	offset := parseIntOr(r.URL.Query().Get("offset"), 0)

	users, err := h.pg.ListUsers(r.Context(), tenantFromCtx(r), limit, offset)
	if err != nil {
		h.logger.Error("list users", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "query error")
		return
	}
	writeJSON(w, http.StatusOK, models.Page[models.User]{
		Items: users,
		Total: int64(len(users)),
	})
}

// GetUser returns a single user by ID within the current tenant.
// GET /api/v1/users/{userId}
func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
	user, err := h.pg.GetUser(r.Context(), chi.URLParam(r, "userId"), tenantFromCtx(r))
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

// CreateUser creates a new user in the current tenant.
// POST /api/v1/users — admin role required in production; enforced by policy layer.
func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req models.CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Username == "" || req.Email == "" {
		writeError(w, http.StatusBadRequest, "username and email are required")
		return
	}

	user, err := h.pg.CreateUser(r.Context(), tenantFromCtx(r), req)
	if err != nil {
		h.logger.Error("create user", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "create failed")
		return
	}
	h.writeAdminAudit(r, "auth", "create", "user", user.ID, "success", user)
	h.writeControlHistory(r, "auth", "create", "user", user.ID, "user created", "success", nil, user, nil)
	writeJSON(w, http.StatusCreated, user)
}

// UpdateUser applies partial updates to a user record.
// PUT /api/v1/users/{userId}
func (h *Handler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	var req models.UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	userID := chi.URLParam(r, "userId")
	before, _ := h.pg.GetUser(r.Context(), userID, tenantFromCtx(r))
	user, err := h.pg.UpdateUser(r.Context(), userID, tenantFromCtx(r), req)
	if err != nil {
		h.logger.Error("update user", zap.Error(err))
		writeError(w, http.StatusNotFound, "user not found or no change")
		return
	}
	h.writeAdminAudit(r, "auth", "update", "user", user.ID, "success", map[string]any{"before": before, "after": user})
	h.writeControlHistory(r, "auth", "update", "user", user.ID, "user updated", "success", before, user, nil)
	writeJSON(w, http.StatusOK, user)
}

// DeleteUser removes a user from the current tenant.
// DELETE /api/v1/users/{userId}
func (h *Handler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userId")
	before, _ := h.pg.GetUser(r.Context(), userID, tenantFromCtx(r))
	if err := h.pg.DeleteUser(r.Context(), userID, tenantFromCtx(r)); err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	h.writeAdminAudit(r, "auth", "delete", "user", userID, "success", before)
	h.writeControlHistory(r, "auth", "delete", "user", userID, "user deleted", "success", before, nil, nil)
	w.WriteHeader(http.StatusNoContent)
}

// ─── Build helpers ────────────────────────────────────────────────────────────

func buildTrace(traceID string, spans []models.Span) models.Trace {
	t := models.Trace{ID: traceID, Spans: spans}
	if len(spans) == 0 {
		return t
	}
	decorateSpans(spans)
	t.Framework = spans[0].Framework
	t.RootSpanName = spans[0].Name
	t.StartTime = time.Unix(0, spans[0].StartTimeNs)
	t.Insights = models.TraceInsights{
		StepTypes:    map[string]int{},
		ErrorClasses: map[string]int{},
	}
	modelsSeen := map[string]struct{}{}
	providersSeen := map[string]struct{}{}
	appsSeen := map[string]struct{}{}
	envsSeen := map[string]struct{}{}
	var maxEnd int64
	for _, sp := range spans {
		end := sp.StartTimeNs + sp.DurationNs
		if end > maxEnd {
			maxEnd = end
		}
		t.TotalCostUSD += sp.CostUSD
		t.TotalTokens += sp.InputTokens + sp.OutputTokens + sp.CacheReadTokens + sp.CacheWriteTokens + sp.ReasoningTokens
		if sp.Model != "" {
			modelsSeen[sp.Model] = struct{}{}
		}
		if sp.Provider != "" {
			providersSeen[sp.Provider] = struct{}{}
		}
		if sp.AppName != "" {
			appsSeen[sp.AppName] = struct{}{}
		}
		if sp.Environment != "" {
			envsSeen[sp.Environment] = struct{}{}
		}
		if sp.StepType != "" {
			t.Insights.StepTypes[sp.StepType]++
		}
		if sp.ErrorClass != "" {
			t.Insights.ErrorClasses[sp.ErrorClass]++
		}
		if sp.StepType == "llm" {
			t.Insights.LLMCalls++
		}
		if sp.StepType == "tool" {
			t.Insights.ToolCalls++
		}
		if sp.Blocked {
			t.Insights.BlockedSpans++
		}
		if models.OutcomeStatusCountsAsFailure(sp.OutcomeStatus) {
			t.Insights.FailedSpans++
		}
		t.Insights.RetryCount += sp.RetryCount
		if sp.Depth > t.Insights.MaxDepth {
			t.Insights.MaxDepth = sp.Depth
		}
		if sp.OutcomeStatus == models.OutcomeStatusError {
			t.ErrorCount++
		}
	}
	t.Insights.Models = appendSortedSet(modelsSeen)
	t.Insights.Providers = appendSortedSet(providersSeen)
	t.Insights.Apps = appendSortedSet(appsSeen)
	t.Insights.Environments = appendSortedSet(envsSeen)
	t.Duration = maxEnd - spans[0].StartTimeNs
	t.SpanCount = len(spans)
	if t.ErrorCount > 0 {
		t.Status = "error"
	} else if t.Insights.BlockedSpans > 0 || t.Insights.FailedSpans > 0 {
		t.Status = "partial"
	} else {
		t.Status = "ok"
	}
	return t
}

func (h *Handler) attachTracePolicyEvents(ctx context.Context, tenantID string, trace *models.Trace) error {
	if trace == nil || trace.ID == "" {
		return nil
	}
	policyEvents, err := h.tracePolicyEvents(ctx, tenantID, trace.ID, trace.Spans)
	if err != nil {
		return err
	}
	trace.PolicyEvents = policyEvents
	return nil
}

func (h *Handler) tracePolicyEvents(ctx context.Context, tenantID, traceID string, spans []models.Span) ([]models.PolicyEvent, error) {
	entries, err := h.pg.ListAuditEntriesForTrace(ctx, tenantID, traceID)
	if err != nil {
		return nil, err
	}
	return auditEntriesToPolicyEvents(entries, observability.EnrichSpans(spans, nil)), nil
}

func auditEntriesToPolicyEvents(entries []store.AuditEntry, spans []models.Span) []models.PolicyEvent {
	if len(entries) == 0 {
		return nil
	}
	spanByID := make(map[string]models.Span, len(spans))
	for _, span := range spans {
		spanByID[span.ID] = span
	}

	events := make([]models.PolicyEvent, 0, len(entries))
	for _, entry := range entries {
		event := models.PolicyEvent{
			DecisionID: entry.DecisionID,
			TraceID:    entry.TraceID,
			SpanID:     entry.SpanID,
			PolicyName: entry.PolicyName,
			Result:     entry.Result,
			Reason:     entry.Reason,
			TenantID:   entry.TenantID,
		}
		if span, ok := spanByID[entry.SpanID]; ok {
			event.Provider = span.Provider
			event.Model = span.Model
			event.Scope = firstNonEmpty(span.Attributes["af.policy.scope"], span.Attributes["policy.scope"])
			if matched := firstNonEmpty(span.Attributes["af.policy.matched"], span.Attributes["policy.matched"]); matched != "" {
				event.Matched = splitCSV(matched)
			}
			if redactions := firstInt(span.Attributes, "af.policy.redactions", "policy.redactions"); redactions > 0 {
				event.Redactions = redactions
			}
		}
		events = append(events, event)
	}
	return events
}

func buildTopologyGraph(spans []models.Span) models.TopologyGraph {
	nodes := map[string]models.TopologyNode{}
	edges := map[string]*models.TopologyEdge{}

	for _, sp := range spans {
		if _, ok := nodes[sp.ID]; !ok {
			nodes[sp.ID] = models.TopologyNode{
				ID:        sp.ID,
				Name:      sp.Name,
				Framework: sp.Framework,
				SpanCount: 1,
			}
		}
		if sp.ParentID != "" {
			key := sp.ParentID + "->" + sp.ID
			if e, ok := edges[key]; ok {
				e.CallCount++
			} else {
				edges[key] = &models.TopologyEdge{
					From:      sp.ParentID,
					To:        sp.ID,
					EdgeType:  "call",
					CallCount: 1,
				}
			}
		}
	}

	g := models.TopologyGraph{}
	for _, n := range nodes {
		g.Nodes = append(g.Nodes, n)
	}
	for _, e := range edges {
		g.Edges = append(g.Edges, *e)
	}
	return g
}

func decorateSpans(spans []models.Span) {
	byID := make(map[string]models.Span, len(spans))
	for _, sp := range spans {
		byID[sp.ID] = sp
	}
	depthMemo := map[string]int{}
	for i := range spans {
		decorateSpan(&spans[i], byID, depthMemo)
	}
}

func decorateSpan(sp *models.Span, byID map[string]models.Span, depthMemo map[string]int) {
	if sp == nil {
		return
	}
	if sp.Attributes == nil {
		sp.Attributes = map[string]string{}
	}
	sp.Depth = spanDepth(sp.ID, byID, depthMemo)
	sp.Provider = firstNonEmpty(sp.Attributes["gen_ai.system"], sp.Attributes["proxy.provider"], sp.Attributes["netproxy.provider"])
	sp.Model = firstNonEmpty(sp.Attributes["gen_ai.request.model"], sp.Attributes["proxy.model"], sp.Attributes["netproxy.model"])
	sp.StepType = firstNonEmpty(sp.Attributes["af.span.step_type"], inferStepType(sp.Name, sp.Provider, sp.Model))
	sp.AppName = firstNonEmpty(sp.Attributes["service.name"], sp.Attributes["af.app.name"], sp.Attributes["application.name"])
	sp.Environment = firstNonEmpty(sp.Attributes["deployment.environment"], sp.Attributes["environment"], sp.Attributes["env"])
	sp.UserID = firstNonEmpty(sp.Attributes["enduser.id"], sp.Attributes["user.id"], sp.Attributes["af.user.id"])
	sp.SessionID = firstNonEmpty(sp.Attributes["session.id"], sp.Attributes["af.session.id"])
	sp.PromptID = firstNonEmpty(sp.Attributes["af.prompt.id"])
	sp.PromptVersion = firstInt(sp.Attributes, "af.prompt.version")
	sp.PromptReleaseTag = firstNonEmpty(sp.Attributes["af.prompt.release_tag"])
	sp.PromptEnvironment = firstNonEmpty(sp.Attributes["af.prompt.environment"], sp.Environment)
	sp.ErrorClass = firstNonEmpty(sp.Attributes["af.error.class"], sp.Attributes["error.type"], sp.Attributes["exception.type"])
	sp.PromptPreview = firstPreview(sp.Attributes, "af.preview.prompt", "gen_ai.prompt", "input.value", "prompt", "llm.prompt")
	sp.ResponsePreview = firstPreview(sp.Attributes, "af.preview.response", "gen_ai.response", "output.value", "response", "llm.response")
	sp.RetryCount = firstInt(sp.Attributes, "retry.count", "http.retry_count", "af.retry.count")
	sp.OutcomeStatus = models.NormalizeOutcomeStatus(sp.StatusCode, sp.Attributes)
	sp.BlockedReason = firstNonEmpty(sp.Attributes["af.policy.reason"], sp.Attributes["policy.reason"], sp.Attributes["budget.reason"])
	sp.Blocked = sp.OutcomeStatus == models.OutcomeStatusBlocked || isTrue(sp.Attributes["af.policy.blocked"]) || strings.EqualFold(sp.Attributes["af.policy.decision"], "deny") || sp.BlockedReason != ""
	sp.PricingRuleID = firstInt64(sp.Attributes, "af.pricing.rule_id")
	sp.PricingScope = sp.Attributes["af.pricing.scope"]
}

func spanDepth(spanID string, byID map[string]models.Span, memo map[string]int) int {
	if spanID == "" {
		return 0
	}
	if depth, ok := memo[spanID]; ok {
		return depth
	}
	sp, ok := byID[spanID]
	if !ok || sp.ParentID == "" {
		memo[spanID] = 0
		return 0
	}
	depth := 1 + spanDepth(sp.ParentID, byID, memo)
	memo[spanID] = depth
	return depth
}

func inferStepType(name, provider, model string) string {
	lowerName := strings.ToLower(name)
	switch {
	case provider != "" || model != "":
		return "llm"
	case strings.Contains(lowerName, "tool"), strings.Contains(lowerName, "function"):
		return "tool"
	case strings.Contains(lowerName, "policy"), strings.Contains(lowerName, "guard"):
		return "policy"
	case strings.Contains(lowerName, "retry"):
		return "retry"
	default:
		return "agent"
	}
}

func firstPreview(attrs map[string]string, keys ...string) string {
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func firstInt(attrs map[string]string, keys ...string) int {
	for _, key := range keys {
		if v, err := strconv.Atoi(strings.TrimSpace(attrs[key])); err == nil {
			return v
		}
	}
	return 0
}

func firstInt64(attrs map[string]string, keys ...string) int64 {
	for _, key := range keys {
		if v, err := strconv.ParseInt(strings.TrimSpace(attrs[key]), 10, 64); err == nil {
			return v
		}
	}
	return 0
}

func isTrue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "blocked":
		return true
	default:
		return false
	}
}

func appendSortedSet(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func previewPolicyDecision(decision policy.Decision) models.PolicyPreviewDecision {
	preview := models.PolicyPreviewDecision{
		Matched:          decision.Matched,
		RuleID:           decision.RuleID,
		PolicyName:       decision.PolicyName,
		Action:           decision.Action,
		Reason:           decision.Reason,
		Scope:            decision.Scope,
		MatchedNames:     append([]string(nil), decision.MatchedNames...),
		GuardrailMatches: append([]string(nil), decision.GuardrailMatches...),
		Redactions:       decision.Redactions,
		Final:            decision.Final,
		Engine:           decision.Explanation.Engine,
		DecisionMode:     decision.Explanation.DecisionMode,
		Version:          decision.Explanation.Version,
		RolloutPercent:   decision.Explanation.RolloutPercent,
		EvaluationPath:   append([]string(nil), decision.Explanation.EvaluationPath...),
		MatchedFields:    append([]string(nil), decision.Explanation.MatchedFields...),
		ConditionTrace:   append([]models.PolicyConditionTrace(nil), decision.Explanation.ConditionTrace...),
		RegoQuery:        decision.Explanation.RegoQuery,
		Explain:          decision.Explanation.Explain,
		RuleConditions:   cloneStringMap(decision.Explanation.RuleConditions),
	}
	if len(decision.RedactedBody) > 0 {
		preview.RedactedPreview = previewString(decision.RedactedBody, 220)
	}
	return preview
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func previewString(body []byte, limit int) string {
	if len(body) == 0 {
		return ""
	}
	text := strings.TrimSpace(string(body))
	if text == "" {
		return ""
	}
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "..."
}

func parseIntOr(s string, def int) int {
	if v, err := strconv.Atoi(s); err == nil && v > 0 {
		return v
	}
	return def
}

func parseDurationOr(s string, def time.Duration) time.Duration {
	if parsed, err := time.ParseDuration(strings.TrimSpace(s)); err == nil && parsed > 0 {
		return parsed
	}
	return def
}

func parseCostReportQuery(r *http.Request) store.CostReportQuery {
	query := r.URL.Query()
	return store.CostReportQuery{
		Since:       parseDurationOr(query.Get("since"), 24*time.Hour),
		AppName:     strings.TrimSpace(query.Get("app_name")),
		Environment: strings.TrimSpace(query.Get("environment")),
		Provider:    strings.TrimSpace(query.Get("provider")),
		Model:       strings.TrimSpace(query.Get("model")),
		PromptID:    strings.TrimSpace(query.Get("prompt_id")),
		ReleaseTag:  strings.TrimSpace(query.Get("release_tag")),
		Limit:       parseIntOr(query.Get("limit"), 100),
	}
}

func (h *Handler) repriceSpan(sp *models.Span) {
	if sp == nil {
		return
	}
	model := strings.TrimSpace(sp.Attributes["gen_ai.request.model"])
	if model == "" {
		model = strings.TrimSpace(sp.Attributes["proxy.model"])
	}
	if model == "" {
		model = strings.TrimSpace(sp.Attributes["netproxy.model"])
	}

	provider := strings.TrimSpace(sp.Attributes["gen_ai.system"])

	var match proxy.PricingMatch

	// FIX P0: Only reprice if the collector has not already computed the costs
	if sp.CostUSD == 0 && sp.InputCostUSD == 0 && sp.OutputCostUSD == 0 && (sp.InputTokens > 0 || sp.OutputTokens > 0) {
		var detailed pricing.Result
		match, detailed = proxy.ComputeDetailedCostForTenant(provider, model, sp.TenantID, sp.ReceivedAt, pricing.Usage{
			InputTokens:      sp.InputTokens,
			OutputTokens:     sp.OutputTokens,
			CacheReadTokens:  sp.CacheReadTokens,
			CacheWriteTokens: sp.CacheWriteTokens,
			ReasoningTokens:  sp.ReasoningTokens,
		})
		sp.CostUSD = detailed.TotalCostUSD
		sp.InputCostUSD = detailed.InputCostUSD
		sp.OutputCostUSD = detailed.OutputCostUSD
		sp.CacheReadCostUSD = detailed.CacheReadCostUSD
		sp.CacheWriteCostUSD = detailed.CacheWriteCostUSD
		sp.ReasoningCostUSD = detailed.ReasoningCostUSD
	}

	if sp.Attributes == nil {
		sp.Attributes = map[string]string{}
	}
	if model != "" {
		sp.Attributes["gen_ai.request.model"] = model
	}
	if provider != "" {
		sp.Attributes["gen_ai.system"] = strings.ToLower(provider)
	}
	sp.Attributes["af.cost.input_usd"] = strconv.FormatFloat(sp.InputCostUSD, 'f', 8, 64)
	sp.Attributes["af.cost.output_usd"] = strconv.FormatFloat(sp.OutputCostUSD, 'f', 8, 64)
	sp.Attributes["af.cost.cache_read_usd"] = strconv.FormatFloat(sp.CacheReadCostUSD, 'f', 8, 64)
	sp.Attributes["af.cost.cache_write_usd"] = strconv.FormatFloat(sp.CacheWriteCostUSD, 'f', 8, 64)
	sp.Attributes["af.cost.reasoning_usd"] = strconv.FormatFloat(sp.ReasoningCostUSD, 'f', 8, 64)
	sp.Attributes["af.cost.total_usd"] = strconv.FormatFloat(sp.CostUSD, 'f', 8, 64)
	if sp.CacheReadTokens > 0 {
		sp.Attributes["gen_ai.usage.cache_read_tokens"] = strconv.FormatInt(sp.CacheReadTokens, 10)
	}
	if sp.CacheWriteTokens > 0 {
		sp.Attributes["gen_ai.usage.cache_write_tokens"] = strconv.FormatInt(sp.CacheWriteTokens, 10)
	}
	if sp.ReasoningTokens > 0 {
		sp.Attributes["gen_ai.usage.reasoning_tokens"] = strconv.FormatInt(sp.ReasoningTokens, 10)
	}
	if match.RuleID > 0 {
		sp.Attributes["af.pricing.rule_id"] = strconv.FormatInt(match.RuleID, 10)
		sp.Attributes["af.pricing.model_pattern"] = match.ModelPattern
		sp.Attributes["af.pricing.scope"] = match.Scope
		sp.Attributes["af.pricing.input_per_million"] = strconv.FormatFloat(match.InputPerMillion, 'f', 4, 64)
		sp.Attributes["af.pricing.output_per_million"] = strconv.FormatFloat(match.OutputPerMillion, 'f', 4, 64)
		sp.Attributes["af.pricing.cache_read_per_million"] = strconv.FormatFloat(match.CacheReadPerMillion, 'f', 4, 64)
		sp.Attributes["af.pricing.cache_write_per_million"] = strconv.FormatFloat(match.CacheWritePerMillion, 'f', 4, 64)
		sp.Attributes["af.pricing.reasoning_per_million"] = strconv.FormatFloat(match.ReasoningPerMillion, 'f', 4, 64)
	}
}

// scoreSpanRisk evaluates governance risk for a span and populates risk fields.
// Converts span to CanonicalEvent format for risk engine evaluation.
func (h *Handler) scoreSpanRisk(sp *models.Span) {
	if h.riskEngine == nil || sp == nil {
		return
	}

	// Convert span to CanonicalEvent for risk evaluation
	ev := &normalization.CanonicalEvent{
		ID:            sp.ID,
		EventTime:     sp.ReceivedAt,
		SourceProduct: sp.Framework,
		InputTokens:   sp.InputTokens,
		OutputTokens:  sp.OutputTokens,
		Severity:      "info",
	}

	// Extract relevant fields from attributes
	if sp.Attributes != nil {
		ev.Command = sp.Attributes["tool.command"]
		ev.FilePath = sp.Attributes["tool.file_path"]
		ev.ToolName = sp.Attributes["tool.name"]
		ev.RepoName = sp.Attributes["git.repository"]
		ev.EventType = sp.Attributes["event.type"]
		ev.Action = sp.Attributes["event.action"]
		if status := sp.Attributes["event.category"]; status != "" {
			ev.EventCategory = status
		}
	}

	// Evaluate risk using the risk engine
	score, category := h.riskEngine.Score(ev)
	sp.RiskScore = score
	sp.RiskCategory = category
}

func (h *Handler) evaluateFirewallDecisions(ctx context.Context, tenantID string, spans []models.Span) ([]ingestDecision, int) {
	decisions := make([]ingestDecision, 0, len(spans))
	blocked := 0
	for i := range spans {
		decision := h.firewallDecisionForSpan(tenantID, &spans[i])
		if decision.Result == "deny" {
			blocked++
		}
		decisions = append(decisions, decision)

		if spans[i].Attributes == nil {
			spans[i].Attributes = map[string]string{}
		}
		spans[i].Attributes["af.firewall.decision_id"] = decision.DecisionID
		spans[i].Attributes["af.firewall.result"] = decision.Result
		spans[i].Attributes["af.firewall.action_taken"] = decision.ActionTaken

		if h.pg != nil {
			if err := h.pg.CreatePolicyAuditEntry(ctx, models.PolicyDecisionAudit{
				DecisionID:  decision.DecisionID,
				TenantID:    tenantID,
				TraceID:     spans[i].TraceID,
				SpanID:      spans[i].ID,
				PolicyName:  "ingest_firewall",
				Result:      decision.Result,
				Reason:      decision.Reason,
				EvaluatedAt: time.Now(),
				Framework:   spans[i].Framework,
				Model:       spans[i].Model,
				Environment: spans[i].Environment,
			}); err != nil {
				h.logger.Warn("firewall audit write failed", zap.Error(err), zap.String("span_id", spans[i].ID))
			}
		}

		if h.hub != nil {
			h.hub.Broadcast(tenantID, &models.LiveEvent{
				Type:      "policy",
				Timestamp: time.Now().UnixMilli(),
				TenantID:  tenantID,
				Data: models.PolicyEvent{
					DecisionID: decision.DecisionID,
					TraceID:    spans[i].TraceID,
					SpanID:     spans[i].ID,
					PolicyName: "ingest_firewall",
					Result:     decision.Result,
					Reason:     decision.Reason,
					TenantID:   tenantID,
					Provider:   spans[i].Provider,
					Model:      spans[i].Model,
					Matched:    []string{decision.RiskCategory},
				},
			})
		}
	}
	return decisions, blocked
}

func (h *Handler) firewallDecisionForSpan(tenantID string, sp *models.Span) ingestDecision {
	if sp == nil {
		return ingestDecision{}
	}
	result := "approved"
	actionTaken := "allow_ingest"
	reason := "risk score below firewall threshold"
	switch {
	case sp.RiskScore >= firewallBlockThreshold:
		result = "deny"
		actionTaken = "reject_ingest_span"
		reason = fmt.Sprintf("risk category %s scored %d and met block threshold %d", sp.RiskCategory, sp.RiskScore, firewallBlockThreshold)
	case sp.RiskScore >= firewallReviewThreshold:
		result = "review"
		actionTaken = "require_governance_review"
		reason = fmt.Sprintf("risk category %s scored %d and met review threshold %d", sp.RiskCategory, sp.RiskScore, firewallReviewThreshold)
	}
	return ingestDecision{
		DecisionID:   fmt.Sprintf("fw-%s-%d", strings.TrimSpace(sp.ID), time.Now().UnixNano()),
		TraceID:      sp.TraceID,
		SpanID:       sp.ID,
		Result:       result,
		Reason:       reason,
		RiskScore:    sp.RiskScore,
		RiskCategory: sp.RiskCategory,
		ActionTaken:  actionTaken,
	}
}

func (h *Handler) ListPricingRules(w http.ResponseWriter, r *http.Request) {
	rules, err := h.pg.ListPricingRules(r.Context())
	if err != nil {
		h.logger.Error("list pricing rules", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "query error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rules, "count": len(rules)})
}

func (h *Handler) UpsertPricingRule(w http.ResponseWriter, r *http.Request) {
	var rule models.PricingRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	rule.Provider = proxy.NormalizeProvider(rule.Provider)
	rule.ModelPattern = strings.ToLower(strings.TrimSpace(rule.ModelPattern))
	if rule.ModelPattern == "" {
		writeError(w, http.StatusBadRequest, "model_pattern is required")
		return
	}
	if rule.InputPerMillion < 0 || rule.OutputPerMillion < 0 {
		writeError(w, http.StatusBadRequest, "pricing values must be non-negative")
		return
	}
	if rule.Priority == 0 {
		rule.Priority = 100
	}
	if rule.EffectiveFrom != nil && rule.EffectiveTo != nil && rule.EffectiveTo.Before(*rule.EffectiveFrom) {
		writeError(w, http.StatusBadRequest, "effective_to must be after effective_from")
		return
	}
	if rule.TenantID != nil && strings.TrimSpace(*rule.TenantID) == "" {
		rule.TenantID = nil
	}

	var beforeJSON string
	if rule.ID > 0 {
		existing, err := h.pg.GetPricingRule(r.Context(), rule.ID)
		if err == nil {
			if b, err := json.Marshal(existing); err == nil {
				beforeJSON = string(b)
			}
		}
	}

	updated, err := h.pg.UpsertPricingRule(r.Context(), rule)
	if err != nil {
		h.logger.Error("upsert pricing rule", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "save failed")
		return
	}
	if err := proxy.LoadPricingRules(r.Context(), h.pg); err != nil {
		h.logger.Error("reload pricing rules", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "reload failed")
		return
	}
	if after, err := json.Marshal(updated); err == nil {
		_ = h.pg.CreatePricingRuleAudit(r.Context(), models.PricingAuditEntry{
			RuleID:      updated.ID,
			Action:      map[bool]string{true: "update", false: "create"}[rule.ID > 0],
			Actor:       actorFromCtx(r),
			TenantID:    tenantFromCtx(r),
			BeforeState: beforeJSON,
			AfterState:  string(after),
		})
	}
	h.writeAdminAudit(r, "pricing", map[bool]string{true: "update", false: "create"}[rule.ID > 0], "pricing_rule", strconv.FormatInt(updated.ID, 10), "success", updated)
	h.writeControlHistory(r, "pricing", map[bool]string{true: "update", false: "create"}[rule.ID > 0], "pricing_rule", strconv.FormatInt(updated.ID, 10), updated.ModelPattern, "success", beforeJSON, updated, nil)
	writeJSON(w, http.StatusOK, updated)
}

func (h *Handler) DeletePricingRule(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "ruleId"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid rule id")
		return
	}
	var beforeJSON string
	existing, err := h.pg.GetPricingRule(r.Context(), id)
	if err == nil {
		if b, err := json.Marshal(existing); err == nil {
			beforeJSON = string(b)
		}
	}
	if err := h.pg.DeletePricingRule(r.Context(), id); err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "pricing rule not found")
			return
		}
		h.logger.Error("delete pricing rule", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	if err := proxy.LoadPricingRules(r.Context(), h.pg); err != nil {
		h.logger.Error("reload pricing rules", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "reload failed")
		return
	}
	_ = h.pg.CreatePricingRuleAudit(r.Context(), models.PricingAuditEntry{
		RuleID:      id,
		Action:      "delete",
		Actor:       actorFromCtx(r),
		TenantID:    tenantFromCtx(r),
		BeforeState: beforeJSON,
		AfterState:  "{}",
	})
	h.writeAdminAudit(r, "pricing", "delete", "pricing_rule", strconv.FormatInt(id, 10), "success", existing)
	h.writeControlHistory(r, "pricing", "delete", "pricing_rule", strconv.FormatInt(id, 10), "pricing rule deleted", "success", beforeJSON, nil, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListPolicyRules(w http.ResponseWriter, r *http.Request) {
	rules, err := h.pg.ListPolicyRules(r.Context())
	if err != nil {
		h.logger.Error("list policy rules", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "query error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rules, "count": len(rules)})
}

func (h *Handler) ListPolicyPacks(w http.ResponseWriter, r *http.Request) {
	catalog, err := policy.LoadPackCatalog(h.resolvedPolicyPackRoot())
	if err != nil {
		h.logger.Error("list policy packs", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "policy pack catalog unavailable")
		return
	}
	packs, err := policy.LoadPackDefinitions(h.resolvedPolicyPackRoot())
	if err != nil {
		h.logger.Error("load policy packs", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "policy pack load failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"catalog": catalog,
		"items":   packs,
		"count":   len(packs),
	})
}

func (h *Handler) GetPolicyPack(w http.ResponseWriter, r *http.Request) {
	packID := strings.TrimSpace(chi.URLParam(r, "packId"))
	if packID == "" {
		writeError(w, http.StatusBadRequest, "pack id is required")
		return
	}
	item, err := policy.GetPackDefinition(h.resolvedPolicyPackRoot(), packID)
	if err != nil {
		writeError(w, http.StatusNotFound, "policy pack not found")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) ApplyPolicyPack(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PackIDs  []string `json:"pack_ids"`
		TenantID string   `json:"tenant_id,omitempty"`
		Enabled  *bool    `json:"enabled,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if len(req.PackIDs) == 0 {
		writeError(w, http.StatusBadRequest, "pack_ids is required")
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	var tenantID *string
	if trimmed := strings.TrimSpace(req.TenantID); trimmed != "" {
		tenantID = &trimmed
	} else if trimmed := strings.TrimSpace(tenantFromCtx(r)); trimmed != "" {
		tenantID = &trimmed
	}
	existing, err := h.pg.ListPolicyRules(r.Context())
	if err != nil {
		h.logger.Error("list existing policy rules", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "policy rule query failed")
		return
	}
	existingByName := make(map[string]models.PolicyRule, len(existing))
	for _, rule := range existing {
		key := rule.Name + "|" + firstNonEmptyStringPtr(rule.TenantID)
		existingByName[key] = rule
	}
	applied := []models.PolicyRule{}
	for _, packID := range req.PackIDs {
		pack, err := policy.GetPackDefinition(h.resolvedPolicyPackRoot(), packID)
		if err != nil {
			writeError(w, http.StatusNotFound, "policy pack not found: "+packID)
			return
		}
		compiled, err := policy.CompilePackRules(pack, tenantID, enabled)
		if err != nil {
			h.logger.Error("compile policy pack", zap.Error(err), zap.String("pack_id", packID))
			writeError(w, http.StatusInternalServerError, "policy pack compile failed")
			return
		}
		for _, rule := range compiled {
			key := rule.Name + "|" + firstNonEmptyStringPtr(rule.TenantID)
			if prior, ok := existingByName[key]; ok {
				rule.ID = prior.ID
			}
			saved, err := h.pg.UpsertPolicyRule(r.Context(), rule)
			if err != nil {
				h.logger.Error("upsert compiled policy rule", zap.Error(err), zap.String("rule", rule.Name))
				writeError(w, http.StatusInternalServerError, "policy pack apply failed")
				return
			}
			existingByName[key] = saved
			applied = append(applied, saved)
		}
	}
	if h.policyEngine != nil {
		if err := h.policyEngine.LoadRules(r.Context(), h.pg); err != nil {
			h.logger.Error("reload policy rules", zap.Error(err))
			writeError(w, http.StatusInternalServerError, "policy reload failed")
			return
		}
	}
	h.writeAdminAudit(r, "policy", "apply_pack", "policy_pack", strings.Join(req.PackIDs, ","), "success", map[string]any{
		"pack_ids": req.PackIDs,
		"rules":    len(applied),
		"enabled":  enabled,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"items": applied,
		"count": len(applied),
	})
}

func (h *Handler) ListEvalPacks(w http.ResponseWriter, r *http.Request) {
	catalog, err := evals.LoadPackCatalog(h.resolvedEvalPackRoot())
	if err != nil {
		h.logger.Error("list eval packs", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "eval pack catalog unavailable")
		return
	}
	packs, err := evals.LoadPackDefinitions(h.resolvedEvalPackRoot())
	if err != nil {
		h.logger.Error("load eval packs", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "eval pack load failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"catalog": catalog,
		"items":   packs,
		"count":   len(packs),
	})
}

func (h *Handler) GetEvalPack(w http.ResponseWriter, r *http.Request) {
	packID := strings.TrimSpace(chi.URLParam(r, "packId"))
	if packID == "" {
		writeError(w, http.StatusBadRequest, "pack id is required")
		return
	}
	item, err := evals.GetPackDefinition(h.resolvedEvalPackRoot(), packID)
	if err != nil {
		writeError(w, http.StatusNotFound, "eval pack not found")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func (h *Handler) UpsertPolicyRule(w http.ResponseWriter, r *http.Request) {
	var rule models.PolicyRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	rule.Name = strings.TrimSpace(rule.Name)
	rule.RuleType = strings.ToLower(strings.TrimSpace(rule.RuleType))
	rule.DecisionMode = strings.ToLower(strings.TrimSpace(rule.DecisionMode))
	rule.Action = strings.ToLower(strings.TrimSpace(rule.Action))
	rule.Provider = proxy.NormalizeProvider(rule.Provider)
	rule.ModelPattern = strings.ToLower(strings.TrimSpace(rule.ModelPattern))
	rule.Environment = strings.ToLower(strings.TrimSpace(rule.Environment))
	rule.Detector = strings.ToLower(strings.TrimSpace(rule.Detector))
	rule.Scope = strings.ToLower(strings.TrimSpace(rule.Scope))
	rule.SchemaJSON = strings.TrimSpace(rule.SchemaJSON)
	if rule.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if rule.RuleType != "traffic" && rule.RuleType != "dlp" {
		writeError(w, http.StatusBadRequest, "rule_type must be traffic or dlp")
		return
	}
	if rule.DecisionMode == "" {
		rule.DecisionMode = "fast"
	}
	if rule.DecisionMode != "fast" && rule.DecisionMode != "rego" {
		writeError(w, http.StatusBadRequest, "decision_mode must be fast or rego")
		return
	}
	if rule.Action != "allow" && rule.Action != "warn" && rule.Action != "redact" && rule.Action != "deny" {
		writeError(w, http.StatusBadRequest, "action must be allow, warn, redact, or deny")
		return
	}
	if rule.RuleType == "traffic" && rule.Action == "redact" {
		writeError(w, http.StatusBadRequest, "traffic rules cannot use redact")
		return
	}
	if rule.RolloutPercent <= 0 {
		rule.RolloutPercent = 100
	}
	if rule.RolloutPercent > 100 {
		writeError(w, http.StatusBadRequest, "rollout_percent must be between 1 and 100")
		return
	}
	if rule.Version <= 0 {
		rule.Version = 1
	}
	if rule.DecisionMode == "rego" && strings.TrimSpace(rule.RegoModule) == "" {
		writeError(w, http.StatusBadRequest, "rego decision_mode requires rego_module")
		return
	}
	if rule.RuleType == "dlp" && rule.Scope == "" {
		rule.Scope = "both"
	}
	if rule.RuleType == "traffic" {
		rule.Scope = "both"
		rule.Detector = ""
	}
	for idx, guard := range rule.Guardrails {
		rule.Guardrails[idx] = strings.ToLower(strings.TrimSpace(guard))
	}
	for idx, category := range rule.UnsafeCategories {
		rule.UnsafeCategories[idx] = strings.ToLower(strings.TrimSpace(category))
	}
	if rule.TenantID != nil && strings.TrimSpace(*rule.TenantID) == "" {
		rule.TenantID = nil
	}
	if rule.RuleConditions == nil {
		rule.RuleConditions = map[string]string{}
	}

	var before any
	if rule.ID > 0 {
		existing, err := h.pg.GetPolicyRule(r.Context(), rule.ID)
		if err == nil {
			before = existing
		}
	}
	updated, err := h.pg.UpsertPolicyRule(r.Context(), rule)
	if err != nil {
		h.logger.Error("upsert policy rule", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "save failed")
		return
	}
	if h.policyEngine != nil {
		if err := h.policyEngine.LoadRules(r.Context(), h.pg); err != nil {
			h.logger.Error("reload policy rules", zap.Error(err))
			writeError(w, http.StatusInternalServerError, "reload failed")
			return
		}
	}
	h.writeAdminAudit(r, "policy", map[bool]string{true: "update", false: "create"}[rule.ID > 0], "policy_rule", strconv.FormatInt(updated.ID, 10), "success", map[string]any{
		"before": before,
		"after":  updated,
	})
	h.writeControlHistory(r, "policy", map[bool]string{true: "update", false: "create"}[rule.ID > 0], "policy_rule", strconv.FormatInt(updated.ID, 10), updated.Name, "success", before, updated, nil)
	writeJSON(w, http.StatusOK, updated)
}

func (h *Handler) DeletePolicyRule(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "ruleId"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid rule id")
		return
	}
	existing, _ := h.pg.GetPolicyRule(r.Context(), id)
	if err := h.pg.DeletePolicyRule(r.Context(), id); err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "policy rule not found")
			return
		}
		h.logger.Error("delete policy rule", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	if h.policyEngine != nil {
		if err := h.policyEngine.LoadRules(r.Context(), h.pg); err != nil {
			h.logger.Error("reload policy rules", zap.Error(err))
			writeError(w, http.StatusInternalServerError, "reload failed")
			return
		}
	}
	h.writeAdminAudit(r, "policy", "delete", "policy_rule", strconv.FormatInt(id, 10), "success", existing)
	h.writeControlHistory(r, "policy", "delete", "policy_rule", strconv.FormatInt(id, 10), "policy deleted", "success", existing, nil, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) PreviewPolicyRule(w http.ResponseWriter, r *http.Request) {
	if h.policyEngine == nil {
		writeError(w, http.StatusServiceUnavailable, "policy engine unavailable")
		return
	}

	var req models.PolicyPreviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	req.Provider = proxy.NormalizeProvider(req.Provider)
	req.Model = strings.ToLower(strings.TrimSpace(req.Model))
	req.Environment = strings.ToLower(strings.TrimSpace(req.Environment))

	traffic, requestDLP, responseDLP := h.policyEngine.Evaluate(policy.EvaluationInput{
		TenantID:        strings.TrimSpace(req.TenantID),
		Environment:     req.Environment,
		Provider:        req.Provider,
		Model:           req.Model,
		EstimatedTokens: req.EstimatedTokens,
		Actor:           strings.TrimSpace(req.Actor),
		App:             strings.TrimSpace(req.App),
		Session:         strings.TrimSpace(req.Session),
		RequestBody:     []byte(req.RequestBody),
		ResponseBody:    []byte(req.ResponseBody),
		Attributes:      req.Attributes,
	})
	resp := models.PolicyPreviewResponse{
		Traffic:     previewPolicyDecision(traffic),
		RequestDLP:  previewPolicyDecision(requestDLP),
		ResponseDLP: previewPolicyDecision(responseDLP),
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) SimulatePolicyRule(w http.ResponseWriter, r *http.Request) {
	if h.policyEngine == nil {
		writeError(w, http.StatusServiceUnavailable, "policy engine unavailable")
		return
	}
	var req models.PolicySimulationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	for i := range req.Samples {
		req.Samples[i].Provider = proxy.NormalizeProvider(req.Samples[i].Provider)
		req.Samples[i].Model = strings.ToLower(strings.TrimSpace(req.Samples[i].Model))
		req.Samples[i].Environment = strings.ToLower(strings.TrimSpace(req.Samples[i].Environment))
	}
	writeJSON(w, http.StatusOK, h.policyEngine.Simulate(req))
}

func (h *Handler) PreviewPricingRule(w http.ResponseWriter, r *http.Request) {
	var req models.PricingPreviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	at := time.Now().UTC()
	if req.At != nil {
		at = req.At.UTC()
	}
	match, detailed := proxy.ComputeDetailedCostForTenant(req.Provider, req.Model, req.TenantID, at, pricing.Usage{
		InputTokens:      req.InputTokens,
		OutputTokens:     req.OutputTokens,
		CacheReadTokens:  req.CacheReadTokens,
		CacheWriteTokens: req.CacheWriteTokens,
		ReasoningTokens:  req.ReasoningTokens,
	})
	resp := models.PricingPreviewResponse{
		Matched:              match.RuleID > 0 || match.ModelPattern != "",
		RuleID:               match.RuleID,
		Provider:             req.Provider,
		Model:                req.Model,
		ModelPattern:         match.ModelPattern,
		PricingScope:         match.Scope,
		InputPerMillion:      match.InputPerMillion,
		OutputPerMillion:     match.OutputPerMillion,
		CacheReadPerMillion:  match.CacheReadPerMillion,
		CacheWritePerMillion: match.CacheWritePerMillion,
		ReasoningPerMillion:  match.ReasoningPerMillion,
		InputTokens:          req.InputTokens,
		OutputTokens:         req.OutputTokens,
		CacheReadTokens:      req.CacheReadTokens,
		CacheWriteTokens:     req.CacheWriteTokens,
		ReasoningTokens:      req.ReasoningTokens,
		InputCostUSD:         detailed.InputCostUSD,
		OutputCostUSD:        detailed.OutputCostUSD,
		CacheReadCostUSD:     detailed.CacheReadCostUSD,
		CacheWriteCostUSD:    detailed.CacheWriteCostUSD,
		ReasoningCostUSD:     detailed.ReasoningCostUSD,
		TotalCostUSD:         detailed.TotalCostUSD,
		Explain: pricing.BuildExplanation(pricing.Explanation{
			RuleID:       match.RuleID,
			Provider:     req.Provider,
			MatchedModel: req.Model,
			ModelPattern: match.ModelPattern,
			Scope:        match.Scope,
			RateCard: pricing.RateCard{
				InputPerMillion:      match.InputPerMillion,
				OutputPerMillion:     match.OutputPerMillion,
				CacheReadPerMillion:  match.CacheReadPerMillion,
				CacheWritePerMillion: match.CacheWritePerMillion,
				ReasoningPerMillion:  match.ReasoningPerMillion,
			},
			Result: detailed,
		}),
		EffectiveFrom: match.EffectiveFrom,
		EffectiveTo:   match.EffectiveTo,
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) ListPricingAudit(w http.ResponseWriter, r *http.Request) {
	limit := parseIntOr(r.URL.Query().Get("limit"), 100)
	entries, err := h.pg.ListPricingRuleAudit(r.Context(), limit)
	if err != nil {
		h.logger.Error("list pricing rule audit", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "query error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": entries, "count": len(entries)})
}

func (h *Handler) ExportPricingRules(w http.ResponseWriter, r *http.Request) {
	rules, err := h.pg.ListPricingRules(r.Context())
	if err != nil {
		h.logger.Error("export pricing rules", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "query error")
		return
	}
	var buf bytes.Buffer
	cw := csv.NewWriter(&buf)
	_ = cw.Write([]string{"id", "tenant_id", "provider", "model_pattern", "input_per_million", "output_per_million", "active", "priority", "effective_from", "effective_to", "description"})
	for _, rule := range rules {
		tenantID := ""
		if rule.TenantID != nil {
			tenantID = *rule.TenantID
		}
		effectiveFrom := ""
		if rule.EffectiveFrom != nil {
			effectiveFrom = rule.EffectiveFrom.Format(time.RFC3339)
		}
		effectiveTo := ""
		if rule.EffectiveTo != nil {
			effectiveTo = rule.EffectiveTo.Format(time.RFC3339)
		}
		_ = cw.Write([]string{
			strconv.FormatInt(rule.ID, 10),
			tenantID,
			rule.Provider,
			rule.ModelPattern,
			strconv.FormatFloat(rule.InputPerMillion, 'f', 6, 64),
			strconv.FormatFloat(rule.OutputPerMillion, 'f', 6, 64),
			strconv.FormatBool(rule.Active),
			strconv.Itoa(rule.Priority),
			effectiveFrom,
			effectiveTo,
			rule.Description,
		})
	}
	cw.Flush()
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="pricing-rules.csv"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

// sumSpanUsage totals tokens and cost across a batch of spans.
func sumSpanUsage(spans []models.Span) (tokens int64, costUSD float64) {
	for _, sp := range spans {
		tokens += sp.InputTokens + sp.OutputTokens + sp.CacheReadTokens + sp.CacheWriteTokens + sp.ReasoningTokens
		costUSD += sp.CostUSD
	}
	return
}
