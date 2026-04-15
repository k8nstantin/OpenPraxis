package memory

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
)

func init() {
	sqlite_vec.Auto()
}

// Index is the local vector search index backed by SQLite + sqlite-vec.
type Index struct {
	db        *sql.DB
	dimension int
}

// SearchResult represents a single search result with score.
type SearchResult struct {
	Memory   Memory  `json:"memory"`
	Distance float64 `json:"distance"`
	Score    float64 `json:"score"`
}

// TreeNode represents a node in the memory hierarchy tree.
type TreeNode struct {
	Path     string      `json:"path"`
	Count    int         `json:"count"`
	Children []*TreeNode `json:"children,omitempty"`
}

// NewIndex opens or creates the SQLite+vec database.
func NewIndex(dataDir string, dimension int) (*Index, error) {
	dbPath := filepath.Join(dataDir, "memories.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	// WAL mode allows concurrent readers + single writer without blocking
	db.Exec("PRAGMA journal_mode=WAL")
	db.Exec("PRAGMA busy_timeout=5000")

	idx := &Index{db: db, dimension: dimension}
	if err := idx.init(); err != nil {
		db.Close()
		return nil, err
	}
	return idx, nil
}

func (idx *Index) init() error {
	// Metadata table
	_, err := idx.db.Exec(`CREATE TABLE IF NOT EXISTS memories (
		id TEXT PRIMARY KEY,
		path TEXT NOT NULL,
		l0 TEXT NOT NULL,
		l1 TEXT NOT NULL,
		l2 TEXT NOT NULL,
		type TEXT NOT NULL,
		tags TEXT NOT NULL DEFAULT '[]',
		source_agent TEXT NOT NULL DEFAULT '',
		source_node TEXT NOT NULL DEFAULT '',
		scope TEXT NOT NULL,
		project TEXT NOT NULL,
		domain TEXT NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		accessed_at TEXT NOT NULL,
		access_count INTEGER NOT NULL DEFAULT 0
	)`)
	if err != nil {
		return fmt.Errorf("create memories table: %w", err)
	}

	_, err = idx.db.Exec(`CREATE INDEX IF NOT EXISTS idx_memories_path ON memories(path)`)
	if err != nil {
		return fmt.Errorf("create path index: %w", err)
	}

	_, err = idx.db.Exec(`CREATE INDEX IF NOT EXISTS idx_memories_scope ON memories(scope, project, domain)`)
	if err != nil {
		return fmt.Errorf("create scope index: %w", err)
	}

	// Migration: soft delete
	idx.db.Exec(`ALTER TABLE memories ADD COLUMN deleted_at TEXT NOT NULL DEFAULT ''`)

	// Vector table
	vecSQL := fmt.Sprintf(`CREATE VIRTUAL TABLE IF NOT EXISTS vec_memories USING vec0(
		id TEXT PRIMARY KEY,
		embedding float[%d]
	)`, idx.dimension)
	_, err = idx.db.Exec(vecSQL)
	if err != nil {
		return fmt.Errorf("create vec table: %w", err)
	}

	return nil
}

// Upsert inserts or updates a memory and its embedding vector.
func (idx *Index) Upsert(mem *Memory, embedding []float32) error {
	tagsJSON, _ := json.Marshal(mem.Tags)
	vecBlob, err := sqlite_vec.SerializeFloat32(embedding)
	if err != nil {
		return fmt.Errorf("serialize vector: %w", err)
	}

	tx, err := idx.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Upsert metadata
	_, err = tx.Exec(`INSERT INTO memories (id, path, l0, l1, l2, type, tags, source_agent, source_node, scope, project, domain, created_at, updated_at, accessed_at, access_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			path=excluded.path, l0=excluded.l0, l1=excluded.l1, l2=excluded.l2,
			type=excluded.type, tags=excluded.tags, source_agent=excluded.source_agent,
			source_node=excluded.source_node, scope=excluded.scope, project=excluded.project,
			domain=excluded.domain, updated_at=excluded.updated_at`,
		mem.ID, mem.Path, mem.L0, mem.L1, mem.L2, mem.Type, string(tagsJSON),
		mem.SourceAgent, mem.SourceNode, mem.Scope, mem.Project, mem.Domain,
		mem.CreatedAt, mem.UpdatedAt, mem.AccessedAt, mem.AccessCount)
	if err != nil {
		return fmt.Errorf("upsert metadata: %w", err)
	}

	// Delete old vector if exists, then insert new
	_, _ = tx.Exec(`DELETE FROM vec_memories WHERE id = ?`, mem.ID)
	_, err = tx.Exec(`INSERT INTO vec_memories (id, embedding) VALUES (?, ?)`, mem.ID, vecBlob)
	if err != nil {
		return fmt.Errorf("insert vector: %w", err)
	}

	return tx.Commit()
}

// Search performs ANN vector search with optional scope/project/domain filters.
func (idx *Index) Search(queryVec []float32, limit int, scope, project, domain string) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 5
	}

	vecBlob, err := sqlite_vec.SerializeFloat32(queryVec)
	if err != nil {
		return nil, fmt.Errorf("serialize query vector: %w", err)
	}

	// Fetch candidate IDs from vector search (get more than needed for post-filtering)
	fetchLimit := limit * 4
	if fetchLimit < 20 {
		fetchLimit = 20
	}

	rows, err := idx.db.Query(
		`SELECT id, distance FROM vec_memories WHERE embedding MATCH ? ORDER BY distance LIMIT ?`,
		vecBlob, fetchLimit)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}
	defer rows.Close()

	type candidate struct {
		id       string
		distance float64
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.id, &c.distance); err != nil {
			return nil, err
		}
		candidates = append(candidates, c)
	}

	// Fetch full memories and apply filters
	var results []SearchResult
	for _, c := range candidates {
		mem, err := idx.GetByID(c.id)
		if err != nil || mem == nil {
			continue
		}
		if scope != "" && mem.Scope != scope {
			continue
		}
		if project != "" && mem.Project != project {
			continue
		}
		if domain != "" && mem.Domain != domain {
			continue
		}

		score := 1.0 / (1.0 + c.distance)
		results = append(results, SearchResult{
			Memory:   *mem,
			Distance: c.distance,
			Score:    math.Round(score*1000) / 1000,
		})
		if len(results) >= limit {
			break
		}
	}

	return results, nil
}

