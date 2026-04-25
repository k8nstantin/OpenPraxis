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
)

// Manifest is a detailed development spec document.
type Manifest struct {
	ID          string    `json:"id"`
	Marker      string    `json:"marker"`
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

	// Computed from linked tasks — populated by EnrichWithCosts()
	TotalTasks int     `json:"total_tasks"`
	TotalTurns int     `json:"total_turns"`
	TotalCost  float64 `json:"total_cost"`
}

// Store manages manifest persistence.
type Store struct {
	db *sql.DB
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
	// One-shot backfill of the legacy comma-separated depends_on column
	// into the new join table. Idempotent via PRIMARY KEY dedup; safe to
	// run every boot until the column is retired in a follow-up PR.
	if _, err := s.BackfillLegacyDependsOn(context.Background()); err != nil {
		return nil, fmt.Errorf("backfill manifest dependencies: %w", err)
	}
	return s, nil
}

func (s *Store) init() error {
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
	s.db.Exec(`ALTER TABLE manifests ADD COLUMN source_node TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`ALTER TABLE manifests ADD COLUMN deleted_at TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`ALTER TABLE manifests ADD COLUMN project_id TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_manifests_project ON manifests(project_id)`)
	s.db.Exec(`ALTER TABLE manifests ADD COLUMN depends_on TEXT NOT NULL DEFAULT ''`)
	return nil
}

// Create stores a new manifest.
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

	_, err := s.db.Exec(`INSERT INTO manifests (id, title, description, content, status, jira_refs, tags, author, source_node, project_id, depends_on, version, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
		id, title, description, content, status, string(jiraJSON), string(tagsJSON), author, sourceNode, projectID, dependsOn,
		now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}

	return &Manifest{
		ID: id, Marker: id[:12], Title: title, Description: description,
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

	_, err := s.db.Exec(`UPDATE manifests SET title=?, description=?, content=?, status=?, project_id=?, depends_on=?, jira_refs=?, tags=?,
		version=version+1, updated_at=? WHERE id=?`,
		title, description, content, status, projectID, dependsOn, string(jiraJSON), string(tagsJSON),
		now.Format(time.RFC3339), id)
	if err != nil {
		return err
	}

	// Fire propagation ONLY on the non-terminal → terminal edge.
	// Repeated writes of an already-terminal status (e.g. closed →
	// closed after a no-op edit) must NOT re-trigger; the handler is
	// idempotent on its own but we save the work.
	if s.onTerminalTransition != nil &&
		!IsTerminalStatus(priorStatus) && IsTerminalStatus(status) {
		// Fire in-process. If the handler blocks, the Update returns
		// only after propagation completes — intentional, so the
		// operator sees "close + downstream activated" as one atomic
		// unit from their UI's POV. Handlers that want async behavior
		// should spawn internally.
		s.onTerminalTransition(context.Background(), id)
	}
	return nil
}

// Get retrieves a manifest by ID or prefix.
func (s *Store) Get(id string) (*Manifest, error) {
	row := s.db.QueryRow(`SELECT id, title, description, content, status, jira_refs, tags, author, source_node, project_id, depends_on, version, created_at, updated_at
		FROM manifests WHERE (id = ? OR id LIKE ?) AND deleted_at = ''`, id, id+"%")
	m, err := scanManifest(row)
	if err == nil && m != nil {
		s.EnrichWithCosts([]*Manifest{m})
	}
	return m, err
}

// ListByProject returns manifests belonging to a project.
func (s *Store) ListByProject(projectID string, limit int) ([]*Manifest, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT id, title, description, content, status, jira_refs, tags, author, source_node, project_id, depends_on, version, created_at, updated_at
		FROM manifests WHERE project_id = ? AND deleted_at = '' ORDER BY CASE status WHEN 'draft' THEN 0 WHEN 'open' THEN 1 WHEN 'closed' THEN 2 WHEN 'archive' THEN 3 ELSE 4 END, updated_at DESC LIMIT ?`, projectID, limit)
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
	return results, rows.Err()
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

	query := `SELECT id, title, description, content, status, jira_refs, tags, author, source_node, project_id, depends_on, version, created_at, updated_at FROM manifests WHERE deleted_at = ''`
	var args []any

	if status != "" {
		query += ` AND status = ?`
		args = append(args, status)
	}

	query += ` ORDER BY CASE status WHEN 'draft' THEN 0 WHEN 'open' THEN 1 WHEN 'closed' THEN 2 WHEN 'archive' THEN 3 ELSE 4 END, updated_at DESC`
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
	s.EnrichWithCosts(results)
	return results, rows.Err()
}

