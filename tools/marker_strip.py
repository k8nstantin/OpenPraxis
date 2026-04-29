#!/usr/bin/env python3
"""Strip marker concept from Go source files.

Usage: marker_strip.py <file>...

Transformations applied per file:
- `rt.Marker` -> `rt.TaskID` (RunningTask.Marker is gone, but TaskID is full UUID)
- `t.Marker`  -> `t.ID`      (Task.Marker is gone)
- `m.Marker`  -> `m.ID`
- `p.Marker`  -> `p.ID`
- `i.Marker`  -> `i.ID`
- `rs.Marker` -> `rs.TaskID`
- log-line `"marker"` key -> `"task_id"` (struct slog kv pairs)
- `Marker: id[:12]` struct-field literal lines deleted (they assign to a removed field)
- `Marker: <expr>,` struct-field literal lines deleted (multiline AND inline)
- `Marker         string` / `Marker string` field declarations deleted
- `taskMarker(<expr>)` helper call -> `<expr>`  (always returned the full ID anyway)

NB: this tool intentionally does NOT touch the *notification* marker system
(internal/marker/, marker_flag/marker_done/marker_list MCP tools, n.Markers).
Run it only on files where Marker means "12-char id prefix".
"""
import re
import sys
from pathlib import Path


def transform(text: str) -> str:
    # Drop multi-line struct-field literal lines like `\t\tMarker: foo,`.
    # Require start-of-line then optional whitespace then the bare word
    # `Marker:` so that `ManifestMarker:` / `RuleMarker:` aren't matched.
    text = re.sub(r"\n[\t ]*Marker:[^\n]*,?\n", "\n", text)
    # Drop inline `Marker: <expr>,` inside struct literals. Use a leading
    # non-word char to avoid eating the suffix of `ManifestMarker:`.
    text = re.sub(r"(?<![A-Za-z0-9_])Marker:\s*[^,}\n]+?,\s*", "", text)
    # Drop field declaration lines like `\tMarker  string  \`json:"marker"\``
    # Allow any whitespace and any json tag content. Word-boundary on Marker.
    text = re.sub(r"\n[\t ]*Marker\s+string\s+`json:\"marker\"`\n", "\n", text)
    # Drop bare `Marker string` field decls (no json tag).
    text = re.sub(r"\n[\t ]*Marker\s+string\b[^\n]*\n", "\n", text)
    # Member access on receivers we know
    text = re.sub(r"\b(rt|rs)\.Marker\b", lambda m: f"{m.group(1)}.TaskID", text)
    text = re.sub(r"\b(t|m|p|i|child|parent|node|d|s|a)\.Marker\b", lambda m: f"{m.group(1)}.ID", text)
    # log slog kv: "marker" -> "task_id"
    text = re.sub(r'"marker"', '"task_id"', text)
    # taskMarker(x) helper call -> x
    text = re.sub(r"taskMarker\(([^)]+)\)", r"\1", text)
    return text


def main(argv: list[str]) -> int:
    if len(argv) < 2:
        print("usage: marker_strip.py <file>...", file=sys.stderr)
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
