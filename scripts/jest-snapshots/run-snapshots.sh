#!/usr/bin/env bash

#set -euo pipefail

set -x

#tmp=$(mktemp -d)
#trap "rm -R $tmp" EXIT
#
#
#cp yarn.lock "$tmp"
#
# remove cypress from (temporary) package.json
# that's so that yarn install is faster
#cat package.json | \
#  sed '/cypress/d' |
#  sed '/postinstall/d' > "$tmp/package.json"
#

#yarn install --cwd "$tmp"

yarn install
RUN_SNAPSHOTS=true yarn test --testNamePattern='group:snapshot' --verbose