// Search finds manifests matching a query. Supports id-exact, id-prefix
// (marker or UUID prefix), id-substring, and keyword substring in
// title/description/content/jira_refs/tags. Previously this only
// matched content fields, so typing a marker returned zero results —
// see manifest 019daafb-b5e M6 for the root-cause write-up.
func (s *Store) Search(query string, limit int) ([]*Manifest, error) {
	if limit <= 0 {
		limit = 20
	}
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, nil
	}
	pattern := "%" + q + "%"
	rows, err := s.db.Query(`SELECT id, title, description, content, status, jira_refs, tags, author, source_node, project_id, depends_on, version, created_at, updated_at
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
	s.EnrichWithCosts(results)
	return results, rows.Err()
}

// EnrichWithCosts populates TotalTasks, TotalTurns, and TotalCost from task_runs for each manifest.
func (s *Store) EnrichWithCosts(manifests []*Manifest) {
	if len(manifests) == 0 {
		return
	}
	mMap := make(map[string]*Manifest, len(manifests))
	for _, m := range manifests {
		mMap[m.ID] = m
	}

	// Aggregate cost from task_runs via tasks.manifest_id
	rows, err := s.db.Query(`SELECT t.manifest_id, COUNT(DISTINCT t.id), COALESCE(SUM(tr.turns),0), COALESCE(SUM(tr.cost_usd),0)
		FROM tasks t LEFT JOIN task_runs tr ON t.id = tr.task_id
		WHERE t.manifest_id != '' AND t.deleted_at = ''
		GROUP BY t.manifest_id`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var mid string
		var tasks, turns int
		var cost float64
		if err := rows.Scan(&mid, &tasks, &turns, &cost); err == nil {
			if m, ok := mMap[mid]; ok {
				m.TotalTasks = tasks
				m.TotalTurns = turns
				m.TotalCost = cost
			}
		}
	}
}

// Delete soft-deletes a manifest.
func (s *Store) Delete(id string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`UPDATE manifests SET deleted_at = ? WHERE (id = ? OR id LIKE ?) AND deleted_at = ''`, now, id, id+"%")
	return err
}

// ListDeleted returns soft-deleted manifests.
func (s *Store) ListDeleted(limit int) ([]*Manifest, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT id, title, description, content, status, jira_refs, tags, author, source_node, project_id, depends_on, version, created_at, updated_at
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
	return results, rows.Err()
}

// Restore un-deletes a soft-deleted manifest.
func (s *Store) Restore(id string) error {
	_, err := s.db.Exec(`UPDATE manifests SET deleted_at = '' WHERE (id = ? OR id LIKE ?) AND deleted_at != ''`, id, id+"%")
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

func scanManifest(row *sql.Row) (*Manifest, error) {
	var m Manifest
	var jiraStr, tagsStr, createdStr, updatedStr string

	err := row.Scan(&m.ID, &m.Title, &m.Description, &m.Content, &m.Status,
		&jiraStr, &tagsStr, &m.Author, &m.SourceNode, &m.ProjectID, &m.DependsOn, &m.Version, &createdStr, &updatedStr)
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
	if len(m.ID) >= 12 {
		m.Marker = m.ID[:12]
	}

	return &m, nil
}

func scanManifestRows(rows *sql.Rows) (*Manifest, error) {
	var m Manifest
	var jiraStr, tagsStr, createdStr, updatedStr string

	err := rows.Scan(&m.ID, &m.Title, &m.Description, &m.Content, &m.Status,
		&jiraStr, &tagsStr, &m.Author, &m.SourceNode, &m.ProjectID, &m.DependsOn, &m.Version, &createdStr, &updatedStr)
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
	if len(m.ID) >= 12 {
		m.Marker = m.ID[:12]
	}

	return &m, nil
}
