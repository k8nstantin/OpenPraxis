package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

// Event represents a WebSocket event pushed to the dashboard.
type Event struct {
	Type string `json:"event"`
	Data any    `json:"data"`
}

// wsClient wraps a connection with a per-connection write mutex.
//
// gorilla/websocket explicitly forbids concurrent calls to WriteMessage /
// WriteJSON / NextWriter on the same connection — it panics when it
// detects them. Hub.Broadcast is invoked from many goroutines (scheduler
// fires, peer events, runner stream events) so writes must be serialized
// per connection. Mutex is per-client (not Hub-wide) so a slow reader on
// one socket can't block broadcasts to the others.
type wsClient struct {
	conn  *websocket.Conn
	write sync.Mutex
}

// Hub manages WebSocket connections and broadcasts events.
type Hub struct {
	mu      sync.RWMutex
	clients map[*wsClient]bool
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// NewHub creates a WebSocket hub.
func NewHub() *Hub {
	return &Hub{
		clients: make(map[*wsClient]bool),
	}
}

// HandleWS upgrades an HTTP connection to WebSocket.
func (h *Hub) HandleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("websocket upgrade failed", "error", err)
		return
	}

	c := &wsClient{conn: conn}
	h.mu.Lock()
	h.clients[c] = true
	h.mu.Unlock()

	// Read loop (discard incoming messages, detect disconnect)
	go func() {
		defer func() {
			h.mu.Lock()
			delete(h.clients, c)
			h.mu.Unlock()
			conn.Close()
		}()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}()
}

// Broadcast sends an event to all connected WebSocket clients.
func (h *Hub) Broadcast(event Event) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	// Snapshot the client set under the read lock, then release it
	// before any I/O. Keeps registration + disconnect paths from
	// waiting on a slow socket.
	h.mu.RLock()
	clients := make([]*wsClient, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c)
	}
	h.mu.RUnlock()

	for _, c := range clients {
		c.write.Lock()
		err := c.conn.WriteMessage(websocket.TextMessage, data)
		c.write.Unlock()
		if err != nil {
			c.conn.Close()
			h.mu.Lock()
			delete(h.clients, c)
			h.mu.Unlock()
		}
	}
}

// ClientCount returns number of connected WebSocket clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
