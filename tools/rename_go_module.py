#!/usr/bin/env python3
"""
Rename a Go module path across the codebase.

Usage:
    python3 tools/rename_go_module.py --old OLD --new NEW [--dry-run] [--root DIR]

Rewrites:
  - go.mod's `module OLD` → `module NEW`
  - All Go import paths "OLD"          → "NEW"
  - All Go import paths "OLD/<pkg>..." → "NEW/<pkg>..."

Safety: only rewrites strings that match a valid Go import path (package
segments must start with a letter or underscore). This avoids hitting
`fmt.Sprintf("openpraxis/%s", …)` and other non-import uses.

Reusable — works for any module-path rename, not hard-coded to OpenPraxis.
"""

import argparse
import os
import re
import sys


SKIP_DIRS = {'.git', 'node_modules', 'vendor', '.expo'}
SKIP_FILES = {'rename_go_module.py'}


def build_import_regex(old_module: str) -> re.Pattern:
    """Match `"OLD/<go-identifier-path>"` as a full quoted import.

    A bare `"OLD"` alone is NOT matched — in Go, you cannot import a module's
    root (it's typically package `main`), and matching it would clobber
    unrelated string literals like command names or map keys.
    """
    escaped = re.escape(old_module)
    segment = r'[A-Za-z_][A-Za-z0-9_]*'
    # Require at least one /segment after the module name.
    path = rf'(?:/{segment})+'
    return re.compile(rf'"{escaped}({path})"')


def rewrite_go_file(path: str, old: str, new: str, regex: re.Pattern) -> tuple[str, int]:
    with open(path, 'r', encoding='utf-8') as f:
        content = f.read()
    new_content, count = regex.subn(lambda m: f'"{new}{m.group(1)}"', content)
    return new_content, count


def rewrite_go_mod(path: str, old: str, new: str) -> tuple[str, int]:
    with open(path, 'r', encoding='utf-8') as f:
        content = f.read()
    # Only match the `module` directive line. Use [^\S\n]* (whitespace minus
    # newline) so the trailing blank line after the directive is preserved.
    pattern = re.compile(rf'^module[^\S\n]+{re.escape(old)}[^\S\n]*$', re.MULTILINE)
    new_content, count = pattern.subn(f'module {new}', content)
    return new_content, count


def walk_files(root: str):
    for dirpath, dirnames, filenames in os.walk(root):
        dirnames[:] = [d for d in dirnames if d not in SKIP_DIRS]
        for name in filenames:
            if name in SKIP_FILES:
                continue
            yield os.path.join(dirpath, name)


def main():
    parser = argparse.ArgumentParser(description=__doc__.strip().splitlines()[0])
    parser.add_argument('--old', required=True, help='Current module path (e.g. openpraxis)')
    parser.add_argument('--new', required=True, help='New module path (e.g. github.com/org/Repo)')
    parser.add_argument('--root', default='.', help='Repo root (default: cwd)')
    parser.add_argument('--dry-run', action='store_true', help='Report changes without writing')
    args = parser.parse_args()

    regex = build_import_regex(args.old)
    total_imports = 0
    total_files = 0
    modfile_changes = 0

    for path in walk_files(args.root):
        basename = os.path.basename(path)
        if basename == 'go.mod':
            new_content, n = rewrite_go_mod(path, args.old, args.new)
            if n:
                modfile_changes += n
                print(f'[mod] {path}: {n} module directive(s) rewritten')
                if not args.dry_run:
                    with open(path, 'w', encoding='utf-8') as f:
                        f.write(new_content)
            continue

        if not path.endswith('.go'):
            continue
        try:
            new_content, n = rewrite_go_file(path, args.old, args.new, regex)
        except (OSError, UnicodeDecodeError) as err:
            print(f'[skip] {path}: {err}', file=sys.stderr)
            continue
        if n:
            total_imports += n
            total_files += 1
            print(f'[go]  {path}: {n} import(s) rewritten')
            if not args.dry_run:
                with open(path, 'w', encoding='utf-8') as f:
                    f.write(new_content)

    suffix = ' (dry run)' if args.dry_run else ''
    print(f'\nSummary{suffix}: {total_imports} import rewrites across {total_files} .go files, '
          f'{modfile_changes} go.mod directive(s).')


if __name__ == '__main__':
    main()
