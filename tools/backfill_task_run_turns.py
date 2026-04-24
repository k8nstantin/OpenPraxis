#!/usr/bin/env python3
"""Backfill task_runs.turns for rows where the value is 0 but the stored output
contains assistant events (the run was killed before the SDK's `result` event
fired, so handlers_task.go:parseTaskResultMetrics recorded turns=0).

Mirrors the Go fallback added in the same change set: scan the JSONL output for
`type=result` first (clean exit, take its num_turns); otherwise count
`type=assistant` events.

Usage: python3 tools/backfill_task_run_turns.py [DB_PATH]
       Defaults to ~/.openpraxis/data/memories.db.
Idempotent — only touches rows whose turns column is currently 0."""
from __future__ import annotations

import json
import os
import sqlite3
import sys


def parse_turns(output: str) -> int:
    if not output:
        return 0
    lines = [ln.strip() for ln in output.split("\n") if ln.strip()]
    # Pass 1: result event (clean exit) wins.
    for ln in reversed(lines):
        try:
            ev = json.loads(ln)
        except json.JSONDecodeError:
            continue
        if isinstance(ev, dict) and ev.get("type") == "result":
            return int(ev.get("num_turns", 0))
    # Pass 2: count assistant events as a turn count proxy.
    return sum(
        1
        for ln in lines
        if (lambda e: isinstance(e, dict) and e.get("type") == "assistant")(
            _try_load(ln)
        )
    )


def _try_load(line: str):
    try:
        return json.loads(line)
    except json.JSONDecodeError:
        return None


def main() -> int:
    db_path = (
        sys.argv[1]
        if len(sys.argv) > 1
        else os.path.expanduser("~/.openpraxis/data/memories.db")
    )
    if not os.path.exists(db_path):
        print(f"DB not found: {db_path}", file=sys.stderr)
        return 1

    conn = sqlite3.connect(db_path)
    conn.row_factory = sqlite3.Row
    cur = conn.cursor()
    cur.execute(
        "SELECT id, task_id, run_number, output FROM task_runs "
        "WHERE turns = 0 AND output != ''"
    )
    rows = cur.fetchall()
    print(f"candidates with turns=0: {len(rows)}")

    updated = 0
    for r in rows:
        turns = parse_turns(r["output"])
        if turns <= 0:
            continue
        conn.execute(
            "UPDATE task_runs SET turns = ? WHERE id = ?",
            (turns, r["id"]),
        )
        updated += 1
        if updated <= 10 or updated % 25 == 0:
            print(
                f"  updated id={r['id']} task={r['task_id'][:12]} "
                f"run#{r['run_number']} → turns={turns}"
            )
    conn.commit()
    conn.close()
    print(f"updated rows: {updated}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
