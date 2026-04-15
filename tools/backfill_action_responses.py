#!/usr/bin/env python3
"""
Backfill task actions from stored stream-json output.

Handles two cases:
1. Actions exist but tool_response is empty (bug: wrong event type for parsing)
2. No actions exist but output is stored (tasks ran before action capture was added)

In both cases, reparses the stored stream-json output and fills/creates action rows.

Usage:
    python3 tools/backfill_action_responses.py                    # dry-run (default)
    python3 tools/backfill_action_responses.py --apply            # apply changes
    python3 tools/backfill_action_responses.py --task-id <id>     # single task
    python3 tools/backfill_action_responses.py --db /path/to.db   # custom DB path
"""

import argparse
import json
import os
import sqlite3
import sys


def get_default_db_path():
    return os.path.expanduser("~/.openloom/data/memories.db")


def truncate(text, max_len=5000):
    """Truncate to match Go behavior."""
    if len(text) > max_len:
        return text[:max_len] + "..."
    return text


def parse_stream_json(output):
    """Parse stream-json output and pair tool_use with tool_result events.

    Returns a list of dicts with keys: tool_name, tool_use_id, tool_input, tool_response.
    Order is chronological (same as execution order).
    """
    pairs = []
    pending = {}  # tool_use_id -> dict with tool info

    for line in output.split("\n"):
        line = line.strip()
        if not line:
            continue
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue

        event_type = event.get("type", "")

        if event_type == "assistant":
            msg = event.get("message", {})
            for block in msg.get("content", []):
                if isinstance(block, dict) and block.get("type") == "tool_use":
                    tool_id = block.get("id", "")
                    tool_name = block.get("name", "")
                    tool_input = block.get("input", "")
                    if isinstance(tool_input, (dict, list)):
                        tool_input = json.dumps(tool_input)
                    pending[tool_id] = {
                        "tool_name": tool_name,
                        "tool_use_id": tool_id,
                        "tool_input": str(tool_input),
                        "tool_response": "",
                    }

        elif event_type == "user":
            msg = event.get("message", {})
            for block in msg.get("content", []):
                if isinstance(block, dict) and block.get("type") == "tool_result":
                    tool_use_id = block.get("tool_use_id", "")
                    result_content = block.get("content", "")
                    if isinstance(result_content, (dict, list)):
                        result_content = json.dumps(result_content)

                    if tool_use_id in pending:
                        entry = pending.pop(tool_use_id)
                        entry["tool_response"] = str(result_content)
                        pairs.append(entry)

    # Add any pending tools that never got a response (task was killed/timed out)
    for entry in pending.values():
        pairs.append(entry)

    return pairs


def backfill_task(conn, task_id, task_title, task_output, source_node, dry_run=True):
    """Backfill actions for a single task. Creates or updates as needed."""
    pairs = parse_stream_json(task_output)
    if not pairs:
        print(f"  No tool calls found in output")
        return 0

    # Check existing actions for this task
    existing = conn.execute(
        "SELECT id, tool_name, tool_input, tool_response FROM actions "
        "WHERE task_id = ? ORDER BY created_at ASC",
        (task_id,),
    ).fetchall()

    if existing:
        # Mode 1: Update existing actions with empty responses
        updated = 0
        pair_idx = 0
        for action_id, action_tool, action_input, existing_response in existing:
            if pair_idx >= len(pairs):
                break

            pair = pairs[pair_idx]
            if action_tool == pair["tool_name"]:
                if not existing_response or existing_response.strip() == "":
                    response = truncate(pair["tool_response"])
                    if response:
                        if dry_run:
                            preview = response[:80] + "..." if len(response) > 80 else response
                            print(f"  [UPDATE] Action #{action_id} {action_tool}: set response ({len(response)} chars)")
                        else:
                            conn.execute(
                                "UPDATE actions SET tool_response = ? WHERE id = ?",
                                (response, action_id),
                            )
                        updated += 1
                pair_idx += 1
            else:
                pair_idx += 1

        return updated

    else:
        # Mode 2: Create new action rows from parsed output
        created = 0
        for pair in pairs:
            tool_input = truncate(pair["tool_input"])
            tool_response = truncate(pair["tool_response"])

            if dry_run:
                print(f"  [CREATE] {pair['tool_name']}: input={len(tool_input)}c response={len(tool_response)}c")
            else:
                conn.execute(
                    "INSERT INTO actions (session_id, source_node, task_id, tool_name, tool_input, tool_response, cwd, created_at) "
                    "VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'))",
                    (f"task:{task_id}", source_node, task_id, pair["tool_name"], tool_input, tool_response, ""),
                )
            created += 1

        return created


def main():
    parser = argparse.ArgumentParser(description="Backfill task actions from stored output")
    parser.add_argument("--db", default=get_default_db_path(), help="Path to SQLite database")
    parser.add_argument("--task-id", help="Backfill a single task (prefix match)")
    parser.add_argument("--apply", action="store_true", help="Apply changes (default is dry-run)")
    args = parser.parse_args()

    if not os.path.exists(args.db):
        print(f"Database not found: {args.db}", file=sys.stderr)
        sys.exit(1)

    conn = sqlite3.connect(args.db)
    dry_run = not args.apply

    if dry_run:
        print("=== DRY RUN (use --apply to write changes) ===\n")

    # Find tasks with stored output
    if args.task_id:
        rows = conn.execute(
            "SELECT id, title, last_output, source_node FROM tasks "
            "WHERE id LIKE ? AND last_output IS NOT NULL AND last_output != ''",
            (args.task_id + "%",),
        ).fetchall()
    else:
        rows = conn.execute(
            "SELECT id, title, last_output, source_node FROM tasks "
            "WHERE last_output IS NOT NULL AND last_output != ''"
        ).fetchall()

    if not rows:
        print("No tasks with stored output found.")
        conn.close()
        return

    total = 0
    for task_id, title, output, source_node in rows:
        action_count = conn.execute(
            "SELECT COUNT(*) FROM actions WHERE task_id = ?", (task_id,)
        ).fetchone()[0]

        empty_count = conn.execute(
            "SELECT COUNT(*) FROM actions WHERE task_id = ? AND (tool_response IS NULL OR tool_response = '')",
            (task_id,),
        ).fetchone()[0]

        pairs = parse_stream_json(output)

        print(f"Task [{task_id[:8]}] {title}")
        print(f"  Existing actions: {action_count} ({empty_count} missing responses)")
        print(f"  Parsed from output: {len(pairs)} tool calls")

        if action_count > 0 and empty_count == 0:
            print(f"  -> All responses already filled, skipping\n")
            continue

        if action_count == 0 and len(pairs) == 0:
            print(f"  -> No tool calls found, skipping\n")
            continue

        count = backfill_task(conn, task_id, title, output, source_node or "", dry_run)
        action = "Would create/update" if dry_run else "Created/updated"
        total += count
        print(f"  -> {action} {count} actions\n")

    if args.apply:
        conn.commit()
        print(f"\nApplied: {total} actions across {len(rows)} tasks")
    else:
        print(f"\nDry run total: {total} actions would be created/updated across {len(rows)} tasks")

    conn.close()


if __name__ == "__main__":
    main()
