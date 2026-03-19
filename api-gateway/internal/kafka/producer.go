// Package kafka provides a non-blocking Kafka producer for fan-out span delivery.
//
// Design contract:
//   - Fail-open: if Kafka is unavailable or the publish fails, ingest still succeeds.
//   - Async: PublishSpans returns immediately; messages are written in a goroutine.
//   - Disabled cleanly: if KAFKA_BROKERS is empty the producer is a no-op.
//   - Message format: one JSON message per span, keyed by tenant_id for partitioning.
//     tenant_id is explicitly included (it is omitted from the normal JSON encoding
//     of models.Span to prevent leaking it in API responses).
package kafka

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/agentfabric/api-gateway/internal/models"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

// Producer wraps a kafka.Writer with fail-open semantics.
type Producer struct {
	writer  *kafka.Writer
	logger  *zap.Logger
	enabled bool
}

// spanWire is models.Span with tenant_id re-added for the Kafka payload.
// models.Span has json:"-" on TenantID to prevent leaking it in API responses,
// so we use a wrapper here that explicitly includes it.
type spanWire struct {
	SpanID       string            `json:"span_id"`
	TraceID      string            `json:"trace_id"`
	ParentID     string            `json:"parent_span_id,omitempty"`
	RunID        string            `json:"run_id"`
	Name         string            `json:"name"`
	Framework    string            `json:"framework"`
	StartTimeNs  int64             `json:"start_time_ns"`
	DurationNs   int64             `json:"duration_ns"`
	StatusCode   int               `json:"status_code"`
	StatusMsg    string            `json:"status_msg,omitempty"`
	Attributes   map[string]string `json:"attributes"`
	InputTokens  int64             `json:"input_tokens"`
	OutputTokens int64             `json:"output_tokens"`
	CostUSD      float64           `json:"cost_usd"`
	TenantID     string            `json:"tenant_id"` // re-added for Kafka
	ReceivedAt   time.Time         `json:"received_at"`
}

func fromSpan(sp models.Span) spanWire {
	return spanWire{
		SpanID:       sp.ID,
		TraceID:      sp.TraceID,
		ParentID:     sp.ParentID,
		RunID:        sp.RunID,
		Name:         sp.Name,
		Framework:    sp.Framework,
		StartTimeNs:  sp.StartTimeNs,
		DurationNs:   sp.DurationNs,
		StatusCode:   sp.StatusCode,
		StatusMsg:    sp.StatusMsg,
		Attributes:   sp.Attributes,
		InputTokens:  sp.InputTokens,
		OutputTokens: sp.OutputTokens,
		CostUSD:      sp.CostUSD,
		TenantID:     sp.TenantID,
		ReceivedAt:   sp.ReceivedAt,
	}
}

// NewProducer creates a Kafka producer.
// If brokers is empty the producer is disabled (all calls are no-ops).
func NewProducer(brokers, topic string, logger *zap.Logger) *Producer {
	if brokers == "" {
		logger.Info("kafka producer disabled (KAFKA_BROKERS not set)")
		return &Producer{enabled: false, logger: logger}
	}

	addrs := strings.Split(brokers, ",")
	w := &kafka.Writer{
		Addr:         kafka.TCP(addrs...),
		Topic:        topic,
		Async:        true, // non-blocking; errors surfaced via logger
		BatchSize:    100,
		BatchTimeout: 200 * time.Millisecond,
		RequiredAcks: kafka.RequireOne,
		MaxAttempts:  3,
		ErrorLogger:  kafka.LoggerFunc(func(msg string, args ...interface{}) {
			logger.Sugar().Warnf("kafka-go: "+msg, args...)
		}),
	}

	logger.Info("kafka producer enabled",
		zap.String("brokers", brokers),
		zap.String("topic", topic),
	)

	return &Producer{
		writer:  w,
		logger:  logger,
		enabled: true,
	}
}

// PublishSpans fans spans out to Kafka asynchronously.
// Returns immediately — Kafka errors are logged but never block ingest.
func (p *Producer) PublishSpans(ctx context.Context, spans []models.Span) {
	if !p.enabled || p.writer == nil || len(spans) == 0 {
		return
	}

	msgs := make([]kafka.Message, 0, len(spans))
	for _, sp := range spans {
		b, err := json.Marshal(fromSpan(sp))
		if err != nil {
			p.logger.Warn("kafka: span marshal failed",
				zap.String("span_id", sp.ID),
				zap.Error(err),
			)
			continue
		}
		msgs = append(msgs, kafka.Message{
			Key:   []byte(sp.TenantID), // partition by tenant
			Value: b,
		})
	}

	if len(msgs) == 0 {
		return
	}

	// Fire-and-forget goroutine — ingest latency is unaffected by Kafka
	go func() {
		// Use a fresh context so a cancelled request context doesn't abort the write
		writeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := p.writer.WriteMessages(writeCtx, msgs...); err != nil {
			p.logger.Warn("kafka: publish failed (fail-open — ingest unaffected)",
				zap.Int("count", len(msgs)),
				zap.Error(err),
			)
		}
	}()
}

// Close flushes pending messages and releases resources.
func (p *Producer) Close() error {
	if p.writer != nil {
		return p.writer.Close()
	}
	return nil
}
