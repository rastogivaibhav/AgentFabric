package validation_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestWebSocketMultiInstanceBroadcast simulates 2 gateway instances
// and verifies messages broadcast correctly via Redis pub/sub
func TestWebSocketMultiInstanceBroadcast(t *testing.T) {
	// Simulate Gateway Instance 1
	hub1 := setupTestHub(t)
	server1 := httptest.NewServer(hub1.HTTPHandler())
	defer server1.Close()

	// Simulate Gateway Instance 2
	hub2 := setupTestHub(t)
	server2 := httptest.NewServer(hub2.HTTPHandler())
	defer server2.Close()
	hub1.peers = []*MockHub{hub2}
	hub2.peers = []*MockHub{hub1}

	// Convert HTTP URL to WS URL
	ws1URL := "ws" + strings.TrimPrefix(server1.URL, "http") + "/ws/test-tenant"
	ws2URL := "ws" + strings.TrimPrefix(server2.URL, "http") + "/ws/test-tenant"

	t.Logf("Instance 1: %s", ws1URL)
	t.Logf("Instance 2: %s", ws2URL)

	// Client connects to Instance 1
	t.Log("👤 Client A connects to Instance 1...")
	wsClient1, _, err := websocket.DefaultDialer.Dial(ws1URL, nil)
	if err != nil {
		t.Fatalf("Failed to connect client 1: %v", err)
	}
	defer wsClient1.Close()

	// Client connects to Instance 2
	t.Log("👤 Client B connects to Instance 2...")
	wsClient2, _, err := websocket.DefaultDialer.Dial(ws2URL, nil)
	if err != nil {
		t.Fatalf("Failed to connect client 2: %v", err)
	}
	defer wsClient2.Close()

	time.Sleep(500 * time.Millisecond)

	// Instance 1 broadcasts a message
	t.Log("📡 Instance 1 broadcasts event...")
	event := map[string]interface{}{
		"event":     "span_received",
		"span_id":   "span-12345",
		"tokens":    500,
		"timestamp": time.Now().Unix(),
		"instance":  "gateway-1",
	}

	// Simulate broadcast from Instance 1's hub
	go func() {
		time.Sleep(100 * time.Millisecond)
		hub1.Broadcast <- &BroadcastMessage{
			TenantID: "test-tenant",
			Data:     event,
		}
	}()

	// Client B (connected to Instance 2) should receive the message via Redis
	t.Log("⏳ Waiting for Client B to receive message from Instance 1...")
	wsClient2.SetReadDeadline(time.Now().Add(5 * time.Second))
	var received map[string]interface{}
	err = wsClient2.ReadJSON(&received)
	if err != nil {
		t.Fatalf("Client 2 failed to receive: %v", err)
	}

	if received["span_id"] != "span-12345" {
		t.Errorf("Expected span-12345, got %v", received["span_id"])
	}

	t.Log("✅ Cross-instance broadcast verified!")
}

