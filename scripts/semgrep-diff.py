#!/usr/bin/env python3
"""Run a Semgrep config against the lines this branch changed.

`make semgrep` runs p/golang over the whole tree, because a finding there is a
defect wherever it sits. The rules in .semgrep-diff/ are different: they refuse
a shape that a diff-scoped CI check (CodeQL's security-and-quality suite)
rejects with reasoning Semgrep cannot reproduce, so they match far more widely
than they find defects. Scoping them to the diff is what Code Scanning does
with CodeQL, and what `make lint` does with golangci-lint.

The changed lines come from scripts/changed-patch.sh, the same patch
`make lint` and `make patch-coverage` read. Semgrep's own --baseline-commit
can't be used: it aborts on unstaged changes, and `make verify` is a
pre-commit gate that has to run on exactly that.

Usage: semgrep-diff.py <config-path>

The semgrep binary is taken from $SEMGREP (default: semgrep on PATH).
"""

import json
import os
import re
import subprocess
import sys
from collections import defaultdict

HUNK = re.compile(r"^@@ -\S+ \+(\d+)(?:,\d+)? @@")
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))


def changed_lines() -> dict[str, set[int]]:
    """Map each changed Go file to the line numbers this branch added to it.

    Only added lines count, matching golangci-lint --new-from-patch. Context
    lines in the patch advance the line counter but are not marked.
    """
    out = subprocess.run(
        [os.path.join(SCRIPT_DIR, "changed-patch.sh")],
        stdout=subprocess.PIPE, text=True, check=False,
    )
    if out.returncode != 0:
        sys.exit(1)

    added: dict[str, set[int]] = defaultdict(set)
    path = ""
    line_no = 0
    for line in out.stdout.splitlines():
        if line.startswith("+++ "):
            path = line[6:] if line.startswith("+++ b/") and line.endswith(".go") else ""
        elif m := HUNK.match(line):
            line_no = int(m.group(1))
        elif line.startswith("+"):
            if path:
                added[path].add(line_no)
            line_no += 1
        elif line.startswith(" "):
            line_no += 1
    return {p: lines for p, lines in added.items() if lines}


def findings(semgrep: str, config: str, files: list[str]) -> list[dict]:
    """Scan the changed files and return Semgrep's results."""
    out = subprocess.run(
        [semgrep, "scan", "--config", config, "--json", "--quiet", "--metrics=off", "--"] + files,
        capture_output=True, text=True, check=False,
    )
    if not out.stdout:
        print(f"ERROR: semgrep produced no output:\n{out.stderr}", file=sys.stderr)
        sys.exit(1)
    report = json.loads(out.stdout)
    for err in report.get("errors", []):
        print(f"semgrep: {err.get('message', err)}", file=sys.stderr)
    return report.get("results", [])


def main() -> None:
    if len(sys.argv) != 2:
        print("Usage: semgrep-diff.py <config-path>", file=sys.stderr)
        sys.exit(2)
    config = sys.argv[1]
    semgrep = os.environ.get("SEMGREP", "semgrep")

    changed = changed_lines()
    # A file the diff added and the working tree has since removed is not on
    # disk; semgrep errors on a path it cannot open.
    files = sorted(p for p in changed if os.path.exists(p))
    if not files:
        print("semgrep-diff: no changed Go lines; nothing to scan.")
        return

    hits = []
    for r in findings(semgrep, config, files):
        span = range(r["start"]["line"], r["end"]["line"] + 1)
        if any(line in changed.get(r["path"], ()) for line in span):
            hits.append(r)

    if not hits:
        print(f"semgrep-diff: scanned {len(files)} changed Go file(s) against {config}: no findings.")
        return

    for r in hits:
        print(
            f"{r['path']}:{r['start']['line']}: [{r['check_id'].split('.')[-1]}] "
            f"{' '.join(r['extra']['message'].split())}"
        )
    print(f"\n{len(hits)} finding(s) on changed lines. See {config}.", file=sys.stderr)
    sys.exit(1)


if __name__ == "__main__":
    main()
