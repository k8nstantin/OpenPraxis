package comments

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
)

// TargetResolver resolves a (target_type, raw target_id) pair — where the
// raw id may be a short marker prefix — to the canonical full UUID. An
// empty string return means the target entity does not exist (orphan).
//
// The sweep uses this to migrate pre-#136/#139 short-marker comment rows.
// Kept as an interface here so the comments package stays free of an
// internal/node import (node already imports comments — reversing would
// cycle).
type TargetResolver interface {
	Resolve(ctx context.Context, target TargetType, id string) (string, error)
}

// SweepReport summarises a SweepOrphans run.
type SweepReport struct {
	Scanned      int
	Migrated     int
	Unresolvable int
	ByTargetType map[string]int
}

// SweepOrphans finds comments whose target_id is a short marker (length < 36)
// and, for each, resolves the real full UUID via the provided TargetResolver.
// Resolvable rows are updated to the full UUID; unresolvable rows are logged
// and left alone (operator-investigable). If dryRun is true, no UPDATEs run —
// the report still reflects what would have been migrated.
func SweepOrphans(ctx context.Context, db *sql.DB, r TargetResolver, dryRun bool) (SweepReport, error) {
	report := SweepReport{ByTargetType: map[string]int{}}

	rows, err := db.QueryContext(ctx,
		`SELECT id, target_type, target_id FROM comments WHERE length(target_id) < 36`)
	if err != nil {
		return report, fmt.Errorf("orphan sweep: scan: %w", err)
	}
	defer rows.Close()

	type row struct{ id, tt, tid string }
	var found []row
	for rows.Next() {
		var x row
		if err := rows.Scan(&x.id, &x.tt, &x.tid); err != nil {
			return report, fmt.Errorf("orphan sweep: scan row: %w", err)
		}
		found = append(found, x)
	}
	if err := rows.Err(); err != nil {
		return report, fmt.Errorf("orphan sweep: rows err: %w", err)
	}
	report.Scanned = len(found)

	for _, x := range found {
		full, rerr := r.Resolve(ctx, TargetType(x.tt), x.tid)
		if rerr != nil || full == "" {
			report.Unresolvable++
			slog.Error("orphan comment unresolvable",
				"comment_id", x.id,
				"target_type", x.tt,
				"target_id", x.tid,
				"err", rerr,
			)
			continue
		}
		if full == x.tid {
			// Already full (shouldn't happen given the length filter, but guard anyway).
			continue
		}
		if !dryRun {
			if _, err := db.ExecContext(ctx,
				`UPDATE comments SET target_id = ? WHERE id = ?`, full, x.id); err != nil {
				return report, fmt.Errorf("orphan sweep: update %s: %w", x.id, err)
			}
		}
		report.Migrated++
		report.ByTargetType[x.tt]++
	}
	return report, nil
}
