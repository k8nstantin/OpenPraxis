package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	automerge "github.com/automerge/automerge-go"
)

// Store is the CRDT-backed memory store using Automerge.
// It is the source of truth for all memory data, replicated across peers.
type Store struct {
	doc      *automerge.Doc
	dataDir  string
	mu       sync.RWMutex
	onChange func(ids []string) // callback when memories change (from local or remote merge)
}

// NewStore creates or loads an Automerge-backed store.
func NewStore(dataDir string) (*Store, error) {
	s := &Store{dataDir: dataDir}

	// Try to load existing document
	docPath := filepath.Join(dataDir, "memories.automerge")
	data, err := os.ReadFile(docPath)
	if err == nil && len(data) > 0 {
		doc, err := automerge.Load(data)
		if err != nil {
			return nil, fmt.Errorf("load automerge doc: %w", err)
		}
		s.doc = doc
	} else {
		s.doc = automerge.New()
	}

	return s, nil
}

// SetOnChange registers a callback fired when memories are added/updated/deleted.
// The callback receives a list of memory IDs that changed.
func (s *Store) SetOnChange(fn func(ids []string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onChange = fn
}

// Put adds or updates a memory in the CRDT document.
func (s *Store) Put(mem *Memory) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.Marshal(mem)
	if err != nil {
		return fmt.Errorf("marshal memory: %w", err)
	}

	// Store as: root map → "memories" map → memory ID → JSON string
	root := s.doc.RootMap()
	memoriesVal, err := root.Get("memories")
	if err != nil {
		return fmt.Errorf("get memories map: %w", err)
	}

	var memories *automerge.Map
	if memoriesVal.Kind() == automerge.KindVoid {
		// Initialize the memories map in the document
		newMap := automerge.NewMap()
		if err := root.Set("memories", newMap); err != nil {
			return fmt.Errorf("init memories map: %w", err)
		}
		memories = newMap
	} else {
		memories = memoriesVal.Map()
	}

	if err := memories.Set(mem.ID, string(data)); err != nil {
		return fmt.Errorf("set memory: %w", err)
	}

	_, err = s.doc.Commit(fmt.Sprintf("put %s", mem.Path))
	if err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	if err := s.persist(); err != nil {
		return fmt.Errorf("persist: %w", err)
	}

	return nil
}

// Get retrieves a memory by ID from the CRDT document.
func (s *Store) Get(id string) (*Memory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	root := s.doc.RootMap()
	memoriesVal, err := root.Get("memories")
	if err != nil || memoriesVal.Kind() == automerge.KindVoid {
		return nil, nil
	}

	val, err := memoriesVal.Map().Get(id)
	if err != nil || val.Kind() == automerge.KindVoid {
		return nil, nil
	}

	var mem Memory
	if err := json.Unmarshal([]byte(val.Str()), &mem); err != nil {
		return nil, fmt.Errorf("unmarshal memory: %w", err)
	}
	return &mem, nil
}

// Delete removes a memory by ID from the CRDT document.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	root := s.doc.RootMap()
	memoriesVal, err := root.Get("memories")
	if err != nil || memoriesVal.Kind() == automerge.KindVoid {
		return nil
	}

	if err := memoriesVal.Map().Delete(id); err != nil {
		return fmt.Errorf("delete memory: %w", err)
	}

	_, err = s.doc.Commit(fmt.Sprintf("delete %s", id))
	if err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return s.persist()
}

// All returns all memories in the store.
func (s *Store) All() ([]*Memory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	root := s.doc.RootMap()
	memoriesVal, err := root.Get("memories")
	if err != nil || memoriesVal.Kind() == automerge.KindVoid {
		return nil, nil
	}

	memories := memoriesVal.Map()
	keys, err := memories.Keys()
	if err != nil {
		return nil, fmt.Errorf("get keys: %w", err)
	}

	var result []*Memory
	for _, key := range keys {
		val, err := memories.Get(key)
		if err != nil || val.Kind() == automerge.KindVoid {
			continue
		}
		var mem Memory
		if err := json.Unmarshal([]byte(val.Str()), &mem); err != nil {
			continue
		}
		result = append(result, &mem)
	}
	return result, nil
}

