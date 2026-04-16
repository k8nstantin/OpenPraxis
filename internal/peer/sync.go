package peer

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"openpraxis/internal/memory"

	automerge "github.com/automerge/automerge-go"
)

// SyncServer handles peer-to-peer sync over HTTP.
type SyncServer struct {
	store      *memory.Store
	registry   *Registry
	nodeID     string
	fanOut     int
	syncStates map[string]*automerge.SyncState // peerID → sync state
	client     *http.Client
}

// NewSyncServer creates a sync server.
func NewSyncServer(store *memory.Store, registry *Registry, nodeID string, fanOut int) *SyncServer {
	return &SyncServer{
		store:      store,
		registry:   registry,
		nodeID:     nodeID,
		fanOut:     fanOut,
		syncStates: make(map[string]*automerge.SyncState),
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Handler returns an HTTP handler for the sync endpoints.
func (ss *SyncServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/sync/state", ss.handleState)
	mux.HandleFunc("/sync/update", ss.handleUpdate)
	mux.HandleFunc("/sync/full", ss.handleFull)
	mux.HandleFunc("/sync/peers", ss.handlePeers)
	return mux
}

// handleState returns the Automerge sync state vector.
func (ss *SyncServer) handleState(w http.ResponseWriter, r *http.Request) {
	data := ss.store.Save()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Node-ID", ss.nodeID)
	w.Write(data)
}

// handleUpdate receives and applies a sync message, returns our update.
func (ss *SyncServer) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST required", 405)
		return
	}

	peerID := r.Header.Get("X-Node-ID")
	if peerID == "" {
		peerID = r.RemoteAddr
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body: "+err.Error(), 400)
		return
	}

	// Get or create sync state for this peer
	syncState, ok := ss.syncStates[peerID]
	if !ok {
		syncState = ss.store.NewSyncState()
		ss.syncStates[peerID] = syncState
	}

	// Apply incoming message
	changed, err := ss.store.ReceiveSyncMessage(syncState, body)
	if err != nil {
		http.Error(w, "sync: "+err.Error(), 500)
		return
	}

	if len(changed) > 0 {
		slog.Info("sync received changes", "count", len(changed), "peer_id", peerID)
		ss.registry.MarkSynced(peerID, len(changed))
	}

	// Generate our response message
	respMsg, valid := ss.store.GenerateSyncMessage(syncState)
	if valid {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("X-Node-ID", ss.nodeID)
		w.Write(respMsg)
	} else {
		w.WriteHeader(204) // no content needed
	}
}

// handleFull returns the full document (for new peers bootstrapping).
func (ss *SyncServer) handleFull(w http.ResponseWriter, r *http.Request) {
	data := ss.store.Save()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Node-ID", ss.nodeID)
	w.Write(data)
}

// handlePeers returns the known peer list (for peer exchange).
func (ss *SyncServer) handlePeers(w http.ResponseWriter, r *http.Request) {
	peers := ss.registry.List()
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, "[")
	for i, p := range peers {
		if i > 0 {
			fmt.Fprintf(w, ",")
		}
		fmt.Fprintf(w, `{"node_id":%q,"ip":%q,"port":%d}`, p.NodeID, p.IP, p.Port)
	}
	fmt.Fprintf(w, "]")
}

// PushToPeers sends CRDT updates to random peers (gossip fan-out).
func (ss *SyncServer) PushToPeers(ctx context.Context) {
	peers := ss.registry.RandomPeers(ss.fanOut, ss.nodeID)
	for _, p := range peers {
		go ss.syncWithPeer(ctx, p)
	}
}

// syncWithPeer performs a sync exchange with a single peer.
func (ss *SyncServer) syncWithPeer(ctx context.Context, p *PeerInfo) {
	syncState, ok := ss.syncStates[p.NodeID]
	if !ok {
		syncState = ss.store.NewSyncState()
		ss.syncStates[p.NodeID] = syncState
	}

	// Generate our sync message
	msg, valid := ss.store.GenerateSyncMessage(syncState)
	if !valid {
		return // nothing to send
	}

	// Send to peer
	url := fmt.Sprintf("http://%s:%d/sync/update", p.IP, p.Port)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(msg))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Node-ID", ss.nodeID)

	resp, err := ss.client.Do(req)
	if err != nil {
		slog.Error("sync push failed", "peer_id", p.NodeID, "error", err)
		return
	}
	defer resp.Body.Close()

	// Apply response
	if resp.StatusCode == 200 {
		respBody, _ := io.ReadAll(resp.Body)
		if len(respBody) > 0 {
			changed, _ := ss.store.ReceiveSyncMessage(syncState, respBody)
			if len(changed) > 0 {
				ss.registry.MarkSynced(p.NodeID, len(changed))
			}
		}
	}
}

// SyncWithPeerByID initiates sync with a specific peer by node ID.
func (ss *SyncServer) SyncWithPeerByID(ctx context.Context, nodeID string) {
	p := ss.registry.Get(nodeID)
	if p != nil {
		ss.syncWithPeer(ctx, p)
	}
}

// StartAntiEntropy runs periodic full sync with a random peer.
func (ss *SyncServer) StartAntiEntropy(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			peers := ss.registry.RandomPeers(1, ss.nodeID)
			if len(peers) > 0 {
				ss.syncWithPeer(ctx, peers[0])
			}
		}
	}
}
