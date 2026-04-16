#!/usr/bin/env python3
"""Scan git history for leaked secrets.

Walks the full object history (`git log --all -p`), matches against a
curated set of secret patterns, then classifies each hit as HIGH (looks
like a real secret value) or LOW (reference/placeholder/variable name).

Usage:
    python3 tools/scan_secrets.py [--repo PATH] [--json OUT] [--high-only]

Exit codes:
    0 — no HIGH-severity hits
    1 — at least one HIGH-severity hit (action required)
"""

from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
from dataclasses import dataclass, field, asdict
from pathlib import Path


# Patterns: (name, regex, default_severity)
# default_severity may be downgraded by the classifier below.
PATTERNS: list[tuple[str, re.Pattern, str]] = [
    # Provider-specific shapes — very high confidence.
    ("anthropic_key",   re.compile(r"sk-ant-[A-Za-z0-9_\-]{20,}"),        "HIGH"),
    ("openai_key",      re.compile(r"sk-[A-Za-z0-9]{20,}"),                "HIGH"),
    ("openai_proj_key", re.compile(r"sk-proj-[A-Za-z0-9_\-]{20,}"),        "HIGH"),
    ("google_api_key",  re.compile(r"AIza[0-9A-Za-z_\-]{35}"),             "HIGH"),
    ("github_pat",      re.compile(r"gh[pousr]_[A-Za-z0-9]{30,}"),         "HIGH"),
    ("aws_access_key",  re.compile(r"AKIA[0-9A-Z]{16}"),                   "HIGH"),
    ("slack_token",     re.compile(r"xox[baprs]-[A-Za-z0-9\-]{10,}"),      "HIGH"),
    ("private_key_pem", re.compile(r"-----BEGIN (?:RSA |EC |DSA |OPENSSH |PGP )?PRIVATE KEY-----"), "HIGH"),
    ("jwt",             re.compile(r"eyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}"), "HIGH"),

    # Generic keyword-value shapes — start LOW, classifier promotes if it
    # looks like a real value.
    ("api_key",         re.compile(r"(?i)api[_\-]?key\s*[:=]\s*[\"']?([^\s\"'`]+)"),        "LOW"),
    ("password",        re.compile(r"(?i)password\s*[:=]\s*[\"']?([^\s\"'`]+)"),            "LOW"),
    ("secret",          re.compile(r"(?i)secret\s*[:=]\s*[\"']?([^\s\"'`]+)"),              "LOW"),
    ("token",           re.compile(r"(?i)(?<![a-z_])token\s*[:=]\s*[\"']?([^\s\"'`]+)"),    "LOW"),
    ("bearer",          re.compile(r"(?i)bearer\s+([A-Za-z0-9_\-\.=]{20,})"),               "LOW"),
    ("authorization",   re.compile(r"(?i)authorization[:=]\s*[\"']?(bearer|basic)\s+([A-Za-z0-9_\-\.=]{10,})"), "LOW"),
]

# Placeholders and obvious non-secrets.
PLACEHOLDER_RE = re.compile(
    r"(?i)(your[_\-]?|my[_\-]?|example|placeholder|dummy|fake|xxx+|changeme|todo|fixme|<[^>]+>|\{\{.+?\}\}|\$\{.+?\}|sk-\.\.\.|\.\.\.|_HERE$|_REDACTED)"
)

# Lines that look like docs / env var names / schema rather than values.
DOC_HINT_RE = re.compile(
    r"(?i)(env\.example|readme|\.md|# |//|/\*|description|docstring|\"name\":|\"type\":)"
)


@dataclass
class Hit:
    commit: str
    author: str
    date: str
    file: str
    line_no: int
    pattern: str
    severity: str
    snippet: str
    reason: str = ""

    def to_row(self) -> str:
        return (
            f"[{self.severity}] {self.commit[:10]} {self.pattern:16} "
            f"{self.file}:{self.line_no}  {self.snippet[:120]}"
            + (f"  ({self.reason})" if self.reason else "")
        )


def run_git(args: list[str], cwd: Path) -> str:
    out = subprocess.run(
        ["git", *args], cwd=cwd, check=True, text=True, capture_output=True,
    )
    return out.stdout


def iter_commits(repo: Path):
    """Yield (commit_sha, author, iso_date) for every commit across all refs."""
    fmt = "%H%x1f%an <%ae>%x1f%aI"
    raw = run_git(["log", "--all", "--no-merges", f"--pretty=format:{fmt}"], repo)
    for line in raw.splitlines():
        sha, author, date = line.split("\x1f")
        yield sha, author, date


def commit_patch(repo: Path, sha: str) -> str:
    try:
        return run_git(
            ["show", "--no-color", "--pretty=format:", "--unified=0", sha],
            repo,
        )
    except subprocess.CalledProcessError:
        return ""


