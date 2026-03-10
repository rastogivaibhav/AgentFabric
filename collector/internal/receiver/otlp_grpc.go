package receiver

import (
	"context"
	"time"

	"github.com/agentfabric/collector/internal/config"
	"github.com/agentfabric/collector/internal/processor"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

var (
	receivedSpans = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "agentfabric_received_spans_total",
		Help: "Total spans received by the collector",
	}, []string{"framework"})

	receiveErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "agentfabric_receive_errors_total",
		Help: "Errors during span receipt",
	}, []string{"type"})

	receiveLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "agentfabric_receive_latency_seconds",
		Help:    "Latency of span receipt processing",
		Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1},
	}, []string{"source"})
)

// OTLPReceiver implements the OTLP gRPC TraceService.
type OTLPReceiver struct {
	coltracepb.UnimplementedTraceServiceServer
	processor *processor.AgentProcessor
	logger    *zap.Logger
	cfg       *config.Config
}

func NewOTLPReceiver(p *processor.AgentProcessor, logger *zap.Logger, cfg *config.Config) *OTLPReceiver {
	return &OTLPReceiver{processor: p, logger: logger, cfg: cfg}
}

func (r *OTLPReceiver) Export(
	ctx context.Context,
	req *coltracepb.ExportTraceServiceRequest,
) (*coltracepb.ExportTraceServiceResponse, error) {
	start := time.Now()
	defer func() {
		receiveLatency.WithLabelValues("grpc").Observe(time.Since(start).Seconds())
	}()

	if req == nil || len(req.ResourceSpans) == 0 {
		return &coltracepb.ExportTraceServiceResponse{}, nil
	}

	spans := flattenSpans(req.ResourceSpans)
	if len(spans) == 0 {
		return &coltracepb.ExportTraceServiceResponse{}, nil
	}

	// Hard limit per request: drop oversized batches
	if len(spans) > 10000 {
		receiveErrors.WithLabelValues("batch_too_large").Inc()
		return nil, status.Errorf(codes.ResourceExhausted, "batch exceeds 10000 spans")
	}

	if err := r.processor.Submit(ctx, req.ResourceSpans); err != nil {
		receiveErrors.WithLabelValues("submit_failed").Inc()
		r.logger.Warn("span submission failed", zap.Error(err), zap.Int("count", len(spans)))
		return nil, status.Errorf(codes.Unavailable, "processor unavailable: %v", err)
	}

	receivedSpans.WithLabelValues("grpc").Add(float64(len(spans)))
	return &coltracepb.ExportTraceServiceResponse{}, nil
}

func flattenSpans(rss []*tracepb.ResourceSpans) []*tracepb.Span {
	var out []*tracepb.Span
	for _, rs := range rss {
		for _, ss := range rs.ScopeSpans {
			out = append(out, ss.Spans...)
		}
	}
	return out
}
