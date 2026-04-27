package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

// Producer writes span messages to Kafka for durability
type Producer struct {
	writer *kafka.Writer
	logger *zap.Logger
	topic  string
	closed bool
}

// SpanMessage represents a span for Kafka serialization
type SpanMessage struct {
	TraceID       string            `json:"trace_id"`
	SpanID        string            `json:"span_id"`
	Framework     string            `json:"framework"`
	InputTokens   int               `json:"input_tokens"`
	OutputTokens  int               `json:"output_tokens"`
	ModelName     string            `json:"model_name"`
	Attributes    map[string]string `json:"attributes"`
	ReceivedAt    time.Time         `json:"received_at"`
	Cost          float64           `json:"cost,omitempty"`
	CostCurrency  string            `json:"cost_currency,omitempty"`
}

// NewProducer creates a new Kafka producer for spans
func NewProducer(brokers []string, topic string, logger *zap.Logger) (*Producer, error) {
	if len(brokers) == 0 {
		return nil, fmt.Errorf("no brokers provided")
	}

	w := &kafka.Writer{
		Addr:                   kafka.TCP(brokers...),
		Topic:                  topic,
		Balancer:               &kafka.LeastBytes{},
		AllowAutoTopicCreation: true,
		WriteBackoffMin:        100 * time.Millisecond,
		WriteBackoffMax:        1 * time.Second,
		// Async writes with batch and compression
		CompressionCodec: kafka.Snappy,
	}

	return &Producer{
		writer: w,
		logger: logger,
		topic:  topic,
		closed: false,
	}, nil
}

// ProduceSpan writes a span message to Kafka
// Returns error only if write fails; caller should decide on retry strategy
func (p *Producer) ProduceSpan(ctx context.Context, msg *SpanMessage) error {
	if p.closed {
		return fmt.Errorf("producer is closed")
	}

	if msg == nil {
		return fmt.Errorf("span message is nil")
	}

	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal span: %w", err)
	}

	// Use trace_id as key to ensure ordering within a trace
	kafkaMsg := kafka.Message{
		Key:   []byte(msg.TraceID),
		Value: body,
		Time:  time.Now(),
	}

	err = p.writer.WriteMessages(ctx, kafkaMsg)
	if err != nil {
		p.logger.Error(
			"failed to produce span to kafka",
			zap.Error(err),
			zap.String("span_id", msg.SpanID),
			zap.String("trace_id", msg.TraceID),
		)
		return fmt.Errorf("produce: %w", err)
	}

	p.logger.Debug(
		"produced span to kafka",
		zap.String("span_id", msg.SpanID),
		zap.String("trace_id", msg.TraceID),
		zap.String("framework", msg.Framework),
	)

	return nil
}

// ProduceSpans writes multiple span messages in batch
func (p *Producer) ProduceSpans(ctx context.Context, msgs []*SpanMessage) error {
	if p.closed {
		return fmt.Errorf("producer is closed")
	}

	if len(msgs) == 0 {
		return nil
	}

	kafkaMessages := make([]kafka.Message, len(msgs))
	for i, msg := range msgs {
		if msg == nil {
			continue
		}

		body, err := json.Marshal(msg)
		if err != nil {
			p.logger.Warn(
				"marshal span failed, skipping",
				zap.Error(err),
				zap.String("span_id", msg.SpanID),
			)
			continue
		}

		kafkaMessages[i] = kafka.Message{
			Key:   []byte(msg.TraceID),
			Value: body,
			Time:  time.Now(),
		}
	}

	err := p.writer.WriteMessages(ctx, kafkaMessages...)
	if err != nil {
		p.logger.Error(
			"failed to produce spans batch to kafka",
			zap.Error(err),
			zap.Int("batch_size", len(msgs)),
		)
		return fmt.Errorf("produce batch: %w", err)
	}

	p.logger.Debug(
		"produced spans batch to kafka",
		zap.Int("batch_size", len(msgs)),
	)

	return nil
}

// Close closes the producer
func (p *Producer) Close() error {
	if p.closed {
		return nil
	}

	p.closed = true
	return p.writer.Close()
}

// Stats returns current producer statistics
func (p *Producer) Stats() map[string]interface{} {
	if p.writer == nil {
		return map[string]interface{}{}
	}

	// Return writer stats if available
	return map[string]interface{}{
		"topic":  p.topic,
		"closed": p.closed,
	}
}
