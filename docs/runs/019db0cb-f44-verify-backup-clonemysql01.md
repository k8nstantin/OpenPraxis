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

