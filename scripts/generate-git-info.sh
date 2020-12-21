#!/bin/sh

set -e

echo "GIT_TAG=\"$(git describe --tags --abbrev=0 --match 'v*' 2> /dev/null || echo v0.0.0)\""
echo "GIT_SHA=\"$( (git show-ref --head --hash=8 2> /dev/null || echo 00000000) | head -n1)\""
echo "GIT_DIRTY=\"$(echo $(git diff --no-ext-diff 2> /dev/null | wc -l))\""
