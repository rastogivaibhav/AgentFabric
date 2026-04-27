package ws

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// RedisPubSub handles cross-replica broadcast via Redis pub/sub
type RedisPubSub struct {
	client *redis.Client
	logger *zap.Logger
}

// BroadcastEvent is the message format for cross-replica broadcasts
type BroadcastEvent struct {
	TenantID string `json:"tenant_id"`
	Data     []byte `json:"data"`
	Source   string `json:"source,omitempty"` // Replica ID for deduplication
}

// NewRedisPubSub creates a new Redis pub/sub broker for WebSocket broadcasts
func NewRedisPubSub(client *redis.Client, logger *zap.Logger) *RedisPubSub {
	return &RedisPubSub{
		client: client,
		logger: logger,
	}
}

// PublishBroadcast sends a broadcast to Redis for other replicas
func (rp *RedisPubSub) PublishBroadcast(ctx context.Context, tenantID string, data []byte, source string) error {
	if rp.client == nil {
		return fmt.Errorf("redis client not configured")
	}

	event := BroadcastEvent{
		TenantID: tenantID,
		Data:     data,
		Source:   source,
	}

	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal broadcast: %w", err)
	}

	// Channel name per tenant (live:<tenant_id>)
	channel := fmt.Sprintf("live:%s", tenantID)

	err = rp.client.Publish(ctx, channel, body).Err()
	if err != nil {
		rp.logger.Warn(
			"failed to publish broadcast to redis",
			zap.Error(err),
			zap.String("channel", channel),
		)
		return fmt.Errorf("publish: %w", err)
	}

	rp.logger.Debug(
		"published broadcast to redis",
		zap.String("channel", channel),
		zap.Int("data_len", len(data)),
	)

	return nil
}

// SubscribeToBroadcasts subscribes to Redis broadcasts for a tenant and returns a channel
func (rp *RedisPubSub) SubscribeToBroadcasts(ctx context.Context, tenantID string) (<-chan *BroadcastEvent, error) {
	if rp.client == nil {
		return nil, fmt.Errorf("redis client not configured")
	}

	channel := fmt.Sprintf("live:%s", tenantID)
	pubsub := rp.client.Subscribe(ctx, channel)

	outCh := make(chan *BroadcastEvent, 100)

	go func() {
		defer close(outCh)
		for {
			msg, err := pubsub.ReceiveMessage(ctx)
			if err != nil {
				if err != context.Canceled {
					rp.logger.Warn(
						"redis subscription error",
						zap.Error(err),
						zap.String("channel", channel),
					)
				}
				return
			}

			var event BroadcastEvent
			if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
				rp.logger.Warn(
					"failed to unmarshal broadcast event",
					zap.Error(err),
					zap.String("payload", msg.Payload[:min(100, len(msg.Payload))]),
				)
				continue
			}

			select {
			case outCh <- &event:
			case <-ctx.Done():
				return
			}
		}
	}()

	return outCh, nil
}

// SubscribeToAllTenants subscribes to pattern "live:*" for all tenant broadcasts
func (rp *RedisPubSub) SubscribeToAllTenants(ctx context.Context) (<-chan *BroadcastEvent, error) {
	if rp.client == nil {
		return nil, fmt.Errorf("redis client not configured")
	}

	pubsub := rp.client.PSubscribe(ctx, "live:*")
	outCh := make(chan *BroadcastEvent, 100)

	go func() {
		defer close(outCh)
		for {
			msg, err := pubsub.ReceiveMessage(ctx)
			if err != nil {
				if err != context.Canceled {
					rp.logger.Warn(
						"redis pattern subscription error",
						zap.Error(err),
					)
				}
				return
			}

			var event BroadcastEvent
			if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
				rp.logger.Warn(
					"failed to unmarshal pattern broadcast",
					zap.Error(err),
				)
				continue
			}

			select {
			case outCh <- &event:
			case <-ctx.Done():
				return
			}
		}
	}()

	return outCh, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
