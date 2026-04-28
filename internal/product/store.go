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
)

// Product is a top-level organizational entity: Peer → Product → Manifest → Task.
type Product struct {
	ID          string    `json:"id"`
	Marker      string    `json:"marker"`
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

// NewStore creates a product store.
func NewStore(db *sql.DB) (*Store, error) {
	s := &Store{db: db}
	if err := s.init(); err != nil {
		return nil, err
	}
	if err := s.initDependenciesSchema(); err != nil {
		return nil, err
	}
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
		ID: id, Marker: id[:12], Title: title, Description: description,
		Status: status, SourceNode: sourceNode, Tags: tags,
		CreatedAt: now, UpdatedAt: now,
	}, nil
}

// Get retrieves a product by ID or prefix.
func (s *Store) Get(id string) (*Product, error) {
	row := s.db.QueryRow(`SELECT id, title, description, status, source_node, tags, created_at, updated_at
		FROM products WHERE (id = ? OR id LIKE ?) AND deleted_at = ''`, id, id+"%")
	p, err := scanProduct(row)
	if err == nil && p != nil {
		s.EnrichWithCosts([]*Product{p})
	}
	return p, err
}

// List returns products sorted by updated_at.
func (s *Store) List(status string, limit int) ([]*Product, error) {
	if limit <= 0 {
		limit = 50
	}

	query := `SELECT id, title, description, status, source_node, tags, created_at, updated_at FROM products WHERE deleted_at = ''`
	var args []any

	if status != "" {
		query += ` AND status = ?`
		args = append(args, status)
	}

	query += ` ORDER BY CASE status WHEN 'draft' THEN 0 WHEN 'open' THEN 1 WHEN 'closed' THEN 2 WHEN 'archive' THEN 3 ELSE 4 END, updated_at DESC LIMIT ?`
	args = append(args, limit)

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
	_ = s.db.QueryRow(`SELECT id, status FROM products WHERE (id = ? OR id LIKE ?) AND deleted_at = ''`,
		id, id+"%").Scan(&fullID, &priorStatus)

	now := time.Now().UTC().Format(time.RFC3339)
	tagsJSON, _ := json.Marshal(tags)
	_, err := s.db.Exec(`UPDATE products SET title = ?, description = ?, status = ?, tags = ?, updated_at = ?
		WHERE (id = ? OR id LIKE ?) AND deleted_at = ''`,
		title, description, status, string(tagsJSON), now, id, id+"%")
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
	_, err := s.db.Exec(`UPDATE products SET deleted_at = ? WHERE (id = ? OR id LIKE ?) AND deleted_at = ''`, now, id, id+"%")
	return err
}

// EnrichRecursiveCosts populates totals on a single product by walking
// descendant sub-products via product_dependencies and summing their
// manifests → tasks → task_runs. Used by the single-product GET so a
// product whose tasks live under sub-product manifests still surfaces
// the cumulative cost on its dashboard. Direct-only EnrichWithCosts
// stays in place for the list endpoints (cheaper, batched).
func (s *Store) EnrichRecursiveCosts(p *Product) {
	if p == nil {
		return
	}
	// Edge direction in product_dependencies: parent.product_id →
	// child.depends_on_product_id (umbrella has rows with product_id=
	// itself for each sub-product). Walk that to collect descendants.
	row := s.db.QueryRow(`
		WITH RECURSIVE descendants(id) AS (
			SELECT ?
			UNION ALL
			SELECT pd.depends_on_product_id FROM product_dependencies pd
			INNER JOIN descendants d ON pd.product_id = d.id
		)
		SELECT
			COUNT(DISTINCT m.id),
			COUNT(DISTINCT t.id),
			COALESCE(SUM(tr.turns), 0),
			COALESCE(SUM(tr.cost_usd), 0),
			COALESCE(SUM(tr.actions), 0),
			COALESCE(SUM(tr.input_tokens + tr.output_tokens + tr.cache_read_tokens + tr.cache_create_tokens), 0)
		FROM descendants d
		LEFT JOIN manifests m ON m.project_id = d.id AND m.deleted_at = ''
		LEFT JOIN tasks t ON t.manifest_id = m.id AND t.deleted_at = ''
		LEFT JOIN task_runs tr ON tr.task_id = t.id`,
		p.ID,
	)
	var manifests, tasks, turns, actions, tokens int
	var cost float64
	if err := row.Scan(&manifests, &tasks, &turns, &cost, &actions, &tokens); err == nil {
		p.TotalManifests = manifests
		p.TotalTasks = tasks
		p.TotalTurns = turns
		p.TotalCost = cost
		p.TotalActions = actions
		p.TotalTokens = tokens
	}
}

// EnrichWithCosts populates TotalManifests, TotalTasks, TotalTurns, TotalCost from manifests → tasks → task_runs.
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

	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}

	rows, err := s.db.Query(
		fmt.Sprintf(`SELECT m.project_id,
			COUNT(DISTINCT m.id),
			COUNT(DISTINCT t.id),
			COALESCE(SUM(tr.turns), 0),
			COALESCE(SUM(tr.cost_usd), 0)
		FROM manifests m
		LEFT JOIN tasks t ON t.manifest_id = m.id AND t.deleted_at = ''
		LEFT JOIN task_runs tr ON tr.task_id = t.id
		WHERE m.project_id IN (%s) AND m.deleted_at = ''
		GROUP BY m.project_id`, placeholders),
		args...,
	)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var pid string
		var manifests, tasks, turns int
		var cost float64
		if err := rows.Scan(&pid, &manifests, &tasks, &turns, &cost); err == nil {
			if p, ok := pMap[pid]; ok {
				p.TotalManifests = manifests
				p.TotalTasks = tasks
				p.TotalTurns = turns
				p.TotalCost = cost
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
	if len(p.ID) >= 12 {
		p.Marker = p.ID[:12]
	}

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
	if len(p.ID) >= 12 {
		p.Marker = p.ID[:12]
	}

	return &p, nil
}
