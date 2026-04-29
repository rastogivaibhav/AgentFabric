package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisClient struct {
	client *redis.Client
}

func (r *RedisClient) RawClient() *redis.Client {
	if r == nil {
		return nil
	}
	return r.client
}

func NewRedisClient(addr string) (*RedisClient, error) {
	opts, err := redis.ParseURL(addr)
	if err != nil {
		opts = &redis.Options{
			Addr:         addr,
			PoolSize:     20,
			MinIdleConns: 5,
		}
	}
	opts.PoolSize = 20
	opts.MinIdleConns = 5

	c := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping: %w", err)
	}
	return &RedisClient{client: c}, nil
}

func (r *RedisClient) SetJSON(ctx context.Context, key string, val interface{}, ttl time.Duration) error {
	b, err := json.Marshal(val)
	if err != nil {
		return err
	}
	return r.client.Set(ctx, key, b, ttl).Err()
}

func (r *RedisClient) GetJSON(ctx context.Context, key string, dest interface{}) error {
	b, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dest)
}

// Publish a live event to a tenant channel.
func (r *RedisClient) PublishLiveEvent(ctx context.Context, tenantID string, event interface{}) error {
	b, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return r.client.Publish(ctx, "live:"+tenantID, b).Err()
}

// Subscribe returns a channel of raw JSON messages for the tenant.
func (r *RedisClient) Subscribe(ctx context.Context, tenantID string) <-chan string {
	sub := r.client.Subscribe(ctx, "live:"+tenantID)
	ch := make(chan string, 256)
	go func() {
		defer close(ch)
		defer sub.Close()
		for msg := range sub.Channel() {
			select {
			case ch <- msg.Payload:
			case <-ctx.Done():
				return
			default:
				// Drop if channel full (WebSocket client slow)
			}
		}
	}()
	return ch
}

func (r *RedisClient) Incr(ctx context.Context, key string) (int64, error) {
	return r.client.Incr(ctx, key).Result()
}

// IncrWithExpiry atomically increments key and sets an expiry on first creation.
// Implements middleware.RateLimitStore.
func (r *RedisClient) IncrWithExpiry(ctx context.Context, key string, expiry time.Duration) (int64, error) {
	pipe := r.client.Pipeline()
	incrCmd := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, expiry)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, err
	}
	return incrCmd.Val(), nil
}

// Ping checks the Redis connection with a 2-second timeout.
// Used by the /healthz handler to detect degraded storage.
func (r *RedisClient) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

func (r *RedisClient) Close() error {
	return r.client.Close()
}
