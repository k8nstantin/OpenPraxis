#!/usr/bin/env python3
"""Analyze inline styles in JS files to find repeated patterns for CSS extraction."""

import re
import os
from collections import Counter

UI_DIR = os.path.join(os.path.dirname(__file__), '..', 'internal', 'web', 'ui')

def find_inline_styles(directory):
    """Find all inline style= attributes in JS files."""
    styles = []
    for root, _, files in os.walk(directory):
        for f in files:
            if not f.endswith('.js'):
                continue
            path = os.path.join(root, f)
            with open(path) as fh:
                for i, line in enumerate(fh, 1):
                    # Find all style="..." in the line
                    for m in re.finditer(r'style="([^"]*)"', line):
                        styles.append({
                            'file': os.path.relpath(path, directory),
                            'line': i,
                            'style': m.group(1),
                        })
    return styles

def main():
    styles = find_inline_styles(UI_DIR)
    print(f'Total inline style attributes found: {len(styles)}\n')

    # Count exact duplicates
    counter = Counter(s['style'] for s in styles)

    print('=== REPEATED INLINE STYLES (3+ occurrences) ===\n')
    for style, count in counter.most_common():
        if count < 3:
            break
        print(f'  [{count}x] style="{style}"')
        # Show where they appear
        locations = [s for s in styles if s['style'] == style]
        for loc in locations[:3]:
            print(f'        {loc["file"]}:{loc["line"]}')
        if len(locations) > 3:
            print(f'        ... and {len(locations) - 3} more')
        print()

    # Group by common property patterns
    print('\n=== COMMON PROPERTY PATTERNS ===\n')
    prop_counter = Counter()
    for s in styles:
        # Split into individual properties
        props = [p.strip() for p in s['style'].split(';') if p.strip()]
        for prop in props:
            prop_counter[prop] += 1

    print('Top 30 most common individual properties:')
    for prop, count in prop_counter.most_common(30):
        print(f'  [{count:3d}x] {prop}')

if __name__ == '__main__':
    main()
