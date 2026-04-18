package task

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/k8nstantin/OpenPraxis/internal/settings"
)

// migrationMarkerKey is the settings row key that records completion of the
// one-shot tasks.max_turns → settings(task-scope) migration. Stored at
// (scope_type='system', scope_id=''), consistent with the _retry_count
// underscore-prefix convention established by M4-T12.
const migrationMarkerKey = "_migration_m4_t14"

// MigrateMaxTurnsToSettings performs the one-time migration of
// tasks.max_turns column values to rows in the settings table scoped to the
// task. Idempotent via the marker at (system, '', _migration_m4_t14).
//
// Ordering contract: MUST run AFTER the task schema has created the tasks
// table (with max_turns column present on upgrades) and AFTER settings
// schema + store are ready, but BEFORE DropMaxTurnsColumn removes the column.
// Fresh installs that were never on a pre-M4-T14 schema simply find zero
// rows to migrate and still mark the marker so subsequent starts skip the
// probe entirely.
func MigrateMaxTurnsToSettings(db *sql.DB, store *settings.Store) (int, error) {
	ctx := context.Background()

	// Marker check — short-circuit on subsequent starts.
	if done, err := migrationDone(ctx, store); err != nil {
		return 0, fmt.Errorf("check migration marker: %w", err)
	} else if done {
		return 0, nil
	}

	// The column may not exist on fresh installs that skipped past the
	// pre-M4-T14 schema. Probe via pragma_table_info; absence is a success
	// case that writes the marker and exits.
	hasCol, err := hasMaxTurnsColumn(db)
	if err != nil {
		return 0, fmt.Errorf("probe max_turns column: %w", err)
	}
	if !hasCol {
		if err := markMigrationDone(ctx, store); err != nil {
			return 0, fmt.Errorf("mark migration done: %w", err)
		}
		return 0, nil
	}

	rows, err := db.QueryContext(ctx,
		`SELECT id, max_turns FROM tasks WHERE max_turns IS NOT NULL AND max_turns > 0`)
	if err != nil {
		return 0, fmt.Errorf("select tasks with max_turns: %w", err)
	}
	defer rows.Close()

	type pair struct {
		id       string
		maxTurns int
	}
	var todo []pair
	for rows.Next() {
		var p pair
		if err := rows.Scan(&p.id, &p.maxTurns); err != nil {
			return 0, fmt.Errorf("scan task row: %w", err)
		}
		todo = append(todo, p)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate tasks: %w", err)
	}

	migrated := 0
	for _, p := range todo {
		// ON CONFLICT DO NOTHING semantics: if the task already has an
		// explicit settings row (e.g. from M2-T6 API write), do not clobber it.
		if _, err := store.Get(ctx, settings.ScopeTask, p.id, "max_turns"); err == nil {
			// row already exists — skip
			continue
		} else if !errors.Is(err, sql.ErrNoRows) {
			return migrated, fmt.Errorf("get existing setting for task %s: %w", p.id, err)
		}

		raw := strconv.Itoa(p.maxTurns)
		if err := store.Set(ctx, settings.ScopeTask, p.id, "max_turns", raw, "migration:m4-t14"); err != nil {
			return migrated, fmt.Errorf("migrate task %s max_turns: %w", p.id, err)
		}
		migrated++
	}

	if err := markMigrationDone(ctx, store); err != nil {
		return migrated, fmt.Errorf("mark migration done: %w", err)
	}

	if migrated > 0 {
		slog.Info("migrated tasks.max_turns to settings table", "rows", migrated, "marker", migrationMarkerKey)
	}
	return migrated, nil
}

// DropMaxTurnsColumn removes the tasks.max_turns column after migration has
// run. Uses SQLite's native ALTER TABLE DROP COLUMN (3.35+); the embedded
// go-sqlite3 driver ships 3.46+, so the classical rename-create-copy-drop
// rewrite is not needed in this codebase. A future downgrade to ancient
// SQLite would require re-adding the fallback path here.
//
// Idempotent: absent column is a no-op.
func DropMaxTurnsColumn(db *sql.DB) error {
	hasCol, err := hasMaxTurnsColumn(db)
	if err != nil {
		return fmt.Errorf("probe max_turns column: %w", err)
	}
	if !hasCol {
		return nil
	}
	if _, err := db.Exec(`ALTER TABLE tasks DROP COLUMN max_turns`); err != nil {
		return fmt.Errorf("drop max_turns column: %w", err)
	}
	slog.Info("dropped tasks.max_turns column (M4-T14)")
	return nil
}

// hasMaxTurnsColumn returns true when the tasks table has a max_turns column.
// Uses pragma_table_info which is SQLite-native and does not require any
// special privileges.
func hasMaxTurnsColumn(db *sql.DB) (bool, error) {
	rows, err := db.Query(`SELECT name FROM pragma_table_info('tasks')`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return false, err
		}
		if name == "max_turns" {
			return true, nil
		}
	}
	return false, rows.Err()
}

func migrationDone(ctx context.Context, store *settings.Store) (bool, error) {
	_, err := store.Get(ctx, settings.ScopeSystem, "", migrationMarkerKey)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return false, err
}

func markMigrationDone(ctx context.Context, store *settings.Store) error {
	return store.Set(ctx, settings.ScopeSystem, "", migrationMarkerKey, `"completed"`, "migration:m4-t14")
}
