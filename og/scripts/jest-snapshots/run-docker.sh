#!/usr/bin/env bash

set -euo pipefail

UPDATE_SNAPSHOTS="${UPDATE_SNAPSHOTS:-}"

docker run \
  -it --rm \
  --entrypoint=/bin/bash \
  -e UPDATE_SNAPSHOTS="$UPDATE_SNAPSHOTS" \
  -v $PWD:/app \
  -v /app/node_modules \
  -w /app \
  node:14.17-slim ./scripts/jest-snapshots/run-snapshots.sh
