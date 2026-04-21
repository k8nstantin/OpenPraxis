# T-verify-backup-clonecm02 — run 3 (task 019db0cb-e7b)

**Wake:** 2026-04-21 ~22:27Z (scheduled 30m fire; next_run_at was 22:12Z).
**T0 of main (019db0cb-52c):** 2026-04-21T21:04:43Z.
**Clone:** clone-iiaflcm02 / us-central1-a / gryphon-int.

## State at this fire

- Local dumps: **COMPLETE** at 18:22:30 local (22:22:30Z UTC) — 1h 17m 47s.
- Staged: 570,282,409,228 B (≈531 GiB / 570 GB incl. schema).
- Log banner: `[WARN] 1 failure(s) during dump` — `mstr` Data FAILED at 17:06:16.
- `all_schema.sql`: uploaded to GCS at 22:22:33Z (29,257,741 B; MD5 re98pr6q/VrdZQHAXT4PnQ==).
- `data/*.sql`: gsutil -m upload IN PROGRESS. ~16 python3 gsutil workers alive. 0 data files in GCS yet (only schema + empty prefix).

## On-disk final sizes

| DB | Bytes | Status |
|---|---|---|
| core_audit | 157,834,375,947 | ok |
| core_backup | 44,703,222 | ok |
| core_config | 672,730,839 | ok |
| core_manager | 4,945,150,476 | ok |
| core_report | 239,890,180,833 | ok |
| core_services | 31,394,154,917 | ok |
| coreint_cpe1 | 22,893,021,884 | ok |
| coreint_ws | 112,572,746,798 | ok |
| cr_debug | 1,369,032 | ok |
| mstr | 4,704,942 | **flagged FAILED by script — content unverified** |

## Decision

- **No self-cancel.** Upload still running.
- Posted `agent_note` progress on main task 019db0cb-52c with sizing, rate, ETA, mstr/schema warnings carried forward.

## Carry-forward for final verdict

When upload completes (screen exits), next verify run must:

1. Confirm all 10 files in GCS `gs://mysqldump_migration/int-clonecm02/2026-04-21_17-04-47/data/` with byte sizes matching on-disk.
2. Spot-check `mstr.sql` locally (`head -50`, `tail -20`) to decide whether the `mstr Data FAILED` flag means partial output or just a non-fatal stderr warning.
3. Spot-check `all_schema.sql` (expect `CREATE DATABASE`/`CREATE TABLE` for all 10 DBs).
4. If mstr is truly partial → `review_rejection` + `agent_note` upstream explaining re-run needed. Else → `review_approval`.
5. Call `task_cancel` on self (019db0cb-e7bf-779d-9473-44af68904e7a).
