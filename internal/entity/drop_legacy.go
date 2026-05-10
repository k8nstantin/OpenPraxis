package entity

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
)

// DropLegacyTables drops all tables that have been superseded by the unified
// entities/execution_log architecture. Idempotent — uses DROP TABLE IF EXISTS.
// Called once after MigrateFromLegacy has successfully copied all data.
func DropLegacyTables(ctx context.Context, db *sql.DB) (dropped []string, err error) {
	tables := []string{
		// Entity tables → replaced by entities
		"products",
		"manifests",
		"ideas",
		// Legacy dep tables → replaced by relationships
		"product_dependencies",
		"manifest_dependencies",
		"idea_manifest_links",
		// Legacy execution migration artefacts
		"execution_log_samples",
		"execution_log_legacy",
		// task_dependency: dep SCD history is now in the relationships table.
		"task_dependency",
		// Task runner legacy tables → replaced by execution_log + relationships.
		// tasks: all data migrated to entities (type='task'). Scheduler no longer instantiated.
		"tasks",
		"task_runs",
		"task_run_host_samples",
		"task_runtime_state",
		"task_manifests",
		// Cost-related tables — cost will be redesigned.
		"model_pricing",
		// system_host_samples — all system metrics now in execution_log sample rows.
		"system_host_samples",
	}
	for _, t := range tables {
		if _, err := db.ExecContext(ctx, "DROP TABLE IF EXISTS "+t); err != nil {
			return dropped, fmt.Errorf("drop %s: %w", t, err)
		}
		slog.Info("dropped legacy table", "table", t)
		dropped = append(dropped, t)
	}
	return dropped, nil
}
