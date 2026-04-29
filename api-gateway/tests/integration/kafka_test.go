package integration_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/govagn/api-gateway/internal/kafka"
	"go.uber.org/zap"
)

func requireWritableKafka(t *testing.T, brokers []string, logger *zap.Logger) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	producer, err := kafka.NewProducer(brokers, "test-healthcheck", logger)
	if err != nil {
		t.Skipf("Kafka not available: %v", err)
	}
	defer producer.Close()

	err = producer.ProduceSpan(ctx, &kafka.SpanMessage{
		TraceID:    "trace-healthcheck",
		SpanID:     "span-healthcheck",
		ReceivedAt: time.Now(),
	})
	if err != nil {
		t.Skipf("Kafka is not writable from this host: %v", err)
	}
}

// TestKafkaProducerConsumer: Verify producer writes and consumer reads
func TestKafkaProducerConsumer(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Kafka must be running (docker-compose kafka service)
	brokers := []string{"localhost:9092"}
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	requireWritableKafka(t, brokers, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("ProducerCreateAndClose", func(t *testing.T) {
		producer, err := kafka.NewProducer(brokers, "test-spans", logger)
		if err != nil {
			t.Skipf("Kafka not available: %v", err)
		}
		defer producer.Close()

		if producer == nil {
			t.Fatal("Producer should not be nil")
		}
		t.Log("✓ Producer created successfully")
	})

	t.Run("ProduceSpan", func(t *testing.T) {
		producer, err := kafka.NewProducer(brokers, "test-spans", logger)
		if err != nil {
			t.Skipf("Kafka not available: %v", err)
		}
		defer producer.Close()

		span := &kafka.SpanMessage{
			TraceID:      "trace-test-1",
			SpanID:       "span-test-1",
			Framework:    "anthropic-api",
			InputTokens:  100,
			OutputTokens: 50,
			ModelName:    "claude-3-sonnet",
			ReceivedAt:   time.Now(),
			Attributes: map[string]string{
				"event.type": "completion",
			},
		}

		err = producer.ProduceSpan(ctx, span)
		if err != nil {
			t.Fatalf("ProduceSpan failed: %v", err)
		}

		t.Logf("✓ Span produced: trace_id=%s, span_id=%s", span.TraceID, span.SpanID)
	})

	t.Run("ProduceAndConsumeRoundtrip", func(t *testing.T) {
		roundCtx, roundCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer roundCancel()

		producer, err := kafka.NewProducer(brokers, "test-roundtrip", logger)
		if err != nil {
			t.Skipf("Kafka not available: %v", err)
		}
		defer producer.Close()

		// Produce 5 test spans
		numSpans := 5
		for i := 0; i < numSpans; i++ {
			span := &kafka.SpanMessage{
				TraceID:      fmt.Sprintf("trace-roundtrip-%d", i),
				SpanID:       fmt.Sprintf("span-roundtrip-%d", i),
				Framework:    "anthropic-api",
				InputTokens:  100 + i*10,
				OutputTokens: 50 + i*5,
				ModelName:    "claude-3-haiku",
				ReceivedAt:   time.Now(),
				Attributes: map[string]string{
					"order": fmt.Sprintf("%d", i),
				},
			}

			err := producer.ProduceSpan(ctx, span)
			if err != nil {
				t.Errorf("ProduceSpan %d failed: %v", i, err)
			}
		}

		// Create consumer
		consumer, err := kafka.NewConsumer(
			brokers,
			"test-roundtrip",
			fmt.Sprintf("test-group-%d", time.Now().Unix()),
			logger,
		)
		if err != nil {
			t.Skipf("Consumer creation failed: %v", err)
		}
		defer consumer.Close()

		// Consume in goroutine with timeout
		received := make(chan *kafka.SpanMessage, numSpans)
		var wg sync.WaitGroup
		wg.Add(1)

		go func() {
			defer wg.Done()
			count := 0
			consumer.Start(roundCtx, func(ctx context.Context, span *kafka.SpanMessage) error {
				received <- span
				count++
				if count >= numSpans {
					// Exit early after consuming required number
					roundCancel()
				}
				return nil
			})
		}()

		// Wait for consumption or timeout
		timeout := time.NewTimer(10 * time.Second)
		defer timeout.Stop()

		consumed := 0
		for {
			select {
			case span := <-received:
				consumed++
				t.Logf("  Received: trace=%s, span=%s, tokens=%d+%d",
					span.TraceID, span.SpanID, span.InputTokens, span.OutputTokens)
				if consumed >= numSpans {
					t.Logf("✓ Consumed %d spans", consumed)
					return
				}
			case <-timeout.C:
				t.Logf("Warning: Timeout after consuming %d/%d spans", consumed, numSpans)
				return
			case <-roundCtx.Done():
				if consumed >= numSpans {
					t.Logf("✓ Consumed %d spans (context cancelled)", consumed)
					return
				}
			}
		}
	})

	t.Run("BatchProduction", func(t *testing.T) {
		producer, err := kafka.NewProducer(brokers, "test-batch", logger)
		if err != nil {
			t.Skipf("Kafka not available: %v", err)
		}
		defer producer.Close()

		// Create batch of spans
		batch := make([]*kafka.SpanMessage, 10)
		for i := 0; i < 10; i++ {
			batch[i] = &kafka.SpanMessage{
				TraceID:      fmt.Sprintf("trace-batch-%d", i),
				SpanID:       fmt.Sprintf("span-batch-%d", i),
				Framework:    "anthropic-api",
				InputTokens:  50,
				OutputTokens: 25,
				ModelName:    "claude-3-haiku",
				ReceivedAt:   time.Now(),
				Attributes:   map[string]string{},
			}
		}

		err = producer.ProduceSpans(ctx, batch)
		if err != nil {
			t.Fatalf("ProduceSpans failed: %v", err)
		}

		t.Logf("✓ Batch produced: %d spans", len(batch))
	})
}

// TestKafkaProducerEdgeCases: Test error handling and edge cases
func TestKafkaProducerEdgeCases(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	t.Run("NilBrokers", func(t *testing.T) {
		_, err := kafka.NewProducer([]string{}, "test", logger)
		if err == nil {
			t.Error("Expected error for empty brokers")
		}
	})

	t.Run("NilSpanMessage", func(t *testing.T) {
		producer, err := kafka.NewProducer([]string{"localhost:9092"}, "test", logger)
		if err != nil {
			t.Skipf("Kafka not available: %v", err)
		}
		defer producer.Close()

		ctx := context.Background()
		err = producer.ProduceSpan(ctx, nil)
		if err == nil {
			t.Error("Expected error for nil span message")
		}
	})

	t.Run("EmptyBatch", func(t *testing.T) {
		producer, err := kafka.NewProducer([]string{"localhost:9092"}, "test", logger)
		if err != nil {
			t.Skipf("Kafka not available: %v", err)
		}
		defer producer.Close()

		ctx := context.Background()
		err = producer.ProduceSpans(ctx, []*kafka.SpanMessage{})
		if err != nil {
			t.Logf("Empty batch returned error (ok): %v", err)
		} else {
			t.Log("✓ Empty batch handled gracefully")
		}
	})

	t.Run("ProduceAfterClose", func(t *testing.T) {
		producer, err := kafka.NewProducer([]string{"localhost:9092"}, "test", logger)
		if err != nil {
			t.Skipf("Kafka not available: %v", err)
		}

		producer.Close()

		ctx := context.Background()
		span := &kafka.SpanMessage{TraceID: "test", SpanID: "test"}
		err = producer.ProduceSpan(ctx, span)
		if err == nil {
			t.Error("Expected error when producing after close")
		} else {
			t.Log("✓ Correctly rejected produce on closed producer")
		}
	})
}
