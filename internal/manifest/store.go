package manifest

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/k8nstantin/OpenPraxis/internal/relationships"
)

// Manifest is a detailed development spec document.
type Manifest struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"` // one-liner
	Content     string    `json:"content"`     // full markdown spec
	Status      string    `json:"status"`      // draft, open, closed, archive
	JiraRefs    []string  `json:"jira_refs"`   // e.g. ["ENG-4816", "ENG-5266"]
	Tags        []string  `json:"tags"`
	Author      string    `json:"author"`      // session that created it
	SourceNode  string    `json:"source_node"`
	ProjectID   string    `json:"project_id"`  // optional project grouping
	DependsOn   string    `json:"depends_on"`  // comma-separated manifest IDs
	Version     int       `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// Computed from linked tasks — populated by EnrichWithCosts() (list-
	// path: turns + cost only) or EnrichRecursiveCosts() (single-get path:
	// adds actions + tokens). Manifests don't recurse like products do —
	// the "recursive" name is kept for symmetry with product.Store but
	// the walk is just task_runs joined to tasks where manifest_id = m.id.
	TotalTasks   int     `json:"total_tasks"`
	TotalTurns   int     `json:"total_turns"`
	TotalCost    float64 `json:"total_cost"`
	TotalActions int     `json:"total_actions"`
	TotalTokens  int     `json:"total_tokens"`
}

// Store manages manifest persistence.
type Store struct {
	db *sql.DB
	// rels is the unified relationships SCD-2 store. Wired post-init via
	// SetRelationshipsBackend so the package import direction (relationships
	// has no internal deps) stays one-way. All dependency reads + writes
	// route through this; the legacy manifest_dependencies table is dormant.
	rels *relationships.Store
	// onTerminalTransition fires AFTER a successful Update that moved
	// the manifest from a non-terminal status (draft/open) into a
	// terminal one (closed/archive). Wired in node.go to
	// task.Store.PropagateManifestClosed so waiting tasks in downstream
	// manifests auto-activate. Nil means propagation is disabled —
	// tests that don't care about the cross-package effect leave it
	// unset and see no hook fires.
	onTerminalTransition func(ctx context.Context, manifestID string)

	// onDepRemoved fires AFTER a successful RemoveDep. The wired
	// handler rehabs any now-unblocked waiting tasks into 'pending'
	// per Option B (issue #74 comment). Distinct from
	// onTerminalTransition: removal is operator-initiated scope
	// reshuffling, not the normal close-advances-work flow, so we
	// refuse to auto-burn budget by scheduling anything. Nil disables.
	onDepRemoved func(ctx context.Context, manifestID string)
}

// SetTerminalTransitionHandler registers the callback that propagates
// manifest-close activation to downstream waiting tasks. Safe to call
// once at startup; there is no mutex because production wiring runs
// before any HTTP/MCP handler is serving.
func (s *Store) SetTerminalTransitionHandler(fn func(ctx context.Context, manifestID string)) {
	s.onTerminalTransition = fn
}

// SetDepRemovedHandler registers the callback that fires after a
// successful RemoveDep. The handler is expected to rehab any waiting
// tasks in manifestID that are no longer blocked — per the Option B
// decision recorded in issue #74, they go to `pending` (not
// `scheduled`), so the operator has to explicitly arm them. This keeps
// an accidental "remove dep" click from auto-burning budget.
func (s *Store) SetDepRemovedHandler(fn func(ctx context.Context, manifestID string)) {
	s.onDepRemoved = fn
}

// SetRelationshipsBackend wires the unified relationships SCD-2 store
// for manifest dependency / ownership edges. Call once at startup before
// any mutation runs.
func (s *Store) SetRelationshipsBackend(r *relationships.Store) {
	s.rels = r
}

// NewStore creates a manifest store.
func NewStore(db *sql.DB) (*Store, error) {
	s := &Store{db: db}
	if err := s.init(); err != nil {
		return nil, err
	}
	if err := s.InitDelusions(); err != nil {
		return nil, err
	}
	if err := s.InitLinks(); err != nil {
		return nil, err
	}
	if err := s.initDependenciesSchema(); err != nil {
		return nil, err
	}
	// Auto-wire a default relationships backend against the same DB
	// handle so tests + any caller that doesn't explicitly call
	// SetRelationshipsBackend get a working dep API. node-level wiring
	// overrides this with the shared singleton.
	rels, err := relationships.New(s.db)
	if err != nil {
		return nil, fmt.Errorf("init relationships backend: %w", err)
	}
	s.rels = rels
	// PR/M3 (2026-04-28): the legacy `manifests.depends_on` →
	// `manifest_dependencies` backfill is no longer fired here. All rows
	// have long since landed in manifest_dependencies on prior boots and
	// from there into the unified relationships store via
	// relationships.MigrateLegacyDeps (called from node.go). The
	// BackfillLegacyDependsOn function is kept on the type for forensic /
	// emergency reseed scenarios but is not part of the boot path.
	return s, nil
}

func (s *Store) init() error {
	// Schema after PR/M3: ownership lives in `relationships` (EdgeOwns
	// product → manifest). The legacy `project_id` column is no longer
	// part of CREATE TABLE; existing DBs get it dropped by
	// relationships.DropOwnershipColumns at boot.
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS manifests (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		content TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'draft',
		jira_refs TEXT NOT NULL DEFAULT '[]',
		tags TEXT NOT NULL DEFAULT '[]',
		author TEXT NOT NULL DEFAULT '',
		version INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("create manifests table: %w", err)
	}

	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_manifests_status ON manifests(status)`)
	if err != nil {
		return fmt.Errorf("create manifests status index: %w", err)
	}

	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_manifests_updated ON manifests(updated_at DESC)`)
	if err != nil {
		return err
	}
	// Legacy ALTERs kept for upgrade path on DBs that pre-date these
	// columns. project_id is intentionally absent — relationships.
	// DropOwnershipColumns is responsible for removing it from any DB
	// that still has it.
	s.db.Exec(`ALTER TABLE manifests ADD COLUMN source_node TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`ALTER TABLE manifests ADD COLUMN deleted_at TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`ALTER TABLE manifests ADD COLUMN depends_on TEXT NOT NULL DEFAULT ''`)
	return nil
}

// Create stores a new manifest.
//
// Ownership wiring (PR/M3): when projectID is non-empty, opens an
// `EdgeOwns(product → manifest)` row in the relationships SCD-2 table.
// That edge IS the canonical ownership record — the legacy
// `manifests.project_id` column was dropped in this PR. Every read
// path (DAG endpoint, hierarchy walkers, settings resolver,
// ListByProject) consults relationships.
func (s *Store) Create(title, description, content, status, author, sourceNode, projectID, dependsOn string, jiraRefs, tags []string) (*Manifest, error) {
	if status == "" {
		status = "draft"
	}
	if jiraRefs == nil {
		jiraRefs = []string{}
	}
	if tags == nil {
		tags = []string{}
	}

	id := uuid.Must(uuid.NewV7()).String()
	now := time.Now().UTC()
	jiraJSON, _ := json.Marshal(jiraRefs)
	tagsJSON, _ := json.Marshal(tags)

	_, err := s.db.Exec(`INSERT INTO manifests (id, title, description, content, status, jira_refs, tags, author, source_node, depends_on, version, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
		id, title, description, content, status, string(jiraJSON), string(tagsJSON), author, sourceNode, dependsOn,
		now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}

	if projectID != "" && s.rels != nil {
		if err := s.rels.Create(context.Background(), relationships.Edge{
			SrcKind:   relationships.KindProduct,
			SrcID:     projectID,
			DstKind:   relationships.KindManifest,
			DstID:     id,
			Kind:      relationships.EdgeOwns,
			CreatedBy: author,
			Reason:    "manifest created under product",
		}); err != nil {
			return nil, fmt.Errorf("manifest created but relationships row failed: %w", err)
		}
	}

	return &Manifest{
		ID: id, Title: title, Description: description,
		Content: content, Status: status, JiraRefs: jiraRefs, Tags: tags,
		Author: author, SourceNode: sourceNode, ProjectID: projectID, DependsOn: dependsOn, Version: 1, CreatedAt: now, UpdatedAt: now,
	}, nil
}

