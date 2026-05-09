package node

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/k8nstantin/OpenPraxis/internal/action"
	"github.com/k8nstantin/OpenPraxis/internal/chat"
	"github.com/k8nstantin/OpenPraxis/internal/comments"
	"github.com/k8nstantin/OpenPraxis/internal/config"
	"github.com/k8nstantin/OpenPraxis/internal/conversation"
	"github.com/k8nstantin/OpenPraxis/internal/delusion"
	"github.com/k8nstantin/OpenPraxis/internal/embedding"
	"github.com/k8nstantin/OpenPraxis/internal/entity"
	executionlog "github.com/k8nstantin/OpenPraxis/internal/execution"
	"github.com/k8nstantin/OpenPraxis/internal/marker"
	"github.com/k8nstantin/OpenPraxis/internal/memory"
	"github.com/k8nstantin/OpenPraxis/internal/relationships"
	"github.com/k8nstantin/OpenPraxis/internal/schedule"
	"github.com/k8nstantin/OpenPraxis/internal/settings"
	"github.com/k8nstantin/OpenPraxis/internal/task"
	"github.com/k8nstantin/OpenPraxis/internal/templates"
	"github.com/k8nstantin/OpenPraxis/internal/watcher"

	gpcpu     "github.com/shirou/gopsutil/v3/cpu"
	gpdisk    "github.com/shirou/gopsutil/v3/disk"
	gpload    "github.com/shirou/gopsutil/v3/load"
	gpmem     "github.com/shirou/gopsutil/v3/mem"
	gpnet     "github.com/shirou/gopsutil/v3/net"
	gpprocess "github.com/shirou/gopsutil/v3/process"
)

