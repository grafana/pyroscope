#!/usr/bin/env bash

set -euo pipefail

if [ $# -ne 1 ]; then
    echo "Usage: $0 <version>"
    exit 1
fi

# search replace all dockefiles
TARGET="*/*Dockerfile*"
git ls-files "$TARGET" | xargs sed -i 's/alpine:[0-9\.]\+/alpine:'$1'/g'

# add changes
git add -u "$TARGET"
git commit -m "Update alpine version to $1"