// Update modifies an existing manifest and bumps version.
//
// Fires the terminal-transition handler (if wired) when status moves
// from a non-terminal value (draft/open) to a terminal one
// (closed/archive). The handler propagates the close into downstream
// waiting tasks — see SetTerminalTransitionHandler.
//
// Ownership re-parenting (projectID changing): post-PR/M3 the legacy
// `manifests.project_id` column is gone, so the parent product is no
// longer a column on the row. If projectID differs from the manifest's
// current parent (resolved via the relationships store), we close the
// prior EdgeOwns edge and open a new one. Empty projectID = "no
// parent"; the prior edge (if any) is closed without replacement.
func (s *Store) Update(id, title, description, content, status, projectID, dependsOn string, jiraRefs, tags []string) error {
	// Read prior status so we can detect the non-terminal → terminal
	// edge. An error here (e.g. row doesn't exist) falls through and
	// the UPDATE will either no-op or surface the real error — we
	// don't want to double-report on a missing row.
	var priorStatus string
	_ = s.db.QueryRow(`SELECT status FROM manifests WHERE id = ? AND deleted_at = ''`, id).Scan(&priorStatus)

	now := time.Now().UTC()
	jiraJSON, _ := json.Marshal(jiraRefs)
	tagsJSON, _ := json.Marshal(tags)

	_, err := s.db.Exec(`UPDATE manifests SET title=?, description=?, content=?, status=?, depends_on=?, jira_refs=?, tags=?,
		version=version+1, updated_at=? WHERE id=?`,
		title, description, content, status, dependsOn, string(jiraJSON), string(tagsJSON),
		now.Format(time.RFC3339), id)
	if err != nil {
		return err
	}

	// Reconcile ownership edge if the caller's projectID differs from
	// what relationships currently records. No-op when both are equal,
	// including when both are empty.
	if s.rels != nil {
		ctx := context.Background()
		priorProjectID := s.lookupOwner(ctx, id)
		if priorProjectID != projectID {
			if priorProjectID != "" {
				_ = s.rels.Remove(ctx, priorProjectID, id, relationships.EdgeOwns,
					"manifest.Store.Update", "manifest reparented")
			}
			if projectID != "" {
				if err := s.rels.Create(ctx, relationships.Edge{
					SrcKind:   relationships.KindProduct,
					SrcID:     projectID,
					DstKind:   relationships.KindManifest,
					DstID:     id,
					Kind:      relationships.EdgeOwns,
					CreatedBy: "manifest.Store.Update",
					Reason:    "manifest reparented",
				}); err != nil {
					return fmt.Errorf("update manifest ownership: %w", err)
				}
			}
		}
	}

	// Fire propagation ONLY on the non-terminal → terminal edge.
	// Repeated writes of an already-terminal status (e.g. closed →
	// closed after a no-op edit) must NOT re-trigger; the handler is
	// idempotent on its own but we save the work.
	if s.onTerminalTransition != nil &&
		!IsTerminalStatus(priorStatus) && IsTerminalStatus(status) {
		s.onTerminalTransition(context.Background(), id)
	}
	return nil
}