// Node is the central orchestrator that wires all components together.
type Node struct {
	Config        *config.Config
	Store         *memory.Store
	Index         *memory.Index
	Conversations *conversation.Store
	Markers       *marker.Store
	Actions       *action.Store
	Entities      *entity.Store
	// Tasks is still needed by the task runner — TODO: migrate to entity store
	// Tasks field removed — all task data lives in entities + execution_log
	ChatSessions     *chat.SessionStore
	Watcher          *watcher.Store
	SettingsStore    *settings.Store
	SettingsResolver *settings.Resolver
	Templates        *templates.Store
	TemplatesResolv  *templates.Resolver
	Comments         *comments.Store
	// Attachments is the comment-attachment store (UB-2). On-disk files
	// land under <data_dir>/attachments/<comment_id>/; rows live in the
	// shared memories.db.
	Attachments *comments.AttachmentStore
	// Relationships is the unified edge store (Praxis Relationships
	// PR/M1). Lives alongside the existing dep tables during the M2
	// dual-write phase; becomes the sole source after M3 cutover.
	Relationships *relationships.Store
	// Schedules is the central SCD-2 store for entity firing schedules
	// (PR/M-Schedule/M1). The runner currently still reads scheduling
	// fields off the task row; a follow-up cuts it over to read here.
	// This PR persists schedules so the dashboard surfaces them.
	Schedules *schedule.Store
	// ScheduleRunner consumes the schedules table — registers each
	// current+enabled row against an in-memory robfig/cron/v3 ticker
	// and dispatches by entity_kind on fire. Wired in cmd/serve.go
	// after the node is built so the dispatcher map can capture
	// references to n.Tasks etc.
	ScheduleRunner *schedule.Runner
	// ExecutionLog is the unified run-history store (EL/M1). Replaces
	// task_runs + task_run_host_samples in EL/M5.
	ExecutionLog *executionlog.Store
	runner       *task.Runner
	hostSampler  *task.HostSampler
	Embedder     *embedding.Engine
	StartedAt    time.Time

	// transcriptPaths stores session_id → live transcript file path so
	// the 5s MCP sampler and PostToolUse hook can parse cumulative data.
	transcriptMu    sync.RWMutex
	transcriptPaths map[string]string

	// sessionRunUIDs maps session_id → execution_log run_uid.
	// Set when an MCP session connects; read by the PostToolUse hook.
	sessionRunUIDMu  sync.RWMutex
	sessionRunUIDs   map[string]string

	// sessionLastSample throttles PostToolUse hook writes to execution_log.
	sessionSampleMu   sync.Mutex
	sessionLastSample map[string]time.Time

	// netDiskBaseline tracks the previous IOCounter reading for delta-rate calculation.
	sysBaselineMu sync.Mutex
	sysBaseline   struct {
		at      time.Time
		netRx   uint64
		netTx   uint64
		diskR   uint64
		diskW   uint64
	}
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

	entityStore, err := entity.NewStore(index.DB())
	if err != nil {
		return nil, fmt.Errorf("init entity store: %w", err)
	}
	if _, err := entityStore.MigrateFromLegacy(context.Background()); err != nil {
		return nil, fmt.Errorf("migrate entities: %w", err)
	}

	// Ensure the delusions table exists (schema lives in internal/delusion,
	// previously owned by internal/manifest).
	if err := delusion.InitSchema(index.DB()); err != nil {
		return nil, fmt.Errorf("init delusion schema: %w", err)
	}

	commentsStore := comments.NewStore(index.DB())

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
	if err := comments.InitAttachmentSchema(index.DB()); err != nil {
		return nil, fmt.Errorf("comments.InitAttachmentSchema: %w", err)
	}
	attachmentRoot := cfg.Storage.DataDir
	if attachmentRoot == "" {
		attachmentRoot = "."
	}
	attachmentsStore := comments.NewAttachmentStore(index.DB(), attachmentRoot+"/attachments")
	commentsStore.SetAttachments(attachmentsStore)

	settingsStore := settings.NewStore(index.DB())
	taskSettingsAdapter := &entityTaskSettingsAdapter{entities: entityStore, rels: nil}
	manifestSettingsAdapter := &entityManifestSettingsAdapter{
		entities: entityStore,
		rels:     nil, // wired after relationships store is constructed below
	}
	settingsResolver := settings.NewResolver(settingsStore, taskSettingsAdapter, manifestSettingsAdapter)

	// RC/M1: prompt_templates substrate. Schema + system seed run every
	// boot; both are idempotent. Resolver walks task → manifest → product
	// → agent → system using relationships edges.
	if err := templates.InitSchema(index.DB()); err != nil {
		return nil, fmt.Errorf("init templates schema: %w", err)
	}
	templatesStore := templates.NewStore(index.DB())
	templatesScopeAdapter := &taskTemplatesScopeAdapter{
		entities: entityStore,
		rels:     nil, // wired after relationships store is constructed below
	}
	templatesResolver := templates.NewResolver(templatesStore, templatesScopeAdapter, nil)

	// M4-T14: one-time migration of legacy tasks.max_turns column values into
	// settings rows at task scope, followed by dropping the column. Both are
	// idempotent — safe when the tasks table is absent (pragma_table_info
	// returns empty, hasMaxTurnsColumn returns false, migration short-circuits).
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
	// PR/M2 + PR/M3 — copy every row out of the three legacy dependency
	// tables (product_dependencies / manifest_dependencies /
	// task_dependency) PLUS the legacy ownership FK columns
	// (manifests.project_id / tasks.manifest_id) into the unified
	// relationships SCD-2 table. Idempotent — re-running inserts zero
	// rows.
	if _, err := relationshipsStore.MigrateLegacyDeps(context.Background()); err != nil {
		return nil, fmt.Errorf("migrate legacy deps to relationships: %w", err)
	}
	// PR/M3 — drop the legacy ownership columns now that every row is
	// represented in `relationships` as an EdgeOwns edge. The migration
	// is a SQLite-portable rename-table swap inside a single
	// transaction; idempotent (a second boot finds the column already
	// gone and is a no-op).
	if err := relationships.DropOwnershipColumns(context.Background(), index.DB()); err != nil {
		return nil, fmt.Errorf("drop legacy ownership columns: %w", err)
	}
	// Wire the relationships store into the task dep store so its
	// Add/Remove/List methods read + write the unified table.
	// Wire the relationships store into all scope adapters now that it's available.
	taskSettingsAdapter.rels = relationshipsStore
	manifestSettingsAdapter.rels = relationshipsStore
	templatesScopeAdapter.rels = relationshipsStore

	// Central SCD-2 schedules table. Migration is idempotent
	// (CREATE TABLE / INDEX IF NOT EXISTS). Same DB handle as every
	// other store so schedules live in memories.db alongside tasks /
	// manifests / products.
	scheduleStore, err := schedule.New(index.DB())
	if err != nil {
		return nil, fmt.Errorf("init schedule store: %w", err)
	}
	if _, err := scheduleStore.CloseStaleOneShots(context.Background()); err != nil {
		return nil, fmt.Errorf("close stale schedules: %w", err)
	}

	if err := executionlog.InitSchema(index.DB()); err != nil {
		return nil, fmt.Errorf("init execution_log schema: %w", err)
	}
	executionStore := executionlog.NewStore(index.DB())


	n := &Node{
		Config:           cfg,
		Store:            store,
		Index:            index,
		Conversations:    convStore,
		Markers:          markerStore,
		Actions:          actionStore,
		Entities:         entityStore,
		ChatSessions:     chatStore,
		Watcher:          watcherStore,
		SettingsStore:    settingsStore,
		SettingsResolver: settingsResolver,
		Templates:        templatesStore,
		TemplatesResolv:  templatesResolver,
		Comments:         commentsStore,
		Attachments:      attachmentsStore,
		Relationships:    relationshipsStore,
		Schedules:        scheduleStore,
		ExecutionLog:     executionStore,
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

	// Drop legacy tables after ALL stores are initialized so no store
	// init can re-create a table we just dropped.
	if _, err := entity.DropLegacyTables(context.Background(), index.DB()); err != nil {
		return nil, fmt.Errorf("drop legacy tables: %w", err)
	}

	return n, nil
}

// taskTemplatesScopeAdapter translates a task id into its manifest and
// product ids for the templates resolver using the relationships edge store.
type taskTemplatesScopeAdapter struct {
	entities *entity.Store
	rels     *relationships.Store
}

func (a *taskTemplatesScopeAdapter) ManifestAndProductForTask(ctx context.Context, taskID string) (string, string, error) {
	if a.rels == nil {
		return "", "", nil
	}
	// Find the manifest that owns this task.
	edges, err := a.rels.ListIncoming(ctx, taskID, relationships.EdgeOwns)
	if err != nil {
		return "", "", nil
	}
	var manifestID string
	for _, e := range edges {
		if e.SrcKind == relationships.KindManifest {
			manifestID = e.SrcID
			break
		}
	}
	if manifestID == "" {
		return "", "", nil
	}
	// Find the product that owns that manifest.
	var productID string
	if prodEdges, err := a.rels.ListIncoming(ctx, manifestID, relationships.EdgeOwns); err == nil {
		for _, e := range prodEdges {
			if e.SrcKind == relationships.KindProduct {
				productID = e.SrcID
				break
			}
		}
	}
	return manifestID, productID, nil
}

// entityManifestSettingsAdapter satisfies settings.ManifestLookup by looking
// up the product that owns a manifest via the relationships edge store.
// Replaces manifest.SettingsAdapter without importing internal/manifest.
type entityManifestSettingsAdapter struct {
	entities *entity.Store
	rels     *relationships.Store
}

func (a *entityManifestSettingsAdapter) GetManifestForSettings(ctx context.Context, manifestID string) (settings.ManifestRec, error) {
	if a == nil || a.rels == nil {
		return settings.ManifestRec{ID: manifestID}, nil
	}
	// When the dispatcher fires a product entity, entityTaskSettingsAdapter
	// returns ManifestID = productID (synthetic). Detect this: if the entity
	// is a product, return ProductID = manifestID so the resolver checks
	// product scope for the correct ID.
	if a.entities != nil {
		e, _ := a.entities.Get(manifestID)
		if e != nil && e.Type == "product" {
			return settings.ManifestRec{ID: manifestID, ProductID: manifestID}, nil
		}
	}
	// Normal manifest: walk incoming owns edges to find the owning product.
	edges, err := a.rels.ListIncoming(ctx, manifestID, relationships.EdgeOwns)
	if err != nil {
		return settings.ManifestRec{}, fmt.Errorf("manifest settings adapter: list incoming: %w", err)
	}
	var productID string
	for _, e := range edges {
		if e.SrcKind == relationships.KindProduct {
			productID = e.SrcID
			break
		}
	}
	return settings.ManifestRec{ID: manifestID, ProductID: productID}, nil
}

// entityTaskSettingsAdapter implements settings.TaskLookup using the
// entity + relationships stores. Handles all three entity kinds so the
// settings resolver always reaches the correct scope tier:
//
//   product  → ManifestID = entityID (synthetic) so GetManifestForSettings
//              is called with the product ID, which then returns ProductID =
//              entityID — resolver checks product scope directly.
//   manifest → ManifestID = entityID so resolver checks manifest scope,
//              then GetManifestForSettings walks up to find the product.
//   task     → ManifestID = owning manifest via relationships owns edge.
type entityTaskSettingsAdapter struct {
	entities *entity.Store
	rels     *relationships.Store
}

func (a *entityTaskSettingsAdapter) GetTaskForSettings(ctx context.Context, entityID string) (settings.TaskRec, error) {
	if a == nil || a.rels == nil {
		return settings.TaskRec{ID: entityID}, nil
	}
	// Check entity type to route the scope walk correctly.
	if a.entities != nil {
		e, _ := a.entities.Get(entityID)
		if e != nil {
			switch e.Type {
			case "product":
				// Entity IS the product — synthetic ManifestID = productID so
				// GetManifestForSettings receives the product ID and returns
				// ProductID = productID, landing the resolver at product scope.
				return settings.TaskRec{ID: entityID, ManifestID: entityID}, nil
			case "manifest":
				// Entity IS the manifest — set ManifestID directly so the
				// resolver checks manifest scope then walks up to product.
				return settings.TaskRec{ID: entityID, ManifestID: entityID}, nil
			}
		}
	}
	// Default: entity is a task — find owning manifest via relationships.
	edges, err := a.rels.ListIncoming(ctx, entityID, relationships.EdgeOwns)
	if err != nil {
		return settings.TaskRec{ID: entityID}, nil
	}
	for _, e := range edges {
		if e.SrcKind == relationships.KindManifest {
			return settings.TaskRec{ID: entityID, ManifestID: e.SrcID}, nil
		}
	}
	return settings.TaskRec{ID: entityID}, nil
}

// InitRunner creates and sets the task Runner using the Node's own stores.
// Must be called after New() and before serving requests.
// The Runner reads its max_parallel cap per task via n.SettingsResolver —
// there is no process-wide cap argument because caps are now per-product.
func (n *Node) InitRunner(onEvent func(string, map[string]string)) *task.Runner {
	// Empty repoDir → Runner falls back to process CWD at spawn time.
	// cmd/serve.go passes an explicit dir via a follow-up SetRepoDir if it
	// runs from outside the repo root.
	n.runner = task.NewRunner(n.Actions, n.SettingsResolver, "", onEvent)
	// Wire the post-completion comment gate (M4-T10). Non-fatal
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
	// Wire the unified execution log so the runner records started/sample/
	// completed/failed events in execution_log instead of the legacy
	// task_runs + task_run_host_samples tables.
	if n.ExecutionLog != nil {
		n.runner.SetExecutionLog(n.ExecutionLog)
	}
	// Host-metrics sampler: polls the serve process's CPU/RSS every
	// `host_sampler_tick_seconds` (system scope, catalog default 5s) and
	// attributes each sample to every currently-running task. Data lands
	// in execution_log as EventSample rows. The sampler is a single
	// shared instance on the node — one-at-boot is fine.
	tick := resolveHostSamplerTick(n.SettingsStore)
	n.hostSampler = task.NewHostSampler(tick)
	n.hostSampler.Start(context.Background())
	n.runner.SetHostSampler(n.hostSampler)
	return n.runner
}

// HostSamplerTick returns the resolved host-sampler tick duration.
// Reads `host_sampler_tick_seconds` at system scope; falls back to
// catalog default on lookup failure.
func (n *Node) HostSamplerTick() time.Duration {
	return resolveHostSamplerTick(n.SettingsStore)
}

// resolveHostSamplerTick reads the host_sampler_tick_seconds knob at
// system scope. Lookup failures fall back to the catalog default. The
// floor is 1s (matches the catalog SliderMin); a 0 or negative value
// would otherwise produce a runaway ticker.
func resolveHostSamplerTick(store *settings.Store) time.Duration {
	const fallback = 5 * time.Second
	const floor = 1 * time.Second
	if store == nil {
		return fallback
	}
	entry, err := store.Get(context.Background(), settings.ScopeSystem, "", "host_sampler_tick_seconds")
	var secs float64
	if err == nil && entry.Value != "" {
		// settings values are JSON-encoded numbers.
		if perr := unmarshalNumber(entry.Value, &secs); perr != nil {
			secs = 0
		}
	}
	if secs <= 0 {
		if def, ok := settings.SystemDefault("host_sampler_tick_seconds"); ok {
			switch v := def.(type) {
			case int:
				secs = float64(v)
			case int64:
				secs = float64(v)
			case float64:
				secs = v
			}
		}
	}
	if secs <= 0 {
		return fallback
	}
	d := time.Duration(secs) * time.Second
	if d < floor {
		return floor
	}
	return d
}

// unmarshalNumber decodes a JSON number value as a float64. Used to
// resolve int + float knobs without taking a settings package import.
func unmarshalNumber(raw string, out *float64) error {
	// Local copy of encoding/json's behavior — the settings store always
	// writes JSON-encoded numbers, so a simple Sscanf is sufficient and
	// avoids pulling encoding/json into node.go's small import set.
	_, err := fmt.Sscanf(raw, "%f", out)
	return err
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

// LiveSessionCost holds current cumulative token/turn metrics parsed from
// a live transcript. Populated every 5s by the MCP sampler.
// Cost is intentionally excluded — costs come from actual billing data only.
type LiveSessionCost struct {
	InputTokens       int
	OutputTokens      int
	CacheReadTokens   int
	CacheCreateTokens int
	Model             string
	Turns             int // number of conversation turns (assistant responses)
	Actions           int // number of tool calls extracted from transcript
}

// ParseLiveTranscript reads the transcript file at path and returns the
// current cumulative cost/token data. Called by the MCP sampler every
// host_sampler_tick_seconds to populate sample rows with full metrics.
// Returns zero-value LiveSessionCost on any error.
var ParseLiveTranscript func(path string) LiveSessionCost

// SetSessionRunUID stores the execution_log run_uid for a session.
// Called by the MCP server when an agent connects.
func (n *Node) SetSessionRunUID(sessionID, runUID string) {
	if sessionID == "" || runUID == "" {
		return
	}
	n.sessionRunUIDMu.Lock()
	if n.sessionRunUIDs == nil {
		n.sessionRunUIDs = make(map[string]string)
	}
	n.sessionRunUIDs[sessionID] = runUID
	n.sessionRunUIDMu.Unlock()
}

// GetSessionRunUID returns the execution_log run_uid for a session, or "".
func (n *Node) GetSessionRunUID(sessionID string) string {
	n.sessionRunUIDMu.RLock()
	defer n.sessionRunUIDMu.RUnlock()
	if n.sessionRunUIDs == nil {
		return ""
	}
	return n.sessionRunUIDs[sessionID]
}

// ShouldWriteSample returns true if enough time (interval) has passed since
// the last execution_log sample for this session. Updates the last-sample
// time atomically so concurrent callers only write one row.
func (n *Node) ShouldWriteSample(sessionID string, interval time.Duration) bool {
	n.sessionSampleMu.Lock()
	defer n.sessionSampleMu.Unlock()
	if n.sessionLastSample == nil {
		n.sessionLastSample = make(map[string]time.Time)
	}
	if time.Since(n.sessionLastSample[sessionID]) < interval {
		return false
	}
	n.sessionLastSample[sessionID] = time.Now()
	return true
}

// SetTranscriptPath stores the live transcript path for a session so the
// 5s sampler can parse current token/cost data mid-session.
func (n *Node) SetTranscriptPath(sessionID, path string) {
	if sessionID == "" || path == "" {
		return
	}
	n.transcriptMu.Lock()
	if n.transcriptPaths == nil {
		n.transcriptPaths = make(map[string]string)
	}
	n.transcriptPaths[sessionID] = path
	n.transcriptMu.Unlock()
}

// GetTranscriptPath returns the live transcript path for a session, or "".
func (n *Node) GetTranscriptPath(sessionID string) string {
	n.transcriptMu.RLock()
	defer n.transcriptMu.RUnlock()
	if n.transcriptPaths == nil {
		return ""
	}
	return n.transcriptPaths[sessionID]
}

// AgentForSession looks up the agent name (e.g. "Claude Code", "cursor") for a
// given session_id. Checks the MCP sessions store using UUID prefix matching.
// Returns "" if the session is not found — callers should leave agent_runtime
// empty rather than hardcoding a value.
func (n *Node) AgentForSession(sessionID string) string {
	if sessionID == "" {
		return ""
	}
	// Try exact match first, then prefix match.
	rows, err := n.Index.DB().QueryContext(context.Background(),
		`SELECT agent FROM sessions WHERE uuid = ? OR uuid LIKE ? LIMIT 1`,
		sessionID, sessionID[:min(8, len(sessionID))]+"%")
	if err != nil {
		return ""
	}
	defer rows.Close()
	if rows.Next() {
		var agent string
		if err := rows.Scan(&agent); err == nil {
			return agent
		}
	}
	return ""
}

// SystemSnapshot reads live system metrics via gopsutil.
// Net and disk rates are computed as delta since the previous call.
// Returns zero values on any error.
func (n *Node) SystemSnapshot() (cpu, rssMB, memUsedMB, memTotalMB, netRx, netTx, diskR, diskW, loadAvg float64) {
	if pcts, err := gpcpu.Percent(0, false); err == nil && len(pcts) > 0 {
		cpu = pcts[0]
	}
	if vm, err := gpmem.VirtualMemory(); err == nil && vm != nil {
		memUsedMB  = float64(vm.Used) / (1024 * 1024)
		memTotalMB = float64(vm.Total) / (1024 * 1024)
	}
	// rss_mb is the openpraxis serve process RSS, distinct from system memory.
	if proc, err := gpprocess.NewProcess(int32(os.Getpid())); err == nil {
		if mi, err := proc.MemoryInfo(); err == nil && mi != nil {
			rssMB = float64(mi.RSS) / (1024 * 1024)
		}
	}
	if la, err := gpload.Avg(); err == nil && la != nil {
		loadAvg = la.Load1
	}

	now := time.Now()
	n.sysBaselineMu.Lock()
	defer n.sysBaselineMu.Unlock()

	// Net rate: sum only non-loopback interfaces to exclude loopback noise.
	if iocs, err := gpnet.IOCounters(true); err == nil {
		var rx, tx uint64
		for _, ioc := range iocs {
			if ioc.Name != "lo" && ioc.Name != "lo0" {
				rx += ioc.BytesRecv
				tx += ioc.BytesSent
			}
		}
		if !n.sysBaseline.at.IsZero() {
			dt := now.Sub(n.sysBaseline.at).Seconds()
			if dt > 0 {
				// Mbps to match readSystemMetrics in host_metrics.go.
				netRx = float64(rx-n.sysBaseline.netRx) * 8 / 1e6 / dt
				netTx = float64(tx-n.sysBaseline.netTx) * 8 / 1e6 / dt
				if netRx < 0 { netRx = 0 }
				if netTx < 0 { netTx = 0 }
			}
		}
		n.sysBaseline.netRx = rx
		n.sysBaseline.netTx = tx
	}

	// Disk I/O rate: delta since last call.
	if diocs, err := gpdisk.IOCounters(); err == nil {
		var rb, wb uint64
		for _, d := range diocs {
			rb += d.ReadBytes
			wb += d.WriteBytes
		}
		if !n.sysBaseline.at.IsZero() {
			dt := now.Sub(n.sysBaseline.at).Seconds()
			if dt > 0 {
				diskR = float64(rb-n.sysBaseline.diskR) / dt / (1024 * 1024)
				diskW = float64(wb-n.sysBaseline.diskW) / dt / (1024 * 1024)
				if diskR < 0 { diskR = 0 }
				if diskW < 0 { diskW = 0 }
			}
		}
		n.sysBaseline.diskR = rb
		n.sysBaseline.diskW = wb
	}

	n.sysBaseline.at = now
	return
}

// DB returns the shared *sql.DB used by all stores on this node.
// Callers that need direct SQL access (e.g. legacy system_host_samples
// queries) use this rather than reaching into a specific store.
func (n *Node) DB() *sql.DB {
	return n.Index.DB()
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

// ResolveProductID validates a product UUID by ensuring it exists in the
// entity store, returning the canonical ID. Empty input returns empty.
func (n *Node) ResolveProductID(productID string) (string, error) {
	if productID == "" {
		return "", nil
	}
	e, err := n.Entities.Get(productID)
	if err != nil || e == nil {
		return "", fmt.Errorf("product not found: %s", productID)
	}
	return e.EntityUID, nil
}

// ResolveManifestID validates a manifest UUID by ensuring it exists in the
// entity store, returning the canonical ID. Empty input returns empty.
func (n *Node) ResolveManifestID(manifestID string) (string, error) {
	if manifestID == "" {
		return "", nil
	}
	e, err := n.Entities.Get(manifestID)
	if err != nil || e == nil {
		return "", fmt.Errorf("manifest not found: %s", manifestID)
	}
	return e.EntityUID, nil
}

// ResolveScopeID validates a settings/comment scope_id by ensuring the
// referenced entity exists. With markers gone, every cross-entity reference
// must already be a full UUID — this just checks the row is reachable so
// callers can surface a 4xx instead of writing a phantom-FK row.
//
// Returns ("", nil) for an empty scope_id; returns the input unchanged
// for unknown scope types (caller validates the type separately).
func (n *Node) ResolveScopeID(scopeType, scopeID string) (string, error) {
	if scopeID == "" {
		return "", nil
	}
	switch scopeType {
	case "product", "manifest":
		e, err := n.Entities.Get(scopeID)
		if err != nil || e == nil {
			return "", fmt.Errorf("%s not found: %s", scopeType, scopeID)
		}
		return e.EntityUID, nil
	case "task":
		// Tasks live in entities — just return the scopeID directly.
		return scopeID, nil
	}
	return scopeID, nil
}

// ResolveManifestDependsOn validates a comma-separated list of manifest IDs
// against the entity store. selfID is the manifest being created/updated
// (empty for create). Validates existence and rejects self-dependency.
// Every entry must be a full UUID.
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
		e, err := n.Entities.Get(p)
		if err != nil {
			return "", fmt.Errorf("resolve manifest dependency %q: %v", p, err)
		}
		if e == nil {
			return "", fmt.Errorf("manifest dependency not found: %s", p)
		}
		if selfID != "" && e.EntityUID == selfID {
			return "", fmt.Errorf("manifest cannot depend on itself")
		}
		resolved = append(resolved, e.EntityUID)
	}
	return strings.Join(resolved, ","), nil
}

// ResolveDependsOnTitles resolves a comma-separated depends_on string to a list of entity titles.
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
		e, _ := n.Entities.Get(p)
		if e != nil {
			titles = append(titles, e.Title)
		} else {
			titles = append(titles, p) // fallback to ID if not found
		}
	}
	return titles
}

