# T-verify-backup-clonecm02 — run log

Task id: `019db0cb-e7bf-779d-9473-44af68904e7a`
Main task (paired): `019db0cb-52c9-719a-a17b-69e7ce5a13e1` (T-backup-clonecm02)
Manifest: `019db0c2-61d` (INT MySQL Backup to GCS — per-clone-node tasks)
Clone VM: `clone-iiaflcm02` (zone `us-central1-a`, project `gryphon-int`)

## Run 1 — 2026-04-21T21:11Z

**Backup state:** RUNNING

- T0 (script start, from log filename): `2026-04-21T21:04:43Z`
- Check time: `2026-04-21T21:11:04Z`
- Elapsed: ~6m 21s
- Backup dir on clone: `/mnt/helper/2026-04-21_17-04-47/`
- GCS dest (expected, not yet populated): `gs://mysqldump_migration/int-clonecm02/2026-04-21_17-04-47/`
- Staged bytes: 55,003,702,865 B (~52 GiB)
- Aggregate rate: ~144 MB/s

### Per-DB on-disk status (from `ls -la /mnt/helper/…/data/`)

| DB | Size | Status |
|---|---|---|
| core_backup | 43 MB | done |
| core_config | 642 MB | done |
| cr_debug | 1.4 MB | done |
| core_manager | 4.7 GB | done |
| mstr | 4.7 MB | **FAILED** (log error at 17:06:16) |
| core_audit | 9.17 GB | dumping |
| coreint_cpe1 | 9.13 GB | dumping |
| coreint_ws | 9.30 GB | dumping |
| core_report | 10.78 GB | dumping |
| core_services | 10.93 GB | dumping |

### Expected per-DB sizes (information_schema)

**Not captured this run** — `mysql -uroot` returned access denied without password on clone-iiaflcm02. Need auth path (my.cnf, defaults-file, or env creds) to cache per-DB expected sizes. Tracking on-disk growth as fallback.

### Issues flagged for final verdict

1. Script logged `Schema dump FAILED` at 17:06:14, but `all_schema.sql` is 29 MB non-empty on disk — needs content inspection at final verdict.
2. `mstr` data dump FAILED at 17:06:15 — only 4.7 MB written, likely truncated or empty. Will fail acceptance criterion #3 unless mstr's legitimate size is ≈0 or the script retries.

### Next step

Recurring 30m schedule — next fire fires another verify pass. Self-cancel + `review_approval` / `review_rejection` when screen ends + GCS upload is observed.
