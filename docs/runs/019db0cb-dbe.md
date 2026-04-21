# T-verify-backup-clonecm01 — run log

Task: `019db0cb-dbe` (verify) — main: `019db0cb-487` (T-backup-clonecm01).
Manifest: `019db0c2-61d` — INT MySQL Backup to GCS — per-clone-node tasks.

## T0 (from main task execution_review)

- Log file on clone: `~/backup-20260421-124004.log`
- Script `[INFO] Backup Dir` line timestamp: `2026-04-21 12:40:08` (clone local, UTC−4)
- T0 in UTC: `2026-04-21T16:40:08Z`
- Databases (5): `core_manager`, `coreint_cpe`, `cr_debug`, `gdx_services`, `mq_services`
- Script: `simple_int_data_and_schema_dump.sh --gcs-path int-clonecm01 --gcs-bucket gs://mysqldump_migration --parallel 8`
- Expected GCS: `gs://mysqldump_migration/int-clonecm01/2026-04-21_12-40-08/`

## Expected per-DB sizes (information_schema)

Not captured — verify-task runner lacks MySQL credentials on the clone (non-interactive `mysql -N` returned empty). Sizes are inferred from completed `data/*.sql` file sizes as the dump progresses.

## Progress snapshot @ 2026-04-21T17:26Z (elapsed ~46m)

| DB | File size | mtime (local) | State |
|---|---|---|---|
| cr_debug | 389 KB | 12:40 | done |
| mq_services | 17 MB | 12:40 | done (log says 12:40:19) |
| gdx_services | 8.30 GB | 12:42 | done (log says 12:42:51) |
| coreint_cpe | 33.70 GB | 12:50 | done (log says 12:50:45) |
| core_manager | 123.28 GB | 13:26 (growing) | dumping |

- Total staged on `/mnt/helper/2026-04-21_12-40-08/`: **165.31 GB** (data) + 17 MB (schema).
- Screen `15924.mysql-backup` alive; active `mysqldump` PID 15982 on `core_manager`.
- Rate on `core_manager` alone: 123.28 GB / ~46m ≈ **45 MB/s** (consistent with gdx/coreint_cpe rates of ~51 MB/s).
- GCS destination empty — script uploads only after all dumps finish.

## Notes

- Next verify scheduled at 17:54 Z (30 min cadence).
- `core_manager` dump still actively growing at snapshot time; no ETA yet since final size unknown (can't read `information_schema` sans creds).
