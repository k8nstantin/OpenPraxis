package node

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/k8nstantin/OpenPraxis/internal/action"
	"github.com/k8nstantin/OpenPraxis/internal/chat"
	"github.com/k8nstantin/OpenPraxis/internal/comments"
	"github.com/k8nstantin/OpenPraxis/internal/config"
	"github.com/k8nstantin/OpenPraxis/internal/conversation"
	"github.com/k8nstantin/OpenPraxis/internal/embedding"
	"github.com/k8nstantin/OpenPraxis/internal/idea"
	"github.com/k8nstantin/OpenPraxis/internal/manifest"
	"github.com/k8nstantin/OpenPraxis/internal/marker"
	"github.com/k8nstantin/OpenPraxis/internal/memory"
	"github.com/k8nstantin/OpenPraxis/internal/product"
	"github.com/k8nstantin/OpenPraxis/internal/relationships"
	"github.com/k8nstantin/OpenPraxis/internal/settings"
	"github.com/k8nstantin/OpenPraxis/internal/task"
	"github.com/k8nstantin/OpenPraxis/internal/templates"
	"github.com/k8nstantin/OpenPraxis/internal/watcher"
)

// Node is the central orchestrator that wires all components together.
type Node struct {
	Config           *config.Config
	Store            *memory.Store
	Index            *memory.Index
	Conversations    *conversation.Store
	Markers          *marker.Store
	Actions          *action.Store
	Products         *product.Store
	Manifests        *manifest.Store
	Ideas            *idea.Store
	Tasks            *task.Store
	ChatSessions     *chat.SessionStore
	Watcher          *watcher.Store
	SettingsStore    *settings.Store
	SettingsResolver *settings.Resolver
	Templates        *templates.Store
	TemplatesResolv  *templates.Resolver
	Comments         *comments.Store
	// Relationships is the unified edge store (Praxis Relationships
	// PR/M1). Lives alongside the existing dep tables during the M2
	// dual-write phase; becomes the sole source after M3 cutover.
	Relationships    *relationships.Store
	runner           *task.Runner
	hostSampler      *task.HostSampler
	Embedder         *embedding.Engine
	StartedAt        time.Time
}

