# Migrations runbook

Reliability Recovery P9. The PR/M3 column-drop migration (`DropOwnershipColumns`)
landed before this runbook existed. Subsequent column-drop or schema-altering
migrations follow the procedure below.

## Pre-flight (every migration that mutates schema)

1. **Snapshot memories.db.**

   ```bash
   cp ~/.openpraxis/data/memories.db \
      ~/.openpraxis/data/memories.db.backup-pre-<migration-name>-$(date +%Y%m%d-%H%M%S)
   ```

   Don't skip this for "tiny" migrations — column drops are not reversible
   from main without a backup.

2. **Confirm no running tasks.**

   ```bash
   curl -s http://localhost:8765/api/tasks?status=running | jq 'length'
   ```

   If non-zero, wait or cancel — restart-during-migration orphans them.

3. **Diff the migration against the live schema.**

   ```bash
   sqlite3 ~/.openpraxis/data/memories.db '.schema <table>'
   ```

   Confirm columns the migration drops/renames actually exist in your
   live schema. Older snapshots may already be ahead.

## Dry run

Run the migration against a fresh DB before touching production:

```bash
cp ~/.openpraxis/data/memories.db /tmp/memories-dryrun.db
go run ./cmd/migrate --db /tmp/memories-dryrun.db --up
sqlite3 /tmp/memories-dryrun.db '.schema <affected-table>'
```

If the dry-run schema doesn't match expectations, do not promote.

## Apply

```bash
cd ~/openloom-serve
git pull github main
go build -o openpraxis ./
# stop serve cleanly (operator)
./openpraxis serve
```

The migration runs at startup. Watch the log for `migration applied` lines.

## Post-deploy validation

1. `curl http://localhost:8765/api/products` returns 200 with non-empty body.
2. `curl http://localhost:8765/api/manifests` returns 200 with non-empty body.
3. `curl http://localhost:8765/api/tasks?status=running` returns 200.
4. Open the Portal V2 dashboard (`/`) and click into a product → manifest →
   task. Stats / Dependencies / Activity tabs render.
5. `go test ./...` against the live binary's source tree.

## Rollback

Migrations are NOT auto-reversible. To roll back a column drop you
must restore from the pre-flight backup:

```bash
# stop serve
cp ~/.openpraxis/data/memories.db.backup-pre-<name>-<ts> \
   ~/.openpraxis/data/memories.db
# checkout prior binary
git -C ~/openloom-serve checkout <prior-commit> -- openpraxis
./openpraxis serve
```

Always keep the backup until the migration has soaked for at least 24 hours
in production with no incident.

## Migration authoring checklist

When adding a new migration:

- [ ] Wrap the schema change in `BEGIN TRANSACTION; … COMMIT;`.
- [ ] Add a unit test that runs the migration against an empty DB and asserts
      the resulting schema (mirror the pattern in `internal/task/migrate_test.go`).
- [ ] Document the migration in `docs/changelog.md` with the column / table
      affected and the rollback recipe.
- [ ] Confirm the change is forward-compatible with one prior binary version
      (the operator's `~/openloom-serve` may be N-1 during a rolling restart).
