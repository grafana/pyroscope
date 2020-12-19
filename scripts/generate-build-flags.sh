#!/bin/sh

set -e

git_stats() {
  (
    cd $1
    GIT_TAG="$(git describe --tags --abbrev=0 --match 'v*' 2> /dev/null || echo v0.0.0)"
    GIT_SHA="$( (git show-ref --head --hash=8 2> /dev/null || echo 00000000) | head -n1)"
    GIT_DIRTY="$(git diff --no-ext-diff 2> /dev/null | wc -l)"
    echo "-X github.com/pyroscope-io/pyroscope/pkg/build.$2Version=$GIT_TAG"
    echo "-X github.com/pyroscope-io/pyroscope/pkg/build.$2GitSHA=$GIT_SHA"
    echo "-X github.com/pyroscope-io/pyroscope/pkg/build.$2GitDirtyStr=$(echo $GIT_DIRTY)"
  )
}

CURRENT_TIME="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

git_stats "." ""
# git_stats "third_party/rbspy" "RbSpy"
# git_stats "third_party/pyspy" "PySpy"
echo "-X github.com/pyroscope-io/pyroscope/pkg/build.Time=$CURRENT_TIME"
if [ "$1" = "true" ]; then
  echo "-X github.com/pyroscope-io/pyroscope/pkg/build.UseEmbeddedAssetsStr=true"
fi