def classify(pattern: str, default_sev: str, line: str, file: str, match: re.Match) -> tuple[str, str]:
    """Return (severity, reason) for a match."""
    if default_sev == "HIGH":
        return "HIGH", ""

    value = match.group(match.lastindex) if match.lastindex else match.group(0)

    if PLACEHOLDER_RE.search(value):
        return "LOW", "placeholder-value"

    stripped = value.strip("\"' ")
    # os.Getenv("API_KEY") — env var name lookup, not a value.
    if re.fullmatch(r"[A-Z][A-Z0-9_]+", stripped):
        return "LOW", "env-var-name"

    # Short values are almost always example/docs.
    if len(stripped) < 16:
        return "LOW", "short-value"

    # Doc / comment lines.
    if DOC_HINT_RE.search(line) and not re.search(r"[a-f0-9]{32,}|[A-Za-z0-9+/]{40,}", stripped):
        return "LOW", "doc-context"

    # File-path heuristics.
    lower_file = file.lower()
    if any(seg in lower_file for seg in (
        ".md", "/docs/", "example", "readme", "claude.md",
        "_test.go", "test_", ".test.",
    )):
        return "LOW", "docs-or-tests"

    # Kebab-case / snake_case English-word identifiers — not secrets.
    # e.g. "local-notifications-enabled", "user_profile_cache_key"
    if re.fullmatch(r"[a-z]+([-_][a-z]+)+", stripped):
        return "LOW", "kebab-or-snake-identifier"

    # Anything still standing with entropy-worthy length is HIGH.
    if re.search(r"[A-Za-z0-9+/=_\-]{24,}", stripped):
        return "HIGH", "entropy-looks-real"

    return "LOW", "low-entropy"


def scan(repo: Path) -> list[Hit]:
    hits: list[Hit] = []
    commits = list(iter_commits(repo))
    total = len(commits)
    print(f"Scanning {total} commits across all refs in {repo}", file=sys.stderr)

    for i, (sha, author, date) in enumerate(commits, 1):
        if i % 25 == 0 or i == total:
            print(f"  {i}/{total}", file=sys.stderr)
        patch = commit_patch(repo, sha)
        if not patch:
            continue

        current_file = "?"
        line_no = 0
        for raw_line in patch.splitlines():
            if raw_line.startswith("diff --git "):
                # "diff --git a/path b/path"
                m = re.match(r"diff --git a/(.+?) b/(.+)$", raw_line)
                if m:
                    current_file = m.group(2)
                line_no = 0
                continue
            if raw_line.startswith("@@"):
                m = re.match(r"@@ -\d+(?:,\d+)? \+(\d+)", raw_line)
                if m:
                    line_no = int(m.group(1)) - 1
                continue
            if not raw_line.startswith("+") or raw_line.startswith("+++"):
                continue
            line_no += 1
            content = raw_line[1:]

            for name, pat, default_sev in PATTERNS:
                for match in pat.finditer(content):
                    sev, reason = classify(name, default_sev, content, current_file, match)
                    hits.append(Hit(
                        commit=sha, author=author, date=date,
                        file=current_file, line_no=line_no,
                        pattern=name, severity=sev,
                        snippet=content.strip(),
                        reason=reason,
                    ))
    return hits


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--repo", default=".", help="Path to git repo (default: cwd)")
    ap.add_argument("--json", help="Write full hit list as JSON to this path")
    ap.add_argument("--high-only", action="store_true", help="Only print HIGH hits")
    args = ap.parse_args()

    repo = Path(args.repo).resolve()
    hits = scan(repo)

    highs = [h for h in hits if h.severity == "HIGH"]
    lows = [h for h in hits if h.severity == "LOW"]

    print("")
    print(f"=== Secret scan summary ===")
    print(f"  total matches: {len(hits)}")
    print(f"  HIGH:          {len(highs)}")
    print(f"  LOW (review):  {len(lows)}")
    print("")

    if highs:
        print("--- HIGH severity hits ---")
        for h in highs:
            print(h.to_row())
    else:
        print("No HIGH-severity hits.")

    if not args.high_only and lows:
        print("")
        print("--- LOW severity hits (triage) ---")
        # Group by pattern to keep output legible.
        by_pattern: dict[str, list[Hit]] = {}
        for h in lows:
            by_pattern.setdefault(h.pattern, []).append(h)
        for pat, items in sorted(by_pattern.items()):
            print(f"  {pat}: {len(items)} hits")
            for h in items[:5]:
                print(f"    {h.to_row()}")
            if len(items) > 5:
                print(f"    ... {len(items) - 5} more")

    if args.json:
        Path(args.json).write_text(json.dumps([asdict(h) for h in hits], indent=2))
        print(f"\nFull results written to {args.json}")

    return 1 if highs else 0


if __name__ == "__main__":
    sys.exit(main())
