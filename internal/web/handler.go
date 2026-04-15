package web

import (
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"openloom/internal/chat"
	"openloom/internal/mcp"
	"openloom/internal/node"
	"openloom/internal/peer"

	"github.com/gorilla/mux"
)

//go:embed ui
var uiFS embed.FS

// Handler creates the main HTTP handler with all routes.
func Handler(n *node.Node, mcpServer *mcp.Server, hub *Hub, peerRegistry *peer.Registry, chatRouter *chat.Router, chatCtx *chat.ContextBuilder, chatTools *chat.ChatTools) http.Handler {
	r := mux.NewRouter()

	// Dashboard UI (embedded static files)
	uiContent, _ := fs.Sub(uiFS, "ui")
	r.PathPrefix("/assets/").Handler(http.FileServer(http.FS(uiContent)))
	// Cache-bust app.js and style.css — prevent browser serving stale UI
	r.Path("/style.css").HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		data, err := uiFS.ReadFile("ui/style.css")
		if err != nil {
			http.Error(w, "not found", 404)
			return
		}
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		w.Write(data)
	})
	// Serve any .js file from ui/ (or ui/views/) with cache-bust headers
	serveJS := func(w http.ResponseWriter, req *http.Request) {
		// Map URL path to embedded file: /app.js → ui/app.js, /views/tasks.js → ui/views/tasks.js
		path := "ui" + req.URL.Path
		data, err := uiFS.ReadFile(path)
		if err != nil {
			http.Error(w, "not found", 404)
			return
		}
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		w.Write(data)
	}
	r.PathPrefix("/views/").HandlerFunc(serveJS)
	r.Path("/app.js").HandlerFunc(serveJS)
	r.Path("/api.js").HandlerFunc(serveJS)
	r.Path("/tree.js").HandlerFunc(serveJS)

	// WebSocket
	r.HandleFunc("/ws", hub.HandleWS)

	// MCP endpoint with request logging
	mcpHandler := http.StripPrefix("/mcp", mcpServer.Handler())
	r.PathPrefix("/mcp").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.Debug("MCP request", "method", r.Method, "path", r.URL.Path, "from", r.RemoteAddr, "session", r.Header.Get("Mcp-Session-Id"))
		mcpHandler.ServeHTTP(w, r)
	})

	// Dashboard REST API
	api := r.PathPrefix("/api").Subrouter()
	api.HandleFunc("/status", apiStatus(n, mcpServer, peerRegistry)).Methods("GET")
	api.HandleFunc("/memories", apiMemories(n)).Methods("GET")
	api.HandleFunc("/memories/search", apiSearch(n)).Methods("POST")
	api.HandleFunc("/memories/tree", apiTree(n)).Methods("GET")
	api.HandleFunc("/memories/by-session", apiMemoriesBySession(n)).Methods("GET")
	api.HandleFunc("/memories/by-peer", apiMemoriesByPeer(n)).Methods("GET")
	api.HandleFunc("/memories/{id}", apiMemory(n)).Methods("GET")
	api.HandleFunc("/memories/{id}", apiMemoryDelete(n)).Methods("DELETE")
	api.HandleFunc("/peers", apiPeers(n, mcpServer, peerRegistry)).Methods("GET")
	api.HandleFunc("/agents", apiAgents(mcpServer)).Methods("GET")
	api.HandleFunc("/activity", apiActivity(n)).Methods("GET")
	api.HandleFunc("/activity/by-peer", apiActivityByPeer(n)).Methods("GET")
	api.HandleFunc("/conversations", apiConversations(n)).Methods("GET")
	api.HandleFunc("/conversations/by-peer", apiConversationsByPeer(n)).Methods("GET")
	api.HandleFunc("/conversations/search", apiConversationSearch(n)).Methods("POST")
	api.HandleFunc("/conversations/{id}/actions", apiConversationActions(n)).Methods("GET")
	api.HandleFunc("/conversations/{id}", apiConversation(n)).Methods("GET")
	api.HandleFunc("/markers", apiMarkers(n)).Methods("GET")
	api.HandleFunc("/markers/{id}/seen", apiMarkerSeen(n)).Methods("POST")
	api.HandleFunc("/markers/{id}/done", apiMarkerDone(n)).Methods("POST")
	// Hook endpoint for Claude Code conversation capture
	api.HandleFunc("/hook", apiHook(n)).Methods("POST")

	api.HandleFunc("/actions", apiActions(n)).Methods("GET")
	api.HandleFunc("/actions/by-peer", apiActionsByPeer(n)).Methods("GET")
	api.HandleFunc("/actions/{id}", apiAction(n)).Methods("GET")
	api.HandleFunc("/amnesia/by-peer", apiAmnesiaByPeer(n)).Methods("GET")
	api.HandleFunc("/amnesia", apiAmnesia(n)).Methods("GET")
	api.HandleFunc("/amnesia/{id}/confirm", apiAmnesiaUpdate(n, "confirmed")).Methods("POST")
	api.HandleFunc("/amnesia/{id}/dismiss", apiAmnesiaUpdate(n, "dismissed")).Methods("POST")
	api.HandleFunc("/ideas/by-peer", apiIdeasByPeer(n)).Methods("GET")
	api.HandleFunc("/ideas", apiIdeaList(n)).Methods("GET")
	api.HandleFunc("/ideas", apiIdeaCreate(n)).Methods("POST")
	api.HandleFunc("/ideas/{id}", apiIdeaGet(n)).Methods("GET")
	api.HandleFunc("/ideas/{id}", apiIdeaUpdate(n)).Methods("PUT")
	api.HandleFunc("/ideas/{id}", apiIdeaDelete(n)).Methods("DELETE")
	api.HandleFunc("/products/by-peer", apiProductsByPeer(n)).Methods("GET")
	api.HandleFunc("/products", apiProductList(n)).Methods("GET")
	api.HandleFunc("/products", apiProductCreate(n)).Methods("POST")
	api.HandleFunc("/products/{id}/hierarchy", apiProductHierarchy(n)).Methods("GET")
	api.HandleFunc("/products/{id}/manifests", apiProductManifests(n)).Methods("GET")
	api.HandleFunc("/products/{id}/ideas", apiProductIdeas(n)).Methods("GET")
	api.HandleFunc("/products/{id}", apiProductGet(n)).Methods("GET")
	api.HandleFunc("/products/{id}", apiProductUpdate(n)).Methods("PUT")
	api.HandleFunc("/products/{id}", apiProductDelete(n)).Methods("DELETE")
	api.HandleFunc("/manifests/by-peer", apiManifestsByPeer(n)).Methods("GET")
	api.HandleFunc("/manifests", apiManifestList(n)).Methods("GET")
	api.HandleFunc("/manifests", apiManifestCreate(n)).Methods("POST")
	api.HandleFunc("/manifests/search", apiManifestSearch(n)).Methods("POST")
	api.HandleFunc("/ideas/{id}/manifests", apiIdeasManifests(n)).Methods("GET")
	api.HandleFunc("/manifests/{id}/ideas", apiManifestIdeas(n)).Methods("GET")
	api.HandleFunc("/link", apiLink(n)).Methods("POST")
	api.HandleFunc("/unlink", apiUnlink(n)).Methods("POST")
	api.HandleFunc("/delusions/by-peer", apiDelusionsByPeer(n)).Methods("GET")
	api.HandleFunc("/delusions", apiDelusions(n)).Methods("GET")
	api.HandleFunc("/delusions/{id}/confirm", apiDelusionUpdate(n, "confirmed")).Methods("POST")
	api.HandleFunc("/delusions/{id}/dismiss", apiDelusionUpdate(n, "dismissed")).Methods("POST")
	api.HandleFunc("/manifests/{id}/tasks", apiManifestTasks(n)).Methods("GET")
	api.HandleFunc("/manifests/{id}", apiManifestGet(n)).Methods("GET")
	api.HandleFunc("/manifests/{id}", apiManifestUpdate(n)).Methods("PUT")
	api.HandleFunc("/manifests/{id}", apiManifestDelete(n)).Methods("DELETE")
	api.HandleFunc("/tasks/by-peer", apiTasksByPeer(n)).Methods("GET")
	api.HandleFunc("/tasks/running", apiRunningTasks(n)).Methods("GET")
	api.HandleFunc("/tasks/stats", apiTaskStats(n)).Methods("GET")
	api.HandleFunc("/tasks/cost-history", apiCostHistory(n)).Methods("GET")
	api.HandleFunc("/tasks/cost-agents", apiCostAgents(n)).Methods("GET")
	api.HandleFunc("/tasks/cost-trend", apiCostTrend(n)).Methods("GET")
	api.HandleFunc("/tasks", apiTaskList(n)).Methods("GET")
	api.HandleFunc("/tasks", apiTaskCreate(n)).Methods("POST")
	api.HandleFunc("/tasks/{id}", apiTaskGet(n)).Methods("GET")
	api.HandleFunc("/tasks/{id}", apiTaskUpdate(n)).Methods("PATCH")
	api.HandleFunc("/tasks/{id}", apiTaskDelete(n)).Methods("DELETE")
	api.HandleFunc("/tasks/{id}/actions", apiTaskActions(n)).Methods("GET")
	api.HandleFunc("/tasks/{id}/amnesia", apiTaskAmnesia(n)).Methods("GET")
	api.HandleFunc("/tasks/{id}/delusions", apiTaskDelusions(n)).Methods("GET")
	api.HandleFunc("/tasks/{id}/runs", apiTaskRuns(n)).Methods("GET")
	api.HandleFunc("/tasks/{id}/runs/{runId}", apiTaskRunGet(n)).Methods("GET")
	api.HandleFunc("/tasks/{id}/start", apiTaskStart(n)).Methods("POST")
	api.HandleFunc("/tasks/{id}/cancel", apiTaskUpdateStatus(n, "cancelled")).Methods("POST")
	api.HandleFunc("/tasks/{id}/kill", apiTaskKill(n)).Methods("POST")
	api.HandleFunc("/tasks/{id}/pause", apiTaskPause(n)).Methods("POST")
	api.HandleFunc("/tasks/{id}/resume", apiTaskResume(n)).Methods("POST")
	api.HandleFunc("/tasks/{id}/reschedule", apiTaskReschedule(n)).Methods("POST")
	api.HandleFunc("/tasks/{id}/output", apiTaskOutput(n)).Methods("GET")
	api.HandleFunc("/tasks/{id}/link-manifest", apiTaskLinkManifest(n)).Methods("POST")
	api.HandleFunc("/tasks/{id}/unlink-manifest", apiTaskUnlinkManifest(n)).Methods("POST")
	api.HandleFunc("/tasks/{id}/manifests", apiTaskManifests(n)).Methods("GET")
	api.HandleFunc("/tasks/{id}/dependency", apiTaskSetDependency(n)).Methods("PUT")
	api.HandleFunc("/visceral/by-peer", apiVisceralByPeer(n)).Methods("GET")
	api.HandleFunc("/visceral", apiVisceralList(n)).Methods("GET")
	api.HandleFunc("/visceral/confirmations", apiVisceralConfirmations(n)).Methods("GET")
	api.HandleFunc("/visceral", apiVisceralAdd(n)).Methods("POST")
	api.HandleFunc("/visceral/{id}", apiVisceralDelete(n)).Methods("DELETE")
	api.HandleFunc("/visceral/patterns", apiRulePatterns(n)).Methods("GET")
	api.HandleFunc("/visceral/patterns/{rule_id}", apiRulePatternGet(n)).Methods("GET")
	api.HandleFunc("/visceral/patterns/{rule_id}", apiRulePatternUpdate(n)).Methods("PUT")
	// Watcher — independent task execution audits
	api.HandleFunc("/watcher/audits", apiWatcherList(n)).Methods("GET")
	api.HandleFunc("/watcher/stats", apiWatcherStats(n)).Methods("GET")
	api.HandleFunc("/watcher/audits/{id}", apiWatcherGet(n)).Methods("GET")
	api.HandleFunc("/watcher/tasks/{id}", apiWatcherForTask(n)).Methods("GET")
	api.HandleFunc("/watcher/audit/{id}", apiWatcherTrigger(n)).Methods("POST")
	// Recall — soft-deleted items
	api.HandleFunc("/recall", apiRecall(n)).Methods("GET")
	api.HandleFunc("/recall/{type}/{id}/restore", apiRestore(n)).Methods("POST")

	api.HandleFunc("/settings/profile", apiProfileGet(n)).Methods("GET")
	api.HandleFunc("/settings/profile", apiProfileUpdate(n)).Methods("PUT")
	api.HandleFunc("/settings/agents", apiSettingsAgents()).Methods("GET")
	api.HandleFunc("/settings/agents/{id}/connect", apiAgentConnect()).Methods("POST")
	api.HandleFunc("/settings/agents/{id}/disconnect", apiAgentDisconnect()).Methods("POST")
	api.HandleFunc("/settings/chat", apiChatSettingsGet(n)).Methods("GET")
	api.HandleFunc("/settings/chat", apiChatSettingsUpdate(n, chatRouter)).Methods("PUT")
	api.HandleFunc("/settings/chat/test", apiChatSettingsTest()).Methods("POST")

	// Chat API
	api.HandleFunc("/chat", apiChatSend(n, chatRouter, chatCtx, chatTools)).Methods("POST")
	api.HandleFunc("/chat/models", apiChatModels(chatRouter)).Methods("GET")
	api.HandleFunc("/chat/sessions", apiChatSessionList(n)).Methods("GET")
	api.HandleFunc("/chat/sessions", apiChatSessionCreate(n, chatRouter)).Methods("POST")
	api.HandleFunc("/chat/sessions/{id}", apiChatSessionGet(n)).Methods("GET")
	api.HandleFunc("/chat/sessions/{id}", apiChatSessionDelete(n)).Methods("DELETE")
	api.HandleFunc("/chat/sessions/{id}/reset", apiChatSessionReset(n)).Methods("POST")
	api.HandleFunc("/chat/sessions/{id}/title", apiChatSessionUpdateTitle(n)).Methods("PUT")
	api.HandleFunc("/chat/sessions/{id}/model", apiChatSessionUpdateModel(n)).Methods("PUT")
	api.HandleFunc("/chat/sessions/{id}/thinking", apiChatSessionUpdateThinking(n)).Methods("PUT")
	api.HandleFunc("/chat/sessions/{id}/restore", apiChatSessionRestore(n)).Methods("POST")

	// Dashboard index (catch-all)
	r.PathPrefix("/").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := uiFS.ReadFile("ui/index.html")
		if err != nil {
			http.Error(w, "Dashboard not found", 500)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	})

	return r
}

