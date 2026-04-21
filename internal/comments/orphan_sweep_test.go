package comments

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
)

// stubResolver implements TargetResolver using two maps keyed by target_type
// then short-marker prefix → full UUID. Unknown keys return "".
type stubResolver struct {
	full map[TargetType]map[string]string
}

func (r *stubResolver) Resolve(ctx context.Context, t TargetType, id string) (string, error) {
	inner, ok := r.full[t]
	if !ok {
		return "", nil
	}
	return inner[id], nil
}

// seedOrphan inserts a comment row with an arbitrary target_id (may be short).
func seedOrphan(t *testing.T, db *sql.DB, target TargetType, targetID string) string {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("uuid: %v", err)
	}
	_, err = db.Exec(
		`INSERT INTO comments (id, target_type, target_id, author, type, body, created_at)
		 VALUES (?, ?, ?, 'agent', 'user_note', 'body', ?)`,
		id.String(), string(target), targetID, time.Now().Unix(),
	)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	return id.String()
}

func TestSweepOrphans_DryRunReportsButDoesNotWrite(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	fullTaskA := "019dafbc-31c0-7000-8000-000000000001"
	fullManB := "019dafe2-5030-7000-8000-000000000002"

	shortTaskA := fullTaskA[:12]
	shortManB := fullManB[:12]

	cA := seedOrphan(t, db, TargetTask, shortTaskA)
	cB := seedOrphan(t, db, TargetManifest, shortManB)
	cGhost := seedOrphan(t, db, TargetTask, "deadbeefcafe")

	// Also seed a non-orphan (full UUID length) row that should be skipped.
	_ = seedOrphan(t, db, TargetProduct, "019dabbf-2df0-7000-8000-000000000003")

	r := &stubResolver{full: map[TargetType]map[string]string{
		TargetTask:     {shortTaskA: fullTaskA},
		TargetManifest: {shortManB: fullManB},
	}}

	rep, err := SweepOrphans(ctx, db, r, true)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if rep.Scanned != 3 {
		t.Fatalf("scanned: want 3, got %d", rep.Scanned)
	}
	if rep.Migrated != 2 {
		t.Fatalf("migrated: want 2, got %d", rep.Migrated)
	}
	if rep.Unresolvable != 1 {
		t.Fatalf("unresolvable: want 1, got %d", rep.Unresolvable)
	}
	if rep.ByTargetType["task"] != 1 || rep.ByTargetType["manifest"] != 1 {
		t.Fatalf("by-type: %+v", rep.ByTargetType)
	}

	// Verify nothing actually changed on disk.
	for _, id := range []string{cA, cB, cGhost} {
		var got string
		if err := db.QueryRow(`SELECT target_id FROM comments WHERE id = ?`, id).Scan(&got); err != nil {
			t.Fatalf("query %s: %v", id, err)
		}
		if len(got) >= 36 {
			t.Fatalf("dry-run mutated row %s: %s", id, got)
		}
	}
}

func TestSweepOrphans_ApplyMigratesResolvableOnly(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	fullTaskA := "019dafbc-31c0-7000-8000-000000000001"
	fullManB := "019dafe2-5030-7000-8000-000000000002"
	shortTaskA := fullTaskA[:12]
	shortManB := fullManB[:12]
	ghost := "deadbeefcafe"

	cA := seedOrphan(t, db, TargetTask, shortTaskA)
	cB := seedOrphan(t, db, TargetManifest, shortManB)
	cGhost := seedOrphan(t, db, TargetTask, ghost)

	r := &stubResolver{full: map[TargetType]map[string]string{
		TargetTask:     {shortTaskA: fullTaskA},
		TargetManifest: {shortManB: fullManB},
	}}

	rep, err := SweepOrphans(ctx, db, r, false)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if rep.Migrated != 2 || rep.Unresolvable != 1 {
		t.Fatalf("report: %+v", rep)
	}

	check := func(id, want string) {
		t.Helper()
		var got string
		if err := db.QueryRow(`SELECT target_id FROM comments WHERE id = ?`, id).Scan(&got); err != nil {
			t.Fatalf("query %s: %v", id, err)
		}
		if got != want {
			t.Fatalf("row %s: want target_id=%q, got %q", id, want, got)
		}
	}
	check(cA, fullTaskA)
	check(cB, fullManB)
	check(cGhost, ghost) // unresolvable left alone

	// Second run: everything resolvable is now length==36, so only the
	// ghost is rescanned and still unresolvable; no new migrations.
	rep2, err := SweepOrphans(ctx, db, r, false)
	if err != nil {
		t.Fatalf("sweep 2: %v", err)
	}
	if rep2.Migrated != 0 {
		t.Fatalf("idempotency: expected 0 migrated on 2nd run, got %d", rep2.Migrated)
	}
	if rep2.Scanned != 1 || rep2.Unresolvable != 1 {
		t.Fatalf("idempotency: expected 1 scanned/1 unresolvable on 2nd run, got %+v", rep2)
	}
}
