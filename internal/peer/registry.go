package peer

import (
	"sync"
	"time"
)

// PeerInfo holds information about a discovered peer.
type PeerInfo struct {
	NodeID     string    `json:"node_id"`
	IP         string    `json:"ip"`
	Port       int       `json:"port"`
	Memories   int       `json:"memories"`
	LastSync   time.Time `json:"last_sync"`
	SyncCount  int       `json:"sync_count"`
	FirstSeen  time.Time `json:"first_seen"`
	Status     string    `json:"status"` // "connected", "syncing", "stale"
}

// Registry maintains the list of known peers.
type Registry struct {
	mu    sync.RWMutex
	peers map[string]*PeerInfo // nodeID → info
}

// NewRegistry creates a peer registry.
func NewRegistry() *Registry {
	return &Registry{
		peers: make(map[string]*PeerInfo),
	}
}

// Add registers or updates a peer.
func (r *Registry) Add(nodeID, ip string, port int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if p, ok := r.peers[nodeID]; ok {
		p.IP = ip
		p.Port = port
		p.Status = "connected"
	} else {
		r.peers[nodeID] = &PeerInfo{
			NodeID:    nodeID,
			IP:        ip,
			Port:      port,
			FirstSeen: time.Now(),
			Status:    "connected",
		}
	}
}

// Remove deletes a peer.
func (r *Registry) Remove(nodeID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.peers, nodeID)
}

// MarkSynced updates sync stats for a peer.
func (r *Registry) MarkSynced(nodeID string, memoriesSynced int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.peers[nodeID]; ok {
		p.LastSync = time.Now()
		p.SyncCount++
		p.Memories += memoriesSynced
		p.Status = "connected"
	}
}

// List returns all known peers.
func (r *Registry) List() []*PeerInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*PeerInfo, 0, len(r.peers))
	for _, p := range r.peers {
		result = append(result, p)
	}
	return result
}

// Count returns number of known peers.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.peers)
}

// Get returns a specific peer.
func (r *Registry) Get(nodeID string) *PeerInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.peers[nodeID]
}

// RandomPeers returns up to n random peers (for gossip fan-out).
func (r *Registry) RandomPeers(n int, exclude string) []*PeerInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*PeerInfo
	for _, p := range r.peers {
		if p.NodeID == exclude {
			continue
		}
		result = append(result, p)
		if len(result) >= n {
			break
		}
	}
	return result
}
