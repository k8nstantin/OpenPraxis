# T-backup-clonemysql01 — run log

Task: `019db0cb-5cfd-73cc-a398-80b39cc74f35`
Manifest: `019db0c2-61d` (INT MySQL Backup to GCS)
Fired: 2026-04-21 12:25:17 UTC-equivalent local TS

## Kickoff

- VM: `clone-iiaflmysql01` (zone `us-central1-c`, project `gryphon-int`, RUNNING)
- SSH: `gcloud compute ssh clone-iiaflmysql01 --zone=us-central1-c --project=gryphon-int --internal-ip`
- Screen: `27835.mysql-backup` (Detached)
- Log: `~/backup-20260421-122517.log`
- Command:
  ```
  screen -dmS mysql-backup bash -c '~/simple_int_data_and_schema_dump.sh \
      --gcs-path int-clonemysql01 \
      --gcs-bucket gs://mysqldump_migration \
      --parallel 8 \
      >~/backup-20260421-122517.log 2>&1'
  ```
- Expected GCS: `gs://mysqldump_migration/int-clonemysql01/<date-stamped-subdir>/`

## Pre-flight verified

- SSH reachable, host keys cached
- Script present: `/home/calexander_gryphonnetworks_com/simple_int_data_and_schema_dump.sh` (9469 bytes)
- No prior `mysql-backup` screens running
- Log file created (45 bytes at t+2s) — process writing

## Handoff

Review task (T-verify-backup-clonemysql01) auto-activates via `depends_on`.
It polls every 30m and posts `review_approval` / `review_rejection` on this task.
