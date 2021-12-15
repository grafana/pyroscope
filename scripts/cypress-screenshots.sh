#!/usr/bin/env bash

set -euo pipefail

CYPRESS_updateSnapshots="${CYPRESS_updateSnapshots:-true}"

# we use net=host since cypress will hit http://localhost:4040
# and run with user 1000 since it
docker run \
  --net=host \
  --user 1000:1000 \
  -it --rm \
  -e CYPRESS_VIDEO=true \
  -e CYPRESS_COMPARE_SNAPSHOTS=true \
  -e CYPRESS_updateSnapshots="$CYPRESS_updateSnapshots" \
  -v $PWD:/cypress -w /cypress cypress/included:8.6.0 "$@"