// GetByID retrieves a single memory by ID.
func (idx *Index) GetByID(id string) (*Memory, error) {
	row := idx.db.QueryRow(`SELECT id, path, l0, l1, l2, type, tags, source_agent, source_node, scope, project, domain, created_at, updated_at, accessed_at, access_count FROM memories WHERE id = ? AND deleted_at = ''`, id)
	return scanMemoryRow(row)
}

// GetByIDPrefix retrieves a memory by ID prefix (short marker like first 8 chars).
func (idx *Index) GetByIDPrefix(prefix string) (*Memory, error) {
	row := idx.db.QueryRow(`SELECT id, path, l0, l1, l2, type, tags, source_agent, source_node, scope, project, domain, created_at, updated_at, accessed_at, access_count FROM memories WHERE id LIKE ? AND deleted_at = '' LIMIT 1`, prefix+"%")
	return scanMemoryRow(row)
}

// ListByType returns all memories of a given type.
func (idx *Index) ListByType(memType string, limit int) ([]*Memory, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := idx.db.Query(`SELECT id, path, l0, l1, l2, type, tags, source_agent, source_node, scope, project, domain, created_at, updated_at, accessed_at, access_count FROM memories WHERE type = ? AND deleted_at = '' ORDER BY created_at LIMIT ?`, memType, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*Memory
	for rows.Next() {
		mem, err := scanMemoryRows(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, mem)
	}
	return results, rows.Err()
}

// GetByPath retrieves a single memory by exact path.
func (idx *Index) GetByPath(path string) (*Memory, error) {
	row := idx.db.QueryRow(`SELECT id, path, l0, l1, l2, type, tags, source_agent, source_node, scope, project, domain, created_at, updated_at, accessed_at, access_count FROM memories WHERE path = ? AND deleted_at = ''`, path)
	return scanMemoryRow(row)
}

// ListByPrefix returns all memories under a path prefix.
func (idx *Index) ListByPrefix(prefix string, limit int) ([]*Memory, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := idx.db.Query(`SELECT id, path, l0, l1, l2, type, tags, source_agent, source_node, scope, project, domain, created_at, updated_at, accessed_at, access_count FROM memories WHERE path LIKE ? AND deleted_at = '' ORDER BY created_at DESC LIMIT ?`, prefix+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*Memory
	for rows.Next() {
		mem, err := scanMemoryRows(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, mem)
	}
	return results, rows.Err()
}

// Tree builds a hierarchy tree with memory counts at each level.
func (idx *Index) Tree() ([]*TreeNode, error) {
	rows, err := idx.db.Query(`SELECT path FROM memories ORDER BY path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
		for i := 1; i < len(parts); i++ {
			dir := "/" + strings.Join(parts[:i], "/") + "/"
			counts[dir]++
		}
	}

	root := make(map[string]*TreeNode)
	for dir, count := range counts {
		parts := strings.Split(strings.TrimPrefix(strings.TrimSuffix(dir, "/"), "/"), "/")
		if len(parts) == 1 {
			root[dir] = &TreeNode{Path: dir, Count: count}
		}
	}

	for dir, count := range counts {
		parts := strings.Split(strings.TrimPrefix(strings.TrimSuffix(dir, "/"), "/"), "/")
		if len(parts) > 1 {
			parentDir := "/" + parts[0] + "/"
			if parent, ok := root[parentDir]; ok {
				parent.Children = append(parent.Children, &TreeNode{Path: dir, Count: count})
			}
		}
	}

	var result []*TreeNode
	for _, n := range root {
		result = append(result, n)
	}
	return result, nil
}

// Delete removes a memory by ID.
func (idx *Index) Delete(id string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := idx.db.Exec(`UPDATE memories SET deleted_at = ? WHERE id = ? AND deleted_at = ''`, now, id)
	return err
}

// ListDeleted returns soft-deleted memories.
func (idx *Index) ListDeleted(limit int) ([]*Memory, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := idx.db.Query(`SELECT id, path, l0, l1, l2, type, tags, source_agent, source_node, scope, project, domain, created_at, updated_at, accessed_at, access_count
		FROM memories WHERE deleted_at != '' ORDER BY deleted_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []*Memory
	for rows.Next() {
		m, err := scanMemoryRows(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, m)
	}
	return results, rows.Err()
}

// Restore un-deletes a soft-deleted memory.
func (idx *Index) Restore(id string) error {
	_, err := idx.db.Exec(`UPDATE memories SET deleted_at = '' WHERE id = ? AND deleted_at != ''`, id)
	return err
}

// DeleteByPrefix removes all memories under a path prefix. Returns count deleted.
func (idx *Index) DeleteByPrefix(prefix string) (int, error) {
	mems, err := idx.ListByPrefix(prefix, 10000)
	if err != nil {
		return 0, err
	}
	for _, m := range mems {
		if err := idx.Delete(m.ID); err != nil {
			return 0, err
		}
	}
	return len(mems), nil
}

// Count returns total number of memories.
func (idx *Index) Count() (int, error) {
	var count int
	err := idx.db.QueryRow(`SELECT COUNT(*) FROM memories WHERE deleted_at = ''`).Scan(&count)
	return count, err
}

// TouchAccess updates accessed_at and increments access_count.
func (idx *Index) TouchAccess(id string) error {
	_, err := idx.db.Exec(`UPDATE memories SET accessed_at = datetime('now'), access_count = access_count + 1 WHERE id = ?`, id)
	return err
}

// DB returns the underlying sql.DB for sharing with other stores.
func (idx *Index) DB() *sql.DB {
	return idx.db
}

// Close closes the database.
func (idx *Index) Close() error {
	return idx.db.Close()
}

func scanMemoryRow(row *sql.Row) (*Memory, error) {
	var m Memory
	var tagsStr string
	err := row.Scan(&m.ID, &m.Path, &m.L0, &m.L1, &m.L2, &m.Type, &tagsStr,
		&m.SourceAgent, &m.SourceNode, &m.Scope, &m.Project, &m.Domain,
		&m.CreatedAt, &m.UpdatedAt, &m.AccessedAt, &m.AccessCount)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(tagsStr), &m.Tags); err != nil {
		slog.Warn("unmarshal memory field failed", "field", "tags", "error", err)
	}
	return &m, nil
}

func scanMemoryRows(rows *sql.Rows) (*Memory, error) {
	var m Memory
	var tagsStr string
	err := rows.Scan(&m.ID, &m.Path, &m.L0, &m.L1, &m.L2, &m.Type, &tagsStr,
		&m.SourceAgent, &m.SourceNode, &m.Scope, &m.Project, &m.Domain,
		&m.CreatedAt, &m.UpdatedAt, &m.AccessedAt, &m.AccessCount)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(tagsStr), &m.Tags); err != nil {
		slog.Warn("unmarshal memory field failed", "field", "tags", "error", err)
	}
	return &m, nil
}
