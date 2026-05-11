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
	"strings"
	"syscall"
	"time"

	"github.com/k8nstantin/OpenPraxis/internal/chat"
	"github.com/k8nstantin/OpenPraxis/internal/comments"
	execution "github.com/k8nstantin/OpenPraxis/internal/execution"
	"github.com/k8nstantin/OpenPraxis/internal/relationships"
	"github.com/k8nstantin/OpenPraxis/internal/settings"
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

// deriveBranchName converts an entity title to a valid git branch suffix.
// "Turn-Level Agent Events" → "openpraxis/turn-level-agent-events"
func deriveBranchName(prefix, title string) string {
	s := strings.ToLower(title)
	s = strings.ReplaceAll(s, " ", "-")
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	slug := b.String()
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	slug = strings.Trim(slug, "-")
	return prefix + "/" + slug
}

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
		//
		// dispatchChainFn is set after dispatchAtomicTasks is defined so the
		// task_completed callback can chain the next DAG-ready task without a
		// forward-declaration problem.
		var dispatchChainFn func(ctx context.Context, parentID string, scheduleID int64) error

		runner := n.InitRunner(func(event string, data map[string]string) {
			hub.Broadcast(web.Event{Type: event, Data: data})
			// When a task completes successfully, re-evaluate the owning manifest
			// so the next depends_on-unblocked task fires automatically.
			// Only advance on status=completed — a failed or cancelled task must
			// not re-trigger the chain or the manifest re-dispatches T1 in a loop.
			if event == task.EventTaskCompleted && data[task.EventKeyStatus] == execution.EventCompleted {
				taskID := data[task.EventKeyTaskID]
				if taskID != "" && n.Relationships != nil && dispatchChainFn != nil {
					edges, _ := n.Relationships.ListIncoming(ctx, taskID, relationships.EdgeOwns)
					for _, e := range edges {
						if e.SrcKind == relationships.KindManifest || e.SrcKind == relationships.KindProduct {
							parentID := e.SrcID
							go func() {
								if err := dispatchChainFn(ctx, parentID, 0); err != nil {
									slog.Info("dag-chain: dispatch after task_completed", "parent", parentID[:12], "note", err.Error())
								}
								// After dispatching within the manifest, check if the
								// manifest itself is now fully done (all owned tasks
								// completed). If so, fire any manifests that depend on
								// it via manifest-level depends_on edges.
								if e.SrcKind != relationships.KindManifest || n.ExecutionLog == nil {
									return
								}
								manifestID := e.SrcID
								ownedTasks, _ := n.Relationships.ListOutgoing(ctx, manifestID, relationships.EdgeOwns)
								allDone := len(ownedTasks) > 0
								for _, ot := range ownedTasks {
									if ot.DstKind != relationships.KindTask {
										continue
									}
									done, _ := n.ExecutionLog.HasCompleted(ctx, ot.DstID)
									if !done {
										allDone = false
										break
									}
								}
								if !allDone {
									return
								}
								// All tasks complete — fire downstream manifests.
								downstreams, _ := n.Relationships.ListIncoming(ctx, manifestID, relationships.EdgeDependsOn)
								for _, ds := range downstreams {
									if ds.SrcKind != relationships.KindManifest {
										continue
									}
									dsID := ds.SrcID
									slog.Info("dag-chain: manifest complete — firing downstream manifest", "done", manifestID[:12], "next", dsID[:12])
									if err := dispatchChainFn(ctx, dsID, 0); err != nil {
										slog.Info("dag-chain: downstream manifest dispatch", "manifest", dsID[:12], "note", err.Error())
									}
								}
							}()
							break
						}
					}
				}
			}
		})

		if err := runner.RecoverInFlight(ctx); err != nil {
			slog.Warn("recover in-flight tasks failed", "error", err)
		}

		// DAG chain recovery: on startup, re-evaluate every active manifest so
		// chains interrupted by a server restart resume from where they left off.
		// dispatchChainFn is not wired yet at this point — schedule this after
		// the runner and dispatcher are fully initialised by deferring into a
		// short-lived goroutine that waits for the wiring to complete.
		go func() {
			// Brief pause to let dispatchChainFn get wired below.
			time.Sleep(2 * time.Second)
			if n.Relationships == nil || n.Entities == nil {
				return
			}
			manifests, err := n.Entities.List(relationships.KindManifest, "active", 0)
			if err != nil || len(manifests) == 0 {
				return
			}
			// Resolve recovery window from dag_chain_recovery_window_minutes (default 60).
			// 0 disables chain recovery entirely.
			windowMinutes := 60
			if n.SettingsResolver != nil {
				if resolved, err := n.SettingsResolver.Resolve(ctx, settings.Scope{}, "dag_chain_recovery_window_minutes"); err == nil {
					if v, ok := resolved.Value.(int64); ok && v >= 0 {
						windowMinutes = int(v)
					}
				}
			}
			if windowMinutes == 0 {
				return
			}
			recoverySince := time.Now().Add(-time.Duration(windowMinutes) * time.Minute)
			resumed := 0
			for _, m := range manifests {
				edges, _ := n.Relationships.ListOutgoing(ctx, m.EntityUID, relationships.EdgeOwns)
				hasRecentlyCompleted, hasPending := false, false
				for _, e := range edges {
					if e.DstKind != relationships.KindTask {
						continue
					}
					recent, _ := n.ExecutionLog.HasCompletedSince(ctx, e.DstID, recoverySince)
					if recent {
						hasRecentlyCompleted = true
					} else {
						done, _ := n.ExecutionLog.HasCompleted(ctx, e.DstID)
						if !done {
							hasPending = true
						}
					}
				}
				if hasRecentlyCompleted && hasPending && dispatchChainFn != nil {
					slog.Info("dag-chain: resuming interrupted chain on startup", "manifest", m.EntityUID[:12], "title", m.Title)
					if err := dispatchChainFn(ctx, m.EntityUID, 0); err != nil {
						slog.Info("dag-chain: resume dispatch", "manifest", m.EntityUID[:12], "note", err.Error())
					}
					resumed++
				}
			}
			if resumed > 0 {
				slog.Info("dag-chain: resumed chains on startup", "count", resumed)
			}
		}()

		// defaultSkillID is the fallback execution protocol used when an entity
		// has no skill assigned via the DAG.
		const defaultSkillID = "019dfecc-a7b3-78a6-acc0-1cfa638eae18"

		// skillPromptFor walks UP the DAG from entityID and collects ALL skill
		// entities found (skill → owns → product). All skills are concatenated
		// in order so every skill assigned to the entity's product is loaded.
		// Falls back to the default skill if none found in the DAG.
		skillPromptFor := func(ctx context.Context, entityID string) (string, string) {
			if n.Relationships == nil {
				return defaultSkillID, ""
			}

			ct := comments.TypePrompt
			var skillIDs []string
			var combined strings.Builder

			// Walk UP through the ownership chain collecting ALL skills.
			// Limit to 4 hops: task → manifest → product → skill.
			current := entityID
			for hop := 0; hop < 4; hop++ {
				edges, _ := n.Relationships.ListIncoming(ctx, current, relationships.EdgeOwns)
				for _, edge := range edges {
					if edge.SrcKind == relationships.KindSkill {
						// Collect all skills — do not stop at the first one.
						revs, err := n.Comments.List(ctx, comments.TargetEntity, edge.SrcID, 1, &ct)
						if err == nil && len(revs) > 0 && revs[0].Body != "" {
							skillIDs = append(skillIDs, edge.SrcID)
							if combined.Len() > 0 {
								combined.WriteString("\n\n---\n\n")
							}
							combined.WriteString(revs[0].Body)
						}
					} else {
						current = edge.SrcID
					}
				}
				if len(edges) == 0 {
					break
				}
			}

			if combined.Len() > 0 {
				slog.Info("skills loaded from DAG", "entity_uid", entityID[:12], "skills", len(skillIDs))
				return strings.Join(skillIDs, ","), combined.String()
			}

			// No skill found in DAG — use the default execution protocol.
			revs, err := n.Comments.List(ctx, comments.TargetEntity, defaultSkillID, 1, &ct)
			if err == nil && len(revs) > 0 && revs[0].Body != "" {
				return defaultSkillID, revs[0].Body
			}

			slog.Warn("no skill found in DAG and default has no prompt — using inline fallback",
				"entity_uid", entityID[:12])
			return defaultSkillID, "You are an experienced software engineer executing a scheduled OpenPraxis entity.\n\n" +
				"Entity UUID: {ENTITY_UUID}\n\n" +
				"Read the entity prompt and comments via the API, assemble full DAG context, execute, report back."
		}

		slog.Info("skill routing: dynamic DAG lookup active", "default_skill", defaultSkillID[:12])

		// visceralRules loads all visceral rules into a numbered string.
		visceralRules := func() string {
			rules, _ := n.Index.ListByType("visceral", 100)
			var sb strings.Builder
			for i, r := range rules {
				sb.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, r.ID, r.L2))
			}
			return sb.String()
		}

		// dispatch is the universal handler for every scheduled entity kind.
		// It injects the entity UUID into the execution skill prompt and fires
		// one agent. The agent owns full DAG traversal — it reads prompts and
		// comments at every level via the HTTP API and assembles its own context.
		dispatch := func(ctx context.Context, entityID string, scheduleID int64) error {
			// System-level concurrency guard: only one agent at a time.
			if runner.RunningCount() > 0 {
				running := runner.ListRunning()
				titles := make([]string, 0, len(running))
				for _, rt := range running {
					titles = append(titles, rt.Title)
				}
				return fmt.Errorf("agent already running (%v) — refusing to start %s concurrently", titles, entityID)
			}
			// Check execution_log for in-flight runs from previous sessions.
			if n.ExecutionLog != nil {
				inFlight, err := n.ExecutionLog.ListInFlight(ctx)
				if err != nil {
					slog.Warn("concurrency guard: could not list in-flight runs", "error", err)
				}
				for _, ifr := range inFlight {
					alive := false
					if ifr.AgentPID > 0 {
						if proc, err := os.FindProcess(ifr.AgentPID); err == nil {
							if err := proc.Signal(syscall.Signal(0)); err == nil {
								alive = true
							}
						}
					}
					if alive {
						return fmt.Errorf("agent PID %d still running (run %s) — refusing to start %s concurrently",
							ifr.AgentPID, ifr.RunUID[:12], entityID)
					}
					slog.Info("concurrency guard: closing zombie run", "run_uid", ifr.RunUID[:12], "pid", ifr.AgentPID)
					_ = n.ExecutionLog.MarkFailed(ctx, ifr.RunUID, ifr.EntityUID, "process-not-found")
				}
			}

			e, err := n.Entities.Get(entityID)
			if err != nil || e == nil {
				return fmt.Errorf("entity not found: %s", entityID)
			}

			// Resolve skill: walk UP the DAG to find the nearest assigned skill,
			// fall back to the default execution protocol.
			skillID, skillBody := skillPromptFor(ctx, entityID)
			prompt := strings.ReplaceAll(skillBody, "{ENTITY_UUID}", entityID)

			// Resolve branch_strategy knob — task (default), manifest, or product.
			// The settings resolver walks task → manifest → product → system so
			// setting it at any scope level cascades down to all children.
			if n.SettingsResolver != nil {
				scope := settings.Scope{TaskID: entityID}
				if res, err := n.SettingsResolver.Resolve(ctx, scope, "branch_strategy"); err == nil {
					strategy, _ := res.Value.(string)
					// Normalise: any unrecognised strategy is treated like "product" so
					// custom entity types work without requiring a server code change.
					if strategy != relationships.KindManifest && strategy != relationships.KindProduct && strategy != "" {
						strategy = relationships.KindProduct
					}
					if strategy == relationships.KindManifest || strategy == relationships.KindProduct {
						// Derive the shared branch name from the owning entity's title.
						sharedBranch := ""
						branchPrefix := "openpraxis"
						if bpRes, err2 := n.SettingsResolver.Resolve(ctx, scope, "branch_prefix"); err2 == nil {
							if bp, _ := bpRes.Value.(string); bp != "" {
								branchPrefix = bp
							}
						}
						branchRemote := "github"
						if brRes, err2 := n.SettingsResolver.Resolve(ctx, scope, "branch_remote"); err2 == nil {
							if br, _ := brRes.Value.(string); br != "" {
								branchRemote = br
							}
						}
						if n.Relationships != nil {
							// Walk up to find the entity whose title drives the branch name.
							lookupID := entityID
							if strategy == relationships.KindManifest {
								if edges, _ := n.Relationships.ListIncoming(ctx, lookupID, relationships.EdgeOwns); len(edges) > 0 {
									for _, edge := range edges {
										if edge.SrcKind == relationships.KindManifest {
											if me, _ := n.Entities.Get(edge.SrcID); me != nil {
												sharedBranch = deriveBranchName(branchPrefix, me.Title)
											}
											break
										}
									}
								}
							} else { // product (or normalised-to-product) — walk up the owns chain
								if manifEdges, _ := n.Relationships.ListIncoming(ctx, lookupID, relationships.EdgeOwns); len(manifEdges) > 0 {
									for _, me := range manifEdges {
										if me.SrcKind == relationships.KindManifest {
											if prodEdges, _ := n.Relationships.ListIncoming(ctx, me.SrcID, relationships.EdgeOwns); len(prodEdges) > 0 {
												for _, pe := range prodEdges {
													if pe.SrcKind == relationships.KindProduct {
														if pe2, _ := n.Entities.Get(pe.SrcID); pe2 != nil {
															sharedBranch = deriveBranchName(branchPrefix, pe2.Title)
														}
														break
													}
												}
											}
											break
										}
									}
								}
							}
						}
						if sharedBranch != "" {
							prompt += fmt.Sprintf(
								"\n\n## Shared Branch (branch_strategy=%s)\n"+
									"Use branch `%s` — do NOT create a new branch or PR.\n"+
									"```bash\ngit fetch %s\n"+
									"git checkout %s 2>/dev/null || git checkout -b %s --track %s/%s\n```\n"+
									"Push all commits to `%s`.\n",
								strategy, sharedBranch, branchRemote, sharedBranch, sharedBranch, branchRemote, sharedBranch, sharedBranch)
							slog.Info("dispatch: shared branch", "entity_uid", entityID[:12], "branch", sharedBranch, "strategy", strategy)
						}
					}
				}
			}

			// Append visceral rules so the agent receives them even without MCP.
			if vr := visceralRules(); vr != "" {
				prompt += "\n\n---\n\n## Visceral Rules\n" + vr
			}

			slog.Info("schedule: dispatching entity",
				"entity_uid", entityID, "type", e.Type, "title", e.Title,
				"skill_id", skillID[:12])

			t := &task.Task{
				ID:     entityID,
				Title:  e.Title,
				Status: "scheduled",
				Agent:  "claude-code",
			}
			return runner.Execute(t, e.Title, prompt, visceralRules())
		}

		// dispatchAtomicTasks fires every non-archived task under a manifest or product
		// individually so each gets its own execution_log entry at the task level.
		// Tasks are the atomic execution unit — run data must aggregate from task → manifest → product,
		// not be recorded at the manifest/product level directly.
		dispatchAtomicTasks := func(ctx context.Context, parentID string, scheduleID int64) error {
			if n.Relationships == nil {
				return fmt.Errorf("relationships store not wired")
			}
			parent, err := n.Entities.Get(parentID)
			if err != nil || parent == nil {
				return fmt.Errorf("entity not found: %s", parentID)
			}

			// Collect all tasks under this entity by recursively walking owns edges.
			// Any entity type that owns tasks (directly or transitively) is handled
			// uniformly — no hardcoded type checks.
			var taskIDs []string
			var collectAll func(id string)
			collectAll = func(id string) {
				edges, _ := n.Relationships.ListOutgoing(ctx, id, relationships.EdgeOwns)
				for _, edge := range edges {
					if edge.DstKind == relationships.KindTask {
						if te, _ := n.Entities.Get(edge.DstID); te != nil &&
							te.Status != "archived" && te.Status != "closed" {
							taskIDs = append(taskIDs, edge.DstID)
						}
					} else {
						// non-task owned entity — recurse
						collectAll(edge.DstID)
					}
				}
			}
			collectAll(parentID)

			if len(taskIDs) == 0 {
				slog.Info("dispatch: no active tasks found", "parent_uid", parentID[:12], "type", parent.Type)
				return nil
			}

			slog.Info("dispatch: firing tasks atomically", "parent_uid", parentID[:12], "type", parent.Type, "task_count", len(taskIDs))
			var lastErr error
			for _, taskID := range taskIDs {
				// DAG gate: skip tasks whose depends_on edges point to tasks
				// that have not yet completed. Reads the relationships table
				// (single source of truth) and uses HasCompleted — a targeted
				// query for the completed event rather than the most-recent-row
				// heuristic which could return a sample row and false-negative.
				if n.Relationships != nil && n.ExecutionLog != nil {
					// Skip tasks that already completed — prevents re-firing
					// tasks with no deps (they'd pass the gate every time).
					if done, _ := n.ExecutionLog.HasCompleted(ctx, taskID); done {
						slog.Info("dag-chain: task already completed — skipping", "task_id", taskID[:12])
						continue
					}
					deps, _ := n.Relationships.ListOutgoing(ctx, taskID, relationships.EdgeDependsOn)
					blocked := false
					for _, dep := range deps {
						done, _ := n.ExecutionLog.HasCompleted(ctx, dep.DstID)
						if !done {
							blocked = true
							break
						}
					}
					if blocked {
						slog.Info("dag-chain: task blocked by unsatisfied depends_on — skipping", "task_id", taskID[:12])
						continue
					}
				}
				if err := dispatch(ctx, taskID, scheduleID); err != nil {
					slog.Error("dispatch: task failed", "task_id", taskID[:12], "error", err)
					lastErr = err
				}
			}
			return lastErr
		}

		// Wire the chain function now that dispatchAtomicTasks is defined.
		dispatchChainFn = dispatchAtomicTasks

		// Schedule dispatcher:
		//   task     → dispatch directly (atomic, execution recorded at task level)
		//   manifest → fire each owned task individually (execution recorded per task)
		//   product  → fire each task across all manifests individually
		//   skill    → fire each task across all owned products
		//   *        → fallback for any entity type added via the entity_types
		//              table at runtime; dispatches the same way as manifest/product
		scheduleRunner := schedule.NewRunner(n.Schedules, map[string]schedule.DispatchFunc{
			relationships.KindTask:     dispatch,
			relationships.KindManifest: dispatchAtomicTasks,
			relationships.KindProduct:  dispatchAtomicTasks,
			relationships.KindSkill:    dispatchAtomicTasks,
			"*":                        dispatchAtomicTasks,
		})
		n.ScheduleRunner = scheduleRunner
		if err := scheduleRunner.Start(ctx); err != nil {
			slog.Error("schedule runner start failed", "error", err)
		}

		// --- Startup state summary ---
		fmt.Println("\n  === OpenPraxis State ===")
		fmt.Printf("  Peer:      %s\n", cfg.Node.UUID)
		fmt.Printf("  Name:      %s %s\n", cfg.Node.Avatar, cfg.Node.DisplayName)

		if rules, err := n.Index.ListByType("visceral", 100); err == nil && len(rules) > 0 {
			fmt.Printf("  Visceral:  %d rules\n", len(rules))
		}
		if active, err := n.Entities.List(relationships.KindTask, "active", 0); err == nil {
			fmt.Printf("  Tasks:     %d active\n", len(active))
		}
		if manifests, err := n.Entities.List(relationships.KindManifest, "active", 0); err == nil {
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