// New creates and initializes a Node.
func New(cfg *config.Config) (*Node, error) {
	store, err := memory.NewStore(cfg.Storage.DataDir)
	if err != nil {
		return nil, fmt.Errorf("init store: %w", err)
	}

	index, err := memory.NewIndex(cfg.Storage.DataDir, cfg.Embedding.Dimension)
	if err != nil {
		return nil, fmt.Errorf("init index: %w", err)
	}

	// Share the same SQLite DB for conversations and markers
	convStore, err := conversation.NewStore(index.DB(), cfg.Embedding.Dimension)
	if err != nil {
		return nil, fmt.Errorf("init conversation store: %w", err)
	}

	markerStore, err := marker.NewStore(index.DB())
	if err != nil {
		return nil, fmt.Errorf("init marker store: %w", err)
	}

	actionStore, err := action.NewStore(index.DB())
	if err != nil {
		return nil, fmt.Errorf("init action store: %w", err)
	}

	ideaStore, err := idea.NewStore(index.DB())
	if err != nil {
		return nil, fmt.Errorf("init idea store: %w", err)
	}

	productStore, err := product.NewStore(index.DB())
	if err != nil {
		return nil, fmt.Errorf("init product store: %w", err)
	}

	manifestStore, err := manifest.NewStore(index.DB())
	if err != nil {
		return nil, fmt.Errorf("init manifest store: %w", err)
	}

	taskStore, err := task.NewStore(index.DB())
	if err != nil {
		return nil, fmt.Errorf("init task store: %w", err)
	}
	// Wire the manifest store as the readiness checker so task.Create
	// seeds tasks in 'waiting' when their manifest has unsatisfied deps.
	// Typed as task.ManifestReadinessChecker — manifest.Store satisfies
	// the interface via IsSatisfied(ctx, manifestID).
	taskStore.SetManifestChecker(manifestStore)
	// Product-tier readiness check. product.Store satisfies
	// task.ProductReadinessChecker via IsSatisfied. A task whose
	// containing manifest belongs to a product with open product-level
	// deps gets seeded in 'waiting' with a product-prefix block_reason,
	// distinct from manifest- and task-level blockers so each tier's
	// propagation walker only flips its own tasks.
	taskStore.SetProductChecker(productStore)

	// Review comments wiring (#93). comments.Store is the durable
	// home for review_rejection + review_approval comments; task.Store
	// calls through these adapters so it doesn't take an
	// internal/comments import. Review reads + writes flow through
	// the same underlying store; the interface split exists for
	// testability (stub either side independently).
	commentsStore := comments.NewStore(index.DB())
	taskStore.SetReviewCommentsAPI(
		&taskReviewWriteAdapter{s: commentsStore},
		&taskReviewReadAdapter{s: commentsStore},
	)

	// Wire the terminal-transition handler: when a manifest moves from
	// non-terminal (draft/open) → terminal (closed/archive), walk its
	// dependents and activate waiting tasks in any newly-satisfied
	// downstream manifest. This is what makes the dependency chain
	// self-advance without an operator-in-the-loop — the core thesis
	// of autonomous-agent workstreams.
	//
	// Both closures are injected (rather than imported) so task
	// doesn't depend on manifest, preserving the one-way package
	// direction. depsFor flattens ListDependents' Dep rows to id
	// strings; satisfiedFor drops the blocker list the core walker
	// doesn't need.
	// Dep removal: when an operator removes a manifest→manifest edge,
	// any waiting tasks in the source manifest that were blocked by
	// a manifest-level dep are rehabbed to 'pending' — Option B, per
	// issue #74 design comment. Pending (not scheduled) keeps an
	// accidental click from auto-spending; operator arms explicitly.
	// We only flip when the manifest is now fully satisfied; if other
	// deps still block it, waiting tasks stay put.
	manifestStore.SetDepRemovedHandler(func(ctx context.Context, manifestID string) {
		ok, _, err := manifestStore.IsSatisfied(ctx, manifestID)
		if err != nil || !ok {
			return
		}
		_, _ = taskStore.FlipManifestBlockedTasks(ctx, manifestID, task.StatusPending)
	})

	manifestStore.SetTerminalTransitionHandler(func(ctx context.Context, manifestID string) {
		depsFor := func(ctx context.Context, m string) ([]string, error) {
			deps, err := manifestStore.ListDependents(ctx, m)
			if err != nil {
				return nil, err
			}
			ids := make([]string, 0, len(deps))
			for _, d := range deps {
				ids = append(ids, d.ID)
			}
			return ids, nil
		}
		satisfiedFor := func(ctx context.Context, m string) (bool, error) {
			ok, _, err := manifestStore.IsSatisfied(ctx, m)
			return ok, err
		}
		if _, err := taskStore.PropagateManifestClosed(ctx, manifestID, depsFor, satisfiedFor); err != nil {
			// Log but don't surface — the manifest close already
			// succeeded; activation failure is recoverable by firing
			// tasks manually or by the next close retriggering.
			_ = err // keep the handler pure; no slog here to avoid import churn in node.go
		}
	})

	// Product-tier dep removal rehab (symmetric to manifest above).
	// When an operator removes a product dep, if the product is now
	// satisfied, flip its product-blocked tasks from waiting → pending
	// (Option B — operator arms manually so an accidental click
	// doesn't auto-spend).
	productStore.SetDepRemovedHandler(func(ctx context.Context, productID string) {
		ok, _, err := productStore.IsSatisfied(ctx, productID)
		if err != nil || !ok {
			return
		}
		_, _ = taskStore.FlipProductBlockedTasks(ctx, productID, task.StatusPending)
	})

	// Product close propagation. Closing a product walks its
	// dependents; for each newly-satisfied dependent product, flip
	// its product-blocked tasks to scheduled so the chain self-
	// advances.
	productStore.SetTerminalTransitionHandler(func(ctx context.Context, productID string) {
		depsFor := func(ctx context.Context, pid string) ([]string, error) {
			deps, err := productStore.ListDependents(ctx, pid)
			if err != nil {
				return nil, err
			}
			ids := make([]string, 0, len(deps))
			for _, d := range deps {
				ids = append(ids, d.ID)
			}
			return ids, nil
		}
		satisfiedFor := func(ctx context.Context, pid string) (bool, error) {
			ok, _, err := productStore.IsSatisfied(ctx, pid)
			return ok, err
		}
		if _, err := taskStore.PropagateProductClosed(ctx, productID, depsFor, satisfiedFor); err != nil {
			_ = err
		}
	})

	chatStore, err := chat.NewSessionStore(index.DB())
	if err != nil {
		return nil, fmt.Errorf("init chat store: %w", err)
	}

	watcherStore, err := watcher.NewStore(index.DB())
	if err != nil {
		return nil, fmt.Errorf("init watcher store: %w", err)
	}

	if err := settings.InitSchema(index.DB()); err != nil {
		return nil, fmt.Errorf("init settings schema: %w", err)
	}

	if err := comments.InitSchema(index.DB()); err != nil {
		return nil, fmt.Errorf("comments.InitSchema: %w", err)
	}

	settingsStore := settings.NewStore(index.DB())
	taskSettingsAdapter := &task.SettingsAdapter{Store: taskStore}
	manifestSettingsAdapter := &manifest.SettingsAdapter{Store: manifestStore}
	settingsResolver := settings.NewResolver(settingsStore, taskSettingsAdapter, manifestSettingsAdapter)

	// RC/M1: prompt_templates substrate. Schema + system seed run every
	// boot; both are idempotent. Resolver walks task → manifest → product
	// → agent → system using a task-scope lookup into the task store.
	if err := templates.InitSchema(index.DB()); err != nil {
		return nil, fmt.Errorf("init templates schema: %w", err)
	}
	templatesStore := templates.NewStore(index.DB())
	templatesResolver := templates.NewResolver(templatesStore, &taskTemplatesScopeAdapter{tasks: taskStore, manifests: manifestStore}, nil)

	// M4-T14: one-time migration of legacy tasks.max_turns column values into
	// settings rows at task scope, followed by dropping the column. Both are
	// idempotent — the migration is gated by a marker row; the drop is a
	// no-op when the column is already absent. Must run before any code
	// path (scanTask, taskColumns) that assumes the column is gone.
	if _, err := task.MigrateMaxTurnsToSettings(index.DB(), settingsStore); err != nil {
		return nil, fmt.Errorf("migrate max_turns to settings: %w", err)
	}
	if err := task.DropMaxTurnsColumn(index.DB()); err != nil {
		return nil, fmt.Errorf("drop max_turns column: %w", err)
	}

	embedder := embedding.NewEngine(cfg.Embedding.OllamaURL, cfg.Embedding.Model, cfg.Embedding.Dimension)

	// Praxis Relationships PR/M1 — unified edge store. Migration is
	// idempotent (CREATE TABLE / CREATE INDEX IF NOT EXISTS). Sharing
	// the same DB handle as every other store so it lives in
	// memories.db alongside products / manifests / tasks etc.
	relationshipsStore, err := relationships.New(index.DB())
	if err != nil {
		return nil, fmt.Errorf("init relationships store: %w", err)
	}

	n := &Node{
		Config:           cfg,
		Store:            store,
		Index:            index,
		Conversations:    convStore,
		Markers:          markerStore,
		Actions:          actionStore,
		Products:         productStore,
		Manifests:        manifestStore,
		Ideas:            ideaStore,
		Tasks:            taskStore,
		ChatSessions:     chatStore,
		Watcher:          watcherStore,
		SettingsStore:    settingsStore,
		SettingsResolver: settingsResolver,
		Templates:        templatesStore,
		TemplatesResolv:  templatesResolver,
		Comments:         commentsStore,
		Relationships:    relationshipsStore,
		Embedder:         embedder,
		StartedAt:        time.Now(),
	}

	// One-time migration: normalize source_node from hostname to UUID
	n.migrateSourceNodeToUUID()

	// RC/M1: seed the prompt_templates system rows on first boot.
	// Idempotent — no-op if any system-scope row already exists.
	if err := templates.Seed(context.Background(), templatesStore, n.PeerID()); err != nil {
		return nil, fmt.Errorf("seed prompt templates: %w", err)
	}

	return n, nil
}

