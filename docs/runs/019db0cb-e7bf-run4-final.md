# T-verify-backup-clonecm02 — run 4 / FINAL (task 019db0cb-e7b)

**Wake:** 2026-04-21 ~23:10Z (30m scheduled).
**T0 of main (019db0cb-52c):** 2026-04-21T21:04:43Z (17:04:47 clone-local).
**End (script):** 2026-04-21T23:04:02Z. Duration 119m 15s.
**Clone:** clone-iiaflcm02 / us-central1-a / gryphon-int.

## Final state at this fire

- `screen -ls | grep mysql-backup`: (empty) — screen ended.
- `pgrep -af mysqldump`: (empty) — no dump processes.
- `ls /mnt/scratch/`: (empty) — cleaned up.
- Log: `~/backup-20260421-210443.log` (69 lines). Summary banner: `Status: FAILED (1 dump failures, GCS=0)`.
- Script-reported errors:
  - 17:06:14 `[ERROR] Schema dump FAILED`
  - 17:06:16 `[ERROR] [mstr] Data FAILED`
  - 18:22:30 `[WARN] 1 failure(s) during dump`

## GCS artifacts (gs://mysqldump_migration/int-clonecm02/2026-04-21_17-04-47/)

| Object | Bytes | Mtime UTC |
|---|---|---|
| all_schema.sql | 29,257,741 | 22:22:33 |
| data/core_audit.sql | 157,834,375,947 | 22:53:33 |
| data/core_backup.sql | 44,703,222 | 22:22:38 |
| data/core_config.sql | 672,730,839 | 22:23:00 |
| data/core_manager.sql | 4,945,150,476 | 22:24:36 |
| data/core_report.sql | 239,890,180,833 | 23:03:59 |
| data/core_services.sql | 31,394,154,917 | 22:29:42 |
| data/coreint_cpe1.sql | 22,893,021,884 | 22:31:02 |
| data/coreint_ws.sql | 112,572,746,798 | 22:39:59 |
| data/cr_debug.sql | 1,369,032 | 22:22:35 |
| data/mstr.sql | 4,704,942 | 22:22:35 |
| dump_…log | 4,405 | 23:04:01 |

Totals: 10/10 data files, 531.09 GiB. All sizes match on-disk sizes from run3.

## Spot-check results (contradicting script self-report)

- **all_schema.sql** — downloaded & inspected:
  - 10 `CREATE DATABASE` statements for all 10 DBs.
  - Table counts: core_audit=3140, core_backup=9, core_config=391, core_manager=361, core_report=896, core_services=561, coreint_cpe1=1344, coreint_ws=395, cr_debug=5, mstr=10.
  - Footer present: `-- Dump completed on 2026-04-21 17:06:14`.
  - Verdict: **complete**, despite script saying schema dump FAILED.
- **mstr.sql** — downloaded & inspected:
  - Valid header, `CREATE DATABASE mstr`, `USE mstr`, LOCK TABLES writes for MOISES_tbl_Dates.
  - Footer: `-- Dump completed on 2026-04-21 17:06:16`.
  - Size matches on-disk (4,704,942 B). Verdict: **complete**, despite script flagging FAILED.

## Decision

- Per manifest 019db0c2-61d spec Step 4 (screen ended + log error) → **post `review_rejection`** on main task 019db0cb-52c.
- Rejection cites script's FAILED self-report as authoritative signal; attaches spot-check evidence showing artifacts are in fact complete. Operator can decide: accept artifacts as-is, or re-run for a clean success banner.
- Root cause note: the script's schema-dump error path and mstr tracking appear to produce intact files but set the failure counter anyway — a bug in `simple_int_data_and_schema_dump.sh` error accounting (per visceral rule #1/#2, fix in the source script, not on the fly).
- `task_cancel` on SELF (019db0cb-e7bf-779d-9473-44af68904e7a).