// IDs returns all memory IDs in the store.
func (s *Store) IDs() ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	root := s.doc.RootMap()
	memoriesVal, err := root.Get("memories")
	if err != nil || memoriesVal.Kind() == automerge.KindVoid {
		return nil, nil
	}

	return memoriesVal.Map().Keys()
}

// --- Sync methods ---

// Save serializes the entire Automerge document.
func (s *Store) Save() []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.doc.Save()
}

// SaveIncremental returns only the changes since the last SaveIncremental call.
func (s *Store) SaveIncremental() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.doc.SaveIncremental()
}

// NewSyncState creates a new sync state for a peer connection.
func (s *Store) NewSyncState() *automerge.SyncState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return automerge.NewSyncState(s.doc)
}

// GenerateSyncMessage generates the next sync message for a peer.
func (s *Store) GenerateSyncMessage(ss *automerge.SyncState) ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	msg, valid := ss.GenerateMessage()
	if !valid {
		return nil, false
	}
	return msg.Bytes(), true
}

// ReceiveSyncMessage applies an incoming sync message from a peer.
// Returns the list of memory IDs that changed.
func (s *Store) ReceiveSyncMessage(ss *automerge.SyncState, msgBytes []byte) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Get IDs before merge
	beforeIDs := s.idsLocked()

	_, err := ss.ReceiveMessage(msgBytes)
	if err != nil {
		return nil, fmt.Errorf("receive sync message: %w", err)
	}

	// Get IDs after merge
	afterIDs := s.idsLocked()

	// Find changed IDs (new or updated)
	changed := diffIDs(beforeIDs, afterIDs)

	if len(changed) > 0 {
		if err := s.persist(); err != nil {
			return changed, fmt.Errorf("persist after sync: %w", err)
		}
		if s.onChange != nil {
			go s.onChange(changed)
		}
	}

	return changed, nil
}

// Merge merges another document into this one (for full state sync).
func (s *Store) Merge(otherBytes []byte) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	other, err := automerge.Load(otherBytes)
	if err != nil {
		return nil, fmt.Errorf("load remote doc: %w", err)
	}

	beforeIDs := s.idsLocked()

	if _, err := s.doc.Merge(other); err != nil {
		return nil, fmt.Errorf("merge: %w", err)
	}

	afterIDs := s.idsLocked()
	changed := diffIDs(beforeIDs, afterIDs)

	if len(changed) > 0 {
		if err := s.persist(); err != nil {
			return changed, err
		}
		if s.onChange != nil {
			go s.onChange(changed)
		}
	}

	return changed, nil
}

// persist saves the document to disk.
func (s *Store) persist() error {
	docPath := filepath.Join(s.dataDir, "memories.automerge")
	return os.WriteFile(docPath, s.doc.Save(), 0644)
}

// idsLocked returns all memory IDs (caller must hold lock).
func (s *Store) idsLocked() map[string]string {
	result := make(map[string]string)
	root := s.doc.RootMap()
	memoriesVal, err := root.Get("memories")
	if err != nil || memoriesVal.Kind() == automerge.KindVoid {
		return result
	}

	keys, err := memoriesVal.Map().Keys()
	if err != nil {
		return result
	}

	for _, key := range keys {
		val, _ := memoriesVal.Map().Get(key)
		if val.Kind() != automerge.KindVoid {
			result[key] = val.Str()
		}
	}
	return result
}

// diffIDs finds IDs that are new or changed between before and after snapshots.
func diffIDs(before, after map[string]string) []string {
	var changed []string
	for id, val := range after {
		if oldVal, exists := before[id]; !exists || oldVal != val {
			changed = append(changed, id)
		}
	}
	return changed
}
