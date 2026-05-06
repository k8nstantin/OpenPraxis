package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/k8nstantin/OpenPraxis/internal/chat"
	"github.com/k8nstantin/OpenPraxis/internal/comments"
	"github.com/k8nstantin/OpenPraxis/internal/config"
	mcpserver "github.com/k8nstantin/OpenPraxis/internal/mcp"
	"github.com/k8nstantin/OpenPraxis/internal/node"
	"github.com/k8nstantin/OpenPraxis/internal/peer"
	"github.com/k8nstantin/OpenPraxis/internal/schedule"
	"github.com/k8nstantin/OpenPraxis/internal/setup"
	"github.com/k8nstantin/OpenPraxis/internal/task"
	"github.com/k8nstantin/OpenPraxis/internal/web"

	"github.com/spf13/cobra"
)

var noBrowser bool

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the OpenPraxis daemon",
	Long:  "Start the MCP server, web dashboard, peer discovery, and sync server.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		// Initialize structured logging — all operational logs go to stderr
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})))

		fmt.Printf("\n  OpenPraxis %s\n\n", Version)

		// Auto-setup: install Ollama + model if missing
		if err := setup.EnsureReady(cfg.Embedding.Model); err != nil {
			return fmt.Errorf("setup: %w", err)
		}

		// Auto-configure coding agents
		if err := setup.ConfigureAgents(); err != nil {
			fmt.Fprintf(os.Stderr, "  Agent config warning: %v\n", err)
		}

		fmt.Println("")
		fmt.Printf("  Peer:       %s\n", cfg.Node.PeerID())
		if cfg.Node.DisplayName != "" {
			fmt.Printf("  Name:       %s\n", cfg.Node.DisplayName)
		}
		fmt.Printf("  Host:       %s\n", cfg.Node.Hostname)
		fmt.Printf("  MAC:        %s\n", cfg.Node.MAC)
		fmt.Printf("  Dashboard:  http://%s:%d\n", cfg.Server.Host, cfg.Server.Port)
		fmt.Printf("  MCP:        http://%s:%d/mcp\n", cfg.Server.Host, cfg.Server.Port)
		fmt.Printf("  Sync:       http://%s:%d\n", cfg.Sync.Host, cfg.Sync.Port)
		fmt.Printf("  Peers:      discovering...\n\n")

		// Cancellable context for all background goroutines
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Initialize node
		n, err := node.New(cfg)
		if err != nil {
			return fmt.Errorf("init node: %w", err)
		}
		defer n.Close()

		// Create MCP server
		mcp := mcpserver.NewServer(n, n.Index.DB())

		// Create WebSocket hub
		hub := web.NewHub()

		// --- Peer discovery + sync ---
		peerRegistry := peer.NewRegistry()

		// Sync server (Automerge sync over HTTP)
		syncServer := peer.NewSyncServer(n.Store, peerRegistry, cfg.Node.PeerID(), cfg.Sync.FanOut)

		// Wire CRDT writes to gossip push
		n.Store.SetOnChange(func(ids []string) {
			// Reindex locally
			n.ReindexMemories(ids)
			// Gossip to peers
			syncServer.PushToPeers(ctx)
			// Broadcast to dashboard
			for _, id := range ids {
				hub.Broadcast(web.Event{Type: "memory_stored", Data: map[string]string{"id": id}})
			}
		})

		// Start sync HTTP server on :8766
		syncAddr := fmt.Sprintf("%s:%d", cfg.Sync.Host, cfg.Sync.Port)
		syncHTTP := &http.Server{
			Addr:         syncAddr,
			Handler:      syncServer.Handler(),
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
		}
		go func() {
			slog.Info("sync server listening", "addr", syncAddr)
			if err := syncHTTP.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Error("sync server failed", "error", err)
			}
		}()

		// mDNS peer discovery
		discovery := peer.NewDiscovery(peerRegistry, cfg.Node.PeerID(), cfg.Sync.Port)
		discovery.SetCallbacks(
			func(nodeID, ip string, port int) {
				// Peer found — initiate sync
				hub.Broadcast(web.Event{Type: "peer_joined", Data: map[string]any{
					"node_id": nodeID, "ip": ip, "port": port,
				}})
				go syncServer.SyncWithPeerByID(ctx, nodeID)
			},
			func(nodeID string) {
				hub.Broadcast(web.Event{Type: "peer_left", Data: map[string]string{"node_id": nodeID}})
			},
		)
		if err := discovery.Start(ctx); err != nil {
			slog.Warn("mDNS discovery failed, continuing without discovery", "error", err)
		} else {
			defer discovery.Stop()
		}

		// Connect static peers
		if len(cfg.Peers.Static) > 0 {
			discovery.ConnectStatic(ctx, cfg.Peers.Static)
		}

		// Anti-entropy: periodic full sync with a random peer
		go syncServer.StartAntiEntropy(ctx, time.Duration(cfg.Sync.AntiEntropySeconds)*time.Second)

		// --- Chat router ---
		bridge := newNodeBridge(n)
		chatRouter := chat.NewRouter(cfg)
		chatContext := chat.NewContextBuilder(bridge)
		chatTools := chat.NewChatTools(bridge)

		// --- Main HTTP server (dashboard + MCP + API) ---
		// ServerDeps bundles the cross-cutting services every HTTP route needs.
		deps := web.ServerDeps{
			Node:         n,
			MCP:          mcp,
			Hub:          hub,
			PeerRegistry: peerRegistry,
			ChatRouter:   chatRouter,
			ChatCtx:      chatContext,
			ChatTools:    chatTools,
		}
		handler := web.Handler(deps)
		addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
		httpServer := &http.Server{
			Addr:         addr,
			Handler:      handler,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
		}

		go func() {
			slog.Info("HTTP server listening", "addr", addr)
			if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Error("HTTP server failed", "error", err)
			}
		}()

		// Task runner — spawns autonomous agent subprocesses.
		// Architecture: schedule.Runner fires entity_uid from the schedules
		// table → runner executes the entity → stats go to execution_log.
		runner := n.InitRunner(func(event string, data map[string]string) {
			hub.Broadcast(web.Event{Type: event, Data: data})
		})

		if err := runner.RecoverInFlight(ctx); err != nil {
			slog.Warn("recover in-flight tasks failed", "error", err)
		}

		// entityDesc returns the description_revision body for an entity,
		// falling back to the entity title when no revision comment exists.
		entityDesc := func(ctx context.Context, entityID, fallback string) string {
			ct := comments.TypeDescriptionRevision
			revs, _ := n.Comments.List(ctx, comments.TargetEntity, entityID, 1, &ct)
			if len(revs) > 0 && revs[0].Body != "" {
				return revs[0].Body
			}
			return fallback
		}

		// visceralRules loads all visceral rules into a numbered string.
		visceralRules := func() string {
			rules, _ := n.Index.ListByType("visceral", 100)
			var s string
			for i, r := range rules {
				s += fmt.Sprintf("%d. [%s] %s\n", i+1, r.ID, r.L2)
			}
			return s
		}

		// parentContext walks UP the DAG from entityID to collect manifest
		// and product context. For a task it walks task→manifest→product;
		// for a manifest it walks manifest→product; for a product it stops.
		// Returns formatted context sections ready for prompt inclusion.
		type parentCtx struct {
			manifestID, manifestTitle, manifestDesc string
			productID, productTitle, productDesc    string
		}
		walkUp := func(ctx context.Context, entityID, entityType string) parentCtx {
			if n.Relationships == nil {
				return parentCtx{}
			}
			var pc parentCtx
			lookupID := entityID

			// For a task: find owning manifest first.
			if entityType == "task" {
				if edges, _ := n.Relationships.ListIncoming(ctx, lookupID, "owns"); len(edges) > 0 {
					for _, edge := range edges {
						if edge.SrcKind == "manifest" {
							pc.manifestID = edge.SrcID
							break
						}
					}
				}
				if pc.manifestID != "" {
					if me, _ := n.Entities.Get(pc.manifestID); me != nil {
						pc.manifestTitle = me.Title
						pc.manifestDesc = entityDesc(ctx, pc.manifestID, me.Title)
					}
					lookupID = pc.manifestID
				}
			}

			// For a task (after finding manifest) or manifest: find owning product.
			if entityType == "task" || entityType == "manifest" {
				if edges, _ := n.Relationships.ListIncoming(ctx, lookupID, "owns"); len(edges) > 0 {
					for _, edge := range edges {
						if edge.SrcKind == "product" {
							pc.productID = edge.SrcID
							break
						}
					}
				}
				if pc.productID != "" {
					if pe, _ := n.Entities.Get(pc.productID); pe != nil {
						pc.productTitle = pe.Title
						pc.productDesc = entityDesc(ctx, pc.productID, pe.Title)
					}
				}
			}
			return pc
		}

		// dispatch is the universal handler for any scheduled entity kind.
		// Every prompt includes the full 3-tier context regardless of entry
		// point: product (big picture) + manifest (refinement) + task
		// (instructions). Context above the entry point is pre-fetched by
		// walking UP the DAG via relationships. Context below is handed to
		// the agent to traverse via MCP tools at runtime.
		dispatch := func(ctx context.Context, entityID string, scheduleID int64) error {
			// System-level concurrency guard: only one agent at a time.
			// Products, manifests, and tasks all share the same codebase —
			// concurrent agents conflict on branches and PRs.
			if runner.RunningCount() > 0 {
				running := runner.ListRunning()
				titles := make([]string, 0, len(running))
				for _, rt := range running {
					titles = append(titles, rt.Title)
				}
				return fmt.Errorf("agent already running (%v) — refusing to start %s concurrently", titles, entityID)
			}

			e, err := n.Entities.Get(entityID)
			if err != nil || e == nil {
				return fmt.Errorf("entity not found: %s", entityID)
			}

			desc := entityDesc(ctx, entityID, e.Title)
			pc := walkUp(ctx, entityID, e.Type)

			// Build the context header — whatever sits above the entry point.
			var contextSection string
			if pc.productTitle != "" {
				contextSection += fmt.Sprintf("## Product Context (Big Picture)\n**%s** [%s]\n\n%s\n\n",
					pc.productTitle, pc.productID, pc.productDesc)
			}
			if pc.manifestTitle != "" {
				contextSection += fmt.Sprintf("## Manifest Context (Refinement)\n**%s** [%s]\n\n%s\n\n",
					pc.manifestTitle, pc.manifestID, pc.manifestDesc)
			}

			var prompt string
			switch e.Type {
			case "product":
				prompt = fmt.Sprintf(
					"## Product Context (Big Picture)\n**%s** [%s]\n\n%s\n\n"+
						"## Your Job\n"+
						"Use OpenPraxis MCP tools to travel the DAG:\n"+
						"1. Find all active manifests under this product via relationships\n"+
						"2. For each manifest read its definition — this is your refinement context\n"+
						"3. For each task under each manifest read its instructions and execute with full context\n"+
						"4. Honour depends_on ordering between tasks\n"+
						"5. Post results/findings back to this product via comment_add\n\n"+
						"## Visceral Rules\n%s",
					e.Title, entityID, desc, visceralRules(),
				)
			case "manifest":
				prompt = fmt.Sprintf(
					"%s"+ // product context if present
						"## Manifest Context (Refinement)\n**%s** [%s]\n\n%s\n\n"+
						"## Your Job\n"+
						"Use OpenPraxis MCP tools to travel the DAG:\n"+
						"1. Find all active tasks under this manifest via relationships\n"+
						"2. Execute each task with the product + manifest context above in mind\n"+
						"3. Honour depends_on ordering between tasks\n"+
						"4. Post results back via comment_add\n\n"+
						"## Visceral Rules\n%s",
					contextSection, e.Title, entityID, desc, visceralRules(),
				)
			default: // task
				prompt = fmt.Sprintf(
					"%s"+ // product + manifest context if present
						"## Task Instructions\n**%s** [%s]\n\n%s\n\n"+
						"Execute the task instructions above. The product and manifest context\n"+
						"sections define the big picture and constraints — follow them.\n\n"+
						"## Visceral Rules\n%s",
					contextSection, e.Title, entityID, desc, visceralRules(),
				)
			}

			slog.Info("schedule: dispatching entity",
				"entity_uid", entityID, "type", e.Type, "title", e.Title,
				"manifest_id", pc.manifestID, "product_id", pc.productID)

			t := &task.Task{
				ID:     entityID,
				Title:  e.Title,
				Status: "scheduled",
				Agent:  "claude-code",
			}
			return runner.Execute(t, e.Title, prompt, visceralRules())
		}

		// Same dispatch function handles any entity kind — the agent travels
		// the DAG using MCP tools at runtime.
		scheduleRunner := schedule.NewRunner(n.Schedules, map[string]schedule.DispatchFunc{
			"task":     dispatch,
			"manifest": dispatch,
			"product":  dispatch,
		})
		n.ScheduleRunner = scheduleRunner
		if err := scheduleRunner.Start(ctx); err != nil {
			slog.Error("schedule runner start failed", "error", err)
		}

		runner.StartActionWatcher(2 * time.Second)

		// --- Startup state summary ---
		fmt.Println("\n  === OpenPraxis State ===")
		fmt.Printf("  Peer:      %s\n", cfg.Node.UUID)
		fmt.Printf("  Name:      %s %s\n", cfg.Node.Avatar, cfg.Node.DisplayName)

		if rules, err := n.Index.ListByType("visceral", 100); err == nil && len(rules) > 0 {
			fmt.Printf("  Visceral:  %d rules\n", len(rules))
		}
		if active, err := n.Entities.List("task", "active", 0); err == nil {
			fmt.Printf("  Tasks:     %d active\n", len(active))
		}
		if manifests, err := n.Entities.List("manifest", "active", 0); err == nil {
			fmt.Printf("  Manifests: %d active\n", len(manifests))
		}
		fmt.Println("  =====================")

		// Open browser
		if cfg.Server.OpenBrowser && !noBrowser {
			time.Sleep(200 * time.Millisecond)
			url := fmt.Sprintf("http://localhost:%d", cfg.Server.Port)
			openBrowser(url)
		}

		// Wait for shutdown signal
		done := make(chan os.Signal, 1)
		signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		<-done

		fmt.Println("\n  Shutting down...")
		cancel() // stop background goroutines

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		httpServer.Shutdown(shutdownCtx)
		syncHTTP.Shutdown(shutdownCtx)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
	serveCmd.Flags().BoolVar(&noBrowser, "no-browser", false, "Don't open the dashboard in the browser")
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	}
	if cmd != nil {
		_ = cmd.Start()
	}
}
