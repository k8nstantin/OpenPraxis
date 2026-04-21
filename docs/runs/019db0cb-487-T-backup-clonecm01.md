# T-backup-clonecm01 — run notes

**Task:** 019db0cb-487b-7a3d-9dde-f614c10a1491
**Manifest:** 019db0c2-61d (INT MySQL Backup to GCS)
**Target:** clone-iiaflcm01 (us-central1-a, gryphon-int)
**Run at:** 2026-04-21T18:45Z

## Outcome: BLOCKED — did not kick off backup

Per visceral rule #6 (test connections first; report, don't improvise) and #1
(never fix scripts on the fly — fix the source), the backup was not started.

## Preconditions checked

| Check | Result |
|---|---|
| `gcloud compute ssh clone-iiaflcm01 --zone=us-central1-a --project=gryphon-int --internal-ip` | OK — hostname resolves, login succeeds |
| `~/simple_int_data_and_schema_dump.sh` present | **MISSING** |
| `simple_int_data_and_schema_dump.sh` anywhere on host (`sudo find /`) | **NOT FOUND** |
| `/mnt/scratch/` present | **MISSING** — `ls: cannot access '/mnt/scratch/': No such file or directory` |
| Any `*dump*.sh` under `/home /opt /usr/local` | none |

## Why this is a manifest-level blocker, not a workaround candidate

The manifest spec (019db0c2-61d) assumes the clone has the dump script and the
scratch mount in place. Creating a replacement script ad-hoc would violate
rules #1 and #6. The fix belongs in whatever provisioning path installs
`simple_int_data_and_schema_dump.sh` and mounts `/mnt/scratch` on clone VMs —
not in this task's worktree.

## What the operator needs to do

1. Confirm which provisioning path (Ansible / Terraform / image) is supposed
   to deliver `simple_int_data_and_schema_dump.sh` to the clone homes.
2. Confirm which mount unit / disk attachment is supposed to back
   `/mnt/scratch` on clone-iiaflcm01.
3. Re-run this task after both are in place. SSH itself is healthy.
