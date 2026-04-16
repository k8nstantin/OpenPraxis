package task

import (
	"database/sql"
	"fmt"
	"time"
)

// Task represents scheduled work against a manifest.
type Task struct {
	ID          string    `json:"id"`
	Marker      string    `json:"marker"`
	ManifestID  string    `json:"manifest_id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Schedule    string    `json:"schedule"`   // "once", "5m", "1h", cron expression
	Status      string    `json:"status"`     // pending, scheduled, running, completed, failed, cancelled
	Agent       string    `json:"agent"`      // agent type: claude-code, cursor, etc.
	SourceNode  string    `json:"source_node"`
	CreatedBy   string    `json:"created_by"` // session or dashboard
	MaxTurns    int       `json:"max_turns"`  // max agent turns (default 100)
	DependsOn   string    `json:"depends_on"`    // task ID that must complete before this runs
	BlockReason string    `json:"block_reason"`  // reason task is blocked (e.g. manifest dependency)
	RunCount    int       `json:"run_count"`
	LastRunAt   string    `json:"last_run_at"`
	NextRunAt   string    `json:"next_run_at"`
	LastOutput  string    `json:"last_output"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// Computed from task_runs — populated by enrichWithCosts()
	TotalTurns int     `json:"total_turns"`
	TotalCost  float64 `json:"total_cost"`
}

