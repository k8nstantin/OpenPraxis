package schedule

import (
	"context"
	"database/sql"
)

// BackfillFromTasks is a no-op. The tasks table has been retired; all
// task scheduling data is now managed via the entities and schedules tables.
func BackfillFromTasks(ctx context.Context, db *sql.DB) (int, error) {
	return 0, nil
}
