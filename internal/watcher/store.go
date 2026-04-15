package watcher

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// Audit represents a post-completion verification of a task against its manifest.
type Audit struct {
	ID              string    `json:"id"`
	Marker          string    `json:"marker"`
	TaskID          string    `json:"task_id"`
	TaskMarker      string    `json:"task_marker"`
	TaskTitle       string    `json:"task_title"`
	ManifestID      string    `json:"manifest_id"`
	ManifestTitle   string    `json:"manifest_title"`
	Status          string    `json:"status"` // passed, failed, warning, pending
	GitPassed       bool      `json:"git_passed"`
	GitDetails      GitResult `json:"git_details"`
	BuildPassed     bool      `json:"build_passed"`
	BuildDetails    BuildResult `json:"build_details"`
	ManifestPassed  bool      `json:"manifest_passed"`
	ManifestScore   float64   `json:"manifest_score"`
	ManifestDetails ManifestResult `json:"manifest_details"`
	OriginalStatus  string    `json:"original_status"`
	FinalStatus     string    `json:"final_status"`
	ActionCount     int       `json:"action_count"`
	CostUSD         float64   `json:"cost_usd"`
	SourceNode      string    `json:"source_node"`
	AuditedAt       time.Time `json:"audited_at"`
	CreatedAt       time.Time `json:"created_at"`
}

// GitResult holds the output of the git verification gate.
type GitResult struct {
	CommitCount      int      `json:"commit_count"`
	FilesChanged     int      `json:"files_changed"`
	Insertions       int      `json:"insertions"`
	Deletions        int      `json:"deletions"`
	UncommittedFiles []string `json:"uncommitted_files"`
	BranchExists     bool     `json:"branch_exists"`
	Reason           string   `json:"reason"`
}

// BuildResult holds the output of the build verification gate.
type BuildResult struct {
	ExitCode int    `json:"exit_code"`
	Output   string `json:"output"`
	Reason   string `json:"reason"`
}

// ManifestResult holds the output of the manifest compliance gate.
type ManifestResult struct {
	Deliverables []Deliverable `json:"deliverables"`
	TotalItems   int           `json:"total_items"`
	DoneItems    int           `json:"done_items"`
	MissingItems int           `json:"missing_items"`
	Reason       string        `json:"reason"`
}

// Deliverable is a single item from the manifest spec that was checked.
type Deliverable struct {
	Item   string `json:"item"`
	Status string `json:"status"` // done, missing, partial
	Evidence string `json:"evidence"`
}

// Stats holds summary statistics for the watcher.
type Stats struct {
	TotalAudits  int     `json:"total_audits"`
	Passed       int     `json:"passed"`
	Failed       int     `json:"failed"`
	Warnings     int     `json:"warnings"`
	PassRate     float64 `json:"pass_rate"`
	GitFailRate  float64 `json:"git_fail_rate"`
	BuildFailRate float64 `json:"build_fail_rate"`
}

// Store manages watcher audit persistence.
type Store struct {
	db *sql.DB
}

