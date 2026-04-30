package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestHubBroadcast_ConcurrentNoPanic — Reliability Recovery P5 regression.
//
// Before PR #289 the Hub had no per-connection write mutex. When the
// scheduler fired N tasks on the same tick, gorilla/websocket detected
// concurrent WriteMessage calls and panicked, taking the whole serve
// process down. This test fires 50 goroutines that each call Broadcast
// against three connected clients and asserts (a) no panic and (b)
// every client receives 50 frames.
func TestHubBroadcast_ConcurrentNoPanic(t *testing.T) {
	hub := NewHub()
	srv := httptest.NewServer(http.HandlerFunc(hub.HandleWS))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")

	const clients = 3
	const broadcasts = 50

	conns := make([]*websocket.Conn, 0, clients)
	for i := 0; i < clients; i++ {
		c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("dial client %d: %v", i, err)
		}
		conns = append(conns, c)
	}
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()

	// Wait for all clients to register on the hub side.
	deadline := time.Now().Add(2 * time.Second)
	for hub.ClientCount() < clients && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := hub.ClientCount(); got != clients {
		t.Fatalf("hub ClientCount = %d, want %d", got, clients)
	}

	// Drain receivers in parallel so writes don't block on full
	// per-connection buffers.
	type result struct {
		idx   int
		count int
		err   error
	}
	results := make(chan result, clients)
	for i, c := range conns {
		i, c := i, c
		go func() {
			c.SetReadDeadline(time.Now().Add(10 * time.Second))
			n := 0
			for n < broadcasts {
				if _, _, err := c.ReadMessage(); err != nil {
					results <- result{idx: i, count: n, err: err}
					return
				}
				n++
			}
			results <- result{idx: i, count: n}
		}()
	}

	// Fire broadcasts concurrently.
	var wg sync.WaitGroup
	wg.Add(broadcasts)
	for i := 0; i < broadcasts; i++ {
		i := i
		go func() {
			defer wg.Done()
			hub.Broadcast(Event{Type: "test", Data: map[string]int{"seq": i}})
		}()
	}
	wg.Wait()

	for i := 0; i < clients; i++ {
		r := <-results
		if r.err != nil {
			t.Fatalf("client %d read error after %d frames: %v", r.idx, r.count, r.err)
		}
		if r.count != broadcasts {
			t.Fatalf("client %d got %d frames, want %d", r.idx, r.count, broadcasts)
		}
	}
}