func apiStatus(n *node.Node, mcpServer *mcp.Server, peerRegistry *peer.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		memCount, _ := n.Index.Count()
		convCount, _ := n.Conversations.Count()
		markerCount, _ := n.Markers.PendingCount(n.PeerID())

		// Count sessions from both MCP and hook-tracked conversations
		sessionCount := mcpServer.Tracker().Count()
		convos, _ := n.Conversations.List("", "", 20, 0)
		hookSessions := make(map[string]bool)
		for _, c := range convos {
			if len(c.ID) > 5 && c.ID[:5] == "hook-" {
				hookSessions[c.ID[5:]] = true
			}
		}
		if len(hookSessions) > sessionCount {
			sessionCount = len(hookSessions)
		}

		writeJSON(w, map[string]any{
			"node":          n.PeerID(),
			"hostname":      n.Config.Node.Hostname,
			"display_name":  n.Config.Node.DisplayName,
			"email":         n.Config.Node.Email,
			"avatar":        n.Config.Node.Avatar,
			"memories":      memCount,
			"conversations": convCount,
			"markers":       markerCount,
			"sessions":      sessionCount,
			"peers":         peerRegistry.Count() + 1, // +1 for local node
			"uptime":        time.Since(n.StartedAt).Round(time.Second).String(),
			"embedding":     n.Config.Embedding.Model,
		})
	}
}

