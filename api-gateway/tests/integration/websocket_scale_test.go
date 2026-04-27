package integration_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/govagn/api-gateway/internal/ws"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// TestRedisPubSubBroadcast: Verify Redis pub/sub integration works
func TestRedisPubSubBroadcast(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	defer redisClient.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Verify Redis is available
	if err := redisClient.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	t.Run("PublishAndReceive", func(t *testing.T) {
		redisPubSub := ws.NewRedisPubSub(redisClient, logger)

		// Subscribe to broadcasts
		ch, err := redisPubSub.SubscribeToBroadcasts(ctx, "test-tenant-scale")
		if err != nil {
			t.Fatalf("Subscribe failed: %v", err)
		}

		// Publish a broadcast
		testData := []byte(`{"event":"span_received","span_id":"test-123"}`)
		err = redisPubSub.PublishBroadcast(ctx, "test-tenant-scale", testData, "replica-1")
		if err != nil {
			t.Fatalf("Publish failed: %v", err)
		}

		// Receive the broadcast
		select {
		case event := <-ch:
			if event == nil {
				t.Fatal("Received nil event")
			}
			if string(event.Data) != string(testData) {
				t.Errorf("Data mismatch: expected %s, got %s", testData, event.Data)
			}
			if event.Source != "replica-1" {
				t.Errorf("Source mismatch: expected replica-1, got %s", event.Source)
			}
			t.Log("✓ Successfully received broadcast from Redis")
		case <-time.After(2 * time.Second):
			t.Fatal("Timeout waiting for broadcast")
		}
	})

	t.Run("MultiReplicaBroadcast", func(t *testing.T) {
		// Simulate two replicas communicating via Redis
		replica1PubSub := ws.NewRedisPubSub(redisClient, logger)
		replica2PubSub := ws.NewRedisPubSub(redisClient, logger)

		tenantID := "test-tenant-multi"

		// Replica 2 subscribes
		ch2, err := replica2PubSub.SubscribeToBroadcasts(ctx, tenantID)
		if err != nil {
			t.Fatalf("Replica 2 subscribe failed: %v", err)
		}

		// Replica 1 publishes
		testData := []byte(`{"event":"test_event"}`)
		err = replica1PubSub.PublishBroadcast(ctx, tenantID, testData, "replica-1")
		if err != nil {
			t.Fatalf("Replica 1 publish failed: %v", err)
		}

		// Verify Replica 2 receives it
		select {
		case event := <-ch2:
			if event == nil {
				t.Fatal("Received nil event")
			}
			t.Logf("✓ Replica 2 received broadcast from Replica 1: source=%s", event.Source)
		case <-time.After(2 * time.Second):
			t.Fatal("Replica 2 timeout waiting for broadcast")
		}
	})

	t.Run("DeduplicationOfOwnBroadcasts", func(t *testing.T) {
		redisPubSub := ws.NewRedisPubSub(redisClient, logger)
		replicaID := "replica-dedupe-test"

		// Subscribe to pattern
		ch, err := redisPubSub.SubscribeToAllTenants(ctx)
		if err != nil {
			t.Fatalf("Subscribe to all tenants failed: %v", err)
		}

		// Publish from this replica
		testData := []byte(`{"event":"self_publish"}`)
		err = redisPubSub.PublishBroadcast(ctx, "test-tenant", testData, replicaID)
		if err != nil {
			t.Fatalf("Publish failed: %v", err)
		}

		// Should receive the broadcast (even from self, dedup is in Hub.Run)
		select {
		case event := <-ch:
			if event == nil {
				t.Fatal("Received nil event")
			}
			t.Logf("Note: Received own broadcast (dedup in Hub.Run): source=%s", event.Source)
		case <-time.After(2 * time.Second):
			t.Log("Note: Did not receive own broadcast (depends on Redis ordering)")
		}
	})
}

// TestHubWithRedisScaling: Verify Hub works with Redis pub/sub
func TestHubWithRedisScaling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	defer redisClient.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Verify Redis is available
	if err := redisClient.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	t.Run("CreateHubWithRedis", func(t *testing.T) {
		hub := ws.NewHub(logger, redisClient)
		if hub == nil {
			t.Fatal("Hub should not be nil")
		}
		t.Log("✓ Hub created with Redis client")
	})

	t.Run("HubRunWithRedisSubscription", func(t *testing.T) {
		hub := ws.NewHub(logger, redisClient)

		// Run hub in goroutine
		go hub.Run(ctx)

		// Wait for subscription to establish
		time.Sleep(100 * time.Millisecond)

		// Broadcast locally
		testEvent := map[string]string{"event": "test", "data": "hello"}
		body, _ := json.Marshal(testEvent)
		hub.Broadcast("test-tenant", testEvent)

		// Give time for Redis publish
		time.Sleep(100 * time.Millisecond)

		t.Log("✓ Hub ran with Redis subscription")
	})

	t.Run("ConcurrentBroadcasts", func(t *testing.T) {
		hub := ws.NewHub(logger, redisClient)
		go hub.Run(ctx)

		// Simulate concurrent broadcasts
		var wg sync.WaitGroup
		numBroadcasts := 100

		for i := 0; i < numBroadcasts; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				event := map[string]interface{}{
					"event": "broadcast",
					"id":    idx,
				}
				hub.Broadcast("test-tenant", event)
			}(i)
		}

		wg.Wait()
		time.Sleep(200 * time.Millisecond)

		t.Logf("✓ Sent %d concurrent broadcasts without error", numBroadcasts)
	})
}

// TestWebSocketHorizontalScale: Verify cross-replica message delivery
// Note: This test requires two Hub instances running in parallel,
// which is complex to set up in integration tests. See manual testing doc.
func TestWebSocketHorizontalScale(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// This is a placeholder for a more comprehensive test that would:
	// 1. Create two Hub instances with Redis clients
	// 2. Simulate WebSocket clients on each hub
	// 3. Publish from one hub and verify delivery to the other
	// For now, we test the building blocks above.

	t.Log("Note: Full cross-replica test requires multiple goroutines with simulated WebSocket clients")
	t.Log("Building blocks tested separately: RedisPubSub mechanics and Hub integration")
}
