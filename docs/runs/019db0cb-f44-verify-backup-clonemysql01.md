# T-verify-backup-clonemysql01 — run log

Task: `019db0cb-f44` (verify task for main backup `019db0cb-5cf`)
Manifest: `019db0c2-61d` (INT MySQL Backup to GCS — per-clone-node tasks)

## Run #1 — 2026-04-21T16:56Z

SSH: `gcloud compute ssh clone-iiaflmysql01 --zone=us-central1-c --project=gryphon-int --internal-ip`

Checks:
- `screen -ls | grep mysql-backup` → `27835.mysql-backup (Detached)`
- `pgrep -af mysqldump` → 6 active dumps + 8 script shells
- `tail -100 ~/backup-20260421-122517.log` → schema done (23M), parallel data dump in progress, cr_debug finished
- `gcloud storage ls gs://mysqldump_migration/int-clonemysql01/` → target dir `2026-04-21_12-25-21/` not yet uploaded (still staging locally to `/mnt/scratch`)

Decision: **still running** → progress comment posted on main task `019db0cb-5cf` (`agent_note` id `019db0de-7552`). No self-cancel.

Next fire: +30 min (scheduler).

## Run #3 — 2026-04-21T17:31Z

SSH: same args.

State: dump phase **complete** at 13:02:29 (all 7 DBs, ≈ 211 GiB); GCS upload **in progress** via `gsutil -m cp -r` (PID 15639).

GCS listing: 4/7 data files uploaded (cr_debug, spr01, gdx, gm) = 58.16 GiB. Pending: core_gdxws (33 GiB), coreint_TMobile (35 GiB), coreint_Charter (85 GiB). `all_schema.sql` (23 MiB) already on GCS.

Observed upload rate: ~33 MB/s effective (parallel composite). ETA to GCS-complete: ~75–90 min → ≈ 18:45–19:00 UTC.

Decision: **still running (upload phase)** → progress comment `agent_note` id `019db119-c81d` posted on main task `019db0cb-5cf`. No `review_approval` yet; no self-cancel.

Note: run #2 occurred at 17:00Z (comment `019db0fc-1068` on main task) but was not logged here — captured now for completeness.

Next fire: +30 min.

## Run #4 — 2026-04-21T18:00Z — TERMINAL ✅

SSH: same args.

State: backup **fully complete**. Script ran 84m 56s (T0 12:25:17 → finish 13:50:18).

- `screen -ls` → no `mysql-backup` session
- `pgrep mysqldump` → no processes
- Log sentinel: `Status: SUCCESS`

GCS final inventory at `gs://mysqldump_migration/int-clonemysql01/2026-04-21_12-25-21/`:

| Object | Bytes | ~GiB |
|---|---:|---:|
| all_schema.sql | 24,082,972 | 0.022 |
| dump_2026-04-21_12-25-21.log | 2,651 | — |
| data/coreint_Charter.sql | 90,764,807,959 | 84.5 |
| data/coreint_TMobile.sql | 37,935,165,757 | 35.3 |
| data/core_gdxws.sql | 35,430,781,343 | 33.0 |
| data/coreint_gdx.sql | 21,486,235,141 | 20.0 |
| data/coreint_gm.sql | 21,372,150,905 | 19.9 |
| data/coreint_spr01.sql | 19,592,077,381 | 18.2 |
| data/cr_debug.sql | 16,979 | ~0 |
| **data/ total** | **226,581,235,465** | **211.02** |

Decision: **SUCCESS** → `review_approval` posted on main task `019db0cb-5cf` (comment `019db137-386e`). Self-cancel requested; `task_cancel` permission denied by harness — operator may need to cancel manually.

Acceptance criteria (manifest `019db0c2-61d`):
- [x] `all_schema.sql` present, size > 0
- [x] `data/*.sql` — 7 files (one per database in BACKUP SUMMARY)
- [x] Every progress comment used `agent_note` (never `watcher_finding`) with size / expected / % / rate / ETA
- [x] Terminal verdict posted as `review_approval` on main task

## Run #5 — 2026-04-21T21:10Z — no-op (already terminal)

SSH to `clone-iiaflmysql01 --zone=us-central1-a` (note: manifest spec says `us-central1-c` but actual zone is `us-central1-a` — worth fixing in manifest 019db0c2-61d).

Re-verified state:
- `screen -ls` → no `mysql-backup` session
- `pgrep mysqldump` → none
- `~/backup-*.log` → cleaned up / absent on host
- GCS `gs://mysqldump_migration/int-clonemysql01/2026-04-21_12-25-21/` still intact: `all_schema.sql` 22.97 MiB + 7 `data/*.sql` files totalling 211.02 GiB — unchanged since run #4.

Decision: backup remains SUCCESS and already `review_approval`-ed in run #4 (`019db137-386e`). No new comment on main task — would be noise. This recurring verify task has nothing more to do; operator should cancel task `019db0cb-f44` to stop the 30m wake-ups. Per visceral rule #12 (confirm before cancelling), agent still not self-cancelling.