// TaskRun represents a single execution of a task, preserving history.
type TaskRun struct {
	ID          int       `json:"id"`
	TaskID      string    `json:"task_id"`
	RunNumber   int       `json:"run_number"`
	Output      string    `json:"output"`
	Status      string    `json:"status"`
	Actions     int       `json:"actions"`
	Lines       int       `json:"lines"`
	CostUSD     float64   `json:"cost_usd"`
	Turns       int       `json:"turns"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
}

// Store manages task persistence.
type Store struct {
	db *sql.DB
}

// NewStore creates a task store.
func NewStore(db *sql.DB) (*Store, error) {
	s := &Store{db: db}
	if err := s.init(); err != nil {
		return nil, err
	}
	return s, nil
}

// DB returns the underlying database connection for cross-store queries.
func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) init() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS tasks (
		id TEXT PRIMARY KEY,
		manifest_id TEXT NOT NULL DEFAULT '',
		title TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		schedule TEXT NOT NULL DEFAULT 'once',
		status TEXT NOT NULL DEFAULT 'pending',
		agent TEXT NOT NULL DEFAULT 'claude-code',
		source_node TEXT NOT NULL DEFAULT '',
		created_by TEXT NOT NULL DEFAULT '',
		run_count INTEGER NOT NULL DEFAULT 0,
		last_run_at TEXT NOT NULL DEFAULT '',
		next_run_at TEXT NOT NULL DEFAULT '',
		last_output TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		deleted_at TEXT NOT NULL DEFAULT ''
	)`)
	if err != nil {
		return fmt.Errorf("create tasks table: %w", err)
	}

	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_tasks_manifest ON tasks(manifest_id)`)
	if err != nil {
		return fmt.Errorf("create tasks manifest index: %w", err)
	}

	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status)`)
	if err != nil {
		return fmt.Errorf("create tasks status index: %w", err)
	}

	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_tasks_next_run ON tasks(next_run_at)`)
	if err != nil {
		return err
	}
	s.db.Exec(`ALTER TABLE tasks ADD COLUMN deleted_at TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`ALTER TABLE tasks ADD COLUMN max_turns INTEGER NOT NULL DEFAULT 50`)
	s.db.Exec(`ALTER TABLE tasks ADD COLUMN depends_on TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`ALTER TABLE tasks ADD COLUMN block_reason TEXT NOT NULL DEFAULT ''`)
	// Cross-process action signal: 'pause' | 'resume' | 'cancel'.
	// MCP sets this; serve's runner watches and acts on the task process it owns.
	s.db.Exec(`ALTER TABLE tasks ADD COLUMN action_request TEXT NOT NULL DEFAULT ''`)

	// Task runs history table
	_, err = s.db.Exec(`CREATE TABLE IF NOT EXISTS task_runs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		task_id TEXT NOT NULL,
		run_number INTEGER NOT NULL,
		output TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT '',
		actions INTEGER NOT NULL DEFAULT 0,
		lines INTEGER NOT NULL DEFAULT 0,
		started_at TEXT NOT NULL,
		completed_at TEXT NOT NULL DEFAULT ''
	)`)
	if err != nil {
		return fmt.Errorf("create task_runs table: %w", err)
	}
	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_task_runs_task ON task_runs(task_id)`)
	if err != nil {
		return fmt.Errorf("create task_runs index: %w", err)
	}

	// Migrate: add cost_usd and turns columns to task_runs
	s.db.Exec(`ALTER TABLE task_runs ADD COLUMN cost_usd REAL NOT NULL DEFAULT 0`)
	s.db.Exec(`ALTER TABLE task_runs ADD COLUMN turns INTEGER NOT NULL DEFAULT 0`)

	// Running task runtime state — persists in-memory RunningTask data to survive restarts
	_, err = s.db.Exec(`CREATE TABLE IF NOT EXISTS task_runtime_state (
		task_id TEXT PRIMARY KEY,
		marker TEXT NOT NULL DEFAULT '',
		title TEXT NOT NULL DEFAULT '',
		manifest TEXT NOT NULL DEFAULT '',
		agent TEXT NOT NULL DEFAULT '',
		pid INTEGER NOT NULL DEFAULT 0,
		paused INTEGER NOT NULL DEFAULT 0,
		actions INTEGER NOT NULL DEFAULT 0,
		lines INTEGER NOT NULL DEFAULT 0,
		last_line TEXT NOT NULL DEFAULT '',
		started_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("create task_runtime_state table: %w", err)
	}

	// Many-to-many: task ↔ manifest link table
	_, err = s.db.Exec(`CREATE TABLE IF NOT EXISTS task_manifests (
		task_id TEXT NOT NULL,
		manifest_id TEXT NOT NULL,
		linked_at TEXT NOT NULL,
		PRIMARY KEY (task_id, manifest_id)
	)`)
	if err != nil {
		return fmt.Errorf("create task_manifests table: %w", err)
	}

	return nil
}

// taskColumns is the standard column list for task SELECT queries.
const taskColumns = `id, manifest_id, title, description, schedule, status, agent, source_node, created_by, max_turns, depends_on, block_reason, run_count, last_run_at, next_run_at, last_output, created_at, updated_at`

func scanTask(row *sql.Row) (*Task, error) {
	var t Task
	var createdStr, updatedStr string
	err := row.Scan(&t.ID, &t.ManifestID, &t.Title, &t.Description, &t.Schedule, &t.Status, &t.Agent, &t.SourceNode, &t.CreatedBy,
		&t.MaxTurns, &t.DependsOn, &t.BlockReason, &t.RunCount, &t.LastRunAt, &t.NextRunAt, &t.LastOutput, &createdStr, &updatedStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	t.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	t.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	if len(t.ID) >= 12 {
		t.Marker = t.ID[:12]
	}
	return &t, nil
}

func scanTasks(rows *sql.Rows) ([]*Task, error) {
	var tasks []*Task
	for rows.Next() {
		var t Task
		var createdStr, updatedStr string
		err := rows.Scan(&t.ID, &t.ManifestID, &t.Title, &t.Description, &t.Schedule, &t.Status, &t.Agent, &t.SourceNode, &t.CreatedBy,
			&t.MaxTurns, &t.DependsOn, &t.BlockReason, &t.RunCount, &t.LastRunAt, &t.NextRunAt, &t.LastOutput, &createdStr, &updatedStr)
		if err != nil {
			return nil, err
		}
		t.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		t.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
		if len(t.ID) >= 12 {
			t.Marker = t.ID[:12]
		}
		tasks = append(tasks, &t)
	}
	return tasks, rows.Err()
}

func scanRuns(rows *sql.Rows) ([]TaskRun, error) {
	var runs []TaskRun
	for rows.Next() {
		var r TaskRun
		var startedStr, completedStr string
		if err := rows.Scan(&r.ID, &r.TaskID, &r.RunNumber, &r.Output, &r.Status, &r.Actions, &r.Lines, &r.CostUSD, &r.Turns, &startedStr, &completedStr); err != nil {
			return nil, err
		}
		r.StartedAt, _ = time.Parse(time.RFC3339, startedStr)
		r.CompletedAt, _ = time.Parse(time.RFC3339, completedStr)
		runs = append(runs, r)
	}
	return runs, rows.Err()
}
