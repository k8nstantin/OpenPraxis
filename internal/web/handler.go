package web

import (
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/k8nstantin/OpenPraxis/internal/chat"
	"github.com/k8nstantin/OpenPraxis/internal/mcp"
	"github.com/k8nstantin/OpenPraxis/internal/node"
	"github.com/k8nstantin/OpenPraxis/internal/peer"

	"github.com/gorilla/mux"
)

//go:embed all:ui/dashboard-v2/dist
var uiFS embed.FS

// ServerDeps bundles the cross-cutting services every HTTP handler needs
// (DB-backed stores via Node, MCP server, WebSocket hub, peer registry,
// chat plumbing). All fields are non-nil at runtime; tests build a
// minimal ServerDeps directly.
type ServerDeps struct {
	Node         *node.Node
	MCP          *mcp.Server
	Hub          *Hub
	PeerRegistry *peer.Registry
	ChatRouter   *chat.Router
	ChatCtx      *chat.ContextBuilder
	ChatTools    *chat.ChatTools
}

// Handler creates the main HTTP handler on :8765 — React dashboard plus the
// full backend (/api/*, /mcp, /ws).
func Handler(deps ServerDeps) http.Handler {
	r := mux.NewRouter()

	// REST API
	api := r.PathPrefix("/api").Subrouter()
	mountAPI(api, deps)

	// WebSocket broadcast hub
	mountWS(r, deps)

	// MCP endpoint — agent-facing. Agent configs (~/.claude/settings.json, etc.)
	// point at :8765/mcp so this stays on the primary port only.
	mountMCP(r, deps)

	// React shell — Vite build output embedded at compile time.
	// The app uses hash-based routing (createHashHistory) so deep links always
	// arrive at the server as "/". The fallback below also handles any future
	// switch to history mode: unknown paths serve index.html instead of 404.
	v2Content, err := fs.Sub(uiFS, "ui/dashboard-v2/dist")
	if err != nil {
		panic(fmt.Sprintf("dashboard-v2 embed sub: %v", err))
	}
	fileServer := http.FileServer(http.FS(v2Content))
	r.PathPrefix("/").HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Strip leading "/" — fs.FS paths must not start with slash.
		path := strings.TrimPrefix(req.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(v2Content, path); err != nil {
			// File not found: fall back to index.html for SPA deep-link support.
			req = req.Clone(req.Context())
			req.URL.Path = "/"
		}
		if req.URL.Path == "/" || req.URL.Path == "/index.html" {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		}
		fileServer.ServeHTTP(w, req)
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
			items = append(items, activityItem{
				ID:      m.ID,
				Time:    m.CreatedAt,
				Type:    "memory",
				Title:   fmt.Sprintf("[%s] %s", m.ID, m.L0),
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
			node := m.SourceNode
			if node == "" {
				node = n.PeerID()
			}
			all = append(all, activityItem{
				ID:         m.ID,
				Time:       m.CreatedAt,
				Type:       "memory",
				Title:      fmt.Sprintf("[%s] %s", m.ID, m.L0),
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


// mountWS registers the WebSocket broadcast endpoint on the given router.
func mountWS(r *mux.Router, deps ServerDeps) {
	r.HandleFunc("/ws", deps.Hub.HandleWS)
}

// mountMCP registers the agent-facing MCP HTTP endpoint with request logging.
func mountMCP(r *mux.Router, deps ServerDeps) {
	mcpHandler := http.StripPrefix("/mcp", deps.MCP.Handler())
	r.PathPrefix("/mcp").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.Debug("MCP request", "method", r.Method, "path", r.URL.Path, "from", r.RemoteAddr, "session", r.Header.Get("Mcp-Session-Id"))
		mcpHandler.ServeHTTP(w, r)
	})
}

// mountAPI registers every /api/* route on the given subrouter.
// Route order matters: gorilla/mux matches in registration order, so
// scoped-search routes must be registered BEFORE /{id} catch-all patterns.
func mountAPI(api *mux.Router, deps ServerDeps) {
	n := deps.Node
	mcpServer := deps.MCP
	peerRegistry := deps.PeerRegistry
	chatRouter := deps.ChatRouter
	chatCtx := deps.ChatCtx
	chatTools := deps.ChatTools

	api.HandleFunc("/status", apiStatus(n, mcpServer, peerRegistry)).Methods("GET")
	// Per-tab scoped search (M2). Registered before /{id} routes because
	// gorilla/mux matches in registration order — otherwise "search" would
	// be swallowed by /ideas/{id}, /actions/{id}.
	api.HandleFunc("/tasks/search", apiTasksSearch(n)).Methods("GET")
	api.HandleFunc("/actions/search", apiActionsSearch(n)).Methods("GET")
	api.HandleFunc("/memories", apiMemories(n)).Methods("GET")
	api.HandleFunc("/memories/search", apiSearch(n)).Methods("POST")
	api.HandleFunc("/memories/search", apiMemoriesSearchGET(n)).Methods("GET")
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
	api.HandleFunc("/conversations/search", apiConversationsSearchGET(n)).Methods("GET")
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
	api.HandleFunc("/entities/search", apiEntitySearch(n)).Methods("GET")
	api.HandleFunc("/entities", apiEntityList(n)).Methods("GET")
	api.HandleFunc("/entities", apiEntityCreate(n)).Methods("POST")
	api.HandleFunc("/entities/{id}/history", apiEntityHistory(n)).Methods("GET")
	api.HandleFunc("/entities/{id}/runs", apiEntityExecutionLog(n)).Methods("GET")
	api.HandleFunc("/entities/{id}/comments", apiEntityCommentsList(n)).Methods("GET")
	api.HandleFunc("/entities/{id}/comments", apiEntityCommentsAdd(n)).Methods("POST")
	api.HandleFunc("/entities/{id}", apiEntityGet(n)).Methods("GET")
	api.HandleFunc("/entities/{id}", apiEntityUpdate(n)).Methods("PUT")
	// Legacy dependency management routes — used by the Dependencies tab.
	// Products expose downstream sub-products; manifests expose upstream deps.
	api.HandleFunc("/products/{id}/dependencies", apiEntityDependencies(n, "product")).Methods("GET")
	api.HandleFunc("/products/{id}/dependencies", apiEntityAddDependency(n, "product")).Methods("POST")
	api.HandleFunc("/products/{id}/dependencies/{dep_id}", apiEntityRemoveDependency(n, "product")).Methods("DELETE")
	api.HandleFunc("/manifests/{id}/dependencies", apiEntityDependencies(n, "manifest")).Methods("GET")
	api.HandleFunc("/manifests/{id}/dependencies", apiEntityAddDependency(n, "manifest")).Methods("POST")
	api.HandleFunc("/manifests/{id}/dependencies/{dep_id}", apiEntityRemoveDependency(n, "manifest")).Methods("DELETE")
	// Legacy hierarchy route — used by the product list-pane tree expansion.
	api.HandleFunc("/products/{id}/hierarchy", apiProductHierarchy(n)).Methods("GET")
	api.HandleFunc("/execution/live", apiExecutionLive(n)).Methods("GET")
	api.HandleFunc("/execution/{runUid}/output", apiExecutionOutput(n)).Methods("GET")
	api.HandleFunc("/execution/{runUid}", apiExecutionLog(n)).Methods("GET")
	api.HandleFunc("/delusions/by-peer", apiDelusionsByPeer(n)).Methods("GET")
	api.HandleFunc("/delusions", apiDelusions(n)).Methods("GET")
	api.HandleFunc("/delusions/{id}/confirm", apiDelusionUpdate(n, "confirmed")).Methods("POST")
	api.HandleFunc("/delusions/{id}/dismiss", apiDelusionUpdate(n, "dismissed")).Methods("POST")
	// Live host CPU/RSS — feeds the node stats chip on the overview.
	api.HandleFunc("/host/stats", apiHostStats()).Methods("GET")
	// Stats tab — entity-scoped run aggregation + continuous host stream.
	// run-stats dispatches per entity_kind (task by task_id). system-stats
	// reads system_host_samples between [from, to], optionally bounded
	// by as_of.
	api.HandleFunc("/stats/aggregate", apiStatsAggregate(n)).Methods("GET")
	api.HandleFunc("/stats/trend", apiStatsTrend(n)).Methods("GET")
	api.HandleFunc("/stats/overview", apiStatsOverview(n)).Methods("GET")
	api.HandleFunc("/stats/charts", apiStatsCharts(n)).Methods("GET")
	api.HandleFunc("/stats/git", apiGitProductivity(n)).Methods("GET")
	api.HandleFunc("/stats/history", apiStatsHistory(n)).Methods("GET")
	api.HandleFunc("/run-stats", apiRunStats(n)).Methods("GET")
	api.HandleFunc("/system-stats", apiSystemStats(n)).Methods("GET")
	api.HandleFunc("/schedules/templates", apiScheduleTemplates()).Methods("GET")
	api.HandleFunc("/schedules/history", apiSchedulesHistory(n)).Methods("GET")
	api.HandleFunc("/schedules", apiSchedulesList(n)).Methods("GET")
	api.HandleFunc("/schedules", apiSchedulesCreate(n)).Methods("POST")
	api.HandleFunc("/schedules/{id}", apiScheduleClose(n)).Methods("DELETE")
	api.HandleFunc("/relationships", apiRelationshipCreate(n)).Methods("POST")
	api.HandleFunc("/relationships", apiRelationshipDelete(n)).Methods("DELETE")
	// /api/relationships/graph?root_id=&root_kind=&depth=&edge_kinds=
	// returns a flat (nodes, edges) shape for the DAG tab. Source of
	// truth: the relationships SCD-2 table; Walk traversal handles the
	// recursive descent.
	api.HandleFunc("/relationships/graph", apiRelationshipsGraph(n)).Methods("GET")
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

	// Hierarchical execution-controls settings (M2-T6) — registered before
	// the profile/agents/chat /settings/* routes because gorilla/mux matches
	// paths in registration order. catalog + scope-keyed endpoints share the
	// /settings prefix without colliding (different verbs / suffixes).
	registerSettingsExecRoutes(api, n)
	registerCommentsRoutesFromNode(api, n)
	registerAttachmentRoutes(api, n)
	registerDescriptionRoutes(api, n)
	registerTemplateRoutes(api, n)
	registerScheduleRoutes(api, n)

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
}
