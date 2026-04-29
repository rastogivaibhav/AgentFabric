package internal

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// SpansIngestedTotal counts total spans received by framework
	SpansIngestedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "collector_spans_ingested_total",
			Help: "Total number of spans ingested by framework",
		},
		[]string{"framework"},
	)

	// IngestLatencySeconds measures time to ingest and export span
	IngestLatencySeconds = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "collector_ingest_latency_seconds",
			Help:    "Time to ingest and export a span",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 8), // 1ms to 256ms
		},
	)

	// PIIScrubbedTotal counts PII items successfully scrubbed
	PIIScrubbedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "collector_pii_scrubbed_total",
			Help: "Total number of PII items scrubbed from spans",
		},
	)

	// PIIDetectedTotal counts PII items detected (before scrubbing)
	PIIDetectedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "collector_pii_detected_total",
			Help: "Total number of PII items detected in spans",
		},
	)

	// ExportErrorsTotal counts export failures to api-gateway
	ExportErrorsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "collector_export_errors_total",
			Help: "Total export errors when sending spans to api-gateway",
		},
	)

	// ExportRetriesTotal counts export retry attempts
	ExportRetriesTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "collector_export_retries_total",
			Help: "Total export retry attempts",
		},
	)

	// GRPCErrorsTotal counts OTLP gRPC errors
	GRPCErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "collector_grpc_errors_total",
			Help: "Total gRPC errors in OTLP receiver",
		},
		[]string{"error_type"},
	)

	// FrameworkDetectionTotal counts successful framework detections
	FrameworkDetectionTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "collector_framework_detection_total",
			Help: "Total framework detections by type",
		},
		[]string{"framework", "detected"},
	)
)