// CheckManifestDeps returns true if all dependency manifests for the given
// manifest are closed or archived. Uses the relationships store to walk
// the depends_on edges.
func (n *Node) CheckManifestDeps(manifestID string) (bool, string) {
	if n.Relationships == nil {
		return true, ""
	}
	ctx := context.Background()
	// List outgoing depends_on edges from this manifest.
	edges, err := n.Relationships.ListOutgoing(ctx, manifestID, relationships.EdgeDependsOn)
	if err != nil || len(edges) == 0 {
		return true, ""
	}
	for _, e := range edges {
		dep, err := n.Entities.Get(e.DstID)
		if err != nil || dep == nil {
			continue // missing dependency — don't block on phantom
		}
		if dep.Status != "closed" && dep.Status != "archived" {
			return false, fmt.Sprintf("manifest not satisfied — blocked by: %s (%s)", e.DstID, dep.Title)
		}
	}
	return true, ""
}

// ValidateArchiveProduct checks that all linked manifests are archived before
// allowing a product to be archived. Uses relationships to find owned manifests.
func (n *Node) ValidateArchiveProduct(productID string) error {
	if n.Relationships == nil {
		return nil
	}
	ctx := context.Background()
	edges, err := n.Relationships.ListOutgoing(ctx, productID, relationships.EdgeOwns)
	if err != nil {
		return fmt.Errorf("check manifests: %w", err)
	}
	for _, e := range edges {
		if e.DstKind != relationships.KindManifest {
			continue
		}
		m, _ := n.Entities.Get(e.DstID)
		if m == nil {
			continue
		}
		if m.Status != "archived" {
			return fmt.Errorf("cannot archive product: manifest [%s] %s is still '%s' — archive all manifests first", m.EntityUID, m.Title, m.Status)
		}
	}
	return nil
}

