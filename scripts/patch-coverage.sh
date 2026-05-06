#!/usr/bin/env bash
# Approximates codecov's patch-coverage check locally.
#
# Usage:
#   scripts/patch-coverage.sh <base-ref> <coverage-profile> <target-pct> <strict>
#
# Reads `git diff <base-ref>...HEAD` to find added Go lines, intersects
# them with the Go coverage profile, and prints a per-file + total
# patch coverage report. With strict=1, exits non-zero if total patch
# coverage is below the target (matches codecov's blocking mode);
# otherwise informational only.
set -euo pipefail

BASE="${1:?base ref required}"
PROFILE="${2:?coverage profile required}"
TARGET="${3:?target percentage required}"
STRICT="${4:-0}"

if ! git rev-parse --verify "$BASE" >/dev/null 2>&1; then
  echo "patch-coverage: base ref '$BASE' not found locally; skipping" >&2
  exit 0
fi

if [ ! -f "$PROFILE" ]; then
  echo "patch-coverage: profile '$PROFILE' missing; run 'make test' first" >&2
  exit 1
fi

MODULE="$(awk '/^module /{print $2; exit}' go.mod)"

# Use awk to do the entire correlation in one pass — no Python required.
git diff --unified=0 "$BASE"...HEAD -- '*.go' ':!*_test.go' \
  | awk -v module="$MODULE" -v profile="$PROFILE" -v target="$TARGET" -v strict="$STRICT" '
      # Pass 1: read coverage profile into a hits[file:line] map.
      BEGIN {
        while ((getline line < profile) > 0) {
          if (line ~ /^mode:/) continue
          # Format: <file>:<startLine>.<col>,<endLine>.<col> <stmts> <count>
          n = split(line, parts, " ")
          if (n != 3) continue
          loc = parts[1]; count = parts[3] + 0
          # Strip module prefix so it matches diff filenames.
          sub("^" module "/", "", loc)
          # Parse "file:start.col,end.col"
          colon = index(loc, ":")
          file = substr(loc, 1, colon - 1)
          rng  = substr(loc, colon + 1)
          comma = index(rng, ",")
          startLine = substr(rng, 1, index(rng, ".") - 1) + 0
          endPart = substr(rng, comma + 1)
          endLine = substr(endPart, 1, index(endPart, ".") - 1) + 0
          for (l = startLine; l <= endLine; l++) {
            key = file ":" l
            if (count > 0) hits[key] = 1
            else if (!(key in hits)) hits[key] = 0
          }
        }
        close(profile)
      }

      # Pass 2: walk the unified diff and mark added lines.
      /^\+\+\+ b\// {
        sub(/^\+\+\+ b\//, "")
        cur = $0
        next
      }
      /^@@/ {
        # @@ -a,b +c,d @@  — extract c (start of added hunk).
        match($0, /\+[0-9]+/)
        line = substr($0, RSTART + 1, RLENGTH - 1) + 0
        next
      }
      /^\+[^+]/ && cur != "" {
        # Added line in a Go non-test file. Skip blank/comment-only —
        # they have no coverage entries anyway (awk fall-through).
        key = cur ":" line
        added[cur ":" line] = 1
        line++
        next
      }
      /^[ ]/ { line++; next }
      /^-/   { next }

      END {
        total = 0; covered = 0
        for (k in added) {
          if (k in hits) {
            total++
            if (hits[k] == 1) covered++
          }
        }
        if (total == 0) {
          print "patch-coverage: no executable Go statements changed (informational pass)"
          exit 0
        }
        pct = (covered * 100) / total
        printf "patch-coverage: %d/%d statements covered = %.2f%% (target %s%%)\n", \
          covered, total, pct, target
        if (pct + 0 < target + 0) {
          if (strict == "1") {
            print "patch-coverage: BELOW TARGET (strict mode)" > "/dev/stderr"
            exit 1
          }
          print "patch-coverage: BELOW TARGET (informational; set STRICT=1 to enforce)"
        }
      }
    '
