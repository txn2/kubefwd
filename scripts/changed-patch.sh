#!/usr/bin/env bash
# Print a unified diff of every Go change on this branch.
#
# Usage:
#   scripts/changed-patch.sh [base-ref]    (default: origin/master)
#
# The diff runs from the merge base with <base-ref> to the working tree, so
# it covers committed, staged and unstaged changes, plus untracked .go files.
# `make lint`, `make patch-coverage` and `make semgrep-diff` all read it, so
# the three changed-line gates agree on what "changed" means, and they judge
# what is about to be committed, not only what already is.
#
# It fails instead of printing an empty diff when no base can be found. A gate
# that skips silently for lack of a base lets issues through to CI.
set -euo pipefail

BASE="${1:-origin/master}"

git fetch --quiet origin master 2>/dev/null || true

if git rev-parse --verify --quiet "$BASE" >/dev/null; then
  base="$BASE"
elif git rev-parse --verify --quiet master >/dev/null; then
  base=master
else
  echo "ERROR: neither $BASE nor master is reachable. Run 'git fetch origin master'." >&2
  echo "       A changed-line gate must run against a base; skipping would let issues through to CI." >&2
  exit 1
fi

if ! merge_base=$(git merge-base "$base" HEAD 2>/dev/null); then
  echo "ERROR: could not compute a merge base against $base." >&2
  exit 1
fi

echo "changed-patch: merge base ${merge_base:0:12} ($base), including uncommitted and untracked files" >&2

git diff "$merge_base" -- '*.go'

# git diff skips untracked files; diff each new .go file against /dev/null.
# --no-index exits 1 when the files differ, which here is always.
while IFS= read -r -d '' file; do
  git diff --no-index -- /dev/null "$file" || true
done < <(git ls-files --others --exclude-standard -z -- '*.go')