// taskTemplatesScopeAdapter translates a task id into its manifest and
// product ids for the templates resolver. Kept here (not in the
// templates package) so templates stays free of a task/manifest import.
type taskTemplatesScopeAdapter struct {
	tasks     *task.Store
	manifests *manifest.Store
}

func (a *taskTemplatesScopeAdapter) ManifestAndProductForTask(ctx context.Context, taskID string) (string, string, error) {
	if a.tasks == nil {
		return "", "", nil
	}
	t, err := a.tasks.Get(taskID)
	if err != nil || t == nil {
		return "", "", nil
	}
	manifestID := t.ManifestID
	var productID string
	if manifestID != "" && a.manifests != nil {
		m, err := a.manifests.Get(manifestID)
		if err == nil && m != nil {
			productID = m.ProjectID
		}
	}
	return manifestID, productID, nil
}

// InitRunner creates and sets the task Runner using the Node's own stores.
// Must be called after New() and before serving requests.
// The Runner reads its max_parallel cap per task via n.SettingsResolver —
// there is no process-wide cap argument because caps are now per-product.
func (n *Node) InitRunner(onEvent func(string, map[string]string)) *task.Runner {
	// Empty repoDir → Runner falls back to process CWD at spawn time.
	// cmd/serve.go passes an explicit dir via a follow-up SetRepoDir if it
	// runs from outside the repo root.
	n.runner = task.NewRunner(n.Tasks, n.Actions, n.SettingsResolver, "", onEvent)
	// Wire the post-completion execution_review gate (M4-T10). Non-fatal
	// if Comments is nil — the runner treats nil as "feature off".
	if n.Comments != nil {
		n.runner.SetExecutionReviewChecker(&executionReviewCheckerAdapter{s: n.Comments})
	}
	// RC/M1: hand the prompt_templates resolver to the runner so
	// buildPrompt walks scope tiers instead of using the in-code
	// defaults. Nil is safe — the runner falls back to the package
	// defaults for any section the resolver doesn't answer.
	if n.TemplatesResolv != nil {
		n.runner.SetTemplateResolver(n.TemplatesResolv)
	}
	// Host-metrics sampler: polls the serve process's CPU/RSS every
	// 5 seconds and attributes each sample to every currently-running
	// task. Data lands on task_run_host_samples for the Run Stats
	// card overlay + on task_runs rollup columns for list views.
	// The sampler is a single shared instance on the node — starting
	// multiple runners is not supported, so one-at-boot is fine.
	n.hostSampler = task.NewHostSampler(5 * time.Second)
	n.hostSampler.Start(context.Background())
	n.runner.SetHostSampler(n.hostSampler)
	return n.runner
}

