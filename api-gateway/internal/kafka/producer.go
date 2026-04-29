package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	kafkago "github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

type Producer struct {
	writer *kafkago.Writer
	logger *zap.Logger
	topic  string
	closed bool
}

func NewProducer(brokers []string, topic string, logger *zap.Logger) (*Producer, error) {
	if len(brokers) == 0 {
		return nil, fmt.Errorf("no brokers provided")
	}
	w := &kafkago.Writer{
		Addr:                   kafkago.TCP(brokers...),
		Topic:                  topic,
		Balancer:               &kafkago.LeastBytes{},
		AllowAutoTopicCreation: true,
		WriteBackoffMin:        100 * time.Millisecond,
		WriteBackoffMax:        time.Second,
		Compression:            kafkago.Snappy,
	}
	return &Producer{writer: w, logger: logger, topic: topic}, nil
}

func (p *Producer) ProduceSpan(ctx context.Context, span *SpanMessage) error {
	if p.closed {
		return fmt.Errorf("producer is closed")
	}
	if span == nil {
		return fmt.Errorf("span message cannot be nil")
	}
	if span.ReceivedAt.IsZero() {
		span.ReceivedAt = time.Now()
	}
	payload, err := json.Marshal(span)
	if err != nil {
		return fmt.Errorf("marshal span: %w", err)
	}
	return p.writer.WriteMessages(ctx, kafkago.Message{
		Key:   []byte(span.TraceID),
		Value: payload,
		Time:  span.ReceivedAt,
	})
}

func (p *Producer) ProduceSpans(ctx context.Context, spans []*SpanMessage) error {
	if p.closed {
		return fmt.Errorf("producer is closed")
	}
	if len(spans) == 0 {
		return nil
	}
	msgs := make([]kafkago.Message, 0, len(spans))
	for _, span := range spans {
		if span == nil {
			return fmt.Errorf("span message cannot be nil")
		}
		if span.ReceivedAt.IsZero() {
			span.ReceivedAt = time.Now()
		}
		payload, err := json.Marshal(span)
		if err != nil {
			return fmt.Errorf("marshal span: %w", err)
		}
		msgs = append(msgs, kafkago.Message{
			Key:   []byte(span.TraceID),
			Value: payload,
			Time:  span.ReceivedAt,
		})
	}
	return p.writer.WriteMessages(ctx, msgs...)
}

func (p *Producer) Close() error {
	if p.closed {
		return nil
	}
	p.closed = true
	if p.writer != nil {
		return p.writer.Close()
	}
	return nil
}
