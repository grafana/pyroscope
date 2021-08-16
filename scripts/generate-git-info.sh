#!/bin/sh

set -e

echo "GIT_TAG=\"$(git describe --tags --abbrev=0 --match 'v*' 2> /dev/null || echo v0.0.0)\""
echo "GIT_SHA=\"$( (git show-ref --head --hash=8 2> /dev/null || echo 00000000) | head -n1)\""
echo "GIT_DIRTY=\"$(echo $(git diff --no-ext-diff 2> /dev/null | wc -l))\""
echo "RBSPY_GIT_DIRTY=\"$(cat third_party/rustdeps/Cargo.toml | grep ^rbspy | sed 's/.\+\?rev.\+\?"\(.\+\?\)".\+/\1/')\""
echo "PYSPY_GIT_DIRTY=\"$(cat third_party/rustdeps/Cargo.toml | grep ^py-spy | sed 's/.\+\?rev.\+\?"\(.\+\?\)".\+/\1/')\""
echo "PHPSPY_GIT_DIRTY=\"$(cat Makefile | grep 'PHPSPY_VERSION.\+\?=\s\+\?' | sed 's/PHPSPY_VERSION.\+\?=\s\+\?//')\""
