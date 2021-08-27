#!/bin/sh

set -e

echo "GIT_TAG=\"$(git describe --tags --abbrev=0 --match 'v*' 2> /dev/null || echo v0.0.0)\""
echo "GIT_SHA=\"$( (git show-ref --head --hash=8 2> /dev/null || echo 00000000) | head -n1)\""
echo "GIT_DIRTY=\"$(echo $(git diff --no-ext-diff 2> /dev/null | wc -l))\""

# TODO: this is too hacky, need to find a better way to do this
echo "RBSPY_GIT_SHA=\"$(cat third_party/rustdeps/Cargo.toml | grep ^rbspy | cut -d '"' -f 4)\""
echo "PYSPY_GIT_SHA=\"$(cat third_party/rustdeps/Cargo.toml | grep ^py-spy | cut -d '"' -f 4)\""
echo "PHPSPY_GIT_SHA=\"$(cat Makefile | grep ^PHPSPY_VERSION | cut -d ' ' -f 3)\""