var budgetRe = regexp.MustCompile(`(?i)daily\s+budget\s*=?\s*\$?([\d.]+)`)

// parseDailyBudget scans visceral rules for a "daily budget = $X" pattern.
func parseDailyBudget(n *node.Node) float64 {
	rules, err := n.Index.ListByType("visceral", 100)
	if err != nil {
		return 0
	}
	for _, r := range rules {
		if m := budgetRe.FindStringSubmatch(r.L2); m != nil {
			if v, err := strconv.ParseFloat(m[1], 64); err == nil {
				return v
			}
		}
	}
	return 0
}

func apiPeers(n *node.Node, mcpServer *mcp.Server, peerRegistry *peer.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		type sessionView struct {
			UUID        string `json:"uuid"`
			Agent       string `json:"agent"`
			ConnectedAt string `json:"connected_at"`
			ToolCalls   int    `json:"tool_calls"`
			TurnCount   int    `json:"turn_count"`
			Source      string `json:"source"` // "mcp" or "hook"
		}
		type nodeView struct {
			NodeID   string        `json:"node_id"`
			IsLocal  bool          `json:"is_local"`
			Sessions []sessionView `json:"sessions"`
			Memories int           `json:"memories,omitempty"`
			Status   string        `json:"status"`
		}

		memCount, _ := n.Index.Count()
		seen := make(map[string]bool)
		var sv []sessionView

		// MCP-connected sessions
		for _, s := range mcpServer.Tracker().List() {
			seen[s.UUID] = true
			sv = append(sv, sessionView{
				UUID:        s.UUID,
				Agent:       s.Agent,
				ConnectedAt: s.ConnectedAt.UTC().Format("2006-01-02T15:04:05Z"),
				ToolCalls:   s.ToolCalls,
				Source:      "mcp",
			})
		}

		// Hook-tracked sessions (from conversations with hook- prefix)
		convos, _ := n.Conversations.List("", "", 20, 0)
		for _, c := range convos {
			if len(c.ID) > 5 && c.ID[:5] == "hook-" {
				sessionID := c.ID[5:]
				shortID := sessionID
				if len(shortID) > 12 {
					shortID = shortID[:12]
				}
				if seen[shortID] {
					continue
				}
				seen[shortID] = true
				sv = append(sv, sessionView{
					UUID:        shortID,
					Agent:       c.Agent,
					ConnectedAt: c.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
					TurnCount:   c.TurnCount,
					Source:      "hook",
				})
			}
		}

		nodes := []nodeView{{
			NodeID:   n.PeerID(),
			IsLocal:  true,
			Sessions: sv,
			Memories: memCount,
			Status:   "online",
		}}

		// Remote peers
		for _, p := range peerRegistry.List() {
			nodes = append(nodes, nodeView{
				NodeID:   p.NodeID,
				IsLocal:  false,
				Memories: p.Memories,
				Status:   p.Status,
			})
		}

		writeJSON(w, nodes)
	}
}

