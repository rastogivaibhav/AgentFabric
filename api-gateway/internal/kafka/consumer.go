package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

// Consumer reads span messages from Kafka and persists them
type Consumer struct {
	reader    *kafka.Reader
	logger    *zap.Logger
	topic     string
	groupID   string
	closed    bool
	committed int64
}

// SpanMessage matches the format from collector/kafka/producer.go
type SpanMessage struct {
	TraceID      string            `json:"trace_id"`
	SpanID       string            `json:"span_id"`
	Framework    string            `json:"framework"`
	InputTokens  int               `json:"input_tokens"`
	OutputTokens int               `json:"output_tokens"`
	ModelName    string            `json:"model_name"`
	Attributes   map[string]string `json:"attributes"`
	ReceivedAt   time.Time         `json:"received_at"`
	Cost         float64           `json:"cost,omitempty"`
	CostCurrency string            `json:"cost_currency,omitempty"`
}

// PersistFunc defines the callback for persisting spans
type PersistFunc func(context.Context, *SpanMessage) error

// NewConsumer creates a new Kafka consumer for spans
func NewConsumer(brokers []string, topic, groupID string, logger *zap.Logger) (*Consumer, error) {
	if len(brokers) == 0 {
		return nil, fmt.Errorf("no brokers provided")
	}

	if groupID == "" {
		return nil, fmt.Errorf("group_id is required")
	}

	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:         brokers,
		Topic:           topic,
		GroupID:         groupID,
		StartOffset:     kafka.LastOffset,
		CommitInterval:  time.Second,
		PartitionWatchInterval: 5 * time.Second,
		MaxBytes:        1024 * 1024, // 1MB max message
		QueueCapacity:   100,
	})

	return &Consumer{
		reader:  r,
		logger:  logger,
		topic:   topic,
		groupID: groupID,
		closed:  false,
	}, nil
}

// Start begins consuming spans from Kafka
// Calls persist for each message; if persist succeeds, message is committed
func (c *Consumer) Start(ctx context.Context, persist PersistFunc) error {
	if c.closed {
		return fmt.Errorf("consumer is closed")
	}

	if persist == nil {
		return fmt.Errorf("persist function is required")
	}

	defer func() {
		c.logger.Info(
			"consumer stopped",
			zap.String("topic", c.topic),
			zap.String("group_id", c.groupID),
			zap.Int64("messages_committed", c.committed),
		)
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Fetch message with timeout
		fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		msg, err := c.reader.FetchMessage(fetchCtx)
		cancel()

		if err != nil {
			if err == context.Canceled || err == context.DeadlineExceeded {
				continue
			}
			c.logger.Warn("fetch message failed", zap.Error(err))
			continue
		}

		// Unmarshal span
		var span SpanMessage
		if err := json.Unmarshal(msg.Value, &span); err != nil {
			c.logger.Warn(
				"unmarshal span failed, skipping",
				zap.Error(err),
				zap.String("message", string(msg.Value[:min(100, len(msg.Value))])),
			)
			// Still commit to avoid reprocessing bad messages
			commitErr := c.reader.CommitMessages(ctx, msg)
			if commitErr != nil {
				c.logger.Error("commit after unmarshal error failed", zap.Error(commitErr))
			}
			continue
		}

		// Persist span
		persistErr := persist(ctx, &span)
		if persistErr != nil {
			c.logger.Error(
				"persist span failed, will retry",
				zap.Error(persistErr),
				zap.String("span_id", span.SpanID),
				zap.String("trace_id", span.TraceID),
			)
			// Don't commit on persist error; will retry on next run
			continue
		}

		// Commit after successful persist
		commitErr := c.reader.CommitMessages(ctx, msg)
		if commitErr != nil {
			c.logger.Error(
				"commit message failed",
				zap.Error(commitErr),
				zap.String("span_id", span.SpanID),
			)
			// Don't return error; consumer should keep running
			continue
		}

		c.committed++
		c.logger.Debug(
			"consumed and persisted span",
			zap.String("span_id", span.SpanID),
			zap.String("trace_id", span.TraceID),
			zap.Int("offset", int(msg.Offset)),
		)
	}
}

// Close closes the consumer
func (c *Consumer) Close() error {
	if c.closed {
		return nil
	}

	c.closed = true
	if c.reader != nil {
		return c.reader.Close()
	}
	return nil
}

// Stats returns consumer statistics
func (c *Consumer) Stats() map[string]interface{} {
	return map[string]interface{}{
		"topic":              c.topic,
		"group_id":           c.groupID,
		"closed":             c.closed,
		"messages_committed": c.committed,
	}
}

// Helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
