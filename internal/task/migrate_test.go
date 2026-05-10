package task

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/k8nstantin/OpenPraxis/internal/settings"
)

// migrateTestDB opens a SQLite DB with WAL + busy_timeout (visceral rule #10),
// applies the settings schema, and returns the handle plus a constructed
// Store. Callers are expected to build their own tasks schema fixture
// (with or without the legacy max_turns column) to exercise the migration
// paths exercised here.
func migrateTestDB(t *testing.T) (*sql.DB, *settings.Store) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "migrate.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		t.Fatalf("set WAL: %v", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		t.Fatalf("set busy_timeout: %v", err)
	}
	if err := settings.InitSchema(db); err != nil {
		t.Fatalf("settings.InitSchema: %v", err)
	}
	return db, settings.NewStore(db)
}

// seedLegacyTasksTable creates a minimal tasks table WITH the legacy
// max_turns column so the migration has something to walk. Mirrors the
// pre-M4-T14 shape closely enough for the SELECT path under test.
func seedLegacyTasksTable(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`CREATE TABLE tasks (
		id TEXT PRIMARY KEY,
		manifest_id TEXT NOT NULL DEFAULT '',
		title TEXT NOT NULL DEFAULT '',
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
		created_at TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL DEFAULT '',
		deleted_at TEXT NOT NULL DEFAULT '',
		max_turns INTEGER NOT NULL DEFAULT 0,
		depends_on TEXT NOT NULL DEFAULT '',
		block_reason TEXT NOT NULL DEFAULT '',
		action_request TEXT NOT NULL DEFAULT ''
	)`)
	if err != nil {
		t.Fatalf("create legacy tasks: %v", err)
	}
}

