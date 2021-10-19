#!/usr/bin/env bash

set -euo pipefail


updateArg=""

if [ "$UPDATE_SNAPSHOTS" = true ]; then
  updateArg="-u"
fi

apt update -y
apt install fontconfig -y

yarn install
RUN_SNAPSHOTS=true yarn test --testNamePattern='group:snapshot' --verbose "$updateArg"

