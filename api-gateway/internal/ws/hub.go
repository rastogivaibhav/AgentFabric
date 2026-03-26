package ws

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

var strictWSOrigins = strictConfigEnabled()

// allowedWSOrigins is built once at startup from AF_CORS_ORIGINS.
// Dev mode keeps localhost defaults for operator ergonomics; strict production
// mode requires explicit origins and rejects empty Origin headers.
var allowedWSOrigins = buildAllowedWSOrigins(os.Getenv("AF_CORS_ORIGINS"), strictWSOrigins)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		return isAllowedWSOrigin(r.Header.Get("Origin"), allowedWSOrigins, strictWSOrigins)
	},
}

func strictConfigEnabled() bool {
	if os.Getenv("AF_STRICT_CONFIG") == "true" {
		return true
	}
	return strings.EqualFold(os.Getenv("AF_ENV"), "production")
}

func buildAllowedWSOrigins(raw string, strict bool) map[string]bool {
	m := make(map[string]bool)
	if !strict {
		m["http://localhost:3000"] = true
		m["http://localhost:5173"] = true
	}
	for _, origin := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(origin); trimmed != "" {
			m[trimmed] = true
		}
	}
	return m
}

func isAllowedWSOrigin(origin string, allowed map[string]bool, strict bool) bool {
	trimmed := strings.TrimSpace(origin)
	if trimmed == "" {
		return !strict
	}
	return allowed[trimmed]
}

// Client is a single WebSocket connection.
type Client struct {
	id       string
	conn     *websocket.Conn
	send     chan []byte
	tenantID string
	hub      *Hub
}

// Hub manages all WebSocket clients and fan-out inside a single process.
// It does not coordinate across api-gateway replicas.
type Hub struct {
	mu         sync.RWMutex
	clients    map[string]map[*Client]struct{} // tenantID -> clients
	logger     *zap.Logger
	register   chan *Client
	unregister chan *Client
	broadcast  chan *broadcastMsg
}

type broadcastMsg struct {
	tenantID string
	data     []byte
}

func NewHub(logger *zap.Logger) *Hub {
	return &Hub{
		clients:    make(map[string]map[*Client]struct{}),
		logger:     logger,
		register:   make(chan *Client, 64),
		unregister: make(chan *Client, 64),
		broadcast:  make(chan *broadcastMsg, 4096),
	}
}

func (h *Hub) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case c := <-h.register:
			h.mu.Lock()
			if h.clients[c.tenantID] == nil {
				h.clients[c.tenantID] = make(map[*Client]struct{})
			}
			h.clients[c.tenantID][c] = struct{}{}
			h.mu.Unlock()
			h.logger.Debug("ws client connected", zap.String("tenant", c.tenantID))

		case c := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[c.tenantID][c]; ok {
				delete(h.clients[c.tenantID], c)
				close(c.send)
			}
			h.mu.Unlock()

		case msg := <-h.broadcast:
			h.mu.RLock()
			tenantClients := h.clients[msg.tenantID]
			h.mu.RUnlock()
			for c := range tenantClients {
				select {
				case c.send <- msg.data:
				default:
					// Client too slow — disconnect
					h.unregister <- c
				}
			}
		}
	}
}

func (h *Hub) Broadcast(tenantID string, event interface{}) {
	b, err := json.Marshal(event)
	if err != nil {
		return
	}
	select {
	case h.broadcast <- &broadcastMsg{tenantID: tenantID, data: b}:
	default:
		// Drop if broadcast queue full
	}
}

func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request, tenantID string) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Warn("ws upgrade failed", zap.Error(err))
		return
	}

	client := &Client{
		conn:     conn,
		send:     make(chan []byte, 512),
		tenantID: tenantID,
		hub:      h,
	}

	h.register <- client

	go client.writePump()
	go client.readPump()
}

func (c *Client) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
	}()
	c.conn.SetReadLimit(512)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
	}
}
