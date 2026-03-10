package receiver

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/agentfabric/collector/internal/auth"
	"github.com/agentfabric/collector/internal/processor"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

type HTTPOTLPHandler struct {
	processor    *processor.AgentProcessor
	jwtValidator *auth.JWTValidator
	logger       *zap.Logger
}

func NewHTTPOTLPHandler(p *processor.AgentProcessor, jv *auth.JWTValidator, logger *zap.Logger) http.Handler {
	return &HTTPOTLPHandler{processor: p, jwtValidator: jv, logger: logger}
}

func (h *HTTPOTLPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Validate bearer token
	if err := h.jwtValidator.ValidateHTTP(r); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 32*1024*1024))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req coltracepb.ExportTraceServiceRequest

	ct := r.Header.Get("Content-Type")
	switch ct {
	case "application/x-protobuf":
		if err := proto.Unmarshal(body, &req); err != nil {
			http.Error(w, "invalid protobuf", http.StatusBadRequest)
			return
		}
	case "application/json":
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
	default:
		// Try protobuf as default
		if err := proto.Unmarshal(body, &req); err != nil {
			http.Error(w, "unsupported content type", http.StatusUnsupportedMediaType)
			return
		}
	}

	ctx := r.Context()
	start := time.Now()

	if err := h.processor.Submit(ctx, req.ResourceSpans); err != nil {
		receiveLatency.WithLabelValues("http").Observe(time.Since(start).Seconds())
		h.logger.Warn("HTTP span submission failed", zap.Error(err))
		http.Error(w, "processor unavailable", http.StatusServiceUnavailable)
		return
	}

	receiveLatency.WithLabelValues("http").Observe(time.Since(start).Seconds())
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{}`))
}