// GetRunner returns the task Runner, or nil if not yet initialized.
func (n *Node) GetRunner() *task.Runner {
	return n.runner
}

// StoreMemory stores a memory: writes to CRDT, embeds, indexes.
func (n *Node) StoreMemory(ctx context.Context, content, path, memType, scope, project, domain, sourceAgent string, tags []string) (*memory.Memory, error) {
	if project == "" {
		project = n.Config.Defaults.Project
	}
	if scope == "" {
		scope = n.Config.Defaults.Scope
	}

	mem, err := memory.NewMemory(content, path, memType, scope, project, domain, sourceAgent, n.PeerID(), tags)
	if err != nil {
		return nil, fmt.Errorf("create memory: %w", err)
	}

	vec, err := n.Embedder.EmbedDocument(ctx, mem.L1)
	if err != nil {
		return nil, fmt.Errorf("embed: %w", err)
	}

	if err := n.Store.Put(mem); err != nil {
		return nil, fmt.Errorf("store: %w", err)
	}

	if err := n.Index.Upsert(mem, vec); err != nil {
		return nil, fmt.Errorf("index: %w", err)
	}

	return mem, nil
}

// SaveConversation saves a full agent conversation with embedding.
func (n *Node) SaveConversation(ctx context.Context, title, agent, project string, turns []conversation.Turn, tags []string) (*conversation.Conversation, error) {
	if project == "" {
		project = n.Config.Defaults.Project
	}

	conv := conversation.NewConversation(title, agent, project, n.PeerID(), turns, tags)

	// Embed the summary for semantic search
	vec, err := n.Embedder.EmbedDocument(ctx, conv.Summary)
	if err != nil {
		return nil, fmt.Errorf("embed conversation: %w", err)
	}

	if err := n.Conversations.Save(conv, vec); err != nil {
		return nil, fmt.Errorf("save conversation: %w", err)
	}

	return conv, nil
}

