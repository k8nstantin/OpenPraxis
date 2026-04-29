package product

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

// Product is a top-level organizational entity: Peer → Product → Manifest → Task.
type Product struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      string    `json:"status"`      // draft, open, closed, archive
	SourceNode  string    `json:"source_node"` // peer UUID
	Tags        []string  `json:"tags"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// Computed — aggregated from manifests → tasks → task_runs.
	// Recursive sums (across descendants in product_dependencies) live
	// alongside the direct-only sums on the same struct; populated by
	// EnrichRecursiveCosts when the dashboard fetches a single product.
	TotalManifests int     `json:"total_manifests"`
	TotalTasks     int     `json:"total_tasks"`
	TotalTurns     int     `json:"total_turns"`
	TotalCost      float64 `json:"total_cost"`
	TotalActions   int     `json:"total_actions"`
	TotalTokens    int     `json:"total_tokens"`
}

// Store manages product persistence.
type Store struct {
	db *sql.DB
	// rels is the unified relationships SCD-2 store. Wired post-init via
	// SetRelationshipsBackend so the package import direction (relationships
	// has no internal deps) stays one-way. All dependency reads + writes
	// route through this; the legacy product_dependencies table is dormant.
	rels *relationships.Store
	// onTerminalTransition fires AFTER a successful Update that moved
	// the product from a non-terminal status (draft/open) into a
	// terminal one (closed/archive). Wired in node.go to
	// task.Store.PropagateProductClosed so waiting tasks in downstream
	// products' manifests auto-activate. Nil disables.
	onTerminalTransition func(ctx context.Context, productID string)
	// onDepRemoved fires AFTER a successful RemoveDep. Wired handler
	// rehabs now-unblocked waiting tasks from the product's manifests
	// into 'pending' per Option B. Nil disables.
	onDepRemoved func(ctx context.Context, productID string)
}

// SetTerminalTransitionHandler registers the callback fired when a
// product moves into a terminal status. Call once at startup; no mutex
// because production wiring runs before any HTTP/MCP handler is
// serving.
func (s *Store) SetTerminalTransitionHandler(fn func(ctx context.Context, productID string)) {
	s.onTerminalTransition = fn
}

// SetDepRemovedHandler registers the callback fired after RemoveDep.
// The handler rehabs product-blocked tasks per Option B.
func (s *Store) SetDepRemovedHandler(fn func(ctx context.Context, productID string)) {
	s.onDepRemoved = fn
}

// SetRelationshipsBackend wires the unified relationships SCD-2 store
// for product dependency / ownership edges. Call once at startup before
// any mutation runs.
func (s *Store) SetRelationshipsBackend(r *relationships.Store) {
	s.rels = r
}

// NewStore creates a product store. The dependency layer auto-wires a
// default relationships backend against the same DB handle so tests
// (and any caller that doesn't explicitly call SetRelationshipsBackend)
// see a working dep API. The node-level wiring overrides this with the
// shared singleton store when the production graph spins up.
func NewStore(db *sql.DB) (*Store, error) {
	s := &Store{db: db}
	if err := s.init(); err != nil {
		return nil, err
	}
	if err := s.initDependenciesSchema(); err != nil {
		return nil, err
	}
	rels, err := relationships.New(db)
	if err != nil {
		return nil, fmt.Errorf("init relationships backend: %w", err)
	}
	s.rels = rels
	s.logSchemaReady()
	return s, nil
}

func (s *Store) init() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS products (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'draft',
		source_node TEXT NOT NULL DEFAULT '',
		tags TEXT NOT NULL DEFAULT '[]',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		deleted_at TEXT NOT NULL DEFAULT ''
	)`)
	if err != nil {
		return fmt.Errorf("create products table: %w", err)
	}

	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_products_status ON products(status)`)
	if err != nil {
		return fmt.Errorf("create products status index: %w", err)
	}

	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_products_updated ON products(updated_at DESC)`)
	if err != nil {
		return err
	}

	return nil
}

