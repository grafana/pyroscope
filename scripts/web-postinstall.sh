#!/bin/sh

# web-postinstall.sh
# to be run by yarn/npm after installing

# don't run where git is not present (probably CI)
if command -v git; then
  # makes git blame ignore commits that are purely reformatting code
  git config blame.ignoreRevsFile .git-blame-ignore-revs
fi