// UpdateConversation updates an existing conversation with new turns and re-embeds.
func (n *Node) UpdateConversation(ctx context.Context, id, title, agent, project string, turns []conversation.Turn) error {
	if project == "" {
		project = n.Config.Defaults.Project
	}

	now := time.Now()
	// Preserve original created_at if conversation already exists
	createdAt := now
	if existing, _ := n.Conversations.GetByID(id); existing != nil {
		createdAt = existing.CreatedAt
	}
	conv := &conversation.Conversation{
		ID:         id,
		Title:      title,
		Agent:      agent,
		Project:    project,
		SourceNode: n.PeerID(),
		Turns:      turns,
		TurnCount:  len(turns),
		CreatedAt:  createdAt,
		UpdatedAt:  now,
		AccessedAt: now,
	}
	conv.Summary = conversation.BuildSummary(turns)

	vec, err := n.Embedder.EmbedDocument(ctx, conv.Summary)
	if err != nil {
		return fmt.Errorf("embed conversation: %w", err)
	}

	return n.Conversations.Save(conv, vec)
}

// SearchConversations performs semantic search over saved conversations.
func (n *Node) SearchConversations(ctx context.Context, query string, limit int, agent, project string) ([]conversation.SearchResult, error) {
	vec, err := n.Embedder.EmbedQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	return n.Conversations.Search(vec, limit, agent, project)
}

// SearchMemories performs semantic search over memories.
func (n *Node) SearchMemories(ctx context.Context, query string, limit int, scope, project, domain string) ([]memory.SearchResult, error) {
	vec, err := n.Embedder.EmbedQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	return n.Index.Search(vec, limit, scope, project, domain)
}

// DeleteMemory removes a memory from both stores.
func (n *Node) DeleteMemory(id string) error {
	if err := n.Store.Delete(id); err != nil {
		return err
	}
	return n.Index.Delete(id)
}

// DeleteByPrefix removes all memories under a path prefix.
func (n *Node) DeleteByPrefix(prefix string) (int, error) {
	mems, err := n.Index.ListByPrefix(prefix, 10000)
	if err != nil {
		return 0, err
	}
	for _, m := range mems {
		_ = n.Store.Delete(m.ID)
		_ = n.Index.Delete(m.ID)
	}
	return len(mems), nil
}

