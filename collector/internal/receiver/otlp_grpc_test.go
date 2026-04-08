package receiver

import (
	"context"
	"testing"

	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"

	"github.com/govagn/collector/internal/config"
	"github.com/govagn/collector/internal/processor"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"go.uber.org/zap"
)

// newTestReceiver wires up a real AgentProcessor backed by a no-op exporter.
// Suitable for unit tests that need to exercise Export() without a live gateway.
func newTestReceiver(t *testing.T) *OTLPReceiver {
	t.Helper()
	cfg := &config.Config{}
	cfg.Processor.BatchSize = 512
	cfg.Processor.BatchTimeoutMS = 100
	cfg.Processor.MaxQueueSize = 10000
	cfg.Processor.Workers = 2
	cfg.Processor.MaxAttributeLen = 4096
	cfg.PII.Enabled = false // disable PII for unit tests

	exp := &noopExporter{}
	// Workers are started inside NewAgentProcessor; no separate Start() call needed.
	p := processor.NewAgentProcessor(cfg, zap.NewNop(), exp)
	t.Cleanup(p.Shutdown)

	return NewOTLPReceiver(p, zap.NewNop(), cfg)
}

// noopExporter satisfies exporter.Exporter without any network calls.
type noopExporter struct{ lastCount int }

func (n *noopExporter) Export(_ context.Context, spans interface{}) error {
	return nil
}

// ─── Export() ─────────────────────────────────────────────────────────────────

func TestExport_NilRequest_ReturnsEmpty(t *testing.T) {
	recv := newTestReceiver(t)
	resp, err := recv.Export(context.Background(), nil)
	if err != nil {
		t.Fatalf("nil request should not error: %v", err)
	}
	if resp == nil {
		t.Error("response should not be nil for nil request")
	}
}

func TestExport_EmptyResourceSpans_ReturnsEmpty(t *testing.T) {
	recv := newTestReceiver(t)
	req := &coltracepb.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{}}
	resp, err := recv.Export(context.Background(), req)
	if err != nil {
		t.Fatalf("empty resource spans should not error: %v", err)
	}
	if resp == nil {
		t.Error("response should not be nil")
	}
}

func TestExport_ValidSpan_Accepted(t *testing.T) {
	recv := newTestReceiver(t)
	req := &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{
			{
				ScopeSpans: []*tracepb.ScopeSpans{
					{
						Spans: []*tracepb.Span{
							{
								TraceId: make([]byte, 16),
								SpanId:  make([]byte, 8),
								Name:    "test-span",
							},
						},
					},
				},
			},
		},
	}
	_, err := recv.Export(context.Background(), req)
	if err != nil {
		t.Fatalf("valid span should be accepted: %v", err)
	}
}

func TestExport_BatchTooLarge_ReturnsResourceExhausted(t *testing.T) {
	recv := newTestReceiver(t)

	// Build a batch of 10001 spans — one over the hard limit
	spans := make([]*tracepb.Span, 10001)
	for i := range spans {
		spans[i] = &tracepb.Span{
			TraceId: make([]byte, 16),
			SpanId:  make([]byte, 8),
			Name:    "span",
		}
	}
	req := &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{
			{ScopeSpans: []*tracepb.ScopeSpans{{Spans: spans}}},
		},
	}
	_, err := recv.Export(context.Background(), req)
	if err == nil {
		t.Error("batch of 10001 spans should be rejected with ResourceExhausted")
	}
}

// ─── flattenSpans ─────────────────────────────────────────────────────────────

func TestFlattenSpans_Empty(t *testing.T) {
	result := flattenSpans(nil)
	if len(result) != 0 {
		t.Errorf("flattenSpans(nil) expected 0 spans, got %d", len(result))
	}
}

func TestFlattenSpans_MultipleScopes(t *testing.T) {
	rss := []*tracepb.ResourceSpans{
		{
			ScopeSpans: []*tracepb.ScopeSpans{
				{Spans: []*tracepb.Span{{Name: "s1"}, {Name: "s2"}}},
				{Spans: []*tracepb.Span{{Name: "s3"}}},
			},
		},
		{
			ScopeSpans: []*tracepb.ScopeSpans{
				{Spans: []*tracepb.Span{{Name: "s4"}}},
			},
		},
	}
	result := flattenSpans(rss)
	if len(result) != 4 {
		t.Errorf("expected 4 spans, got %d", len(result))
	}
}
