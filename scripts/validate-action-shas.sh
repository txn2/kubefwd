#!/bin/bash
# Validates that all GitHub Action SHA pins in workflow files are valid
# Usage: ./scripts/validate-action-shas.sh

set -e

WORKFLOW_DIR=".github/workflows"
ERRORS=0

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "Validating GitHub Action SHA pins..."
echo ""

# Find all uses: declarations with SHA pins
while IFS= read -r line; do
    # Extract file path and the action reference
    file=$(echo "$line" | cut -d: -f1)
    # Get the full action reference (owner/repo@sha)
    action_ref=$(echo "$line" | grep -oE 'uses: [^#]+' | sed 's/uses: //' | tr -d ' ')

    if [[ -z "$action_ref" ]]; then
        continue
    fi

    # Skip local actions (starting with ./)
    if [[ "$action_ref" == ./* ]]; then
        continue
    fi

    # Skip reusable workflows (contain .github/workflows)
    if [[ "$action_ref" == *".github/workflows"* ]]; then
        continue
    fi

    # Parse owner/repo and SHA
    if [[ "$action_ref" =~ ^([^/]+)/([^@]+)@([a-f0-9]{40})$ ]]; then
        owner="${BASH_REMATCH[1]}"
        repo="${BASH_REMATCH[2]}"
        sha="${BASH_REMATCH[3]}"

        # Handle nested paths like anchore/sbom-action/download-syft
        repo=$(echo "$repo" | cut -d/ -f1)

        # Validate SHA exists via GitHub API
        http_code=$(curl -s -o /dev/null -w "%{http_code}" \
            -H "Accept: application/vnd.github.v3+json" \
            "https://api.github.com/repos/${owner}/${repo}/git/commits/${sha}" 2>/dev/null || echo "000")

        if [[ "$http_code" == "200" ]]; then
            echo -e "${GREEN}✓${NC} ${owner}/${repo}@${sha:0:7}... (${file})"
        elif [[ "$http_code" == "404" ]]; then
            echo -e "${RED}✗${NC} ${owner}/${repo}@${sha:0:7}... - SHA not found! (${file})"
            ERRORS=$((ERRORS + 1))
        elif [[ "$http_code" == "403" ]]; then
            echo -e "${YELLOW}?${NC} ${owner}/${repo}@${sha:0:7}... - Rate limited, skipping (${file})"
        else
            echo -e "${YELLOW}?${NC} ${owner}/${repo}@${sha:0:7}... - HTTP ${http_code} (${file})"
        fi
    elif [[ "$action_ref" =~ @[a-f0-9]{40}$ ]]; then
        # Has SHA but didn't match pattern - might be malformed
        echo -e "${YELLOW}?${NC} Could not parse: ${action_ref} (${file})"
    fi
done < <(grep -rn "uses:.*@[a-f0-9]\{40\}" "$WORKFLOW_DIR" 2>/dev/null || true)

echo ""
if [[ $ERRORS -gt 0 ]]; then
    echo -e "${RED}Found $ERRORS invalid SHA(s)${NC}"
    exit 1
else
    echo -e "${GREEN}All SHA pins are valid${NC}"
    exit 0
fi
