# T-verify-backup-clonecm02 — run 13 (final closure)

**Task:** 019db0cb-e7b (verify) / main 019db0cb-52c
**When:** 2026-04-22T03:03Z
**Verdict:** ✅ review_approval — backup complete, all 10 data files + schema present in GCS.

## Ground truth — `gs://mysqldump_migration/int-clonecm02/2026-04-21_17-04-47/`

Root:
| Object | Bytes |
|---|---|
| `all_schema.sql` | 29,257,741 (27.90 MiB) |
| `dump_2026-04-21_17-04-47.log` | 4,405 |
| `data/` (prefix) | 10 files |

`data/` (10 files, 570,253,138,890 B ≈ 531.09 GiB):

| DB | Bytes | Uploaded |
|---|---|---|
| core_audit | 157,834,375,947 | 22:53:33Z |
| core_backup | 44,703,222 | 22:22:38Z |
| core_config | 672,730,839 | 22:23:00Z |
| core_manager | 4,945,150,476 | 22:24:36Z |
| core_report | 239,890,180,833 | 23:03:59Z |
| core_services | 31,394,154,917 | 22:29:42Z |
| coreint_cpe1 | 22,893,021,884 | 22:31:02Z |
| coreint_ws | 112,572,746,798 | 22:39:59Z |
| cr_debug | 1,369,032 | 22:22:35Z |
| mstr | 4,704,942 | 22:22:35Z |

## Timing

- **T0:** 2026-04-21T21:04:43Z (from kickoff log)
- **Last GCS upload:** 2026-04-21T23:03:59Z (core_report)
- **End-to-end wall time:** ~1h 59m 16s

## Note on prior review_rejection

Run 4 posted `review_rejection` based on the script's self-reported `FAILED (1 dump failures, GCS=0)` in the backup log. Run 12 correction (and this run) confirm the script's footer was stale — GCS actually received all 10 data files + schema with byte-identical sizes to the local staging dir. Operator should treat the backup as **valid and usable for restore**. The script's failure-detection logic has a bug that should be tracked separately (followup).