// NewStore creates a watcher store.
func NewStore(db *sql.DB) (*Store, error) {
	s := &Store{db: db}
	if err := s.init(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) init() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS watcher_audits (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		task_marker TEXT NOT NULL DEFAULT '',
		task_title TEXT NOT NULL DEFAULT '',
		manifest_id TEXT NOT NULL DEFAULT '',
		manifest_title TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'pending',
		git_passed INTEGER NOT NULL DEFAULT 0,
		git_details TEXT NOT NULL DEFAULT '{}',
		build_passed INTEGER NOT NULL DEFAULT 0,
		build_details TEXT NOT NULL DEFAULT '{}',
		manifest_passed INTEGER NOT NULL DEFAULT 0,
		manifest_score REAL NOT NULL DEFAULT 0,
		manifest_details TEXT NOT NULL DEFAULT '{}',
		original_status TEXT NOT NULL DEFAULT '',
		final_status TEXT NOT NULL DEFAULT '',
		action_count INTEGER NOT NULL DEFAULT 0,
		cost_usd REAL NOT NULL DEFAULT 0,
		source_node TEXT NOT NULL DEFAULT '',
		audited_at TEXT NOT NULL,
		created_at TEXT NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("create watcher_audits table: %w", err)
	}

	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_watcher_task ON watcher_audits(task_id)`)
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_watcher_status ON watcher_audits(status)`)
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_watcher_created ON watcher_audits(created_at DESC)`)

	return nil
}

// Record stores a new audit result.
func (s *Store) Record(audit *Audit) error {
	if audit.ID == "" {
		audit.ID = uuid.Must(uuid.NewV7()).String()
	}
	if len(audit.ID) >= 12 {
		audit.Marker = audit.ID[:12]
	}

	gitJSON, _ := json.Marshal(audit.GitDetails)
	buildJSON, _ := json.Marshal(audit.BuildDetails)
	manifestJSON, _ := json.Marshal(audit.ManifestDetails)

	_, err := s.db.Exec(`INSERT INTO watcher_audits (id, task_id, task_marker, task_title, manifest_id, manifest_title, status, git_passed, git_details, build_passed, build_details, manifest_passed, manifest_score, manifest_details, original_status, final_status, action_count, cost_usd, source_node, audited_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		audit.ID, audit.TaskID, audit.TaskMarker, audit.TaskTitle,
		audit.ManifestID, audit.ManifestTitle,
		audit.Status, boolToInt(audit.GitPassed), string(gitJSON),
		boolToInt(audit.BuildPassed), string(buildJSON),
		boolToInt(audit.ManifestPassed), audit.ManifestScore, string(manifestJSON),
		audit.OriginalStatus, audit.FinalStatus,
		audit.ActionCount, audit.CostUSD, audit.SourceNode,
		audit.AuditedAt.Format(time.RFC3339), audit.CreatedAt.Format(time.RFC3339))
	return err
}

// Get retrieves an audit by ID or prefix.
func (s *Store) Get(id string) (*Audit, error) {
	row := s.db.QueryRow(`SELECT id, task_id, task_marker, task_title, manifest_id, manifest_title, status, git_passed, git_details, build_passed, build_details, manifest_passed, manifest_score, manifest_details, original_status, final_status, action_count, cost_usd, source_node, audited_at, created_at
		FROM watcher_audits WHERE id = ? OR id LIKE ?`, id, id+"%")
	return scanAudit(row)
}

// GetByTask retrieves the most recent audit for a task.
func (s *Store) GetByTask(taskID string) (*Audit, error) {
	row := s.db.QueryRow(`SELECT id, task_id, task_marker, task_title, manifest_id, manifest_title, status, git_passed, git_details, build_passed, build_details, manifest_passed, manifest_score, manifest_details, original_status, final_status, action_count, cost_usd, source_node, audited_at, created_at
		FROM watcher_audits WHERE task_id = ? OR task_id LIKE ? ORDER BY created_at DESC LIMIT 1`, taskID, taskID+"%")
	return scanAudit(row)
}

