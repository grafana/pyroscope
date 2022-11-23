#!/bin/bash

set -e

CURRENT_TIME="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
if [ -z $NO_BUILD_TIME_TAG ]
then
  echo "-X github.com/pyroscope-io/pyroscope/pkg/build.Time=$CURRENT_TIME"
else
  echo "-X github.com/pyroscope-io/pyroscope/pkg/build.Time=NO_BUILD_TIME_TAG"
fi

# we don't copy .git to docker context, so in docker context we use git-info
if [ -d ".git" ]
then
  scripts/generate-git-info.sh > scripts/packages/git-info
fi
source scripts/packages/git-info

echo "-X github.com/pyroscope-io/pyroscope/pkg/build.Version=$GIT_TAG"
echo "-X github.com/pyroscope-io/pyroscope/pkg/build.GitSHA=$GIT_SHA"
echo "-X github.com/pyroscope-io/pyroscope/pkg/build.GitDirtyStr=$GIT_DIRTY"
echo "-X github.com/pyroscope-io/pyroscope/pkg/build.RbspyGitSHA=$RBSPY_GIT_SHA"
echo "-X github.com/pyroscope-io/pyroscope/pkg/build.PyspyGitSHA=$PYSPY_GIT_SHA"
echo "-X github.com/pyroscope-io/pyroscope/pkg/build.PhpspyGitSHA=$PHPSPY_GIT_SHA"
