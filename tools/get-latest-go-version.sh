#!/usr/bin/env bash

set -euo pipefail

# Check if GITHUB_TOKEN is provided
if [ -z "${GITHUB_TOKEN:-}" ]; then
    echo "Error: GITHUB_TOKEN environment variable is required" >&2
    exit 1
fi

# Extract current Go version from go.mod
CURRENT_VERSION=$(grep -E "^go [0-9]+\.[0-9]+\.[0-9]+" go.mod | awk '{print $2}')

if [ -z "$CURRENT_VERSION" ]; then
    echo "Error: Could not determine Go version from go.mod" >&2
    exit 1
fi

# Extract major and minor version
MAJOR_MINOR=$(echo "$CURRENT_VERSION" | cut -d. -f1-2)

echo "Current Go version in go.mod: $CURRENT_VERSION" >&2
echo "Fetching latest patch version for $MAJOR_MINOR.x..." >&2

# Fetch the latest patch version from golang/go repository
ALL_TAGS=$(curl -s -H "Authorization: Bearer $GITHUB_TOKEN" "https://api.github.com/repos/golang/go/git/refs/tags")
LATEST_VERSION=$(echo ${ALL_TAGS}|
                grep -o '"ref": "refs/tags/go[0-9]\+\.[0-9]\+\.[0-9]\+"' |
                grep "go$MAJOR_MINOR" |
                sort -V |
                tail -n 1 |
                grep -o '[0-9]\+\.[0-9]\+\.[0-9]\+')

if [ -z "$LATEST_VERSION" ]; then
    echo "Error: Could not determine latest patch version for $MAJOR_MINOR.x" >&2
    exit 1
fi

echo "Debug: Latest patch version for $MAJOR_MINOR.x: $LATEST_VERSION" >&2
echo "$LATEST_VERSION"
