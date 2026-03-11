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
	// Fetch recent runs for this agent to get trace IDs, then build topology
	page, err := h.pg.ListRuns(r.Context(), models.RunQuery{
		TenantID:  tenantFromCtx(r),
		AgentName: agentID,
		Limit:     10,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var allSpans []models.Span
	seen := map[string]bool{}
	for _, run := range page.Items {
		if seen[run.TraceID] {
			continue
		}
		seen[run.TraceID] = true
		spans, _ := h.pg.GetTraceSpans(r.Context(), run.TraceID, tenantFromCtx(r))
		allSpans = append(allSpans, spans...)
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
