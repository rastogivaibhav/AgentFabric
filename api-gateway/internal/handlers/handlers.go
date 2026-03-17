package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/agentfabric/api-gateway/internal/models"
	"github.com/agentfabric/api-gateway/internal/store"
	"github.com/agentfabric/api-gateway/internal/ws"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

type Handler struct {
	pg        *store.PostgresStore
	redis     *store.RedisClient
	hub       *ws.Hub
	logger    *zap.Logger
	jwtSecret string
}

func New(pg *store.PostgresStore, redis *store.RedisClient, hub *ws.Hub, logger *zap.Logger, jwtSecret string) *Handler {
	return &Handler{pg: pg, redis: redis, hub: hub, logger: logger, jwtSecret: jwtSecret}
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func tenantFromCtx(r *http.Request) string {
	if t, ok := r.Context().Value("tenant_id").(string); ok && t != "" {
		return t
	}
	return "default"
}

// ─── Health ──────────────────────────────────────────────────────────────────

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ─── Ingest (internal, called by collector) ──────────────────────────────────

type ingestRequest struct {
	Spans []models.Span `json:"spans"`
}

func (h *Handler) Ingest(w http.ResponseWriter, r *http.Request) {
	var req ingestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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

	// Tenant from collector header or default
	tenantID := r.Header.Get("X-AF-Tenant")
	if tenantID == "" {
		tenantID = "default"
	}
	for i := range req.Spans {
		req.Spans[i].TenantID = tenantID
		if req.Spans[i].ReceivedAt.IsZero() {
			req.Spans[i].ReceivedAt = time.Now()
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
	writeJSON(w, http.StatusOK, map[string]interface{}{"spans": spans})
}

func (h *Handler) GetTraceCost(w http.ResponseWriter, r *http.Request) {
	traceID := chi.URLParam(r, "traceId")
	spans, err := h.pg.GetTraceSpans(r.Context(), traceID, tenantFromCtx(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
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
	agents, err := h.pg.ListAgents(r.Context(), tenantFromCtx(r), 200)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, a := range agents {
		if a.ID == agentID || a.Name == agentID {
			writeJSON(w, http.StatusOK, a)
			return
		}
	}
	writeError(w, http.StatusNotFound, "agent not found")
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
	t.Framework = spans[0].Framework
	t.RootSpanName = spans[0].Name
	t.StartTime = time.Unix(0, spans[0].StartTimeNs)
	var maxEnd int64
	for _, sp := range spans {
		end := sp.StartTimeNs + sp.DurationNs
		if end > maxEnd {
			maxEnd = end
		}
		t.TotalCostUSD += sp.CostUSD
		t.TotalTokens += sp.InputTokens + sp.OutputTokens
		if sp.StatusCode == 2 {
			t.ErrorCount++
		}
	}
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

func parseIntOr(s string, def int) int {
	if v, err := strconv.Atoi(s); err == nil && v > 0 {
		return v
	}
	return def
}
