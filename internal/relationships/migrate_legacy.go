// Migration of legacy dependency tables into the unified relationships
// SCD-2 store.
//
// Direction convention (verified against existing readers + sample data):
//   - product_dependencies: (product_id, depends_on_product_id) =
//     (depender, dependee). Reads + writes treat product_id as the row
//     that "depends on" depends_on_product_id; the recursive CTE in
//     internal/product/store.go EnrichRecursiveCosts walks
//     pd.product_id → pd.depends_on_product_id as parent → child, where
//     "parent" is the product that has open work blocked on "child"
//     completing. That is the depender→dependee semantic.
//   - manifest_dependencies: (manifest_id, depends_on_manifest_id) =
//     (depender, dependee). manifest.AddDep / IsSatisfied / pathExists
//     all read manifest_id as the row that depends on
//     depends_on_manifest_id.
//   - task_dependency: (task_id, depends_on) = (depender, dependee).
//     The whole task.SetDependency path treats task_id as the depender;
//     tasks.depends_on cache mirrors the same direction.
//
// Canonical mapping into relationships:
//   src = depender, dst = dependee, kind = EdgeDependsOn.
//
// The migration is idempotent: composite PK (src_id, dst_id, kind,
// valid_from) plus a Get() probe before each insert. Re-running on an
// already-migrated DB inserts zero rows.
package relationships

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

// MigrateLegacyDeps copies every row out of product_dependencies,
// manifest_dependencies, and task_dependency into the relationships
// table as EdgeDependsOn rows. Returns counts per source for the boot
// log line. Safe to run on every startup.
//
// Skips:
//   - rows whose src or dst is empty (task_dependency clear-marker rows
//     with depends_on='' are NOT edges; they just record that a dep was
//     cleared in the SCD audit stream — meaningless once the audit has
//     moved into relationships SCD semantics).
//   - rows that already have an exactly matching relationships row
//     (probed via Get on the (src, dst, kind) tuple — covers the
//     re-run case where a different valid_from would otherwise be
//     accepted by the composite PK).
//
// task_dependency rows carry both current (valid_to='') AND closed
// historical revisions. We migrate ALL of them so the audit trail
// stays intact in the new store: closed rows land with their original
// valid_from + valid_to, current rows land with valid_to=''.
func (s *Store) MigrateLegacyDeps(ctx context.Context) (int, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("relationships: nil store / db")
	}
	productCount, err := s.migrateProductDeps(ctx)
	if err != nil {
		return 0, fmt.Errorf("migrate product deps: %w", err)
	}
	manifestCount, err := s.migrateManifestDeps(ctx)
	if err != nil {
		return 0, fmt.Errorf("migrate manifest deps: %w", err)
	}
	taskCount, err := s.migrateTaskDeps(ctx)
	if err != nil {
		return 0, fmt.Errorf("migrate task deps: %w", err)
	}
	total := productCount + manifestCount + taskCount
	if total > 0 {
		slog.Info("relationships: backfilled legacy dep rows",
			"product", productCount, "manifest", manifestCount,
			"task", taskCount, "total", total)
	}
	return total, nil
}

// migrateProductDeps copies product_dependencies → relationships.
// created_at is INTEGER unix seconds in the legacy schema; converted
// to RFC3339Nano UTC to match relationships' TEXT timestamp format.
func (s *Store) migrateProductDeps(ctx context.Context) (int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT product_id, depends_on_product_id, created_at, created_by
		 FROM product_dependencies`)
	if err != nil {
		// Missing table on a fresh DB is fine — nothing to migrate.
		if isNoSuchTable(err) {
			return 0, nil
		}
		return 0, err
	}
	defer rows.Close()

	type legacy struct {
		src, dst, by string
		ts           int64
	}
	var pending []legacy
	for rows.Next() {
		var l legacy
		if err := rows.Scan(&l.src, &l.dst, &l.ts, &l.by); err != nil {
			return 0, err
		}
		pending = append(pending, l)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	inserted := 0
	for _, l := range pending {
		if l.src == "" || l.dst == "" || l.src == l.dst {
			continue
		}
		// Idempotency probe — Get returns the current edge, which is
		// what re-runs would collide with. valid_to='' is the only
		// thing we'd be re-creating in steady state.
		if _, found, err := s.Get(ctx, l.src, l.dst, EdgeDependsOn); err != nil {
			return inserted, err
		} else if found {
			continue
		}
		e := Edge{
			SrcKind:   KindProduct,
			SrcID:     l.src,
			DstKind:   KindProduct,
			DstID:     l.dst,
			Kind:      EdgeDependsOn,
			ValidFrom: time.Unix(l.ts, 0).UTC().Format(time.RFC3339Nano),
			ValidTo:   "",
			CreatedBy: l.by,
			Reason:    "backfill from legacy product_dependencies table",
		}
		if err := s.BackfillRow(ctx, e); err != nil {
			return inserted, fmt.Errorf("product_dependencies %s→%s: %w", l.src, l.dst, err)
		}
		inserted++
	}
	return inserted, nil
}

// migrateManifestDeps copies manifest_dependencies → relationships.
// Same INTEGER-unix-seconds → RFC3339 conversion as products.
func (s *Store) migrateManifestDeps(ctx context.Context) (int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT manifest_id, depends_on_manifest_id, created_at, created_by
		 FROM manifest_dependencies`)
	if err != nil {
		if isNoSuchTable(err) {
			return 0, nil
		}
		return 0, err
	}
	defer rows.Close()

	type legacy struct {
		src, dst, by string
		ts           int64
	}
	var pending []legacy
	for rows.Next() {
		var l legacy
		if err := rows.Scan(&l.src, &l.dst, &l.ts, &l.by); err != nil {
			return 0, err
		}
		pending = append(pending, l)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	inserted := 0
	for _, l := range pending {
		if l.src == "" || l.dst == "" || l.src == l.dst {
			continue
		}
		if _, found, err := s.Get(ctx, l.src, l.dst, EdgeDependsOn); err != nil {
			return inserted, err
		} else if found {
			continue
		}
		e := Edge{
			SrcKind:   KindManifest,
			SrcID:     l.src,
			DstKind:   KindManifest,
			DstID:     l.dst,
			Kind:      EdgeDependsOn,
			ValidFrom: time.Unix(l.ts, 0).UTC().Format(time.RFC3339Nano),
			ValidTo:   "",
			CreatedBy: l.by,
			Reason:    "backfill from legacy manifest_dependencies table",
		}
		if err := s.BackfillRow(ctx, e); err != nil {
			return inserted, fmt.Errorf("manifest_dependencies %s→%s: %w", l.src, l.dst, err)
		}
		inserted++
	}
	return inserted, nil
}

