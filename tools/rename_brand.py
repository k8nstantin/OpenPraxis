#!/usr/bin/env python3
"""
Rename brand across the codebase.

Usage:
    python3 tools/rename_brand.py [--dry-run] [--root DIR]

Walks all matching files and replaces:
  - openloom  → openpraxis   (lowercase)
  - OpenLoom  → OpenPraxis   (capitalized)
  - OPENLOOM  → OPENPRAXIS   (uppercase)

Reports all changes made.
"""

import argparse
import os
import sys

# File extensions/names to process
EXTENSIONS = {
    '.go', '.js', '.html', '.css', '.md', '.yaml', '.yml',
    '.json', '.mod', '.sh', '.ts', '.tsx', '.py',
}
EXACT_NAMES = {'Makefile', '.gitignore'}

# Directories to skip
SKIP_DIRS = {'.git', 'node_modules', 'vendor', '.expo'}

# Files to skip (keep this script reusable per visceral rule #2)
SKIP_FILES = {'rename_brand.py'}

# Replacement pairs (order matters: longest/most-specific first to avoid partial matches)
REPLACEMENTS = [
    ('OPENLOOM', 'OPENPRAXIS'),
    ('OpenLoom', 'OpenPraxis'),
    ('openloom', 'openpraxis'),
]


def should_process(filepath):
    """Check if a file should be processed based on extension or name."""
    basename = os.path.basename(filepath)
    if basename in SKIP_FILES:
        return False
    if basename in EXACT_NAMES:
        return True
    _, ext = os.path.splitext(basename)
    return ext in EXTENSIONS


def process_file(filepath, dry_run=False):
    """Process a single file, returning list of (line_num, old_line, new_line) changes."""
    try:
        with open(filepath, 'r', encoding='utf-8', errors='ignore') as f:
            content = f.read()
    except (OSError, UnicodeDecodeError):
        return []

    new_content = content
    for old, new in REPLACEMENTS:
        new_content = new_content.replace(old, new)

    if new_content == content:
        return []

    # Compute per-line diff for reporting
    old_lines = content.splitlines()
    new_lines = new_content.splitlines()
    changes = []
    for i, (ol, nl) in enumerate(zip(old_lines, new_lines), 1):
        if ol != nl:
            changes.append((i, ol.strip(), nl.strip()))

    if not dry_run:
        with open(filepath, 'w', encoding='utf-8') as f:
            f.write(new_content)

    return changes


def walk_files(root):
    """Yield all processable file paths under root."""
    for dirpath, dirnames, filenames in os.walk(root):
        # Prune skipped directories
        dirnames[:] = [d for d in dirnames if d not in SKIP_DIRS]
        for fname in filenames:
            fpath = os.path.join(dirpath, fname)
            if should_process(fpath):
                yield fpath


def main():
    parser = argparse.ArgumentParser(description='Rename brand across codebase')
    parser.add_argument('--dry-run', action='store_true', help='Show changes without writing')
    parser.add_argument('--root', default='.', help='Root directory to process')
    args = parser.parse_args()

    root = os.path.abspath(args.root)
    total_files = 0
    total_changes = 0

    print(f"{'DRY RUN — ' if args.dry_run else ''}Scanning: {root}")
    print(f"Replacements: {REPLACEMENTS}")
    print()

    for fpath in sorted(walk_files(root)):
        rel = os.path.relpath(fpath, root)
        changes = process_file(fpath, dry_run=args.dry_run)
        if changes:
            total_files += 1
            total_changes += len(changes)
            print(f"  {rel} ({len(changes)} replacement{'s' if len(changes) != 1 else ''})")
            for line_num, old_line, new_line in changes:
                print(f"    L{line_num}: {old_line}")
                print(f"      → {new_line}")

    print()
    print(f"{'Would modify' if args.dry_run else 'Modified'}: {total_files} files, {total_changes} replacements")
    return 0 if total_files >= 0 else 1


if __name__ == '__main__':
    sys.exit(main())