// lookupOwner returns the current product owner of a manifest from the
// relationships store, or "" if no current EdgeOwns row points at it.
// Used by Update to detect re-parenting and by the post-scan ownership
// populator to fill Manifest.ProjectID.
func (s *Store) lookupOwner(ctx context.Context, manifestID string) string {
	if s.rels == nil {
		return ""
	}
	edges, err := s.rels.ListIncoming(ctx, manifestID, relationships.EdgeOwns)
	if err != nil {
		return ""
	}
	for _, e := range edges {
		if e.SrcKind == relationships.KindProduct {
			return e.SrcID
		}
	}
	return ""
}

// populateOwnership fills ProjectID on a slice of manifests by issuing
// a single batched ListIncomingForMany lookup against the relationships
// store. Empty input is a no-op. Used by every read path (Get / List /
// ListByProject / Search / ListDeleted) so consumers continue to see
// `manifest.ProjectID` populated even though the column is gone.
func (s *Store) populateOwnership(manifests []*Manifest) {
	if s.rels == nil || len(manifests) == 0 {
		return
	}
	ctx := context.Background()
	ids := make([]string, 0, len(manifests))
	mapByID := make(map[string]*Manifest, len(manifests))
	for _, m := range manifests {
		if m == nil {
			continue
		}
		ids = append(ids, m.ID)
		mapByID[m.ID] = m
	}
	if len(ids) == 0 {
		return
	}
	byDst, err := s.rels.ListIncomingForMany(ctx, ids, relationships.EdgeOwns)
	if err != nil {
		return
	}
	for mid, edges := range byDst {
		m, ok := mapByID[mid]
		if !ok {
			continue
		}
		for _, e := range edges {
			if e.SrcKind == relationships.KindProduct {
				m.ProjectID = e.SrcID
				break
			}
		}
	}
}