func apiAgents(mcpServer *mcp.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessions := mcpServer.Tracker().List()
		// Group sessions by node
		type nodeGroup struct {
			Node     string             `json:"node"`
			Sessions []*mcp.SessionInfo `json:"sessions"`
		}
		groups := make(map[string]*nodeGroup)
		for _, s := range sessions {
			g, ok := groups[s.Node]
			if !ok {
				g = &nodeGroup{Node: s.Node}
				groups[s.Node] = g
			}
			g.Sessions = append(g.Sessions, s)
		}
		result := make([]*nodeGroup, 0, len(groups))
		for _, g := range groups {
			result = append(result, g)
		}
		writeJSON(w, result)
	}
}

func apiActivity(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		type activityItem struct {
			ID      string `json:"id"`
			Time    string `json:"time"`
			Type    string `json:"type"`
			Title   string `json:"title"`
			Detail  string `json:"detail"`
			Session string `json:"session"`
		}

		var items []activityItem

		// Recent memories
		mems, _ := n.Index.ListByPrefix("/", 50)
		for _, m := range mems {
			marker := ""
			if len(m.ID) >= 12 {
				marker = m.ID[:12]
			}
			items = append(items, activityItem{
				ID:      m.ID,
				Time:    m.CreatedAt,
				Type:    "memory",
				Title:   fmt.Sprintf("[%s] %s", marker, m.L0),
				Detail:  m.Path,
				Session: m.SourceAgent,
			})
		}

		// Recent conversations
		convos, _ := n.Conversations.List("", "", 20, 0)
		for _, c := range convos {
			items = append(items, activityItem{
				ID:      c.ID,
				Time:    c.UpdatedAt.UTC().Format(time.RFC3339),
				Type:    "conversation",
				Title:   c.Title,
				Detail:  fmt.Sprintf("%d turns", c.TurnCount),
				Session: c.Agent,
			})
		}

		// Sort by time descending
		for i := 0; i < len(items); i++ {
			for j := i + 1; j < len(items); j++ {
				if items[j].Time > items[i].Time {
					items[i], items[j] = items[j], items[i]
				}
			}
		}

		// Cap at 50
		if len(items) > 50 {
			items = items[:50]
		}

		writeJSON(w, items)
	}
}

