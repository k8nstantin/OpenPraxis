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
var portalV2Port int

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
		// ServerDeps bundles the cross-cutting services every HTTP route in
		// the web package needs. Both Portal A (Handler) and Portal V2
		// (HandlerV2 in a later commit) take the same struct so the
		// dependency surface stays in lockstep as we add new services.
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

		// --- Portal V2 listener (dual-port architecture) ---
		// Same Go binary, same DB, same backend services — different frontend
		// tree at `internal/web/ui/dashboard-v2/`. Static-only for now (chunk 1
		// plumbing); the real shadcn-admin scaffold lands in a later chunk
		// once we've verified the dual-port pattern.
		var portalV2Server *http.Server
		if portalV2Port > 0 {
			v2Addr := fmt.Sprintf("%s:%d", cfg.Server.Host, portalV2Port)
			portalV2Server = &http.Server{
				Addr:         v2Addr,
				Handler:      web.HandlerV2(deps),
				ReadTimeout:  30 * time.Second,
				WriteTimeout: 30 * time.Second,
			}
			fmt.Printf("  Portal V2:  http://%s:%d\n", cfg.Server.Host, portalV2Port)
			go func() {
				slog.Info("Portal V2 listening", "addr", v2Addr)
				if err := portalV2Server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					slog.Error("Portal V2 failed", "error", err)
				}
			}()
		}

		// Task runner — spawns autonomous agents. RecoverInFlight is the
		// RC/M5 replacement for the blanket CleanupOrphaned sweep: it
		// resolves `on_restart_behavior` per task and honours stop /
		// restart / fail. Must run before the scheduler starts, otherwise
		// a `restart`-eligible orphan races with the first tick. The
		// legacy CleanupOrphaned sweep runs after as a defensive fallback
		// for rows the resolver could not classify (e.g. transient DB
		// lookup errors).

		// Backfill cost_usd for existing runs that have output but no cost recorded
		if backfilled, err := n.Tasks.BackfillCosts(); err == nil && backfilled > 0 {
			slog.Info("backfilled cost data", "runs", backfilled)
		}

		// Post-task subsystem torn out per operator request 2026-04-30.
		// To be redesigned. Only dependent activation remains on
		// task_completed events.
		_ = n.Watcher
		_ = n.Comments

		runner := n.InitRunner(func(event string, data map[string]string) {
			hub.Broadcast(web.Event{Type: event, Data: data})

			if event == "task_completed" {
				go func() {
					time.Sleep(5 * time.Second)
					taskID := data["task_id"]
					status := data["status"]
					if status == "completed" || status == "max_turns" {
						activated, actErr := n.Tasks.ActivateDependents(taskID)
						if actErr != nil {
							slog.Error("activate dependents failed", "task_id", taskID, "error", actErr)
						} else if activated > 0 {
							slog.Info("activated dependent tasks", "task_id", taskID, "count", activated)
						}
					}
				}()
			}
		})

		// RC/M5 orphan recovery: resolve on_restart_behavior per task and
		// flip stuck running/paused rows accordingly. Runs BEFORE the
		// scheduler starts so a restart-eligible orphan is not missed by
		// the first tick. CleanupOrphaned is kept as a defensive fallback
		// for rows the recovery path could not classify.
		if err := runner.RecoverInFlight(ctx); err != nil {
			slog.Warn("recover in-flight tasks failed; falling back to blanket cleanup",
				"error", err)
			n.Tasks.CleanupOrphaned()
		}

		// Task scheduler — initial interval is 10s; the
		// `scheduler_tick_seconds` knob (system scope) overrides this and
		// is re-read on every tick so operator changes take effect live.
		scheduler := task.NewScheduler(n.Tasks, 10*time.Second, func(t *task.Task) {
			slog.Info("task fired", "task_id", t.ID, "title", t.Title, "manifest_id", t.ManifestID, "schedule", t.Schedule)

			// Resolve manifest — standalone tasks have no manifest
			var manifestTitle, manifestContent string
			if t.ManifestID != "" {
				m, _ := n.Manifests.Get(t.ManifestID)
				if m == nil {
					if _, err := n.Tasks.RecordRun(t.ID, "manifest not found: "+t.ManifestID, "failed", 0, 0, 0, 0, time.Now(), task.Usage{}, ""); err != nil {
					slog.Error("record run failed", "reason", "manifest not found", "error", err)
				}
					return
				}
				manifestTitle = m.Title
				manifestContent = m.Content
			} else {
				manifestTitle = t.Title
				manifestContent = t.Description
			}

			// Load visceral rules
			rules, _ := n.Index.ListByType("visceral", 100)
			var visceralText string
			for i, r := range rules {
				visceralText += fmt.Sprintf("%d. [%s] %s\n", i+1, r.ID, r.L2)
			}

			// Spawn the agent
			if err := runner.Execute(t, manifestTitle, manifestContent, visceralText); err != nil {
				if _, recErr := n.Tasks.RecordRun(t.ID, "execution error: "+err.Error(), "failed", 0, 0, 0, 0, time.Now(), task.Usage{}, ""); recErr != nil {
					slog.Error("record run failed", "reason", "execution error", "error", recErr)
				}
				hub.Broadcast(web.Event{Type: "task_failed", Data: map[string]string{
					"task_id": t.ID, "error": err.Error(),
				}})
			}
		}, n)
		scheduler.SetResolver(n.SettingsResolver)
		scheduler.Start()
		defer scheduler.Stop()

		// Schedules consumer — registers every current+enabled row in
		// the schedules table against an in-memory robfig/cron/v3 ticker.
		// Dispatches by entity_kind: today only `task`; product/manifest
		// dispatchers slot in here when their fire semantics are defined.
		// HTTP handlers in handlers_schedule.go call ScheduleRunner.Reload
		// after every Create/Close so the in-memory cron stays in sync.
		scheduleRunner := schedule.NewRunner(n.Schedules, map[string]schedule.DispatchFunc{
			"task": func(ctx context.Context, entityID string, scheduleID int64) error {
				now := time.Now().UTC().Format(time.RFC3339)
				return n.Tasks.UpdateSchedule(entityID, "once", now)
			},
		})
		n.ScheduleRunner = scheduleRunner
		if err := scheduleRunner.Start(ctx); err != nil {
			slog.Error("schedule runner start failed", "error", err)
		}

		// Cross-process action watcher — applies pause/resume/cancel signals
		// written by other binaries (MCP) to tasks this runner owns.
		runner.StartActionWatcher(2 * time.Second)

		// Continuous SystemSampler — writes one row per tick into
		// system_host_samples, independent of any task. Same cadence as
		// the per-run HostSampler (host_sampler_tick_seconds knob, default
		// 5s). Powers the Stats tab System Capacity panel.
		systemSampler := task.NewSystemSampler(n.Tasks.DB(), n.HostSamplerTick())
		systemSampler.Start(ctx)
		defer systemSampler.Stop()

		// --- Startup State Log ---
		fmt.Println("\n  === OpenPraxis State ===")
		fmt.Printf("  Peer:      %s\n", cfg.Node.UUID)
		fmt.Printf("  MAC:       %s\n", cfg.Node.MAC)
		fmt.Printf("  Name:      %s %s\n", cfg.Node.Avatar, cfg.Node.DisplayName)

		// Visceral rules
		if rules, err := n.Index.ListByType("visceral", 100); err == nil {
			fmt.Printf("  Visceral:  %d rules\n", len(rules))
			for i, r := range rules {
				text := r.L2
				if len(text) > 60 { text = text[:60] + "..." }
				fmt.Printf("    %d. [%s] %s\n", i+1, r.ID, text)
			}
		}

		// Active manifests
		if manifests, err := n.Manifests.List("open", 50); err == nil {
			fmt.Printf("  Manifests: %d active\n", len(manifests))
			for _, m := range manifests {
				fmt.Printf("    [%s] %s\n", m.ID, m.Title)
			}
		}

		// Pending tasks
		if tasks, err := n.Tasks.List("", 100); err == nil {
			running := 0; scheduled := 0; waiting := 0; completed := 0
			for _, t := range tasks {
				switch t.Status {
				case "running": running++
				case "scheduled": scheduled++
				case "waiting": waiting++
				case "completed": completed++
				}
			}
			fmt.Printf("  Tasks:     %d total (%d running, %d scheduled, %d waiting, %d completed)\n",
				len(tasks), running, scheduled, waiting, completed)
		}

		// Top memories
		if mems, err := n.Index.ListByPrefix("/", 5); err == nil {
			fmt.Printf("  Memories:  latest %d\n", len(mems))
			for _, m := range mems {
				text := m.L0
				if len(text) > 60 { text = text[:60] + "..." }
				fmt.Printf("    [%s] %s\n", m.ID, text)
			}
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
		if portalV2Server != nil {
			portalV2Server.Shutdown(shutdownCtx)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
	serveCmd.Flags().BoolVar(&noBrowser, "no-browser", false, "Don't open the dashboard in the browser")
	// Portal V2 is the redesigned operator dashboard built fresh on shadcn-admin
	// in `internal/web/ui/dashboard-v2/`. Default :9766 mirrors Portal A's :8765
	// with the leading 8→9 swap (Portal A on 8, Portal V2 on 9, same trailing
	// digits as the sync :8766 cluster). Set 0 to disable while the v2 work is
	// in flight.
	serveCmd.Flags().IntVar(&portalV2Port, "portal-v2-port", 9766, "Portal V2 listener port (0 to disable)")
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
