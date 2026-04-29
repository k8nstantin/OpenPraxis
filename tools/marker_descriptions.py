#!/usr/bin/env python3
"""Strip 'marker' wording from MCP tool description strings + Go comments.

Targets the descriptions/comments that talked about an entity being identified
by either an 8/12-char marker or a full UUID. After marker elimination the
only valid identifier is the full UUID.

Usage: marker_descriptions.py <file>...
"""
import re
import sys
from pathlib import Path

# Order matters — longer patterns first so they don't get partially eaten.
REPLACEMENTS: list[tuple[str, str]] = [
    # Long forms inside string literals.
    (r"\(8-char marker or full UUID\)", "(full UUID)"),
    (r"\(12-char marker or full UUID\)", "(full UUID)"),
    (r"\(ID or 8-char marker\)", "(ID)"),
    (r"\(ID or 12-char marker\)", "(ID)"),
    (r"ID or 8-char marker", "full UUID"),
    (r"ID or 12-char marker", "full UUID"),
    (r"ID or short marker", "full UUID"),
    (r"ID or marker", "ID"),
    (r"IDs or markers", "IDs"),
    (r"IDs or 12-char markers", "IDs"),
    (r"manifest IDs or markers", "manifest IDs"),
    (r"Project ID or marker", "Project ID"),
    (r"Manifest ID or marker", "Manifest ID"),
    (r"Idea ID or marker", "Idea ID"),
    (r"Task ID or marker", "Task ID"),
    (r"Source manifest \(ID or marker\)", "Source manifest ID"),
    (r"Dep to remove \(ID or marker\)", "Dep ID"),
    (r"Manifest that will wait \(ID or 12-char marker\)", "Manifest that will wait"),
    (r"Manifest that must close first \(ID or marker\)", "Manifest that must close first"),
    (r"Entity ID or short marker", "Entity ID"),
    (r"\b8-char marker\b", "full UUID"),
    (r"\b12-char marker\b", "full UUID"),
    (r"\bshort marker\b", "full UUID"),
    # Comment cleanup.
    (r"// Resolve short markers to full IDs", "// Validate IDs"),
]


def transform(text: str) -> str:
    for pat, repl in REPLACEMENTS:
        text = re.sub(pat, repl, text)
    return text


def main(argv: list[str]) -> int:
    if len(argv) < 2:
        print("usage: marker_descriptions.py <file>...", file=sys.stderr)
        return 2
    for path in argv[1:]:
        p = Path(path)
        original = p.read_text()
        updated = transform(original)
        if updated != original:
            p.write_text(updated)
            print(f"updated {path}")
        else:
            print(f"unchanged {path}")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