// List returns recent audits.
func (s *Store) List(status string, limit int) ([]*Audit, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `SELECT id, task_id, task_marker, task_title, manifest_id, manifest_title, status, git_passed, git_details, build_passed, build_details, manifest_passed, manifest_score, manifest_details, original_status, final_status, action_count, cost_usd, source_node, audited_at, created_at FROM watcher_audits`
	var args []any
	if status != "" {
		query += ` WHERE status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAudits(rows)
}

// Stats returns summary statistics.
func (s *Store) Stats() (*Stats, error) {
	var stats Stats
	s.db.QueryRow(`SELECT COUNT(*) FROM watcher_audits`).Scan(&stats.TotalAudits)
	s.db.QueryRow(`SELECT COUNT(*) FROM watcher_audits WHERE status = 'passed'`).Scan(&stats.Passed)
	s.db.QueryRow(`SELECT COUNT(*) FROM watcher_audits WHERE status = 'failed'`).Scan(&stats.Failed)
	s.db.QueryRow(`SELECT COUNT(*) FROM watcher_audits WHERE status = 'warning'`).Scan(&stats.Warnings)

	if stats.TotalAudits > 0 {
		stats.PassRate = float64(stats.Passed) / float64(stats.TotalAudits) * 100
		var gitFails int
		s.db.QueryRow(`SELECT COUNT(*) FROM watcher_audits WHERE git_passed = 0`).Scan(&gitFails)
		stats.GitFailRate = float64(gitFails) / float64(stats.TotalAudits) * 100
		var buildFails int
		s.db.QueryRow(`SELECT COUNT(*) FROM watcher_audits WHERE build_passed = 0`).Scan(&buildFails)
		stats.BuildFailRate = float64(buildFails) / float64(stats.TotalAudits) * 100
	}

	return &stats, nil
}

func scanAudit(row *sql.Row) (*Audit, error) {
	var a Audit
	var gitJSON, buildJSON, manifestJSON, auditedStr, createdStr string
	var gitPassed, buildPassed, manifestPassed int

	err := row.Scan(&a.ID, &a.TaskID, &a.TaskMarker, &a.TaskTitle,
		&a.ManifestID, &a.ManifestTitle,
		&a.Status, &gitPassed, &gitJSON,
		&buildPassed, &buildJSON,
		&manifestPassed, &a.ManifestScore, &manifestJSON,
		&a.OriginalStatus, &a.FinalStatus,
		&a.ActionCount, &a.CostUSD, &a.SourceNode,
		&auditedStr, &createdStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	a.GitPassed = gitPassed != 0
	a.BuildPassed = buildPassed != 0
	a.ManifestPassed = manifestPassed != 0
	if err := json.Unmarshal([]byte(gitJSON), &a.GitDetails); err != nil {
		slog.Warn("unmarshal audit field failed", "field", "git_details", "error", err)
	}
	if err := json.Unmarshal([]byte(buildJSON), &a.BuildDetails); err != nil {
		slog.Warn("unmarshal audit field failed", "field", "build_details", "error", err)
	}
	if err := json.Unmarshal([]byte(manifestJSON), &a.ManifestDetails); err != nil {
		slog.Warn("unmarshal audit field failed", "field", "manifest_details", "error", err)
	}
	a.AuditedAt, _ = time.Parse(time.RFC3339, auditedStr)
	a.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	if len(a.ID) >= 12 {
		a.Marker = a.ID[:12]
	}
	return &a, nil
}

func scanAudits(rows *sql.Rows) ([]*Audit, error) {
	var results []*Audit
	for rows.Next() {
		var a Audit
		var gitJSON, buildJSON, manifestJSON, auditedStr, createdStr string
		var gitPassed, buildPassed, manifestPassed int

		err := rows.Scan(&a.ID, &a.TaskID, &a.TaskMarker, &a.TaskTitle,
			&a.ManifestID, &a.ManifestTitle,
			&a.Status, &gitPassed, &gitJSON,
			&buildPassed, &buildJSON,
			&manifestPassed, &a.ManifestScore, &manifestJSON,
			&a.OriginalStatus, &a.FinalStatus,
			&a.ActionCount, &a.CostUSD, &a.SourceNode,
			&auditedStr, &createdStr)
		if err != nil {
			return nil, err
		}

		a.GitPassed = gitPassed != 0
		a.BuildPassed = buildPassed != 0
		a.ManifestPassed = manifestPassed != 0
		if err := json.Unmarshal([]byte(gitJSON), &a.GitDetails); err != nil {
			slog.Warn("unmarshal audit field failed", "field", "git_details", "error", err)
		}
		if err := json.Unmarshal([]byte(buildJSON), &a.BuildDetails); err != nil {
			slog.Warn("unmarshal audit field failed", "field", "build_details", "error", err)
		}
		if err := json.Unmarshal([]byte(manifestJSON), &a.ManifestDetails); err != nil {
			slog.Warn("unmarshal audit field failed", "field", "manifest_details", "error", err)
		}
		a.AuditedAt, _ = time.Parse(time.RFC3339, auditedStr)
		a.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		if len(a.ID) >= 12 {
			a.Marker = a.ID[:12]
		}
		results = append(results, &a)
	}
	return results, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
