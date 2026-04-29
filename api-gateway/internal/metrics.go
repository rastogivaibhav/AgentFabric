package internal

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// HTTPRequestsTotal counts total HTTP requests by method, endpoint, and status
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_http_requests_total",
			Help: "Total HTTP requests by method, endpoint, and status",
		},
		[]string{"method", "endpoint", "status"},
	)

	// HTTPRequestLatencySeconds measures request processing time
	HTTPRequestLatencySeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gateway_http_request_latency_seconds",
			Help:    "HTTP request latency in seconds",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 10), // 1ms to 512ms
		},
		[]string{"method", "endpoint"},
	)

	// HTTPRequestErrorsTotal counts HTTP errors (4xx, 5xx)
	HTTPRequestErrorsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "gateway_http_request_errors_total",
			Help: "Total HTTP request errors (4xx, 5xx)",
		},
	)

	// DBOperationDurationSeconds measures database query duration
	DBOperationDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gateway_db_operation_duration_seconds",
			Help:    "Database operation duration in seconds",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 10), // 1ms to 512ms
		},
		[]string{"operation", "table"},
	)

	// DBConnectionPoolSize tracks active, idle, and waiting connections
	DBConnectionPoolSize = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gateway_db_connection_pool_size",
			Help: "Database connection pool size by state",
		},
		[]string{"state"},
	)

	// RedisOperationDurationSeconds measures Redis command duration
	RedisOperationDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gateway_redis_operation_duration_seconds",
			Help:    "Redis operation duration in seconds",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 8), // 1ms to 256ms
		},
		[]string{"command"},
	)

	// WebSocketConnectionsActive tracks active WebSocket connections
	WebSocketConnectionsActive = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gateway_websocket_connections_active",
			Help: "Active WebSocket connections by tenant",
		},
		[]string{"tenant_id"},
	)

	// WebSocketBroadcastTotal counts broadcast events
	WebSocketBroadcastTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "gateway_websocket_broadcast_total",
			Help: "Total WebSocket broadcast events",
		},
	)

	// SpansProcessedTotal counts spans processed by the gateway
	SpansProcessedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_spans_processed_total",
			Help: "Total spans processed by the gateway",
		},
		[]string{"status"},
	)

	// RateLimitViolationsTotal counts rate limit violations
	RateLimitViolationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_rate_limit_violations_total",
			Help: "Total rate limit violations",
		},
		[]string{"tenant_id", "user_id", "endpoint"},
	)
)