func insertLegacyTask(t *testing.T, db *sql.DB, id string, maxTurns int) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO tasks (id, max_turns) VALUES (?, ?)`, id, maxTurns)
	if err != nil {
		t.Fatalf("insert legacy task %s: %v", id, err)
	}
}

// TestMigrateMaxTurns_CopiesLegacyColumnToSettings — baseline path: a task
// row with a non-null max_turns value gets a settings(task-scope) row with
// the JSON-encoded int value, and the marker row is written.
func TestMigrateMaxTurns_CopiesLegacyColumnToSettings(t *testing.T) {
	t.Skip("task store migrated to entities")
	db, store := migrateTestDB(t)
	seedLegacyTasksTable(t, db)
	insertLegacyTask(t, db, "task-1", 100)
	insertLegacyTask(t, db, "task-2", 42)

	n, err := MigrateMaxTurnsToSettings(db, store)
	if err != nil {
		t.Fatalf("MigrateMaxTurnsToSettings: %v", err)
	}
	if n != 2 {
		t.Fatalf("migrated rows = %d, want 2", n)
	}
	ctx := context.Background()
	got1, err := store.Get(ctx, settings.ScopeTask, "task-1", "max_turns")
	if err != nil {
		t.Fatalf("get task-1 max_turns: %v", err)
	}
	if got1.Value != "100" {
		t.Fatalf("task-1 max_turns value = %q, want %q", got1.Value, "100")
	}
	got2, err := store.Get(ctx, settings.ScopeTask, "task-2", "max_turns")
	if err != nil {
		t.Fatalf("get task-2 max_turns: %v", err)
	}
	if got2.Value != "42" {
		t.Fatalf("task-2 max_turns value = %q, want %q", got2.Value, "42")
	}
	// Marker must be set so the migration is skipped on the next run.
	marker, err := store.Get(ctx, settings.ScopeSystem, "", migrationMarkerKey)
	if err != nil {
		t.Fatalf("get marker: %v", err)
	}
	if marker.Value != `"completed"` {
		t.Fatalf("marker value = %q, want %q", marker.Value, `"completed"`)
	}
}

// TestMigrateMaxTurns_Idempotent_DoesNotRunTwice — after the marker is set,
// re-running the migration returns 0 migrated rows even if new legacy values
// would otherwise be picked up. This protects against double-migration on
// process restart.
func TestMigrateMaxTurns_Idempotent_DoesNotRunTwice(t *testing.T) {
	t.Skip("task store migrated to entities")
	db, store := migrateTestDB(t)
	seedLegacyTasksTable(t, db)
	insertLegacyTask(t, db, "task-1", 100)

	if _, err := MigrateMaxTurnsToSettings(db, store); err != nil {
		t.Fatalf("first migrate: %v", err)
	}

	// Introduce a new legacy row AFTER the marker is set. A naive
	// re-run would migrate it — idempotency requires it stays un-migrated.
	insertLegacyTask(t, db, "task-2", 999)

	n, err := MigrateMaxTurnsToSettings(db, store)
	if err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	if n != 0 {
		t.Fatalf("second run migrated %d rows, want 0 (marker should have short-circuited)", n)
	}
	ctx := context.Background()
	_, err = store.Get(ctx, settings.ScopeTask, "task-2", "max_turns")
	if err == nil {
		t.Fatalf("task-2 got a settings row on second run; idempotency broken")
	}
}

// TestMigrateMaxTurns_DoesNotOverwriteExplicitSettingsRow — if a task already
// has an explicit settings row for max_turns (e.g. from the M2-T6 API write
// path), the migration must preserve it rather than clobber with the column
// value. This matches the ON CONFLICT DO NOTHING semantics the spec requires.
func TestMigrateMaxTurns_DoesNotOverwriteExplicitSettingsRow(t *testing.T) {
	t.Skip("task store migrated to entities")
	db, store := migrateTestDB(t)
	seedLegacyTasksTable(t, db)
	insertLegacyTask(t, db, "task-1", 100)

	ctx := context.Background()
	// User already pinned 250 via the settings API before the migration ran.
	if err := store.Set(ctx, settings.ScopeTask, "task-1", "max_turns", "250", "user"); err != nil {
		t.Fatalf("pre-seed settings: %v", err)
	}

	n, err := MigrateMaxTurnsToSettings(db, store)
	if err != nil {
		t.Fatalf("MigrateMaxTurnsToSettings: %v", err)
	}
	// The explicit row should NOT be counted as migrated — we skipped it.
	if n != 0 {
		t.Fatalf("migrated %d rows, want 0 (explicit row should be skipped)", n)
	}
	got, err := store.Get(ctx, settings.ScopeTask, "task-1", "max_turns")
	if err != nil {
		t.Fatalf("get after migrate: %v", err)
	}
	if got.Value != "250" {
		t.Fatalf("explicit row clobbered: value = %q, want %q", got.Value, "250")
	}
}

// TestSchema_MaxTurnsColumnDropped — after running the full
// init-then-migrate-then-drop sequence in the same order as node.New, the
// tasks table must not carry the max_turns column. Uses pragma_table_info
// to enumerate columns post-drop.
func TestSchema_MaxTurnsColumnDropped(t *testing.T) {
	t.Skip("task store migrated to entities")
	db, store := migrateTestDB(t)
	seedLegacyTasksTable(t, db)
	insertLegacyTask(t, db, "task-1", 100)

	if _, err := MigrateMaxTurnsToSettings(db, store); err != nil {
		t.Fatalf("MigrateMaxTurnsToSettings: %v", err)
	}
	if err := DropMaxTurnsColumn(db); err != nil {
		t.Fatalf("DropMaxTurnsColumn: %v", err)
	}

	hasCol, err := hasMaxTurnsColumn(db)
	if err != nil {
		t.Fatalf("hasMaxTurnsColumn: %v", err)
	}
	if hasCol {
		t.Fatalf("max_turns column still present after DropMaxTurnsColumn")
	}

	// Second drop must be a no-op, not an error — idempotency for restart
	// paths that retry the drop step.
	if err := DropMaxTurnsColumn(db); err != nil {
		t.Fatalf("second DropMaxTurnsColumn: %v", err)
	}
}
