#!/bin/sh

set -e

GIT_TAG="$(git describe --tags --abbrev=0 --match 'v*' 2> /dev/null || echo v0.0.0)"
GIT_SHA="$( (git show-ref --head --hash=8 2> /dev/null || echo 00000000) | head -n1)"
GIT_DIRTY="$(git diff --no-ext-diff 2> /dev/null | wc -l)"
CURRENT_TIME="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

echo "-X github.com/petethepig/pyroscope/pkg/build.Version=$GIT_TAG"
echo "-X github.com/petethepig/pyroscope/pkg/build.GitSHA=$GIT_SHA"
echo "-X github.com/petethepig/pyroscope/pkg/build.GitDirtyStr=$(echo $GIT_DIRTY)"
echo "-X github.com/petethepig/pyroscope/pkg/build.Time=$CURRENT_TIME"
if [ "$1" = "true" ]; then
  echo "-X github.com/petethepig/pyroscope/pkg/build.UseEmbeddedAssetsStr=true"
fi