// ValidateArchiveManifest checks that all linked tasks are terminal before allowing a manifest to be archived.
// Uses the relationships + entities stores (no legacy tasks table).
func (n *Node) ValidateArchiveManifest(manifestID string) error {
	if n.Relationships == nil || n.Entities == nil {
		return nil
	}
	edges, err := n.Relationships.ListOutgoing(context.Background(), manifestID, "owns")
	if err != nil {
		return fmt.Errorf("check tasks: %w", err)
	}
	terminal := map[string]bool{"completed": true, "failed": true, "cancelled": true, "archived": true}
	for _, edge := range edges {
		e, _ := n.Entities.Get(edge.DstID)
		if e != nil && e.Type == "task" && !terminal[e.Status] {
			return fmt.Errorf("cannot archive manifest: task [%s] %s is still '%s' — all tasks must be completed, failed, or cancelled first", e.EntityUID, e.Title, e.Status)
		}
	}
	return nil
}

// executionReviewCheckerAdapter satisfies task.ExecutionReviewChecker by
// asking the comments store whether the given task carries at least one
// comment comment authored by "agent". Used by the runner's
// post-completion amnesia gate (M4-T10).
type executionReviewCheckerAdapter struct{ s *comments.Store }

func (a *executionReviewCheckerAdapter) HasAgentExecutionReview(ctx context.Context, taskID string) (bool, error) {
	t := comments.TypeExecutionReview
	rows, err := a.s.List(ctx, comments.TargetEntity, taskID, 50, &t)
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