// Get retrieves a manifest by full UUID. ProjectID is populated post-
// scan from the relationships store (the legacy column was dropped in
// PR/M3).
func (s *Store) Get(id string) (*Manifest, error) {
	row := s.db.QueryRow(`SELECT `+manifestColumns+`
		FROM manifests WHERE id = ? AND deleted_at = ''`, id)
	m, err := scanManifest(row)
	if err == nil && m != nil {
		s.populateOwnership([]*Manifest{m})
		s.EnrichWithCosts([]*Manifest{m})
	}
	return m, err
}

// ListByProject returns manifests belonging to a project, reading
// ownership from the relationships SCD-2 store via
// `EdgeOwns(product → manifest)`. Post-PR/M3 there is no legacy column
// fallback — the column is gone.
func (s *Store) ListByProject(projectID string, limit int) ([]*Manifest, error) {
	if limit <= 0 {
		limit = 50
	}
	if s.rels == nil {
		return nil, fmt.Errorf("manifest.ListByProject: relationships backend not wired")
	}
	ctx := context.Background()
	edges, err := s.rels.ListOutgoing(ctx, projectID, relationships.EdgeOwns)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(edges))
	for _, e := range edges {
		if e.DstKind == relationships.KindManifest {
			ids = append(ids, e.DstID)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, 0, len(ids)+1)
	for _, id := range ids {
		args = append(args, id)
	}
	args = append(args, limit)
	rows, err := s.db.Query(`SELECT `+manifestColumns+`
		FROM manifests WHERE id IN (`+placeholders+`) AND deleted_at = '' ORDER BY CASE status WHEN 'draft' THEN 0 WHEN 'open' THEN 1 WHEN 'closed' THEN 2 WHEN 'archive' THEN 3 ELSE 4 END, updated_at DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []*Manifest
	for rows.Next() {
		m, err := scanManifestRows(rows)
		if err != nil {
			return nil, err
		}
		// Caller already knows the projectID — short-circuit the
		// rels lookup for this hot path.
		m.ProjectID = projectID
		results = append(results, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

// List returns manifests sorted by updated_at.
//
// limit semantics:
//   - limit > 0  → cap at that many rows
//   - limit == 0 → UNBOUNDED (return every matching row). The caller is
//     declaring "I want everything." Used by apiTasksByPeer's bulk-
//     fetch optimization which needs every manifest to resolve task
//     groupings; without unbounded support, the handler silently
//     synthesised "Unknown" labels for tasks beyond the 50-row cap.
//   - limit < 0  → defensively treated as 50 (likely a caller bug)
//
// Was: any limit <= 0 collapsed to 50, which silently truncated the
// "give me all manifests" intent. 50-cap is fine for paginated views;
// it broke the bulk-fetch caller. 2026-04-25.
func (s *Store) List(status string, limit int) ([]*Manifest, error) {
	if limit < 0 {
		limit = 50 // defensive — negative is ambiguous, fall back to a sane cap
	}

	query := `SELECT ` + manifestColumns + ` FROM manifests WHERE deleted_at = ''`
	var args []any

	if status != "" {
		query += ` AND status = ?`
		args = append(args, status)
	}

	// Chronological — newest first. Same rationale as Product.List:
	// operators expect "what did I create most recently" at the top.
	query += ` ORDER BY created_at DESC`
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*Manifest
	for rows.Next() {
		m, err := scanManifestRows(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	s.populateOwnership(results)
	s.EnrichWithCosts(results)
	return results, nil
}

// Search finds manifests matching a query. Supports id-substring and
// keyword substring in title/description/content/jira_refs/tags.
func (s *Store) Search(query string, limit int) ([]*Manifest, error) {
	if limit <= 0 {
		limit = 20
	}
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, nil
	}
	pattern := "%" + q + "%"
	rows, err := s.db.Query(`SELECT `+manifestColumns+`
		FROM manifests WHERE deleted_at = '' AND (id LIKE ? OR title LIKE ? OR description LIKE ? OR content LIKE ? OR jira_refs LIKE ? OR tags LIKE ?)
		ORDER BY CASE status WHEN 'draft' THEN 0 WHEN 'open' THEN 1 WHEN 'closed' THEN 2 WHEN 'archive' THEN 3 ELSE 4 END, updated_at DESC LIMIT ?`, pattern, pattern, pattern, pattern, pattern, pattern, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*Manifest
	for rows.Next() {
		m, err := scanManifestRows(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	s.populateOwnership(results)
	s.EnrichWithCosts(results)
	return results, nil
}

// EnrichWithCosts populates TotalTasks, TotalTurns, and TotalCost from task_runs for each manifest.
func (s *Store) EnrichWithCosts(manifests []*Manifest) {
	if len(manifests) == 0 {
		return
	}
	mMap := make(map[string]*Manifest, len(manifests))
	ids := make([]string, 0, len(manifests))
	for _, m := range manifests {
		mMap[m.ID] = m
		ids = append(ids, m.ID)
	}

	// Resolve task ownership via the unified relationships store. The
	// legacy tasks.manifest_id JOIN was retired in PR/M3 — this path
	// is the sole source of truth.
	if s.rels == nil {
		return
	}
	ctx := context.Background()
	ownsByMan, err := s.rels.ListOutgoingForMany(ctx, ids, relationships.EdgeOwns)
	if err != nil {
		return
	}
	// Flatten to (taskID → manifestID) so a single SQL aggregate over
	// task_runs yields the per-manifest totals without N+1.
	taskToMan := make(map[string]string)
	allTaskIDs := []string{}
	for mID, edges := range ownsByMan {
		for _, e := range edges {
			if e.DstKind != relationships.KindTask {
				continue
			}
			taskToMan[e.DstID] = mID
			allTaskIDs = append(allTaskIDs, e.DstID)
		}
	}
	// Initialise per-manifest task counts from the ownership graph;
	// tasks with no runs still count toward TotalTasks.
	for mID := range mMap {
		edges := ownsByMan[mID]
		n := 0
		for _, e := range edges {
			if e.DstKind == relationships.KindTask {
				n++
			}
		}
		mMap[mID].TotalTasks = n
	}
	if len(allTaskIDs) == 0 {
		return
	}
	placeholders := strings.Repeat("?,", len(allTaskIDs))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, len(allTaskIDs))
	for i, id := range allTaskIDs {
		args[i] = id
	}
	rows, err := s.db.Query(`SELECT t.id, COALESCE(SUM(tr.turns),0), COALESCE(SUM(tr.cost_usd),0)
		FROM tasks t LEFT JOIN task_runs tr ON t.id = tr.task_id
		WHERE t.id IN (`+placeholders+`) AND t.deleted_at = ''
		GROUP BY t.id`, args...)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var tID string
		var turns int
		var cost float64
		if err := rows.Scan(&tID, &turns, &cost); err == nil {
			if mID, ok := taskToMan[tID]; ok {
				if m, ok := mMap[mID]; ok {
					m.TotalTurns += turns
					m.TotalCost += cost
				}
			}
		}
	}
}

// EnrichRecursiveCosts populates totals on a single manifest by summing
// task_runs across every task owned by this manifest. Used by the
// single-manifest GET so the dashboard surfaces actions + tokens
// alongside the existing turns + cost. Manifests don't have descendants
// the way products do (no recursive walk over manifest_dependencies);
// the "recursive" name is kept for symmetry with product.Store, but the
// query is a flat task_runs JOIN tasks WHERE manifest_id = m.id.
func (s *Store) EnrichRecursiveCosts(m *Manifest) {
	if m == nil {
		return
	}
	// Resolve owned task IDs through the relationships store; aggregate
	// task_runs WHERE task_id IN (...). The legacy tasks.manifest_id
	// JOIN was retired in PR/M3.
	if s.rels == nil {
		return
	}
	ctx := context.Background()
	edges, err := s.rels.ListOutgoing(ctx, m.ID, relationships.EdgeOwns)
	if err != nil {
		return
	}
	taskIDs := make([]string, 0, len(edges))
	for _, e := range edges {
		if e.DstKind == relationships.KindTask {
			taskIDs = append(taskIDs, e.DstID)
		}
	}
	if len(taskIDs) == 0 {
		m.TotalTasks = 0
		m.TotalTurns = 0
		m.TotalCost = 0
		m.TotalActions = 0
		m.TotalTokens = 0
		return
	}
	placeholders := strings.Repeat("?,", len(taskIDs))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, len(taskIDs))
	for i, id := range taskIDs {
		args[i] = id
	}
	row := s.db.QueryRow(`
		SELECT
			COUNT(DISTINCT t.id),
			COALESCE(SUM(tr.turns), 0),
			COALESCE(SUM(tr.cost_usd), 0),
			COALESCE(SUM(tr.actions), 0),
			COALESCE(SUM(tr.input_tokens + tr.output_tokens + tr.cache_read_tokens + tr.cache_create_tokens), 0)
		FROM tasks t
		LEFT JOIN task_runs tr ON tr.task_id = t.id
		WHERE t.id IN (`+placeholders+`) AND t.deleted_at = ''`,
		args...,
	)
	var tasks, turns, actions, tokens int
	var cost float64
	if err := row.Scan(&tasks, &turns, &cost, &actions, &tokens); err == nil {
		m.TotalTasks = tasks
		m.TotalTurns = turns
		m.TotalCost = cost
		m.TotalActions = actions
		m.TotalTokens = tokens
	}
}

// Delete soft-deletes a manifest.
func (s *Store) Delete(id string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`UPDATE manifests SET deleted_at = ? WHERE id = ? AND deleted_at = ''`, now, id)
	return err
}

// ListDeleted returns soft-deleted manifests.
func (s *Store) ListDeleted(limit int) ([]*Manifest, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT `+manifestColumns+`
		FROM manifests WHERE deleted_at != '' ORDER BY deleted_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []*Manifest
	for rows.Next() {
		m, err := scanManifestRows(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	s.populateOwnership(results)
	return results, nil
}

// Restore un-deletes a soft-deleted manifest.
func (s *Store) Restore(id string) error {
	_, err := s.db.Exec(`UPDATE manifests SET deleted_at = '' WHERE id = ? AND deleted_at != ''`, id)
	return err
}

// Count returns total manifests.
func (s *Store) Count() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM manifests WHERE deleted_at = ''`).Scan(&count)
	return count, err
}

