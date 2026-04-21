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