// Create stores a new product.
func (s *Store) Create(title, description, status, sourceNode string, tags []string) (*Product, error) {
	if status == "" {
		status = "open"
	}
	if tags == nil {
		tags = []string{}
	}

	id := uuid.Must(uuid.NewV7()).String()
	now := time.Now().UTC()
	tagsJSON, _ := json.Marshal(tags)

	_, err := s.db.Exec(`INSERT INTO products (id, title, description, status, source_node, tags, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, title, description, status, sourceNode, string(tagsJSON),
		now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}

	return &Product{
		ID: id, Title: title, Description: description,
		Status: status, SourceNode: sourceNode, Tags: tags,
		CreatedAt: now, UpdatedAt: now,
	}, nil
}

// Get retrieves a product by full UUID.
func (s *Store) Get(id string) (*Product, error) {
	row := s.db.QueryRow(`SELECT id, title, description, status, source_node, tags, created_at, updated_at
		FROM products WHERE id = ? AND deleted_at = ''`, id)
	p, err := scanProduct(row)
	if err == nil && p != nil {
		s.EnrichWithCosts([]*Product{p})
	}
	return p, err
}

// List returns products sorted by updated_at.
//
// limit semantics:
//   - limit > 0  → cap at that many rows
//   - limit == 0 → UNBOUNDED (return every matching row). Used by the
//     dashboard list pane which paginates client-side at PAGE_SIZE=10
//     with "Load more" → the backend MUST return every row, not the
//     prior arbitrary 50-row cap that silently truncated.
//   - limit < 0  → defensively treated as 50 (likely a caller bug)
func (s *Store) List(status string, limit int) ([]*Product, error) {
	if limit < 0 {
		limit = 50
	}

	query := `SELECT id, title, description, status, source_node, tags, created_at, updated_at FROM products WHERE deleted_at = ''`
	var args []any

	if status != "" {
		query += ` AND status = ?`
		args = append(args, status)
	}

	// Chronological — newest first. Status-first ordering hid newly
	// created products under the wall of legacy drafts; chronological
	// matches what the operator actually wants when they open the list.
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

	var results []*Product
	for rows.Next() {
		p, err := scanProductRows(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, p)
	}
	s.EnrichWithCosts(results)
	return results, rows.Err()
}

// Update modifies a product. Fires the terminal-transition handler
// (if wired) when status moves from a non-terminal value (draft/open)
// to a terminal one (closed/archive). The handler propagates the
// close into downstream products' task queues — see
// SetTerminalTransitionHandler.
func (s *Store) Update(id, title, description, status string, tags []string) error {
	// Read prior status before the UPDATE so we can detect the
	// non-terminal → terminal edge. Missing row falls through; the
	// UPDATE will either be a no-op or surface the real error.
	var priorStatus, fullID string
	_ = s.db.QueryRow(`SELECT id, status FROM products WHERE id = ? AND deleted_at = ''`,
		id).Scan(&fullID, &priorStatus)

	now := time.Now().UTC().Format(time.RFC3339)
	tagsJSON, _ := json.Marshal(tags)
	_, err := s.db.Exec(`UPDATE products SET title = ?, description = ?, status = ?, tags = ?, updated_at = ?
		WHERE id = ? AND deleted_at = ''`,
		title, description, status, string(tagsJSON), now, id)
	if err != nil {
		return err
	}

	// Fire propagation only on the non-terminal → terminal edge.
	// Repeat writes of an already-terminal status (closed → closed)
	// must not re-trigger; the handler is idempotent on its own but
	// we save the walk.
	if s.onTerminalTransition != nil && fullID != "" &&
		!IsTerminalStatus(priorStatus) && IsTerminalStatus(status) {
		s.onTerminalTransition(context.Background(), fullID)
	}
	return nil
}

// Delete soft-deletes a product.
func (s *Store) Delete(id string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`UPDATE products SET deleted_at = ? WHERE id = ? AND deleted_at = ''`, now, id)
	return err
}

// EnrichRecursiveCosts populates totals on a single product by walking
// descendant sub-products via the unified relationships store and
// summing their manifests → tasks → task_runs. Used by the single-
// product GET so a product whose tasks live under sub-product
// manifests still surfaces the cumulative cost on its dashboard.
// Direct-only EnrichWithCosts stays in place for the list endpoints
// (cheaper, batched).
func (s *Store) EnrichRecursiveCosts(p *Product) {
	if p == nil || s.rels == nil {
		return
	}
	ctx := context.Background()
	// Walk follows EdgeDependsOn outwards. For products this is
	// "depender → dependee", which in the umbrella convention means
	// "umbrella → sub-product" — exactly the descendant set we want
	// to roll costs up from. Self-row included at depth 0.
	rows, err := s.rels.Walk(ctx, p.ID, relationships.KindProduct,
		[]string{relationships.EdgeDependsOn}, relationships.MaxWalkDepth)
	if err != nil {
		return
	}
	descendantIDs := make([]string, 0, len(rows))
	for _, r := range rows {
		if r.Kind != relationships.KindProduct {
			continue
		}
		descendantIDs = append(descendantIDs, r.ID)
	}
	if len(descendantIDs) == 0 {
		return
	}
	placeholders := strings.Repeat("?,", len(descendantIDs))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, len(descendantIDs))
	for i, id := range descendantIDs {
		args[i] = id
	}
	// Resolve owned manifests + their tasks via the relationships store
	// instead of the legacy `m.project_id` / `t.manifest_id` JOINs.
	// Owns(product→manifest) for each descendant product, then
	// Owns(manifest→task) for each manifest. Aggregate task_runs over
	// the union of task IDs.
	manifestEdges, err := s.rels.ListOutgoingForMany(ctx, descendantIDs, relationships.EdgeOwns)
	if err != nil {
		return
	}
	manifestIDs := []string{}
	for _, edges := range manifestEdges {
		for _, e := range edges {
			if e.DstKind == relationships.KindManifest {
				manifestIDs = append(manifestIDs, e.DstID)
			}
		}
	}
	if len(manifestIDs) == 0 {
		p.TotalManifests = 0
		p.TotalTasks = 0
		p.TotalTurns = 0
		p.TotalCost = 0
		p.TotalActions = 0
		p.TotalTokens = 0
		return
	}
	taskEdges, err := s.rels.ListOutgoingForMany(ctx, manifestIDs, relationships.EdgeOwns)
	if err != nil {
		return
	}
	taskIDs := []string{}
	for _, edges := range taskEdges {
		for _, e := range edges {
			if e.DstKind == relationships.KindTask {
				taskIDs = append(taskIDs, e.DstID)
			}
		}
	}
	if len(taskIDs) == 0 {
		// Manifests exist but no tasks; report manifest count.
		p.TotalManifests = len(manifestIDs)
		p.TotalTasks = 0
		p.TotalTurns = 0
		p.TotalCost = 0
		p.TotalActions = 0
		p.TotalTokens = 0
		return
	}
	taskPH := strings.Repeat("?,", len(taskIDs))
	taskPH = taskPH[:len(taskPH)-1]
	taskArgs := make([]any, len(taskIDs))
	for i, id := range taskIDs {
		taskArgs[i] = id
	}
	row := s.db.QueryRow(fmt.Sprintf(`
		SELECT
			COUNT(DISTINCT t.id),
			COALESCE(SUM(tr.turns), 0),
			COALESCE(SUM(tr.cost_usd), 0),
			COALESCE(SUM(tr.actions), 0),
			COALESCE(SUM(tr.input_tokens + tr.output_tokens + tr.cache_read_tokens + tr.cache_create_tokens), 0)
		FROM tasks t
		LEFT JOIN task_runs tr ON tr.task_id = t.id
		WHERE t.id IN (%s) AND t.deleted_at = ''`, taskPH),
		taskArgs...,
	)
	var tasks, turns, actions, tokens int
	var cost float64
	if err := row.Scan(&tasks, &turns, &cost, &actions, &tokens); err == nil {
		p.TotalManifests = len(manifestIDs)
		p.TotalTasks = tasks
		p.TotalTurns = turns
		p.TotalCost = cost
		p.TotalActions = actions
		p.TotalTokens = tokens
	}
}

// EnrichWithCosts populates TotalManifests, TotalTasks, TotalTurns,
// TotalCost from manifests → tasks → task_runs. Direct-only (no
// recursive descend) — list endpoints use this for the cheaper path.
// Reads ownership through the relationships store; legacy JOIN is the
// fallback when rels isn't wired.
func (s *Store) EnrichWithCosts(products []*Product) {
	if len(products) == 0 {
		return
	}
	pMap := make(map[string]*Product, len(products))
	ids := make([]string, len(products))
	for i, p := range products {
		pMap[p.ID] = p
		ids[i] = p.ID
	}

	// Resolve manifest + task ownership through the relationships store.
	// PR/M3 dropped the legacy m.project_id / t.manifest_id JOIN; rels
	// is now the sole source of truth.
	if s.rels == nil {
		return
	}
	ctx := context.Background()
	ownsByProduct, err := s.rels.ListOutgoingForMany(ctx, ids, relationships.EdgeOwns)
	if err != nil {
		return
	}
	// (manifestID → productID) so a single SQL aggregate over task_runs
	// rolls up by product without N+1.
	manifestToProduct := make(map[string]string)
	perProductManifestCount := make(map[string]int)
	allManifestIDs := []string{}
	for pid, edges := range ownsByProduct {
		for _, e := range edges {
			if e.DstKind != relationships.KindManifest {
				continue
			}
			manifestToProduct[e.DstID] = pid
			perProductManifestCount[pid]++
			allManifestIDs = append(allManifestIDs, e.DstID)
		}
	}
	for pid, n := range perProductManifestCount {
		if p, ok := pMap[pid]; ok {
			p.TotalManifests = n
		}
	}
	if len(allManifestIDs) == 0 {
		return
	}
	taskEdgesByManifest, err := s.rels.ListOutgoingForMany(ctx, allManifestIDs, relationships.EdgeOwns)
	if err != nil {
		return
	}
	taskToProduct := make(map[string]string)
	perProductTaskCount := make(map[string]int)
	allTaskIDs := []string{}
	for mID, edges := range taskEdgesByManifest {
		pid, ok := manifestToProduct[mID]
		if !ok {
			continue
		}
		for _, e := range edges {
			if e.DstKind != relationships.KindTask {
				continue
			}
			taskToProduct[e.DstID] = pid
			perProductTaskCount[pid]++
			allTaskIDs = append(allTaskIDs, e.DstID)
		}
	}
	for pid, n := range perProductTaskCount {
		if p, ok := pMap[pid]; ok {
			p.TotalTasks = n
		}
	}
	if len(allTaskIDs) == 0 {
		return
	}
	ph := strings.Repeat("?,", len(allTaskIDs))
	ph = ph[:len(ph)-1]
	tArgs := make([]any, len(allTaskIDs))
	for i, id := range allTaskIDs {
		tArgs[i] = id
	}
	rows, err := s.db.Query(`SELECT t.id,
		COALESCE(SUM(tr.turns), 0),
		COALESCE(SUM(tr.cost_usd), 0)
		FROM tasks t LEFT JOIN task_runs tr ON tr.task_id = t.id
		WHERE t.id IN (`+ph+`) AND t.deleted_at = ''
		GROUP BY t.id`, tArgs...)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var tid string
		var turns int
		var cost float64
		if err := rows.Scan(&tid, &turns, &cost); err == nil {
			if pid, ok := taskToProduct[tid]; ok {
				if p, ok := pMap[pid]; ok {
					p.TotalTurns += turns
					p.TotalCost += cost
				}
			}
		}
	}
}

func scanProduct(row *sql.Row) (*Product, error) {
	var p Product
	var tagsStr, createdStr, updatedStr string

	err := row.Scan(&p.ID, &p.Title, &p.Description, &p.Status,
		&p.SourceNode, &tagsStr, &createdStr, &updatedStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(tagsStr), &p.Tags); err != nil {
		slog.Warn("unmarshal product field failed", "field", "tags", "error", err)
	}
	p.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	p.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)

	return &p, nil
}

func scanProductRows(rows *sql.Rows) (*Product, error) {
	var p Product
	var tagsStr, createdStr, updatedStr string

	err := rows.Scan(&p.ID, &p.Title, &p.Description, &p.Status,
		&p.SourceNode, &tagsStr, &createdStr, &updatedStr)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(tagsStr), &p.Tags); err != nil {
		slog.Warn("unmarshal product field failed", "field", "tags", "error", err)
	}
	p.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	p.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)

	return &p, nil
}