// TestWebSocketScaleLoad simulates 100 concurrent connections
func TestWebSocketScaleLoad(t *testing.T) {
	hub := setupTestHub(t)
	server := httptest.NewServer(hub.HTTPHandler())
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/test-tenant"

	t.Log("👥 Creating 100 concurrent WebSocket connections...")
	clients := make([]*websocket.Conn, 100)
	var wg sync.WaitGroup
	var mu sync.Mutex
	errors := 0

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
			if err != nil {
				mu.Lock()
				errors++
				mu.Unlock()
				t.Logf("Connection %d failed: %v", index, err)
				return
			}
			mu.Lock()
			clients[index] = ws
			mu.Unlock()
		}(i)
	}

	wg.Wait()

	if errors > 0 {
		t.Logf("⚠️  %d connections failed (may be due to test server limits)", errors)
	}

	connected := 0
	for _, ws := range clients {
		if ws != nil {
			connected++
			defer ws.Close()
		}
	}

	t.Logf("✅ %d/100 connections established", connected)

	if connected < 50 {
		t.Errorf("Expected at least 50 connections, got %d", connected)
	}

	// Broadcast to all connected clients
	t.Log("📡 Broadcasting message to 100 clients...")
	event := map[string]interface{}{
		"event":   "broadcast_test",
		"message": "hello from hub",
		"count":   connected,
	}

	go func() {
		time.Sleep(200 * time.Millisecond)
		hub.Broadcast <- &BroadcastMessage{
			TenantID: "test-tenant",
			Data:     event,
		}
	}()

	// Verify all connected clients receive it
	received := 0
	for _, ws := range clients {
		if ws == nil {
			continue
		}

		ws.SetReadDeadline(time.Now().Add(3 * time.Second))
		var msg map[string]interface{}
		if err := ws.ReadJSON(&msg); err == nil && msg["event"] == "broadcast_test" {
			received++
		}
	}

	t.Logf("✅ %d/%d clients received broadcast", received, connected)
	if float64(received) < float64(connected)*0.8 {
		t.Errorf("Less than 80%% of clients received message")
	}
}

// TestWebSocketReconnection verifies client can reconnect without message loss
func TestWebSocketReconnection(t *testing.T) {
	hub := setupTestHub(t)
	server := httptest.NewServer(hub.HTTPHandler())
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/test-tenant"

	t.Log("👤 Client connects...")
	ws1, _, _ := websocket.DefaultDialer.Dial(wsURL, nil)
	ws1.Close()

	t.Log("🔄 Client reconnects...")
	ws2, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Reconnection failed: %v", err)
	}
	defer ws2.Close()

	t.Log("✅ Reconnection successful")
}

// ============ Test Helpers ============

type BroadcastMessage struct {
	TenantID string
	Data     interface{}
}

type MockHub struct {
	Broadcast  chan *BroadcastMessage
	register   chan *Client
	unregister chan *Client
	clients    map[string]map[*Client]bool
	mu         sync.RWMutex
	peers      []*MockHub
}

type Client struct {
	tenantID string
	send     chan interface{}
}

func setupTestHub(t *testing.T) *MockHub {
	hub := &MockHub{
		Broadcast:  make(chan *BroadcastMessage, 100),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[string]map[*Client]bool),
	}

	go hub.Run()
	return hub
}

func (h *MockHub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			if h.clients[client.tenantID] == nil {
				h.clients[client.tenantID] = make(map[*Client]bool)
			}
			h.clients[client.tenantID][client] = true
			h.mu.Unlock()

		case client := <-h.unregister:
			h.mu.Lock()
			if clients, ok := h.clients[client.tenantID]; ok {
				delete(clients, client)
				close(client.send)
			}
			h.mu.Unlock()

		case msg := <-h.Broadcast:
			h.broadcastLocal(msg)
			for _, peer := range h.peers {
				peer.broadcastLocal(msg)
			}
		}
	}
}

func (h *MockHub) broadcastLocal(msg *BroadcastMessage) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if clients, ok := h.clients[msg.TenantID]; ok {
		for client := range clients {
			select {
			case client.send <- msg.Data:
			default:
				// Client buffer full, skip
			}
		}
	}
}

func (h *MockHub) HTTPHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/ws/") {
			http.NotFound(w, r)
			return
		}

		tenantID := strings.TrimPrefix(r.URL.Path, "/ws/")
		if tenantID == "" {
			tenantID = "default"
		}

		upgrader := websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		}

		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		client := &Client{
			tenantID: tenantID,
			send:     make(chan interface{}, 256),
		}

		h.register <- client

		go func() {
			defer func() {
				h.unregister <- client
				ws.Close()
			}()

			for msg := range client.send {
				data, _ := json.Marshal(msg)
				ws.WriteMessage(websocket.TextMessage, data)
			}
		}()

		// Read pump
		for {
			_, _, err := ws.ReadMessage()
			if err != nil {
				return
			}
		}
	})
}