// PeerID returns the stable UUID for this node — the canonical identifier for all stored data.
func (n *Node) PeerID() string {
	return n.Config.Node.PeerID()
}

// migrateSourceNodeToUUID normalizes all source_node columns from hostname (or empty) to UUID.
// This is idempotent — once all rows match the UUID, the UPDATE WHERE clauses match zero rows.
func (n *Node) migrateSourceNodeToUUID() {
	db := n.Index.DB()
	peerID := n.PeerID()
	hostname := n.Config.Node.Hostname

	// Tables with source_node column
	tables := []string{
		"memories", "conversations", "actions", "amnesia",
		"manifests", "ideas", "tasks", "chat_sessions",
		"watcher_audits", "delusions",
	}
	for _, table := range tables {
		// Update hostname → UUID
		if hostname != "" {
			res, err := db.Exec(
				fmt.Sprintf("UPDATE %s SET source_node = ? WHERE source_node = ?", table),
				peerID, hostname,
			)
			if err == nil {
				if cnt, _ := res.RowsAffected(); cnt > 0 {
					fmt.Printf("  Migration: %s — %d rows hostname→UUID\n", table, cnt)
				}
			}
		}
		// Update empty → UUID
		res, err := db.Exec(
			fmt.Sprintf("UPDATE %s SET source_node = ? WHERE source_node = ''", table),
			peerID,
		)
		if err == nil {
			if cnt, _ := res.RowsAffected(); cnt > 0 {
				fmt.Printf("  Migration: %s — %d rows empty→UUID\n", table, cnt)
			}
		}
	}

	// sessions table uses 'node' column instead of 'source_node'
	if hostname != "" {
		db.Exec("UPDATE sessions SET node = ? WHERE node = ?", peerID, hostname)
	}
	db.Exec("UPDATE sessions SET node = ? WHERE node = ''", peerID)

	// markers table uses 'from_node' and 'to_node'
	if hostname != "" {
		db.Exec("UPDATE markers SET from_node = ? WHERE from_node = ?", peerID, hostname)
		db.Exec("UPDATE markers SET to_node = ? WHERE to_node = ?", peerID, hostname)
	}
	db.Exec("UPDATE markers SET from_node = ? WHERE from_node = ''", peerID)
}

// Close shuts down all components.
func (n *Node) Close() error {
	return n.Index.Close()
}

// ReindexMemories re-embeds and re-indexes memories that changed via CRDT sync.
func (n *Node) ReindexMemories(ids []string) {
	ctx := context.Background()
	for _, id := range ids {
		mem, err := n.Store.Get(id)
		if err != nil || mem == nil {
			continue
		}
		vec, err := n.Embedder.EmbedDocument(ctx, mem.L1)
		if err != nil {
			continue
		}
		_ = n.Index.Upsert(mem, vec)
	}
}

// ResolveScopeID canonicalises a settings/comment scope_id from a short
// marker to its full UUID by dispatching on scopeType. Used by every
// settings + cross-entity write path so short markers don't end up
// silently orphaned in the database (visceral rule #14, issue #207).
//
// Returns ("", nil) for an empty scope_id; returns the input unchanged
// for unknown scope types (caller validates the type separately).
func (n *Node) ResolveScopeID(scopeType, scopeID string) (string, error) {
	if scopeID == "" {
		return "", nil
	}
	switch scopeType {
	case "product":
		return n.ResolveProductID(scopeID)
	case "manifest":
		return n.ResolveManifestID(scopeID)
	case "task":
		if n.Tasks == nil {
			return scopeID, nil
		}
		t, err := n.Tasks.Get(scopeID)
		if err != nil {
			return "", fmt.Errorf("resolve task %q: %w", scopeID, err)
		}
		if t == nil {
			return "", fmt.Errorf("task not found: %s", scopeID)
		}
		return t.ID, nil
	}
	return scopeID, nil
}

