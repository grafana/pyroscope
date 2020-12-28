#!/bin/sh

set -e

# we don't copy .git to docker context, so in docker context we use git-info
if [ -d ".git" ]
then
  scripts/generate-git-info.sh > scripts/packages/git-info
fi
source scripts/packages/git-info

CURRENT_TIME="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

echo "-X github.com/pyroscope-io/pyroscope/pkg/build.Version=$GIT_TAG"
echo "-X github.com/pyroscope-io/pyroscope/pkg/build.GitSHA=$GIT_SHA"
echo "-X github.com/pyroscope-io/pyroscope/pkg/build.GitDirtyStr=$GIT_DIRTY"
echo "-X github.com/pyroscope-io/pyroscope/pkg/build.Time=$CURRENT_TIME"
if [ "$1" = "true" ]; then
  echo "-X github.com/pyroscope-io/pyroscope/pkg/build.UseEmbeddedAssetsStr=true"
fi
