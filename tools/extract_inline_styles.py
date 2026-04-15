#!/usr/bin/env python3
"""
Extract repeated inline styles into CSS utility classes.

Reads JS files in internal/web/ui/, replaces exact style="..." patterns
with corresponding CSS classes, handles merging with existing class attrs.

Reusable: add new entries to REPLACEMENTS to extend.
"""

import os
import re
import sys

UI_DIR = os.path.join(os.path.dirname(__file__), '..', 'internal', 'web', 'ui')

# Map: exact style value -> CSS class name
REPLACEMENTS = [
    # Form labels (13x + 6x)
    ('font-size:12px;color:var(--text-muted);display:block;margin-bottom:6px;font-weight:500', 'form-label'),
    ('font-size:12px;color:var(--text-muted);display:block;margin-bottom:4px', 'form-label-compact'),

    # Breadcrumb navigation (5x + 10x + 10x)
    ('font-size:11px;color:var(--text-muted);margin-bottom:8px;font-family:var(--font-mono)', 'breadcrumb'),
    ('cursor:pointer;color:var(--accent)', 'breadcrumb-link'),
    ('opacity:0.4', 'breadcrumb-sep'),

    # Stats bar — long compound style (match with flexible spacing)
    ('display:flex;gap:12px;font-size:12px;color:var(--text-muted);margin-bottom:12px;align-items:center;flex-wrap:wrap;padding:8px 12px;background:var(--bg-secondary);border:1px solid var(--border);border-radius:6px;font-family:var(--font-mono)', 'stats-bar'),
    # Variant without margin-bottom
    ('display:flex;gap:12px;font-size:12px;color:var(--text-muted);align-items:center;flex-wrap:wrap;padding:8px 12px;background:var(--bg-secondary);border:1px solid var(--border);border-radius:6px;font-family:var(--font-mono)', 'stats-bar'),

    # Table headers (4x + 8x + 6x + 6x)
    ('text-align:left;padding:8px 12px;font-size:11px;color:var(--text-muted);font-weight:500', 'th-left'),
    ('text-align:right;padding:8px 12px;font-size:11px;color:var(--text-muted);font-weight:500', 'th-right'),
    ('text-align:left;padding:6px 12px;font-size:10px;color:var(--text-muted);font-weight:500', 'th-left-sm'),
    ('text-align:right;padding:6px 12px;font-size:10px;color:var(--text-muted);font-weight:500', 'th-right-sm'),

    # Table data cells
    ('padding:6px 12px;font-family:var(--font-mono);font-size:12px', 'td-mono'),
    ('padding:6px 12px;text-align:right;font-family:var(--font-mono);font-size:12px', 'td-mono-right'),
    ('padding:5px 12px;text-align:right;font-family:var(--font-mono);font-size:12px', 'td-mono-right-sm'),
    ('padding:8px 12px;text-align:right;font-family:var(--font-mono);font-size:12px;font-weight:600', 'td-mono-right-bold'),

    # Section structure (4x + 4x + 3x + 3x)
    ('margin-top:16px;padding-top:12px;border-top:1px solid var(--border)', 'section-divider'),
    ('font-size:13px;color:var(--text-primary);margin-bottom:8px;font-weight:600', 'section-title'),
    ('font-size:12px;color:var(--text-muted);margin-bottom:6px;font-weight:500', 'section-label'),
    ('font-size:12px;font-weight:600;color:var(--text-primary)', 'heading-sm'),

    # Meta/timestamps (8x + 3x)
    ('color:var(--text-muted);font-size:11px;margin-left:auto', 'meta-time'),
    ('font-weight:400;font-size:12px;color:var(--text-muted)', 'sub-count'),

    # Badge/dot sizes (18x + 3x)
    ('font-size:10px', 'badge-sm'),
    ('width:6px;height:6px', 'dot-sm'),

    # Separators (6x)
    ('opacity:0.3', 'separator'),

    # Common containers (3x + 3x + 3x + 3x)
    ('font-size:12px;color:var(--text-muted);padding:12px;border:1px dashed var(--border);border-radius:4px;text-align:center', 'empty-placeholder'),
    ('border:1px solid var(--border);border-radius:4px;overflow:hidden', 'bordered-container'),
    ('margin-left:24px;cursor:pointer', 'tree-indent clickable'),
    ('max-height:400px;overflow-y:auto', 'scroll-detail'),

    # Button sizes
    ('font-size:11px;padding:2px 10px', 'btn-xs'),
    ('font-size:11px;padding:4px 10px', 'btn-sm'),
    ('font-size:12px;padding:6px 14px', 'btn-md'),
    ('font-size:12px;padding:6px 16px', 'btn-action'),

    # Watcher gate titles (3x)
    ('font-size:13px;font-weight:600', 'gate-title'),

    # Layout helpers
    ('display:flex;align-items:center;gap:8px', 'flex-row'),
    ('display:flex;gap:8px', 'flex-gap'),
]


def replace_style_with_class(content, style_value, class_name):
    """
    Replace exact style="<style_value>" with CSS class, merging with
    existing class attributes where present.
    """
    escaped = re.escape(style_value)
    count = 0

    # Pattern 1: class="..." followed by style="target" (possibly with other attrs between)
    # We look for class="..." and style="target" on the same HTML tag
    def merge_class_before_style(m):
        nonlocal count
        count += 1
        existing = m.group(1)
        between = m.group(2)
        # Deduplicate: don't add classes already present
        new_classes = [c for c in class_name.split() if c not in existing.split()]
        merged = existing + (' ' + ' '.join(new_classes) if new_classes else '')
        return f'class="{merged}"{between}'

    pattern1 = r'class="([^"]*)"([^>]*?)\s*style="' + escaped + '"'
    content = re.sub(pattern1, merge_class_before_style, content)

    # Pattern 2: style="target" followed by class="..."
    def merge_style_before_class(m):
        nonlocal count
        count += 1
        between = m.group(1)
        existing = m.group(2)
        return f'{between}class="{class_name} {existing}"'

    pattern2 = r'style="' + escaped + r'"([^>]*?)\s*class="([^"]*)"'
    content = re.sub(pattern2, merge_style_before_class, content)

    # Pattern 3: style="target" with no class attr nearby (standalone replacement)
    def standalone_replace(m):
        nonlocal count
        count += 1
        return f'class="{class_name}"'

    pattern3 = r'style="' + escaped + '"'
    content = re.sub(pattern3, standalone_replace, content)

    return content, count


def process_file(filepath, dry_run=False):
    """Process a single JS file, applying all replacements."""
    with open(filepath) as f:
        original = f.read()

    content = original
    total = 0

    for style_value, class_name in REPLACEMENTS:
        content, count = replace_style_with_class(content, style_value, class_name)
        if count > 0:
            total += count

    if content != original:
        relpath = os.path.relpath(filepath, UI_DIR)
        print(f'  {relpath}: {total} replacements')
        if not dry_run:
            with open(filepath, 'w') as f:
                f.write(content)
    return total


def main():
    dry_run = '--dry-run' in sys.argv
    if dry_run:
        print('DRY RUN — no files will be modified\n')

    total = 0
    for root, _, files in os.walk(UI_DIR):
        for f in sorted(files):
            if not f.endswith('.js'):
                continue
            filepath = os.path.join(root, f)
            total += process_file(filepath, dry_run)

    print(f'\nTotal replacements: {total}')

    if dry_run:
        print('\nRun without --dry-run to apply changes.')


if __name__ == '__main__':
    main()
