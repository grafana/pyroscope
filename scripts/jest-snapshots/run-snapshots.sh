#!/usr/bin/env bash

set -euo pipefail


updateArg=""

if [ "$UPDATE_SNAPSHOTS" = true ]; then
  updateArg="-u"
fi


apt update -y
apt install fontconfig -y

if [ $(dpkg --print-architecture) = "arm64" ]; then
# The chromium binary is not available for arm64.
  apt install chromium -y

  # To build canvas from source
  apt install python pkg-config -y
  apt install -y build-essential libcairo2-dev libpango1.0-dev libjpeg-dev libgif-dev librsvg2-dev
fi

# ignore-engines due to
# warning webpack-plugin-serve@1.5.0: Invalid bin field for "webpack-plugin-serve".
# error eslint-import-resolver-webpack@0.13.1: The engine "node" is incompatible with this module. Expected version "^16 || ^15 || ^14 || ^13 || ^12 || ^11 || ^10 || ^9 || ^8 || ^7 || ^6". Got "17.0.1"
# error Found incompatible module.
yarn install --ignore-engines --frozen-lockfile
RUN_SNAPSHOTS=true yarn test --testNamePattern='group:snapshot' --verbose "$updateArg" --runInBand