// migrateTaskDeps copies task_dependency → relationships. Differs from
// the other two:
//
//   - timestamp columns are already RFC3339 TEXT, no conversion needed.
//   - the table is SCD-2 itself (valid_from/valid_to rows + closed
//     audit history), so we copy ALL rows including closed ones.
//   - rows with depends_on='' are clear-marker audit entries (a dep
//     was deliberately removed). Those don't translate to a
//     relationships row — the equivalent in the new store is the
//     CLOSE side of the prior edge's row, which we already capture
//     when migrating that prior edge's valid_to. Skip the marker rows.
//
// changed_by + reason columns map to created_by + reason in
// relationships with no transform.
func (s *Store) migrateTaskDeps(ctx context.Context) (int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT task_id, depends_on, valid_from, valid_to, changed_by, reason
		 FROM task_dependency`)
	if err != nil {
		if isNoSuchTable(err) {
			return 0, nil
		}
		return 0, err
	}
	defer rows.Close()

	type legacy struct {
		src, dst, vfrom, vto, by, reason string
	}
	var pending []legacy
	for rows.Next() {
		var l legacy
		if err := rows.Scan(&l.src, &l.dst, &l.vfrom, &l.vto, &l.by, &l.reason); err != nil {
			return 0, err
		}
		pending = append(pending, l)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	inserted := 0
	for _, l := range pending {
		if l.src == "" || l.dst == "" || l.src == l.dst {
			// Empty dst = clear-marker audit row — see docstring.
			continue
		}
		// Idempotency probe is keyed on (src, dst, kind, valid_from)
		// at the PK level. Get() only checks the current row, so for
		// closed rows we lean on the composite PK + INSERT failure
		// being benign here. To stay clean, probe by exact valid_from
		// match instead.
		var existing int
		if err := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM relationships
			 WHERE src_id = ? AND dst_id = ? AND kind = ? AND valid_from = ?`,
			l.src, l.dst, EdgeDependsOn, l.vfrom).Scan(&existing); err != nil {
			return inserted, err
		}
		if existing > 0 {
			continue
		}
		e := Edge{
			SrcKind:   KindTask,
			SrcID:     l.src,
			DstKind:   KindTask,
			DstID:     l.dst,
			Kind:      EdgeDependsOn,
			ValidFrom: l.vfrom,
			ValidTo:   l.vto,
			CreatedBy: l.by,
			Reason:    firstNonEmpty(l.reason, "backfill from legacy task_dependency table"),
		}
		if err := s.BackfillRow(ctx, e); err != nil {
			return inserted, fmt.Errorf("task_dependency %s→%s @%s: %w", l.src, l.dst, l.vfrom, err)
		}
		inserted++
	}
	return inserted, nil
}

// firstNonEmpty returns the first non-empty string from the args. Used
// to keep the legacy `reason` column when populated and fall back to
// the migration tag otherwise.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// isNoSuchTable returns true when err is SQLite's missing-table error.
// Lets the migration treat a fresh DB (no legacy tables) as a zero-row
// migrate rather than a failure. String match is fine here — this is
// the SQLite-only boot path; the relationships package overall remains
// portable because Create / Read / Walk all use parameterised queries.
func isNoSuchTable(err error) bool {
	if err == nil {
		return false
	}
	if err == sql.ErrNoRows {
		return false
	}
	msg := err.Error()
	return contains(msg, "no such table")
}

// contains is a tiny stdlib-free substring check so this file doesn't
// pull strings into its import set.
func contains(s, sub string) bool {
	if len(sub) > len(s) {
		return false
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
