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

// Hub manages WebSocket connections and broadcasts events.
type Hub struct {
	mu      sync.RWMutex
	clients map[*websocket.Conn]bool
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// NewHub creates a WebSocket hub.
func NewHub() *Hub {
	return &Hub{
		clients: make(map[*websocket.Conn]bool),
	}
}

// HandleWS upgrades an HTTP connection to WebSocket.
func (h *Hub) HandleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("websocket upgrade failed", "error", err)
		return
	}

	h.mu.Lock()
	h.clients[conn] = true
	h.mu.Unlock()

	// Read loop (discard incoming messages, detect disconnect)
	go func() {
		defer func() {
			h.mu.Lock()
			delete(h.clients, conn)
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

	h.mu.RLock()
	defer h.mu.RUnlock()

	for conn := range h.clients {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			conn.Close()
			go func(c *websocket.Conn) {
				h.mu.Lock()
				delete(h.clients, c)
				h.mu.Unlock()
			}(conn)
		}
	}
}

// ClientCount returns number of connected WebSocket clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