func apiActivityByPeer(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		type activityItem struct {
			ID         string `json:"id"`
			Time       string `json:"time"`
			Type       string `json:"type"`
			Title      string `json:"title"`
			Session    string `json:"session"`
			SourceNode string `json:"source_node"`
		}

		var all []activityItem

		// Memories
		mems, _ := n.Index.ListByPrefix("/", 100)
		for _, m := range mems {
			marker := ""
			if len(m.ID) >= 12 {
				marker = m.ID[:12]
			}
			node := m.SourceNode
			if node == "" {
				node = n.PeerID()
			}
			all = append(all, activityItem{
				ID:         m.ID,
				Time:       m.CreatedAt,
				Type:       "memory",
				Title:      fmt.Sprintf("[%s] %s", marker, m.L0),
				Session:    m.SourceAgent,
				SourceNode: node,
			})
		}

		// Conversations
		convos, _ := n.Conversations.List("", "", 50, 0)
		for _, c := range convos {
			node := c.SourceNode
			if node == "" {
				node = n.PeerID()
			}
			all = append(all, activityItem{
				ID:         c.ID,
				Time:       c.UpdatedAt.UTC().Format(time.RFC3339),
				Type:       "conversation",
				Title:      c.Title,
				Session:    c.Agent,
				SourceNode: node,
			})
		}

		// Sort by time descending
		for i := 0; i < len(all); i++ {
			for j := i + 1; j < len(all); j++ {
				if all[j].Time > all[i].Time {
					all[i], all[j] = all[j], all[i]
				}
			}
		}

		// Group: peer -> session -> items
		type sessionGroup struct {
			Session    string         `json:"session"`
			Count      int            `json:"count"`
			Activities []activityItem `json:"activities"`
		}
		type peerGroup struct {
			PeerID   string         `json:"peer_id"`
			Count    int            `json:"count"`
			Sessions []sessionGroup `json:"sessions"`
		}

		type peerData struct {
			sessionOrder []string
			sessions     map[string][]activityItem
		}
		peers := make(map[string]*peerData)
		peerOrder := []string{}

		for _, a := range all {
			pid := a.SourceNode
			sess := a.Session
			if sess == "" {
				sess = "unknown"
			}

			pd, ok := peers[pid]
			if !ok {
				pd = &peerData{sessions: make(map[string][]activityItem)}
				peers[pid] = pd
				peerOrder = append(peerOrder, pid)
			}
			if _, ok := pd.sessions[sess]; !ok {
				pd.sessionOrder = append(pd.sessionOrder, sess)
			}
			pd.sessions[sess] = append(pd.sessions[sess], a)
		}

		var result []peerGroup
		for _, pid := range peerOrder {
			pd := peers[pid]
			var sgs []sessionGroup
			totalCount := 0
			for _, sess := range pd.sessionOrder {
				alist := pd.sessions[sess]
				sgs = append(sgs, sessionGroup{
					Session:    sess,
					Count:      len(alist),
					Activities: alist,
				})
				totalCount += len(alist)
			}
			result = append(result, peerGroup{
				PeerID:   pid,
				Count:    totalCount,
				Sessions: sgs,
			})
		}
		writeJSON(w, result)
	}
}

// writeJSON, writeError, decodeBody are in helpers.go
