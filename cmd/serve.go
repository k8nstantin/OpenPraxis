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

		// Schedule-driven dispatcher — the sole source of task firing.
		// Reads entity_uid from the schedules table; looks up entity +
		// description from entities+comments; executes via the runner.
		scheduleRunner := schedule.NewRunner(n.Schedules, map[string]schedule.DispatchFunc{
			"task": func(ctx context.Context, entityID string, scheduleID int64) error {
				e, err := n.Entities.Get(entityID)
				if err != nil || e == nil {
					return fmt.Errorf("entity not found: %s", entityID)
				}

				// Build agent content from latest description_revision comment.
				var content string
				ct := comments.TypeDescriptionRevision
				revs, _ := n.Comments.List(ctx, comments.TargetEntity, entityID, 1, &ct)
				if len(revs) > 0 {
					content = revs[0].Body
				}
				if content == "" {
					content = e.Title
				}

				// Load visceral rules for the prompt.
				rules, _ := n.Index.ListByType("visceral", 100)
				var visceralText string
				for i, r := range rules {
					visceralText += fmt.Sprintf("%d. [%s] %s\n", i+1, r.ID, r.L2)
				}

				slog.Info("schedule fired entity", "entity_uid", entityID, "type", e.Type, "title", e.Title)

				// Execute against the legacy task runner using entity_uid.
				t, _ := n.Tasks.Get(entityID)
				if t == nil {
					return fmt.Errorf("task record not found for entity: %s", entityID)
				}
				return runner.Execute(t, e.Title, content, visceralText)
			},
		})
		n.ScheduleRunner = scheduleRunner
		if err := scheduleRunner.Start(ctx); err != nil {
			slog.Error("schedule runner start failed", "error", err)
		}

		runner.StartActionWatcher(2 * time.Second)

		// System host metrics sampler — writes CPU/RSS/disk to
		// system_host_samples for the Stats tab System Capacity panel.
		systemSampler := task.NewSystemSampler(n.Tasks.DB(), n.HostSamplerTick())
		systemSampler.Start(ctx)
		defer systemSampler.Stop()

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