// ParseDependsOn splits the comma-separated depends_on string into a list of manifest IDs.
func (m *Manifest) ParseDependsOn() []string {
	if m.DependsOn == "" {
		return nil
	}
	parts := strings.Split(m.DependsOn, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// HasDependency checks if a manifest depends on the given manifest ID.
func (m *Manifest) HasDependency(manifestID string) bool {
	for _, dep := range m.ParseDependsOn() {
		if dep == manifestID {
			return true
		}
	}
	return false
}

// manifestColumns is the canonical SELECT projection for manifests.
// Mirrors scanManifest / scanManifestRows; PR/M3 dropped project_id
// from this list along with the underlying column. Manifest.ProjectID
// is now populated post-scan via populateOwnership / explicit assignment.
const manifestColumns = `id, title, description, content, status, jira_refs, tags, author, source_node, depends_on, version, created_at, updated_at`

func scanManifest(row *sql.Row) (*Manifest, error) {
	var m Manifest
	var jiraStr, tagsStr, createdStr, updatedStr string

	err := row.Scan(&m.ID, &m.Title, &m.Description, &m.Content, &m.Status,
		&jiraStr, &tagsStr, &m.Author, &m.SourceNode, &m.DependsOn, &m.Version, &createdStr, &updatedStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(jiraStr), &m.JiraRefs); err != nil {
		slog.Warn("unmarshal manifest field failed", "field", "jira_refs", "error", err)
	}
	if err := json.Unmarshal([]byte(tagsStr), &m.Tags); err != nil {
		slog.Warn("unmarshal manifest field failed", "field", "tags", "error", err)
	}
	m.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	m.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)

	return &m, nil
}

func scanManifestRows(rows *sql.Rows) (*Manifest, error) {
	var m Manifest
	var jiraStr, tagsStr, createdStr, updatedStr string

	err := rows.Scan(&m.ID, &m.Title, &m.Description, &m.Content, &m.Status,
		&jiraStr, &tagsStr, &m.Author, &m.SourceNode, &m.DependsOn, &m.Version, &createdStr, &updatedStr)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(jiraStr), &m.JiraRefs); err != nil {
		slog.Warn("unmarshal manifest field failed", "field", "jira_refs", "error", err)
	}
	if err := json.Unmarshal([]byte(tagsStr), &m.Tags); err != nil {
		slog.Warn("unmarshal manifest field failed", "field", "tags", "error", err)
	}
	m.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	m.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)

	return &m, nil
}