// ResolveProductID resolves a product marker or full ID to the full UUID.
// Returns empty string if productID is empty, error if not found.
func (n *Node) ResolveProductID(productID string) (string, error) {
	if productID == "" {
		return "", nil
	}
	p, err := n.Products.Get(productID)
	if err != nil || p == nil {
		return "", fmt.Errorf("product not found: %s", productID)
	}
	return p.ID, nil
}

// ResolveManifestID resolves a manifest marker or full ID to the full UUID.
// Returns empty string if manifestID is empty, error if not found.
func (n *Node) ResolveManifestID(manifestID string) (string, error) {
	if manifestID == "" {
		return "", nil
	}
	m, err := n.Manifests.Get(manifestID)
	if err != nil || m == nil {
		return "", fmt.Errorf("manifest not found: %s", manifestID)
	}
	return m.ID, nil
}

// ResolveManifestDependsOn resolves a comma-separated list of manifest markers/IDs to full IDs.
// selfID is the manifest being created/updated (empty for create). Validates existence,
// rejects self-dependency, and detects circular dependencies.
func (n *Node) ResolveManifestDependsOn(raw, selfID string) (string, error) {
	if raw == "" {
		return "", nil
	}
	parts := strings.Split(raw, ",")
	resolved := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		m, err := n.Manifests.Get(p)
		if err != nil {
			return "", fmt.Errorf("resolve manifest dependency %q: %v", p, err)
		}
		if m == nil {
			return "", fmt.Errorf("manifest dependency not found: %s", p)
		}
		if selfID != "" && m.ID == selfID {
			return "", fmt.Errorf("manifest cannot depend on itself")
		}
		// Check for circular dependency: if the dependency transitively depends on selfID
		if selfID != "" {
			if n.hasTransitiveDependency(m.ID, selfID, make(map[string]bool)) {
				return "", fmt.Errorf("circular dependency: %s transitively depends on this manifest", m.Marker)
			}
		}
		resolved = append(resolved, m.ID)
	}
	return strings.Join(resolved, ","), nil
}

// hasTransitiveDependency checks if fromID transitively depends on targetID.
func (n *Node) hasTransitiveDependency(fromID, targetID string, visited map[string]bool) bool {
	if visited[fromID] {
		return false
	}
	visited[fromID] = true
	m, _ := n.Manifests.Get(fromID)
	if m == nil {
		return false
	}
	for _, dep := range m.ParseDependsOn() {
		if dep == targetID {
			return true
		}
		if n.hasTransitiveDependency(dep, targetID, visited) {
			return true
		}
	}
	return false
}

// ResolveDependsOnTitles resolves a comma-separated depends_on string to a list of manifest titles.
func (n *Node) ResolveDependsOnTitles(dependsOn string) []string {
	if dependsOn == "" {
		return nil
	}
	parts := strings.Split(dependsOn, ",")
	titles := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		m, _ := n.Manifests.Get(p)
		if m != nil {
			titles = append(titles, m.Title)
		} else {
			titles = append(titles, p) // fallback to ID if not found
		}
	}
	return titles
}

// CheckManifestDeps returns true if all dependency manifests for the given manifest are closed/archive.
// Implements task.ManifestDepChecker interface for scheduler blocking.
func (n *Node) CheckManifestDeps(manifestID string) (bool, string) {
	m, err := n.Manifests.Get(manifestID)
	if err != nil || m == nil {
		return true, "" // manifest not found — don't block
	}
	deps := m.ParseDependsOn()
	if len(deps) == 0 {
		return true, "" // no dependencies
	}
	for _, depID := range deps {
		dep, err := n.Manifests.Get(depID)
		if err != nil || dep == nil {
			continue // missing dependency — don't block on phantom
		}
		if dep.Status != "closed" && dep.Status != "archive" {
			marker := depID
			if len(depID) >= 12 {
				marker = depID[:12]
			}
			// Canonical format — matches the prefix FlipManifestBlockedTasks
			// filters on in task.Store. Diverging from this means the
			// close-propagation walker (#78) can't see the task and the
			// dep chain won't auto-advance. Kept trailing metadata
			// (marker + title) for operator legibility.
			return false, fmt.Sprintf("manifest not satisfied — blocked by: %s (%s)", marker, dep.Title)
		}
	}
	return true, ""
}

