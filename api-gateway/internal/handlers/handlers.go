package handlers

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/agentfabric/api-gateway/internal/budget"
	"github.com/agentfabric/api-gateway/internal/middleware"
	"github.com/agentfabric/api-gateway/internal/models"
	"github.com/agentfabric/api-gateway/internal/policy"
	"github.com/agentfabric/api-gateway/internal/proxy"
	"github.com/agentfabric/api-gateway/internal/store"
	"github.com/agentfabric/api-gateway/internal/ws"
	"github.com/go-chi/chi/v5"
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
}

func New(pg *store.PostgresStore, redis *store.RedisClient, hub *ws.Hub, logger *zap.Logger, jwtSecret string, budgetEnforcer *budget.BudgetEnforcer, policyEngine *policy.Engine) *Handler {
	return &Handler{
		pg:             pg,
		redis:          redis,
		hub:            hub,
		logger:         logger,
		jwtSecret:      jwtSecret,
		budgetEnforcer: budgetEnforcer,
		policyEngine:   policyEngine,
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

// ─── Health ──────────────────────────────────────────────────────────────────

// Health checks both Postgres and Redis with a 2-second timeout each.
// Returns 200 {"status":"ok"} when all deps are reachable.
// Returns 503 {"status":"degraded","error":"..."} if any dep fails.
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := h.pg.Ping(ctx); err != nil {
		h.logger.Warn("healthz: postgres ping failed", zap.Error(err))
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "degraded",
			"error":  "postgres unavailable",
		})
		return
	}
	if err := h.redis.Ping(ctx); err != nil {
		h.logger.Warn("healthz: redis ping failed", zap.Error(err))
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "degraded",
			"error":  "redis unavailable",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ─── Ingest (internal, called by collector) ──────────────────────────────────

type ingestRequest struct {
	Spans []models.Span `json:"spans"`
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
	}

	// Budget enforcement — check before writing to DB.
	// Fail open: if the enforcer errors, we still allow the ingest.
	if h.budgetEnforcer != nil {
		totalTokens, totalCost := sumSpanUsage(req.Spans)
		allowed, err := h.budgetEnforcer.CheckAndRecord(r.Context(), tenantID, totalTokens, totalCost)
		if err != nil {
			h.logger.Warn("budget check error (fail open)", zap.String("tenant", tenantID), zap.Error(err))
		}
		if !allowed {
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
	for _, sp := range req.Spans {
		h.hub.Broadcast(tenantID, &models.LiveEvent{
			Type:      "span",
			Timestamp: time.Now().UnixMilli(),
			TenantID:  tenantID,
			Data:      sp,
		})
	}

	w.WriteHeader(http.StatusOK)
}

// ─── Traces ──────────────────────────────────────────────────────────────────

func (h *Handler) ListTraces(w http.ResponseWriter, r *http.Request) {
	q := models.TraceQuery{
		TenantID:  tenantFromCtx(r),
		Framework: r.URL.Query().Get("framework"),
		Model:     r.URL.Query().Get("model"),
		AgentName: r.URL.Query().Get("agent"),
		Status:    r.URL.Query().Get("status"),
		Limit:     parseIntOr(r.URL.Query().Get("limit"), 50),
		Cursor:    r.URL.Query().Get("cursor"),
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

	spans, err := h.pg.GetTraceSpans(r.Context(), traceID, tenantID)
	if err != nil || len(spans) == 0 {
		writeError(w, http.StatusNotFound, "trace not found")
		return
	}

	trace = buildTrace(traceID, spans)
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
	spans, err := h.pg.GetTraceSpans(r.Context(), traceID, tenantFromCtx(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	decorateSpans(spans)
	writeJSON(w, http.StatusOK, map[string]interface{}{"spans": spans})
}

func (h *Handler) GetTraceCost(w http.ResponseWriter, r *http.Request) {
	traceID := chi.URLParam(r, "traceId")
	spans, err := h.pg.GetTraceSpans(r.Context(), traceID, tenantFromCtx(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	decorateSpans(spans)
	type costBreakdown struct {
		SpanID    string  `json:"span_id"`
		Name      string  `json:"name"`
		Model     string  `json:"model"`
		InputTok  int64   `json:"input_tokens"`
		OutputTok int64   `json:"output_tokens"`
		CostUSD   float64 `json:"cost_usd"`
	}
	var total float64
	breakdown := make([]costBreakdown, 0, len(spans))
	for _, sp := range spans {
		if sp.CostUSD > 0 {
			breakdown = append(breakdown, costBreakdown{
				SpanID:    sp.ID,
				Name:      sp.Name,
				Model:     sp.Attributes["gen_ai.request.model"],
				InputTok:  sp.InputTokens,
				OutputTok: sp.OutputTokens,
				CostUSD:   sp.CostUSD,
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
	since := 24 * time.Hour
	if s := r.URL.Query().Get("since"); s != "" {
		if d, err := time.ParseDuration(s); err == nil {
			since = d
		}
	}
	rows, err := h.pg.GetCostReport(r.Context(), tenantFromCtx(r), since)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rows)
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

	user, err := h.pg.UpdateUser(r.Context(), chi.URLParam(r, "userId"), tenantFromCtx(r), req)
	if err != nil {
		h.logger.Error("update user", zap.Error(err))
		writeError(w, http.StatusNotFound, "user not found or no change")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

// DeleteUser removes a user from the current tenant.
// DELETE /api/v1/users/{userId}
func (h *Handler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	if err := h.pg.DeleteUser(r.Context(), chi.URLParam(r, "userId"), tenantFromCtx(r)); err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
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
		t.TotalTokens += sp.InputTokens + sp.OutputTokens
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
		t.Insights.RetryCount += sp.RetryCount
		if sp.Depth > t.Insights.MaxDepth {
			t.Insights.MaxDepth = sp.Depth
		}
		if sp.StatusCode == 2 {
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
	} else {
		t.Status = "ok"
	}
	return t
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
	sp.ErrorClass = firstNonEmpty(sp.Attributes["af.error.class"], sp.Attributes["error.type"], sp.Attributes["exception.type"])
	sp.PromptPreview = firstPreview(sp.Attributes, "af.preview.prompt", "gen_ai.prompt", "input.value", "prompt", "llm.prompt")
	sp.ResponsePreview = firstPreview(sp.Attributes, "af.preview.response", "gen_ai.response", "output.value", "response", "llm.response")
	sp.RetryCount = firstInt(sp.Attributes, "retry.count", "http.retry_count", "af.retry.count")
	sp.BlockedReason = firstNonEmpty(sp.Attributes["af.policy.reason"], sp.Attributes["policy.reason"], sp.Attributes["budget.reason"])
	sp.Blocked = isTrue(sp.Attributes["af.policy.blocked"]) || strings.EqualFold(sp.Attributes["af.policy.decision"], "deny") || sp.BlockedReason != ""
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

func parseIntOr(s string, def int) int {
	if v, err := strconv.Atoi(s); err == nil && v > 0 {
		return v
	}
	return def
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
	match, inputCost, totalCost := proxy.ComputeExactCostForTenant(provider, model, sp.TenantID, sp.ReceivedAt, sp.InputTokens, sp.OutputTokens)
	sp.CostUSD = totalCost

	if sp.Attributes == nil {
		sp.Attributes = map[string]string{}
	}
	if model != "" {
		sp.Attributes["gen_ai.request.model"] = model
	}
	if provider != "" {
		sp.Attributes["gen_ai.system"] = strings.ToLower(provider)
	}
	sp.Attributes["af.cost.input_usd"] = strconv.FormatFloat(inputCost, 'f', 8, 64)
	sp.Attributes["af.cost.output_usd"] = strconv.FormatFloat(totalCost-inputCost, 'f', 8, 64)
	sp.Attributes["af.cost.total_usd"] = strconv.FormatFloat(totalCost, 'f', 8, 64)
	if match.RuleID > 0 {
		sp.Attributes["af.pricing.rule_id"] = strconv.FormatInt(match.RuleID, 10)
		sp.Attributes["af.pricing.model_pattern"] = match.ModelPattern
		sp.Attributes["af.pricing.scope"] = match.Scope
		sp.Attributes["af.pricing.input_per_million"] = strconv.FormatFloat(match.InputPerMillion, 'f', 4, 64)
		sp.Attributes["af.pricing.output_per_million"] = strconv.FormatFloat(match.OutputPerMillion, 'f', 4, 64)
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
	rule.Provider = strings.ToLower(strings.TrimSpace(rule.Provider))
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

func (h *Handler) UpsertPolicyRule(w http.ResponseWriter, r *http.Request) {
	var rule models.PolicyRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	rule.Name = strings.TrimSpace(rule.Name)
	rule.RuleType = strings.ToLower(strings.TrimSpace(rule.RuleType))
	rule.Action = strings.ToLower(strings.TrimSpace(rule.Action))
	rule.Provider = strings.ToLower(strings.TrimSpace(rule.Provider))
	rule.ModelPattern = strings.ToLower(strings.TrimSpace(rule.ModelPattern))
	rule.Environment = strings.ToLower(strings.TrimSpace(rule.Environment))
	rule.Detector = strings.ToLower(strings.TrimSpace(rule.Detector))
	rule.Scope = strings.ToLower(strings.TrimSpace(rule.Scope))
	if rule.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if rule.RuleType != "traffic" && rule.RuleType != "dlp" {
		writeError(w, http.StatusBadRequest, "rule_type must be traffic or dlp")
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
	if rule.RuleType == "dlp" && rule.Scope == "" {
		rule.Scope = "both"
	}
	if rule.RuleType == "traffic" {
		rule.Scope = "both"
		rule.Detector = ""
	}
	if rule.TenantID != nil && strings.TrimSpace(*rule.TenantID) == "" {
		rule.TenantID = nil
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
	w.WriteHeader(http.StatusNoContent)
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
	match, inputCost, totalCost := proxy.ComputeExactCostForTenant(req.Provider, req.Model, req.TenantID, at, req.InputTokens, req.OutputTokens)
	resp := models.PricingPreviewResponse{
		Matched:          match.RuleID > 0 || match.ModelPattern != "",
		RuleID:           match.RuleID,
		Provider:         req.Provider,
		Model:            req.Model,
		ModelPattern:     match.ModelPattern,
		PricingScope:     match.Scope,
		InputPerMillion:  match.InputPerMillion,
		OutputPerMillion: match.OutputPerMillion,
		InputTokens:      req.InputTokens,
		OutputTokens:     req.OutputTokens,
		InputCostUSD:     inputCost,
		OutputCostUSD:    totalCost - inputCost,
		TotalCostUSD:     totalCost,
		EffectiveFrom:    match.EffectiveFrom,
		EffectiveTo:      match.EffectiveTo,
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
		tokens += sp.InputTokens + sp.OutputTokens
		costUSD += sp.CostUSD
	}
	return
}
