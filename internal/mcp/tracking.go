package mcp

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"openloom/internal/conversation"

	"github.com/google/uuid"
)

// SessionInfo tracks a connected MCP session with a stable UUID.
type SessionInfo struct {
	UUID           string     `json:"uuid"`
	Agent          string     `json:"agent"`
	MCPSession     string     `json:"mcp_session"`
	Node           string     `json:"node"`
	ConnectedAt    time.Time  `json:"connected_at"`
	DisconnectedAt *time.Time `json:"disconnected_at,omitempty"`
	LastSeenAt     time.Time  `json:"last_seen_at"`
	ToolCalls      int        `json:"tool_calls"`
	ConversationID string     `json:"conversation_id"`
	turns          []conversation.Turn
}

// AgentTracker tracks connected MCP sessions and their tool call history.
// Active sessions are cached in-memory; all sessions are persisted to SQLite.
type AgentTracker struct {
	mu       sync.RWMutex
	sessions map[string]*SessionInfo // mcp session ID → info (active sessions only)
	nodeName string
	store    *SessionStore // SQLite backing store
}

// NewAgentTracker creates a new tracker with SQLite persistence.
func NewAgentTracker(nodeName string, store *SessionStore) *AgentTracker {
	return &AgentTracker{
		sessions: make(map[string]*SessionInfo),
		nodeName: nodeName,
		store:    store,
	}
}

// Connect registers a new session with a unique UUID and persists it.
func (t *AgentTracker) Connect(mcpSessionID, agentName string) *SessionInfo {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	info := &SessionInfo{
		UUID:           uuid.Must(uuid.NewV7()).String(),
		Agent:          agentName,
		MCPSession:     mcpSessionID,
		Node:           t.nodeName,
		ConnectedAt:    now,
		LastSeenAt:     now,
		ConversationID: uuid.Must(uuid.NewV7()).String(),
	}
	t.sessions[mcpSessionID] = info

	// Persist to SQLite
	if t.store != nil {
		if err := t.store.Insert(info); err != nil {
			slog.Error("session store insert failed", "error", err)
		}
	}

	return info
}

// GetSession returns session info by mcp session ID.
func (t *AgentTracker) GetSession(mcpSessionID string) *SessionInfo {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.sessions[mcpSessionID]
}

// RecordToolCall appends a tool invocation as conversation turns and returns the full turn list.
func (t *AgentTracker) RecordToolCall(mcpSessionID, toolName string, args map[string]any, result string) []conversation.Turn {
	t.mu.Lock()
	defer t.mu.Unlock()
	session, ok := t.sessions[mcpSessionID]
	if !ok {
		return nil
	}
	session.ToolCalls++
	session.LastSeenAt = time.Now()

	// User turn: the tool call
	userContent := fmt.Sprintf("[tool: %s]", toolName)
	if argsJSON, err := json.Marshal(args); err == nil && len(args) > 0 {
		userContent = fmt.Sprintf("[tool: %s] %s", toolName, string(argsJSON))
	}
	session.turns = append(session.turns, conversation.Turn{
		Role:    "user",
		Content: userContent,
	})

	// Assistant turn: the result (truncate for readability)
	resultText := result
	if len(resultText) > 2000 {
		resultText = resultText[:2000] + "..."
	}
	session.turns = append(session.turns, conversation.Turn{
		Role:    "assistant",
		Content: resultText,
	})

	// Persist tool call count + turn content to SQLite immediately
	if t.store != nil {
		if err := t.store.UpdateToolCalls(session.UUID, session.ToolCalls); err != nil {
			slog.Error("session store update tool calls failed", "error", err)
		}
		if err := t.store.RecordTurn(session.UUID, "user", toolName, userContent); err != nil {
			slog.Error("session store record turn failed", "role", "user", "error", err)
		}
		if err := t.store.RecordTurn(session.UUID, "assistant", toolName, resultText); err != nil {
			slog.Error("session store record turn failed", "role", "assistant", "error", err)
		}
	}

	// Return a copy of all turns
	out := make([]conversation.Turn, len(session.turns))
	copy(out, session.turns)
	return out
}

// Disconnect removes a session from the active cache and marks it disconnected in the DB.
func (t *AgentTracker) Disconnect(mcpSessionID string) *SessionInfo {
	t.mu.Lock()
	defer t.mu.Unlock()
	session, ok := t.sessions[mcpSessionID]
	if !ok {
		return nil
	}
	delete(t.sessions, mcpSessionID)

	// Mark disconnected in SQLite
	if t.store != nil {
		if err := t.store.MarkDisconnected(session.UUID); err != nil {
			slog.Error("session store disconnect failed", "error", err)
		}
	}

	return session
}

// List returns all currently connected sessions (from in-memory cache).
func (t *AgentTracker) List() []*SessionInfo {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := make([]*SessionInfo, 0, len(t.sessions))
	for _, s := range t.sessions {
		result = append(result, &SessionInfo{
			UUID:           s.UUID,
			Agent:          s.Agent,
			MCPSession:     s.MCPSession,
			Node:           s.Node,
			ConnectedAt:    s.ConnectedAt,
			LastSeenAt:     s.LastSeenAt,
			ToolCalls:      s.ToolCalls,
			ConversationID: s.ConversationID,
		})
	}
	return result
}

// Count returns number of currently connected sessions.
func (t *AgentTracker) Count() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.sessions)
}

// Store returns the underlying session store for direct queries (e.g., historical sessions).
func (t *AgentTracker) Store() *SessionStore {
	return t.store
}