// ValidateArchiveProduct checks that all linked manifests are "archive" before allowing a product to be archived.
func (n *Node) ValidateArchiveProduct(productID string) error {
	manifests, err := n.Manifests.ListByProject(productID, 1000)
	if err != nil {
		return fmt.Errorf("check manifests: %w", err)
	}
	for _, m := range manifests {
		if m.Status != "archive" {
			return fmt.Errorf("cannot archive product: manifest [%s] %s is still '%s' — archive all manifests first", m.Marker, m.Title, m.Status)
		}
	}
	return nil
}

// ValidateArchiveManifest checks that all linked tasks are terminal before allowing a manifest to be archived.
func (n *Node) ValidateArchiveManifest(manifestID string) error {
	tasks, err := n.Tasks.ListByManifest(manifestID, 1000)
	if err != nil {
		return fmt.Errorf("check tasks: %w", err)
	}
	terminal := map[string]bool{"completed": true, "failed": true, "cancelled": true}
	for _, t := range tasks {
		if !terminal[t.Status] {
			return fmt.Errorf("cannot archive manifest: task [%s] %s is still '%s' — all tasks must be completed, failed, or cancelled first", t.Marker, t.Title, t.Status)
		}
	}
	return nil
}

// taskReviewWriteAdapter satisfies task.ReviewWriter by delegating to
// comments.Store.Add with the TargetType/CommentType type-conversion
// the comments package needs. Keeps task free of an internal/comments
// import; the type conversion lives here in the node layer where both
// packages are already in scope.
type taskReviewWriteAdapter struct{ s *comments.Store }

func (a *taskReviewWriteAdapter) AddReviewComment(ctx context.Context, taskID, author, commentType, body string) error {
	_, err := a.s.Add(ctx, comments.TargetTask, taskID, author, comments.CommentType(commentType), body)
	return err
}

// taskReviewReadAdapter satisfies task.ReviewReader by listing
// comments on the task target and filtering down to review-type rows.
// Non-review comments (user_note, execution_review, etc.) are dropped
// here so the caller's "latest review comment wins" logic doesn't
// have to re-filter.
type taskReviewReadAdapter struct{ s *comments.Store }

// executionReviewCheckerAdapter satisfies task.ExecutionReviewChecker by
// asking the comments store whether the given task carries at least one
// execution_review comment authored by "agent". Used by the runner's
// post-completion amnesia gate (M4-T10).
type executionReviewCheckerAdapter struct{ s *comments.Store }

func (a *executionReviewCheckerAdapter) HasAgentExecutionReview(ctx context.Context, taskID string) (bool, error) {
	t := comments.TypeExecutionReview
	rows, err := a.s.List(ctx, comments.TargetTask, taskID, 50, &t)
	if err != nil {
		return false, err
	}
	for _, c := range rows {
		if c.Author == "agent" {
			return true, nil
		}
	}
	return false, nil
}

func (a *taskReviewReadAdapter) ListReviewCommentsForTask(ctx context.Context, taskID string, limit int) ([]task.ReviewComment, error) {
	all, err := a.s.List(ctx, comments.TargetTask, taskID, limit, nil)
	if err != nil {
		return nil, err
	}
	var out []task.ReviewComment
	for _, c := range all {
		t := string(c.Type)
		if t != "review_rejection" && t != "review_approval" {
			continue
		}
		out = append(out, task.ReviewComment{
			ID:        c.ID,
			Type:      t,
			Author:    c.Author,
			Body:      c.Body,
			CreatedAt: c.CreatedAt,
		})
	}
	return out, nil
}
